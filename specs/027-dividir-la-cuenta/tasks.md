---

description: "Tareas de la feature 027 — dividir la cuenta por productos"
---

# Tasks: Dividir la cuenta por productos

**Input**: documentos de diseño en `specs/027-dividir-la-cuenta/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md), [quickstart.md](./quickstart.md)

**Tests**: obligatorios y **primero**. El principio IV no es negociable: cada tarea de código va
precedida por la de su test, que se ve **en rojo por la razón correcta** antes de escribir el código.
La migración además la exige el hook `migracion-con-test.sh`. Ningún test se marca `skip`. La única
excepción de orden es T032, registrada en *Complexity Tracking* del plan.

**Dónde se trabaja**: worktree `/home/ramy/git/egb-027-dividir-cuenta`, rama `027-dividir-la-cuenta`.
El directorio principal del repo es de otra sesión y no se toca. Los tests de integración corren contra
un Postgres propio (`egb027-pg`, puerto 5499, ver [quickstart.md](./quickstart.md)), nunca contra
`deploy-postgres-1`.

**Rutas**: `server/…` y `web/…` son relativas al worktree. `IT` = test de integración con build tag
`integration` en `server/internal/integration/`, bajo `appRoleStore` + `AcquireTenant`. `IT HTTP` =
test de integración contra el router real, con el patrón de `ventas_http_test.go` (no hay tests de
router ni de handlers de pedidos en `httpapi`). `inTheThreeCases` = otra empresa, conexión reciclada
y sin empresa (constitución IV). `[P]` = no comparte archivo con otra tarea `[P]` de la misma fase.

**La rama de Uber (021) manda el orden** (D-11): `inTheThreeCases`, `TestEveryCompanyTableIsIsolated`
y las políticas con `nullif` llegan con ella. Lo que no toca tablas nuevas (Fases 2–4) se construye
ya; las consultas existentes que esas fases modifican quedan cubiertas por T032. Lo demás (Fase 5 en
adelante) espera a T002.

---

## Phase 1: Setup

- [x] T001 Confirmar el punto de partida en verde: `cd server && go build ./... && go test ./...`, integración contra `egb027-pg`, y `cd web && bun run lint && bun run vitest run` en el worktree
- [x] T002 Esperar a que 021 esté en `develop` y rebasar 027 encima:
  - confirmar que existen `server/internal/integration/three_cases_test.go` y `server/internal/integration/rls_all_tables_test.go`; ya no se copia nada;
  - el conflicto en `.specify/memory/constitution.md` se resuelve **reaplicando la enmienda de roles** sobre la 1.13.0 de esa rama, que queda en 1.14.0; verificar que la puerta *Roles y permisos configurables por empresa* está en la tabla y que la versión y la fecha de enmienda son las de la enmienda;
  - **bloquea solo la Fase 5 en adelante** (ver Dependencies)

---

## Phase 2: Foundational sin tablas nuevas — permisos, dinero puro y textos

**Permisos (D-16)**

- [x] T003 [P] Test table-driven de `domain.PermissionsFor(role)` en server/internal/domain/permission_test.go:
  - `payments.void` y `orders.cancel` solo admin y gerente;
  - `orders.move_lines` y `orders.cancel_pending` para los cuatro roles;
  - rol desconocido → ninguno;
  - todo permiso tiene su texto «Tu usuario no puede …» en `domain.PermissionDeniedMessage`, y ninguno nombra un rol
- [x] T004 Implementar `domain.Permission`, sus cuatro constantes, `PermissionsFor` y `PermissionDeniedMessage` en server/internal/domain/permission.go, con comentario del porqué: el mapa es lo único que pasará a la base con los roles por empresa
- [x] T005 Tests:
  - `RequirePermission` en server/internal/httpapi/middleware_test.go: 403 sin permiso con el texto de `PermissionDeniedMessage`, `SecurityEvent "forbidden"`, pasa con permiso; con un resolutor de permisos inyectado que no da ninguno (así se prueba un permiso que hoy tienen todos los roles);
  - IT HTTP en server/internal/integration/permissions_http_test.go, sobre JSON crudo: login, refresh, `/auth/pin-switch` y `/auth/me` devuelven `permissions` como arreglo, nunca `null`
- [x] T006 Implementar `RequirePermission(p)` en server/internal/httpapi/middleware.go: responde con `PermissionDeniedMessage` y resuelve los permisos con una función inyectable en el router (hoy `domain.PermissionsFor`), que es la que T005 y las pruebas de 403 sustituyen. Agregar `permissions` en `writeSession` (server/internal/httpapi/handlers.go:244), que comparten login, refresh y pin-switch, y en `Me` (handlers.go:485)
- [x] T007 [P] Test vitest de `can(permission)` en web/src/app/permissions.test.ts (sin sesión → false; con `permissions` ausente → false)
- [x] T008 Implementar `permissions?: string[]` en `SessionUser` (web/src/stores/session.ts) y `can()` en web/src/app/permissions.ts

**Dinero puro (D-3, D-4, D-7)**

- [x] T009 [P] Tabla de `domain.SelectionAmount` en server/internal/domain/split_bill_test.go:
  - una pieza;
  - piezas de un renglón de 2;
  - descuento repartido;
  - **[A1]** en **toda** selección, no solo la final, Σ montos por renglón = monto del pago exacto, con el residuo de redondeo en el último renglón (p. ej. tres renglones de $33.33 con descuento);
  - la selección que cubre todo lo pendiente cobra exactamente lo que falta (envío y residuo);
  - **[H6]** cubre todo lo pendiente tras un pago por monto: cobra menos que el bruto descontado, y el monto por renglón se escala en proporción al bruto descontado de cada uno, con el residuo en el último renglón;
  - **[H7]** `allRemaining` con cero piezas sin cubrir y saldo positivo: cobra lo que falta sin cobertura (no es selección vacía);
  - pago por monto previo que hace exceder → error con el monto;
  - selección vacía o piezas de más → `ErrValidation`;
  - pieza ya cubierta → `ErrPieceAlreadyPaid`.
  Cada caso dice qué peso se duplicaría
- [x] T010 Tabla de `domain.SplitParts` y `domain.SplitPartAmount` en server/internal/domain/split_bill_test.go: reparte lo que falta, la última absorbe el centavo, `of` entre 2 y 12, parte ya cobrada → `ErrSplitPartAlreadyCharged`
- [x] T011 [P] Tabla de `domain.SplitLine` y `domain.MovablePieces` en server/internal/domain/renglon_test.go:
  - las dos mitades de cada movimiento suman exacto a 4 decimales;
  - `k ≤ n − cubiertas`;
  - renglón mixto con más piezas que las pendientes → `ErrMixedDeliveredPieces` con el texto de contracts/api.md;
  - pendientes primero
**Textos sin prefijo (D-10)**

- [x] T012 [P] Test en server/internal/httpapi/respond_test.go: ninguna respuesta 400, 409 ni **422 que envuelva `ErrValidation`** (`ErrCobroFueraDeLugar`) lleva el prefijo del sentinel («conflicto:», «datos inválidos:»), también con sentinels envueltos dos veces (`ErrCobroExcede`); y el texto nuevo de `ErrCancelarConEntregas` no sugiere un reembolso
- [x] T013 Quitar el prefijo del sentinel del mensaje para quien opera en `httpapi.Error` (server/internal/httpapi/respond.go, `Error`: `msg := err.Error()` en la línea 44, sobre sentinels envueltos con `%w: …`), para **todas** las respuestas, y cambiar el texto de `ErrCancelarConEntregas` en server/internal/domain/entrega.go. Antes de cambiarlo, medir el radio corriendo los tests existentes (go y vitest) y listar los que esperan el prefijo —p. ej. web/src/shared/CobrarSheet.test.tsx:190 espera `'conflicto: …'`—; actualizarlos, y revisar que web/src/api/mensajes.ts no clasifique por el prefijo
- [x] T014 Test del mapeo de los sentinels nuevos en server/internal/httpapi/respond_test.go: cada uno de la tabla *Sentinels nuevos* de contracts/api.md sale con su status, su código y su texto
- [x] T015 Implementar `SelectionAmount`, `SplitParts` y `SplitPartAmount` en server/internal/domain/split_bill.go, y `SplitLine`/`MovablePieces` en server/internal/domain/renglon.go; los sentinels nuevos (nombres en inglés, contracts/api.md) en server/internal/domain/errors.go, mapeados solo en server/internal/httpapi/respond.go

**Checkpoint**: permisos, aritmética y textos sin prefijo listos; el mapeo de los sentinels nuevos
queda verde en esta fase, sin haber tocado el esquema.

---

## Phase 3: User Story 7 — Ningún pedido se queda sin salida (P1)

**Goal**: ninguna tarjeta ni pedido queda sin una acción que lo cierre. Va primero porque es lo que
convirtió el problema en incidente y no depende del esquema nuevo.

**Independent Test**: un pedido con todo lo vivo entregado y abierto (forzado en la base de prueba)
muestra «Cerrar pedido» y se cierra al tocarlo; un pedido sin productos vivos y sin pagos también.

- [x] T016 [P] [US7] IT `TestDeliverAllRejectsAnOrderWithoutProducts` en server/internal/integration/no_way_out_test.go (hoy cierra un pedido sin productos como venta de $0)
- [x] T017 [US7] Rechazar en `DeliverAll` (server/internal/app/orders.go) un pedido sin renglones vivos, con la regla en server/internal/domain/entrega.go y el texto de contracts/api.md
- [x] T018 [US7] IT `TestCancelPendingClosesWithWhatWasDelivered` en server/internal/integration/no_way_out_test.go:
  - renglón con entrega parcial (3 piezas, 1 entregada): se parte; las 2 pendientes se quitan en un renglón nuevo que copia precio, costo, modificadores y estado de cocina, y la entregada se queda;
  - **[M1]** los movimientos del renglón partido son pares `venta` con `reason = 'renglón partido'` que se anulan: sin cambio neto en existencias antes de reponer lo que toque;
  - «Ya no falta nada por entregar» y la respuesta `{ removed, restocked }`
- [x] T019 [US7] **[U2]** `TestAnOrderWithoutProductsCanBeClosedByAnyRole` en server/internal/integration/no_way_out_test.go, como IT de servicio **e IT HTTP** (patrón de `ventas_http_test.go`):
  - IT HTTP: `POST /orders/{id}/lines/cancel-pending` **sin cuerpo** sobre un pedido sin productos vivos y sin pagos responde 200 con cada uno de los cuatro roles (cajero y mesero incluidos), sin pasar por un resolutor inyectado;
  - el pedido queda `cancelada` con el motivo «Sin productos», sin movimientos de inventario, y **sí** cuenta como cancelación en `SalesTotalsByStatus`;
  - sin productos vivos y con pagos: rechazo «Tiene pagos: hay que devolverlos primero»
- [x] T020 [US7] IT HTTP en server/internal/integration/no_way_out_test.go:
  - `POST /orders/{id}/lines/cancel-pending` → 403 con un resolutor de permisos inyectado que no da ninguno (**[U3]**: hoy todos los roles tienen `orders.cancel_pending`, así que no existe un usuario en la base sin él);
  - `POST /orders/{id}/cancel` con un cajero → 403 con «Tu usuario no puede cancelar pedidos»
- [x] T021 [US7] Implementar `CancelPending` en server/internal/app/devolucion.go, en una transacción con el orden de candados de `CancelarRenglon`, que responde `{ removed, restocked }`; sin productos vivos y sin pagos cancela con «Sin productos» y sin reponer (cada renglón ya se resolvió al quitarlo); con pagos rechaza. `reason` es obligatorio con productos pendientes y se ignora sin productos vivos. **[M1]** Aquí nacen las consultas de partir un renglón (copiar el renglón y sus `order_line_modifiers`, insertar los pares `venta` con `reason = 'renglón partido'`) en server/queries/orders.sql con `make sqlc`, usando `domain.SplitLine`: no tocan tablas nuevas, su aislamiento entra en T032 y T057 las reusa. Ruta `POST /orders/{id}/lines/cancel-pending` con `RequirePermission(orders.cancel_pending)`, y `/orders/{id}/cancel` pasa de `RequireRole` a `RequirePermission(orders.cancel)`, en server/internal/httpapi/router.go y handlers_orders.go
- [x] T022 [P] [US7] Crear web/src/features/orders/OrdersBoardPage.test.tsx:
  - las **diez** combinaciones de la tabla de D-10: las ocho de pendiente × debe × el tablero cobra, más sin productos vivos con y sin pagos;
  - cada «Cerrar pedido» llama al endpoint que le toca (contracts/api.md): entregado y sin deuda → `deliver`; sin productos y sin pagos → `lines/cancel-pending` **sin motivo**;
  - **[M4]** el menú ofrece un ítem propio «Quitar lo que falta» con `can('orders.cancel_pending')` (hoy todos los roles), que abre `CancelPendingSheet`; «Cancelar pedido» sigue aparte;
  - sin productos y con pagos: solo el texto «Tiene pagos por devolver» (el acceso a devolver llega con la devolución, en la Fase 10);
  - una tarjeta de 11 productos con los botones a la vista;
  - «Cancelar pedido» con algo entregado abre la hoja de quitar lo que falta (con `[role=dialog]` presente tras tocar el ítem del menú);
  - «Cancelar pedido» solo con `can('orders.cancel')`;
  - el toast «Producto quitado»
- [x] T023 [US7] Implementar en web/src/features/orders/OrdersBoardPage.tsx:
  - las salidas de la tarjeta;
  - lista con `maxH` en dvh y scroll propio cuando hay más de 5 pendientes;
  - «Cancelar pedido» preguntando por `can('orders.cancel')`, y el ítem «Quitar lo que falta» por `can('orders.cancel_pending')`, que abre `CancelPendingSheet`;
  - el toast «Producto quitado»
- [x] T024 [P] [US7] Test vitest de la hoja de quitar lo que falta en web/src/features/orders/CancelPendingSheet.test.tsx:
  - motivos en filas de 44 px, sin preselección («Quitar los N que faltan» deshabilitado hasta elegir);
  - «Dejarlo» y «Quitar los N que faltan» separados;
  - con la petición pendiente la hoja sigue abierta; cierra solo cuando responde el servidor, y ante un rechazo se queda abierta con el texto
- [x] T025 [US7] Crear la hoja «Quitar los N que faltan» en web/src/features/orders/CancelPendingSheet.tsx, montada con el truco de montar cerrada de `CobrarSheet`
- [x] T026 [P] [US7] Test vitest en web/src/features/orders/CancelarRenglonDialog.test.tsx: motivos sin preselección («Quitar» deshabilitado hasta elegir) y la lista única de motivos (D-18). El contador «1 de 2» va en T060, porque depende de `qty` en el servidor
- [x] T027 [US7] Implementar los motivos en web/src/features/orders/CancelarRenglonDialog.tsx

**Checkpoint**: el incidente ya no se puede repetir aunque no exista dividir. El aislamiento de
`CancelPending` se prueba en T032, tras el rebase.

---

## Phase 4: User Story 8 — Quitar devuelve lo que no se consumió (P2, va antes por ser defecto)

**Goal**: quitar un producto sin preparación repone; cancelar después no repone dos veces.

**Independent Test**: se quita un refresco embotellado y su existencia vuelve; se quita un frappé ya
enviado y no se repone; cancelar el pedido después no repone ninguno de los dos.

- [x] T028 [P] [US8] IT `TestRemovingAProductWithoutPrepRestocksIt` en server/internal/integration/restock_unconsumed_test.go, **sin** desmarcar la cocina a mano
- [x] T029 [US8] IT `TestCancellingAfterRemovingALineRestocksOnce` en el mismo archivo, con las dos formas: renglón ya repuesto y renglón quitado ya consumido
- [x] T030 [P] [US8] Tabla de `domain.ReponeInventario(needsPrep, enviado)` en server/internal/domain/renglon_test.go: sin preparación repone aunque se haya enviado; con preparación y enviado no repone
- [x] T031 [US8] Cambiar `domain.ReponeInventario` en server/internal/domain/renglon.go. Traer `needs_prep` en `GetOrderLineForCancel` y excluir de `RestockCancelledOrder` los movimientos de renglones con `cancelled_at is not null` (server/queries/orders.sql, `make sqlc`). Ajustar `TestCancelarUnRenglonReponeSoloSiNoSalioACocina`, que hoy desmarca a mano

**Checkpoint**: lo de esta fase espera su prueba de aislamiento en T032.

---

## Phase 5: Foundational tras 021 — aislamiento de lo ya construido, esquema y vista

**Bloqueada por T002.** Todo lo que sigue toca tablas nuevas o consultas nuevas sobre tablas de empresa.

**Aislamiento de lo construido en las Fases 3–4 (D-11)**

- [x] T032 Casos `inTheThreeCases` para lo que se construyó antes del rebase:
  - las consultas modificadas `RestockCancelledOrder` y `GetOrderLineForCancel`, en server/internal/integration/restock_unconsumed_test.go;
  - el servicio `CancelPending` y las consultas de partir un renglón que nacen en T021, en server/internal/integration/no_way_out_test.go.
  Es la única prueba que se escribe después de su código (excepción aprobada por el dueño el 2026-10-05, *Complexity Tracking* del plan): si sale verde a la primera, se rompe a propósito la política o el `AcquireTenant` para verla en rojo por la razón correcta, y se restaura

**Esquema (data-model.md)**

- [x] T033 Escribir el test de la migración (en rojo) en server/internal/integration/migration_split_bill_test.go. Usa `restoredStore`, que lee `TEST_RESTORED_DATABASE_URL` y hace `t.Skip` si falta (server/internal/integration/harness_restaurado_test.go). Corre contra una **base restaurada de un respaldo real con dos empresas**, en una base **aparte** dentro de `egb027-pg` (`gatobobah_restored`; nunca la de `TEST_DATABASE_URL`, que `newTestStore` borra): `PG_CONTAINER=egb027-pg POSTGRES_DB=gatobobah_restored bash scripts/restaurar-respaldo.sh`, el script de `make db-restaurar`, con dueños y GRANT; nunca `--no-owner` ni `--no-privileges`. Exportar `TEST_RESTORED_DATABASE_URL=postgres://gatobobah:pw@localhost:5499/gatobobah_restored?sslmode=disable` además de `TEST_DATABASE_URL` (ver quickstart.md). Casos:
  - pagos existentes con `split_part`, `split_of` y `payment_number` nulos, y pedidos con `merged_into_order_id` nulo;
  - las cuatro tablas se pueden leer e insertar como `gatobobah_app`;
  - una fila de la empresa A no puede apuntar a un renglón, pago o pedido de la B (FK compuesta), tampoco con `merged_into_order_id`;
  - `merged_into_order_id` solo se acepta en un pedido `cancelada`;
  - **[U4]** el Down se niega si hay filas en `order_payment_voids`, en `order_payment_lines`, en `order_line_move_batches` u `order_line_moves`, algún `merged_into_order_id` no nulo, o **[L4]** algún `split_part` o `payment_number` no nulo en `order_payments`: un caso por cada una
