# Fotos del catálogo en Cloudflare R2

Runbook del almacenamiento de las fotos de productos y categorías (spec 091). La decisión de usar
R2 y no GCS está en el issue #316.

## Cómo está armado

| Ambiente | Bucket | Dominio público | Dónde viven las variables |
|---|---|---|---|
| Producción | `gatobobah-media` | `https://media.elgatobobah.com` | `deploy/.env` de `pos-vps` |
| Pruebas | `gatobobah-media-dev` | `https://media-dev.elgatobobah.com` | `deploy/.env` de `pos-vps-dev` |
| Local | `media` (versitygw del compose dev) | `http://localhost:93NN/media` | `scripts/dev-api.sh` (sin `.env`) |

- La API **sube y borra**; nunca sirve una imagen. El navegador las pide al dominio de medios, que
  Cloudflare guarda en caché (`Cache-Control: public, max-age=31536000, immutable`).
- Cada foto son dos objetos: `<empresa>/<products|categories>/<id>/<huella>-1200.webp` y
  `…-400.webp`. La huella es el SHA-256 del archivo grande: un objeto publicado no cambia nunca.
- `image_url` guarda la URL de la de 1200; la de 400 se deriva.
- Sin las variables `MEDIA_*` la API arranca con las fotos apagadas (subir responde 503) y el POS
  opera igual.

## Variables

```
MEDIA_S3_ENDPOINT=https://<ACCOUNT_ID>.r2.cloudflarestorage.com
MEDIA_S3_BUCKET=gatobobah-media
MEDIA_S3_REGION=auto
MEDIA_S3_ACCESS_KEY_ID=<Access Key ID del token>
MEDIA_S3_SECRET_ACCESS_KEY=<Secret Access Key del token>
MEDIA_PUBLIC_BASE_URL=https://media.elgatobobah.com
```

Todas o ninguna: la API se niega a arrancar con una a medias, y en producción exige `https` en el
endpoint y en el dominio.

## Crear un bucket nuevo (otro ambiente u otro negocio)

1. Cloudflare › R2 › **Create bucket**, ubicación automática.
2. Bucket › Settings › **Custom Domains** › conectar el subdominio de medios. Dejar **apagado** el
   acceso por `r2.dev`.
3. R2 › **Manage API tokens** › Create API token: permiso **Object Read & Write**, *Apply to
   specific buckets only* con ese bucket, sin vencimiento o con vencimiento anotado aquí. Copiar
   Access Key ID y Secret Access Key (el secreto solo se ve una vez).
4. Poner las variables en el `deploy/.env` de la VM y desplegar.
5. Agregar el dominio a `img-src` de `web/public/_headers` (producción) o a la reescritura de CI
   (pruebas).

## Rotar el token

1. Crear un token nuevo con el mismo permiso y bucket.
2. Cambiar `MEDIA_S3_ACCESS_KEY_ID` y `MEDIA_S3_SECRET_ACCESS_KEY` en el `.env` de la VM y
   desplegar (`make deploy-image`); verificar `readyz` y subir una foto.
3. Revocar el token viejo en Cloudflare.

## Borrados pendientes

Si borrar el archivo viejo falla (el almacenamiento no respondió), la foto nueva queda puesta y la
llave vieja se anota en `media_pending_deletions`. Cada foto que la misma empresa suba o quite
después reintenta hasta 10. Para ver qué hay (solo lectura):

```sql
begin read only;
select company_id, object_key, attempts, last_error, updated_at from media_pending_deletions order by id;
```

El Down de la 0117 se niega mientras haya filas: bajarla dejaría esos archivos en el bucket para
siempre.

## Subir el catálogo de fotos en lote (MCP)

La computadora que tiene las fotos prepara las dos medidas con ImageMagick 7 y las manda con
`product_image_set` (primero con vista previa, luego con `dry_run: false` y una
`idempotency_key`):

```bash
magick foto.jpg -auto-orient -strip -resize 1200x1200^ -gravity center -extent 1200x1200 \
  -quality 82 -define webp:method=6 foto-1200.webp
magick foto.jpg -auto-orient -strip -resize 400x400^ -gravity center -extent 400x400 \
  -quality 80 -define webp:method=6 foto-400.webp
```

Topes: 400 KB la de 1200 y 100 KB la de 400; si una pasa, repetir con `-quality 70`. Cada subida
cuenta contra el límite de 120 por minuto por persona. Contrato: `specs/091-fotos-del-catalogo/contracts/mcp.md`.
