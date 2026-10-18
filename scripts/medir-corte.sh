#!/usr/bin/env bash
# Mide el corte de una subida (spec 041): pide URL cada 200 ms durante SEGUNDOS y cuenta cuántas
# peticiones fallaron (lo que no sea 200, o más de 2 s sin respuesta) y la mayor latencia.
#
#   scripts/medir-corte.sh https://api-dev.elgatobobah.com/healthz 180
#
# Se corre desde otra terminal MIENTRAS sube la versión. Sale ≠0 si hubo una sola fallida: la meta
# es cero, y un número que hay que leer para saber si pasó es uno que nadie lee.
set -uo pipefail

URL=${1:?uso: medir-corte.sh URL SEGUNDOS}
DURACION=${2:?uso: medir-corte.sh URL SEGUNDOS}
# -k solo para la vuelta local, donde Caddy firma con su propia CA.
CURL_EXTRA=${CURL_EXTRA:-}

total=0 fallidas=0 max_ms=0
fin=$(( $(date +%s) + DURACION ))
while [ "$(date +%s)" -lt "$fin" ]; do
  # shellcheck disable=SC2086
  salida=$(curl -s -o /dev/null --max-time 2 $CURL_EXTRA -w '%{http_code} %{time_total}' "$URL")
  codigo=${salida%% *}
  ms=$(awk -v t="${salida##* }" 'BEGIN { printf "%d", t * 1000 }')
  total=$((total + 1))
  [ "$ms" -gt "$max_ms" ] && max_ms=$ms
  if [ "$codigo" != "200" ]; then
    fallidas=$((fallidas + 1))
    echo "$(date +%T.%3N) falló: código=$codigo ${ms} ms"
  fi
  sleep 0.2
done

echo "peticiones=$total fallidas=$fallidas latencia_max_ms=$max_ms"
[ "$fallidas" -eq 0 ]
