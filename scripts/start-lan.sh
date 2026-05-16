#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE=(docker compose -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.prod.yml" --profile production)

printf 'Starting LokLingo LAN stack...\n'
"${COMPOSE[@]}" up -d --build --remove-orphans
printf 'Waiting for services to become healthy...\n'
"${COMPOSE[@]}" up -d --wait
printf 'Running health check...\n'
"$ROOT_DIR/scripts/check-health.sh"
