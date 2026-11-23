---
description: "Tareas de la spec 096: merma esperada (subida 1, costos) e inventario y conteo (subida 2)"
---

# Tasks: Merma esperada, conteo contra ventas y rediseño del inventario

**Input**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD): cada test va antes de su implementación y se ve en rojo
por la razón correcta.

**Dos subidas**:
- **Subida 1 — Costos** (Fases 1 a 7): se implementa y sube ya.
- **Subida 2 — Inventario y conteo** (Fases 8 a 13): **no se empieza** hasta que el #65 (spec 094,
  varios almacenes) esté completo en producción. Va encima de sus pantallas con selector de almacén.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: puede ir en paralelo (otro archivo, sin dependencia pendiente).
- **[Story]**: US1–US9 de [spec.md](spec.md).

---

# SUBIDA 1 — COSTOS

## Phase 1: Setup

- [X] T001 Traer `origin/develop` en local con merge firmado y confirmar que la migración libre es la 0126 (`ls server/migrations`); si develop ya trae una 0121, renumerar esta (R9, R14 M4)
- [X] T002 Crear `server/migrations/0126_expected_waste.sql` vacío con su cabecera goose y `set local lock_timeout = '3s'`

## Phase 2: Foundational (bloquea US1–US5)

### Tests primero

- [X] T003 Test de migración en `server/internal/integration/migration_0126_test.go` sobre la base restaurada con dos empresas: (a) tras el Up, `ingredients.expected_waste_pct` = `waste_pct` donde > 0 y nulo donde 0, `products` y `business_settings` nulos; (b) recalcular costos tras el Up no cambia ningún `current_cost` de producto, extra ni preparado (SC-001); (c) el Down copia `expected_waste_pct` a `waste_pct` antes de quitar las columnas (R14 H3)
- [X] T004 [P] Tests table-driven en `server/internal/domain/expected_waste_test.go`: `ExpectedWastePct(article, fallback)` (artículo capturado gana, 0 capturado gana y da 0, nulo cae al fallback, ambos nulos = 0 con origen `none`) y `ValidExpectedWastePct` (nulo válido; 0, 99.99 válidos; −0.01, 100, NaN, ±Inf, 3 decimales → `ErrValidation`)
- [X] T005 [P] Tests en `server/internal/domain/costing_test.go`: con 5 % general un producto sin otros % sube exactamente 5 %; 10 % producto + 5 % general → 10 %; artículo 15 % dentro de producto 10 % → 15 % solo en ese renglón; artículo 0 % capturado con general 5 % → 0; preparado dentro de preparado no recibe el general ni el del producto (no compone, R2); extra con receta usa artículo → general; combo suma sus productos cada uno con su %; producto de costo manual no cambia

### Implementación

- [X] T006 Escribir el Up y el Down de `server/migrations/0126_expected_waste.sql` según [data-model.md](data-model.md) (tres columnas nullable con check `>= 0 and < 100`; copia de `waste_pct > 0`; Down copia de regreso y quita)
- [X] T007 [P] Crear `server/internal/domain/expected_waste.go` con `WasteSource`, `ExpectedWastePct` y `ValidExpectedWastePct`
- [X] T008 Cambiar `server/internal/domain/costing.go`: `CostIngredient.ExpectedWastePct *float64`, `CostProduct.ExpectedWastePct *float64`, `NewCostGraph(..., general *float64)`; `recipeCost` recibe el fallback; `recCache` solo para preparados (R3)
- [X] T009 Cambiar `server/queries/costing.sql` (`ListIngredientsForCosting` lee `expected_waste_pct`, `ListProductsForCosting` también, consulta nueva del general en `server/queries/settings.sql`) y correr `make sqlc`
- [X] T010 Cambiar `loadGraph` en `server/internal/app/costing.go` para cargar el general y los % nuevos; `ProductCost` usa el mismo grafo
- [X] T011 [P] Cambiar `server/cmd/fudo-import/main.go` para escribir `expected_waste_pct` (nulo si 0) en vez de `waste_pct`
- [X] T012 Correr `cd server && go build ./... && go test ./internal/domain/` y la prueba T003 contra el Postgres de pruebas (`make test-integration`)

