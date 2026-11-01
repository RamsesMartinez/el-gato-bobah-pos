---
description: "Tareas de la spec 053: editar un gasto ya guardado (entrega 1)"
---

# Tasks: Editar un gasto ya guardado (entrega 1)

**Input**: `specs/053-editar-gasto/` — plan.md, spec.md, research.md, data-model.md,
contracts/edit-expense.md, quickstart.md

**Tests**: obligatorios (constitución IV, TDD). Cada tarea de prueba va ANTES de su implementación y
se ve fallar por la razón correcta.

**Organization**: por historia de la spec. La vista previa (US5) es transversal y vive en la fase
fundacional: todas las historias la usan.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: se puede hacer en paralelo (otro archivo, sin dependencia pendiente)
- **[Story]**: US1…US5 de spec.md

---

## Phase 1: Setup

- [X] T001 Confirmar que la migración libre es la 0103 (`ls server/migrations`, ramas remotas de la 052) y que `make sqlc` corre en el worktree; si la 0103 ya llegó a develop con otro contenido, no hay que renumerar nada aquí

---

## Phase 2: Foundational (bloquea todas las historias)

**Purpose**: esquema, reglas puras, permiso, bitácora y el motor de edición con vista previa.

### Pruebas primero

- [X] T002 [P] Test de la migración 0103 en `server/internal/integration/migration_0103_test.go`: dos empresas con zonas distintas; el relleno de `effective_on` usa la zona de cada una; un insert `compra` sin la columna (API vieja) la llena con hoy en la zona de la empresa; el Down se niega si una fecha capturada difiere del día de `created_at`, y baja limpio si no (data-model.md, revisión M3/M4)
- [X] T003 [P] Tests table-driven en `server/internal/domain/expense_edit_test.go`: `ExpenseAmountAfterEdit` (caso 1-nov, sustitución, borrado simple, sin desglosar, de más, resultado ≤ 0 rechazado), `PaymentStatusFor` y la regla «solo si cambió el importe y nunca degrada una pagada sin pagos» (M5), `ShortfallDefault` (pieza/empaque → pending; masa/volumen/longitud → adjust), `AdjustLineToReceived`, `ValidDeliveryDate` (futura y anterior al gasto rechazadas), `ParseCountDecision`/`ParseMoneyDecision`/`ParseShortfall` (desconocido → ErrValidation), `PlanDeliveryReversal` (netea negativos contra los positivos más recientes; nunca devuelve positivos; suma = −aplicado; M2), `EditNeedsReceive`, `PreviewFingerprint` (estable ante orden de mapas, cambia con una cifra o una decisión), `CountPosteriorCutoff` (least(R, fin del día D en la zona))
- [X] T004 [P] Test en `server/internal/domain/expense_test.go`: `CanCancelExpense(status, hasPayments)` — pendiente sin pagos sí; pendiente con pagos no; pagada no (revisión A1)
- [X] T005 [P] Test en `server/internal/domain/permission_test.go`: `expenses.edit` existe, lo tiene admin, NO gerente ni cajero, y tiene su mensaje de permiso denegado

### Implementación

