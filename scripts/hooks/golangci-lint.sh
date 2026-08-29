#!/usr/bin/env bash
# Corre golangci-lint (con gosec) sobre el módulo. Lo usa el pre-commit de lefthook y `make lint`.
#
# Mismo problema y misma salida que scripts/hooks/govulncheck.sh: Smart App Control (Windows 11,
# encendido de fábrica) bloquea binarios por REPUTACIÓN, y el veredicto es por binario y se mueve
# solo — golangci-lint corrió meses y un día empezó a dar "Permission denied" sin que nada cambiara.
# Cuando pasa, el linter corre en contenedor. Lo que NO se hace es saltarse el gate: los hooks
# quedan verdes antes de commitear y `--no-verify` no se usa (constitución, Quality gates).
#
# La versión del contenedor se fija a la MISMA que usa CI (.github/workflows/ci.yml): golangci-lint
# rechaza analizar un módulo cuyo Go sea de un minor mayor al que lo compiló, así que las dos tienen
# que moverse juntas al subir el toolchain (AGENTS.md §3).
set -euo pipefail

VERSION="v2.13.1"

if golangci-lint --version >/dev/null 2>&1; then
  exec golangci-lint run ./...
fi

echo "golangci-lint no se puede ejecutar en este equipo; corriendo el mismo linter en contenedor…"

dir="$(pwd)"
# Docker no entiende la ruta POSIX de Git Bash (/d/git/…); cygpath la traduce a D:/git/…
if command -v cygpath >/dev/null 2>&1; then
  dir="$(cygpath -m "$(pwd)")"
fi

# Los volúmenes de caché no son opcionales: sin ellos cada commit vuelve a bajar el módulo entero y
# a reanalizarlo desde cero.
MSYS_NO_PATHCONV=1 exec docker run --rm \
  -v "$dir:/src" -w /src \
  -v gatobobah_gocache:/root/.cache/go-build \
  -v gatobobah_gomod:/go/pkg/mod \
  -v gatobobah_lintcache:/root/.cache/golangci-lint \
  "golangci/golangci-lint:${VERSION}" golangci-lint run ./...
