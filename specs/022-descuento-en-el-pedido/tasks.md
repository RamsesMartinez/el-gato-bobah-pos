---

description: "Tareas de la feature 022 — el descuento que se le hace a un pedido"
---

# Tasks: El descuento que se le hace a un pedido

**Input**: documentos de diseño en `specs/022-descuento-en-el-pedido/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md)

**Estado**: implementado y con los gates en verde el 2026-09-19. Lo que quedó fuera está marcado
abajo y dicho en el reporte: **T031** (pantalla para corregir el descuento de un pedido ya creado) y
**T034** (la verificación con el teclado de la tableta, que ningún test puede simular).

**Tests**: obligatorios y **primero**. El principio IV de la constitución no es negociable y la
migración además tiene un hook de pre-commit que la rechaza sin su test
([migracion-con-test.sh](../../scripts/hooks/migracion-con-test.sh)). Cada test se ve **en rojo**
por la razón correcta antes de escribir el código que lo pasa.

**Dónde se trabaja**: worktree `/home/ramy/git/egb-022-descuentos`, rama
`022-descuento-en-el-pedido`. El directorio principal del repo está en otra feature y no se toca.

---

## Phase 1: Setup

- [x] T001 Confirmar el punto de partida en verde: `cd server && go build ./... && go test ./...` y `cd web && bun run lint` en /home/ramy/git/egb-022-descuentos

---

## Phase 2: Foundational — bloquea todas las historias

- [x] T002 Escribir el test de la migración (en rojo) en server/internal/integration/descuento_test.go: sobre base restaurada y **dos empresas**, los pedidos existentes quedan con `discount_total = 0` y rastro nulo, y las columnas nuevas se pueden escribir **bajo `appRoleStore`** (rol `gatobobah_app`), no solo como owner
- [x] T003 Escribir la migración server/migrations/0072_descuento_del_pedido.sql con `set local lock_timeout = '3s'`, las dos columnas de rastro y los cuatro checks `not valid` + su `validate constraint` aparte, según [data-model.md](./data-model.md)
- [x] T004 [P] Agregar el sentinel `ErrDescuentoMayorQueLaVenta` en server/internal/domain/errors.go y su mapeo a **422** en server/internal/httpapi/respond.go (único lugar donde un error se vuelve HTTP)
- [x] T005 Cambiar `RecalcOrderTotals` a `total = greatest(subtotal − o.discount_total, 0) + o.delivery_fee` en server/queries/orders.sql, y sumar `discount_total`, `discount_set_by`, `discount_set_at` a las columnas del `insert` de `CreateOrder` (hoy no las nombra y se apoya en el default)
- [x] T006 Correr `make sqlc` y verificar que server/internal/store/db/ quedó regenerado (nunca se edita a mano)

**Checkpoint**: el esquema y el recálculo ya soportan descuento aunque nadie lo capture todavía.

---

## Phase 3: User Story 1 — Capturar el descuento con el pedido (P1) 🎯 MVP

**Meta**: un pedido con promoción se manda con su descuento y el total ya rebajado.

**Prueba independiente**: capturar $385 con $50 de descuento, mandar y cobrar con el método de su
plataforma; la venta del día sube $335.

### Dominio (puro, primero el test)

- [x] T007 [US1] Escribir server/internal/domain/descuento_test.go, table-driven, contra los bordes: monto y porcentaje juntos → `ErrValidation`; porcentaje fuera de [0,100], NaN, ±Inf → `ErrValidation`; monto > subtotal → `ErrDescuentoMayorQueLaVenta`; 33 % de $100.05 → $33.02; descuento + envío = $385 − $50 + $35 = $370; sin descuento el total no cambia
- [x] T008 [US1] Implementar `ResolverDescuento` y `AplicarDescuento` en server/internal/domain/descuento.go, con `Round2` y `ValidMoney` en la frontera
- [x] T009 [US1] Corregir `ApplyDeliveryFee` en server/internal/domain/order.go para sumar el envío **sobre el total ya descontado** (hoy escribe `Total = Subtotal + fee` y borraría el descuento), y actualizar su comentario, que deja de ser cierto

### Servicio y frontera

- [x] T010 [US1] Aceptar `discountAmount` / `discountPercent` (exclusivos) en `CreateOrderCmd` y llamar a `AplicarDescuento` entre `BuildOrder` y `ApplyDeliveryFee` en server/internal/app/orders.go, persistiendo monto + autor + `now()`
- [x] T011 [US1] Decodificar los dos campos en el body de creación en server/internal/httpapi/handlers_orders.go — handler fino: sin aritmética de dinero
- [x] T012 [US1] Agregar `discount` (siempre presente, "0.00" cuando no hubo) a `OrderView` en server/internal/app/orders.go, según [contracts/api.md](./contracts/api.md)
- [x] T013 [US1] Test de integración en server/internal/integration/descuento_test.go: crear un pedido con porcentaje y verificar que el servidor guardó **pesos** resueltos contra SU subtotal, ignorando cualquier total que mande el cliente

### Pantalla (primero el test)

- [x] T014 [P] [US1] Escribir web/src/domain/descuento.test.ts: campo vacío → sin descuento (`paraElServidor: undefined`, no `0`); texto ilegible → `malEscrito` que bloquea el envío; monto > subtotal → `excede`; porcentaje resuelto a pesos con `round2`
- [x] T015 [US1] Implementar web/src/domain/descuento.ts siguiendo el patrón de web/src/domain/envio.ts (una sola fuente del monto por lado)
- [x] T016 [US1] Agregar `descuento: string` y `descuentoModo: 'monto' | 'pct'` a `TicketTab` en web/src/types/pos.ts y a la cuenta en web/src/stores/ticket.ts, con **guarda de hidratación**: una cuenta guardada antes del deploy llega sin esos campos y se trata como sin descuento
- [x] T017 [US1] Test en web/src/stores/ticket.test.ts de esa hidratación vieja (es el defecto que rompería la primera cuenta abierta tras el deploy)
- [x] T018 [US1] Poner el acceso al descuento **dentro de la fila del `Total`** de web/src/features/pos/Ticket.tsx (la fila pasa de 40 a 44 px), desplegando campo + selector `$`/`%` de dos botones tipo segmento — nunca un `<select>` nativo
- [x] T019 [US1] `scrollIntoView` al enfocar el campo y `maxH` en **dvh** para la zona de totales de web/src/features/pos/Ticket.tsx, con test en web/src/features/pos/Ticket.test.tsx de que el contenedor tiene alto acotado
- [x] T020 [US1] Hacer que las **tres** superficies que pintan total — panel, píldora y barra angosta de web/src/features/pos/POSPage.tsx — lean el mismo cálculo de `descuento.ts`, con test en web/src/features/pos/POSPage.test.tsx de que muestran la misma cifra
- [x] T021 [US1] Mandar `discountAmount`/`discountPercent` desde web/src/domain/pedido.ts y web/src/api/pos.ts (`CreateOrderBody`), y bloquear el envío con un descuento mal escrito o que exceda

### Cobertura que el análisis encontró faltante

- [x] T037 [US1] Test de integración en server/internal/integration/descuento_test.go del **corte de caja y el resumen de ventas** con un pedido con descuento: el ingreso es el total ya rebajado y el descuento **no se cuenta dos veces**. Falla nombrando el concepto duplicado, como `TestElFondoDeCajaSeCuentaUnaSolaVez` — lo exige el principio III, y sin él SC-003 no tiene quién lo pruebe
- [x] T038 [P] [US1] Test en web/src/features/pos/Ticket.test.tsx de que el acceso al descuento existe en los **cuatro** tipos de pedido (mostrador, para llevar, domicilio propio y plataforma): FR-011 se cumple hoy por omisión, y una condición agregada después lo rompería sin que nada falle

**Checkpoint**: la US1 entrega valor sola. Si nada más se construye, el POS ya cuadra con lo que la
plataforma cobró.

---

## Phase 4: User Story 2 — El descuento se ve donde se lee el dinero (P1)

**Meta**: el renglón del descuento existe en el papel y en el detalle de la venta.

**Prueba independiente**: imprimir el ticket de un pedido con descuento; subtotal − descuento +
envío = total.

- [x] T022 [P] [US2] Escribir en web/src/utils/printReceipt.test.ts los dos casos, sobre el **HTML crudo**: con descuento salen los tres renglones y cierran; sin descuento **no** aparece un renglón en $0.00
- [x] T023 [US2] Pintar el renglón de descuento en web/src/utils/printReceipt.ts y cambiar la condición del desglose, que hoy solo mira `deliveryFee > 0`
- [x] T024 [P] [US2] Llevar el descuento a la pre-cuenta en web/src/features/pos/preCuenta.ts (hoy deriva `subtotal = total − envio`, que con descuento deja de ser cierto) y su caso en web/src/features/pos/preCuenta.test.ts
- [x] T025 [P] [US2] Mostrar el descuento en el detalle de la venta en web/src/features/sales/SaleDetailDialog.tsx, con el mismo patrón condicional del renglón de Envío, y el campo en web/src/api/sales.ts
- [x] T026 [US2] Verificar que la hoja de cobro (web/src/shared/CobrarSheet.tsx) ofrece cobrar el total rebajado: lee `total`/`outstanding` del servidor, así que el test es que **no** recalcula por su cuenta

---

## Phase 5: User Story 3 — Cambiar o quitar el descuento antes de cobrar (P2)

**Meta**: corregir un descuento mal tecleado sin cancelar el pedido.

**Prueba independiente**: crear con $50, cambiar a $30, cobrar; el detalle muestra $30 y quién lo
aplicó.

- [x] T027 [US3] Escribir los tests de integración en server/internal/integration/descuento_test.go: cambiar el descuento de un pedido abierto recalcula el total; un pedido **cobrado por completo** lo rechaza sin mover el total; cancelar una línea deja el total en cero y nunca negativo; agregar líneas **no** recalcula el descuento
- [x] T028 [US3] Escribir la query `SetOrderDiscount` en server/queries/orders.sql **con `for update`** sobre el pedido antes de leer subtotal y escribir (mismo motivo que `GetOrderForUpdate`), que limpia el rastro a NULL cuando el descuento queda en cero
- [x] T029 [US3] Implementar `OrdersService.SetDiscount` en server/internal/app/orders.go, dentro de `WithTx`, rechazando pedido cobrado/cancelado/reembolsado con `ErrConflict`
- [x] T030 [US3] Handler `PUT /orders/{id}/discount` en server/internal/httpapi/handlers_orders.go y su ruta con `RequireAuth` en server/internal/httpapi/router.go
- [x] T039 [US3] Test en server/internal/integration/descuento_test.go de que el endpoint nuevo **no exige un rol distinto** del que ya captura pedidos (FR-012): la ausencia de barrera es una decisión, y una decisión sin test se revierte sola en la siguiente refactorización
- [ ] T031 [US3] **NO HECHA** — Permitir editar el descuento de un pedido ya creado desde la pantalla. El endpoint (`PUT /orders/{id}/discount`) está escrito y probado; lo que falta es la pantalla. Se dejó fuera de esta entrega a propósito: el único lugar natural es la hoja de cobro, que es por donde pasa todo el dinero, y tocarla para un deploy urgente arriesga el camino de cobro por una corrección que hoy se resuelve antes de mandar el pedido

---

## Phase 6: Pulido y verificación

- [x] T032 Gates completos: `cd server && go build ./... && go test ./...`; `cd web && bun run lint && bun run vitest run && bun run build`
- [x] T033 Correr los revisores de código (`/revision-de-codigo`) sobre el diff — es el hook `after_implement` y no es opcional
- [ ] T034 **PENDIENTE (requiere la tableta)** — Verificar a mano lo que ningún test cubre: el teclado numérico abierto sobre el campo de descuento y que COBRAR sigue alcanzable a 1024×600
- [x] T035 Anotar el caso nuevo en docs/matriz-de-cobro.md (por dónde se pierde dinero) — un descuento es dinero que sale del total
- [x] T036 Dejar dicho en el commit que la rama `021-recibir-pedidos-uber` tiene que renumerar su `0072_pedidos_de_plataforma.sql` a `0073` al rebasar sobre develop

---

## Dependencias

```text
Setup (T001)
  └── Foundational (T002–T006)  ← bloquea todo
        ├── US1 (T007–T021)     ← MVP
        ├── US2 (T022–T026)     ← necesita el campo `discount` de T012
        └── US3 (T027–T031)     ← necesita el esquema de T003
              └── Pulido (T032–T036)
```

- **US2 y US3 son independientes entre sí**: se pueden hacer en cualquier orden una vez que US1
  cerró.
- Las tareas marcadas `[P]` tocan archivos distintos y pueden ir en paralelo.

## Alcance mínimo entregable

**US1 sola** (T001–T021). Con eso un pedido de plataforma con promoción ya se captura con el total
correcto, que es lo que apura. US2 se hace en la misma tanda porque un total rebajado sin decir por
qué le llega al cliente en el papel.
