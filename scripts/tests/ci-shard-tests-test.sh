#!/usr/bin/env bash
# Prueba de scripts/ci-shard-tests.sh (spec 044, bordes E12 y E13).
set -euo pipefail
S="$(cd "$(dirname "$0")/.." && pwd)/ci-shard-tests.sh"
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
fail() { echo "FALLA: $*" >&2; exit 1; }

printf 'TestA\nTestB\nTestC\nTestD\nTestE\nTestF\nTestG\n' >"$T/list"
for s in 1 2 3; do bash "$S" shard $s 3 <"$T/list" >"$T/s$s"; done

# Los 3 cubren la lista completa sin repetir.
bash "$S" verify "$T/list" "$T/s1" "$T/s2" "$T/s3" >/dev/null || fail "reparto correcto rechazado"
[[ $(cat "$T"/s? | sort | uniq -d) == "" ]] || fail "una prueba en dos shards"
[[ $(wc -l <"$T/s1") -eq 3 && $(wc -l <"$T/s3") -eq 2 ]] || fail "el reparto no es por turno"

# E12: una prueba que ningún shard toma.
grep -v TestD "$T/s1" >"$T/s1x" || true; grep -v TestD "$T/s2" >"$T/s2x" || true; grep -v TestD "$T/s3" >"$T/s3x" || true
! bash "$S" verify "$T/list" "$T/s1x" "$T/s2x" "$T/s3x" 2>/dev/null || fail "prueba sin asignar no detectada"
# E12: una prueba que dos toman.
cat "$T/s1" <(echo TestB) >"$T/s1d"
! bash "$S" verify "$T/list" "$T/s1d" "$T/s2" "$T/s3" 2>/dev/null || fail "prueba repetida no detectada"

# E13: con menos pruebas que shards, el shard vacío no imprime regex (CI se salta go test).
printf 'TestA\n' >"$T/short"
[[ -z $(bash "$S" run-regex 3 3 <"$T/short") ]] || fail "shard vacío imprimió regex"
[[ $(bash "$S" run-regex 1 3 <"$T/short") == '^(TestA)$' ]] || fail "regex del shard 1"
[[ $(bash "$S" run-regex 1 3 <"$T/list") == '^(TestA|TestD|TestG)$' ]] || fail "regex con varias"

# Un shard fuera de rango es error, no un shard vacío.
! bash "$S" shard 4 3 <"$T/list" 2>/dev/null || fail "shard 4 de 3 aceptado"
echo "ci-shard-tests: ok"