- [x] T034 Escribir la migración server/migrations/NNNN_split_bill.sql, con NNNN = el siguiente número libre de `develop` **al momento de fusionar** (anotarlo como pendiente en el PR). Contenido:
  - `set local lock_timeout = '3s'`;
  - `unique (id, company_id)` en `order_lines` y `order_payments`;
  - `order_payment_lines`, `order_payment_voids`, `order_line_move_batches`, `order_line_moves` con RLS `nullif`, grants explícitos, FKs `no action` (salvo la cascada de la cobertura) e índices;
  - columnas `split_part`, `split_of`, `payment_number` en `order_payments` y `merged_into_order_id` en `orders`, con sus checks;
  - Down con la guarda de data-model.md (`raise exception`, patrón de 0037/0040/0041) y en su orden
- [x] T035 Tests `inTheThreeCases` en server/internal/integration/split_bill_isolation_test.go para cada consulta nueva de T036 (cobertura, bitácora, lotes, movimientos, piezas cubiertas por renglón, pagos del pedido con su cobertura, pagos devueltos del pedido y del turno). Se escriben primero: en rojo porque las consultas aún no existen
- [x] T036 Escribir esas consultas en server/queries/orders.sql y server/queries/cash.sql y correr `make sqlc` (store/db nunca se edita a mano)

