#!/usr/bin/env bash
# Prueba de scripts/nuevo-worktree.sh: no hereda el entorno del worktree desde el que se corre.
#
# POR QUÉ. Al crear la 044 (2026-10-21) desde el worktree de la 042, `make nuevo-worktree` dejó
# en el entorno lo que el Makefile de la 042 exporta (DEV_DATABASE_URL con el puerto 5542, entre
# otras). El Makefile de la 044 las toma con `?=`, así que su API migró y le cambió la contraseña
# al rol de la app en la base de la 042, y después no pudo conectarse a la suya. Toda variable que
# el Makefile exporta tiene que limpiarse en el script antes de levantar el worktree nuevo; una
# que se agregue al Makefile y no al script hace fallar esta prueba.
#
#   bash scripts/tests/nuevo-worktree-test.sh
set -euo pipefail
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
exportadas=$(grep -E '^export ' "$SRC/Makefile" | sed 's/^export //' | tr ' ' '\n' | sed '/^$/d' | sort -u)
limpiadas=$( (grep -E '^unset ' "$SRC/scripts/nuevo-worktree.sh" || true) | sed 's/^unset //' | tr ' ' '\n' | sed '/^$/d' | sort -u)
faltan=$(comm -23 <(echo "$exportadas") <(echo "$limpiadas"))
if [[ -n "$faltan" ]]; then
  echo "FALLA: nuevo-worktree.sh no limpia lo que el Makefile exporta y heredaría del worktree de origen:" >&2
  echo "$faltan" | sed 's/^/  /' >&2
  exit 1
fi
echo "ok: nuevo-worktree.sh limpia las $(echo "$exportadas" | wc -l) variables que exporta el Makefile"
