# Implementation Plan: Dividir la cuenta por productos

**Branch**: `027-dividir-la-cuenta` | **Date**: 2026-10-05 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/027-dividir-la-cuenta/spec.md`

## Summary

Un pago aprende qué productos cubrió. Con eso, la hoja de cobro divide por productos sin cancelar
ni recapturar nada, y dos tabletas no pueden cobrar la misma pieza. Alrededor de esa pieza central
van tres capacidades que cierran los otros caminos del incidente: devolver un pago concreto, pasar
productos a otro pedido moviéndolos, y quitar piezas sueltas devolviendo lo que no se consumió.

Lo que carga el peso:

1. **`domain`** decide todo el dinero: el monto de una selección con su parte de descuento y el
   residuo en el último pago (D-3), las partes iguales (D-4, movidas desde el front), partir un
   renglón (D-7) y qué repone inventario (D-8). La pantalla ve el monto antes de cobrar con
   `POST /orders/{id}/quote`, de solo lectura y con la misma función.
2. **Una migración** (número asignado al fusionar, D-11): la cobertura (`order_payment_lines`), la
   bitácora de pagos devueltos (`order_payment_voids`), el rastro de lo que se pasó de un pedido a
   otro (`order_line_move_batches`, `order_line_moves`), tres columnas en `order_payments` (parte y
   número estable) y una en `orders` (`merged_into_order_id`, el pedido que se juntó con otro, D-5).
3. **Devolver un pago lo saca de `order_payments`** (D-2). Es la decisión más cargada: deja
   correctas por construcción las ~32 consultas que leen `order_payments` (medido: 32 menciones en
   `server/queries/`), en lugar de exigir que cada una
   aprenda a ignorar un pago marcado.
4. **Los controles nuevos preguntan por permiso, no por rol** (D-16): los roles serán
   configurables por empresa, y el mapa rol → permisos de hoy es lo único que pasará a la base.
   «Cancelar pedido» pasa también a permiso (`orders.cancel`).
5. **La rama de Uber (021) manda el orden** (D-11): lo que toca tablas nuevas o consultas nuevas
   sobre tablas de empresa espera a que 021 llegue a `develop` y se rebasa encima.

Las decisiones con su porqué y lo descartado están en [research.md](./research.md); el esquema, en
[data-model.md](./data-model.md); los endpoints, en [contracts/api.md](./contracts/api.md).

## Technical Context

**Language/Version**: Go 1.27 (backend) · TypeScript 5 + React 19 (web)

**Primary Dependencies**: chi · pgx + sqlc · goose · shopspring/decimal | Vite · Chakra UI v3 ·
TanStack Query · Zustand

**Storage**: PostgreSQL. Cuatro tablas nuevas, cuatro columnas (tres en `order_payments`, una en
`orders`), dos `unique (id, company_id)`; sin relleno de datos.

**Testing**: unitarios table-driven en `domain`; integración contra Postgres real bajo
`appRoleStore` con `inTheThreeCases`; vitest; Playwright contra el ambiente de pruebas a 1024×600.

**Target Platform**: tabletas de 7–10", presupuesto 1024×600.

**Project Type**: monorepo web (server/ + web/).

**Constraints**: la hoja de cobro ya mide ~580 px con efectivo y propina (estimado); la lista de
productos no puede empujar el botón de cobrar fuera de la pantalla. El inventario no puede moverse
por dividir una cuenta.

**Scale/Scope**: un local, ~200 pedidos al día; una mesa dividida tiene de 2 a 12 pagos.

## Constitution Check

*GATE: pasa antes de Phase 0 y se volvió a evaluar después del diseño.*

| Principio | Cómo lo cumple |
|---|---|
| **I. Layering** | Ver *Dónde aterriza cada pieza*. Ninguna regla de dinero ni de inventario vive en un handler o en la pantalla; el front deja de calcular partes (D-4). |
| **II. Errores** | Sentinels de `domain` envueltos con `%w`; mapeo solo en `httpapi.Error`. Los textos para quien opera no llevan el prefijo del sentinel (D-10). `context` propagado hasta cada query. |
| **III. Dinero** | `Round2`/`ValidMoney` en cada frontera; el servidor calcula todo monto (selección, parte, lo que falta) e ignora el de la pantalla. **Cada peso se clasifica una vez**: un pago devuelto en su turno no es ingreso ni salida (D-2); la propina sigue fuera del total; el envío lo cubre el último pago. Tests que nombran el concepto duplicado (ver bordes). |
| **IV. Test-first** | Bordes enumerados en el spec y atados a su test abajo; cada test se ve en rojo antes del código, también los de contrato y el IT HTTP de 403 de cada ruta nueva. Migración con su test, contra un respaldo real con dos empresas (el hook `migracion-con-test.sh` lo exige). `inTheThreeCases` sobre toda consulta nueva o modificada y sobre cada servicio nuevo que lee tablas de empresa (`CancelPending`, `VoidPayment`, `MoveLines`, `load` con pagos, la vista del turno). La única excepción de orden es la de D-11: el aislamiento de lo construido antes de 021 se prueba tras el rebase, y se fuerza a verlo en rojo. |
| **V. Seguridad** | Devolver un pago exige el permiso `payments.void` en el router (`RequirePermission`), con `SecurityEvent` y tope por usuario, y toma el candado del pedido antes de leer el pago (no revive por reintento). Cancelar un pedido pasa a `RequirePermission(orders.cancel)`. Pasar productos no puede mover dinero: rechaza piezas pagadas y dejar el origen sobrepagado; tope por usuario. Cada ruta nueva tiene su test de router con 403. FKs compuestas: los chequeos de FK saltan RLS. Entrada absurda = 400. |
| **VI. YAGNI** | Sin mesas, sin personas, sin dividir pedidos de plataforma, sin reparto de descuento entre pedidos (D-6). Se reusa `Charge`, `DeliverAll`, el folio y la bolsa de nombres. |
| **VII. Comentarios** | Porqué en cada decisión no obvia: sacar el pago en vez de marcarlo, insertar movimientos al partir, el orden de candados, rechazar con descuento. **Identificadores nuevos en inglés**: tipos y funciones (`SelectionAmount`, `SplitParts`, `SplitPartAmount`, `SplitLine`, `MovablePieces`, `VoidPayment`, `MoveLines`, `CancelPending`), archivos (`split_bill.go`, `permissions.ts`, `listOrder.ts`, `ListRow.tsx`, `CancelPendingSheet.tsx`, `NNNN_split_bill.sql`, `split-bill-incident.spec.ts`), componentes (`ByProducts`, `EvenSplit`, `ByAmount`, `PaymentDetail`, `MoveToOrder`, `PaymentChips`), campos JSON (`removed`, `restocked`) y las claves de medición, que se guardan como dato. Lo existente en español (`CancelarRenglon`, `ReponeInventario`, `CobrarSheet`) no se renombra. El texto de pantalla sigue en español. |
| **VIII. Puertas** | **¿Se puede agregar después al mismo costo?** La cobertura por renglón **no**: un pago sin ella no se puede reconstruir, por eso se construye hoy. Reparto de descuento entre pedidos, saber qué persona pagó, mesas: **sí**, quedan fuera sin cerrarse. *Más de una caja*: los candados son por pedido, no por caja; un pago guarda su turno. El pedido nuevo de «pasar» copia el turno del origen; nada pide «la caja abierta». *Saber de quién es un pedido*: el pedido nuevo lleva el `opened_by` del origen (quien capturó esos productos) y quien los pasó queda en `moved_by`. *Descuentos*: se rechaza mover con descuento y cambiar el descuento con pagos; no se pierde ni se reescribe. *Roles y permisos por empresa* (nueva, 2026-10-05): todo control nuevo pregunta por permiso, y `/orders/{id}/cancel` deja `RequireRole`; no se agrega ningún `RequireRole` ni `role ===`. |

Una excepción registrada en *Complexity Tracking*; ninguna otra violación.

## Dónde aterriza cada pieza

| Pieza | Capa | Archivo |
|---|---|---|
| Monto de una selección (`SelectionAmount`), prorrateo por renglón, residuo, tope contra lo que falta | domain | `server/internal/domain/split_bill.go` (nuevo) |
| Partes iguales `SplitParts`/`SplitPartAmount` (desde `web/src/domain/cobro.ts`) | domain | `split_bill.go` |
| Partir un renglón (`SplitLine`) y piezas movibles (`MovablePieces`) | domain | `server/internal/domain/renglon.go` |
| `ReponeInventario(needsPrep, enviado)` | domain | `renglon.go` |
| Pedido sin productos no se entrega; texto de `ErrCancelarConEntregas`; sentinels nuevos | domain | `entrega.go`, `errors.go` |
| Permisos: `Permission`, `PermissionsFor(role)`, `PermissionDeniedMessage(p)` | domain | `server/internal/domain/permission.go` (nuevo) |
| Cobrar con renglones, todo lo que falta o parte; número de pago; idempotencia contra la bitácora | app | `server/internal/app/orders.go` (`Charge`) |
| Cotizar el monto antes de cobrar, sin escribir ni tomar candados | app | `orders.go` (`Quote`) |
| Devolver un pago | app | `server/internal/app/devolucion.go` (`VoidPayment`) |
| Pasar productos y juntar el origen vacío | app | `server/internal/app/move_lines.go` (nuevo, `MoveLines`) |
| Quitar piezas y quitar lo que falta, con **una sola** validación de pagos para los dos; cerrar un pedido sin productos y sin pagos | app | `devolucion.go` (`CancelarRenglon`, `CancelPending`) |
| Partir un renglón (copiar renglón y modificadores, pares `venta` «renglón partido»): nacen con `CancelPending`, antes del rebase, y los reusan quitar piezas y pasar | store + app | `server/queries/orders.sql`, `devolucion.go` |
| Pagos en la vista del pedido; lista del corte | app | `orders.go` (`load`), `backoffice.go` |
| Queries | store | `server/queries/orders.sql`, `server/queries/cash.sql` (incluye `SessionSales`/`CountSessionSales` con el pedido juntado), `server/queries/sales.sql` (pedido juntado) + `make sqlc` |
| Esquema | migración | `server/migrations/NNNN_split_bill.sql` (número al fusionar) |
| Handlers y rutas (incluida `POST /orders/{id}/quote`) | httpapi | `handlers_orders.go`, `router.go` |
| `RequirePermission`; permisos en la sesión y en `/auth/me`; mensaje sin prefijo del sentinel | httpapi | `middleware.go`, `handlers.go` (`writeSession`, que comparten login, refresh y pin-switch; `Me`), `respond.go` |
| Medición | domain + web | `domain/uso.go`, llamadas a `medirAccion` |
| `can(permission)` | web | `web/src/stores/session.ts`, `web/src/app/permissions.ts` (nuevo) |
| Hoja de cobro con selector y vistas | web | `web/src/shared/CobrarSheet.tsx` y `web/src/shared/cobro/` (nuevos: `ModePicker`, `ByProducts`, `EvenSplit`, `ByAmount`, `PaymentChips`, `PaymentDetail`, `MoveToOrder`, `listOrder.ts`) |
| Fila de lista de 56 px | web | `web/src/components/ListRow.tsx` (sale de `PickerRow`) |
| Tarjeta del tablero (ítems «Quitar lo que falta» con `orders.cancel_pending` y «Cancelar pedido» con `orders.cancel`, aparte) y hoja de quitar lo que falta | web | `web/src/features/orders/OrdersBoardPage.tsx`, `CancelPendingSheet.tsx` (nuevo) |
| Quitar piezas, motivos sin preselección | web | `CancelarRenglonDialog.tsx` |
| «Pedidos por cobrar» con pagado y falta | web | `web/src/features/pos/PedidosEnCurso.tsx` |
| Pagos devueltos en el corte | web | `web/src/features/backoffice/CashPage.tsx` |
| Ticket por pago | web | `web/src/utils/printReceipt.ts`, `AutoPrintTicket` |
| Tipos y API | web | `web/src/types/pos.ts`, `web/src/api/pos.ts` |

## Bordes → test (se escribe antes que el código)

| Borde | Test | Nivel |
|---|---|---|
| La mesa del incidente se resuelve con 3 pagos que suman el total, sin cancelaciones ni cambio neto en existencias | `TestTheIncidentTableSplitsWithoutCancellingAnything` | integración |
| Misma pieza desde dos tabletas a la vez | `TestTheSamePieceCannotBePaidTwiceConcurrently` | integración |
| Descuento repartido: la suma de pagos es el total al centavo; el último absorbe | tabla de `SelectionAmount` + `TestDiscountedSplitAddsUpToTheTotal` | unitario + integración |
| Pago por monto antes que por productos: la selección no pasa de lo que falta | tabla de `SelectionAmount` | unitario |
| Cubrir todo lo pendiente tras un pago por monto: Σ montos por renglón = monto del pago (prorrateo, D-3) | tabla de `SelectionAmount` + `TestDiscountedSplitAddsUpToTheTotal` | unitario + integración |
| «Todo lo que falta» sin piezas por cubrir y con saldo | tabla de `SelectionAmount` + `TestAllRemainingWithNothingUncoveredChargesTheBalance` | unitario + integración |
| `lines`, `split`, `allRemaining` y `amount` se excluyen; la misma llave con otra selección; `paymentId`, `number` y el monto cobrado en la respuesta | IT HTTP + IT de `/pay` | integración |
| Cotizar no escribe nada y da el monto que `/pay` cobra; mismos rechazos; aislada | IT de `quote` con `inTheThreeCases` | integración |
| Toda selección: Σ montos por renglón = monto del pago, residuo en el último renglón | tabla de `SelectionAmount` | unitario |
| Dividir un pedido de plataforma o de un turno cerrado | IT de `/pay` y `TestMoveRejections` | integración |
| Partes iguales sobre lo que falta, con residuo en la última | tabla de `SplitParts` | unitario |
| Pago devuelto: no cuenta en el esperado, se lista, sus productos vuelven a deberse | `TestAVoidedPaymentCountsZeroTimesInTheDrawer` (falla nombrando dónde apareció) | integración |
| Reintento del cobro original tras devolverlo, **también concurrente** con la devolución | `TestAVoidedPaymentCannotBeRevivedByItsKey` (secuencial y con dos goroutines) | integración |
| Devolver el mismo pago dos veces | `TestAPaymentIsVoidedOnce` | integración |
| El Down se niega con bitácora, cobertura, lotes o movimientos no vacíos, o con algún pedido juntado | test de la migración | integración |
| Un pedido sin productos y sin pagos lo cierra cualquier rol (200 por HTTP para los cuatro, sin motivo), y cuenta como cancelación; con pagos se rechaza | `TestAnOrderWithoutProductsCanBeClosedByAnyRole` (IT e IT HTTP) | integración |
| Quitar lo que falta con piezas pagadas o dejando el total bajo lo pagado: los mismos 409 que quitar un producto | casos de `cancel-pending` junto a los de `qty` | integración |
| Pasar parte de los productos cierra solos origen y destino cuando ya no les falta nada | `TestAPartialMoveClosesBothOrdersWhenNothingIsLeft` | integración |
| Devolver un pago de un turno cerrado | `TestAPaymentFromAClosedShiftCannotBeVoided` | integración |
| Sin el permiso, por la API: devolver, pasar, quitar lo que falta, cancelar el pedido | un IT HTTP por ruta (403 con su texto), con el patrón de `ventas_http_test.go`; para los permisos que hoy tienen todos los roles, un resolutor de permisos inyectado que no da ninguno | integración |
| `PermissionsFor`: qué tiene cada rol de hoy; todo permiso con su texto | tabla | unitario |
| La sesión (login, refresh, pin-switch) y `/auth/me` traen `permissions` como arreglo | JSON crudo | httpapi |
| Ninguna respuesta 400, 409 ni 422 que envuelva `ErrValidation` lleva «conflicto:» ni «datos inválidos:»; cada sentinel nuevo sale con su status y su texto | `respond_test.go` | httpapi |
| Pasar productos: sin comanda, existencias iguales, totales correctos | `TestMovedLinesKeepKitchenStateAndStock` | integración |
| Pasar piezas pagadas, con descuento, de/hacia plataforma, a un pedido cerrado, a otro turno o día, desde un turno cerrado, todo hacia un pedido nuevo | `TestMoveRejections` (tabla) | integración |
| El pedido nuevo nace en el turno y el día del origen, con su `opened_by`, sin pedir la caja abierta | `TestTheNewOrderInheritsTheOriginShift` | integración |
| Reintento de «Pasar» con la misma llave, con otro destino | `TestMoveIsIdempotentByBatch` | integración |
| Partir deja las dos mitades que suman exacto (4 decimales) | tabla de `SplitLine` | unitario |
| Pasar todo a un pedido existente: el origen se junta, sin reponer | `TestMovingEverythingMergesTheOriginWithoutRestock` | integración |
| El pedido juntado no es una cancelación en Ventas ni en las ventas del turno; lo quitado antes de juntar sí (hoy `SalesCancelledLines` no lo contaría) | `TestAMergedOrderIsNotACancellation` | integración |
| Quitar 1 de 2 y luego la otra mitad | `TestASplitLineRestocksEachHalfOnce` | integración |
| Refresco sin preparación se quita y vuelve al almacén | `TestRemovingAProductWithoutPrepRestocksIt` (sin desmarcar la cocina a mano) | integración |
| Quitar un renglón (repuesto **y** ya consumido) y luego cancelar el pedido | `TestCancellingAfterRemovingALineRestocksOnce`, con las dos formas | integración |
| Quitar un renglón de un pedido ya cobrado (FR-023, hoy no verificado) | `TestRemovingAPaidLineIsRejected`: primero se confirma si hoy pasa | integración |
| Entregar todo sin renglones vivos | `TestDeliverAllRejectsAnOrderWithoutProducts` | integración |
| Descuento con pagos hechos | `TestDiscountIsRejectedOncePaymentsExist` | integración |
| Número de pago estable con un devuelto en medio y un pago viejo sin número | `TestPaymentNumbersSurviveAVoid` | integración |
| Parte ya cobrada de la misma serie | `TestTheSameSplitPartCannotBeChargedTwice` | integración |
| Quitar lo que falta deja cerrado el pedido con lo entregado, partiendo el renglón con entrega parcial sin cambio neto en existencias | `TestCancelPendingClosesWithWhatWasDelivered` | integración |
| Ninguna secuencia de cobrar, quitar, pasar y devolver deja un pedido sin salida (SC-003) | `TestNoSequenceLeavesAnOrderWithoutAWayOut` | integración |
| `payments`, `lines` y `voidedPayments` nunca `null` | test sobre JSON crudo | httpapi |
| Aislamiento: consultas nuevas y modificadas (`RestockCancelledOrder`, `GetOrderLineForCancel`), y servicios nuevos (`CancelPending`, `VoidPayment`, `MoveLines`, `load` con pagos, vista del turno) | `inTheThreeCases` + FK compuesta entre empresas + `TestEveryCompanyTableIsIsolated` | integración |
| La migración con dos empresas, sobre un respaldo real restaurado | test de la migración de esta feature | integración |
| Lo pagado al final y en gris tras recargar; Devolver deshabilitado sin permiso; selector solo tras «Dividir» y nunca en plataforma ni turno cerrado; fichas con número, método y monto; «Todo lo que falta» manda `allRemaining`; confirmar destino; motivos sin preselección | `CobrarSheet.test.tsx`, `CancelarRenglonDialog.test.tsx` | vitest |
| Bote deshabilitado «Pagado» en lo pagado, con las piezas cubiertas que trae `BoardLine` | JSON crudo e `inTheThreeCases` de la consulta del tablero + `OrdersBoardPage.test.tsx` | integración + vitest |
| `Charge` y `CancelarRenglon` leen `order_payment_lines` | `inTheThreeCases` a nivel de servicio | integración |
| La hoja de quitar lo que falta: motivos sin preselección, no cierra antes de la respuesta | `CancelPendingSheet.test.tsx` | vitest |
| La tarjeta en las diez combinaciones (ocho de pendiente × debe × el tablero cobra, más sin productos vivos con y sin pagos), con el endpoint de cada «Cerrar pedido»; tarjeta de 11 productos con botones a la vista; Quitar lo que falta abre su hoja | `OrdersBoardPage.test.tsx` (nuevo) | vitest |
| El botón muestra el monto de `quote` (con debounce) y se refresca si `/pay` cobra otro | `CobrarSheet.test.tsx` | vitest |
| La hoja cabe en 600 px con efectivo, propina y la lista de productos, y con el teclado del sistema abierto «Cobrar» sigue alcanzable; `CancelPendingSheet` y la vista `MoveToOrder` también caben | E7 bis | e2e |
| El recorrido del incidente con la feature, ≤ 20 toques | `split-bill-incident.spec.ts` (reescrito desde `dividir-cuenta-incidente.spec.ts`) | e2e |

Las matrices [docs/matriz-de-cobro.md](../../docs/matriz-de-cobro.md) y
[docs/matriz-de-pantallas.md](../../docs/matriz-de-pantallas.md) se editan junto con estos tests.

## Orden de construcción

La rama de Uber (021) trae `inTheThreeCases`, `TestEveryCompanyTableIsIsolated` y las políticas con
`nullif` (D-11). Se construye ya lo que no toca tablas nuevas; las consultas existentes que se
modifican antes quedan cubiertas por la prueba de aislamiento tras el rebase. Lo demás espera a que 021 llegue a `develop` y se rebasa encima. Ningún test se marca `skip`.

**Sin esperar a 021**

1. Permisos (D-16): `domain.Permission`, `RequirePermission`, permisos en la sesión, `can()`.
2. `domain/split_bill.go` y partir renglón, con sus tablas.
3. Ningún pedido sin salida (US7): entregar todo sin productos, quitar lo que falta (con las
   consultas de partir un renglón), «Quitar lo que falta» y «Cancelar pedido» como ítems aparte,
   cada uno por su permiso, textos sin prefijo (D-10, D-18).
4. Reposición en sus dos formas (US8, D-8).

**Tras el rebase sobre 021**

5. `inTheThreeCases` de lo construido en 3 y 4 (`CancelPending`, las consultas de partir un
   renglón, `RestockCancelledOrder`, `GetOrderLineForCancel`).
6. Migración con su test sobre un respaldo real con dos empresas, consultas con su aislamiento,
   `make sqlc`, pagos en la vista.
7. `Charge` con renglones y todo lo que falta, y `quote`; luego partes.
8. Quitar piezas (`qty`) y descuento con pagos (D-17).
9. Devolver un pago y la lista del corte.
10. Pasar productos y el pedido juntado en Ventas.
11. Front de cada paso junto con su backend; secuencias sin salida (SC-003), medición, E2E y matrices.

**La rama no se fusiona antes de terminar la Fase 8 de tasks.md (US3)**: las fases intermedias no
salen a producción por separado. Hasta ahí, un pago por productos podría convivir con quitar el
producto pagado o cambiar el descuento, que es justo lo que US3 cierra.

## Project Structure

### Documentation (this feature)

```text
specs/027-dividir-la-cuenta/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
├── checklists/requirements.md
└── tasks.md            # /speckit-tasks
```

### Source Code

```text
server/
├── migrations/NNNN_split_bill.sql
├── queries/{orders.sql,cash.sql,sales.sql}
└── internal/
    ├── domain/{split_bill.go,split_bill_test.go,renglon.go,renglon_test.go,entrega.go,
    │           errors.go,uso.go,permission.go,permission_test.go}
    ├── app/{orders.go,devolucion.go,move_lines.go,backoffice.go}
    ├── httpapi/{handlers_orders.go,router.go,middleware.go,middleware_test.go,
    │            handlers.go,respond.go,respond_test.go}
    └── integration/{split_by_products_test.go,split_parts_test.go,split_bill_isolation_test.go,
                     quote_test.go,void_payment_test.go,move_lines_test.go,restock_unconsumed_test.go,
                     no_way_out_test.go,migration_split_bill_test.go,permissions_http_test.go,
                     order_view_http_test.go}
