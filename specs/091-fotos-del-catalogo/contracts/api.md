# Contrato: API de fotos del catálogo

Todas bajo `/api/v1`, con sesión, gateadas con `RequirePermission(menu.configure)`. Errores por
`httpapi.Error` (principio II). Con el almacenamiento sin configurar, las escrituras responden
`503 MEDIA_DISABLED` («Las fotos no están configuradas en este servidor»).

## PUT /admin/products/{id}/image · PUT /admin/categories/{id}/image

Cuerpo (JSON; el `maxBody` de 1 MiB del router lo acota antes de leerlo):

```json
{ "image1200": "<base64 estándar>", "image400": "<base64 estándar>" }
```

Validación (todo en el servidor, sobre los bytes decodificados):

| Regla | Error |
|---|---|
| Falta un campo o el base64 no decodifica | 400 `VALIDATION` «falta la foto de 1200» / «la foto de 400 no se pudo leer» |
| No es WebP (firma `RIFF…WEBP`, tamaño RIFF = tamaño del archivo) | 422 `IMAGE_INVALID` «solo se aceptan fotos WebP» |
| WebP animado | 422 `IMAGE_INVALID` «la foto no puede ser animada» |
| Dimensiones distintas de 1200×1200 / 400×400 | 422 `IMAGE_INVALID` «la foto de 1200 mide 800×600; debe medir 1200×1200» |
| Peso > 400 KB (1200) / > 100 KB (400) | 413 `IMAGE_TOO_LARGE` «la foto de 1200 pesa 512 KB; el máximo es 400 KB» |
| Producto/categoría inexistente (o de otra empresa) | 404 `NOT_FOUND`, sin subir nada |
| Almacenamiento no responde | 502 `MEDIA_UNAVAILABLE`, el registro queda como estaba |

Respuesta 200:

```json
{ "imageUrl": "https://media.elgatobobah.com/2/products/15/<hash32>-1200.webp" }
```

Objetos que escribe (con `Content-Type: image/webp` y `Cache-Control: public,
max-age=31536000, immutable`, fijados por la API):

- `2/products/15/<hash32>-400.webp`
- `2/products/15/<hash32>-1200.webp`

`<hash32>` = primeros 32 hex del SHA-256 de la variante de 1200, calculado por el servidor.

## DELETE /admin/products/{id}/image · DELETE /admin/categories/{id}/image

Idempotente: sin foto responde 200 igual. Respuesta `{ "imageUrl": null }`.

## PATCH /admin/categories/{id}

```json
{ "foodFamily": "coffee" }   // o null para volver a la genérica
```

Valor fuera de la lista → 400 `VALIDATION`. Respuesta: la categoría como en la lista.

## GET /admin/categories (existente, ampliado)

Cada renglón agrega `imageUrl`, `foodFamily` y `productCount` (productos activos), sin quitar nada
de lo que ya devuelve.

## GET /admin/products y GET /pos/menu (existentes)

`imageUrl` ya viaja en el menú del POS (`MenuProduct.imageUrl`, `Category.imageUrl`); el menú agrega
`foodFamily` en cada categoría. `GET /admin/products` agrega `imageUrl` y `categoryImageUrl` /
`categoryFoodFamily` para pintar la miniatura sin otra llamada.
