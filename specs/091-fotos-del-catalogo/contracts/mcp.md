# Contrato: herramientas MCP de fotos

Las cinco son escrituras (`writeTool`): alcance `write:catalog`, permiso `menu.configure`,
`dry_run` (por omisión `true`) e `idempotency_key` para aplicar. Despachan a las rutas de
[api.md](api.md); las validaciones son las mismas.

| Herramienta | Argumentos | Ruta |
|---|---|---|
| `product_image_set` | `product_id`, `image_1200_base64`, `image_400_base64` | `PUT /admin/products/{id}/image` |
| `product_image_remove` | `product_id` | `DELETE /admin/products/{id}/image` |
| `category_image_set` | `category_id`, `image_1200_base64`, `image_400_base64` | `PUT /admin/categories/{id}/image` |
| `category_image_remove` | `category_id` | `DELETE /admin/categories/{id}/image` |
| `category_family_set` | `category_id`, `food_family` (o vacío para la genérica) | `PATCH /admin/categories/{id}` |

## Vista previa

Lee la entidad (nombre, si ya tiene foto) y **valida las dos imágenes en el servidor sin
subirlas** (`strictPreview`): un archivo inválido falla ya en la vista previa con el mismo mensaje
que daría al aplicar. Devuelve `{ product: {id, name, hadImage}, image1200: {bytes, side},
image400: {bytes, side} }`.

## Bitácora del MCP

Los argumentos se guardan **sin** el base64: cada imagen se reemplaza por `{bytes, sha256}`. La
huella de idempotencia sí se calcula con los argumentos completos (misma foto + misma clave =
repetición; otra foto con la misma clave = conflicto).

## Cómo viaja el archivo

Base64 estándar dentro de los argumentos JSON. Tope del mensaje: 1 MiB (el `maxBody` del router),
que alcanza para 400 KB + 100 KB de foto (~667 KB en base64). La computadora del catálogo prepara
las dos variantes con el comando del [quickstart](../quickstart.md#preparar-fotos-en-lote).
