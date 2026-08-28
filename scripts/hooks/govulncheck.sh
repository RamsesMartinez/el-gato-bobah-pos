#!/usr/bin/env bash
# Escanea el módulo en busca de CVEs. Se corre desde el pre-push (lefthook) y desde `make vuln`.
#
# Smart App Control (Windows 11, encendido de fábrica) bloquea el BINARIO de govulncheck: no lo deja
# ejecutarse por reputación, se recompile como se recompile. Cuando eso pasa, el escáner corre en
# contenedor — el mismo golang:1.27 que usa CI. Lo que NO se hace es saltarse el gate: un CVE
# bloquea el merge (constitución, Restricciones del producto) y `--no-verify` no se usa.
#
# Igual que gofmt-staged.sh, esto vive en un script porque lefthook v2 rompe cualquier `run:` con
# comillas dobles al pasarlo a `sh -c "…"`.
set -euo pipefail

if govulncheck -version >/dev/null 2>&1; then
  exec govulncheck ./...
fi

echo "govulncheck no se puede ejecutar en este equipo; corriendo el mismo escáner en contenedor…"

dir="$(pwd)"
# Docker no entiende la ruta POSIX de Git Bash (/d/git/…); cygpath la traduce a D:/git/…
if command -v cygpath >/dev/null 2>&1; then
  dir="$(cygpath -m "$(pwd)")"
fi

# Los volúmenes de caché no son opcionales: sin ellos cada push vuelve a bajar el módulo entero.
MSYS_NO_PATHCONV=1 exec docker run --rm \
  -v "$dir:/src" -w /src \
  -v gatobobah_gocache:/root/.cache/go-build \
  -v gatobobah_gomod:/go/pkg/mod \
  golang:1.27 sh -c 'go install golang.org/x/vuln/cmd/govulncheck@latest >/dev/null 2>&1 && govulncheck ./...'
