# Contratos: Compras al almacén

Todas bajo `/api/v1`, con la empresa de la sesión (RLS). Errores por `httpapi.Error`: 400
validación, 403 permiso, 404 no existe, 409 conflicto (incluye `BRANCH_AMBIGUOUS`).

## Compras

### `GET /expenses` (existente, cambia)

Acepta `pending=true`: solo gastos con algún renglón pendiente de llegar. Ausente = todos; otro
valor → 400. `CountExpenses` usa el mismo `where`.

### `POST /expenses` (existente, cambia)

- `receivedAt` se guarda solo si el gasto lleva al menos un renglón de almacén; sin renglones se
  ignora (R1).
- Un renglón de almacén acepta `presentationId`; si viene, el servidor copia su `qtyInBase` en
  `packQtyInBase` y fija la unidad «pieza». Una presentación de otro insumo → 400.
- Si el gasto llega recibido, cada renglón suma y el costo del insumo se actualiza (R4).

### `POST /expenses/{id}/receive` (existente, cambia de guarda)

Permiso `inventory.receive`. Mismo cuerpo de hoy. Ahora actualiza el costo de cada insumo y deja
`expense.received` en la bitácora.

### `PUT /expenses/{id}/stock-lines` (nuevo)

Permiso `inventory.receive`. El conjunto deseado de renglones de almacén del gasto.

```json
{
  "lines": [
    { "id": 41, "seenApplied": "36000", "ingredientId": 7, "quantity": "3", "presentationId": 2, "amount": "540.00" },
    { "ingredientId": 9, "quantity": "2", "unitId": 2, "amount": "90.00" }
  ]
}
```

- Un renglón con `id` se corrige; sin `id` se agrega; un `id` existente que no viene se quita.
- `seenApplied` es obligatorio en los renglones con `id`; si ya no es lo aplicado → 409.
- Respuesta 200: el gasto con sus renglones, y por renglón `applied` (unidad base) y la unidad.
- Un gasto anulado → 409. Un gasto sin recibir → los renglones se guardan sin tocar el almacén.

### `GET /stock/pending-receipts` (nuevo)

Permiso `inventory.receive`. Lo pendiente de la sucursal: por renglón, la compra (id, fecha,
proveedor), el artículo, lo pedido, lo recibido y lo que falta, en su empaque y en unidad base.
Siempre arreglo, aunque vacío.

### `POST /expenses/{id}/items/{itemId}/receive` (nuevo)

Permiso `inventory.receive`. `{ "qty": "2", "seenApplied": "0", "extraAmount": null }` en la
unidad del renglón (empaque o unidad). Suma la parte. Si pasa de lo que falta, `extraAmount` es
obligatorio (> 0): lo extra sube la cantidad y el importe del renglón y el total de la compra. Si
`seenApplied` ya no es lo aplicado → 409 con lo ya recibido.

### `POST /expenses/{id}/items` (nuevo)

Permiso `inventory.receive`. «Llegó algo más»: `{ "ingredientId", "quantity", "presentationId" |
"unitId", "amount", "isGift": false }`. Llega ya recibido: suma al guardarse. Con `isGift`, `amount`
debe ser 0 y no cambia el total; sin él, `amount > 0` y el total de la compra sube por ese importe.

### `POST /expenses/{id}/items/{itemId}/not-coming` (nuevo)

Permiso `inventory.receive`. «Ya no va a llegar»: `{ "qty": "1" }` en la unidad del renglón,
hasta lo que falta (puede ser una parte). No suma ni cambia el total; si ya no queda nada
pendiente en la compra, fija `received_at`.

### `POST /expenses/{id}/cancel` (existente, cambia)

Un gasto recibido se anula si todo lo aplicado es 0 (se revierte antes con `stock-lines` vacío; la
pantalla hace los dos pasos con una sola confirmación). Si queda algo aplicado → 409.

## Presentaciones

Permiso `inventory.configure` para escribir; leer, cualquiera con acceso a Almacén.

| Método | Ruta | Cuerpo / respuesta |
|---|---|---|
| GET | `/stock/ingredients/{id}/presentations` | `[{ id, name, qtyInBase, active }]` |
| POST | `/stock/ingredients/{id}/presentations` | `{ name, qtyInBase }` → 201; nombre repetido → 409 |
| PUT | `/stock/presentations/{id}` | `{ name, qtyInBase, active }` |

No hay DELETE: una presentación usada se desactiva.

## Conteo

Permiso `inventory.count`.

| Método | Ruta | Qué hace |
|---|---|---|
| GET | `/stock/counts/current` | El conteo abierto de la sucursal con sus renglones, o 204 |
| POST | `/stock/counts` | Abre uno; si ya hay uno abierto → 409 con su id |
| PUT | `/stock/counts/{id}/lines/{ingredientId}` | `{ countedQty }` en unidad base; guarda `system_qty` actual. Conteo no abierto → 409 |
| DELETE | `/stock/counts/{id}/lines/{ingredientId}` | Quita una cifra capturada (vuelve a «no contado») |
| POST | `/stock/counts/{id}/close` | Inserta los ajustes y responde `{ adjusted, notCounted }` |
| POST | `/stock/counts/{id}/discard` | Descarta sin tocar el almacén |

`countedQty` negativo, NaN o fuera de `MaxStockQty` → 400.

`GET /stock/counts/last-closed` (permiso `inventory.receive`: lo usa la captura de compras; `RequirePermission` acepta un solo permiso): `{ "closedAt":
"2026-10-14T07:55:00-06:00" }` del último conteo cerrado de la sucursal, o `{ "closedAt": null }`.
La captura lo usa para avisar cuando la fecha del gasto es anterior (Assumptions del spec).

`GET /stock/counts/current` devuelve los insumos activos ordenados por lugar (los sin lugar al
final) y por nombre, cada uno con sus empaques activos; **no** devuelve `system_qty` mientras el
conteo esté abierto. `POST …/close` responde además, por renglón contado, lo que el sistema
tenía y la diferencia, y la lista de los no contados por nombre; la pantalla pide confirmar antes
con `GET /stock/counts/{id}/preview` (mismo cálculo, sin escribir).

`PUT /stock/ingredients/{id}/place` — permiso `inventory.configure`: `{ "place": "Refri" | null }`.

## Configuración

`PUT /settings/inventory` — permiso `inventory.configure`, registrada fuera del grupo con rol.
`{ "costingMethod": "last" | "weighted", "stockItemLabel": "artículo" }`; otro método, o una
palabra vacía o de más de 20 caracteres → 400. Deja `settings.costing_method` en la
bitácora. `GET /business-settings` (existente) los incluye como `costingMethod` y `stockItemLabel`. Va en su propia
ruta y no en `PUT /business-settings` porque ésa exige rol y lleva datos fiscales: un permiso de
almacén no debe poder cambiar el nombre fiscal.
