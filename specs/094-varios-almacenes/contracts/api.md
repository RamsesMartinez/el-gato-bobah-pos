# Contratos HTTP y MCP

Todo bajo `/api/v1`, con `RequireAuth` + `RequirePermission`. Un `warehouseId` de otra empresa, de
otra sucursal o archivado (donde no se admite) responde **404 `WAREHOUSE_NOT_FOUND`**, igual que uno
que no existe. Un `warehouseId` malformado es 400 (nunca cae al de venta en silencio; constitución V).
Ausente = el de venta.

## Subida 1 (E1 + E2)

| Método y ruta | Permiso | Cuerpo / query | Respuesta |
| --- | --- | --- | --- |
| `GET /stock/warehouses` | `inventory.view` | `?includeArchived=1` | `{items: [{id, name, isSales, isActive, articlesWithStock}]}` — siempre arreglo |
| `POST /stock/warehouses` | `inventory.warehouse_manage` | `{name}` | 201 `{id, …}`; 409 `WAREHOUSE_NAME_TAKEN` |
| `PATCH /stock/warehouses/{id}` | `inventory.warehouse_manage` | `{name}` | 200; en subida 2 también `{isSales: true}` |
| `POST /stock/warehouses/{id}/archive` | `inventory.warehouse_manage` | — | 200; 409 `WAREHOUSE_IS_SALES` / `WAREHOUSE_HAS_STOCK` (con `{items: n}`) / `WAREHOUSE_COUNT_OPEN` |
| `POST /stock/warehouses/{id}/restore` | `inventory.warehouse_manage` | — | 200 |
| `GET /stock/levels` | `inventory.view` | `?warehouseId=<id>|all` | un renglón por artículo; con `all`, la suma de los almacenes activos de la sucursal. `transferFrom` (sugerir traspaso en un negativo) llega con los traspasos, en la subida 3 |

## Subida 2 (E3)

- Renglón de gasto (`POST /expenses`, `PUT /expenses/{id}`, `PUT /expenses/{id}/stock-lines`,
  recibir en «Por recibir»): campo `warehouseId` por renglón. Ausente = el de venta (la tableta con
  la pantalla anterior sigue funcionando). Un renglón sin artículo lo ignora; un excluido lo guarda pero no
  mueve existencias.
- Cambiar `warehouseId` de un renglón ya aplicado: salida del almacén anterior por lo aplicado y
  entrada al nuevo (`PlanStockLines` por almacén).
- `PATCH /stock/warehouses/{id}` acepta `{isSales: true}`.

## Subida 3 (E4)

| Método y ruta | Permiso | Cuerpo | Respuesta |
| --- | --- | --- | --- |
| `POST /stock/transfers` | `inventory.transfer` | `{clientKey, fromWarehouseId, toWarehouseId, note?, confirmNegative?, lines: [{itemType, ingredientId|productId, qty, presentationId?, presentationQty?}]}` | 201 `{id, …}`; reintento igual → 200 el mismo; cuerpo distinto con la misma `clientKey` → 409 `TRANSFER_KEY_REUSED`; origen insuficiente sin `confirmNegative` → 409 `NEEDS_CONFIRM` con `{lines: [{…, available}]}` |
| `GET /stock/transfers` | `inventory.view` | `?from&to&limit` | lista con renglones |

## Subida 4 (E5)

- `POST /stock/counts` acepta `{warehouseId?}`; uno abierto por almacén.
- Merma (`POST /waste`), «se acabó» (`POST /stock/out-reports`) y mínimos (`PUT …/low-stock`) aceptan
  `warehouseId?`. La lista de compra y los avisos devuelven `warehouseId` y `warehouseName`.
- Lugares de conteo: `warehouseId` en crear y listar; contar un almacén solo trae sus lugares.

## Subida 5 (E6)

- `GET /stock/movements?warehouseId=<id>|all` — cada movimiento con `warehouseId`, `warehouseName`,
  `transferId` y, si es traspaso, el almacén contraparte.
- `GET /stock/valuation` — `{warehouses: [{id, name, value, items}], total}`.

## MCP (cada herramienta en la subida de su ruta)

`warehouse_id` opcional (por omisión el de venta) en `stock_levels`, `stock_adjust`, `waste_record`,
`stock_count_open`, `purchase_create`, `purchase_receive`, `expense_create`, `expense_edit`,
`low_stock_update` y `stock_movements`. Nuevas: `warehouses_list`, `stock_transfer` (con vista
previa e idempotencia por la clave de la herramienta). La vista previa nombra el almacén. Sin
`warehouse_id` por decisión: las de recetas y pedidos (descuentan siempre del de venta; documentado
en su descripción).

En la subida 1, `stock_adjust` y `stock_levels` aceptan `warehouse_id`, pero escribir en un almacén
que no es el de venta responde 409 `WAREHOUSE_NOT_WRITABLE_YET` hasta la subida 2 (research R2).
