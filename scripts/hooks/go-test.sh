#!/usr/bin/env bash
# Corre los tests del backend desde el pre-push (lefthook).
#
# Smart App Control (Windows 11, encendido de fábrica) bloquea BINARIOS recién compilados por
# reputación, y el ejecutable que `go test` deja en %TEMP% es uno nuevo en cada corrida. El síntoma
# es "Una directiva de Control de aplicaciones bloqueó este archivo" en un paquete al azar — hoy
# `internal/cache`, mañana otro — con el resto de la suite en verde. No es un test que falle: es un
# proceso que no arranca.
#
# Cuando pasa, la suite corre en contenedor: el mismo golang:1.27 de CI, donde SAC no alcanza. Lo
# que NO se hace es aflojar el gate — la constitución prohíbe `--no-verify`; el gate se corre en
# otro lado. Mismo patrón que govulncheck.sh y golangci-lint.sh.
set -euo pipefail

salida="$(go test ./... 2>&1)" && { echo "$salida"; exit 0; }

# Solo el bloqueo de SAC justifica el contenedor. Un test que de verdad falla tiene que fallar aquí
# y ahora, con su mensaje, no esconderse detrás de una segunda corrida.
if ! echo "$salida" | grep -qi "Control de aplicaciones\|Application Control"; then
  echo "$salida"
  exit 1
fi

echo "$salida"
echo
echo "Smart App Control bloqueó un binario de test; corriendo la misma suite en contenedor…"

# Se monta el REPO, no solo server/: folio_espejo_test.go compara la lista de animales del
# servidor contra su copia en web/, y con el módulo suelto no encontraría el archivo — se
# saltaría en silencio, que es justo el modo de fallar que ese test existe para evitar.
raiz="$(git rev-parse --show-toplevel)"
if command -v cygpath >/dev/null 2>&1; then
  # Docker no entiende la ruta POSIX de Git Bash (/d/git/…); cygpath la traduce a D:/git/…
  raiz="$(cygpath -m "$raiz")"
fi

# Los volúmenes de caché no son opcionales: sin ellos cada push vuelve a bajar el módulo entero.
MSYS_NO_PATHCONV=1 exec docker run --rm \
  -v "$raiz:/repo" -w /repo/server \
  -v gatobobah_gocache:/root/.cache/go-build \
  -v gatobobah_gomod:/go/pkg/mod \
  golang:1.27 go test ./...
