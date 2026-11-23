---
description: "Tareas de la spec 098: caducidad por lote"
---

# Tasks: Caducidad por lote

**Input**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD): cada test va antes de su implementación y se ve en rojo
por la razón correcta. Los bordes del spec van primero.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: puede ir en paralelo (otro archivo, sin dependencia pendiente).
- **[Story]**: US1–US7 de [spec.md](spec.md).

---

## Phase 1: Setup

- [X] T001 Traer `origin/develop` y confirmar que la migración libre es la 0128 (`ls server/migrations`); si ya está tomada, usar la siguiente y renombrar en estos documentos
- [X] T002 Crear `server/migrations/0128_stock_lots.sql` con su cabecera goose y `set local lock_timeout = '3s'`

## Phase 2: Foundational (bloquea todas las historias)

### Tests primero

- [X] T003 [P] Tests table-driven en `server/internal/domain/lots_test.go`: `ParseExpiry` (rango 2000-01-01..hoy+10 años, fecha pasada válida, formato inválido → `ErrValidation`), `ValidLotBreakdown` (cantidad > 0, fecha repetida, suma > contado → `ErrLotsExceedCounted`), `ProjectLots` (sin fecha primero, sobrante de un lote caducado no retrasa al siguiente, caduca hoy, sin consumo, caducado, sin fecha negativo) y `DaysLeft`
- [X] T004 Test de migración en `server/internal/integration/migration_0128_test.go` sobre la base restaurada con dos empresas: tras el Up ninguna `stock_levels.on_hand` cambia y `stock_lots` está vacía (SC-006, FR-005); el Down (con lotes, asignaciones, desglose y un aviso `expired_left` cargados) baja sin error y un Up de nuevo funciona; agregar su renglón en el harness de Downs si hace falta (`harness_restaurado_test.go`)
- [X] T005 Test de aislamiento en `server/internal/integration/stock_lots_isolation_test.go`: `stock_lots`, `stock_lot_allocations` y `stock_count_line_lots` con `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa) bajo `appRoleStore`; confirmar que `TestEveryCompanyTableIsIsolated` las recorre, y que un lote con un almacén de otra sucursal se rechaza por la llave compuesta (FR-062)
- [X] T006 Tests de borde de la regla en `server/internal/integration/stock_lots_test.go` (helpers `lotIn`, `lotsOf`, `undatedOf` que leen la base): (a) lote sin fecha: un positivo sin fecha no crea lote y la suma de lotes + sin fecha = existencia; (b) dos compras con la misma fecha y almacén = un lote; en otro almacén = dos; (c) venta: sin fecha primero, luego por fecha; (d) nunca de un caducado: con un caducado y uno bueno, vender más que lo bueno deja el bueno en 0, el caducado intacto y lo sin fecha negativo; (e) caduca hoy (con `business_settings.timezone`) todavía se descuenta; (f) un `insert` de varias filas del mismo artículo descuenta en orden sin contar dos veces; (g) ningún lote queda negativo (corrección negativa más grande que el lote)

### Implementación

- [X] T007 [P] Implementar `server/internal/domain/lots.go` (`ParseExpiry`, `ValidLotBreakdown`, `ErrLotsExceedCounted`, `DaysLeft`, `ProjectLots`) hasta que T003 pase
- [X] T008 Migración `server/migrations/0128_stock_lots.sql` según [data-model.md](data-model.md): `stock_movements_id_company_key`, tablas `stock_lots`, `stock_lot_allocations`, `stock_count_line_lots` con RLS (`nullif`), grants y llaves compuestas; columnas `stock_movements.expires_on`, `expense_items.expires_on`, `waste_records.lot_expires_on`; checks de `notifications` (`expired_left`, `expired_cleared`); `business_today`; `apply_stock_lots` y `trg_stock_movement_lots` con las reglas de R4 y los atajos y la fecha derivada de R12; `evaluate_expired_left`; Down de R12. Hasta que T004–T006 pasen
- [X] T009 Clasificar `stock_lots` y `stock_count_line_lots` en `app.MergeTables` y `app.UnitChangeTables` (`server/internal/app/`), hasta que `TestEveryTableThatPointsToAnArticleIsClassified` pase
- [X] T010 [P] `server/queries/lots.sql` con las consultas de lectura (lotes por almacén o sucursal, consumo de 30 días, lo pendiente de merma por lote, fechas conocidas por artículo) y `make sqlc`

**Checkpoint**: la regla vive en la base y cubre a la versión anterior; ninguna existencia cambió.

---

## Phase 3: User Story 1 — Recibir con fecha (P1)

**Goal**: la fecha capturada al recibir crea o suma al lote del almacén del renglón.
**Independent Test**: recibir 2 kg con caducidad en Bodega deja ese lote.

### Tests primero

- [X] T011 [US1] Tests en `server/internal/integration/stock_lots_purchases_test.go`: recibir con fecha crea el lote; sin fecha no; corregir lo recibido mueve el lote en la diferencia sin dejarlo negativo; cambiar la fecha de un renglón recibido pasa la cantidad a la fecha nueva (par de reubicación); anular la compra baja el lote; cambiar el almacén del renglón lleva el lote; la compensación «no mover» deshace el mismo lote; un movimiento de compra **sin** `expires_on` pero con `expense_item_id` de un renglón fechado (la versión anterior) va al lote del renglón (R12 A2); fecha fuera de rango → 400; `expiresOn` en un producto → 400

### Implementación

- [X] T012 [US1] `expiresOn` en los renglones de entrega y de gasto (`server/internal/app/purchases*.go`, `expense_edit.go`, `server/queries/purchases.sql`, `expenses.sql`): se valida con `domain.ParseExpiry`, se guarda en `expense_items.expires_on`, los movimientos de compra y la compensación llevan la fecha, y un cambio de fecha con lo aplicado > 0 inserta el par de reubicación
- [X] T013 [US1] Handlers (`server/internal/httpapi/`) aceptan `expiresOn` en los cuerpos de entrega y gasto; tipos en `web/src/api/backoffice.ts`
- [X] T014 [P] [US1] Test de `web/src/components/ExpiryPickerButton.test.tsx`: abre en la hoja de meses de este año, ‹ año › pagina, elige mes y día (3 toques), «No trae fecha» deja vacío, sin `allowPast` los meses pasados están apagados, todas las celdas ≥ 44 px
- [X] T015 [US1] Componente común `web/src/components/ExpiryPickerButton.tsx` (maqueta «Fecha al recibir», opción C; estilo de `DatePickerButton`)
- [X] T016 [US1] Test y cambio en `web/src/features/backoffice/DeliverySheet.tsx` (+ `DeliverySheet.test.tsx`): «Caduca…» compacto solo en renglones con cantidad > 0, la fecha viaja en el cuerpo, una fecha pasada pide confirmar, el botón del encabezado dice «Día de llegada por renglón» (fila «Fecha al recibir» de Maquetas)

---

## Phase 4: User Story 2 — Primero el que caduca antes, nunca caducado (P1)

**Goal**: ventas, mermas normales y cancelaciones respetan los lotes; la campana avisa.
**Independent Test**: 1 kg sin fecha, 2 kg al 10-dic y 2 kg al 20-dic; vender 2 kg deja 0 / 1 / 2.

### Tests primero

- [X] T017 [US2] Tests en `server/internal/integration/stock_lots_sales_test.go` con pedidos reales del servicio: venta que agota lo bueno (aviso `expired_left` abierto, uno solo aunque haya dos ventas, sin correo y fuera del resumen de la mañana); cancelación de un renglón que ya descontó de dos lotes repone a esos lotes aunque uno haya caducado; vender hasta 0 un lote y cancelar repone al lote (R12 A1); cancelar el pedido completo igual; merma normal descuenta como venta; deshacer una merma repone al lote; el aviso se cierra al llegar existencia buena que cubre el negativo y al mandar el caducado a merma
- [X] T018 [P] [US2] Test de `OpenNotificationsForMorning` (o del mailer) que no cuenta `expired_left` (R12 M3)

### Implementación

- [X] T019 [US2] Ajustes que pidan T017–T018 en la migración 0128, `server/queries/notifications.sql` (resumen de la mañana) y `domain/notifications.go` (kind `expired_left`)
- [X] T020 [US2] Campana (`web/src/features/notificaciones/Campana.tsx` + test): «Se está usando [artículo] y lo que queda está caducado» con «Ver caducidades»

---

## Phase 5: User Story 3 — Caducidades (P1)

**Goal**: ver lo caducado y lo que caduca, con la proyección por lote.
**Independent Test**: un lote de 3 kg a 20 días con 100 g/día dice «no se termina: quedan ~0.9 kg».

### Tests primero

- [X] T021 [US3] Tests en `server/internal/integration/expiries_test.go`: `GET /stock/expiries` con lotes de dos almacenes, filtro por almacén, consumo de 30 días (ventas netas de cancelaciones; un traspaso no cuenta), proyección con lo sin fecha y lotes de la sucursal, `pendingWaste`, `summary`; permiso `inventory.view` (403 sin él); `warehouseId` vacío 400 y ajeno 404; aislamiento con `inTheThreeCases`

### Implementación

- [X] T022 [US3] Servicio `server/internal/app/expiries.go` (usa `domain.ProjectLots`) y handlers `server/internal/httpapi/handlers_expiries.go` con rutas en `router.go`
- [X] T023 [US3] Test y pantalla `web/src/features/backoffice/expiries/ExpiriesTab.tsx` (+ `ExpiriesTab.test.tsx`): lista por plazo con «Ya caducó», «Esta semana», «8 a 30 días», «31 a 60 días» y «Más adelante» cerrado, frase de proyección, vacío/carga/error, selector de almacén (fila «Caducidades» de Maquetas)
- [X] T024 [US3] Pestaña «Caducidades» después de «Existencias» y «Traspaso» en `web/src/features/backoffice/StockPage.tsx`, solo con `inventory.view` (+ test)

---

## Phase 6: User Story 4 — Mandar a merma desde la vista (P2)

### Tests primero

- [X] T025 [US4] Tests en `server/internal/integration/stock_lots_waste_test.go`: merma con `lotExpiresOn` total y parcial descuenta de ese lote; con aprobación, el lote baja al aprobar (también si aprueba un movimiento sin fecha: fecha derivada del registro, R12 A2); `lotExpiresOn` en un producto → 400; sin `inventory.write_off` → 403

### Implementación

- [X] T026 [US4] `lotExpiresOn` en `server/internal/app/waste.go`, `server/queries/waste.sql` (registro, movimiento, aprobación y deshacer) y su handler
- [X] T027 [US4] Test y hoja `web/src/features/backoffice/expiries/WasteLotSheet.tsx` (+ test): todo el lote por omisión, motivo «Caducado» por omisión en lo caducado y obligatorio en lo vigente, decimales según la unidad, «Merma pendiente de aprobación» en el renglón

---

## Phase 7: User Story 5 — Contar con fechas (P2)

### Tests primero

- [X] T028 [US5] Tests en `server/internal/integration/stock_lots_count_test.go`: capturar con desglose y cerrar deja los lotes contados (incluido uno caducado) y lo demás sin fecha; los no mencionados quedan en 0; sin desglose no se reescriben y la diferencia va por el orden de consumo; una venta entre captura y cierre se resta del lote contado y una cancelación posterior suma (neto); un artículo con desglose y diferencia 0 también fija sus lotes; suma > contado → 400 `LOTS_EXCEED_COUNTED`; `lots: []` borra el desglose; la lectura del conteo trae `lots` y `knownDates`

### Implementación

- [X] T029 [US5] Captura con `lots` y cierre que fija lotes (`server/internal/app/stock_counts.go`, `server/queries/stock_counts.sql`, `lots.sql`), con `evaluate_expired_left` al final; handler y lectura
- [X] T030 [US5] Test y cambio en `web/src/features/backoffice/StockCountTab.tsx` (+ test): fechas como chips en la captura del artículo, fechas conocidas sin cantidad, «+ Agregar fecha» con `ExpiryPickerButton allowPast`, teclado en el campo marcado, «Sin fecha: X», error del artículo sin bloquear el cierre (fila «Fechas en el conteo» de Maquetas)

---

## Phase 8: User Story 6 — Traspasos con fecha (P2)

- [X] T031 [US6] Tests en `server/internal/integration/stock_lots_transfers_test.go`: traspaso parcial de un lote lo parte con la misma fecha; lo sin fecha llega sin fecha; los caducados no se mueven; traspasar más de lo bueno (confirmado) no mueve caducados
- [X] T032 [US6] Ajustes que pida T031 en la migración (la pata de entrada lee las asignaciones de la de salida)

---

## Phase 9: User Story 7 — Aviso en Existencias (P3)

- [X] T033 [US7] Test y aviso en `web/src/features/backoffice/StockPage.tsx` (o su pestaña de Existencias): «N caducado · M caducan en 7 días · Ver caducidades ›» con `GET /stock/expiries/summary`, solo si hay algo (fila «Aviso en Existencias» de Maquetas)

---

## Phase 10: Unión y cambio de unidad

- [X] T034 Tests en `server/internal/integration/stock_lots_merge_test.go`: unir pasa los lotes al destino con el factor por almacén y fecha (se suman si coinciden) y deja los del origen en 0; cambiar la unidad directo escala lotes, desglose y asignaciones
- [X] T035 Mover lotes en la unión y el reemplazo (`server/internal/app/` de unión y cambio de unidad, `server/queries/ingredient_merge.sql`), con candados de `stock_levels` en orden de id

---

## Phase 11: Polish

- [ ] T036 Medir SC-005 (con lotes ≤ 2× sin lotes): un pedido de 90 movimientos sin lotes y con lotes en el Postgres de pruebas; anotar las cifras en `docs/mecanica/almacen.md`
- [ ] T037 [P] `docs/mecanica/almacen.md`: sección de lotes (lo que no se ve: regla en el trigger, sin fecha primero, fecha derivada del origen, lo sin fecha negativo solo lo corrige un conteo, límite del traslape)
- [ ] T038 `/revision-de-codigo` y su corrección con test
- [ ] T039 Prueba con alguien nuevo de una fonda a 1024×600 (y 1280×854) contra el `make start` con `make db-restaurar`, y `bun run e2e` de lo tocado
- [ ] T040 `make ci-local` verde

## Dependencies

- Fase 2 bloquea todo. US1 y US2 primero (P1); US3 depende de los lotes de US1 para probarse en pantalla. US4 usa la vista de US3. US5, US6 y la Fase 10 son independientes entre sí después de la Fase 2. US7 depende del `summary` de US3.

## Parallel

- T003/T007 (dominio) junto a T008 (migración). T014–T015 (componente) junto a T011–T012 (servidor). T018 junto a T017.

## Implementation Strategy

Todo sube en una sola entrega: la migración es aditiva y la regla en la base cubre a la versión
anterior. MVP interno para probar: Fase 2 + US1 + US2 + US3.