- [X] T006 Migración `server/migrations/0103_stock_movement_effective_on.sql`: columna `effective_on date`, función y trigger `before insert` `fill_stock_movement_effective_on` (compra o con `expense_id`, zona de `business_settings` por `new.company_id`, `America/Mexico_City` por omisión), relleno de los `compra` existentes, Down que se niega con fechas capturadas (data-model.md)
- [X] T007 [P] Reglas puras en `server/internal/domain/expense_edit.go` (todo lo de T003) y centinelas nuevos `ErrDecisionRequired`, `ErrExpenseEditStale` en `server/internal/domain/errors.go` (o donde vivan los centinelas)
- [X] T008 [P] `CanCancelExpense(status, hasPayments)` en `server/internal/domain/expense.go`; ajustar su llamador en `server/internal/app/expenses.go` y la consulta `CancelExpense` en `server/queries/expenses.sql` (`and not exists (select 1 from expense_payments …)`)
- [X] T009 [P] Permiso `PermExpensesEdit = "expenses.edit"` en `server/internal/domain/permission.go` (en `AllPermissions`, no en gerente; mensaje «Tu usuario no puede editar gastos») y su espejo en `web/src/app/permissions.ts`
- [X] T010 [P] Acción de bitácora `AuditExpenseEdited = "expense.edited"` en `server/internal/domain/audit.go` (lista blanca, grupo caja) y su etiqueta «Gasto editado» en `web/src/shared/audit/labels.ts`
- [X] T011 Consultas en `server/queries/purchases.sql` + `make sqlc`: `InsertLineMovement` con `sqlc.narg(effective_on)`; `InsertCompensationMovement` (ajuste, `stock_count_id`, `expense_id`, `branch_id` explícito, `effective_on`); `LatestPosteriorCount` (insumo, corte, `branch_id = current_branch_id()`, status closed → count_id, branch_id, día capturado); `LineDeliveryDays` (por renglón: `effective_on`, suma, max `created_at`); `ExpenseDeliveryHistory` (por gasto: renglón, día, suma en base); `MaxDeliveryDate`; `StockOnHandFor` (insumos y productos, sucursal de la sesión); `IngredientCosts`; `UpdateExpenseHeader`; `SetExpenseAmountStatus`; `CountExpensePayments`
- [X] T012 `applyStockLines` en `server/internal/app/purchases.go` acepta la fecha por renglón (`map[itemID]time.Time`, la de `day` por omisión), la graba en `effective_on`, devuelve los movimientos que insertó (insumo, cantidad, fecha, instante) y fija `received_at` con `MaxDeliveryDate`; las rutas viejas (`purchases_receive.go`, `SaveStockLines`, `SetItemExcluded`, `ReceiveExpense`) siguen pasando hoy
- [X] T013 Motor `server/internal/app/expense_edit.go`: `runExpenseEdit(ctx, id, userID, preview, fn)` toma el gasto (`LockExpenseForStock`, 409 si anulado), lee el antes (encabezado, importe, pagado, estado, existencias y costo de los artículos tocados), corre `fn`, aplica las decisiones de conteo (`LatestPosteriorCount`; sin decisión → la marca como pendiente; `keep` → `InsertCompensationMovement` con la sucursal del conteo), recalcula importe y estado (`ExpenseAmountAfterEdit`, `PaymentStatusFor`), valida `excludedStillCovers`, lee el después, arma `EditPreview` con `fingerprint`, escribe la bitácora y, si es vista previa, revierte con un centinela interno; al confirmar compara la huella (`ErrExpenseEditStale`) y exige las decisiones (`ErrDecisionRequired`)
- [X] T014 Mapeo en `server/internal/httpapi/respond.go`: `ErrDecisionRequired` → 400 `DECISION_REQUIRED`, `ErrExpenseEditStale` → 409 `EXPENSE_EDIT_STALE` (antes de `ErrValidation`/`ErrConflict`)
- [X] T015 Test de integración del motor en `server/internal/integration/expense_edit_test.go`: una vista previa no deja ningún movimiento, evento de bitácora, cambio de importe ni de costo; la confirmación con la misma huella escribe exactamente lo previsualizado (SC-002); con una venta entre las dos (o otra edición) responde `EXPENSE_EDIT_STALE`; aislamiento bajo `appRoleStore` con `inTheThreeCases`
- [X] T016 [P] Tipos y llamadas en `web/src/api/backoffice.ts`: `EditPreview`, `editExpense(id, body, preview)`, `deliverExpense(id, body, preview)`, historial de entregas en `ExpenseItem`
- [X] T017 [P] Test de `web/src/features/backoffice/EditPreviewSheet.test.tsx` (US5): tabla Qué/Antes/Después/Cambio con signo; filas de decisión primero en su grupo; «Confirmar» deshabilitado hasta decidir; el pie dice «Falta decidir N» y al tocarlo enfoca la siguiente; versión compacta sin avisos; un 409 `EXPENSE_EDIT_STALE` pide otra vista previa y la muestra
- [X] T018 `web/src/features/backoffice/EditPreviewSheet.tsx` (Maquetas: «Vista previa», opción B): `DrawerRoot` inferior `maxH="80dvh"`, encabezado de columnas fijo, decisiones en una línea con botones de 44 px, ⚠ y texto además del color, tokens del tema

**Checkpoint**: la vista previa existe de punta a punta sin ninguna acción de historia.

