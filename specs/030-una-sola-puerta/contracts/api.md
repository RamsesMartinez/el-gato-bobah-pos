# Contrato de API: cuentas en captura y cuentas vivas

Base `/api/v1`. Todo bajo `RequireAuth` + `WithTenant` (RLS). **Sin `RequireRole`**: es el mismo
gate que `POST /orders` y `POST /orders/{id}/lines` hoy (quien levanta el pedido puede capturarlo).
Errores con el sobre de siempre (`{code, message}`) mapeados **solo** en `httpapi.Error`.

Este documento es la frontera entre el implementador de backend y el de frontend: si algo cambia
aquí, cambia en los dos lados en el mismo commit.

Convenciones:

- Dinero como **string** decimal (`"74.00"`), igual que `OrderView`.
- Todo arreglo viaja como `[]`, nunca `null` (AGENTS.md §1). Los tests van sobre el JSON crudo.
- `uuid` = uuid v4 en minúsculas que genera la tableta (`utils/uuid`).

## Tipos

### `DraftView`

```jsonc
{
  "id": "uuid",
  "orderId": 123,                // null = cuenta nueva; número = «Nuevo» de ese pedido
  "folioName": "Levkoy",         // null en una «Nuevo» (el nombre es el del pedido)
  "status": "capturando",        // capturando | enviada | descartada
  "headerVersion": 3,
  "updatedAt": "2026-10-08T18:02:11Z",
  "createdAt": "2026-10-08T17:55:00Z",
  "openedBy": "Ana",             // nombre de quien la abrió
  "serviceType": "mostrador",
  "customerName": null,
  "platformId": null,
  "platformOrderRef": null,
  "deliveryFee": "0.00",
  "discount": null,              // {"amount":"20.00"} | {"percent":"10"} | null
  "lines": [
    {
      "id": "uuid", "version": 2, "productId": 41, "productName": "Taro",
      "qty": "2", "unitPrice": "55.00",
      "modifiers": [{"optionId": 7, "name": "Perlas", "qty": 1, "priceDelta": "10.00", "portion": ""}],
      "notes": "", "lineTotal": "130.00",
      "available": true          // false = ya no está en el menú; no suma y bloquea enviar
    }
  ],
  "subtotal": "130.00", "discountTotal": "0.00", "total": "130.00",
  "unavailable": []              // nombres que ya no se venden; el front avisa antes de enviar
}
```

Los precios los calcula el servidor con la lista de la cuenta (mostrador o la plataforma) en **cada
lectura**: no se guardan (research R-16). Al enviar, `Create`/`AddLines` vuelven a calcular, como hoy.

### `AccountItem` (renglón de la fila)

```jsonc
{
  "key": "d:uuid" | "o:123",     // estable para la fila y la selección
  "kind": "draft" | "order",
  "draftId": "uuid",             // en kind=draft
  "orderId": 123,                // en kind=order
  "number": 17,                  // folio del turno; null en draft
  "folioName": "Levkoy",
  "state": "capturing",          // capturing | in_kitchen | paid_in_kitchen | partly_paid | delivered_owes
  "group": "capturing",          // capturing | in_kitchen | delivered_owes | previous_days
  "kitchenReady": false,         // pedido en `lista`
  "platformId": null,
  "serviceType": "mostrador",
  "customerName": null,
  "openedAt": "…", "updatedAt": "…",
  "businessDate": "2026-10-08",  // null en draft
  "total": "130.00", "paid": "0.00", "outstanding": "130.00",
  "lineCount": 2,
  "pendingDraftId": null,        // «Nuevo» viva del pedido
  "pendingCount": 0,             // renglones en esa «Nuevo»
  "closedWithPending": false     // pedido cerrado que conserva una «Nuevo» (research R-9)
}
```

`state` y `group` salen de `domain.AccountState`/`AccountGroup`; el front **no** los recalcula.

## Endpoints

### `GET /pos/accounts`

Corre el barrido (12 h y «Nuevo» de pedidos cancelados) y devuelve las cuentas vivas.

