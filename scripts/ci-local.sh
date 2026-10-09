#!/usr/bin/env bash
# Corre en esta máquina lo mismo que los jobs de CI que gatean el despliegue (backend, integración y
# frontend), en paralelo, ANTES de empujar. CI tarda ~10 minutos en decir que algo está mal; aquí
# sale en lo que tarda el más lento de los tres, y con las cachés locales calientes.
#
# Lo que NO cubre, a propósito: el entorno de GitHub (versiones de las actions, el Go que instala
# setup-go) y el despliegue. CI sigue siendo el que despliega y el que manda; esto es para enterarse
# antes. La integración corre contra un Postgres desechable propio (puerto CI_PG_PORT, 5510), nunca
# contra el de dev.
set -uo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
logs="$(mktemp -d)"
port="${CI_PG_PORT:-5510}"
pg="egb-ci-local-pg-$$"
url="postgres://test:test@localhost:${port}/gatobobah_test?sslmode=disable"

cleanup() { docker rm -f "$pg" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --rm --name "$pg" -p "${port}:5432" -e POSTGRES_USER=test -e POSTGRES_PASSWORD=test \
  -e POSTGRES_DB=gatobobah_test postgres:16-alpine \
  -c fsync=off -c full_page_writes=off -c synchronous_commit=off >/dev/null || exit 1

backend() {
  cd "$root/server" &&
    go build ./... &&
    go test ./... &&
    bash ../scripts/hooks/golangci-lint.sh &&
    go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
}

integration() {
  cd "$root/server" || return 1
  until docker exec "$pg" pg_isready -U test >/dev/null 2>&1; do sleep 0.5; done
  sqlc diff &&
    DATABASE_URL="$url" go run ./cmd/migrate &&
    SQLC_DB_URI="$url" sqlc vet &&
    TEST_DATABASE_URL="$url" go test -count=1 -timeout 10m -tags=integration ./internal/integration/... ./internal/httpapi/...
}

frontend() {
  cd "$root/web" && bun run lint && bun run vitest run && bun run build
}

backend >"$logs/backend.log" 2>&1 & b=$!
integration >"$logs/integration.log" 2>&1 & i=$!
frontend >"$logs/frontend.log" 2>&1 & f=$!

fallo=0
for job in "backend:$b" "integration:$i" "frontend:$f"; do
  name="${job%%:*}"
  if wait "${job##*:}"; then
    echo "OK    $name"
  else
    echo "FALLA $name — últimas líneas de $logs/$name.log:"
    tail -25 "$logs/$name.log"
    fallo=1
  fi
done
exit "$fallo"
