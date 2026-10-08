# Contratos HTTP

Todo bajo `/api/v1`, con `RequireAuth`. Los errores salen **solo** por `httpapi.Error`. La columna
«Texto» es lo que ve quien opera: ningún texto lleva el prefijo del sentinel («conflicto:», «datos
inválidos:»; se quita en `httpapi.Error` para todas las respuestas, D-10), dice
«producto» y no «renglón», y no nombra roles (D-16, D-18). Toda fila tiene texto: ninguna cae al
mensaje genérico.

## Cambia: la sesión (`POST /auth/login`, `POST /auth/refresh`, `POST /auth/pin-switch`) y `GET /auth/me`

El usuario gana `permissions: string[]` (siempre arreglo). Login, refresh y pin-switch lo entregan
por `writeSession`; `/auth/me` lo agrega a su respuesta. Hoy lo resuelve
`domain.PermissionsFor(role)`; mañana sale de los roles de la empresa. La pantalla pregunta por
permiso (`can()`), nunca por nombre de rol.

Un 403 de `RequirePermission` lleva el texto de `domain.PermissionDeniedMessage(p)`:

| Permiso | Hoy lo tienen | Texto sin permiso |
|---|---|---|
| `payments.void` | admin, gerente | «Tu usuario no puede devolver pagos» |
| `orders.cancel` | admin, gerente | «Tu usuario no puede cancelar pedidos» |
| `orders.move_lines` | los cuatro roles | «Tu usuario no puede pasar productos» |
| `orders.cancel_pending` | los cuatro roles | «Tu usuario no puede quitar productos» |

## Cambia: `GET /orders/{id}` → `OrderView.payments`

`payments` es un arreglo, **nunca** `null` (test sobre el JSON crudo).

```json
"payments": [
  {
    "id": 269, "number": 1, "voided": false,
    "methodId": 10, "methodName": "Tarjeta débito",
    "amount": "110.00", "tip": "0.00", "reference": "",
    "paidAt": "2026-10-04T21:23:28-06:00", "receivedBy": "carlos",
    "split": null,
    "lines": [ { "lineId": 704, "qty": "1.00", "amount": "110.00" } ]
  },
  { "id": 270, "number": 2, "voided": true, "voidedAt": "…", "voidReason": "…",
    "methodId": 9, "methodName": "Efectivo", "amount": "177.00", "tip": "0.00",
    "paidAt": "…", "receivedBy": "carlos", "split": { "part": 1, "of": 3 }, "lines": [] }
]
```

- `number` es estable: un pago devuelto conserva el suyo y aparece con `voided: true` (sale de la
  bitácora). La pantalla lo pinta tachado.
- `lines` también es arreglo siempre.
- `OrderView` gana además `mergedIntoOrderId: number | null`: el pedido con el que se juntó (D-5).

## Cambia: la respuesta del tablero (`OrdersService.Board`)

Cada `BoardLine` gana `paidQty`: cuántas de sus piezas cubren pagos vivos. Siempre presente (0 sin
pagos). La tarjeta deshabilita el bote con «Pagado» cuando `paidQty` > 0.

## Cambia: `POST /orders/{id}/pay`

Mismo endpoint y mismo gate. El cuerpo gana tres formas que se excluyen entre sí y con `amount`:

```json
{ "methodId": 10, "clientUuid": "…", "tip": "0", "lines": [ { "lineId": 704, "qty": "1" } ] }
{ "methodId": 9,  "clientUuid": "…", "split": { "part": 2, "of": 3 } }
{ "methodId": 9,  "clientUuid": "…", "allRemaining": true }
```

- Con `lines`, el monto lo calcula el servidor (`domain.SelectionAmount`). Si además viene
  `amount`, responde 400 para que nadie crea que se respetó.
- Con `split`, el servidor calcula la parte sobre lo que falta (`domain.SplitPartAmount`), y la
  última absorbe el residuo.
- `allRemaining` cubre todas las piezas aún no cubiertas y cobra exactamente lo que falta. Si ya no
  queda pieza sin cubrir y el saldo es positivo, cobra el saldo sin cobertura (no es selección vacía).
- Cuando la selección cubre todo lo pendiente y cobra menos que su bruto descontado (hubo pagos por
  monto), el monto de cada renglón en la cobertura se prorratea (D-3).
- La idempotencia compara también `lines` y `split`: la misma llave con otra selección da 409.
- **El monto se recalcula siempre**: lo que haya mostrado `quote` no se manda ni se respeta. Si la
  cotización quedó vieja (otra tableta cobró o agregó algo entre medias), `/pay` cobra lo que
  calcula en ese momento y lo devuelve; la pantalla refresca el botón y «Falta» con la respuesta.

Respuesta: la de hoy más `paymentId`, `number` (D-12: vivos + devueltos del pedido, contando los
viejos sin número, más 1) y `amount`, el monto que se cobró.

