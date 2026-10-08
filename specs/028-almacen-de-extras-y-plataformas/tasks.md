# Tasks: El almacén descuenta lo que de verdad se vendió

**Tests**: obligatorios y primero (constitución IV). Integración bajo `appRoleStore` con
`inTheThreeCases` para lo que lee tablas de empresa; migración sobre `restoredStore`.

## Phase 1: Defectos existentes (se reproducen primero)

- [x] T001 Test: cancelar un renglón antes de cocina y después el pedido entero repone una sola vez; cancelar un pedido con un renglón ya enviado a cocina no repone ese renglón (server/internal/integration/stock_composition_test.go)
- [x] T002 `RestockCancelledOrder` neto por renglón (server/queries/orders.sql). La regla de no reponer lo ya enviado a cocina NO se aplicó al pedido completo: sería un cambio de negocio y queda para el dueño, y `CancelarConDevolucion` con `GetOrderForUpdate` (server/internal/app/devolucion.go)
- [x] T003 [P] Test de tableta: `ProductEditDialog` y `OptionFormDialog` hacen scroll en el cuerpo y «Guardar» queda visible (web/src/features/admin/shared/*.test.tsx)
- [x] T004 `scrollBehavior="inside"` y `placement="center"` en los dos diálogos
- [x] T005 [P] `fudo-import`: la consulta de recetas filtra la empresa explícita (server/cmd/fudo-import/main.go:504)

## Phase 2: Fundación

- [x] T006 [P] Tests de dominio de `ExpandSale` (server/internal/domain/composition_test.go): receta por cantidad; existencias propias; extra con receta y extra ligado a producto, multiplicados por cantidad del extra y del renglón; paquete con la semántica de `productCost`; insumo preparado por rendimiento; cantidad positiva que no redondea a cero; un ciclo viejo que no cuelga la venta; recorrido lineal ante capas de insumos (auditoría); origen de cada delta
- [x] T007 [P] Tests de `ValidateComposition`: ciclo directo e indirecto, paquete en paquete. La unidad de otro tipo va con los endpoints (T016): el grafo no carga unidades
- [x] T008 Test de la migración 0078 sobre respaldo real: estados rellenados (receta o producto ligado → estimada; `track_stock` → confirmada), columnas nuevas, único en `order_lines`, Down
- [x] T009 server/migrations/0078_stock_composition.sql según data-model.md
- [x] T010 `domain/composition.go`: `ExpandSale`, `ValidatePackage`, `ValidatePrepIngredient`, sentinels. Envuelven `ErrValidation`, así que respond.go no cambia. Un insumo preparado que no se puede descomponer (ciclo viejo, sin rendimiento) se descuenta él mismo
- [x] T011 Consultas del grafo de composición por empresa (server/queries/composition.sql) y cargador en server/internal/app/composition.go, reusando `ListComboSlotDefaultsForCosting`

## Phase 3: US2 — El mostrador descuenta los extras

- [x] T012 Test: venta con extras (receta, producto ligado, sin composición) y paquete; movimientos con su origen; cancelar renglón repone todo lo del renglón; sucursal correcta
- [x] T013 `Create` y `AddLines` usan `ExpandSale` en lugar de `descontarRenglon`; escriben `order_line_components` para paquetes (server/internal/app/orders.go)

## Phase 4: US3 — Los pedidos de plataforma descuentan

- [x] T014 Test: aceptar un pedido con platillo y opción emparejados descuenta los dos; el genérico no; registrar guarda la opción emparejada del renglón hijo
- [x] T015 `registrarPedido` guarda `modifier_option_id` de los hijos emparejados; `Aceptar` expande y descuenta en su transacción (server/internal/app/pedidos_de_plataforma.go). La cantidad de la opción se toma por unidad del platillo, como en mostrador: sin un pedido real con cantidad > 1, no verificado

## Phase 5: US1 — Capturar la composición

- [x] T016 Test de integración: guardar composición (insumos o producto ligado) de producto y extra (la del insumo compuesto va con T020); confirmar; rechazos de ciclo y de insumo de otra empresa; aislamiento en los tres casos; solo admin y gerente
- [x] T017 Endpoints de composición (leer, guardar, confirmar) en server/internal/app/composition.go y httpapi/handlers_composition.go; filtro de composición en la lista del catálogo
- [x] T018 [P] Tests de la hoja «Qué lleva»: renglones con Picker y cantidad, quitar de 44 px, «Es el producto…», confirmar, estado estimado visible
- [x] T019 `CompositionSheet` y el botón «Qué lleva ›» en los dos diálogos; filtro en el menú de `ProductsAdminPage`

## Phase 6: US4 y US5 — Paquetes e insumos compuestos

- [x] T020 Test: el historial por producto cuenta componentes de paquete sin los renglones cancelados; un insumo preparado descuenta sus componentes
- [x] T021 Consulta de vendidos por producto incluyendo componentes. Quedó en Reportes y no en Almacén: comparte el periodo de la utilidad por producto, y en Almacén no hay rango de fechas

## Phase 7: Carga de FUDO

- [x] T022 Test (con CSV de prueba en testdata, sin datos del negocio): no pisa lo capturado; marca estimada; reporta nombres ambiguos e insumos inexistentes; filtra por empresa
- [x] T023 `cmd/fudo-import -compositions -company <slug> [-dry-run]` (server/cmd/fudo-import/compositions.go, lógica en server/internal/fudoimport). Los paquetes de FUDO se reportan y no se convierten: cambiar un producto a paquete cambia cómo se vende

## Phase 8: Cierre

- [x] T024 AGENTS.md (cómo se descuenta, la carga de FUDO) y docs/emparejamiento-de-plataformas.md §6
- [x] T025 Gates completos
- [x] T026 Ensayo en pruebas con respaldo de producción y carga de FUDO; cobrar lo creado. La 0078
  corrió en ~150 ms sobre la copia de producción (incluye el único nuevo de `order_lines`, que
  bloquea ventas mientras se construye). Venta con extra, paquete armado en «Qué lleva» y vendido,
  insumo preparado confirmado, reporte de unidades y filtros: todo con su origen y en la sucursal
  del pedido. Pedidos cobrados y entregados; el ambiente se regresó a su base. **No se ensayó**
  aceptar un pedido de plataforma: no hay credenciales de Uber en pruebas (lo cubren las pruebas
  de integración)
- [x] T027 `/revision-de-codigo`, dos rondas. La segunda encontró la expansión exponencial, las
  validaciones fuera de la transacción, el paquete con hueco vacío y la hoja que nacía abierta

## Phase 9: Catálogo › Recetas (rediseño tras revisar la pantalla con el dueño)

La hoja «Qué lleva» funcionaba pero no se entendía: no decía qué faltaba ni por dónde empezar. El
vocabulario (Receta, Insumo, Preparado, Extra, Combo; Pendiente, Por revisar, Lista) y el prototipo
los aprobó el dueño el 2026-10-07.

- [x] T028 Test y consulta: lista de recetas por producto, extra y preparado con estado, resumen,
  ventas de 30 días, búsqueda sin acentos que también busca por insumo, y su `Count` con el mismo
  `where` (server/queries/recipes.sql, app/recipes.go, domain/recipes.go)
- [x] T029 Test y servicio: confirmar de una vez las estimadas cargadas; guardar la misma receta en
  los extras del mismo nombre; aviso de copia vieja (409) también para esos extras
- [x] T030 Pestaña Catálogo › Recetas (RecipesPage) y hoja de receta reescrita (CompositionSheet,
  SearchSheet): cantidades escritas, cambio de unidad, copiar de otro, abrir la siguiente al guardar
- [x] T031 Revisiones (base de datos, Go, seguridad, tableta) y capturas a 1024×600 contra una copia
  de producción. Lo que salió se arregló con su test: extra repetido, desde desbordado, NUL en la
  búsqueda, gemelo pisado, aislamiento de extras y preparados, «0.215 kg» en vez de «215 g»

## Dependencies

Phase 1 primero (defectos). Phase 2 bloquea 3 a 7. US2 antes que US3 (comparten la expansión). La
carga de FUDO necesita la 0078.
