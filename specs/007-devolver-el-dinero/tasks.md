# Tareas — Devolver el dinero de una venta que no fue

**Spec**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md)

Cada tarea de implementación va precedida de su tarea de test. Un test que no se vio en rojo no
prueba nada.

## Decisiones que cerró el análisis

Salieron de `/speckit-analyze` como ambigüedades y se fijan aquí para que no se decidan a mitad del
código:

- **Un renglón se puede devolver varias veces**, y su tope es **lo cobrado de ESE renglón**, no lo
  del pedido. Sin esa cota, devolver tres veces un platillo de $60 en un pedido de $500 pasa.
- **Devolver por un método desactivado SÍ se permite.** Cobrar con uno inactivo se rechaza porque no
  debe entrar dinero nuevo por ahí; el que ya entró tiene que poder salir por donde entró, o queda
  atrapado.
- **"Devolución"** es el acto en pantalla; **`refund`** es su nombre en el código y en la base. No se
  mezclan.

## Fase 1 — Dominio (sin base de datos)

- [X] T001 [P] [US2] Test de `MontoDevolvible` y `ValidarDevolucion` en `server/internal/domain/devolucion_test.go`: no se devuelve más de lo cobrado, ni dos veces lo mismo, ni cero, ni un pedido sin cobros
- [X] T002 [US2] Implementar `MontoDevolvible` y `ValidarDevolucion` en `server/internal/domain/devolucion.go`
- [X] T002b [P] [US2] Test: el tope de un renglón es **lo cobrado de ese renglón**, y devolverlo dos veces no pasa de ahí
- [X] T003 [P] [US2] Test de `RepartirDevolucion` en `server/internal/domain/devolucion_test.go`: reparte por método en el orden en que entró, nunca excede lo que entró por cada uno, y el residuo cae en el último
- [X] T004 [US2] Implementar `RepartirDevolucion` en `server/internal/domain/devolucion.go`
- [X] T004b [P] [US2] Test: se devuelve por un método **desactivado** si por ahí entró el dinero — cobrar con él se rechaza, devolver no
- [X] T005 [P] [US3] Test de `ReponeInventario` y `PuedeCancelarRenglon` en `server/internal/domain/renglon_test.go`: repone solo si no salió a cocina; un renglón ya entregado no se cancela
- [X] T006 [US3] Implementar `ReponeInventario` y `PuedeCancelarRenglon` en `server/internal/domain/renglon.go`
- [X] T007 Sentinels `ErrDevolucionExcede`, `ErrSinCobrosQueDevolver`, `ErrRenglonYaEntregado` en `server/internal/domain/errors.go` y su mapeo en `server/internal/httpapi/respond.go`

## Fase 2 — Esquema

- [X] T008 [US2] Migración `server/migrations/0060_devoluciones.sql`: tabla `order_refunds`, columna `stock_movements.order_line_id`, grants al rol de app, Down reversible
- [X] T008b [US2] Test de integración (FR-009): un arqueo YA CERRADO da las mismas cifras antes y después de la migración
- [X] T009 [US2] Test de integración de la migración en `server/internal/integration/migracion_devoluciones_test.go`: corre sobre datos previos y con DOS empresas; verifica el grant al rol de app (sin él, producción da 42501 y dev nunca)

## Fase 3 — Consultas y servicio