---

## Phase 3: User Story 1 - Agregar la mercancía a un gasto guardado sin ella (P1) 🎯 MVP

**Goal**: el caso del 1-nov se corrige desde la tableta.

**Independent Test**: quickstart escenario 1 contra el respaldo restaurado.

- [X] T019 [P] [US1] Tests de integración en `server/internal/integration/expense_edit_test.go`: gasto de $67.50 pagado sin renglones + dos renglones que suman $67.50 → importe y pago intactos, sin movimientos; luego recibir → existencias suben, gasto recibido; agregar con `arrivedOn` recibe en la misma confirmación con esa fecha; agregar $80 a un pagado de $67.50 → pendiente con saldo $12.50, decisión `money` requerida, pago original igual; `arrivedOn` sin `inventory.receive` → 403; renglón sin artículo no mueve inventario; regalo no cambia importe ni costo; bitácora `expense.edited` con antes/después
- [X] T020 [US1] Agregar renglones en `server/internal/app/expense_edit.go` (`EditExpense`: validación de cada renglón como `saveStockLine` nuevo, `CreateExpenseItem`, «Ya llegó» con su fecha por `applyStockLines`)
- [X] T021 [US1] Handler `POST /expenses/{id}/edit[?preview=1]` en `server/internal/httpapi/handlers_expense_edit.go` (decodifica, `preview` solo `1` o ausente, revalida `inventory.receive` si `domain.EditNeedsReceive`) y ruta con `RequirePermission(expenses.edit)` en `server/internal/httpapi/router.go`, fuera del grupo por rol
- [X] T022 [P] [US1] Test de `web/src/features/backoffice/ExpenseDetailDialog.test.tsx`: un gasto sin renglones muestra «Editar gasto»; en modo edición «+ Agregar renglón» agrega en verde con «Deshacer»; «Revisar cambios» pide la vista previa y «Confirmar» guarda; sin `expenses.edit` no aparece «Editar gasto»
- [X] T023 [US1] `web/src/features/backoffice/AddLineSheet.tsx`: el formulario de `ArrivedItemSheet` (artículo con `Picker`, empaque o unidad, «Lo pagué / Fue regalo», importe) más «Ya llegó» con `DatePickerButton`, y opción de renglón sin artículo; la cantidad y el importe con `NumericKeypad` (`inputMode="none"`)
- [X] T024 [US1] Modo edición en `web/src/features/backoffice/ExpenseDetailDialog.tsx` (Maquetas: «Detalle del gasto», opción A): «Editar gasto» también sin renglones, renglones nuevos en verde, pie «N cambio(s) sin confirmar · Cancelar · Revisar cambios», abre `EditPreviewSheet`; reemplaza «Agregar un artículo que faltó». En el estado *Ver*: «Falta pagar $X» o «Pagado de más $X» junto a Importe y Pagado (FR-012, FR-013)
- [X] T024b [US1] «Registrar pago» de la vista previa (FR-012): al confirmar con `money = pay`, abre el `PayDialog` exportado de `web/src/features/backoffice/ExpensesPage.tsx` con el importe precargado en el saldo (prop nueva `defaultAmount`); su caso en `ExpenseDetailDialog.test.tsx`

**Checkpoint**: criterio 1 de la spec en verde.

---

## Phase 4: User Story 2 - Recibir en varias entregas con su fecha (P1)

**Independent Test**: quickstart escenario 4.