**Checkpoint**: el motor aplica la precedencia; nada visible cambia todavía.

---

## Phase 3: US1 — Merma general en Ajustes (P1)

**Goal**: capturar el % general y que los costos se recalculen.
**Independent Test**: producto de $20.00 con general vacío; 5 % → $21.00; vaciar → $20.00.

- [X] T013 [P] [US1] Test de integración en `server/internal/integration/expected_waste_test.go` bajo `appRoleStore` con `inTheThreeCases`: `PUT /settings/expected-waste` con 5 recalcula el costo del producto de prueba al centavo, con `null` lo regresa; otra empresa no ve ni cambia el general; bitácora `settings.expected_waste` con antes y después
- [X] T014 [P] [US1] Tests de frontera en `server/internal/httpapi/handlers_expected_waste_test.go`: cuerpo sin la llave `pct` = 400; `pct` 100, −1, texto = 400; sin `menu.configure` = 403
- [X] T015 [US1] Agregar `AuditSettingsExpectedWaste`, `AuditProductExpectedWaste`, `AuditIngredientExpectedWaste` (grupo precio) en `server/internal/domain/audit.go` y sus etiquetas en `web/src/shared/audit/labels.ts`
- [X] T016 [US1] Servicio `SetGeneralExpectedWaste` en `server/internal/app/expected_waste.go`: valida, actualiza, deja bitácora y llama a `recomputeCosts` en la misma transacción; un candado ocupado sale como `ErrConflict` «Se está recalculando el costo; intenta en un momento»
- [X] T017 [US1] Consulta `UpdateExpectedWaste` en `server/queries/settings.sql`, `expectedWastePct` en `GetBusinessSettings`, `make sqlc`
- [X] T018 [US1] Handler fino `PutGeneralExpectedWaste` en `server/internal/httpapi/handlers_expected_waste.go` y ruta `PUT /settings/expected-waste` con `RequirePermission(menu.configure)` en `server/internal/httpapi/router.go`; `expectedWastePct` en la respuesta de `/business-settings`
- [X] T019 [P] [US1] Test vitest en `web/src/features/admin/CostSettingsSection.test.tsx`: vacío dice «Sin merma» y manda `null`; teclear 5 y Guardar manda 5; Guardar deshabilitado sin cambio; 400 muestra el error del campo
- [X] T020 [US1] Sección «Costos» (maqueta A, plan › Maquetas fila «Ajustes › Costos») en `web/src/features/admin/CostSettingsSection.tsx` y su entrada en `web/src/features/admin/BusinessSettingsPage.tsx` visible con `can('menu.configure')`; campo de 44 px, texto «Sube el costo de lo que preparas…», sin repetir el título; API en `web/src/api/backoffice.ts` y tipo en `web/src/api/pos.ts`

**Checkpoint**: el general funciona de punta a punta.

---

## Phase 4: US2 — % por producto (P1)

**Goal**: un producto reemplaza al general.
**Independent Test**: 5 % general y 10 % en el producto → sube 10 %, no 15 %.

- [X] T021 [P] [US2] Test de integración en `server/internal/integration/expected_waste_test.go`: `PUT /admin/products/{id}/expected-waste` 10 con general 5 → +10 %; 0 → sin merma; `null` → vuelve al general; producto de otra empresa = 404 (`inTheThreeCases`); bitácora `product.expected_waste`
- [X] T022 [US2] Servicio `SetProductExpectedWaste` en `server/internal/app/expected_waste.go` (valida, `notFound`, bitácora, recálculo en la misma transacción) y consulta `AdminSetProductExpectedWaste` en `server/queries/admin.sql`; `expected_waste_pct` en `AdminListProducts`/detalle; `make sqlc`
- [X] T023 [US2] Handler y ruta `PUT /admin/products/{id}/expected-waste` (grupo `menuConfigure`) en `server/internal/httpapi/handlers_expected_waste.go` y `router.go`; `expectedWastePct` en `web/src/api/admin.ts` (`AdminProduct`, opcional)