**Pagos en la vista (D-12)**

- [x] T037 Tests de la vista:
  - IT HTTP en server/internal/integration/order_view_http_test.go, sobre JSON crudo: `GET /orders/{id}` trae `payments` y cada `lines` como arreglo aunque estén vacíos; un pago devuelto aparece con `voided: true` y su número; `mergedIntoOrderId` viene `null` en un pedido que no se juntó;
  - `inTheThreeCases` sobre `load` con `payments` en server/internal/integration/split_bill_isolation_test.go
- [x] T038 Agregar `payments` y `mergedIntoOrderId` a `OrderView` en server/internal/app/orders.go (`load`), con la numeración de D-12: los pagos sin número se numeran por `created_at` contando vivos y devueltos; los devueltos salen de la bitácora con el número que guardó
- [ ] T039 [P] Tipos `PaymentView`, `payments?: PaymentView[]` y `mergedIntoOrderId?: number | null` en `OrderView` de web/src/types/pos.ts (opcional para que `tsc` obligue a la guarda) y métodos nuevos de `posApi` en web/src/api/pos.ts (incluido `quote`); agregar cada método nuevo a los `vi.mock` existentes

**Checkpoint**: esquema y vista listos; `TestEveryCompanyTableIsIsolated` pasa con las cuatro tablas nuevas.

