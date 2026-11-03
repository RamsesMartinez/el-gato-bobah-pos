# Contratos: Nombres alternativos (063)

## PUT /api/v1/admin/products/{id}/alt-names

Permiso: `menu.configure`. Reemplaza la lista completa.

```json
{ "altNames": ["Café frío", "Café helado"] }
```

| Respuesta | Cuándo |
| --- | --- |
| 200 `{ "altNames": ["Café frío", "Café helado"] }` | Guardado; la lista ya limpia |
| 400 `VALIDATION` | Sin `altNames` (vaciar exige `[]`), más de 10, uno de más de 60 letras, con caracteres que no se ven (NUL, controles, ancho cero), repetido en la lista o igual al nombre propio |
| 404 `NOT_FOUND` | No existe, es de otra empresa o es el producto de sistema |
| 409 `ALT_NAME_TAKEN` `details: { name, existingId, archived }` | Choca con el nombre o un alternativo de otro producto |

## PUT /api/v1/stock/articles/{id}/alt-names

Permiso: `inventory.configure`. Cuerpo `{ "altNames": [...], "expectedUpdatedAt": "…" }`: la versión
de la ficha que vio la pantalla, obligatoria; con otra, 409 `STALE` y no guarda (revisión de código:
sin ella, guardar sobre una ficha que otra tableta renombró y luego autoguardar deshacía el renombre).
Mismos errores (`existingId` es el artículo). La respuesta
trae además la versión nueva de la ficha: `200 { "altNames": [...], "updatedAt": "…" }`. Funciona
también sobre un archivado.

## GET /api/v1/admin/products/{id}/alt-names/similar?name=…

Permiso: `menu.configure`. Hasta 5 parecidos, sin el propio ni los idénticos. 400 si `name` está
vacío o pasa de 60 letras.

```json
[{ "id": 12, "name": "Café frío de olla" }, { "id": 40, "name": "Frappé de café", "viaAlt": "Café frío batido" }]
```

## GET /api/v1/stock/articles/{id}/alt-names/similar?name=…

Permiso: `inventory.configure`. Igual.

## Lecturas que ganan campos

| Ruta | Campo |
| --- | --- |
| `GET /pos/menu` | `products[].altNames?: string[]` (omitido si vacío) |
| `GET /admin/products` | `items[].altNames: string[]`, `items[].matchedAlt?: string` (con `search`) |
| `GET /stock/articles` | `items[].altNames: string[]`, `items[].matchedAlt?: string` (con `q`) |
| `GET /stock/articles/{id}` | `altNames: string[]` |
| `GET /ingredients` | `[].altNames: string[]` |
| `GET /articles` | `[].altNames: string[]` |
| `GET /stock/counts/current` | `items[].altNames: string[]` |

Todo arreglo prometido sale `[]`, nunca `null` (AGENTS §1); `altNames` del menú es la excepción
declarada (opcional, para no engordar el documento del menú) y el tipo del front lo marca opcional.

## Cambios en rutas existentes

- `POST /admin/products`, `PATCH /admin/products/{id}` (nombre cambiado),
  `POST /admin/products/{id}/duplicate`: 409 `ALT_NAME_TAKEN` si el nombre es alternativo de otro.
- `POST /stock/articles`, `PATCH /stock/articles/{id}` (nombre cambiado) y las altas dentro de un
  gasto o una receta: 409 `NAME_TAKEN` con `existingId` si el nombre es alternativo de otro; dentro
  de un gasto o receta se usa el existente activo, como hoy.
