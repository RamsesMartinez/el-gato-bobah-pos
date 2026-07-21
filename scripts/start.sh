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
echo "▶ Iniciando API (Go) en :$BACK_PORT…"
bash scripts/dev-api.sh &
API_PID=$!
trap 'echo; echo "Deteniendo…"; kill $API_PID 2>/dev/null; exit 0' INT TERM

echo "▶ Iniciando web (Vite) en :$FRONT_PORT (proxy /api → :$BACK_PORT)…"
cd web && bun run dev

kill $API_PID 2>/dev/null || true