**Checkpoint**: el % del producto se guarda; la pantalla llega en US4.

---

## Phase 5: US3 — % por artículo con «sin captura» (P1)

**Goal**: capturar 0 % a propósito distinto de «sin capturar».
**Independent Test**: fresa 15 %, leche sin captura, lata 0 % en un producto con 10 % → 15 / 10 / 0.

- [X] T024 [P] [US3] Test de integración en `server/internal/integration/expected_waste_test.go`: `PUT /stock/articles/{id}/expected-waste` 0 y `null` dejan estados distintos y el costo lo refleja; artículo ajeno = 404 (`inTheThreeCases`); bitácora `ingredient.expected_waste`
- [X] T025 [P] [US3] Test en `server/internal/integration/ingredient_merge_test.go`: unir un origen con 6 % a un destino sin captura deja el destino en 6 %; si el destino tenía 3 %, se queda en 3 % (R14 M1)
- [X] T026 [US3] Servicio `SetArticleExpectedWaste` en `server/internal/app/expected_waste.go` y consulta en `server/queries/ingredients_admin.sql`; `expectedWastePct` en `GetArticle`; `make sqlc`
- [X] T027 [US3] Herencia del % al unir en `server/internal/app/ingredient_merge.go` (y su consulta en `server/queries/ingredient_merge.sql`)
- [X] T028 [US3] Handler y ruta `PUT /stock/articles/{id}/expected-waste` con `menu.configure` en `server/internal/httpapi/handlers_expected_waste.go` y `router.go`; revisar que ninguna inserción del MCP ni de `harness_test.go` liste `waste_pct`
- [X] T029 [P] [US3] Test vitest en `web/src/features/backoffice/articles/ExpectedWasteField.test.tsx`: «Sin capturar» manda `null`; «Capturada» con 0 muestra «Aquí no se pierde nada» y manda 0; vacío nunca manda 0; sin `menu.configure` se ve pero no se edita
- [X] T030 [US3] Panel «Merma al preparar» (maqueta A2, plan › Maquetas fila «Ficha del artículo») en `web/src/features/backoffice/articles/ExpectedWasteField.tsx` (`Segmented` «Sin capturar / Capturada», `InfoButton`, `Input` de 44 px, `bg.muted`) y colocarlo justo después de «Ubicación» en `web/src/features/backoffice/articles/ArticleSheet.tsx`, guardando con el autoguardado de la ficha y su indicador; «Compras y correcciones» sin mayúsculas

**Checkpoint**: los tres niveles se capturan.

---

## Phase 6: US4 — De dónde sale el costo (P1)

**Goal**: desglose por renglón con %, origen y pesos.
**Independent Test**: el producto de US3 muestra 3 renglones con su origen y la suma es el total.

- [X] T031 [P] [US4] Tests en `server/internal/domain/costing_test.go` para `ProductBreakdown`: renglones con base, %, origen y merma redondeados a 4 decimales; total = suma de renglones (SC-003); preparado muestra su costo con su merma interna y solo la merma aplicada al usarlo; combo trae componentes; manual y compra traen `source` sin renglones; arreglos vacíos nunca nulos
- [X] T032 [P] [US4] Test en `server/internal/httpapi/handlers_menu_test.go` sobre el JSON crudo de `GET /products/{id}/costing`: `lines`, `components` y `realWaste` son `[]` y no `null` cuando no hay
- [X] T033 [US4] `CostGraph.ProductBreakdown` en `server/internal/domain/costing.go` y el servicio que lo arma (con nombres y unidades) en `server/internal/app/costing.go`; amplía `ProductCosting` en `server/internal/httpapi/handlers_menu.go` según [contracts/api.md](contracts/api.md)
- [X] T034 [US4] Test de integración en `server/internal/integration/expected_waste_test.go`: vender un producto, cambiar el general, el `order_lines.unit_cost` de esa venta no cambia (SC-004, R7)
- [X] T035 [P] [US4] Tests vitest en `web/src/shared/producto/CostTab.test.tsx`: campo vacío dice la general y manda `null`; las tres cifras y la suma de renglones coinciden; costo manual muestra la línea y no el campo; combo lista sus productos; un % inválido marca el punto rojo de la pestaña Costo
- [X] T036 [US4] Pestaña «Costo» (maqueta B, plan › Maquetas fila «Ficha del producto») en `web/src/shared/producto/CostTab.tsx` y `web/src/shared/producto/CostBreakdownTable.tsx` (5 columnas, encabezado y primera columna fijos, `overflowX=auto`, origen en palabras «general del negocio / de este producto / de este artículo»), registrada en `PESTANAS` entre «Receta e inventario» y «Extras» de `web/src/shared/ProductEditDialog.tsx`; el % viaja con el Guardar del diálogo (llama al `PUT` de T023) y entra a `erroresDe()`; en General, «Ver cómo se calcula ›» junto al costo de receta abre la pestaña

