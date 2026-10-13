# Contratos: Salud del inventario

Todas bajo `/api/v1`, autenticadas, filtradas por empresa (RLS).

## GET /stock/health

Permiso: `inventory.health_view` (admin y gerente por omisión).

Query (todo opcional; uno presente y mal formado → 400 `VALIDATION`, nunca un valor por omisión en silencio):

| Parámetro | Tipo | Omisión |
|---|---|---|
| `from`, `to` | `YYYY-MM-DD`, día de negocio | últimos 30 días |
| `noPurchasesSince` | `YYYY-MM-DD` | inicio de los datos |
| `section` | una de las 5 secciones | todas, con 20 renglones cada una |
| `limit` / `offset` | 1–100 / ≥0 | 20 / 0 (solo con `section`) |

Respuesta 200 (todo arreglo viaja aunque esté vacío, nunca `null`):

```json
{
  "from": "2026-09-13", "to": "2026-10-13", "multipleBranches": false,
  "unsoldDeduction": {
    "total": 31,
    "items": [{ "kind": "product", "id": 1013, "name": "…", "group": "Ramen & Asiática",
                "mode": "undecided", "units": [{ "channel": "Mostrador", "units": "1" }],
                "linesWithoutDeduction": 1, "fixAt": "/catalogo/recetas?open=product:1013" }],
    "unpairedPlatformLines": 0
  },
  "negativeStock": { "total": 126, "items": [{ "ingredientId": 566, "name": "…", "unit": "pieza",
                     "onHand": "-10", "branch": null, "usedBy": 2, "fixAt": "/almacen/insumos/566" }] },
  "noPurchases": { "total": 118, "of": 130, "items": [] },
  "malformedRecipes": { "total": 0, "items": [{ "ownerKind": "extra", "ownerId": 0, "ownerName": "…",
                        "ingredient": "…", "problem": "unit_mismatch|zero_quantity|archived_ingredient|duplicate_ingredient",
                        "fixAt": "…" }] },
  "pendingRecipes": { "products": 42, "extras": 200, "fixAt": "/catalogo/recetas?status=review" }
}
```

- `units` y `onHand` son cadenas decimales, como el resto de la API.
- `noPurchases.items` viene vacío salvo con `section=no_purchases`: la pantalla lo pide al desplegar.

## GET /stock/health.csv

Mismo permiso y mismos parámetros que `/stock/health`, sin paginar.

- `200 text/csv; charset=utf-8`, con BOM y `Content-Disposition: attachment; filename="salud-del-inventario-<from>-<to>.csv"`.
- Columnas: `seccion, tipo, id, nombre, grupo, estado, canal, unidades, existencia, sucursal, problema`. Un renglón por problema y por canal.
- Lo calcula el mismo servicio que la pantalla (SC-005).

## PUT /admin/products/{id}/composition (cambio)

Guarda igual que hoy; agrega el modo `own`:

```json
{ "mode": "own", "basedOn": "<stamp>" }
```

| Transición | Efecto |
|---|---|
| `own` → `items` (un insumo) | traslada la existencia al insumo (2 ajustes ligados) y apaga la existencia propia |
| `own` → `items` (varios insumos) | cierra la existencia del producto en cero y apaga la existencia propia |
| `items` / `nothing` → `own` | prende la existencia propia; la receta queda guardada sin usarse |

- Toda transición deja un evento `product.consumption_mode` en la bitácora.
- Sin cambio de modo, el comportamiento es el de hoy.
- Errores: 409 `CONFLICT` si la copia está vieja (como hoy); 400 si se pide `own` en un paquete.

## GET /admin/recipes (cambio menor)

Sin cambio de contrato. La pantalla Menú › Recetas lee `?status=` y `?open=` de la URL para que «Salud del inventario» lleve directo a la lista o a la receta.
