#!/usr/bin/env bash
# El menú público en local (spec 097): construye y lo sirve con `wrangler pages dev` contra la API
# del worktree, en MENU_PORT (8700+NN). Sin MENU_EDGE_KEY: la API local limita por la IP de quien llama.
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$(cd .. && pwd)"
if [ -f "$ROOT/.env.worktree.local" ]; then set -a; . "$ROOT/.env.worktree.local"; set +a; fi
bun menu/build.ts
exec bun x wrangler pages dev dist-menu --port "${MENU_PORT:-8790}" --ip 0.0.0.0 \
  --binding "API_ORIGIN=http://localhost:${BACKEND_PORT:-8080}" \
  --binding "MEDIA_ORIGIN=http://localhost:${MEDIA_PORT:-9390}"