---

## Phase 6: User Story 1 — Cobrar a una persona solo lo suyo (P1) 🎯 MVP

**Goal**: «Dividir → Por productos» muestra el monto que calcula el servidor antes de cobrar, cobra la
selección, registra su cobertura e imprime el ticket de esa persona.

**Independent Test**: en un pedido de 11 renglones se cobra solo el Soju con tarjeta; «Falta» baja en
ese monto, el Soju aparece pagado tras recargar y el ticket trae solo el Soju.

- [ ] T040 [P] [US1] IT `TestTheIncidentTableSplitsWithoutCancellingAnything` en server/internal/integration/split_by_products_test.go: la mesa del incidente con pagos de 1, 4 y «Todo lo que falta»; suma = total, 0 cancelaciones, sin cambio neto en existencias, cobertura correcta. Y **[H7]** `TestAllRemainingWithNothingUncoveredChargesTheBalance`: con todas las piezas cubiertas y saldo positivo (estado armado en la base de prueba; en operación aparece al devolver un pago por monto), «Todo lo que falta» cobra el saldo sin cobertura
- [ ] T041 [US1] IT `TestDiscountedSplitAddsUpToTheTotal` en el mismo archivo (el último absorbe el centavo; falla nombrando el peso que se duplicó o faltó), con el caso **[H6]**: tras un pago por monto, «Todo lo que falta» deja Σ `order_payment_lines.amount` = monto del pago
- [ ] T042 [US1] IT y IT HTTP del contrato de `/pay` con `lines` y `allRemaining`, en el mismo archivo:
  - `lines`, `allRemaining` y `amount` se excluyen → 400;
  - la misma llave con otra selección → 409;
  - la respuesta trae `paymentId`, `number` y el monto cobrado; si la cotización quedó vieja, cobra lo que calcula el servidor y lo devuelve;
  - **[H8]** `payment_number` cuenta vivos y devueltos, incluidos los viejos sin número;
  - **[H2]** pedido de plataforma → «Los pedidos de plataforma no se dividen»;
  - **[H9]** pedido de un turno cerrado → «Ese pedido es de un turno cerrado; no se divide»;
  - **[D1]** `inTheThreeCases` sobre el servicio `Charge` con `lines` y `allRemaining`, que empieza a leer `order_payment_lines`
- [ ] T043 [P] [US1] **[U1]** IT e IT HTTP de `POST /orders/{id}/quote` en server/internal/integration/quote_test.go:
  - no escribe nada: mismas filas en `orders`, `order_payments` y `order_payment_lines` antes y después;
  - con `lines` y con `allRemaining` devuelve `{ amount, lines, outstandingAfter }`, y `amount` es el que `/pay` cobra después con la misma selección;
  - no pide `methodId`; las formas se excluyen → 400;
  - los mismos rechazos que `/pay`, sin los de idempotencia (pieza cubierta, plataforma, turno cerrado, excede);
  - mismo gate que `/pay`;
  - `inTheThreeCases` sobre el servicio `Quote`
