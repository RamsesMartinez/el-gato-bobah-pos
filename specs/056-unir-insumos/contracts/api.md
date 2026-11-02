# Contratos HTTP (056)

Todas bajo `/api/v1`, con sesión. Errores con el mapeo de `httpapi.Error`.

## `POST /stock/articles/{id}/merge?preview=1` y `POST /stock/articles/{id}/merge`

Permiso: `inventory.configure`. `{id}` es el destino.

```json
{
  "sources": [ { "id": 12, "factor": "1" }, { "id": 40, "factor": "0.25" } ],
  "fingerprint": "…"           // solo al confirmar: la de la última vista previa
}
```

- `factor` es cuánto del destino vale una unidad base del origen. Omitido o `"1"` con la misma unidad
  base; obligatorio y > 0 con unidad base distinta.
- Vista previa → 200 con `MergePreview` (ver [data-model.md](../data-model.md)). No cambia nada.
- Confirmar → 200 con el `MergePreview` aplicado (sin `fingerprint` nueva).

| Respuesta | Código | Cuándo |
|---|---|---|
| 400 | `VALIDATION` | sin orígenes, más de 20, repetidos, el destino entre ellos, factor ≤ 0 o que no cabe, falta el factor con unidad distinta, una cantidad convertida en cero |
| 403 | `FORBIDDEN` | sin `inventory.configure` |
| 404 | `NOT_FOUND` | destino u origen que no existe (u otra empresa, por RLS) |
| 409 | `MERGE_BLOCKED` | conteo abierto, compras por recibir o mermas pendientes de un origen, preparado, origen archivado o ya unido, destino archivado; el mensaje dice qué y de cuál |
| 409 | `MERGE_PRESENTATION_CONFLICT` | dos presentaciones con mismo nombre y marca y distinto contenido |
| 409 | `MERGE_STALE` | la huella no coincide: algo cambió desde la vista previa |
| 409 | `CONFLICT` | una venta, compra, merma o ajuste en curso sobre alguno (nowait); el mensaje dice «vuelve a intentar en un momento» |

## `GET /stock/articles/similar?name=…`

Permiso: `inventory.configure` (el mismo del alta). `name` obligatorio, 1–80 caracteres; vacío o
más largo → 400 (nunca un default).

```json
{ "items": [ { "id": 3, "name": "Leche entera", "baseUnitCode": "ml" } ] }
```

Hasta 5, los más parecidos primero; solo activos. Nunca `null`: sin parecidos, `"items": []`. La
hoja de alta lo llama al tocar «Guardar» y, si hay parecidos, pregunta antes de `POST /stock/articles`.

## `GET /stock/articles/duplicate-groups`

Permiso: `inventory.configure`.

```json
{ "groups": [ { "items": [ { "id": 3, "name": "Leche entera", "baseUnitCode": "ml", "onHand": "8400" },
                           { "id": 9, "name": "Leche entera Marca B", "baseUnitCode": "ml", "onHand": "3000" } ] } ] }
```

Grupos de 2 o más insumos activos (sin preparados) por la regla de parecido, agrupados por
transitividad y sin los pares descartados. Ordenados por el nombre del primero. Nunca `null`.

## `POST /stock/articles/distinct-pairs`

Permiso: `inventory.configure`. `{ "a": 3, "b": 9 }` → 204. Idempotente. `a == b`, ids de otra
empresa o inexistentes → 400/404.

## Cambios a contratos existentes

- `GET /stock/presentations`, `GET /stock/ingredients/{id}/presentations` y las presentaciones de la ficha: cada una trae
  `"brand": string | null`.
- `POST /stock/ingredients/{id}/presentations` y `PUT /stock/presentations/{id}`: aceptan
  `"brand"` opcional (vacío o solo espacios = sin marca; más de 40 → 400). Choque de nombre+marca →
  409 como hoy.
- `GET /stock/articles/{id}`: trae `"mergedInto": { "id", "name" } | null` (si es un origen unido)
  y `"mergedSources": int` (cuántos orígenes tiene).
- `POST /stock/articles/{id}/reactivate` sobre un origen unido → 409 `MERGE_BLOCKED`.
- `POST /stock/articles/{id}/unit/preview` y `/unit` sobre un destino con orígenes → 409
  `MERGE_BLOCKED`.
- `GET /stock/articles/{id}/cost-history`: cada renglón trae `"fromName": string | null` (el origen
  del que viene; nulo si es del propio) y el costo ya convertido a la unidad del destino.
- Cualquier escritura que apunte a un origen unido (receta, renglón de compra, presentación,
  artículo de proveedor, formato, merma, conteo, corrección de costo) → 409 `INGREDIENT_MERGED`
  «Ese insumo se unió a <destino>; vuelve a elegirlo».
