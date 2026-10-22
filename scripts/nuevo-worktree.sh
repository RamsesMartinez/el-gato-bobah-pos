#!/usr/bin/env bash
# nuevo-worktree.sh — prepara el worktree de una feature nueva, de cero a la API respondiendo.
#
#   bash scripts/nuevo-worktree.sh 041 subidas-sin-corte
#
# Deja: la carpeta ../egb-041-subidas-sin-corte con la rama 041-subidas-sin-corte desde develop,
# su infraestructura propia (proyecto de compose egb41, puertos 55NN/56NN/64NN/80NN/30NN; ver
# `make worktree-env`), su deploy/.env con contraseñas nuevas, las dependencias del front y
# `make start` corriendo en segundo plano hasta que /readyz responde.
#
# Por qué existe: arrancar la 041 a mano costó cinco vueltas — deploy/.env no existía, `vite` no
# estaba porque node_modules no viaja con el worktree, y la API se negó a arrancar dos veces por
# ADMIN_PASSWORD y ADMIN_PIN de ejemplo. Cada uno se veía solo al fallar el anterior.
set -euo pipefail

NUM="${1:-}"; SLUG="${2:-}"
if [[ ! "$NUM" =~ ^[0-9]{3}$ || -z "$SLUG" || ! "$SLUG" =~ ^[a-z0-9-]+$ ]]; then
  echo "Uso: bash scripts/nuevo-worktree.sh NNN slug-en-minusculas   (ej. 041 subidas-sin-corte)" >&2
  exit 2
fi
N=$((10#$NUM))
BRANCH="$NUM-$SLUG"
REPO="$(git rev-parse --show-toplevel)"
DEST="$(dirname "$REPO")/egb-$NUM-$SLUG"
LOG="/tmp/egb$N-start.log"

[[ -e "$DEST" ]] && { echo "Ya existe $DEST: úsalo o quítalo con 'git worktree remove'." >&2; exit 1; }
git -C "$REPO" show-ref --quiet "refs/heads/$BRANCH" && { echo "La rama $BRANCH ya existe." >&2; exit 1; }

echo "▶ Worktree $DEST (rama $BRANCH desde origin/develop)"
git -C "$REPO" fetch -q origin develop
git -C "$REPO" worktree add -q "$DEST" -b "$BRANCH" origin/develop
cd "$DEST"
# Lo que exporta el Makefile del worktree DESDE el que se corre (puertos, DEV_DATABASE_URL) llega
# heredado, y el Makefile nuevo lo respeta con `?=`: la API nueva migraba la base del otro worktree y le
# cambiaba la contraseña al rol de la app (pasó al crear la 044 desde la 042). Lo vigila
# scripts/tests/nuevo-worktree-test.sh: cada `export` del Makefile tiene que estar aquí.
unset BACKEND_PORT CI_PG_PORT COMPOSE_PROJECT_NAME DEV_DATABASE_URL DEV_JWT_SECRET DEV_REDIS_URL
unset FRONTEND_PORT MAILPIT_HTTP_PORT MAILPIT_SMTP_PORT PG_PORT PG_TEST_PORT PORT REDIS_PORT TEST_DATABASE_URL

echo "▶ Infraestructura propia (n=$N)"
make -s worktree-env n="$N"

echo "▶ deploy/.env"
make -s check-env >/dev/null 2>&1 || true   # la primera vez lo crea y avisa de lo que falta
# check-env genera los secretos, pero deja el admin inicial con «cambia-esto»: la API se niega a
# arrancar así (bootstrap admin). En local se ponen valores propios de este worktree.
# POSTGRES_PASSWORD es la de producción; el compose de desarrollo usa «gatobobah» fijo.
if grep -q '^POSTGRES_PASSWORD=cambia-esto' deploy/.env; then
  sed -i "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=gatobobah|" deploy/.env
fi
if grep -q '^ADMIN_PASSWORD=cambia-esto' deploy/.env; then
  sed -i "s|^ADMIN_PASSWORD=.*|ADMIN_PASSWORD=Local-$NUM-$(openssl rand -hex 3)!|" deploy/.env
fi
if grep -q '^ADMIN_PIN=cambia-esto' deploy/.env; then
  # Cuatro dígitos sin repetir ni ser secuencia: la API rechaza 1234, 0000 y parecidos.
  PIN="$(shuf -i 0-9 -n 4 | tr -d '\n')"
  while [[ "0123456789" == *"$PIN"* || "9876543210" == *"$PIN"* ]]; do PIN="$(shuf -i 0-9 -n 4 | tr -d '\n')"; done
  sed -i "s|^ADMIN_PIN=.*|ADMIN_PIN=$PIN|" deploy/.env
fi
make -s check-env

echo "▶ Dependencias del front (node_modules no viaja con el worktree)"
(cd web && bun install --silent)

echo "▶ make start en segundo plano (log: $LOG)"
nohup make start > "$LOG" 2>&1 &
API_PORT=$((8000 + N))
for _ in $(seq 1 60); do
  if curl -fs "localhost:$API_PORT/readyz" >/dev/null 2>&1; then break; fi
  sleep 2
done
if ! curl -fs "localhost:$API_PORT/readyz" >/dev/null 2>&1; then
  echo "✗ La API no respondió en :$API_PORT. Últimos errores:" >&2
  grep '"level":"ERROR"' "$LOG" | tail -3 >&2 || tail -15 "$LOG" >&2
  exit 1
fi

cat <<EOF
✅ Listo: $DEST
   API  http://localhost:$API_PORT   (readyz ok)
   Web  http://localhost:$((3000 + N))
   Entrar: usuario admin · empresa $(grep '^COMPANY_SLUG=' deploy/.env | cut -d= -f2) · contraseña ADMIN_PASSWORD de $DEST/deploy/.env
   Detener: (cd $DEST && make stop)   · log: $LOG
   Datos de producción en local (opcional, con la API detenida): make db-restaurar dump=…
Siguiente: cd $DEST && claude   y ahí /speckit-specify …
EOF
