# Contratos de API (096)

Todas bajo `/api/v1`, con sesión. Errores por `httpapi.Error`: 400 validación, 403 sin permiso,
404 ajeno o inexistente.

## Subida 1

### `PUT /settings/expected-waste` — `menu.configure`
Body `{ "pct": number | null }`. `null` = sin merma general. 200 → `{ "pct": number|null }`.
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
`no_sales` o `no_data` con `ratio: null`.

### `GET /stock/articles/{id}/real-waste` — `inventory.view`
`{ "days": 90, "items": [RealWaste...] }`.

## Subida 2 (se detalla al implementar; forma esperada)

- `GET /stock/counts/{id}/vs-sales` — `inventory.view`: renglones del reporte (R10) con
  `suggestedPct` o `flag: "review"`.
- `POST /stock/articles/{id}/expected-waste/apply-suggestion` — `menu.configure`.
- `PUT /settings/inventory` (existente) agrega `expectedWasteDeducts: boolean`.
- El detalle del conteo deja de mandar lo esperado a quien solo tiene `inventory.count`.