`200 {"items": AccountItem[], "outstanding": "430.00", "serverTime": "…"}` — `outstanding` suma el
`outstanding` de los `items` **de tipo `order`** (un predicado, constitución III). **Excluye las
cuentas en captura**: lo que se está capturando no es dinero que alguien deba todavía. En un `draft`,
`paid` = 0 y `outstanding` = su `total` (lo que costaría), y no entra a la suma. Orden: `openedAt`
ascendente.

Query: `olderDebts=true` (opcional; solo `true` o ausente, cualquier otro valor → `400`) agrega las
entregadas que deben de cualquier fecha. Lo piden la hoja «+N» al abrirse y el cierre de caja; la
fila cada 30 s no (research R-7).

Incluye: cuentas `capturando` sin pedido; pedidos `abierta|lista` de cualquier fecha; pedidos
`entregada` que deben de los últimos 90 días (todas con `olderDebts=true`); pedidos cerrados con una «Nuevo» viva
(`closedWithPending: true`). Excluye cancelada y reembolsada.

Reemplaza `GET /orders/open` (se elimina con `PedidosEnCurso`; `open` lo atrapa `GET /orders/{id}` y
responde `400`, nunca una lista).

Un pedido con `closedWithPending: true` (pagado y entregado, con una «Nuevo» viva) viaja con `state:
"paid_in_kitchen"` y `group: "in_kitchen"`: la ficha existe para que lo capturado no se pierda de vista,
y lo que la pantalla ofrece ahí lo decide `closedWithPending` (enviar responde `ORDER_CLOSED`).

### `POST /pos/drafts` — crear con el primer producto

```jsonc
{
  "id": "uuid",
  "orderId": null,               // o el pedido al que se le agrega («Nuevo»)
  "folioName": "Levkoy",         // propuesta; el servidor decide (research R-3). Ignorada con orderId
  "header": {                    // opcional; mismos campos que PATCH
    "serviceType": "mostrador", "customerName": null, "platformId": null,
    "platformOrderRef": null, "deliveryFee": "0.00", "discount": null
  },
  "lines": [{"opId": "uuid", "productId": 41, "qty": "1", "modifiers": [], "notes": ""}]  // ≥ 1
}
```

| Respuesta | Cuándo |
|---|---|
| `201 DraftView` | Creada |
| `200 DraftView` | Reintento con el mismo `id` (no reaplica `lines`: sus `opId` ya existen) |
| `200 DraftView` con **otro `id`** | `orderId` ya tenía una «Nuevo» viva: se le aplican los `lines` a ésa. El front adopta el id devuelto |
| `400 VALIDATION` | `lines` vacío, qty fuera de tope, modificador mal formado, `header` en una «Nuevo» |
| `404 NOT_FOUND` | `orderId` no existe en la empresa (RLS) |
| `409 ORDER_CLOSED` | El pedido está pagado y entregado (D-9). El front ofrece cuenta nueva |
| `422 PLATFORM_ORDER_NO_LINES` | Pedido de plataforma (D-11) |
| `409 DRAFT_DISCARDED` / `409 DRAFT_SENT` | El `id` ya existe y está terminal |

Publica `draft.updated`. Si `header.discount` viene puesto, lleva el mismo tope por usuario y el mismo
evento `draft_discount_set` que el `PATCH` (también en `import`). Más de 200 renglones → `400`. Con todos
los nombres en cuentas vivas, el nombre sale numerado («Persa 2») en vez de rechazar la cuenta.

### `GET /pos/drafts/{id}`

`200 DraftView` (también terminales: la pantalla necesita saber que se envió o se descartó en otra
tableta). `404` si no es de la empresa.

### `POST /pos/drafts/{id}/lines` — agregar (idempotente por `opId`)

```jsonc
{"opId": "uuid", "productId": 41, "qty": "1", "modifiers": [], "notes": ""}
// o, para el «+» de un renglón:
{"opId": "uuid", "intoLineId": "uuid", "qty": "1"}
```