| Caso | Error | Texto |
|---|---|---|
| Pieza ya cubierta | `ErrConflict` | «Ese producto ya se pagó» |
| Parte ya cobrada de esa serie | `ErrConflict` | «Esa parte ya se cobró» |
| Selección vacía, `qty` ≤ 0 o mayor que la pendiente | `ErrValidation` | «Elige qué productos paga» |
| Dos o más de `lines`, `split`, `allRemaining`, `amount` | `ErrValidation` | «Elige una sola forma de cobrar» |
| `lines`, `split` o `allRemaining` en un pedido de plataforma | `ErrConflict` | «Los pedidos de plataforma no se dividen» |
| `lines`, `split` o `allRemaining` en un pedido de un turno cerrado | `ErrConflict` | «Ese pedido es de un turno cerrado; no se divide» |
| La misma llave con otra selección o parte | `ErrConflict` | «Ese cobro ya se hizo con otros productos. Vuelve a intentarlo» |
| Renglón de otro pedido o cancelado | `ErrNotFound` | «Ese producto ya no está en el pedido» |
| El monto pasa de lo que falta | `ErrCobroExcede` | «Ya se cobraron $X sin elegir productos. Esta selección pasa de lo que falta: usa «Todo lo que falta»» |
| `client_uuid` de un pago devuelto | `ErrConflict` | «Ese pago ya se devolvió. Vuelve a cobrar» |

## Nuevo: `POST /orders/{id}/quote` — el monto antes de cobrar

**Solo lectura.** Mismo cuerpo que `/pay` con `lines`, `split` o `allRemaining` (excluyentes), sin
`methodId`, `clientUuid`, `tip` ni `amount`. **Gate**: el mismo que `/pay`. No escribe nada: lee bajo
un `SELECT` sin `FOR UPDATE` y calcula con la misma función de `domain` que `/pay`
(`SelectionAmount` / `SplitPartAmount`).

```json
{ "lines": [ { "lineId": 704, "qty": "1" } ] }
```

Respuesta:

```json
{ "amount": "110.00",
  "lines": [ { "lineId": 704, "qty": "1.00", "amount": "110.00" } ],
  "outstandingAfter": "797.00" }
```

`lines` es arreglo siempre (vacío con `split` o con `allRemaining` sin piezas por cubrir). La hoja la
llama al cambiar la selección o las partes, con debounce, y el botón muestra `amount`. Rechazos: los
mismos de `/pay` salvo los de idempotencia (misma llave con otra selección, llave de un pago
devuelto). Es una cotización, no una reserva: `/pay` recalcula siempre.

## Nuevo: `POST /orders/{id}/payments/{paymentId}/void`

**Gate**: `RequirePermission(payments.void)`, que hoy tienen admin y gerente, y tope por usuario
(`rateLimitUser`). Emite el evento de
seguridad `SecurityEvent "order_payment_voided"` con `order_id`, `payment_id` y `user_id`, sin
montos ni PII. Toma el `FOR UPDATE` del pedido antes de leer el pago (D-2).

```json
{ "reason": "Se le cobró a otra persona" }
```

Respuesta: `{ "outstanding": "266.00", "paid": false }`. Publica `order.updated`. Un pago por monto
no tenía cobertura y no deja productos por cobrar; solo vuelve a deberse su monto.

| Caso | Error | Texto |
|---|---|---|
| Pago de un turno cerrado o sin turno | `ErrConflict` | «Ese pago es de un turno cerrado: devuélvelo desde Pedidos entregados» |
| Sin permiso | 403 | ver la tabla de permisos |
| Pago ya devuelto | `ErrConflict` | «Ese pago ya se devolvió» |
| Pedido cancelado o reembolsado | `ErrConflict` | «Ese pedido ya se cerró; no se le pueden devolver pagos» |
| Motivo vacío | `ErrValidation` | «Elige por qué se devuelve» |
| Pago de otro pedido | `ErrNotFound` | «Ese pago no es de este pedido» |

## Nuevo: `POST /orders/{id}/lines/move`

**Gate**: `RequirePermission(orders.move_lines)`, hoy para todos. No mueve dinero: rechaza piezas
pagadas y dejar el origen sobrepagado. Tope por usuario (`rateLimitUser`). Idempotente por lote
(D-12b).

```json
{ "clientUuid": "…", "toOrderId": 291,
  "lines": [ { "lineId": 698, "qty": "1" }, { "lineId": 705, "qty": "1" } ] }
```

`toOrderId: null` crea un pedido nuevo en el turno y el día del origen, con el `opened_by` del
origen (quien capturó esos productos); quien los pasó queda en `moved_by` del lote. Respuesta:
`{ "from": OrderView, "to": OrderView }`. Publica `order.updated` para el origen y
`order.created`/`order.updated` para el destino.

