#!/usr/bin/env sh
set -eu

metrics_proxy_url="${METRICS_PROXY_URL:-http://localhost:18081}"
prometheus_url="${PROMETHEUS_URL:-http://localhost:19090}"
grafana_url="${GRAFANA_URL:-http://localhost:13030}"
grafana_user="${GRAFANA_ADMIN_USER:-admin}"
grafana_password="${GRAFANA_ADMIN_PASSWORD:-admin}"
internal_token="${INTERNAL_TOKEN:-}"

if [ -z "$internal_token" ]; then
  echo "ERROR: INTERNAL_TOKEN must be set so the metrics proxy can inject it"
  exit 1
fi

echo "Checking metrics proxy health: ${metrics_proxy_url}/healthz"
proxy_health="$(curl -fsS "${metrics_proxy_url}/healthz")"
echo "$proxy_health" | grep '^ok' >/dev/null

echo "Checking Prometheus readiness: ${prometheus_url}/-/ready"
prometheus_ready="$(curl -fsS "${prometheus_url}/-/ready")"
echo "$prometheus_ready" | grep 'Ready\|OK' >/dev/null

echo "Checking Prometheus scrape target health through proxy"
scrape_target="$(curl -fsS "${prometheus_url}/api/v1/query?query=up%7Bjob%3D%22loklingo-metrics-proxy%22%7D")"

python3 - "$scrape_target" <<'PYEOF'
import json, sys
payload = json.loads(sys.argv[1])
if payload.get("status") != "success":
    raise SystemExit("Prometheus query did not succeed")
result = payload.get("data", {}).get("result", [])
if not result:
    raise SystemExit("Prometheus did not return a scrape target result")
if str(result[0].get("value", [None, None])[1]) != "1":
    raise SystemExit("Prometheus scrape target is not healthy")
PYEOF

echo "Checking Grafana health: ${grafana_url}/api/health"
grafana_health="$(curl -fsS -u "${grafana_user}:${grafana_password}" "${grafana_url}/api/health")"
python3 - "$grafana_health" <<'PYEOF'
import json, sys
payload = json.loads(sys.argv[1])
if payload.get("database") != "ok":
    raise SystemExit("Grafana database not ready")
PYEOF

echo "Checking Grafana datasource provisioning"
datasources_json="$(curl -fsS -u "${grafana_user}:${grafana_password}" "${grafana_url}/api/datasources")"
python3 - <<'PYEOF' "$datasources_json"
import json, sys
payload = json.loads(sys.argv[1])
if not any(ds.get("name") == "LokLingo Prometheus" for ds in payload):
    raise SystemExit("LokLingo Prometheus datasource not found")
PYEOF

echo "Checking Grafana dashboard provisioning"
search_json="$(curl -fsS -u "${grafana_user}:${grafana_password}" "${grafana_url}/api/search?type=dash-db&query=LokLingo")"
python3 - <<'PYEOF' "$search_json"
import json, sys
payload = json.loads(sys.argv[1])
want = {
    "LokLingo Reliability Overview",
    "LokLingo Incident Timeline Companion",
}
found = {item.get("title") for item in payload}
missing = sorted(want - found)
if missing:
    raise SystemExit("Missing dashboards: " + ", ".join(missing))
PYEOF

echo "Observability smoke test passed"
