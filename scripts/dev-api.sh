#!/usr/bin/env bash
# Lanza la API en dev. Los SECRETOS (JWT_SECRET, ADMIN_*) los carga el binario desde
# deploy/.env con parseo LITERAL (godotenv) — soporta #, $, espacios, comillas sin que
# el shell los interprete. Este script solo fija la CONEXIÓN local (postgres/redis del
# compose dev, puertos en PG_PORT/REDIS_PORT) y la ruta del .env.
# Uso: dev-api.sh [air|reset-admin|reset-password USUARIO@SLUG]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ ! -f "$ROOT/deploy/.env" ]; then
  echo "Falta deploy/.env — corre 'make check-env' para crearlo y configúralo." >&2
  exit 1
fi

# Conexión + entorno de dev (tiene prioridad; godotenv no sobrescribe lo ya definido).
export DATABASE_URL="${DEV_DATABASE_URL:-postgres://gatobobah:gatobobah@localhost:${PG_PORT:-5490}/gatobobah?sslmode=disable}"
export REDIS_URL="${DEV_REDIS_URL:-redis://localhost:${REDIS_PORT:-6390}}"
export APP_ENV=development
export LOG_DIR="${LOG_DIR:-$ROOT/logs}"
export SMTP_HOST="${SMTP_HOST:-localhost}"
export SMTP_PORT="${SMTP_PORT:-${MAILPIT_SMTP_PORT:-1095}}"  # SMTP del Mailpit del compose dev
export MAIL_FROM="${MAIL_FROM:-no-reply@gatobobah.local}"
export APP_BASE_URL="${APP_BASE_URL:-http://localhost:${FRONTEND_PORT:-3000}}"
# el binario carga los secretos de aquí (JWT_SECRET, ADMIN_*, etc.) de forma literal
export ENV_FILE="$ROOT/deploy/.env"

# La API de dev sirve con los MISMOS roles que producción, no como owner. El owner salta RLS y
# todos los GRANT, así que una API local que sirve como él devuelve filas de todas las empresas
# (el selector de plataformas salía duplicado) y nunca ve un 42501 que en producción tumba el
# primer request. Con estas dos URLs, el arranque corre assertRLSEnforced y AssertPlatformGrants
# igual que allá y se niega a servir si el aislamiento no es real.
valor_env() { grep -E "^$1=" "$ROOT/deploy/.env" | head -1 | cut -d= -f2- || true; }
APP_DB_PASSWORD_DEV="$(valor_env APP_DB_PASSWORD)"
PLATFORM_DB_PASSWORD_DEV="$(valor_env PLATFORM_DB_PASSWORD)"
for par in "APP_DB_PASSWORD:$APP_DB_PASSWORD_DEV" "PLATFORM_DB_PASSWORD:$PLATFORM_DB_PASSWORD_DEV"; do
  nombre="${par%%:*}"; valor="${par#*:}"
  # Solo caracteres que no hay que escapar dentro de una URL: openssl rand -hex 24 ya los cumple.
  if [[ -z "$valor" || "$valor" == cambia-esto* || ! "$valor" =~ ^[A-Za-z0-9._~-]+$ ]]; then
    echo "Falta $nombre en deploy/.env (vacío, de ejemplo o con caracteres fuera de [A-Za-z0-9._~-])." >&2
    echo "Genéralo con: openssl rand -hex 24 — ver AGENTS.md, «La base local se comporta como producción»." >&2
    exit 1
  fi
done
HOST_PG="localhost:${PG_PORT:-5490}"
export APP_DATABASE_URL="${DEV_APP_DATABASE_URL:-postgres://gatobobah_app:${APP_DB_PASSWORD_DEV}@${HOST_PG}/gatobobah?sslmode=disable}"
export PLATFORM_DATABASE_URL="${DEV_PLATFORM_DATABASE_URL:-postgres://gatobobah_platform:${PLATFORM_DB_PASSWORD_DEV}@${HOST_PG}/gatobobah?sslmode=disable}"

cd "$ROOT/server"

if [ "${1:-}" = "air" ]; then exec "$(go env GOPATH)/bin/air"; fi

# Se compila a ./tmp/api (la misma ruta que usa air) en vez de `go run`: el binario que `go run`
# deja en %TEMP%\go-build… lo bloquea SIEMPRE Smart App Control de Windows 11 —"Una directiva de
# Control de aplicaciones bloqueó este archivo"— y la API muere al arrancar sin decir por qué.
#
# OJO: compilar a una ruta del repo NO es un arreglo, solo mejora la probabilidad. Smart App
# Control decide por reputación de CADA binario, así que una recompilación puede quedar bloqueada
# aunque la anterior corriera desde esta misma ruta. Cuando pase, la salida es levantar la API en
# contenedor (ver AGENTS.md §7), no seguir recompilando a ver si esta vez sí.
go build -o ./tmp/api ./cmd/api

case "${1:-}" in
  reset-admin)    exec ./tmp/api -reset-admin ;;
  reset-password) exec ./tmp/api -reset-password="${2:?uso: make reset-password user=usuario@slug}" ;;
  *)              exec ./tmp/api ;;
esac
