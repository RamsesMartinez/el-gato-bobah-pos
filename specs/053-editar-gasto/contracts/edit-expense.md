# Contrato: editar un gasto y registrar entregas

Base: `/api/v1`. Ambas rutas con `RequireAuth`. Cuerpo y respuesta en JSON; el dinero y las
cantidades viajan como cadenas decimales (como el resto de gastos).

## `POST /expenses/{id}/edit[?preview=1]` — permiso `expenses.edit`

Encabezado, renglones agregados y renglones quitados, en una sola operación.

```json
{
  "header": {
    "supplierId": 12,            // null = sin proveedor; ausente = no cambia
    "categoryId": 4,
    "description": "Tapas y fajillas",
    "expenseDate": "2026-10-31"
  },
  "add": [
    { "itemType": "ingrediente", "ingredientId": 464, "description": "Tapas Traveler 16 oz",
      "quantity": "50", "unitId": 7, "presentationId": null, "amount": "50.00", "isGift": false,
      "arrivedOn": "2026-10-31" }            // presente = «Ya llegó» ese día (exige inventory.receive)
  ],
  "remove": [ 812, 813 ],
  "decisions": { "count:464": "keep", "money": "leave" },
  "fingerprint": "…"                       // solo al confirmar: la de la última vista previa
}
```

- Ausentes `header`, `add` y `remove` (o todos vacíos) → 400.
- `add[].itemType` nulo → renglón sin artículo (no mueve inventario).
- Un `remove` de un renglón excluido, quitado o de otro gasto → 404/400.

## `POST /expenses/{id}/deliveries[?preview=1]` — permiso `inventory.receive`

Una entrega de uno o varios renglones.

```json
{
  "on": "2026-10-31",
  "lines": [
    { "itemId": 901, "qty": "3", "on": null, "shortfall": "pending", "extraAmount": null }
  ],
  "decisions": { "count:464": "adjust" },
  "fingerprint": "…"
}
```

- `qty` en la unidad del renglón, > 0. `lines[].on` (avanzado) sustituye a `on` en ese renglón.
- `shortfall` (`pending` | `adjust`) solo cuando la entrega deja algo pendiente; si falta, la vista
  previa lo preselecciona (`ShortfallDefault`) y la confirmación lo exige.
- `extraAmount` obligatorio si `qty` pasa de lo que falta y el renglón no es regalo (regla 038).

## Respuesta 200 (vista previa o confirmación)

```json
{
  "preview": {
    "header": [ { "field": "supplier", "before": "Inix", "after": "Costco" } ],
    "lines": [
      { "kind": "added", "itemId": null, "name": "Tapas Traveler 16 oz", "qty": "50", "amount": "50.00", "on": "2026-10-31" },
      { "kind": "removed", "itemId": 812, "name": "Fajillas", "amount": "17.50" },
      { "kind": "received", "itemId": 901, "name": "Leche", "qty": "3", "on": "2026-10-31", "missingAfter": "2" },
      { "kind": "adjusted", "itemId": 902, "name": "Queso", "amountBefore": "300.00", "amountAfter": "255.00" }
    ],
    "inventory": [
      { "ingredientId": 464, "productId": null, "name": "TAPA TRAVELER 16 INIX", "baseUnit": "pieza",
        "before": "-5", "after": "45", "negativeAfter": false, "decisionKey": null }
    ],
    "money": {
      "amountBefore": "67.50", "amountAfter": "67.50", "paid": "67.50",
      "statusBefore": "pagada", "statusAfter": "pagada",
      "due": "0.00", "overpaid": "0.00", "decisionKey": null
    },
    "costs": [ { "ingredientId": 464, "name": "TAPA TRAVELER 16 INIX", "before": "1.000000", "after": "1.000000" } ],
    "decisions": [
      { "key": "count:464", "kind": "count", "countId": 11, "countedOn": "2026-10-20",
        "options": ["adjust", "keep"], "chosen": null },
      { "key": "money", "kind": "money", "options": ["pay", "leave"], "chosen": null },
      { "key": "shortfall:901", "kind": "shortfall", "options": ["pending", "adjust"], "chosen": "pending" }
    ],
    "fingerprint": "3f1c…"
  },
  "expense": { "…": "ExpenseDetail, solo al confirmar" }
}
```

- Todo arreglo se entrega aunque esté vacío (AGENTS.md, «Un slice nil de Go sale como null»).
- `decisions[].chosen` nulo = falta decidir; la pantalla deshabilita «Confirmar».
- `money.decisionKey` = `"money"` solo si `due > 0` y hay pagos (FR-011).
- `shortfall:<itemId>` se decide en el cuerpo con `lines[].shortfall`, no en `decisions`.

## Errores

| Código | Cuándo |
|---|---|
| 400 `VALIDATION` | cantidades, dinero o fechas inválidas; fecha futura o anterior al gasto; importe que quedaría ≤ 0; decisión o `shortfall` desconocidos |
| 400 `DECISION_REQUIRED` | al confirmar falta una decisión que la vista previa pide |
| 400 `DRAWER_EXTRA_TOO_HIGH` | el importe nuevo deja lo salido de la caja por encima del tope (spec 050) |
| 403 | sin `expenses.edit` / `inventory.receive`; `arrivedOn` sin `inventory.receive` |
| 404 | gasto o renglón inexistente (o de otra empresa: RLS) |
| 409 `CONFLICT` | gasto anulado; una venta tiene el insumo («vuelve a intentar») |
| 409 `EXPENSE_EDIT_STALE` | la huella no coincide: el estado cambió desde la vista previa |

## Lo que no cambia

`POST /expenses/{id}/pay` (el «Registrar pago» del aviso de dinero), `/receive`, `/stock-lines`,
`/items`, `/items/{itemId}/receive`, `/items/{itemId}/not-coming` y `PATCH /items/{itemId}` siguen
iguales; los que insertan movimientos graban `effective_on` = hoy.
