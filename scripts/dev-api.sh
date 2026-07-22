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
# Email a Mailpit del compose dev (localhost:1026; UI en http://localhost:8026). La API sirve
# como owner en dev (single-tenant, RLS moot con una empresa); el aislamiento por RLS se prueba
# en los tests de integración con el rol gatobobah_app. Link de reset apunta al front dev.
export SMTP_HOST="${SMTP_HOST:-localhost}"
export SMTP_PORT="${SMTP_PORT:-1026}"  # Mailpit del compose dev publica 1025→1026 (no estándar)
export MAIL_FROM="${MAIL_FROM:-no-reply@gatobobah.local}"
export APP_BASE_URL="${APP_BASE_URL:-http://localhost:${FRONTEND_PORT:-3000}}"
# el binario carga los secretos de aquí (JWT_SECRET, ADMIN_*, etc.) de forma literal
export ENV_FILE="$ROOT/deploy/.env"

cd "$ROOT/server"
case "${1:-}" in
  air)         exec "$(go env GOPATH)/bin/air" ;;
  reset-admin) exec go run ./cmd/api -reset-admin ;;
  *)           exec go run ./cmd/api ;;
esac