`200 DraftView`. Sin modificadores ni nota se fusiona con el renglón igual (`domain.MergeTarget`).
`409 DRAFT_SENT|DRAFT_DISCARDED`; `404` si `intoLineId` no es de la cuenta (fue quitado: el front
recarga). Tope de renglones por cuenta: `domain.MaxDraftLines = 200` → `400`.

### `PATCH /pos/drafts/{id}/lines/{lineId}` — cambiar

```jsonc
{"expectedVersion": 2, "qty": "1", "modifiers": [...], "notes": "sin hielo"}  // cualquiera de los tres
```

`200 DraftView` · `409 DRAFT_CHANGED` (versión distinta o renglón ya no existe; nada se aplicó; el
front hace `GET` y avisa) · `400` qty ≤ 0 (para quitar se usa `DELETE`).

### `DELETE /pos/drafts/{id}/lines/{lineId}?expectedVersion=2` — quitar

`200 DraftView` · `409 DRAFT_CHANGED`. Quitar el último renglón deja la cuenta vacía y viva (con su
nombre); el front la descarta sin preguntar si el operador la cierra (D-7).

### `PATCH /pos/drafts/{id}` — cabecera

```jsonc
{"expectedHeaderVersion": 3, "customerName": "Mesa 4", "serviceType": "domicilio",
 "platformId": 2, "platformOrderRef": "A1B2", "deliveryFee": "25.00",
 "discount": {"percent": "10"}}   // campos ausentes no cambian; "discount": null lo quita
```

Lleva `rateLimitUser(h.descuentoWrites)` —el mismo tope que `PUT /orders/{id}/discount`— y, cuando
cambia el descuento, el evento `logging.SecurityEvent("draft_discount_set", …)` con el anterior y el
nuevo: un camino nuevo para poner un descuento no nace sin los controles del viejo. Quien cambia el
descuento o el folio de plataforma queda en `discount_set_by` / `platform_ref_set_by` y pasa al
pedido al enviar.

`200 DraftView` (reprecia si cambió `platformId`; cambiar de plataforma tira `platformOrderRef`) ·
`409 DRAFT_CHANGED` · `422 DRAFT_HAS_ORDER_HEADER` en una «Nuevo» · `422 PLATFORM_NOT_FOUND` ·
`422` descuento mayor que la venta (`ErrDescuentoMayorQueLaVenta`).

### `POST /pos/drafts/{id}/discard`

`{}` → `204`. Idempotente (ya descartada → `204`). `409 DRAFT_SENT` si ya es pedido (el mensaje lo
dice: «Ya se mandó a cocina; para quitarla hay que cancelar el pedido»). Suelta el nombre (research
R-3). Publica `draft.updated`.

### `POST /pos/drafts/{id}/send` — enviar a cocina

`{}` →

```jsonc
{
  "order": OrderView,            // el de hoy, completo
  "printLineIds": [501, 502],    // renglones para la comanda: todos si nació, solo lo nuevo si se agregó
  "created": true                // true = nació el pedido; false = se agregó a uno existente
}
```

| Respuesta | Cuándo |
|---|---|
| `200` | Enviada, o reintento de una ya enviada (mismo cuerpo; `printLineIds` vacío en el reintento para no reimprimir) |
| `409 NO_OPEN_REGISTER` | Sin turno abierto (cuenta sin pedido). La cuenta queda intacta |
| `422` producto no disponible | El de hoy (`ProductUnavailable`), con los nombres |
| `409 ORDER_CLOSED` / `422 PLATFORM_ORDER_NO_LINES` | La «Nuevo» de un pedido que se cerró / de plataforma |
| `409 DRAFT_DISCARDED` | Se descartó en otra tableta |
| `400 VALIDATION` | Cuenta vacía |