- [X] T010 [P] [US2] Queries en `server/queries/orders.sql`: `SumOrderPaymentsByMethod`, `InsertOrderRefund`, `SumOrderRefunds`, `RecalcOrderRefundAmount`
- [X] T011 [P] [US3] Queries en `server/queries/orders.sql`: `CancelOrderLine`, `RestockCancelledLine`, `GetOrderLineForCancel`
- [X] T012 Regenerar con `make sqlc`
- [X] T013 [P] [US2] Test de integración: reembolsar un entregado y cobrado devuelve **lo cobrado**, no el total del pedido
- [X] T014 [P] [US2] Test de integración: reembolsar un entregado SIN cobrar se rechaza y no anota pérdida
- [X] T014b [P] [US2] Test de integración (A3): `orders.refund_amount` recalculado es la suma del libro, y `RefundsByDay` sigue leyendo lo mismo — si el recálculo se equivoca, ese reporte cambia en silencio
- [X] T015 [US2] `OrdersService.Refund` con monto, renglones y reparto por método en `server/internal/app/orders.go`
- [X] T016 [P] [US2] Test de integración: la devolución en efectivo deja **salida de caja** y el arqueo la descuenta; la de tarjeta NO toca el cajón
- [X] T017 [US2] Salida de caja dentro de la misma transacción de la devolución
- [ ] T017b [P] [US2] Test de integración (SC-002): el reporte de devoluciones del periodo y la suma de salidas de dinero **cuadran**
- [X] T018 [P] [US1] Test de integración: cancelar un pedido con cobros SIN devolución se rechaza; con devolución, el arqueo queda cuadrado
- [X] T019 [US1] `OrdersService.Cancel` exige y ejecuta la devolución en la misma transacción
- [X] T020 [P] [US3] Test de integración: cancelar un renglón NO enviado a cocina repone; uno ya enviado NO repone y el total baja igual
- [X] T021 [US3] `OrdersService.CancelLine` en `server/internal/app/orders.go`

## Fase 4 — Frontera

- [X] T022 [P] Test en `server/internal/httpapi/`: monto y motivo inválidos son 400, no 500
- [X] T023 `POST /orders/{id}/refund` acepta renglones y monto en `server/internal/httpapi/handlers_orders.go`
- [X] T024 `POST /orders/{id}/lines/{lineId}/cancel` en `server/internal/httpapi/handlers_orders.go`
- [ ] T024b [US3] (FR-008) Revisar `ErrCancelarConEntregas`: hoy manda a "cancela los que falten", que no existía. Con US3 se vuelve cierto — test que lo compruebe, no solo el mensaje reescrito
- [X] T025 [US1] `RequireRole` en `/orders/{id}/cancel` en `server/internal/httpapi/router.go` — hoy no lo tiene y mueve el mismo dinero que el reembolso

## Fase 5 — Pantalla

- [X] T026 [P] [US2] Test de `web/src/domain/devolucion.ts`: cuánto se puede devolver, qué se ofrece y por qué se apaga el botón
- [X] T027 [US2] `web/src/domain/devolucion.ts`, espejo de las reglas del servidor
- [X] T028 [P] [US2] Test de la hoja de devolución en `web/src/features/orders/`
- [X] T029 [US2] Hoja de devolución con selección por renglón, a 44 px y dentro de los 600 px
- [ ] T030 [P] [US3] Test: cancelar un renglón ya enviado a cocina **avisa** que el insumo no vuelve
- [ ] T031 [US3] Cancelar renglón desde la tarjeta del pedido, con su aviso
- [X] T032 [US1] La tarjeta deja de ofrecer "Reembolsar" en un pedido sin cobros (SC-003)

## Fase 6 — Cierre

- [X] T033 Renglones nuevos en `docs/matriz-de-pantallas.md`, cada uno con su test
- [X] T034 Cerrar X1, X2 y X3 en la tabla de pendientes de la matriz
- [ ] T035 Gates: `go build`, `go test`, integración, `golangci-lint`, `bun run lint`, vitest, `bun run build`, e2e

## Dependencias

Fase 1 no depende de nada. Fase 2 depende de T007 (los sentinels los usa el test de la migración).
Fase 3 depende de 1 y 2. Fase 4 de 3. Fase 5 de 4. Fase 6 al final.

**MVP**: US1 + US2 (el dinero). US3 (cancelar renglón) es el que cierra el mensaje de error que hoy
manda a una acción inexistente, y va en el mismo entregable porque sin él P4 sigue abierto.
