#!/usr/bin/env bash
# Corre el lint y el typecheck del frontend desde el pre-commit (lefthook).
#
# Mismo problema y misma salida que go-test.sh, govulncheck.sh y golangci-lint.sh: Smart App Control
# (Windows 11, encendido de fábrica) bloquea BINARIOS por reputación, y el veredicto se mueve solo.
# Con bun el síntoma no es el mensaje de SAC sino un lacónico `bun: unknown error:` seguido de un
# exit 1, sin decir qué script falló — `tsc` y `eslint` corridos a mano pasan limpios.
#
# Cuando pasa, los dos gates corren en contenedor. Lo que NO se hace es `--no-verify`: la
# constitución lo prohíbe, así que el gate no se afloja, se corre en otro lado.
set -euo pipefail

salida="$(bun run lint && bun run typecheck 2>&1)" && { echo "$salida"; exit 0; }

# Solo el bloqueo justifica el contenedor. Un error de tipos de verdad tiene que fallar aquí y
# ahora, con su mensaje, no esconderse detrás de una segunda corrida de dos minutos.
if ! echo "$salida" | grep -qi "unknown error\|Control de aplicaciones\|Application Control\|Permission denied"; then
  echo "$salida"
  exit 1
fi

echo "$salida"
echo
echo "Windows bloqueó el binario de bun; corriendo lint y typecheck en contenedor…"

dir="$(pwd)"
# Docker no entiende la ruta POSIX de Git Bash (/d/git/…); cygpath la traduce a D:/git/…
if command -v cygpath >/dev/null 2>&1; then
  dir="$(cygpath -m "$(pwd)")"
fi

# node_modules va en un VOLUMEN y no en el bind mount: las dependencias con binarios nativos
# (esbuild, rolldown) se instalan para Linux dentro del contenedor, y montarlas encima de las del
# host dejaría al host sin poder correr nada.
MSYS_NO_PATHCONV=1 exec docker run --rm \
  -v "$dir:/app" -w /app \
  -v gatobobah_web_modules:/app/node_modules \
  oven/bun:1 sh -c "bun install --frozen-lockfile --silent && bun run lint && bun run typecheck"
