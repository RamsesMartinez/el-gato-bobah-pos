#!/usr/bin/env bash
# Levanta todo el stack de desarrollo: postgres+redis (docker), API (Go) y web (Vite).
# Pregunta los puertos de API y web (con defaults) y DETECTA puertos ocupados antes de
# levantar, ofreciendo el siguiente libre. Se puede fijar sin preguntar con las env
# BACKEND_PORT / FRONTEND_PORT (o en entornos no interactivos se usan esos defaults).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

bash scripts/check.sh || exit 1

# --- Selección de puertos (API + web) ---------------------------------------
if ! command -v lsof >/dev/null 2>&1; then
  echo "⚠ 'lsof' no disponible: no puedo detectar puertos ocupados (uso los valores tal cual)."
fi
port_busy() { # 0 = ocupado (hay un proceso escuchando), 1 = libre
  command -v lsof >/dev/null 2>&1 || return 1
  lsof -iTCP:"$1" -sTCP:LISTEN -n -P >/dev/null 2>&1
}
free_port() { # primer puerto libre >= $1
  local p="$1"
  while port_busy "$p"; do p=$((p + 1)); done
  echo "$p"
}

# defaults (override con env) ya ajustados al primer puerto libre → el prompt sugiere uno usable
BACK_PORT="$(free_port "${BACKEND_PORT:-8080}")"
FRONT_PORT="$(free_port "${FRONTEND_PORT:-3000}")"

# pregunta solo si hay terminal interactiva y no se fijó por env
if [ -t 0 ] && [ -z "${BACKEND_PORT:-}" ]; then
  read -r -p "Puerto del backend (API) [$BACK_PORT]: " ans || true
  BACK_PORT="${ans:-$BACK_PORT}"
fi
if [ -t 0 ] && [ -z "${FRONTEND_PORT:-}" ]; then
  read -r -p "Puerto del frontend (web) [$FRONT_PORT]: " ans || true
  FRONT_PORT="${ans:-$FRONT_PORT}"
fi

# revalida lo elegido (pudieron teclear uno ocupado) y evita colisión API == web
if port_busy "$BACK_PORT"; then
  alt="$(free_port $((BACK_PORT + 1)))"
  echo "⚠ El puerto $BACK_PORT (API) está ocupado → uso $alt."
  BACK_PORT="$alt"
fi
if [ "$FRONT_PORT" = "$BACK_PORT" ] || port_busy "$FRONT_PORT"; then
  alt="$(free_port $((FRONT_PORT + 1)))"
  while [ "$alt" = "$BACK_PORT" ]; do alt="$(free_port $((alt + 1)))"; done
  echo "⚠ El puerto $FRONT_PORT (web) no está disponible → uso $alt."
  FRONT_PORT="$alt"
fi

# El binario Go lee PORT (config.Port); Vite lee BACKEND_PORT (target del proxy /api) y
# FRONTEND_PORT (puerto del dev server). Se exportan para que los hijos los hereden.
export PORT="$BACK_PORT"
export BACKEND_PORT="$BACK_PORT"
export FRONTEND_PORT="$FRONT_PORT"

# --- Puertos de la infra (postgres/redis/mailpit) ---------------------------
# No son fijos: el 5433/6380 "no estándar" de antes igual chocaba con el postgres/redis de
# otros compose de la misma máquina. Si el contenedor ya está vivo se reusa EL PUERTO QUE YA
# PUBLICA (mover el puerto lo recrearía en cada arranque); si no, el primero libre desde el
# default. El compose y dev-api.sh los leen de estas env.
infra_port() { # $1=servicio compose  $2=puerto interno  $3=default
  local pub
  pub="$(docker compose -f deploy/docker-compose.dev.yml port "$1" "$2" 2>/dev/null | sed 's/.*://')"
  if [ -n "$pub" ]; then echo "$pub"; else free_port "$3"; fi
}
export PG_PORT="${PG_PORT:-$(infra_port postgres 5432 5490)}"
export REDIS_PORT="${REDIS_PORT:-$(infra_port redis 6379 6390)}"
export MAILPIT_HTTP_PORT="${MAILPIT_HTTP_PORT:-$(infra_port mailpit 8025 8095)}"
export MAILPIT_SMTP_PORT="${MAILPIT_SMTP_PORT:-$(infra_port mailpit 1025 1095)}"

echo "▶ Levantando postgres (:${PG_PORT}) + redis (:${REDIS_PORT}) + mailpit (:${MAILPIT_HTTP_PORT})…"
docker compose -f deploy/docker-compose.dev.yml up -d

echo "▶ Esperando a postgres…"
PG_ID=$(docker compose -f deploy/docker-compose.dev.yml ps -q postgres)
for _ in $(seq 1 60); do
  [ "$(docker inspect --format '{{.State.Health.Status}}' "$PG_ID" 2>/dev/null)" = "healthy" ] && break
  sleep 1
done

# API en background; usa scripts/dev-api.sh (secretos desde deploy/.env + conexión dev).
# Se detiene junto con la web al salir (Ctrl-C).
echo "> Iniciando API (Go) en :${BACK_PORT}"
bash scripts/dev-api.sh &
API_PID=$!
trap 'echo; echo "Deteniendo..."; kill ${API_PID} 2>/dev/null; exit 0' INT TERM

echo "> Iniciando web (Vite) en :${FRONT_PORT} (proxy /api -> :${BACK_PORT})"
cd web && bun run dev

kill $API_PID 2>/dev/null || true
