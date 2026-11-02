#!/usr/bin/env bash
# Reparte las pruebas de integración entre N shards del job `integration` de CI (spec 044, US9).
#
#   ci-shard-tests.sh list                   lista las pruebas (go test -list), una por renglón
#   ci-shard-tests.sh shard N TOTAL < lista  imprime las del shard N (1..TOTAL)
#   ci-shard-tests.sh verify LISTA S1 S2 …   falla si la unión de los shards no es la lista, hay
#                                            repetidas o una prueba pesada ya no existe
#   ci-shard-tests.sh run-regex N TOTAL < lista  imprime '^(A|B)$' para -run; vacío si el shard no tiene
#
# Las pruebas PESADAS van solas en el último shard; las demás se reparten por turno entre los otros.
# Por turno y no por nombre: con el orden alfabético de `go test -list`, las pruebas de un mismo
# archivo (que suelen pesar parecido) quedan repartidas entre los shards. El paquete internal/httpapi
# no pasa por aquí: va entero en el shard 1 (ver ci.yml).
#
# Por qué un carril aparte: TestTheAuditLogLoadsAYearWithinTheCeiling siembra 1.46 millones de
# eventos y tarda sola 59-102 s en CI, más que todo lo demás de un shard junto. Por turno caía en
# cualquier shard según cuántas pruebas hubiera antes en la lista, y el que la recibía llegaba a
# 244-249 s mientras los otros terminaban en 140-175 s (medido en las corridas del 2026-11-01).
# Una prueba entra aquí solo con su medición: una lista que crece sin medir vuelve a desbalancear.
set -euo pipefail

HEAVY_TESTS=${HEAVY_TESTS:-TestTheAuditLogLoadsAYearWithinTheCeiling}

is_heavy() { [[ " $HEAVY_TESTS " == *" $1 "* ]]; }

list() {
  # Desde server/. Solo los nombres Test*: `go test -list` imprime también "ok  paquete".
  go test -list '.*' -tags=integration ./internal/integration/... | grep -E '^Test' | sort -u
}

shard() {
  local n=$1 total=$2
  [[ $n =~ ^[0-9]+$ && $total =~ ^[0-9]+$ && $n -ge 1 && $n -le $total ]] \
    || { echo "shard inválido: $n de $total" >&2; exit 2; }
  # Con un solo shard no hay carril aparte: todo va junto.
  if [[ $total -eq 1 ]]; then
    awk 'NF'
    return
  fi
  local name i=0
  while IFS= read -r name; do
    [[ -n $name ]] || continue
    if is_heavy "$name"; then
      [[ $n -eq $total ]] && echo "$name"
    else
      [[ $(( (i % (total - 1)) + 1 )) -eq $n ]] && echo "$name"
      i=$((i + 1))
    fi
  done
  return 0
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
  # Una pesada que se renombró o se borró regresaría al reparto por turno sin que nadie lo note.
  local h
  for h in $HEAVY_TESTS; do
    grep -qx "$h" <<<"$all" || { echo "la prueba pesada $h ya no está en la lista: actualiza HEAVY_TESTS" >&2; exit 1; }
  done
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