Una cuenta de plataforma **sin folio sí se manda** y el pedido queda pendiente de folio, como hoy con
`POST /orders`: la pantalla lo pide antes (`hayQuePedirElFolio` + `FolioPlataformaSheet`) y «Mandar sin
folio» es su salida explícita; un `400` aquí la dejaría sin salida. El pedido sale siempre a domicilio
cuando tiene plataforma (`domain.OrderServiceType`, la regla que antes aplicaba `domain/pedido.ts`).

Publica `order.created` u `order.updated` (lo de hoy) **y** `draft.updated`.

### `POST /pos/drafts/import` — subir las pestañas de la versión anterior (D-12)

```jsonc
{"accounts": [{
  "id": "uuid-de-la-pestaña", "folioName": "Persa",
  "header": {...},
  "lines": [{"opId": "uuidv5(tab.id, line.lineId)", "productId": 41, "qty": "2", "modifiers": [], "notes": ""}]
}]}
```

`200 {"results": [{"id": "uuid", "outcome": "created|exists|already_sent|skipped_empty|rejected", "draftId": "uuid|null", "orderId": 123|null, "reason": "…"}]}`.
`rejected` (con `reason` para quien opera): esa pestaña no se pudo subir —una opción de modificador que
ya no existe, una cabecera que ya no vale— y las demás sí; la tableta la descarta con aviso y borra su
copia igual. Un producto que ya no está en el menú se queda fuera de la cuenta sin rechazarla (si no
queda nada, `skipped_empty`). Más de `MaxDraftLines` (200) renglones en una pestaña → `400` de toda la
petición, igual que en `POST /pos/drafts`.
`already_sent`: existe `orders.client_uuid` o `order_line_batches.client_uuid` con ese id (la pestaña
se envió y no se cerró). Tope: 20 cuentas por llamada → `400`.

### `GET /pos/folio-names` (existente, cambia el predicado)

Excluye además los nombres de las cuentas vivas (research R-3). Mismo cuerpo.

### `POST /orders/{id}/lines` (existente, cambia la regla)

Rechaza con `409 ORDER_CLOSED` el pedido entregado y saldado, y con `422 PLATFORM_ORDER_NO_LINES` el
de plataforma. El POS ya no lo llama directo (pasa por `send`), pero el endpoint queda para la API.

### Vista del turno (`GET /cash-sessions/…`, `SessionView`) — campo nuevo

```jsonc
"liveAccounts": [ AccountItem ]   // cuentas en captura y entregadas que deben; NO bloquean
```

`Pending` (bloquea) y su guardia `sinPedidosPendientes` **no cambian** (D-10), salvo que cada
`PendingOrder` trae ahora su `id` (`{"id": 12, "number": 7, "name": "Persa"}`) para que el cierre
ofrezca «Abrir» (`/pos?pedido=<id>`). `liveAccounts` sale
del mismo servicio que `GET /pos/accounts?olderDebts=true` filtrado a `group ∈ {capturing, delivered_owes,
previous_days}`.

## Eventos SSE (`GET /events`)

| Tipo | Data | Cuándo |
|---|---|---|
| `draft.updated` | `{"id": "uuid", "orderId": 123\|null, "status": "capturando", "updatedAt": "…"}` | Toda escritura de cuenta, incluido el barrido |
| `order.created` / `order.updated` | los de hoy | Sin cambio |

## Códigos nuevos en `httpapi.Error`

| Sentinel (`domain`) | HTTP | `code` |
|---|---|---|
| `ErrDraftChanged` | 409 | `DRAFT_CHANGED` |
| `ErrDraftDiscarded` | 409 | `DRAFT_DISCARDED` |
| `ErrDraftAlreadySent` | 409 | `DRAFT_SENT` |
| `ErrDraftHasOrderHeader` | 422 | `DRAFT_HAS_ORDER_HEADER` |
| `ErrOrderClosed` | 409 | `ORDER_CLOSED` |
| `ErrPlatformOrderNoLines` | 422 | `PLATFORM_ORDER_NO_LINES` |

Los textos (lo que ve quien opera) van en el sentinel y en `web/src/api/mensajes.ts`, sin nombrar
internals (constitución, *Restricciones del producto*).
