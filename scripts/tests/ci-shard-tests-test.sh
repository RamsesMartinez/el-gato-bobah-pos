#!/usr/bin/env bash
# Prueba de scripts/ci-shard-tests.sh (spec 044, bordes E12 y E13; carril de las pesadas).
set -euo pipefail
S="$(cd "$(dirname "$0")/.." && pwd)/ci-shard-tests.sh"
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
fail() { echo "FALLA: $*" >&2; exit 1; }
export HEAVY_TESTS=TestHeavy

printf 'TestA\nTestB\nTestC\nTestD\nTestE\nTestHeavy\nTestF\nTestG\n' >"$T/list"
for s in 1 2 3 4; do bash "$S" shard $s 4 <"$T/list" >"$T/s$s"; done

# Los 4 cubren la lista completa sin repetir.
bash "$S" verify "$T/list" "$T/s1" "$T/s2" "$T/s3" "$T/s4" >/dev/null || fail "reparto correcto rechazado"
[[ $(cat "$T"/s? | sort | uniq -d) == "" ]] || fail "una prueba en dos shards"

# La pesada va sola en el último; las demás, por turno entre los otros tres.
[[ $(cat "$T/s4") == TestHeavy ]] || fail "la pesada no quedó sola en el último shard: $(tr '\n' ' ' <"$T/s4")"
! grep -qx TestHeavy "$T/s1" "$T/s2" "$T/s3" || fail "la pesada cayó en un shard por turno"
[[ $(tr '\n' ' ' <"$T/s1") == "TestA TestD TestG " ]] || fail "el reparto no es por turno: s1 = $(tr '\n' ' ' <"$T/s1")"
[[ $(tr '\n' ' ' <"$T/s3") == "TestC TestF " ]] || fail "el turno se corrió al saltar la pesada: s3 = $(tr '\n' ' ' <"$T/s3")"

# E12: una prueba que ningún shard toma.
for s in 1 2 3 4; do grep -v TestD "$T/s$s" >"$T/x$s" || true; done
! bash "$S" verify "$T/list" "$T/x1" "$T/x2" "$T/x3" "$T/x4" 2>/dev/null || fail "prueba sin asignar no detectada"
# E12: una prueba que dos toman.
cat "$T/s1" <(echo TestB) >"$T/s1d"
! bash "$S" verify "$T/list" "$T/s1d" "$T/s2" "$T/s3" "$T/s4" 2>/dev/null || fail "prueba repetida no detectada"
# Una pesada que se renombró regresaría al turno en silencio: verify lo dice.
! HEAVY_TESTS=TestRenombrada bash "$S" verify "$T/list" "$T/s1" "$T/s2" "$T/s3" "$T/s4" 2>/dev/null || fail "pesada inexistente no detectada"

# E13: con menos pruebas que shards, el shard vacío no imprime regex (CI se salta go test).
printf 'TestA\n' >"$T/short"
[[ -z $(bash "$S" run-regex 4 4 <"$T/short") ]] || fail "shard de pesadas vacío imprimió regex"
[[ -z $(bash "$S" run-regex 3 4 <"$T/short") ]] || fail "shard vacío imprimió regex"
[[ $(bash "$S" run-regex 1 4 <"$T/short") == '^(TestA)$' ]] || fail "regex del shard 1"
[[ $(bash "$S" run-regex 1 4 <"$T/list") == '^(TestA|TestD|TestG)$' ]] || fail "regex con varias"
[[ $(bash "$S" run-regex 4 4 <"$T/list") == '^(TestHeavy)$' ]] || fail "regex del carril de pesadas"

# Con un solo shard no hay carril aparte: todo va en el 1.
[[ $(bash "$S" shard 1 1 <"$T/list" | wc -l) -eq 8 ]] || fail "con un shard se perdieron pruebas"

# Un shard fuera de rango es error, no un shard vacío.
! bash "$S" shard 5 4 <"$T/list" 2>/dev/null || fail "shard 5 de 4 aceptado"
echo "ci-shard-tests: ok"
