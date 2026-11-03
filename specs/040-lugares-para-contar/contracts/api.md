# API: Lugares para contar

Todo bajo `/api/v1`, con sesión. Los arreglos se entregan como `[]` aunque estén vacíos.

| Método y ruta | Permiso | Cuerpo | Respuesta |
|---|---|---|---|
| `GET /stock/count-places` | `inventory.count` **o** `inventory.configure` | — | `[{ id, name, position, ingredients }]`, los activos en orden. `ingredients` cuenta también los artículos inactivos |
| `POST /stock/count-places` | `inventory.count` | `{ name }` | 201 `{ id }`; 409 `{ details: { existingId } }` si el nombre ya existe. Va al final del recorrido |
| `PUT /stock/count-places/{id}` | `inventory.configure` | `{ name }` | 204; 409 con `existingId`; 404 si es de otra empresa |
| `POST /stock/count-places/{id}/move` | `inventory.configure` | `{ toPosition: n }`, el ordinal entre los activos (0 = primero) | 204; fuera de rango → 400 |
| `POST /stock/count-places/{id}/archive` | `inventory.configure` | `{ moveTo: id \| null }`, obligatorio si el lugar tiene artículos | 204; 409 `{ details: { ingredients } }` si tiene artículos y no vino `moveTo`; 400 si `moveTo` es el mismo lugar; 409 si `moveTo` está archivado |
| `PUT /stock/ingredients/{id}/place` (cambia) | `inventory.count` | `{ placeId: id \| null }`; antes era `{ place: texto }` | 204; 404 si el lugar es de otra empresa; 409 si está archivado |

- `GET /stock/ingredients` (`app/articles.go`) y la hoja del conteo (`app/stock_counts.go`) traen
  `countPlaceId` y `countPlace` (el nombre).
- Un `name` vacío o de más de 30 letras → 400. Un `toPosition` negativo o mayor que el último → 400.
