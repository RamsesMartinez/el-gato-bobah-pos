#!/usr/bin/env bash
# Prueba de scripts/entrantes-de-prueba.sh: a qué contenedor le habla y qué rechaza.
#
# POR QUÉ. El script escribe pedidos de prueba. En la base de la carpeta principal aparecerían en la
# tableta de quien la use, sonando, y bloquearían el cierre de caja. Esta prueba usa un `docker`
# falso que solo anota a quién le hablan; nunca toca un contenedor de verdad.
#
#   bash scripts/tests/entrantes-de-prueba-test.sh
set -euo pipefail
# shellcheck disable=SC2046
unset $(git rev-parse --local-env-vars)

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
fallas=0
falla() { echo "FALLA: $*" >&2; fallas=$((fallas + 1)); }

mkdir -p "$TMP/bin" "$TMP/wt/scripts"
cat > "$TMP/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
echo "$*" >> "$DOCKER_LOG"
cat > /dev/null
DOCKER
chmod +x "$TMP/bin/docker"
cp "$SRC/scripts/entrantes-de-prueba.sh" "$SRC/scripts/worktree-env-load.sh" "$TMP/wt/scripts/"

correr() { # $@ = argumentos; deja la salida en $TMP/out y el log de docker en $TMP/docker.log
  : > "$TMP/docker.log"
  (cd "$TMP/wt" && env -u COMPOSE_PROJECT_NAME PATH="$TMP/bin:$PATH" DOCKER_LOG="$TMP/docker.log" \
    bash scripts/entrantes-de-prueba.sh "$@") > "$TMP/out" 2>&1 || true
}

# 1. Sin infraestructura propia: se niega antes de tocar docker y dice cómo arreglarlo.
correr 1
[ -s "$TMP/docker.log" ] && falla "sin .env.worktree.local llamó a docker: $(cat "$TMP/docker.log")"
grep -q "worktree-env" "$TMP/out" || falla "el rechazo no dice cómo arreglarlo: $(cat "$TMP/out")"

# 2. Con infraestructura propia: le habla a SU contenedor y nunca al de la carpeta principal.
printf 'COMPOSE_PROJECT_NAME=egb99\n' > "$TMP/wt/.env.worktree.local"
correr 2
grep -q "egb99-postgres-1" "$TMP/docker.log" || falla "no le habló a egb99-postgres-1: $(cat "$TMP/docker.log")"
grep -q "deploy-postgres-1" "$TMP/docker.log" && falla "le habló a deploy-postgres-1"

# 3. Argumentos absurdos se rechazan sin tocar docker.
for malo in 0 21 abc "1;drop"; do
  correr "$malo"
  [ -s "$TMP/docker.log" ] && falla "n=$malo llegó a docker"
done
correr 1 "Gato;Bobah"
[ -s "$TMP/docker.log" ] && falla "un slug con caracteres raros llegó a docker"

[ "$fallas" -eq 0 ] && echo "entrantes-de-prueba: ok" || exit 1