**Checkpoint**: la pantalla de costos explica cada peso.

---

## Phase 7: US5 — Merma real de 90 días (P2)

**Goal**: ver la merma real por almacén junto al %.
**Independent Test**: 10 kg vendidos y 0.5 kg mermados → 5 %; el costo no cambia.

- [X] T037 [P] [US5] Tests en `server/internal/domain/expected_waste_test.go` para `RealWasteRatio` (merma/consumo; consumo ≤ 0 con merma → `no_sales`; ambos 0 → `no_data`; redondeo a 1 decimal) y `MergeChain` (factores multiplicados, profundidad 16, solo `kind = 'merge'`)
- [X] T038 [P] [US5] Tests de integración en `server/internal/integration/real_waste_test.go` bajo `appRoleStore` con `inTheThreeCases`: aplicadas y cubiertas cuentan; comida del personal, por aprobar, rechazadas y deshechas no; cancelación resta del consumo; merma de producto terminado no suma a ningún artículo; dos almacenes dan dos cifras; artículo unido con venta y merma previas da el % del conjunto con su factor (R14 H1); tras un cambio de unidad el artículo nuevo solo cuenta desde el cambio
- [X] T039 [US5] Consulta agrupada `ListRealWaste` (`ingredient_id = any($1)`, por `ingredient_id, warehouse_id`, últimos 90 días, con los orígenes unidos) en `server/queries/expected_waste.sql`; `make sqlc`
- [X] T040 [US5] Servicio `RealWaste` en `server/internal/app/expected_waste.go` (aplica `MergeChain` y `RealWasteRatio`, nombra almacenes) usado por el desglose (T033) y por `GET /stock/articles/{id}/real-waste` (`inventory.view`) en `server/internal/httpapi/handlers_expected_waste.go` y `router.go`
- [X] T041 [P] [US5] Tests vitest en `web/src/features/backoffice/articles/RealWasteCard.test.tsx`: un renglón por almacén; «sin ventas» y «sin datos»; ícono de aviso y «pasa lo esperado» solo cuando la real supera el % aplicado
- [X] T042 [US5] Tarjeta «Merma real, últimos 90 días» gemela del costo (maqueta A2) en `web/src/features/backoffice/articles/RealWasteCard.tsx` y su lugar en `ArticleSheet.tsx`; la merma real bajo el % en `CostBreakdownTable.tsx`

**Checkpoint**: subida 1 completa.

---

## Phase S1-Polish: cierre de la subida 1