**Pasar todos los productos** (D-5): hacia un pedido nuevo se rechaza; hacia uno existente, el origen
queda `cancelada` con el motivo «Se juntó con otro pedido» y `mergedIntoOrderId` = destino, sin
reponer inventario. `from` lo trae así. Ni Ventas ni las ventas del turno del corte lo cuentan
ni lo listan como cancelación; lo que se quitó de él antes de juntarlo cuenta como producto
cancelado porque `SalesCancelledLines` y su gemela agregan `or o.merged_into_order_id is not null`
a su filtro de estado (hoy no lo contarían). Ese reporte es un total; no lista folios.

| Caso | Error | Texto |
|---|---|---|
| Piezas pagadas | `ErrConflict` | «Ese producto ya se pagó; no se puede pasar» |
| Plataforma (origen o destino) | `ErrConflict` | «Los pedidos de plataforma no se dividen» |
| Origen de un turno cerrado | `ErrConflict` | «Ese pedido es de un turno cerrado; no se puede pasar» |
| Todos los productos hacia un pedido nuevo | `ErrConflict` | «Ya es su propio pedido; no hace falta pasarlo» |
| Destino cerrado, cancelado o igual al origen | `ErrConflict` | «Ese pedido ya no recibe productos» |
| Destino de otro turno o de otro día | `ErrConflict` | «Ese pedido es de otro turno» |
| Con descuento | `ErrConflict` | «Quita el descuento antes de pasar productos» |
| Pasar todo hacia un pedido existente cuando el origen cobra envío | `ErrConflict` | «Ese pedido tiene envío; cóbralo o quítalo antes de juntarlo» |
| Producto con devolución | `ErrConflict` | «Ese producto tiene una devolución; no se puede pasar» |
| Producto con receta sin consumo atribuible (anterior a 0060) | `ErrConflict` | «Ese producto es de un pedido viejo; no se puede pasar» |
| El origen quedaría con más pagado que su total | `ErrOrderWouldBeOverpaid` | «Ya se cobró más de lo que quedaría. Primero hay que devolver un pago» |
| Piezas mixtas entregadas y pendientes de más | `ErrConflict` | «Ese producto tiene piezas entregadas y otras sin entregar. Pásalas todas juntas» |
| Misma llave con otro origen o destino | `ErrConflict` | «Esto ya se pasó a otro pedido» |
| Sin permiso | 403 | ver la tabla de permisos |

## Cambia: `POST /orders/{id}/cancel`

El gate pasa de `RequireRole(admin, gerente)` a `RequirePermission(orders.cancel)`: mismos roles hoy,
y el texto sin permiso de la tabla. La tarjeta del tablero ofrece «Cancelar pedido» solo con
`can('orders.cancel')`.

## Cambia: `POST /orders/{id}/lines/{lineId}/cancel`

El cuerpo gana `qty` (opcional; por omisión, todas las piezas pendientes). Respuesta igual,
`{ "repusoInventario": bool }`.

| Caso | Error | Texto |
|---|---|---|
| Piezas pagadas | `ErrConflict` | «Ese producto ya se pagó. Primero hay que devolver el pago» |
| El total quedaría bajo lo pagado | `ErrConflict` | «Ya se cobró más de lo que quedaría. Primero hay que devolver un pago» |
| `qty` mayor que lo pendiente | `ErrValidation` | «No hay tantas piezas por quitar» |

## Nuevo: `POST /orders/{id}/lines/cancel-pending`

Quitar lo que falta por entregar, en una transacción. Gate: `RequirePermission(orders.cancel_pending)`,
hoy para todos (igual que quitar un producto).

```json
{ "reason": "Ya no lo quiere" }
```

`reason` es obligatorio cuando hay productos pendientes. Es **opcional y se ignora** cuando el pedido
no tiene productos vivos: el motivo fijo es «Sin productos», y «Cerrar pedido» lo manda sin cuerpo.

Respuesta: `{ "removed": 10, "restocked": 3 }` (cuántos productos se quitaron y cuántos repusieron
inventario). **Mismos rechazos que quitar un producto**, evaluados sobre el conjunto y con la misma
función de validación que `CancelarRenglon`. Sin permiso: 403 con el texto de la tabla.

| Caso | Error | Texto |
|---|---|---|
| Alguna pieza pendiente está cubierta por un pago | `ErrPieceAlreadyPaid` | «Ese producto ya se pagó. Primero hay que devolver el pago» |
| El total quedaría bajo lo pagado | `ErrOrderWouldBeOverpaid` | «Ya se cobró más de lo que quedaría. Primero hay que devolver un pago» |
| Motivo vacío con productos pendientes | `ErrValidation` | «Elige por qué se quitan» |