- [X] T025 [P] [US2] Tests de integración en `server/internal/integration/expense_edit_test.go`: 3 de 5 con fecha de ayer y `pending` → faltan 2, gasto por recibir, `effective_on` = ayer; 2 más hoy → completo, historial de dos días, `received_at` = hoy; 8.5 de 10 kg con `adjust` → renglón 8.5 kg con importe proporcional y el del gasto baja; llegó de más sin `extraAmount` → 400; fecha futura o anterior al gasto → 400; bitácora `expense.received` con la fecha de cada renglón y la decisión (FR-030); entrega con fecha anterior a un conteo cerrado de esa sucursal → decisión `count:<insumo>` requerida; `keep` deja existencias iguales con compra + ajuste ligado al conteo y `branch_id` del conteo; un conteo de OTRA sucursal no pregunta (M1); un conteo abierto no pregunta; recibir hoy nunca pregunta; `inventory.receive` sin `expenses.edit` puede recibir
- [X] T026 [US2] `DeliverExpense` en `server/internal/app/expense_edit.go`: valida fecha por renglón, sube `qty_received`, `shortfall` (`AdjustLineToReceived` + importe), regla de lo de más (FR-021), aplica con `applyStockLines` por fecha
- [X] T027 [US2] Handler `POST /expenses/{id}/deliveries[?preview=1]` en `server/internal/httpapi/handlers_expense_edit.go` y ruta con `RequirePermission(inventory.receive)` en `server/internal/httpapi/router.go`
- [X] T028 [US2] Historial de entregas por renglón en el detalle: `ExpenseDetail` en `server/internal/app/expenses.go` agrega `deliveries: [{on, qtyBase}]` a cada renglón (siempre arreglo) con `ExpenseDeliveryHistory`; test del JSON crudo en `server/internal/integration/expense_edit_test.go`
- [X] T029 [P] [US2] Test de `web/src/features/backoffice/DeliverySheet.test.tsx`: renglones precargados con lo que falta; «No llegó» deja 0 y cambia a «Llegó todo»; tocar la cantidad muestra `NumericKeypad` en lugar del pie y «Listo» lo regresa; «⚠ Faltarán N» en línea; con todo en 0 «Ver efectos» deshabilitado y «Nada llegó todavía»; «Fechas por renglón» muestra una fecha por renglón; manda `shortfall` elegido en la vista previa
- [X] T030 [US2] `web/src/features/backoffice/DeliverySheet.tsx` (Maquetas: «Hoja de entrega», opción B + «No llegó»): `DatePickerButton` arriba, cuerpo `flex="1" minH={0}`, −/+ con casilla `inputMode="none"` y `NumericKeypad`, abre `EditPreviewSheet` con las decisiones de `shortfall`
- [X] T031 [US2] Detalle: «Recibir» abre `DeliverySheet`, cada renglón muestra pedido / recibido / falta y «Entregas (N)» con su historial en `web/src/features/backoffice/ExpenseDetailDialog.tsx`
- [X] T032 [P] [US2] Atajo «Recibir» en la lista: botón en cada gasto con algo por recibir (con `inventory.receive`) en `web/src/features/backoffice/ExpensesPage.tsx`, y «Recibir» de «Por recibir» abre `DeliverySheet` del gasto en `web/src/features/backoffice/PendingReceiptsTab.tsx`; tests en sus `.test.tsx`

**Checkpoint**: criterio 4 en verde.

---

## Phase 5: User Story 3 - Borrar un renglón o sustituir un producto (P2)

**Independent Test**: quickstart escenarios 2 y 3.

- [X] T033 [P] [US3] Tests de integración en `server/internal/integration/expense_edit_test.go`: borrar sin recibir → cero movimientos, importe baja; borrar recibido el 10-oct sin conteos → contra-movimiento con `effective_on` 10-oct, existencias bajan, suma de los movimientos del renglón = 0 (SC-006); recibido 50 el día D, corregido a 30 hoy por `/stock-lines`, borrado → solo −30 fechado D y nunca una compra positiva (M2); con conteo posterior → decisión requerida, `keep` y `adjust` como en la spec; sustitución (borrar y agregar el mismo importe en una edición) → importe igual; borrar dejando negativo → `negativeAfter` y se confirma; un excluido no se borra (400); borrar todo de un gasto sin «sin desglosar» → 400; un gasto pagado que una edición dejó pendiente no se anula (409, A1); bitácora `expense.edited` con los renglones quitados y la decisión de conteo (FR-030); un gasto `pagada` sin ningún pago al que se le borra un renglón sigue `pagada` (FR-011)
- [X] T034 [US3] Quitar renglones en `EditExpense` (`server/internal/app/expense_edit.go`): `LineDeliveryDays` → `PlanDeliveryReversal` → contra-movimientos fechados por día, decisiones de conteo por insumo con el instante del original, `VoidStockLine`
- [X] T035 [US3] «✕ Quitar» / «Deshacer» por renglón no excluido en modo edición en `web/src/features/backoffice/ExpenseDetailDialog.tsx` (Maquetas: «Detalle del gasto», opción A) y su caso en `ExpenseDetailDialog.test.tsx`

