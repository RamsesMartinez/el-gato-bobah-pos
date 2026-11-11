#!/usr/bin/env bash
# Prueba de scripts/hooks/sin-tokens-mcp.sh: un token del servidor MCP pegado en un archivo no se
# commitea (este repositorio es público). Corre en un repositorio desechable.
set -euo pipefail
HOOK="$(cd "$(dirname "$0")/.." && pwd)/hooks/sin-tokens-mcp.sh"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# Dentro de un hook de lefthook, git exporta GIT_DIR y GIT_INDEX_FILE del repositorio de verdad, y
# todo `git` de abajo (el init, el config, el reset) caería en él: así esta prueba dejó una vez el
# índice vacío —commits sin archivos— y el autor del repositorio compartido en «t <t@t>». Se apagan
# antes de tocar nada, y se comprueba al final que el repositorio de afuera no cambió.
centinela="$tmp/centinela.git"
git init -q --bare "$centinela"
export GIT_DIR="$centinela"
for v in $(git rev-parse --local-env-vars); do unset "$v"; done

mkdir "$tmp/repo"
cd "$tmp/repo" && git init -q && git config user.email t@t && git config user.name t
fallas=0
esperar() { # $1 = código esperado, $2 = descripción
  set +e; bash "$HOOK" >/dev/null 2>&1; got=$?; set -e
  if [ "$got" != "$1" ]; then echo "FALLA: $2 (esperaba $1, salió $got)"; fallas=$((fallas+1)); fi
  git reset -q; rm -f ./*.txt ./*.json
}
secreto=$(printf 'a%.0s' $(seq 1 64))
echo "export POS_MCP_TOKEN='egbpat_2_15_${secreto}'" > a.txt; git add a.txt
esperar 1 "un token completo en un archivo nuevo"
printf '{"headers":{"Authorization":"Bearer egbpat_1_3_%s"}}' "$secreto" > b.json; git add b.json
esperar 1 "un token dentro de un .mcp.json"
echo 'el formato es egbpat_<empresa>_<id>_<secreto>' > c.txt; git add c.txt
esperar 0 "la descripción del formato no es un token"
echo "egbpat_2_15_abcd" > d.txt; git add d.txt
esperar 0 "un secreto incompleto (la pista de 4 caracteres) no es un token"
if [ -n "$(git config --file "$centinela/config" --get user.email || true)" ]; then
  echo "FALLA: la prueba escribió en el repositorio de afuera"; fallas=$((fallas+1))
fi
[ "$fallas" = 0 ] && echo "sin-tokens-mcp: OK" || exit 1