| Estado del pedido | Resultado |
|---|---|
| Con productos pendientes de entregar | Los quita; cierra el pedido si lo vivo queda entregado y pagado |
| Con productos vivos, todos entregados | `ErrConflict` «Ya no falta nada por entregar» |
| **Sin productos vivos y sin pagos** | Lo cancela: `status = cancelada`, motivo fijo «Sin productos», sin reponer inventario (cada renglón ya se resolvió al quitarlo). Responde `{ "removed": 0, "restocked": 0 }`. **Es una cancelación verdadera y sí cuenta en los reportes** |
| Sin productos vivos y con pagos | `ErrOrderHasPayments` «Tiene pagos: hay que devolverlos primero» (devolverlos pide `payments.void`) |

## Qué llama cada «Cerrar pedido» de la tarjeta

| Estado | Endpoint |
|---|---|
| Lo vivo entregado y sin deuda | `POST /orders/{id}/deliver` |
| Sin productos vivos y sin pagos | `POST /orders/{id}/lines/cancel-pending` |

## Cambia: `POST /orders/{id}/deliver`

Pedido sin productos vivos: `ErrNoProducts`, «Este pedido ya no tiene productos: ciérralo». La
tarjeta no ofrece «Entregar todo» en ese estado; ofrece «Cerrar pedido» (tabla de arriba).

## Cambia: `PUT /orders/{id}/discount`

Pedido con pagos: `ErrConflict`, «Ya hay pagos; el descuento se pone antes de cobrar» (D-17).

## Cambia: corte de caja (la vista del turno)

Gana `voidedPayments: [{ method, amount, tip, orderFolio, voidedBy, voidedAt, reason }]`, siempre
arreglo. El esperado por método **no** cambia de fórmula: el pago devuelto ya no está en
`order_payments`.

## Sentinels nuevos

Nombres en inglés (constitución VII). Cada uno envuelve a un sentinel base con `%w`, que decide el
status en `httpapi.Error`; el texto es el que ve quien opera, sin el prefijo del base (D-10).

| Sentinel | Envuelve | Status | Texto |
|---|---|---|---|
| `ErrPieceAlreadyPaid` | `ErrConflict` | 409 | «Ese producto ya se pagó» (al pasar: «…; no se puede pasar»; al quitar: «… Primero hay que devolver el pago») |
| `ErrSplitPartAlreadyCharged` | `ErrConflict` | 409 | «Esa parte ya se cobró» |
| `ErrChargeKeyMismatch` | `ErrConflict` | 409 | «Ese cobro ya se hizo con otros productos. Vuelve a intentarlo» |
| `ErrPaymentVoidedKey` | `ErrConflict` | 409 | «Ese pago ya se devolvió. Vuelve a cobrar» |
| `ErrPaymentAlreadyVoided` | `ErrConflict` | 409 | «Ese pago ya se devolvió» |
| `ErrPaymentFromClosedShift` | `ErrConflict` | 409 | «Ese pago es de un turno cerrado: devuélvelo desde Pedidos entregados» |
| `ErrOrderFromClosedShift` | `ErrConflict` | 409 | «Ese pedido es de un turno cerrado; no se divide» / «…; no se puede pasar» |
| `ErrPlatformOrderNotSplittable` | `ErrConflict` | 409 | «Los pedidos de plataforma no se dividen» |
| `ErrOrderWouldBeOverpaid` | `ErrConflict` | 409 | «Ya se cobró más de lo que quedaría. Primero hay que devolver un pago» (al quitar y al pasar) |
| `ErrMixedDeliveredPieces` | `ErrConflict` | 409 | «Ese producto tiene piezas entregadas y otras sin entregar. Pásalas todas juntas» |
| `ErrAlreadyItsOwnOrder` | `ErrConflict` | 409 | «Ya es su propio pedido; no hace falta pasarlo» |
| `ErrMoveKeyMismatch` | `ErrConflict` | 409 | «Esto ya se pasó a otro pedido» |
| `ErrOrderHasPayments` | `ErrConflict` | 409 | «Tiene pagos: hay que devolverlos primero» |
| `ErrNoProducts` | `ErrConflict` | 409 | «Este pedido ya no tiene productos: ciérralo» |
| `ErrDiscountWithPayments` | `ErrConflict` | 409 | «Ya hay pagos; el descuento se pone antes de cobrar» |
| `ErrOneChargeShape` | `ErrValidation` | 400 | «Elige una sola forma de cobrar» |
| `ErrEmptySelection` | `ErrValidation` | 400 | «Elige qué productos paga» |
| `ErrTooManyPieces` | `ErrValidation` | 400 | «No hay tantas piezas por quitar» |

Los demás textos de las tablas de arriba reusan sentinels existentes (`ErrNotFound`,
`ErrCobroExcede`, `ErrValidation`) con su texto propio.
