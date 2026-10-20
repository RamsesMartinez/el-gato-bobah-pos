# Tasks: Gestión de artículos del almacén

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

Las reglas de la constitución que aplican a todas las tareas:

- **TDD estricto (IV).** Cada test se escribe primero y se ve fallar por la razón correcta antes de
  escribir el código.
- **Aislamiento entre empresas.** Toda lectura nueva lleva su test bajo `appRoleStore` con
  `inTheThreeCases`.
- **Pantallas.** Cada tarea de pantalla cita la fila de «Maquetas» del plan que implementa y su
  ajuste de la «Revisión after_plan».
- **Rutas del plan.** «Revisión B1…B4» y «E1…E16» se refieren a esas secciones de [plan.md](plan.md).

## Phase 1: Setup

- [ ] T001 Confirmar que la migración `0096` es la siguiente libre en `server/migrations/` y que
  ninguna rama abierta la usa.
- [ ] T002 Confirmar que el worktree tiene la base restaurada de producción con dos empresas
  (`make db-restaurar`). La base debe traer artículos con ajustes de conteo, compras, una compra
  por recibir y una merma pendiente.

## Phase 2: Foundational (bloquea a todas las historias)

- [ ] T003 [P] Test de dominio en `server/internal/domain/ingredients_test.go`:
  - **Validar la ficha:** nombre de 1 a 60, mínimo ≥ 0 y costo con `ValidMoney`.
  - **`UnitChangePath`:** solo `ajuste` da `direct`; cualquier compra, venta, merma, producción o
    cancelación da `replace`.
  - **`ConvertQty`:** factor con tope `ValidQty`. Masa↔volumen se rechaza (E10).
- [ ] T004 [P] Test de dominio de `CountSanity` en `server/internal/domain/count_sanity_test.go`.
  Una tabla con:
  - las cuatro referencias, en orden: esperado > 0, último conteo, 10 × el empaque más grande y el
    tope de 50 kg, 50 L, 50 m o 5,000 pz;
  - contar 0 nunca avisa;
  - menos de 1/100 del empaque más chico avisa;
  - una referencia ≤ 0 pasa a la siguiente (E13).
- [ ] T005 Implementar `server/internal/domain/ingredients.go` y `count_sanity.go` hasta que T003 y
  T004 pasen.
- [ ] T006 [P] Permiso `inventory.view`:
  - en `AllPermissions` y en el rol gerente, en `server/internal/domain/permission.go` (con su test);
  - su espejo en `web/src/app/permissions.ts`.
- [ ] T007 [P] Las diez acciones de FR-014 más la lectura de FR-016:
  - en la lista blanca de `server/internal/domain/audit.go`;
  - con sus etiquetas en `web/src/shared/audit/labels.ts`.
- [ ] T008 Test de la migración 0096 en `server/internal/integration/migration_0096_test.go`, sobre
  el respaldo restaurado con dos empresas:
  - las dos tablas con RLS `nullif` y grants `select, insert`;
  - las FK compuestas `no action`, y que borrar una empresa en cascada no truena;
  - `check old <> new`;
  - `stock_movements.replacement_id`;
  - el índice de bitácora por entidad y el índice parcial de compras;
  - el Down se niega con datos (E15) y funciona en vacío;
  - las dos tablas entran a `TestEveryCompanyTableIsIsolated`.
- [ ] T009 Migración `server/migrations/0096_ingredient_management.sql`, según
  [data-model.md](data-model.md), hasta que T008 pase. La revisa `db-architect` antes de aplicarla.
- [ ] T010 Consultas en `server/queries/ingredients_admin.sql` y `make sqlc`. Sin nombrar
  `company_id` en tablas de la 0023. Son:
  - lista con filtros;
  - ficha;
  - `update` con `updated_at` esperado;
  - archivar y reactivar;
  - `archive-check`;
  - historial (compras con `quantity > 0`, costo y `expense_item_id`, sin regalos, más las
    correcciones, por llave);
  - bitácora por entidad, junto con la de sus empaques;
  - sugerencias por `unit_kind`;
  - las conversiones del cambio de unidad.

**Checkpoint**: dominio, permiso, acciones y esquema listos.

