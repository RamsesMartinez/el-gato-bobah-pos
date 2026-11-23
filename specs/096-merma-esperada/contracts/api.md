# Contratos de API (096)

Todas bajo `/api/v1`, con sesión. Errores por `httpapi.Error`: 400 validación, 403 sin permiso,
404 ajeno o inexistente.

## Subida 1

### `PUT /settings/expected-waste` — `menu.configure`
Body `{ "pct": number | null }`. `null` = sin merma general. 200 → `{ "pct": "5" | null }` (decimal como texto, como todo decimal de la API).
Recalcula costos en la misma transacción. Bitácora `settings.expected_waste`.

### `PUT /admin/products/{id}/expected-waste` — `menu.configure`
Body `{ "pct": number | null }`. `null` = usa el general. 200 → `{ "pct": number|null, "cost": "12.3400" }`.
Bitácora `product.expected_waste`.

### `PUT /stock/articles/{id}/expected-waste` — `menu.configure`
Body `{ "pct": number | null }`. `null` = sin captura; `0` = capturado en 0. Bitácora
`ingredient.expected_waste`.

Un `pct` ausente (sin la llave) es 400: el cuerpo siempre dice qué se quiere.

### `GET /business-settings` (existente)
Agrega `expectedWastePct: number | null`.

### `GET /admin/products` y detalle (existentes)
Agregan `expectedWastePct: number | null`.

### `GET /stock/articles/{id}` (existente)
Agrega `expectedWastePct: number | null`.

### `GET /products/{id}/costing` (existente, se amplía) — `menu.configure`
```json
{
  "productId": 12, "cost": "21.0000",
  "source": "receta" | "manual" | "compra" | "combo",
  "base": "20.0000", "waste": "1.0000",
  "generalPct": 5, "productPct": null,
  "lines": [{
    "ingredientId": 3, "name": "Leche", "baseUnit": "ml",
    "qtyBase": "200.0000", "unitCost": "0.025000", "base": "5.0000",
    "pct": 5, "pctSource": "article" | "product" | "general" | "none", "waste": "0.2500",
    "isPrep": false,
    "realWaste": [{ "warehouseId": 1, "warehouseName": "Local", "isSales": true,
                    "wasted": "0.5000", "consumed": "10.0000", "ratio": 5.0, "state": "ratio" }]
  }],
  "components": [{ "productId": 7, "name": "Té", "qty": 1, "cost": "8.0000", "waste": "0.4000" }]
}
```
`lines` y `components` siempre son arreglo (nunca `null`). `realWaste` siempre arreglo; estado
`no_sales` o `no_data` con `ratio: null`. Sin `inventory.view`, `realWaste` viaja vacío (son consumo y
mermas por almacén, que su ruta propia pide con ese permiso).

### `GET /stock/articles/{id}/real-waste` — `inventory.view`
`{ "days": 90, "items": [RealWaste...] }`.

## Subida 2 (como quedó implementada)

### `GET /stock/counts/{id}/vs-sales` — `inventory.health_view`
El mismo permiso que el faltante del conteo (`/counts/{id}/loss`), junto al que se muestra.
`{ "firstCount": bool, "items": [CountVsSalesItem...] }`; cada renglón: `ingredientId`, `name`,
`unitCode`, `hasPrevious`, `initial`, `entries`, `sold`, `expectedWaste`, `registered`,
`transfersOut`, `system`, `counted`, `diff`, `pct` (diferencia ÷ vendido, `null` sin ventas),
`group` (`review` | `suggest` | `small`) y `suggestedPct` (solo en `suggest`). Solo movimientos del
almacén del conteo; un traspaso es entrada o salida, no venta.

**Aplicar la sugerencia no tiene ruta propia**: «Usar N %» llama a `PUT /stock/articles/{id}/expected-waste`
(`menu.configure`, bitácora `ingredient.expected_waste`). Una ruta aparte habría repetido la misma
validación y la misma bitácora con otro nombre.

### `PUT /settings/expected-waste-deducts` — `inventory.configure`
`{ "deducts": bool }` (la llave es obligatoria: sin ella, 400). Responde `204`. Bitácora
`settings.expected_waste_deducts` solo si cambió. Va en su propia ruta y no en `PUT /settings/inventory`
porque el interruptor guarda al tocarlo: por la otra ruta tendría que reenviar el método de costeo y
la palabra del almacén, y un formulario abierto en otra tableta los pisaría. `GET /settings/business` lo devuelve como `expectedWasteDeducts`.

### `GET /stock/counts/{id}/expected` — `inventory.configure`
`{ "items": [{ "ingredientId", "expected" }] }`: lo que el sistema tiene del almacén del conteo, en
unidad base. El detalle del conteo (`/stock/counts/current`) nunca manda lo esperado: quien solo
cuenta no tiene cómo pedirlo (conteo a ciegas en el servidor, R12).

### Tipo de movimiento `merma_esperada`
Lo escribe la venta solo con la opción encendida, en el almacén de venta, con el mismo pedido,
renglón, opción y componente que su `venta`. Cancelar el pedido o el renglón y partir lo reponen.
