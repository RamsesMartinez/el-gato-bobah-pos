#!/usr/bin/env bash
# Prueba de scripts/cerrar-worktree.sh y de lo que baja: un repositorio con la carpeta principal
# BARE (como la de verdad), un worktree y un `docker` falso que anota cada llamada.
#
# POR QUÉ. Al cerrar la 041 (2026-10-18) salieron dos defectos: `make cerrar-worktree` no corría
# desde la carpeta principal porque es bare y no tiene Makefile, y el Postgres de pruebas (perfil
# `test` del compose) seguía vivo después de cerrar, con la red del proyecto ocupada.
#
#   bash scripts/tests/cerrar-worktree-test.sh
set -euo pipefail
# shellcheck disable=SC2046
unset $(git rev-parse --local-env-vars)

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
fallas=0
falla() { echo "FALLA [$caso]: $*" >&2; fallas=$((fallas + 1)); }

mkdir -p "$TMP/bin"
cat > "$TMP/bin/docker" <<'EOF'
#!/usr/bin/env bash
echo "$*" >> "$DOCKER_LOG"
exit 0
EOF
printf '#!/usr/bin/env bash\nexit 0\n' > "$TMP/bin/lsof"
chmod +x "$TMP/bin/docker" "$TMP/bin/lsof"
export PATH="$TMP/bin:$PATH" DOCKER_LOG="$TMP/docker.log"
G=(git -c user.email=t@t -c user.name=t -c commit.gpgsign=false)

# origin con develop, una carpeta principal bare con su remoto, y lo que el script necesita.
git init -q --bare "$TMP/origin.git"
git init -q -b develop "$TMP/semilla"
mkdir -p "$TMP/semilla/scripts" "$TMP/semilla/deploy"
cp "$SRC/scripts/"{cerrar-worktree,stop,guard-shared-compose,worktree-env-load}.sh "$TMP/semilla/scripts/"
cp "$SRC/Makefile" "$TMP/semilla/"
touch "$TMP/semilla/deploy/docker-compose.dev.yml"
printf '.env.worktree.local\n' > "$TMP/semilla/.gitignore"
"${G[@]}" -C "$TMP/semilla" add -A && "${G[@]}" -C "$TMP/semilla" commit -qm semilla
git -C "$TMP/semilla" push -q "$TMP/origin.git" develop

nuevo_worktree() { # $1 = rama; deja el worktree en $TMP/wt-$1, empujado a origin
  git -C "$TMP/principal.git" worktree add -q -b "$1" "$TMP/wt-$1" origin/develop
  printf 'COMPOSE_PROJECT_NAME=egb99\n' > "$TMP/wt-$1/.env.worktree.local"
  git -C "$TMP/wt-$1" push -q origin "$1"
}
git clone -q --bare "$TMP/origin.git" "$TMP/principal.git"
git -C "$TMP/principal.git" config remote.origin.fetch '+refs/heads/*:refs/remotes/origin/*'
git -C "$TMP/principal.git" fetch -q origin

caso="desde dentro del propio worktree, sin argumentos"
nuevo_worktree uno; : > "$DOCKER_LOG"
if ! (cd "$TMP/wt-uno" && bash scripts/cerrar-worktree.sh) > "$TMP/out" 2>&1; then
  falla "salió con error: $(cat "$TMP/out")"
fi
[ -d "$TMP/wt-uno" ] && falla "la carpeta del worktree sigue ahí"
grep -q -- "--profile test" "$DOCKER_LOG" && grep -q " down" "$DOCKER_LOG" \
  || falla "no bajó el Postgres de pruebas (perfil test): $(cat "$DOCKER_LOG")"

caso="desde otra carpeta con --datos"
nuevo_worktree dos; : > "$DOCKER_LOG"
(cd "$TMP" && bash "$TMP/wt-dos/scripts/cerrar-worktree.sh" "$TMP/wt-dos" --datos) > "$TMP/out" 2>&1 || falla "salió con error: $(cat "$TMP/out")"
[ -d "$TMP/wt-dos" ] && falla "la carpeta del worktree sigue ahí"
[ "$(grep -c -- "--profile test.* down" "$DOCKER_LOG")" -ge 1 ] || falla "deps-down no bajó el perfil test: $(cat "$DOCKER_LOG")"
grep -q "label=com.docker.compose.project=egb99" "$DOCKER_LOG" || falla "no buscó los volúmenes de egb99"

caso="cambios sin commit"
nuevo_worktree tres; touch "$TMP/wt-tres/sin-commit"
(cd "$TMP/wt-tres" && bash scripts/cerrar-worktree.sh) > "$TMP/out" 2>&1 && falla "cerró un worktree con cambios sin commit"
[ -d "$TMP/wt-tres" ] || falla "borró un worktree con cambios sin commit"

if [ "$fallas" -gt 0 ]; then echo "cerrar-worktree: $fallas falla(s)" >&2; exit 1; fi
echo "cerrar-worktree: ok"
