# Quickstart: validar

1. `cd server && go test ./internal/cache/ ./internal/httpapi/ ./internal/integration/ -run 'Menu|Catalog'`.
2. `cd web && bun run vitest run src/api src/hooks src/features/pos`.
3. Manual en dev: abrir la venta, en `psql` como owner `update modifier_groups set is_active=false where id=<g>;`
   → en ≤60 s el grupo desaparece de la hoja sin recargar.
4. `curl -H "If-None-Match: <etag>"` dos veces → la segunda 304.
