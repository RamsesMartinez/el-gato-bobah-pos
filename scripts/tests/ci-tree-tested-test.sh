#!/usr/bin/env bash
# Vigila cuándo un push a main se ahorra las pruebas: solo si un PR en verde probó ese mismo árbol.
set -uo pipefail
script="$(cd "$(dirname "$0")/.." && pwd)/ci-tree-tested.sh"
fake=$(mktemp -d)
trap 'rm -rf "$fake"' EXIT

# `gh` falso: dos corridas en verde (la cabeza aaa repetida) y el árbol de cada cabeza.
cat >"$fake/gh" <<'EOF'
#!/usr/bin/env bash
case "$2" in
  *actions/workflows*) printf 'aaa https://run/1\naaa https://run/2\nbbb https://run/3\n' ;;
  */git/commits/aaa) echo tree-probado ;;
  */git/commits/bbb) echo otro-arbol ;;
esac
EOF
chmod +x "$fake/gh"

fallas=0
espera() {
  local quiere=$1 tree=$2 sale
  PATH="$fake:$PATH" bash "$script" o/r "$tree" >/dev/null; sale=$?
  if [ "$sale" != "$quiere" ]; then echo "✗ árbol $tree: esperaba salida $quiere y salió $sale"; fallas=$((fallas+1)); fi
}
espera 0 tree-probado   # el árbol del release ya pasó en su PR: no se repite
espera 1 nunca-probado  # un push directo trae un árbol que ningún PR probó: corre todo
[ $fallas -eq 0 ] && echo "ci-tree-tested: ok" || exit 1
