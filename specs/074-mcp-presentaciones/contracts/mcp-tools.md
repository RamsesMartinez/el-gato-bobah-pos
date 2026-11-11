# Contrato: herramientas MCP de la 074

Comunes (072): `dry_run` ausente/`true` = vista previa; `dry_run: false` + `idempotency_key` aplica.
Dinero y cantidades como texto decimal; fechas `AAAA-MM-DD`.

## presentation_create — `write:inventory`, `inventory.configure`

| Argumento | Tipo | Nota |
| --- | --- | --- |
| `ingredient_id` | int | obligatorio |
| `name` | string | «Bolsa», «Caja»…; no el nombre de una unidad |
| `qty` | decimal texto | cuánto trae, en `unit_id` |
| `unit_id` | int | de `units_list` |
| `brand` | string? | ≤ 40 letras; misma marca y nombre en el insumo → 409 |
| `approximate` | bool? | se compra en otra clase de unidad |

Despacha `POST /api/v1/stock/ingredients/{ingredient_id}/presentations` con
`{name, qty, unitId, approximate, brand}`. Vista previa: los empaques actuales del insumo.
Lecturas permitidas: `GET /api/v1/stock/presentations`.

## presentation_update — `write:inventory`, `inventory.configure`

`presentation_id` obligatorio; `name`, `qty` + `unit_id` (juntos), `brand` (`""` la quita),
`approximate`, `active`: opcionales, al menos uno. Lo no mandado se toma del empaque actual (R4).
Despacha `PUT /api/v1/stock/presentations/{presentation_id}`. Lecturas: `GET /stock/presentations`,
`GET /units`. Antes y después: el empaque.

## expense_edit — `write:expenses`, `expenses.edit`

| Argumento | Tipo | Nota |
| --- | --- | --- |
| `expense_id` | int | obligatorio |
| `supplier_id` | int? | cambia el proveedor |
| `clear_supplier` | bool? | quita el proveedor (excluye `supplier_id`) |
| `category_id`, `description`, `expense_date` | ? | encabezado |
| `remove` | int[] | ids de renglón (de `expense_get`) |
| `add` | renglón[] | como `purchase_create.lines` + `arrived_on` (vacío = queda por recibir); un renglón sin artículo es un gasto sin mercancía |
| `decisions` | {clave: opción} | las que pidió la vista previa (`count:<id_insumo>`: `adjust` o `keep`; `money`: `pay` o `leave`) |
| `fingerprint` | string | obligatorio al aplicar: el de la última vista previa |

Vista previa: `POST /api/v1/expenses/{id}/edit?preview=1` → `{preview: {header, lines, inventory,
costs, money, decisions, fingerprint}}` y notas (renglones que quedan por recibir). Aplicar: la misma
ruta sin consulta. Errores con corrección: `EXPENSE_EDIT_STALE` (409: vuelve a pedir la vista previa),
`DECISION_REQUIRED` (400: manda la decisión). Lecturas: `GET /api/v1/expenses/{id}`.

## Cambio a purchase_create (vista previa)

En cada renglón con insumo y sin `presentation_id`: `presentations` (activos, con `brand`) y
`create_presentation: {tool, args: {ingredient_id, name, unit_id?}, ask: ["brand", "qty"]}`; con
`unit_id`, nota de que entra sin marca ni empaque. Lecturas nuevas: `GET /stock/articles/{id}`,
`GET /units`.