## Phase 3: User Story 1 — Encontrar un artículo (P1) 🎯 MVP

**Independent test**: escribir «hie» deja solo los artículos o empaques con «hie»; los filtros
funcionan solos y combinados.

- [ ] T011 [P] [US1] Tests de integración de `GET /stock/articles` en
  `server/internal/integration/ingredients_admin_test.go`:
  - texto sin acentos ni mayúsculas, que busca también en los empaques;
  - activos o archivados, lugar, «Sin lugar», sin costo y existencia negativa;
  - `inTheThreeCases`;
  - sin `inventory.view` da 403.
- [ ] T012 [US1] Servicio `ListArticles` en `server/internal/app/ingredients_admin.go` y su handler
  en `server/internal/httpapi/handlers_ingredients_admin.go`. La ruta va con
  `RequirePermission(inventory.view)` en `router.go`.
- [ ] T013 [P] [US1] Test vitest de la lista en
  `web/src/features/backoffice/articles/ArticlesTab.test.tsx`: buscador, chips, «Lugar» con
  «Sin lugar» y estado vacío.
- [ ] T014 [US1] Lista en `web/src/features/backoffice/articles/ArticlesTab.tsx`. Reemplaza la
  pestaña de `StockPage.tsx`. Sigue la fila «Lista de artículos» de Maquetas y su ajuste: «Lugar»
  (`Picker`) y «Activos / Archivados» en la fila del buscador, y los chips «Sin costo» y
  «Existencia negativa». Se mide a 1024×600 con el encabezado real.

## Phase 4: User Story 2 — Crear sin gasto ni receta (P1)

**Independent test**: crear «Leche entera» en líquido desde la lista. Cancelar cualquier captura no
deja artículo.

- [ ] T015 [P] [US2] Tests de integración de `POST /stock/articles`:
  - con existencia inicial: queda un `ajuste` con `reason = 'existencia inicial'` y la sucursal
    resuelta;
  - `BRANCH_AMBIGUOUS` con dos sucursales y sin selector (E12);
  - nombre repetido da 409 con `{id, archived}` (E2);
  - deja el evento `ingredient.created`.
- [ ] T016 [P] [US2] Tests de integración de FR-003, uno por camino (E1). En cada uno, cancelar o
  hacer fallar el guardado no deja artículo:
  - gasto con `newArticle`;
  - receta, «Agregar insumo»;
  - receta, «Crear una nueva».
  Aparte: `POST /ingredients` sigue respondiendo (Revisión B1).
- [ ] T017 [US2] `createArticleTx` en `app/ingredients_admin.go`. `app/expenses.go` y
  `app/composition.go` aceptan `newArticle` dentro de su transacción.
- [ ] T018 [P] [US2] Tests vitest: cancelar la hoja no llama a la API; el gasto y la receta mandan
  `newArticle` y ya no llaman a `createIngredient` (`ExpenseDialog.test.tsx` y
  `CompositionSheet.test.tsx`).
- [ ] T019 [US2] Hoja «Nuevo artículo» en `web/src/features/backoffice/articles/NewArticleSheet.tsx`
  (Maquetas › Lista, ajuste del dueño):
  - hoja corta con encabezado fijo y los obligatorios marcados con *;
  - cantidades con número y selector de unidad;
  - «Reactivar» si el nombre está archivado;
  - al guardar, abre la ficha con «Agregar empaque» a la vista.
- [ ] T020 [US2] Quitar en `ExpenseDialog.tsx` y `CompositionSheet.tsx` la creación al tocar la
  unidad, y mandar `newArticle` en su guardado.

## Phase 5: User Story 3 — Ver y editar la ficha (P1)

**Independent test**: renombrar y cambiar el mínimo. El nombre nuevo se ve en recetas y conteo. El
historial muestra compras y correcciones.

- [ ] T021 [P] [US3] Tests de integración:
  - `PATCH` con `updated_at` viejo da 409 `STALE` (E3);
  - renombrar a un nombre que existe da 409;
  - `POST …/cost` en un preparado da 409 (E11);
  - la corrección pone `cost_source = 'manual'` y deja su fila y su evento;
  - el historial excluye las diferencias negativas y los regalos, y pagina por llave;
  - la bitácora junta los eventos del artículo y de sus empaques;
  - los tres `inTheThreeCases`.