**Checkpoint**: criterios 2 y 3 en verde.

---

## Phase 6: User Story 4 - Corregir el encabezado (P2)

**Independent Test**: quickstart escenario 7.

- [X] T036 [P] [US4] Tests de integración en `server/internal/integration/expense_edit_test.go`: proveedor (y quitarlo), categoría, comentario y fecha cambian con bitácora antes/después; cambiar la fecha no mueve fechas de entrega ni de pagos; fecha futura → 400; categoría de otra empresa → 404/400; una edición solo de encabezado no toca el estado de pago (FR-011)
- [X] T037 [US4] Encabezado en `EditExpense` (`server/internal/app/expense_edit.go`) con `UpdateExpenseHeader`, y su sección en la vista previa
- [X] T038 [US4] Campos en línea del modo edición (proveedor y categoría con `Picker`, fecha con `DatePickerButton` en una fila de tres; comentario debajo) en `web/src/features/backoffice/ExpenseDetailDialog.tsx` (Maquetas: «Detalle del gasto», opción A) y su caso en el test

**Checkpoint**: criterio 7 en verde.

---

## Phase 7: Polish & Cross-Cutting

- [X] T039 [P] Medición de uso: las acciones `editar-gasto` y `recibir-entrega` de la pantalla `gastos` en la lista blanca de `server/internal/domain/uso.go` (con su test), y la llamada `medirAccion('gastos', …)` al confirmar en `ExpenseDetailDialog.tsx` y `DeliverySheet.tsx`
- [X] T040 [P] `TestEveryAuditActionIsProduced` cubre `expense.edited` (la prueba de T019 la produce); correr `go test ./internal/domain/...`
- [X] T041 [P] e2e `web/e2e/editar-gasto.spec.ts` a 1024×600 contra el `make start` del worktree: criterios 1, 2, 4 y 5, y una vista previa con 2 decisiones donde «Confirmar» y «Falta decidir» se ven sin desplazar (U2); capturas a `test-results/`
- [X] T042 [P] `AGENTS.md` §2: párrafo de la spec 053 con lo que no se ve (`effective_on` y su trigger, «No mover» como ajuste ligado al conteo con la sucursal del conteo, la vista previa como transacción revertida con huella, anular exige cero pagos)
- [ ] T043 Prueba con usuario nuevo a 1024×600 (subagente sin contexto) sobre la base restaurada: quickstart escenarios 1 y 4; lo observado en texto
- [X] T044 Aplicar la 0103 sobre la base restaurada del 1-nov (`make start` del worktree, dos empresas): `select count(*) from stock_movements where movement_type = 'compra' and effective_on is null` = 0 y el día rellenado coincide con el de `created_at` en la zona de cada empresa (constitución IV)
- [ ] T044b Gates: `cd server && go build ./... && go test ./...`, `make test-integration`, `cd web && bun run lint && bun run vitest run && bun run build`, `make ci-local`
- [X] T045 Issue nuevo para la entrega 2 del #167 (#175) (devoluciones, anular con pagos y mercancía, mover fechas juntas, «fechas por separado») con liga a esta spec

---

## Dependencies & Execution Order

- Phase 1 → Phase 2 (bloquea todo) → US1 → US2 → US3 → US4 → Polish. US3 y US4 solo dependen de
  Phase 2 y podrían ir en paralelo con US2 en otro archivo de pruebas; en la práctica comparten
  `expense_edit.go` y `ExpenseDetailDialog.tsx`, así que van en orden.
- Dentro de cada fase: prueba (ver fallar) → implementación → la prueba pasa.

## Parallel Example: Phase 2

```text
T002 test migración · T003 tests domain · T004 test cancelar · T005 test permiso  (en paralelo)
T007 domain · T008 cancelar · T009 permiso · T010 bitácora                         (en paralelo)
T016 tipos web · T017 test vista previa                                           (en paralelo con el backend)
```

## Implementation Strategy

MVP = Phase 2 + US1: resuelve el caso del 1-nov. US2 cierra el criterio de entregas parciales; US3
y US4 completan la entrega 1. Se empuja todo junto al terminar (un solo PR), con la entrega 2 como
issue aparte.
