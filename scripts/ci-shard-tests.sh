#!/usr/bin/env bash
# Reparte las pruebas de integración entre N shards del job `integration` de CI (spec 044, US9).
#
#   ci-shard-tests.sh list                   lista las pruebas (go test -list), una por renglón
#   ci-shard-tests.sh shard N TOTAL < lista  imprime las del shard N (1..TOTAL), por turno
#   ci-shard-tests.sh verify LISTA S1 S2 …   falla si la unión de los shards no es la lista o hay repetidas
#   ci-shard-tests.sh run-regex N TOTAL < lista  imprime '^(A|B)$' para -run; vacío si el shard no tiene
#
# Por turno y no por nombre: con el orden alfabético de `go test -list`, las pruebas de un mismo
# archivo (que suelen pesar parecido) quedan repartidas entre los shards. El paquete internal/httpapi
# no pasa por aquí: va entero en el shard 1 (ver ci.yml).
set -euo pipefail

list() {
  # Desde server/. Solo los nombres Test*: `go test -list` imprime también "ok  paquete".
  go test -list '.*' -tags=integration ./internal/integration/... | grep -E '^Test' | sort -u
}

shard() {
  local n=$1 total=$2
  [[ $n =~ ^[0-9]+$ && $total =~ ^[0-9]+$ && $n -ge 1 && $n -le $total ]] \
    || { echo "shard inválido: $n de $total" >&2; exit 2; }
  awk -v n="$n" -v t="$total" 'NF { if ((i++ % t) + 1 == n) print }'
}

verify() {
  # verify LISTA SHARD1 SHARD2 ...: compara archivos ya repartidos contra la lista completa.
  local list=$1; shift
  local all union dups
  all=$(grep -v '^$' "$list" | sort || true)
  union=$(cat "$@" | grep -v '^$' | sort || true)
  dups=$( { uniq -d <<<"$all"; uniq -d <<<"$union"; } | grep -v '^$' | sort -u || true)
  if [[ -n $dups ]]; then
    echo "pruebas repetidas: $(tr '\n' ' ' <<<"$dups")" >&2; exit 1
  fi
  if [[ $union != "$all" ]]; then
    echo "la unión de los shards no es la lista completa (< falta en los shards, > sobra):" >&2
    diff <(echo "$all") <(echo "$union") >&2 || true
    exit 1
  fi
  echo "ok: $(grep -c . <<<"$all" || true) pruebas en $# shards"
}

run_regex() {
  local names
  names=$(shard "$1" "$2" | paste -sd'|' -)
  [[ -z $names ]] || echo "^(${names})\$"
}

case "${1:-}" in
  list) list ;;
  shard) shard "$2" "$3" ;;
  verify) shift; verify "$@" ;;
  run-regex) run_regex "$2" "$3" ;;
  *) sed -n '2,7p' "$0" >&2; exit 2 ;;
esac