- [X] T043 Nota en `docs/mecanica/almacen.md`: la precedencia, por qué el general no compone en preparados, que la merma real sigue las uniones y que el Down copia el % de regreso
- [X] T044 E2E en `web/e2e/merma-esperada.spec.ts` contra el `make start` del worktree a 1024×600 y 1280×854 (capturas solo en `test-results/`): general 5 % sube el costo, % del producto, 0 % de artículo, desglose suma, historial del artículo con ≥ 2 renglones visibles a 1024×600; cuenta los toques de capturar el general y el % de un producto (≤ 4 cada uno, SC-005); deja todo como estaba
- [X] T045 `/revision-de-codigo` sobre el diff y corregir cada hallazgo con su test
- [X] T046 Prueba con alguien nuevo de una fonda a 1024×600 en local con la base restaurada (subagente sin contexto): capturar el general, el de un producto y ver de dónde sale el costo; anotar en este spec lo que se atore
- [X] T047 `make ci-local` en verde
- [ ] T048 PR a develop al día (develop en local con commit firmado), CI verde y merge
- [ ] T049 Release develop → main: respaldo previo, PR nombrando lo que trae develop de otras sesiones, CI verde, merge, deploy hasta `readyz` 200, journal de solo lectura sin ERROR ni 5xx propios
- [ ] T050 Comentar en el #226 qué entró y que la subida 2 queda lista en esta lista de tareas para cuando el #65 esté completo

---

# SUBIDA 2 — INVENTARIO Y CONTEO (cuando el #65 esté completo en producción)

## Phase 8: Setup de la subida 2

- [ ] T051 Confirmar que la spec 094 entregó E2–E5 en producción (ventas, compras, traspasos, conteo y merma por almacén); si no, esta subida no empieza
- [ ] T052 Medir la carga de hoy con el build servido en local (`bun run build && bun run preview`), base restaurada, 1024×600, CPU 4×: primera pantalla y lista completa de Existencias y del conteo; anotar las cifras en [research.md › R13](research.md) (SC-007)
- [ ] T053 Migración siguiente libre (al traer develop) con solo `alter type stock_movement_type add value 'merma_esperada'`, y otra con `business_settings.expected_waste_deducts boolean not null default false` y, en ese orden, `drop trigger` de los dos espejos y `drop function mirror_expected_waste_pct()` de la 0126 y `drop column ingredients.waste_pct` (ninguna versión en producción la lee desde la subida 1), cada una con su test de migración

## Phase 9: US6 — Conteo contra ventas (P1)

**Independent Test**: 10 + 5 − 12 − 0.3 = 2.7 esperado, 2.2 contado → −0.5 kg y −4.2 %.

- [ ] T054 [P] [US6] Tests table-driven en `server/internal/domain/count_vs_sales_test.go`: renglón (inicial, entradas, ventas, merma esperada, merma registrada, traspasos, esperado, contado, diferencia, %); sugerencia solo con tres conteos estables entre 2 y 4 % y rango ≤ 1.5 puntos, redondeada a 0.5; «revisar» con > 10 % o errática; primer conteo sin comparación
- [ ] T055 [P] [US6] Test de integración en `server/internal/integration/count_vs_sales_test.go` con `inTheThreeCases`: solo cuenta movimientos del almacén del conteo; traspasos como entrada o salida, no venta
- [ ] T056 [US6] Consulta agrupada por artículo y tipo de movimiento entre conteos en `server/queries/expected_waste.sql`, servicio en `server/internal/app/expected_waste.go`, `GET /stock/counts/{id}/vs-sales` (`inventory.view`) y `POST /stock/articles/{id}/expected-waste/apply-suggestion` (`menu.configure`, bitácora)
- [ ] T057 [P] [US6] Test vitest de `web/src/features/backoffice/CountVsSalesReport.test.tsx` (tres grupos, «Usar N %» llama al endpoint, primer conteo)
- [ ] T058 [US6] Reporte agrupado (maqueta C, plan › Maquetas fila «Conteo vs ventas») en `web/src/features/backoffice/CountVsSalesReport.tsx` junto a `CountLossReport`

## Phase 10: US7 — La merma esperada descuenta inventario (P2)