- [ ] T044 [US1] Extender `Charge` y `ChargeCmd` en server/internal/app/orders.go (`split` no: va en T067):
  - `lines` y `allRemaining`, excluyentes entre sí y con `amount` (el handler en server/internal/httpapi/handlers_orders.go responde 400);
  - monto calculado siempre con `SelectionAmount` bajo el `FOR UPDATE` del pedido, con el prorrateo de D-3;
  - inserción de la cobertura y de `payment_number` (D-12), calculado bajo el candado;
  - rechazos de plataforma y de turno cerrado;
  - idempotencia que compara también la selección;
  - `ChargeResult` con `paymentId`, `number` y el monto cobrado
- [ ] T045 [US1] Implementar `OrdersService.Quote` en server/internal/app/orders.go con la misma función de `domain` que `Charge`, bajo un `SELECT` sin `FOR UPDATE` y sin escribir, y la ruta `POST /orders/{id}/quote` con el mismo gate que `/pay`, en server/internal/httpapi/router.go y handlers_orders.go
- [ ] T046 [P] [US1] Test de orden de la lista (pendientes arriba, pagados al final y agrupados con más de 4) en web/src/shared/cobro/listOrder.test.ts
- [ ] T047 [US1] Implementar web/src/shared/cobro/listOrder.ts
- [ ] T048 [P] [US1] Casos C16+ en web/src/shared/CobrarSheet.test.tsx:
  - el selector solo aparece tras tocar «Dividir»;
  - Por productos muestra los renglones con casillas de 48 px en dos columnas;
  - el contador «1 de 2» con − y + de 44 px, que arranca en 1;
  - **[U1]** al cambiar la selección se pide `quote` (con debounce, una sola llamada por ráfaga de toques) y el botón muestra ese monto; si `/pay` responde otro monto, el botón y «Falta» se refrescan con el de la respuesta;
  - la llave rota solo tras éxito y la selección se congela mientras se envía;
  - lo pagado sigue en gris tras recargar (viene de `payments`);
  - **[U2]** las fichas de pagos muestran número, método y monto;
  - **[U2]** «Todo lo que falta» manda `allRemaining`, no una lista de renglones;
  - **[U1]** en un pedido de plataforma o de un turno cerrado no se ofrece «Dividir»
- [ ] T049 [US1] Partir web/src/shared/CobrarSheet.tsx en componentes bajo web/src/shared/cobro/ (`ModePicker`, `ByProducts`, `PaymentChips`):
  - estado `view` para cambiar el contenido del mismo `DrawerContent`;
  - pie fijo mínimo (método, «Esta persona: N productos», botones);
  - propina y recibido dentro de la zona con scroll;
  - hoja a `100dvh` con el pie encogible en dvh (D-13);
  - contador «1 de 2» por renglón;
  - monto del botón desde `quote` con debounce
- [ ] T050 [P] [US1] Tests en web/src/utils/printReceipt.test.ts y de los tickets:
  - `buildReceiptHtml` con `opts.payment`: solo sus renglones y piezas, «Pago N», método, propina, cambio, «Del pedido quedan por pagar»;
  - `AutoPrintTicket` recuerda por id de pago;
  - `VerTicket` usa el prefijo `['orders', …]`: se refresca cuando cambia el pedido
- [ ] T051 [US1] Implementar el ticket por pago en web/src/utils/printReceipt.ts y web/src/shared/tickets/AutoPrintTicket.tsx; pasar `VerTicket` al prefijo `['orders', …]` en web/src/shared/tickets/ReprintTicket.tsx

**Checkpoint**: el caso B del incidente se resuelve sin cancelar nada.

---

## Phase 7: User Story 2 — Lo pagado no se cobra dos veces (P1)

**Goal**: una pieza no se cobra dos veces, ni desde dos tabletas.

**Independent Test**: dos cobros simultáneos de la misma pieza producen un solo pago.

- [ ] T052 [US2] IT `TestTheSamePieceCannotBePaidTwiceConcurrently` en server/internal/integration/split_by_products_test.go, con dos goroutines y varias vueltas, como `TestConcurrentDeliverAndCancelStillCloseTheOrder`; y el caso de CobrarSheet.test.tsx: ante «Ese producto ya se pagó» la pantalla refresca el pedido
- [ ] T053 [US2] Validar en `Charge` las piezas ya cubiertas bajo el candado del pedido y devolver `ErrPieceAlreadyPaid` («Ese producto ya se pagó»). La pantalla en web/src/shared/cobro/ refresca el pedido ante ese rechazo

---

## Phase 8: User Story 3 — El pedido sigue vivo tras un pago parcial (P1)

**Goal**: pagado uno, el pedido recibe más productos sin tocar lo pagado.

**Independent Test**: tras un pago por productos se agregan dos productos; la hoja los muestra sin
pagar y el pago anterior sigue intacto.

- [ ] T054 [P] [US3] IT `TestAddingLinesAfterAPartialPaymentKeepsItIntact` en server/internal/integration/split_by_products_test.go (la cocina recibe solo los nuevos)
- [ ] T055 [US3] IT `TestRemovingAPaidLineIsRejected` en el mismo archivo. **Primero confirmar con el test** si hoy quitar un renglón de un pedido cobrado deja dinero de más (FR-023, no verificado), y anotarlo en research.md
- [ ] T056 [P] [US3] Tests de `qty` al quitar, en server/internal/integration/restock_unconsumed_test.go:
  - quitar 1 de 2 parte el renglón y deja las dos mitades con sus totales;
  - `qty` mayor que lo pendiente → 400 «No hay tantas piezas por quitar»;
  - `TestASplitLineRestocksEachHalfOnce`: con un producto sin preparación, quitar 1 de 2 y luego la otra; cada mitad repone solo la suya;
  - **[H1]** `cancel-pending` con piezas pendientes cubiertas por un pago → 409 «Ese producto ya se pagó. Primero hay que devolver el pago»;
  - **[H1]** `cancel-pending` que dejaría el total bajo lo pagado → 409 «Ya se cobró más de lo que quedaría. Primero hay que devolver un pago».
  - **[D1]** `inTheThreeCases` sobre el servicio `CancelarRenglon`, que empieza a leer `order_payment_lines`.
  Las consultas de partir son las de T021, ya aisladas en T032
