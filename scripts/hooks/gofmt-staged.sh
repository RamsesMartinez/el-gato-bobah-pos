#!/usr/bin/env bash
# Falla si alguno de los archivos Go staged no está formateado.
#
# Vive en un script y no inline en lefthook.yml porque lefthook v2 pasa el `run:` a
# `sh -c "…"` SIN escapar las comillas dobles del script: cualquier `"` dentro del comando rompe
# el quoting y el hook muere con «unexpected EOF» en vez de revisar el formato. Un gate que falla
# por su propia sintaxis es peor que no tener gate: pasa desapercibido hasta que alguien commitea
# Go y se topa con un error que no habla de su código.
set -euo pipefail

# Sin archivos Go en el commit no hay nada que revisar.
[ "$#" -gt 0 ] || exit 0

sin_formato="$(gofmt -l "$@")"
if [ -n "$sin_formato" ]; then
  echo "$sin_formato"
  echo "corre gofmt -w"
  exit 1
fi
