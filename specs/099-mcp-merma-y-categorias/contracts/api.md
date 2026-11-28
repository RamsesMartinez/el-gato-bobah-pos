# Contratos de la API — 099

Todas bajo `/api/v1`, con sesión y `menu.configure`.

## GET /stock/articles/{id}/expected-waste/preview?pct=…  (nueva, solo lectura)

`pct`: obligatorio; número `0 ≤ pct < 100` con hasta dos decimales, o `none` (sin captura). Ausente o
mal formado → 400 `VALIDATION`. Id de otra empresa o inexistente → 404.

```json
{
  "ingredientId": 331, "name": "…", "isActive": true, "baseUnit": "ml",
  "unitCost": 0.1234,
  "current": {"pct": 7, "effectiveCost": 0.1320},
  "next":    {"pct": 8, "effectiveCost": 0.1333},
  "fallback": {"productOrGeneral": true, "generalPct": 5},
  "unchanged": false,
  "uses": [
    {"kind": "product", "id": 10, "name": "…", "before": 25.10, "after": 25.14},
    {"kind": "product", "id": 12, "name": "…", "before": 30, "after": 30, "manual": true},
    {"kind": "option",  "id": 44, "name": "…", "before": 3.20, "after": 3.21},
    {"kind": "prep",    "id": 90, "name": "…", "before": 0.0400, "after": 0.0404}
  ]
}
```

- `current.pct`/`next.pct` = `null` cuando no hay captura; entonces su `effectiveCost` es `null` y
  `fallback.productOrGeneral` = `true` (el costo depende del producto; ver `uses`).
- `uses` es siempre arreglo (vacío si nadie lo usa); transitivo por preparados, combos y extras que
  son otro producto. `manual: true` = producto de costo manual: usa el insumo y `before == after`.
- Todos los costos con 4 decimales, el redondeo de `current_cost`: con 2, un cambio chico se vería
  como «sin cambio».

## PUT /stock/articles/{id}/expected-waste  (ampliada)

Cuerpo: `{"pct": número | null, "reason"?: "texto"}`. `reason` opcional (la pantalla no lo manda);
si llega: se quitan los espacios de los extremos; vacío = sin motivo; más de 200 letras = 400. Va al `after` del evento `ingredient.expected_waste`.
Respuesta sin cambios: `{"pct": …}`.

## GET /admin/categories?status=all  (ampliada)

Sin `status`: igual que hoy (solo activas). `status=all`: activas e inactivas. Otro valor → 400.
Cada renglón gana `isActive` e `inactiveProductCount`.

## POST /admin/categories  (nueva)

Cuerpo: `{"name": "Tés", "parentId": 52 | null}`. 201 con el renglón de la categoría
(`id, name, parentId, isActive, …` como en la lista).

| Caso | Respuesta |
|---|---|
| nombre vacío o > 60 | 400 `VALIDATION` |
| padre inexistente / de otra empresa | 404 |
| padre que no es raíz, o inactivo | 422 `CATEGORY_PARENT_INVALID` |
| nombre repetido bajo el mismo padre | 409 `CATEGORY_NAME_TAKEN`, mensaje con `(id N)` y si está inactiva |

## PATCH /admin/categories/{id}  (ampliada)

Cuerpo con al menos uno de: `foodFamily` (como hoy), `name`, `isActive` (bool), `parentId`
(número o `null` = volverla raíz). `foodFamily` no se combina con los otros (400). Sin ninguno → 400.

| Caso | Respuesta |
|---|---|
| id de otra empresa / inexistente | 404 |
| `parentId` igual a sí misma, padre no raíz, padre inactivo con la categoría activa, o la categoría tiene hijos | 422 `CATEGORY_PARENT_INVALID` |
| `isActive:false` con productos activos | 409 `CATEGORY_HAS_ACTIVE_PRODUCTS` («tiene N productos activos») |
| `isActive:false` con subcategorías activas | 409 `CATEGORY_HAS_ACTIVE_CHILDREN` (las nombra) |
| `isActive:true` con padre inactivo | 422 `CATEGORY_PARENT_INVALID` |
| nombre repetido | 409 `CATEGORY_NAME_TAKEN` |
| nada cambia de verdad | 200 con el renglón, sin bitácora |

200 con el renglón actualizado.
