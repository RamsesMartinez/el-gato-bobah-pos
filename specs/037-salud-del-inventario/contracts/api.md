# Contratos: Salud del inventario

Todas bajo `/api/v1`, autenticadas, filtradas por empresa (RLS).

## GET /stock/health

Permiso: `inventory.health_view` (admin y gerente por omisión).

Query (todo opcional; uno presente y mal formado → 400 `VALIDATION`, nunca un valor por omisión en silencio):

| Parámetro | Tipo | Omisión |
|---|---|---|
| `from`, `to` | `YYYY-MM-DD`, día de negocio | últimos 30 días |
| `noPurchasesSince` | `YYYY-MM-DD` | inicio de los datos |
| `section` | una de las 5 secciones | todas, con 25 renglones cada una |
| `limit` / `offset` | 1–100 / ≥0 | 25 / 0 (solo con `section`), igual que «Ver 25 más» de Recetas |

Respuesta 200 (todo arreglo viaja aunque esté vacío, nunca `null`):

```json
{
  "from": "2026-09-13", "to": "2026-10-13", "multipleBranches": false, "salesInPeriod": 812,
  "unsoldDeduction": {
    "total": 31,
    "items": [{ "kind": "product", "id": 1013, "name": "…", "group": "Ramen & Asiática",
                "mode": "undecided", "units": [{ "channel": "Mostrador", "units": "1" }],
                "linesWithoutDeduction": 1, "estimatedCost": "47.00",
                "fixAt": "/catalogo/recetas?open=product:1013" }],
    "unpairedPlatformLines": 0
  },
  "comboChoices": { "total": 0, "items": [{ "productId": 0, "name": "…", "slot": "…", "options": 3,
                    "fixAt": "/catalogo/recetas?open=product:0" }] },
  "negativeStock": { "total": 126, "collapsed": true, "items": [{ "ingredientId": 566, "name": "…", "unit": "pieza",
                     "onHand": "-10", "branch": null, "usedBy": 2, "fixAt": "/almacen/insumos/566" }] },
  "noPurchases": { "total": 118, "of": 130, "items": [] },
  "malformedRecipes": { "total": 0, "items": [{ "ownerKind": "extra", "ownerId": 0, "ownerName": "…",
                        "ingredient": "…", "problem": "unit_mismatch|zero_quantity|archived_ingredient|duplicate_ingredient",
                        "fixAt": "…" }] },
  "pendingRecipes": {
    "undecided": { "products": 24, "extras": 180, "fixAt": "/catalogo/recetas?status=pending" },
    "review":    { "products": 42, "extras": 200, "fixAt": "/catalogo/recetas?status=review" }
  }
}
```

- `units` y `onHand` son cadenas decimales, como el resto de la API.
- `noPurchases.items` viene vacío salvo con `section=no_purchases`: la pantalla lo pide al desplegar.
- `negativeStock.collapsed` es `true` mientras la empresa no tenga ninguna compra registrada (FR-005): la pantalla abre la sección con su renglón explicativo.
- `estimatedCost` = piezas sin restar × costo de hoy; cadena decimal, ordena la lista de mayor a menor. No se suma a ningún total. Es `null` cuando el producto o extra no tiene costo capturado (costo en 0): la pantalla dice «Sin costo», y esos renglones van después de los que tienen monto, ordenados por piezas.
- `comboChoices` lista los combos con un hueco de más de un producto (FR-002b). No es una sección propia: la pantalla los muestra dentro de «Se vende sin restar» y el total de esa sección los incluye. No se pagina (son pocos: uno por combo). En la exportación salen con `seccion=unsold_deduction`, `tipo=combo` y `problema=varias_opciones`.

## GET /stock/health.csv

Mismo permiso y mismos parámetros que `/stock/health`, sin paginar.

- `200 text/csv; charset=utf-8`, con BOM y `Content-Disposition: attachment; filename="salud-del-inventario-<from>-<to>.csv"`.
- Columnas: `seccion, tipo, id, nombre, grupo, estado, canal, piezas, costo_estimado, existencia, sucursal, problema`.
- «Recetas pendientes» no tiene renglones de lista: va como cuatro renglones de resumen, `seccion=pending_recipe`, `tipo=resumen`, `problema` = `undecided_products`, `undecided_extras`, `review_products` o `review_extras`, y la cuenta en `piezas`. Así el archivo trae las mismas cifras que la pantalla (FR-011). `tipo` ∈ producto, extra, combo, insumo, renglon_receta, resumen. `seccion` es el código estable de la sección (`unsold_deduction`, `negative_stock`, `no_purchases`, `malformed_recipe`, `pending_recipe`), el mismo de `?section=`. En lo vendido sin restar, `costo_estimado` se reparte por canal con el costo unitario de hoy: la columna suma el total del producto; sin costo capturado, la celda va vacía (no 0). Las celdas de texto que empiezan con `=`, `+`, `-`, `@`, tabulador o retorno de carro llevan un apóstrofo delante; las numéricas no. En `negative_stock`, `existencia` es solo el número (una hoja de cálculo la suma) y la unidad va en `grupo`. Un renglón por problema y por canal.
- Lo calcula el mismo servicio que la pantalla (SC-005).

## PUT /admin/products/{id}/composition (cambio)

Permiso: `menu.configure` (antes, rol admin o gerente; mismo resultado hoy). Aplica también a la ruta de composición de extras y a los `confirm` de los dos. Las de insumo siguen como están (research R5).

Los mismos modos que hoy (`items`, `package`, `nothing`); **no** se agrega un modo de existencia propia (research R6). Deja de rechazar un producto con existencia propia y agrega un campo opcional:

```json
{ "mode": "items", "items": [{ "ingredientId": 9, "quantity": "1", "unitId": 8 }],
  "actualStock": "20", "seenProductOnHand": "12", "seenIngredientOnHand": "20",
  "basedOn": "<stamp>" }
```

| Producto con existencia propia → | Efecto |
|---|---|
| `items` de un insumo sin existencia | traslada la existencia al insumo (2 ajustes ligados) y apaga la existencia propia |
| `items` de un insumo con existencia | exige `actualStock`; el insumo queda en esa cifra (1 ajuste), el producto en cero |
| `items` de varios insumos, `package` o `nothing` | cierra la existencia del producto en cero y apaga la existencia propia |

- Toda transición deja un evento `product.consumption_mode` en la bitácora.
- Sin existencia propia de por medio, el comportamiento es el de hoy, salvo el evento de bitácora cuando cambia el modo.
- Errores: 409 `CONFLICT` si la copia está vieja (como hoy); 400 `VALIDATION` si falta `actualStock` cuando hace falta, o si viene negativo o mal formado; 409 `CONFLICT` si la existencia cambió desde que se mostró (una venta en medio): la hoja vuelve a preguntar con las cifras nuevas; 409 si una venta en curso tiene tomada la existencia del insumo (se reintenta en un momento); 400 si el producto tiene existencia propia y la petición no dice `mode`; 409 si el producto tiene existencia en más de una sucursal y el insumo también (se resuelve con un conteo).

## GET /admin/recipes (cambio menor)

Sin cambio de contrato. La pantalla Menú › Recetas lee `?status=` y `?open=` de la URL para que «Salud del inventario» lleve directo a la lista o a la receta.
