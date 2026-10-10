# Quickstart: validar la 029

1. Postgres de pruebas propio:
   `docker run -d --rm --name egb029-pg -p 5503:5432 -e POSTGRES_USER=gatobobah -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=gatobobah_test postgres:16-alpine`
2. Backend: `cd server && go build ./... && go test ./...` y
   `TEST_DATABASE_URL=postgres://gatobobah:pw@localhost:5503/gatobobah_test?sslmode=disable go test -tags=integration ./internal/integration/ -run 'VentasNetas|Corte|Detalle'`.
3. Web: `cd web && bun run lint && bun run vitest run && bun run build`.
4. Visual: API local contra ese Postgres + `bun run dev`; capturas a 1024×600 de Ventas (hoy y mes),
   Caja (turno abierto con devoluciones en efectivo y tarjeta), Histórico → Ver del turno abierto y
   Reportes. Esperado: SC-001 (Total = Σ medios), SC-004 (≥4 renglones), las devoluciones del corte
   en «Devoluciones» y no en «Salidas».
