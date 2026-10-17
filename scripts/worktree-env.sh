#!/usr/bin/env bash
# Le da a un worktree su propia infraestructura de desarrollo: Postgres, Postgres de pruebas, Redis,
# mailpit, API y web, cada uno en un puerto que sale de su número. Escribe .env.worktree.local
# (ignorado por git), que leen el Makefile, start.sh, dev-api.sh y ci-local.sh.
#
#   make worktree-env n=35      # worktree de la feature 035
#
# POR QUÉ. docker compose toma el nombre del proyecto de la carpeta del compose (`deploy/`), que es
# la misma en todos los worktrees: cada `make start` caía en los MISMOS contenedores
# (deploy-postgres-1, deploy-redis-1). Una rama migraba la base de otra —dos ramas con la misma
# migración 0089 dejaban a la otra API sin arrancar—, el caché del menú y los bloqueos de login se
# compartían en Redis, y las pruebas de integración, que fijan las contraseñas de los roles de todo
# el servidor de Postgres, tiraban la API local que usaba ese mismo servidor.
#
# La carpeta principal no se toca: sin este archivo todo sigue con los puertos de siempre.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
N="${1:-}"
OUT="${WORKTREE_ENV_OUT:-$ROOT/.env.worktree.local}"

if ! [[ "$N" =~ ^[0-9]{1,2}$ ]] || [ "$((10#$N))" -lt 1 ]; then
  echo "Uso: make worktree-env n=NN (1 a 99, el número de la feature: 035 → n=35)." >&2
  exit 1
fi
N=$((10#$N))

# La carpeta principal comparte .git con todos los worktrees; en ella, --git-dir y --git-common-dir
# son el mismo. Ahí se queda la base de siempre.
if [ "$(git -C "$ROOT" rev-parse --git-dir)" = "$(git -C "$ROOT" rev-parse --git-common-dir)" ] \
   && [ -z "${WORKTREE_ENV_FORCE:-}" ]; then
  echo "Esta es la carpeta principal: se queda con la base y los puertos de siempre." >&2
  exit 1
fi

pg=$((5500 + N)) pgtest=$((5600 + N)) ci=$((5700 + N)) redis=$((6400 + N))
mail_http=$((8200 + N)) mail_smtp=$((1100 + N)) api=$((8000 + N)) web=$((3000 + N))

# Un puerto ocupado por OTRA cosa se dice antes de escribir nada; uno ocupado por los contenedores
# de este mismo proyecto es el caso normal de volver a correrlo.
busy() { ss -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]$1\$"; }
mine="$(docker ps --filter "label=com.docker.compose.project=egb$N" --format '{{.Ports}}' 2>/dev/null || true)"
for p in $pg $pgtest $redis $mail_http $mail_smtp; do
  if busy "$p" && ! grep -q ":$p->" <<<"$mine"; then
    echo "El puerto $p ya lo usa otro proceso; elige otro número o libéralo." >&2
    exit 1
  fi
done

cat > "$OUT" <<EOF
# Generado por scripts/worktree-env.sh $N — infraestructura propia de este worktree. No se versiona.
COMPOSE_PROJECT_NAME=egb$N
PG_PORT=$pg
PG_TEST_PORT=$pgtest
CI_PG_PORT=$ci
REDIS_PORT=$redis
MAILPIT_HTTP_PORT=$mail_http
MAILPIT_SMTP_PORT=$mail_smtp
BACKEND_PORT=$api
# El binario de la API lee PORT; start.sh lo deriva de BACKEND_PORT, dev-api.sh no.
PORT=$api
FRONTEND_PORT=$web
TEST_DATABASE_URL=postgres://gatobobah:gatobobah@localhost:$pgtest/gatobobah_test?sslmode=disable
EOF

echo "Listo: $OUT"
echo "  Postgres :$pg  ·  pruebas :$pgtest  ·  Redis :$redis  ·  API :$api  ·  web :$web  (proyecto egb$N)"
echo "Siguiente: make start (levanta todo con estos puertos). Las credenciales salen de deploy/.env"
echo "de ESTE worktree; si no existe, make check-env lo crea con contraseñas nuevas."