- [ ] T022 [US3] Servicio y rutas de ficha, editar, corregir costo, historial y bitácora (FR-004 a
  FR-006 y FR-016). El servidor entrega el costo por unidad, lo pagado por empaque y el porcentaje
  de tendencia contra la más vieja de las últimas 20, solo con 2 compras o más.
- [ ] T023 [P] [US3] Tests vitest de la ficha:
  - «Ver más» de 20 en 20;
  - sin indicador con menos de 2 compras (E16);
  - «Corregir costo» oculto en un preparado;
  - «¿Descartar cambios?» al salir con cambios.
- [ ] T024 [US3] Ficha en `web/src/features/backoffice/articles/ArticleSheet.tsx` y gráfica en
  `CostTrend.tsx`. Sigue la fila «Ficha del artículo» de Maquetas, con:
  - dos columnas con `Page fill` y `minH={0}`, y una sola columna por debajo de 800 px;
  - el SVG con `viewBox`;
  - «Archivar» al pie de la columna izquierda;
  - «Bitácora» en el encabezado.

## Phase 6: User Story 4 — Archivar y reactivar (P2)

**Independent test**: archivar un artículo sin recetas; deja de ofrecerse y sigue en su historia.

- [ ] T025 [P] [US4] Tests de integración:
  - con receta activa da 409 con la lista (E4);
  - con un conteo abierto que lo capturó da 409;
  - con una compra por recibir se avisa y se archiva; recibirla después no lo reactiva (E5);
  - deja de ofrecerse en el gasto, la receta y el conteo (FR-008);
  - reactivar;
  - los eventos.
- [ ] T026 [US4] Servicio y rutas de `archive-check`, archivar y reactivar.
- [ ] T027 [US4] Diálogo de archivado: cada receta abre su `CompositionSheet` y regresa; sin
  bloqueos, confirma. Ajuste de la Revisión, en `ArticleSheet.tsx`.

## Phase 7: User Story 5 — Cambiar la unidad (P2)

**Independent test**: un artículo solo con ajustes pasa de piezas a gramos y conserva sus recetas;
uno con compras se reemplaza.

- [ ] T028 [P] [US5] Tests de integración del cambio directo (E7, Revisión B2 y B3):
  - convierte existencia, ajustes, renglones de conteo, recetas, empaques, mínimo, costo y
    rendimiento;
  - la existencia queda igual a la suma de movimientos con el factor 1/150 y 30 ajustes;
  - el `before` guarda los originales por `batch_id`;
  - un caso por cambio de clase (masa↔pieza, volumen↔pieza).
- [ ] T029 [P] [US5] Test que recorre las FK hacia `ingredients` y truena si una tabla no está
  clasificada como «convierte» o «bloquea».
- [ ] T030 [P] [US5] Tests de los bloqueos:
  - compra por recibir o merma pendiente da 409 `UNIT_CHANGE_PENDING` con lo que falta;
  - un conteo abierto da 409 (E9);
  - una venta concurrente con `nowait` da 409 `BUSY` (E6).
- [ ] T031 [P] [US5] Tests del reemplazo (E8):
  - un `ajuste` de salida y otro de entrada por sucursal, con `replacement_id` y `branch_id`;
  - las recetas pasan con la cantidad convertida;
  - el nuevo nace con el costo convertido y su fila de corrección;
  - el viejo queda archivado;
  - deja el evento `ingredient.replaced`.
- [ ] T032 [US5] Vista previa y aplicación en `app/ingredients_admin.go`, con el orden de escritura
  de la Revisión B3, hasta que T028 a T031 pasen.
- [ ] T033 [US5] `UnitChangeSheet.tsx` en dos pasos:
  - pie fijo;
  - grupos Existencia, Recetas y Empaques, con los que no convierten exacto primero;
  - «Revisar» deshabilitado sin clase o sin equivalencia;
  - aviso con lo que falta cerrar.

## Phase 8: User Story 6 — Empaques que se entienden (P2)

