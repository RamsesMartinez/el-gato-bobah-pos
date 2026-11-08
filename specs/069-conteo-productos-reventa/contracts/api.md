# Contratos de API — 069

Todo bajo `/api/v1/stock`, en el grupo con `RequirePermission(PermInventoryCount)` (el mismo de
artículos, FR-010). La empresa la pone RLS; la sucursal es la del conteo.

## Conteo

### `GET /counts/current`, `POST /counts` — se amplía la respuesta

```json
{ "id": 12, "openedAt": "…", "items": [ /* artículos, sin cambio */ ],
  "products": [ { "productId": 81, "name": "Coca-Cola lata", "category": "Bebidas",
                  "countedQty": "15", "altNames": [] } ] }
```

- `products` siempre es arreglo (vacío si no hay). Sin lo que el sistema tiene (a ciegas, FR-004).
- La pantalla anterior ignora el campo.

### `PUT /counts/{id}/product-lines/{productId}`

Cuerpo: `{ "countedQty": "15", "confirmable": true, "confirmed": false }`.

| Respuesta | Cuándo |
| --- | --- |
| 200 `{ "countedQty": "15" }` | guardado |
| 400/422 `VALIDATION` | falta `countedQty` (o es null), decimal, negativo, sobre el máximo de piezas |
| 409 `NEEDS_CONFIRM` | `confirmable` y cifra rara sin `confirmed` (no dice la referencia) |
| 409 `CONFLICT` | el conteo ya se cerró |
| 404 | el producto no existe en la empresa, está archivado o no lleva existencia propia |

### `DELETE /counts/{id}/product-lines/{productId}` → 204. Vuelve a «sin contar».

### `GET /counts/{id}/preview`, `POST /counts/{id}/close` — se amplía la respuesta

```json
{ "adjusted": [ /* artículos */ ], "notCounted": [ /* artículos */ ],
  "adjustedProducts": [ { "productId": 81, "name": "…", "system": "-2", "counted": "15", "delta": "17" } ],
  "notCountedProducts": [ { "productId": 90, "name": "…" } ],
  "skippedProducts": [ { "productId": 77, "name": "…" } ] }
```

Los tres arreglos nuevos siempre presentes. `skippedProducts`: contados pero que ya no llevan
existencia propia; no se ajustan (FR-009).

## Composición de producto

### `PUT /admin/products/{id}/composition` (permiso `menu.configure`, sin cambio) — `mode: "options"`

- Sin `items`, `components` ni `linkedProductId`; si traen algo, 422.
- 422 `Ninguna de sus opciones resta del almacén` si el producto no tiene opción que reste.
- Desde existencia propia: mismo traslado que `nothing` (cierra en cero, bitácora).

### `GET /admin/products/{id}/composition` — campos nuevos

- `optionsDeduct: boolean` — alguna opción ligada resta (FR-011).
- `optionsRequired: boolean` — algún grupo con opción que resta es obligatorio (`min_select ≥ 1`) (FR-014).
- `mode` deducido vale `options` en Recetas (`mode: "options"`). Salud del inventario no lista estos productos (FR-015).
