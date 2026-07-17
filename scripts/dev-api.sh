#!/usr/bin/env bash
# Lanza la API en dev. Los SECRETOS (JWT_SECRET, ADMIN_*) los carga el binario desde
# deploy/.env con parseo LITERAL (godotenv) — soporta #, $, espacios, comillas sin que
# el shell los interprete. Este script solo fija la CONEXIÓN local (postgres/redis del
# compose dev en 5433/6380) y la ruta del .env.
# Uso: dev-api.sh [air|reset-admin]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ ! -f "$ROOT/deploy/.env" ]; then
  echo "Falta deploy/.env — corre 'make check-env' para crearlo y configúralo." >&2
  exit 1
fi

# Conexión + entorno de dev (tiene prioridad; godotenv no sobrescribe lo ya definido).
export DATABASE_URL="${DEV_DATABASE_URL:-postgres://gatobobah:gatobobah@localhost:5433/gatobobah?sslmode=disable}"
export REDIS_URL="${DEV_REDIS_URL:-redis://localhost:6380}"
export APP_ENV=development
export LOG_DIR="${LOG_DIR:-$ROOT/logs}"
# el binario carga los secretos de aquí (JWT_SECRET, ADMIN_*, etc.) de forma literal
export ENV_FILE="$ROOT/deploy/.env"

cd "$ROOT/server"
case "${1:-}" in
  air)         exec "$(go env GOPATH)/bin/air" ;;
  reset-admin) exec go run ./cmd/api -reset-admin ;;
  *)           exec go run ./cmd/api ;;
esac
