#!/usr/bin/env bash
# Dice si un árbol de archivos ya pasó las pruebas en un PR.
#
# Uso: ci-tree-tested.sh OWNER/REPO TREE_SHA
#   Sale 0 e imprime la corrida si encuentra una corrida de CI de un pull_request en verde cuyo commit
#   de cabeza tiene ese mismo árbol; sale 1 si no la encuentra.
#
# POR QUÉ EL ÁRBOL Y NO EL SHA. El merge de un PR a main crea un commit nuevo, así que su SHA nunca
# es el que se probó. Pero cuando main no trae nada que la rama del PR no tenga (develop se sincroniza
# con main tras cada hotfix), el árbol del merge es IDÉNTICO al de la cabeza del PR: verificado en los
# PR #202, #209 y #213. Igual árbol = exactamente los mismos archivos que pasaron las pruebas. Si el
# árbol no coincide (un push directo, o un PR que no estaba al día), no hay corrida que lo respalde y
# el push corre la suite completa.
set -euo pipefail

repo=${1:?repo}
tree=${2:?tree}
# 50 corridas en verde cubren de sobra el tiempo entre que el PR pasa y se mergea.
heads=$(gh api "repos/$repo/actions/workflows/ci.yml/runs?event=pull_request&status=success&per_page=50" \
  --jq '.workflow_runs[] | "\(.head_sha) \(.html_url)"' | awk '!seen[$1]++')

while read -r sha url; do
  [ -z "$sha" ] && continue
  if [ "$(gh api "repos/$repo/git/commits/$sha" --jq .tree.sha)" = "$tree" ]; then
    echo "$url"
    exit 0
  fi
done <<<"$heads"
exit 1