- [ ] T057 [US3] En server/internal/app/devolucion.go, **una sola función** de validación de pagos —rechazar piezas cubiertas y dejar el total bajo lo pagado— que llaman `CancelarRenglon` y `CancelPending` (no una copia en cada una). `CancelarRenglon` gana `qty` usando `SplitLine` y las consultas de partir de T021. El handler acepta `qty` opcional. **`CancelPending` y `splitOrderLine` hoy llaman `SplitLine` con `Covered` en cero**: con esta tarea pasan las piezas cubiertas por pagos vivos en `LinePieces.Covered` y, si lo que se quita está pagado, devuelven `domain.ErrPieceAlreadyPaidToRemove` («Ese producto ya se pagó. Primero hay que devolver el pago»), ya definido en server/internal/domain/errors.go
- [ ] T058 [US3] IT `TestDiscountIsRejectedOncePaymentsExist` en server/internal/integration/split_by_products_test.go, y el caso de CobrarSheet.test.tsx: «Descuento» oculto con pagos
- [ ] T059 [US3] Rechazar en `SetDiscount` (server/internal/app/orders.go) un pedido con pagos (D-17). Ocultar «Descuento» con pagos en la hoja
- [ ] T060 [P] [US7] Test del contador «1 de 2» en web/src/features/orders/CancelarRenglonDialog.test.tsx (arranca en 1, − y + de 44 px, manda `qty`). Va aquí porque depende de T057
- [ ] T061 [US7] Implementar el contador en web/src/features/orders/CancelarRenglonDialog.tsx
- [ ] T062 [US3] **[C1]** Tests de los renglones del tablero en server/internal/integration/split_by_products_test.go: IT HTTP sobre JSON crudo de la respuesta del tablero (`OrdersService.Board`), donde cada `BoardLine` trae sus piezas cubiertas por pagos (`paidQty`, 0 si no hay pagos, nunca ausente); e `inTheThreeCases` sobre la consulta que las suma (junto a `ListLinesOfActiveOrders`)
- [ ] T063 [US3] Agregar las piezas cubiertas a `BoardLine` (server/internal/app/orders.go, `lineasDelTablero`) con su consulta en server/queries/orders.sql y `make sqlc`
- [ ] T064 [P] [US3] Tests vitest: «Pedidos por cobrar» dice cuánto se pagó y cuánto falta, sin «1 de 3» (web/src/features/pos/PedidosEnCurso.test.tsx); el caso del bote deshabilitado con «Pagado» en productos pagados vive en web/src/features/orders/OrdersBoardPage.test.tsx y usa `paidQty` de `BoardLine`
- [ ] T065 [US3] Implementar los dos en web/src/features/pos/PedidosEnCurso.tsx y web/src/features/orders/OrdersBoardPage.tsx

---

## Phase 9: User Story 4 — Partes iguales y por monto (P2)

**Goal**: los otros dos modos conviven con Por productos y sobreviven a recargar.

**Independent Test**: se cobra en tres partes, se recarga tras la primera y las partes pagadas siguen
marcadas; la suma es lo que faltaba.

- [ ] T066 [P] [US4] IT en server/internal/integration/split_parts_test.go:
  - `TestTheSameSplitPartCannotBeChargedTwice` y `TestSplitPartsSurviveAReload`;
  - IT HTTP: `split` junto con `lines`, `allRemaining` o `amount` → 400;
  - `split` en pedido de plataforma o de turno cerrado → rechazo (H2, H9);
  - `quote` con `split` devuelve la parte que `/pay` cobra después
- [ ] T067 [US4] `Charge` y `Quote` con `split`: monto con `SplitPartAmount`, validación de parte ya cobrada (`ErrSplitPartAlreadyCharged`), columnas `split_part`/`split_of`, exclusión con las otras formas
- [ ] T068 [P] [US4] Casos en web/src/shared/CobrarSheet.test.tsx:
  - Entre personas reparte lo que falta y muestra fichas por parte (tablero A7), con el monto de cada parte pedido a `quote` con `split`;
  - Por monto con teclado propio de 52 px, montos rápidos y «Lo que falta» (A8);
  - el mensaje cuando la selección por productos excede tras un pago por monto
- [ ] T069 [US4] Implementar `EvenSplit` (montos desde `quote`) y `ByAmount` en web/src/shared/cobro/; quitar `dividirEnPartes`/`montoDeLaParte` de web/src/domain/cobro.ts (ya viven en el servidor) y ajustar cobro.test.ts

---

## Phase 10: User Story 6 — Corregir un cobro (P2)

**Goal**: devolver un pago concreto deja sus productos por cobrar y el cajón cuadrado.

**Independent Test**: se devuelve un pago y se cobra a la persona correcta; el corte espera por método
lo correcto y lista el devuelto.

- [ ] T070 [P] [US6] IT `TestAVoidedPaymentCountsZeroTimesInTheDrawer` en server/internal/integration/void_payment_test.go (corte por método, propinas y lo pendiente; falla nombrando dónde reapareció)
- [ ] T071 [US6] IT en el mismo archivo:
  - `TestAVoidedPaymentCannotBeRevivedByItsKey` (secuencial **y** concurrente con la devolución);
  - `TestAPaymentIsVoidedOnce`;
  - `TestAPaymentFromAClosedShiftCannotBeVoided` (incluye sesión nula);
  - `TestPaymentNumbersSurviveAVoid`, con un pago viejo sin número: la bitácora guarda el número que la vista le daba, y el siguiente pago no lo repite
- [ ] T072 [US6] Aislamiento y permiso de la devolución, en el mismo archivo:
  - `inTheThreeCases` sobre el servicio `VoidPayment` y sobre la vista del turno con `voidedPayments`;
  - IT HTTP: `POST /orders/{id}/payments/{paymentId}/void` con un cajero → 403 con «Tu usuario no puede devolver pagos»
