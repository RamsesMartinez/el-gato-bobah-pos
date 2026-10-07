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

- [x] T006 [P] Tests de dominio de `ExpandSale` (server/internal/domain/composition_test.go): receta por cantidad; existencias propias; extra con receta y extra ligado a producto, multiplicados por cantidad del extra y del renglón; paquete con la semántica de `productCost`; insumo preparado por rendimiento; cantidad positiva que no redondea a cero; corte por profundidad ante un ciclo viejo; origen de cada delta
- [x] T007 [P] Tests de `ValidateComposition`: ciclo directo e indirecto, paquete en paquete. La unidad de otro tipo va con los endpoints (T016): el grafo no carga unidades
- [x] T008 Test de la migración 0078 sobre respaldo real: estados rellenados (receta o producto ligado → estimada; `track_stock` → confirmada), columnas nuevas, único en `order_lines`, Down
- [x] T009 server/migrations/0078_stock_composition.sql según data-model.md
- [x] T010 `domain/composition.go`: `ExpandSale`, `ValidatePackage`, `ValidatePrepIngredient`, sentinels. Envuelven `ErrValidation`, así que respond.go no cambia. Un insumo preparado que no se puede descomponer (ciclo viejo, sin rendimiento) se descuenta él mismo
- [ ] T011 Consultas del grafo de composición por empresa (server/queries/composition.sql) y cargador en server/internal/app/composition.go, reusando `ListComboSlotDefaultsForCosting`

## Phase 3: US2 — El mostrador descuenta los extras

- [ ] T012 Test: venta con extras (receta, producto ligado, sin composición) y paquete; movimientos con su origen; cancelar renglón repone todo lo del renglón; sucursal correcta
- [ ] T013 `Create` y `AddLines` usan `ExpandSale` en lugar de `descontarRenglon`; escriben `order_line_components` para paquetes (server/internal/app/orders.go)

## Phase 4: US3 — Los pedidos de plataforma descuentan

- [ ] T014 Test: aceptar un pedido con platillo y opción emparejados descuenta los dos; el genérico no; registrar guarda la opción emparejada del renglón hijo
- [ ] T015 `registrarPedido` guarda `modifier_option_id` de los hijos emparejados; `Aceptar` expande y descuenta en su transacción (server/internal/app/pedidos_de_plataforma.go)

## Phase 5: US1 — Capturar la composición

- [ ] T016 Test de integración: guardar composición (insumos o producto ligado) de producto, extra e insumo; confirmar; rechazos de ciclo y de insumo de otra empresa; aislamiento en los tres casos; solo admin y gerente
- [ ] T017 Endpoints de composición (leer, guardar, confirmar) en server/internal/app/composition.go y httpapi/handlers_composition.go; filtro de composición en la lista del catálogo
- [ ] T018 [P] Tests de la hoja «Qué lleva»: renglones con Picker y cantidad, quitar de 44 px, «Es el producto…», confirmar, estado estimado visible
- [ ] T019 `CompositionSheet` y el botón «Qué lleva ›» en los dos diálogos; filtro en el menú de `ProductsAdminPage`

## Phase 6: US4 y US5 — Paquetes e insumos compuestos

- [ ] T020 Test: el historial por producto cuenta componentes de paquete sin los renglones cancelados; un insumo preparado descuenta sus componentes
- [ ] T021 Consulta de vendidos por producto incluyendo componentes, y su lugar en Almacén

## Phase 7: Carga de FUDO

- [ ] T022 Test (con CSV de prueba en testdata, sin datos del negocio): no pisa lo capturado; marca estimada; reporta nombres ambiguos e insumos inexistentes; filtra por empresa
- [ ] T023 `cmd/fudo-import -compositions -company <slug>` (server/cmd/fudo-import/compositions.go)

## Phase 8: Cierre

- [ ] T024 AGENTS.md (cómo se descuenta, la carga de FUDO) y docs/emparejamiento-de-plataformas.md §6
- [ ] T025 Gates completos
- [ ] T026 Ensayo en pruebas con respaldo de producción y carga de FUDO; cobrar lo creado
- [ ] T027 `/revision-de-codigo`

## Dependencies

Phase 1 primero (defectos). Phase 2 bloquea 3 a 7. US2 antes que US3 (comparten la expansión). La
carga de FUDO necesita la 0078.
