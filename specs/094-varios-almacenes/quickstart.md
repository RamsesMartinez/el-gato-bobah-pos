# Quickstart: validar varios almacenes

Contra el `make start` de este worktree (API :8094, web :3094) con base restaurada.

## Preparación

```bash
make stop && make db-restaurar dump=backups/prod/pre-094-20261123-0241.dump && make start
```

## Migración (subida 1)

1. Antes de migrar, guardar `select branch_id, coalesce(ingredient_id, -product_id), sum(on_hand) from stock_levels group by 1,2`.
2. Migrar (`make db-migrate`). Repetir la suma: idéntica. `select count(*) from <cada tabla> where warehouse_id is null` = 0 en las nueve tablas (en `expense_items`, solo renglones de almacén).
3. Lo automatiza `TestWarehouseMigrationKeepsBranchTotals` sobre la base restaurada con dos empresas.

## Escenarios por entrega

| Entrega | Escenario | Resultado esperado |
| --- | --- | --- |
| 1 | Almacén › Almacenes: dar de alta «Bodega» | Aparece vacía; «Local» marcado como de venta |
| 1 | Archivar «Local» | Se niega |
| 1 | Vender un producto con receta | Baja «Local»; Movimientos del de venta lo muestra |
| 1 | Sucursal con un solo almacén → Existencias | Sin selector, igual que hoy |
| 2 | Compra de 8 renglones: 2 a «Bodega», 1 excluido | «Local» +5 renglones, «Bodega» +2 |
| 2 | Cambiar destino de un renglón recibido | Sale de uno y entra al otro |
| 3 | Traspasar 6 L de «Bodega» a «Local» (10 y 20 antes) | 16 y 14, dos movimientos ligados |
| 3 | Reenviar el mismo traspaso | No se duplica |
| 3 | Traspasar más de lo que hay | Pide confirmar |
| 4 | Contar «Bodega» con diferencias y cerrar | «Local» no cambia |
| 5 | Reporte de valor | Suma de almacenes = total |

## Pruebas

```bash
cd server && go build ./... && go test ./...
make test-integration         # RLS en los tres casos, migración sobre respaldo, idempotencia
cd ../web && bun run vitest run && bun run lint
```

E2E a 1280×854 y 1024×600: `E2E_BASE_URL=http://localhost:3094 E2E_API_URL=http://localhost:8094/api/v1 … bun run e2e`.