- [ ] T073 [US6] Implementar `VoidPayment` en server/internal/app/devolucion.go:
  - `GetOrderForUpdate` antes de leer el pago;
  - copia completa a la bitácora, con el número que la vista le daba, y borrado;
  - consulta de la bitácora en la idempotencia de `Charge`.
  Ruta `POST /orders/{id}/payments/{paymentId}/void` con `RequirePermission(payments.void)`, `rateLimitUser` y `SecurityEvent "order_payment_voided"`
- [ ] T074 [US6] Tests de la lista de devueltos en el corte: IT HTTP sobre JSON crudo de la vista del turno con `voidedPayments` siempre arreglo (void_payment_test.go), y web/src/features/backoffice/CashPage.test.tsx
- [ ] T075 [US6] Agregar `voidedPayments` a la vista del turno en server/internal/app/backoffice.go y mostrarlo en web/src/features/backoffice/CashPage.tsx
- [ ] T076 [P] [US6] Casos en web/src/shared/CobrarSheet.test.tsx:
  - la ficha abre el detalle;
  - «Devolver este pago» deshabilitado sin `can('payments.void')`, con «Tu usuario no puede devolver pagos»;
  - motivo sin preselección;
  - aviso según método (tarjeta: el reembolso en la terminal va aparte);
  - el pago devuelto queda tachado con su número;
  - y en web/src/features/orders/OrdersBoardPage.test.tsx: la tarjeta sin productos y con pagos ofrece, junto a «Tiene pagos por devolver», el acceso a devolverlos solo con `can('payments.void')`
- [ ] T077 [US6] Implementar `PaymentDetail` en web/src/shared/cobro/ (vista del mismo Drawer; «Reimprimir» abre el `Dialog` existente) y el acceso a devolver desde la tarjeta sin productos en web/src/features/orders/OrdersBoardPage.tsx

---

## Phase 11: User Story 5 — Pasar productos a otro pedido (P2)

**Goal**: los productos se mueven sin cancelar, recapturar ni tocar inventario.

**Independent Test**: se pasan dos productos ya enviados a cocina a un pedido nuevo; no hay comanda
nueva, existencias iguales, y origen y destino tienen sus totales correctos.

- [ ] T078 [P] [US5] IT en server/internal/integration/move_lines_test.go:
  - `TestMovedLinesKeepKitchenStateAndStock`;
  - `TestTheNewOrderInheritsTheOriginShift`: turno, día, servicio y `opened_by` del origen (quien capturó esos productos); quien los pasó queda en `moved_by`;
  - **[C2]** `TestAPartialMoveClosesBothOrdersWhenNothingIsLeft`: tras pasar **parte** de los productos, el origen y el destino se cierran solos si ya no les falta nada por entregar y están pagados (AS5)
- [ ] T079 [US5] IT en el mismo archivo:
  - `TestMoveRejections` (tabla con todos los rechazos de contracts/api.md, incluidos otro turno u otro día, **[H9]** origen de un turno cerrado, todos los productos hacia un pedido nuevo y todos hacia uno existente con envío en el origen);
  - `TestMoveIsIdempotentByBatch`;
  - `TestMovingEverythingMergesTheOriginWithoutRestock`: hacia un pedido existente, el origen queda `cancelada` con el motivo fijo y `merged_into_order_id` = destino, sin reponer inventario;
  - `TestAMergedOrderIsNotACancellation`, tras un movimiento total:
    - el reporte de cancelaciones (`SalesTotalsByStatus` y su gemela) no cuenta el origen, y lista y resumen de Ventas siguen derivándose del mismo predicado;
    - **[C1]** las ventas del turno del corte (`SessionSales` y `CountSessionSales`) tampoco lo listan como cancelado, con el mismo predicado en lista y conteo;
    - **[M2]** `SalesCancelledLines` y su gemela cuentan los productos que se quitaron del origen antes de juntarlo. Hoy **no** los cuentan: filtran `o.status not in ('cancelada', 'reembolsada')`, así que el test se ve en rojo antes de T082
- [ ] T080 [US5] Aislamiento y permiso de «Pasar», en el mismo archivo:
  - `inTheThreeCases` sobre el servicio `MoveLines`, cada consulta nueva de T081 y las consultas que cambia T082;
  - IT HTTP: `POST /orders/{id}/lines/move` → 403 con un resolutor de permisos inyectado que no da ninguno (**[U3]**: hoy todos los roles tienen `orders.move_lines`), con «Tu usuario no puede pasar productos»
- [ ] T081 [US5] Implementar `MoveLines` en server/internal/app/move_lines.go:
  - candados en orden ascendente de id;
  - lote idempotente (D-12b);
  - pedido nuevo con turno, día, servicio y `opened_by` del origen, y folio de ese turno;
  - partir con pares de movimientos;
  - mover renglones y sus movimientos por `order_line_id`;
  - recalcular y cerrar los dos; el origen vacío se junta con el destino (D-5) y su `OrderView` sale con `mergedIntoOrderId`.
  Ruta `POST /orders/{id}/lines/move` con `RequirePermission(orders.move_lines)` y `rateLimitUser`; publica eventos para los dos pedidos
- [ ] T082 [US5] Ajustar los reportes para el pedido juntado (D-5), con `make sqlc`:
  - server/queries/sales.sql: las gemelas de Ventas (`ListSales`, `CountSales`, `SalesTotalsByStatus`, `SalesTotalsByMethod` y sus `…SinFolio`) excluyen el pedido con `merged_into_order_id` con el mismo predicado en lista y resumen; **[M2]** `SalesCancelledLines` y su gemela agregan `or o.merged_into_order_id is not null` a su filtro de estado, para que lo quitado antes de juntar siga contando como producto cancelado;
  - server/queries/cash.sql: `SessionSales` y `CountSessionSales` con el mismo predicado en lista y conteo (la lista de ventas del turno en web/src/features/backoffice/CashPage.tsx pinta lo que trae la consulta).
