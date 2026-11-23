# Quickstart: Fotos de productos y categorías

## Prerrequisitos

- `make start` del worktree arriba (levanta también el almacenamiento S3 local, `versitygw`, en
  `localhost:93NN`, con el bucket `media` público para lectura).
- Base restaurada: `make db-restaurar`.
- `dev-api.sh` exporta las variables `MEDIA_*` del almacenamiento local; `deploy/.env` no las
  necesita en desarrollo.

## 1. Pantalla: foto de un producto (US1)

1. Entrar como admin a `http://localhost:30NN/catalogo/productos` a 1024×600 (y 1280×854).
2. Abrir un producto sin foto → «Agregar foto» → elegir una foto JPEG de un celular (vertical,
   4000×3000).
3. **Esperado**: vista previa cuadrada derecha; se puede arrastrar y acercar; «Guardar foto».
4. La lista muestra la miniatura. En la pestaña Red del navegador, la miniatura sale de
   `localhost:93NN/media/…-400.webp`, no de la API.
5. Reemplazar con otra foto → el objeto viejo ya no existe:
   `curl -sI localhost:93NN/media/<llave vieja>` → 404.
6. Quitar la foto (con confirmación) → la lista muestra la ilustración de la categoría.

## 2. Ilustraciones (US2)

1. Pestaña «Categorías» → elegir familia «Café» en una categoría sin foto.
2. **Esperado**: sus productos sin foto muestran la taza. Una categoría sin familia, el plato.
3. Con 100+ productos en la lista, la pestaña Red muestra **una** petición a `food-families.v1.svg`.

## 3. Rechazos del servidor (FR-007)

```bash
TOKEN=… ; API=http://localhost:80NN/api/v1
# Un PNG renombrado
jq -n --arg a "$(base64 -w0 foto.png)" '{image1200:$a,image400:$a}' |
  curl -s -XPUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  --data @- $API/admin/products/1/image          # → 422 IMAGE_INVALID
```

Casos: PNG renombrado (422), WebP 800×600 (422 con las medidas), WebP de 1200 de 500 KB (413),
WebP animado (422), producto de otra empresa (404, sin objeto nuevo en el bucket).

## 4. MCP en lote (US3)

### Preparar fotos en lote

En la computadora del catálogo, por cada foto (ImageMagick 7):

```bash
magick foto.jpg -auto-orient -resize 1200x1200^ -gravity center -extent 1200x1200 \
  -quality 82 -define webp:method=6 foto-1200.webp
magick foto.jpg -auto-orient -resize 400x400^ -gravity center -extent 400x400 \
  -quality 80 -define webp:method=6 foto-400.webp
```

Si una pasa del tope (400 KB / 100 KB), repetir con `-quality 70`.

### Subir

1. `product_image_set` con `product_id`, `image_1200_base64`, `image_400_base64` y sin `dry_run`
   → vista previa con el nombre del producto y las medidas leídas por el servidor.
2. Repetir con `dry_run: false` e `idempotency_key` → aplicado; la foto se ve en la pantalla.
3. Repetir la misma llamada → `replayed: true`, sin otra subida.
4. `audit_list` → el renglón dice que lo hizo el token. La bitácora del MCP guarda `{bytes,
   sha256}`, no el base64.

## 5. Pruebas automáticas

```bash
cd server && go test ./internal/domain/ ./internal/objstore/   # WebP, llaves, SigV4 (vector de AWS)
make test-integration                                          # servicio, RLS en los tres casos, MCP
cd web && bun run vitest run src/media src/components/PhotoField* src/components/CatalogImage*     # recorte, calidad, cadena de respaldo
```
