#!/usr/bin/env bash
# LokLingo backup and restore helpers for self-hosted deployments.
#
# Backs up:
#   - Redis RDB/AOF via redis-cli SAVE + dump (when REDIS_URL is local/reachable)
#   - Optional Postgres dump when POSTGRES_DSN is set
#   - Shared OCR upload volume contents
#   - Active .env (redacted copy optional)
#
# Usage:
#   bash scripts/backup-restore.sh backup [outdir]
#   bash scripts/backup-restore.sh restore <backup-dir>
#   bash scripts/backup-restore.sh list [backup-root]

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

CMD="${1:-}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
BACKUP_ROOT="${LOKLINGO_BACKUP_ROOT:-${ROOT_DIR}/.backups}"

log() { printf '[backup %s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { log "missing required command: $1"; exit 1; }
}

backup_now() {
  local outdir="${1:-${BACKUP_ROOT}/${STAMP}}"
  mkdir -p "$outdir"
  log "writing backup to $outdir"

  # Capture compose config (no secrets expanded beyond current env).
  if command -v docker >/dev/null 2>&1; then
    docker compose -f docker-compose.yml config >"$outdir/compose.config.yml" 2>/dev/null || true
    docker ps -a --filter 'name=loklingo-' --format '{{.Names}} {{.Status}} {{.Image}}' >"$outdir/containers.txt" 2>/dev/null || true
  fi

  # Env template (never store live secrets in git; this stays local under .backups/)
  if [[ -f .env ]]; then
    # Redact obvious secret values for a shareable inventory; keep full encrypted offline separately.
    sed -E \
      -e 's/(PASSWORD|TOKEN|API_KEY|SECRET)=.*/\1=***REDACTED***/I' \
      .env >"$outdir/env.redacted"
    if [[ "${BACKUP_INCLUDE_SECRETS:-0}" == "1" ]]; then
      cp .env "$outdir/env.full"
      chmod 600 "$outdir/env.full"
      log "included full .env (BACKUP_INCLUDE_SECRETS=1) — store offline only"
    fi
  fi

  # Redis dump when redis-cli available
  if [[ -n "${REDIS_URL:-}" ]] && command -v redis-cli >/dev/null 2>&1; then
    log "redis SAVE via REDIS_URL"
    redis-cli -u "$REDIS_URL" SAVE >/dev/null || true
    redis-cli -u "$REDIS_URL" --rdb "$outdir/dump.rdb" >/dev/null 2>&1 || log "redis RDB export skipped/failed"
  else
    log "skip redis dump (set REDIS_URL and install redis-cli for full backup)"
  fi

  # Postgres dump
  if [[ -n "${POSTGRES_DSN:-}" ]] && command -v pg_dump >/dev/null 2>&1; then
    log "pg_dump"
    pg_dump "$POSTGRES_DSN" --no-owner --format=custom -f "$outdir/postgres.dump" || log "pg_dump failed"
  else
    log "skip postgres dump (set POSTGRES_DSN and install pg_dump)"
  fi

  # Shared uploads volume
  if command -v docker >/dev/null 2>&1; then
    vol="$(docker volume ls -q | grep -E 'loklingo.*pdf_uploads|pdf_uploads' | head -n1 || true)"
    if [[ -n "$vol" ]]; then
      log "exporting volume $vol"
      docker run --rm -v "${vol}:/data:ro" -v "${outdir}:/backup" alpine:3.20 \
        tar czf /backup/pdf_uploads.tar.gz -C /data . || log "volume export failed"
    fi
  fi

  printf '%s\n' "$STAMP" >"$outdir/MANIFEST.txt"
  printf 'created_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"$outdir/MANIFEST.txt"
  log "backup complete: $outdir"
  printf '%s\n' "$outdir"
}

restore_from() {
  local src="${1:-}"
  [[ -n "$src" && -d "$src" ]] || { log "usage: restore <backup-dir>"; exit 1; }
  log "restoring from $src — this may overwrite live data"
  if [[ "${CONFIRM_RESTORE:-}" != "yes" ]]; then
    log "refusing: set CONFIRM_RESTORE=yes to proceed"
    exit 2
  fi

  if [[ -f "$src/postgres.dump" && -n "${POSTGRES_DSN:-}" ]] && command -v pg_restore >/dev/null 2>&1; then
    log "pg_restore"
    pg_restore --clean --if-exists -d "$POSTGRES_DSN" "$src/postgres.dump" || true
  fi

  if [[ -f "$src/dump.rdb" && -n "${REDIS_URL:-}" ]]; then
    log "Redis RDB restore is host-specific — copy dump.rdb into Redis data dir and restart Redis manually"
    log "saved path: $src/dump.rdb"
  fi

  if [[ -f "$src/pdf_uploads.tar.gz" ]] && command -v docker >/dev/null 2>&1; then
    vol="$(docker volume ls -q | grep -E 'loklingo.*pdf_uploads|pdf_uploads' | head -n1 || true)"
    if [[ -n "$vol" ]]; then
      log "restoring volume $vol"
      docker run --rm -v "${vol}:/data" -v "${src}:/backup:ro" alpine:3.20 \
        sh -c 'rm -rf /data/*; tar xzf /backup/pdf_uploads.tar.gz -C /data' || true
    fi
  fi

  log "restore finished — restart stack: docker compose up -d"
}

list_backups() {
  local root="${1:-$BACKUP_ROOT}"
  if [[ ! -d "$root" ]]; then
    log "no backups under $root"
    return 0
  fi
  find "$root" -mindepth 1 -maxdepth 1 -type d | sort
}

case "$CMD" in
  backup) backup_now "${2:-}" ;;
  restore) restore_from "${2:-}" ;;
  list) list_backups "${2:-}" ;;
  *)
    cat <<'EOF'
Usage:
  bash scripts/backup-restore.sh backup [outdir]
  bash scripts/backup-restore.sh restore <backup-dir>   # requires CONFIRM_RESTORE=yes
  bash scripts/backup-restore.sh list [backup-root]
EOF
    exit 1
    ;;
esac