- [ ] T059 [P] [US7] Tests en `server/internal/integration/expected_waste_deduct_test.go`: apagada descuenta como hoy; encendida agrega `merma_esperada` = receta × % (precedencia de R2) en el almacén de venta; cancelar el pedido, cancelar un renglón, partir y pasar un producto reponen también la merma esperada («cancelar renglón con merma esperada deja el almacén igual», R14 H2); activar no toca ventas pasadas
- [ ] T060 [US7] Escribir los movimientos `merma_esperada` en la venta y ampliar cada consulta que hoy filtra `movement_type = 'venta'`: `server/queries/orders.sql` (cancelar pedido, cancelar renglón, partir/pasar, ventas sin renglón), `server/queries/inventory_health.sql`, `server/queries/shopping_list.sql` (decidir y anotar si el consumo cuenta la merma esperada)
- [ ] T061 [US7] Interruptor en `web/src/features/admin/InventorySettingsSection.tsx` (plan › Maquetas fila «Ajustes › Almacén: descontar merma») y en `PUT /settings/inventory`; nota en el plan de la subida: no se enciende mientras corra la versión anterior (R14 M3)
- [ ] T062 [US7] Mapas de tipo de movimiento en Go y en `web/` (Movimientos, etiquetas) muestran «Merma esperada» y no truenan con el valor nuevo

## Phase 11: US8 — Conteo sencillo y avanzado (P1)

- [ ] T063 [P] [US8] Test de integración: quien solo tiene `inventory.count` no recibe lo esperado en el detalle del conteo (conteo a ciegas en el servidor, R12)
- [ ] T064 [P] [US8] Tests vitest de la vista sencilla en `web/src/features/backoffice/StockCountTab.test.tsx`: lugar con avance N/M, teclado fijo, contar en la presentación elegida convierte a la unidad base, aviso «menos de la mitad / más del doble» contra el conteo anterior sin mostrar lo esperado, cambiar de vista y regresar conserva lo capturado, F5 conserva lo capturado
- [ ] T065 [US8] Vista sencilla (maqueta B, plan › Maquetas fila «Conteo sencilla/avanzada») y avanzada en `web/src/features/backoffice/StockCountTab.tsx`, cambio de vista con `Segmented` con íconos, vista recordada por equipo con try/catch, forzada a sencilla sin `inventory.configure`

## Phase 12: US9 — Existencias sencilla y avanzada y carga medida (P2)

- [ ] T066 [P] [US9] Tests vitest en `web/src/features/backoffice/StockPage.test.tsx`: tablero de tres columnas por estado y cambio de vista con íconos
- [ ] T067 [US9] Vista sencilla en tablero y avanzada con valor y mínimos (maqueta B, plan › Maquetas fila «Existencias») en `web/src/features/backoffice/StockPage.tsx`
- [ ] T068 [US9] Atacar lo que la medición de T052 señale (peticiones por renglón, lista sin paginar ni virtualizar, índices que empiecen por `company_id`); volver a medir y anotar antes y después en [research.md › R13](research.md) contra SC-007

## Phase 13: Cierre de la subida 2

- [ ] T069 E2E a 1024×600 y 1280×854: conteo completo en la vista sencilla, reporte, opción de descontar; prueba con alguien nuevo de una fonda (SC-006)
- [ ] T070 `/revision-de-codigo`, `make ci-local`, PR, release y comentario en el #226; cerrar el issue y el worktree solo con OK del dueño

---

## Dependencies & Execution Order

- Fase 1 → Fase 2 (bloquea todo) → US1, US2, US3 (paralelas entre sí) → US4 (usa T023 y T026) → US5 (usa el desglose de US4) → cierre de la subida 1.
- La subida 2 depende del #65 completo (T051) y de la subida 1 en producción (la migración de S2 borra `waste_pct`).
- Dentro de cada historia: tests en rojo → dominio → consultas y `make sqlc` → servicio → handler → front.

## Parallel Example: Subida 1

```text
T004 y T005 (tests de dominio) en paralelo.
T013, T021, T024 (integración) en paralelo una vez hecha la Fase 2.
T019, T029, T035, T041 (vitest) en paralelo con el backend de su historia.
```

## Implementation Strategy

- **MVP**: Fases 1–3 (el general) ya entrega el pedido original; US2–US5 completan la subida 1 y suben juntas.
- La subida 2 se implementa con este mismo documento cuando T051 se cumpla; cualquier cambio de la 094 entre hoy y entonces se refleja aquí antes de empezar (`/speckit-analyze` otra vez).