- [ ] T083 [P] [US5] Test de `ListRow` en web/src/components/ListRow.test.tsx (alto parametrizable, 56 px para pedidos) y de que `Picker` sigue igual
- [ ] T084 [US5] Sacar `PickerRow` a web/src/components/ListRow.tsx (`ListRow`) y que web/src/components/Picker.tsx la importe sin cambiar su comportamiento
- [ ] T085 [P] [US5] Casos en web/src/shared/CobrarSheet.test.tsx:
  - «Pasar a otro pedido» deshabilitado con su motivo cuando se sabe de antemano;
  - con todo seleccionado, «+ Pedido nuevo» deshabilitado con «Ya es su propio pedido; no hace falta pasarlo»;
  - la lista con «+ Pedido nuevo» y filas de 56 px;
  - el buscador no se enfoca solo;
  - tocar destino marca y pide confirmar;
  - tras «+ Pedido nuevo» aparece «Cobrar #N»;
  - origen vacío cierra la hoja con aviso
- [ ] T086 [US5] Implementar `MoveToOrder` en web/src/shared/cobro/ como vista del mismo Drawer, y el salto a cobrar el pedido nuevo sin cerrar una hoja y abrir otra en el mismo update (AGENTS.md §3)

---

## Phase 12: Polish & Cross-Cutting

- [ ] T087 IT `TestNoSequenceLeavesAnOrderWithoutAWayOut` en server/internal/integration/no_way_out_test.go (SC-003): recorre secuencias de cobrar, quitar (un producto y **lo que falta**, incluidos sus rechazos por pagos), pasar y devolver en varios órdenes, y al final verifica que cada pedido abierto tiene una acción que lo cierra. Si encuentra uno, el arreglo lleva su test de regresión antes
- [ ] T088 [P] Medición (D-15): test primero en server/internal/domain/uso_test.go; acciones `split-by-products`, `move-lines`, `void-payment`, `close-order` y `cancel-pending` en server/internal/domain/uso.go y llamadas a `medirAccion` en el front; actualizar `ARCHIVOS` en web/src/api/uso-orden.test.ts
- [ ] T089 [P] E7 bis en web/e2e/cabe-en-la-tableta.spec.ts, a 600 px de alto: la hoja de cobro con Efectivo, propina y la lista de productos, y con el teclado del sistema abierto «Cobrar» sigue alcanzable; **[L3]** `CancelPendingSheet` y la vista `MoveToOrder` también caben con su botón de confirmar visible
- [ ] T090 Reescribir web/e2e/dividir-cuenta-incidente.spec.ts como regresión y renombrarlo a web/e2e/split-bill-incident.spec.ts: la misma mesa con «Dividir → Por productos», ≤ 20 toques contados y ningún pedido abierto al final; usa `tokenDeRequest` y deja todo cobrado y entregado
- [ ] T091 [P] Actualizar docs/matriz-de-cobro.md y docs/matriz-de-pantallas.md con los casos nuevos y lo que **no** queda cubierto
- [ ] T092 [P] Documentar en AGENTS.md la mecánica nueva (cobertura, cotización, bitácora de pagos devueltos, pedidos juntados, permisos)
- [ ] T093 Gates completos en el worktree: `go build ./... && go test ./...`, integración contra `egb027-pg` con `TEST_DATABASE_URL` **y** `TEST_RESTORED_DATABASE_URL`, incluido `TestEveryCompanyTableIsIsolated`; verificar que el test de la migración corrió de verdad: `go test -tags=integration -v -run MigrationSplitBill ./internal/integration/... | grep -c SKIP` da 0; golangci-lint, `bun run lint`, `bun run vitest run`, `bun run build`, `bun audit --audit-level=high`
- [ ] T094 Correr `/revision-de-codigo` sobre el diff (hook `after_implement`)

---

## Dependencies & Execution Order

- **T001** → todo.
- **Fase 2** (permisos, dinero puro), **US7 (3)** y **US8 (4)** no tocan tablas nuevas: se construyen
  sin esperar a 021. Las consultas existentes que modifican quedan cubiertas por T032.
- **T002** (021 en `develop` y rebase) bloquea la **Fase 5** y todo lo posterior.
- **Fase 5** necesita T002; T032 cierra el aislamiento de US7 y US8.
- **US1 (6)** necesita la Fase 5 completa. **US2, US3 y US4** dependen de T044; el front de US1 y
  US4 depende de T045 (`quote`).
- **T021** (`CancelPending` y las consultas de partir) necesita T015 (`SplitLine`).
- **T014** (mapeo de los sentinels nuevos) va después de T013 (textos sin prefijo): los dos son de
  `respond.go`.
- **T062–T064** (piezas pagadas en el tablero y el bote «Pagado») necesitan la Fase 5 y T044.
- **El acceso a devolver desde la tarjeta** (T076–T077) depende de T073 (`VoidPayment`).
- **T057** (`qty` al quitar y la validación de pagos compartida, en US3) necesita T021 y la Fase 5; **T060–T061** (el
  contador de US7) dependen de T057.
- **US6 (10)** depende de T038 (números y vista) y de T044.
- **US5 (11)** depende de T015 (partir) y de T057.
- **Polish (12)** al final; T087 necesita todas las operaciones.
- **Fusión**: no antes de terminar la Fase 8 (ver *Implementation Strategy*).

## Parallel Opportunities

- En la Fase 2: T003, T007, T009, T011 y T012 tocan archivos distintos (T010 comparte archivo con
  T009, y T014 con T012; van después).
- Mientras se espera T002: Fases 2, 3 y 4 completas.
- Dentro de cada historia, solo las tareas marcadas [P] van a la vez: varias pruebas de integración
  de una misma historia comparten archivo y van en serie.
- Front y back de una misma historia van en paralelo una vez fijado el contrato (contracts/api.md).

## Implementation Strategy

1. **Antes de 021**: Setup + Fase 2 + US7 + US8. Ningún pedido se atora y el inventario se repone bien.
2. **MVP tras 021**: Fase 5 + US1 + US2. La mesa del incidente se divide por productos sin cancelar nada.
3. Después US3 y US4: el pedido vivo tras pagos y los otros dos modos.
4. Después US6 y US5: corregir un pago y pasar productos.
5. Cada incremento cierra con sus gates en verde y su checkpoint.
6. **La rama no se fusiona antes de terminar la Fase 8** (US3): las fases intermedias no salen a
   producción por separado. Antes de ella, un pago por productos podría quedar en un pedido al que
   todavía se le puede quitar el producto pagado o cambiar el descuento.
7. **Al fusionar**: renumerar la migración al siguiente libre de `develop` y verificar el orden
   frente a 021, 025 y 026 (D-11).