**Independent test**: en un artículo en litros, las sugerencias no incluyen «Bolsa · 15 kg».

- [ ] T034 [P] [US6] Tests de integración:
  - sugerencias por clase de unidad (E14);
  - crear, editar y retirar un empaque deja su evento sin cambiar las compras pasadas.
- [ ] T035 [US6] Ruta de sugerencias y `recordAudit` en las rutas de empaques que ya existen.
- [ ] T036 [US6] Sección Empaques de la ficha:
  - la explicación con ejemplo;
  - «Bolsa · 5 kg = 5,000 g»;
  - las sugerencias con su cantidad;
  - «Se pesa también» para el empaque aproximado («Kilo ≈ 400 pz»).

## Phase 9: User Story 7 — Contar sin equivocarse (P2)

**Independent test**: 740 kg en un artículo en gramos sin existencia pide confirmar; «No hay»
guarda 0.

- [ ] T037 [P] [US7] Tests de integración de la captura:
  - sin `confirmable` guarda aunque la cifra sea absurda (Revisión B1);
  - con `confirmable` da 409 `NEEDS_CONFIRM` sin revelar la referencia;
  - con `confirmed` guarda;
  - el peso crudo de un artículo en piezas con empaque aproximado lo convierte el servidor;
  - todos los empaques activos y no aproximados se ofrecen;
  - la búsqueda encuentra por nombre de empaque.
- [ ] T038 [US7] Cambios en `app/stock_counts.go` y `queries/stock_counts.sql`.
- [ ] T039 [P] [US7] Tests vitest:
  - tocar la cantidad de un empaque lleva el teclado a ese renglón;
  - «+» y «−» ajustan de uno en uno;
  - «Guardar» sin captura no guarda;
  - «No hay»;
  - selector g/kg (o pz/g/kg con empaque aproximado) que abre en la base;
  - total fijo arriba con más de 3 empaques.
- [ ] T040 [US7] Teclado en `StockCountTab.tsx` y `countEntry.ts` con `NumericKeypad`. Sigue la fila
  «Teclado del conteo» de Maquetas, con aviso no bloqueante y regreso a la lista tras guardar.

## Phase 10: Polish & Cross-Cutting

- [ ] T041 `TestEveryAuditActionIsProduced` en verde con las acciones nuevas.
- [ ] T042 [P] `make api-build && make api-test`, `make test-integration`, `cd web && bun run lint
  && bun run vitest run && bun run build`, y `make ci-local`.
- [ ] T043 [P] Playwright a 1024×600 en `web/e2e/gestion-de-articulos.spec.ts`:
  - lista con 6 o 7 renglones visibles;
  - ficha con la tableta de lado y de pie;
  - teclado del conteo.
  La prueba con usuario sin contexto corre en local (constitución 1.23.0).
- [ ] T044 [P] Medición de uso: las pantallas nuevas van en `domain/uso.go` y
  `web/src/app/rutas-medidas.ts`.
- [ ] T045 Correr [quickstart.md](quickstart.md) contra la base restaurada y anotar en el plan, en
  «Al construir», lo que no se vio venir.
- [ ] T046 En una subida posterior: borrar `POST /ingredients` (Revisión B1).

## Dependencies

- **Setup → Foundational → historias.**
- **Orden entre historias:**
  - US1, US2 y US3 se hacen en ese orden, porque la ficha se abre desde la lista y desde la alta.
  - US4, US5 y US6 dependen de US3, porque viven en la ficha.
  - US7 solo depende de Foundational.
- **Dentro de cada historia:** primero los tests marcados [P], luego el servicio y al final la
  pantalla.

## Parallel examples

- **Foundational:** T003, T004, T006 y T007 a la vez.
- **US5:** T028, T029, T030 y T031 se escriben a la vez y después T032.
- **US7** puede ir en paralelo con US2 a US6.

## Implementation strategy

1. **MVP:** US1, US2 y US3. Con eso se puede encontrar, crear y editar sin pasar por un gasto, y ya
   se pueden corregir los costos que pidió finanzas.
2. **Después:** US7, el conteo, que es el que evita el error de 740 kg.
3. **Al final:** US4, US5 y US6.
