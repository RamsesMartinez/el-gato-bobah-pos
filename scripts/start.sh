#!/usr/bin/env bash
# Levanta todo el stack de desarrollo: postgres+redis (docker), API (Go) y web (Vite).
# Corre el preflight primero; si algo falta, lo dice y aborta.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

bash scripts/check.sh || exit 1

echo "▶ Levantando postgres + redis…"
docker compose -f deploy/docker-compose.dev.yml up -d

echo "▶ Esperando a postgres…"
PG_ID=$(docker compose -f deploy/docker-compose.dev.yml ps -q postgres)
for _ in $(seq 1 60); do
  [ "$(docker inspect --format '{{.State.Health.Status}}' "$PG_ID" 2>/dev/null)" = "healthy" ] && break
  sleep 1
done

# API en background; usa scripts/dev-api.sh (secretos desde deploy/.env + conexión dev).
# Se detiene junto con la web al salir (Ctrl-C).
echo "▶ Iniciando API (Go) en :8080…"
bash scripts/dev-api.sh &
API_PID=$!
trap 'echo; echo "Deteniendo…"; kill $API_PID 2>/dev/null; exit 0' INT TERM

echo "▶ Iniciando web (Vite) en :3000…"
cd web && bun run dev

kill $API_PID 2>/dev/null || true