web/src/
├── shared/CobrarSheet.tsx, shared/cobro/{ModePicker,ByProducts,EvenSplit,ByAmount,PaymentChips,
│                                         PaymentDetail,MoveToOrder}.tsx, shared/cobro/listOrder.ts
├── components/{ListRow.tsx,ListRow.test.tsx}
├── features/orders/{OrdersBoardPage.tsx,OrdersBoardPage.test.tsx,CancelarRenglonDialog.tsx,
│                    CancelPendingSheet.tsx,CancelPendingSheet.test.tsx}
├── features/pos/PedidosEnCurso.tsx, features/backoffice/CashPage.tsx
├── utils/printReceipt.ts, shared/tickets/AutoPrintTicket.tsx
├── types/pos.ts, api/pos.ts, stores/session.ts, app/permissions.ts
web/e2e/{split-bill-incident.spec.ts,cabe-en-la-tableta.spec.ts}
```

`three_cases_test.go` y `rls_all_tables_test.go` llegan con 021 (D-11); esta rama no los copia.

**Structure Decision**: monorepo existente, sin paquetes nuevos.

## Revisión de arquitectura

`db-architect` y `tablet-ui-reviewer` pidieron cambios el 2026-10-05 (3 + 3 bloqueantes). Todos
están incorporados en research.md (marcados **[rev]**), data-model.md, contracts/api.md y en las
filas de bordes de arriba. El dueño agregó en la misma revisión la decisión de roles configurables
(D-16) y su puerta en la constitución.

`/speckit-analyze` (2026-10-05) encontró 4 críticos, 9 altos, 10 medios y 10 bajos. El dueño aprobó
las recomendaciones y tres decisiones: pasar todos los productos se rechaza hacia un pedido nuevo y
junta el origen con uno existente (D-5); lo que toca tablas nuevas espera a 021 (D-11); la enmienda
de la constitución va en su propio commit, fuera de estas tareas.

## Complexity Tracking

| Excepción | Por qué se necesita | Alternativa más simple descartada |
|---|---|---|
| **Principio IV, test primero**: el aislamiento (`inTheThreeCases`) de lo construido antes del rebase —`CancelPending`, las consultas de partir un renglón (T021), `RestockCancelledOrder`, `GetOrderLineForCancel`— se escribe **después** de su código (T032). Aprobada por el dueño el 2026-10-05, al elegir la opción (a) de D-11 | El helper `inTheThreeCases` y `TestEveryCompanyTableIsIsolated` llegan con la rama de Uber (021). US7 y US8 corrigen defectos que ya costaron un incidente y no tienen por qué esperarla | Copiar el helper byte por byte a esta rama: dos ramas manteniendo el mismo archivo. Esperar a 021 para todo: deja sin arreglo el pedido sin salida y la reposición. Mitigación: T032 obliga a ver cada prueba en rojo rompiendo a propósito la política o el `AcquireTenant` |

## Riesgos que el plan acepta

- **Partir renglones** es la pieza con más casos (modificadores, movimientos de stock, redondeo a
  4 decimales). Por eso D-7 rechaza en lugar de adivinar en el caso mixto.
- **La hoja de cobro crece**: tres modos, sub-hojas y fichas en un componente que ya tiene 677
  líneas. Se parte en componentes desde el primer commit, y E7 bis vigila el alto.
- **Los hallazgos fuera de alcance** de research.md (devoluciones que no restan en reportes) siguen
  ahí. Esta feature no los empeora: un pago devuelto sale de `order_payments` y no pasa por esos
  reportes.
- **El pedido juntado no aparece en Ventas**: lista, conteo y resumen excluyen el mismo pedido para
  no divergir (principio III), así que el folio del origen no se ve en esa pantalla; su rastro queda
  en el lote de «Pasar» y en `merged_into_order_id` (D-5).
- **Esperar a 021** deja el aislamiento de US7 y US8 probado después de su código (D-11). Se acepta
  porque la alternativa era copiar el helper y que dos ramas lo mantuvieran.
