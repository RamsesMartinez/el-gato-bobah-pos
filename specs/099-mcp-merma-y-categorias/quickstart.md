# Quickstart — 099

Validación de punta a punta en local (worktree 099: API en `:8099`, `make start` arriba, base
restaurada con `make db-restaurar`).

## 1. Pruebas

```bash
cd server && go build ./... && go test ./internal/domain/... ./internal/httpapi/...
make test-integration   # incluye categories_099, expected_waste_099, mcp_099 (inTheThreeCases)
```

## 2. MCP contra la API local

Con un token MCP de la empresa de pruebas (alcances `read` y `write_catalog`, usuario con
`menu.configure`):

1. `ingredient_expected_waste_set` `{ingredient_id, pct:"8", reason:"…"}` sin `dry_run` → vista previa:
   `current.pct`, `next.pct: 8`, costos antes/después, `uses` no vacío para un insumo con receta. La
   ficha del insumo no cambió.
2. Repetir con `dry_run:false`, `idempotency_key` nueva → aplicado; `audit_events` tiene
   `ingredient.expected_waste` con `reason`. Repetir la misma llamada → misma respuesta, un solo
   renglón de bitácora.
3. `clear:true` → `next.pct: null`, `fallback.productOrGeneral: true`. `pct:"0"` → 0, no nulo.
4. `category_update` `{category_id: <una sin productos activos>, active:false}` → vista previa sin
   avisos; aplicar → desaparece de `categories_list` y del menú del POS (recargar la pantalla de
   venta en `:3099`). `categories_list {include_inactive:true}` la muestra inactiva.
5. `category_update` sobre una con productos activos, `active:false` → rechazo con la cifra.
6. `category_create` `{name:"Prueba 099", parent_id:<raíz>}` → aparece como subcategoría; con
   `parent_id` de una subcategoría → rechazo de padre inválido.

Lo que se crea en la prueba (la categoría «Prueba 099») se desactiva al terminar.
