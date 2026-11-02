# Quickstart: validar la 056 en local

## Preparación

```bash
make start                       # API :8056, web :3056 (este worktree)
make db-restaurar                # respaldo real de producción (detén la API antes)
make test-integration            # Postgres de pruebas :5656, nunca el de la API
```

Usuario con `inventory.configure` (admin). Tableta simulada: Playwright 1024×600 con `hasTouch`.

## Escenarios

| # | Pasos | Esperado |
|---|---|---|
| Q1 | Almacén › Artículos › abrir un insumo de leche › «Unir otros aquí» › elegir la otra marca › «Ver qué cambia» | Vista previa: existencia por sucursal de los dos antes/después, recetas que cambian, presentaciones que llegan con marca, costo antes/después |
| Q2 | Confirmar Q1 | El origen sale de la lista de activos; en Archivados dice «Unido a <destino>»; ninguna receta lo lleva (`select count(*) from recipe_items where ingredient_id = <origen>` = 0) |
| Q3 | Vender en el POS un producto cuya receta llevaba el origen | El movimiento de venta es del destino; el origen no se mueve |
| Q4 | Cancelar un pedido vendido ANTES de la unión que restó el origen (sin mandar a cocina) | Lo repuesto entra al destino |
| Q5 | Capturar una compra con la presentación que venía del origen | La existencia del destino sube lo que trae; el selector muestra «Nombre · Marca» |
| Q6 | Abrir un conteo | El destino aparece con la suma; el origen no |
| Q7 | Unir un insumo en g en uno en piezas sin equivalencia | La pantalla pide la equivalencia y no deja confirmar |
| Q8 | Con la vista previa abierta, vender el producto en otra pestaña y luego confirmar | «Algo cambió mientras revisabas» y vista previa nueva |
| Q9 | Nuevo artículo › escribir «leche entera lala» | «Ya hay uno parecido: Leche entera» con «Abrir»; guardar de todos modos lo crea |
| Q10 | Dos presentaciones «Caja» con marcas distintas en el mismo insumo | Se guardan las dos; una tercera «Caja» sin marca y otra igual sin marca chocan |
| Q11 | Ver el historial de costos del destino | Aparecen las compras del origen con su nombre, convertidas |
| Q12 | Usuario nuevo (subagente sin contexto) une dos marcas reales y explica qué pasará antes de confirmar | Lo logra sin ayuda; se anotan los tropiezos en texto |

## Pruebas automáticas que deben quedar verdes

- `cd server && go test ./internal/domain/ -run 'Merge|Similar'`
- `make test-integration` — entre ellas `TestMergeMovesEverythingAndLeavesNoRecipeOnTheSource`,
  `TestASaleRacingTheMergeNeverLosesStock`, `TestALateMovementOnAMergedSourceGoesToTheTarget`,
  `TestEveryTableThatPointsToAnArticleIsClassified`, `TestEveryRejectingTableHasTheMergeGuard`,
  la prueba de la 0104 contra un respaldo real con dos empresas, y los tres casos de RLS.
- `cd web && bun run vitest run src/features/backoffice/articles src/shared/preview`
- `bun run e2e` con el spec de unir, contra el `make start` del worktree.
