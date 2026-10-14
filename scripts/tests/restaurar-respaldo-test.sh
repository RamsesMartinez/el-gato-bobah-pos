#!/usr/bin/env bash
# Prueba de scripts/restaurar-respaldo.sh: a qué contenedor le habla según desde dónde se corre.
#
# POR QUÉ. El 2026-10-13 un `make db-restaurar` corrido desde un worktree con infraestructura propia
# borró y recreó la base de la carpeta principal: el script usaba deploy-postgres-1 sin mirar
# .env.worktree.local. Esta prueba arma un repositorio con un worktree y un `docker` falso que solo
# anota a qué contenedor le hablan; nunca toca un contenedor de verdad.
#
#   bash scripts/tests/restaurar-respaldo-test.sh
set -euo pipefail

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fallas=0
falla() { echo "FALLA: $*" >&2; fallas=$((fallas + 1)); }

# docker falso: anota la llamada y falla, para que el script se detenga en el primer contacto.
mkdir -p "$TMP/bin"
cat > "$TMP/bin/docker" <<'EOF'
#!/usr/bin/env bash
echo "$*" >> "$DOCKER_LOG"
exit 1
EOF
chmod +x "$TMP/bin/docker"

git init -q "$TMP/principal"
git -C "$TMP/principal" -c user.email=t@t -c user.name=t commit -q --allow-empty -m inicio
git -C "$TMP/principal" worktree add -q "$TMP/wt" 2>/dev/null
for d in "$TMP/principal" "$TMP/wt"; do
  mkdir -p "$d/scripts"
  cp "$SRC/scripts/restaurar-respaldo.sh" "$SRC/scripts/worktree-env-load.sh" "$d/scripts/"
done
touch "$TMP/respaldo.dump"

correr() { # $1 = carpeta; deja la salida en $TMP/out y el log de docker en $TMP/docker.log
  : > "$TMP/docker.log"
  (cd "$1" && env -u PG_CONTAINER -u COMPOSE_PROJECT_NAME PATH="$TMP/bin:$PATH" DOCKER_LOG="$TMP/docker.log" \
    bash scripts/restaurar-respaldo.sh "$TMP/respaldo.dump") > "$TMP/out" 2>&1 || true
}

# 1. Worktree con infraestructura propia: le habla a SU contenedor, nunca al de la carpeta principal.
printf 'COMPOSE_PROJECT_NAME=egb99\n' > "$TMP/wt/.env.worktree.local"
correr "$TMP/wt"
grep -q "egb99-postgres-1" "$TMP/docker.log" || falla "el worktree egb99 no le habló a egb99-postgres-1: $(cat "$TMP/docker.log")"
grep -q "deploy-postgres-1" "$TMP/docker.log" && falla "el worktree egb99 le habló a deploy-postgres-1, la base de la carpeta principal"

# 2. Worktree sin infraestructura propia: se niega antes de tocar docker.
rm "$TMP/wt/.env.worktree.local"
correr "$TMP/wt"
[ -s "$TMP/docker.log" ] && falla "un worktree sin .env.worktree.local llamó a docker: $(cat "$TMP/docker.log")"
grep -q "worktree-env" "$TMP/out" || falla "el rechazo no dice cómo arreglarlo (make worktree-env): $(cat "$TMP/out")"

# 3. Desde un worktree, ni pidiéndolo explícitamente se restaura sobre la carpeta principal.
printf 'COMPOSE_PROJECT_NAME=egb99\n' > "$TMP/wt/.env.worktree.local"
: > "$TMP/docker.log"
(cd "$TMP/wt" && PATH="$TMP/bin:$PATH" DOCKER_LOG="$TMP/docker.log" PG_CONTAINER=deploy-postgres-1 \
  bash scripts/restaurar-respaldo.sh "$TMP/respaldo.dump") > "$TMP/out" 2>&1 || true
grep -q "deploy-postgres-1" "$TMP/docker.log" && falla "PG_CONTAINER=deploy-postgres-1 desde un worktree tocó la base de la carpeta principal"

# 4. La carpeta principal sigue usando su contenedor de siempre.
correr "$TMP/principal"
grep -q "deploy-postgres-1" "$TMP/docker.log" || falla "la carpeta principal ya no le habla a deploy-postgres-1: $(cat "$TMP/docker.log")"

if [ "$fallas" -gt 0 ]; then
  echo "$fallas falla(s)" >&2
  exit 1
fi
echo "ok: restaurar-respaldo.sh respeta el worktree"
