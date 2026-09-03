#!/usr/bin/env bash
# Corre el build del frontend desde el pre-push (lefthook).
#
# Misma historia y misma salida que web-lint.sh: Smart App Control bloquea el binario de bun por
# reputación y el síntoma es un exit 1 SIN mensaje —ni siquiera el `bun: unknown error:` del lint—,
# así que el push falla sin decir por qué. Ya pasó con un build que en contenedor salía limpio.
#
# Lo que NO se hace es `--no-verify`: el gate no se afloja, se corre en otro lado.
set -euo pipefail

salida="$(bun run build 2>&1)" && { echo "$salida"; exit 0; }

# Solo el bloqueo justifica el contenedor. Un error de build de verdad tiene que fallar aquí, con su
# mensaje, y no esconderse detrás de una segunda corrida de dos minutos.
#
# El caso del exit sin salida entra: es la forma en que bun bloqueado se manifiesta aquí, y un error
# de build real siempre imprime algo.
if [ -n "${salida// }" ] && ! echo "$salida" | grep -qi "unknown error\|Control de aplicaciones\|Application Control\|Permission denied"; then
  echo "$salida"
  exit 1
fi

echo "$salida"
echo "Windows bloqueó el binario de bun; corriendo el build en contenedor…"

dir="$(pwd)"
# Docker no entiende la ruta POSIX de Git Bash (/d/git/…); cygpath la traduce a D:/git/…
if command -v cygpath >/dev/null 2>&1; then
  dir="$(cygpath -m "$(pwd)")"
fi

# node_modules en VOLUMEN y no en el bind mount: las dependencias con binarios nativos (esbuild,
# rolldown) se instalan para Linux dentro del contenedor, y montarlas encima dejaría al host sin
# poder correr nada.
MSYS_NO_PATHCONV=1 exec docker run --rm \
  -v "$dir:/app" -w /app \
  -v gatobobah_web_modules:/app/node_modules \
  oven/bun:1 sh -c "bun install --frozen-lockfile --silent && bun run build"
