# Research: Fotos de productos y categorías

## R1. Proveedor de almacenamiento

- **Decision**: Cloudflare R2 con dominio propio de medios, decidido por el dueño en el #316
  (comentario «Decisión: medios en Cloudflare R2»). Dos buckets: uno de producción y uno del
  ambiente de pruebas, cada uno con su dominio y su token.
- **Rationale**: salida sin costo, CDN y DDoS ya incluidos, DNS y frontend ya en Cloudflare.
- **Alternatives**: GCS (cobra salida, sin protección de bucket público), S3+CloudFront (otro
  proveedor), B2 (necesita Cloudflare delante). Detalle en el #316.

## R2. Cliente S3: SigV4 propia sobre stdlib

- **Decision**: paquete `server/internal/objstore` con firma AWS SigV4 propia (`crypto/hmac`,
  `crypto/sha256`, `net/http`) para dos operaciones: `PUT` y `DELETE` de un objeto, en
  direccionamiento por ruta (`<endpoint>/<bucket>/<key>`). Región `auto` para R2.
- **Rationale**: principio VI («sin dependencia nueva para lo que hace la stdlib»). El SDK de AWS
  son ~15 módulos para dos llamadas. La firma se prueba contra el ejemplo publicado por AWS en la
  documentación de SigV4 (vector conocido), y en local contra un servidor S3 real (R3), que la
  verifica de verdad.
- **Alternatives**: `aws-sdk-go-v2` (peso y superficie), `minio-go` (otra dependencia grande).

## R3. Almacenamiento S3 en local

- **Decision**: `versity/versitygw` (Apache-2.0, 38 MB) en `deploy/docker-compose.dev.yml`, con
  backend posix y volumen propio; puerto por worktree `MEDIA_PORT = 9300+NN`. `start.sh` crea el
  bucket `media` y su política de lectura pública con `curl --aws-sigv4` (sin otra imagen).
- **Medido el 2026-11-23**: con versitygw, `PUT` firmado, política pública y `GET` anónimo responden
  200 y devuelven el `Cache-Control: public, max-age=31536000, immutable` del `PUT`.
- **Alternatives**: MinIO (ya no publica imagen: `pull access denied` el 2026-11-23), RustFS
  (488 MB), SeaweedFS (1.08 GB), un servidor falso propio (no verificaría la firma).
- Las pruebas de integración usan un servidor S3 falso en proceso (`httptest`) que registra las
  llamadas: lo que se prueba ahí es la orquestación (qué se sube, qué se borra, cuándo), no la firma.

## R4. Cómo viaja la foto

- **Decision**: un solo endpoint JSON, `PUT /admin/products/{id}/image`, con las dos variantes
  WebP en base64 (`image1200`, `image400`). El MCP lo usa por el despachador interno, igual que
  las demás escrituras.
- **Rationale**: el despachador del MCP solo sabe mandar JSON; un multipart obligaría a otro
  camino. Topes: 400 KB + 100 KB = 500 KB de foto → ~667 KB en base64, dentro del `maxBody` de
  1 MiB que ya tiene el router. El tope por variante se vuelve a aplicar después de decodificar.
- **MCP**: el agente manda los dos archivos ya preparados. La computadora del catálogo los genera
  con el comando documentado en el quickstart (ImageMagick o `cwebp`), porque el servidor no
  codifica WebP (sin cgo, y el e2-micro no debe gastar CPU en eso).
- **Alternatives**: URL firmada de subida directa a R2 (dos pasos, el servidor ya no ve el archivo
  antes de que exista → la validación se vuelve un borrado posterior); multipart (no lo habla el
  despachador del MCP).
- **Consecuencia**: la bitácora del MCP (`Args`) guardaría el base64 completo. Se guarda en su
  lugar `{bytes, sha256}` de cada variante.

## R5. Validación del archivo en el servidor

- **Decision**: `domain.ValidateWebP(data, side, maxBytes)` puro: firma `RIFF….WEBP`, tamaño RIFF
  igual al del archivo, primer chunk `VP8 `, `VP8L` o `VP8X`; dimensiones leídas del encabezado
  del chunk; `VP8X` con bandera de animación se rechaza; exige exactamente `side×side`. Sin
  decodificar los píxeles.
- **Rationale**: solo se acepta un formato; el archivo se sirve desde otro dominio con su
  `Content-Type: image/webp` fijado por la API (nunca el que dice el cliente). Un archivo que pasa
  la cabecera y no decodifica se vería roto, no ejecutaría nada. La huella SHA-256 la calcula el
  servidor.
- **Alternatives**: `golang.org/x/image/webp` (dependencia nueva para leer 30 bytes),
  `http.DetectContentType` (solo confirma la firma, no las dimensiones).

## R6. WebP en el navegador

- **Decision**: `createImageBitmap(file, { imageOrientation: 'from-image' })` → canvas cuadrado
  con el recorte → `canvas.toBlob('image/webp', q)`, bajando la calidad de 0.85 hasta 0.5 si
  rebasa el tope.
- **Consecuencia que no se ve**: Safari (iPhone, iPad) no codifica WebP en canvas: `toBlob`
  devuelve PNG. La pantalla lo detecta y dice «Este navegador no puede preparar la foto; usa
  Chrome o Edge». El equipo de mostrador usa Chromium. Un codificador en WASM (~100 KB) queda
  como camino si un cliente lo pide.

## R7. Ilustraciones por omisión (revisión de arquitectura, 2026-11-23)

Comparación pedida antes del plan, por peso, licencia y rendimiento:

| Criterio | SVG propio | Stock: Tabler (MIT) + Lucide (ISC) | Generadas por IA |
|---|---|---|---|
| Peso de las 11 | 3–6 KB gzip (estimado) | **1.3 KB gzip** (medido, sprite `<symbol>`) | 110–330 KB en WebP (estimado) |
| Licencia | Propia | Aviso de copyright en el archivo; sin atribución en pantalla ni share-alike | Sin derecho de autor; términos variables |
| Recolorear / oscuro | Total | Total (`currentColor` + tokens `brand.fg`/`brand.subtle`) | No |
| Cobertura | 11/11 | 10/11; crepa aproximada | 11/11 |
| Consistencia | Riesgo de verse tosco | Mismo trazo que los íconos que ya usa la UI | Baja |

- **Decision**: sprite propio `web/public/ilustraciones/food-families.v1.svg` con `<symbol>` por
  familia, paths de Tabler (bubble tea, malteada, tazón con palillos, etc.) y `LuPopcorn` para
  snack, a dos tonos (trazo `currentColor`, fondo `var(--illu-bg)`). Encabezado con los avisos MIT
  e ISC. Se consume con `<svg><use href="…#coffee"/></svg>`: una descarga para toda la pantalla.
- Descartadas: Twemoji/Streamline/Game Icons (piden atribución), OpenMoji (share-alike), Noto y
  Fluent (color fijo, 2–10 KB cada una), unDraw (no cubre comida).
- **Nombre versionado** (`.v1.svg`): `public/` no lleva hash; un dibujo nuevo cambia el nombre.
- **Para el menú público (#316)**: `<use href>` no cruza orígenes. Si el menú vive en otro dominio,
  inyecta el sprite en su HTML (~4 KB) o sirve su copia.

## R8. Qué se guarda en la base

- **Decision**: `image_url` guarda la URL pública de la variante de 1200; la de 400 se deriva
  reemplazando `-1200.webp` por `-400.webp`. Llave del objeto:
  `<company_id>/<products|categories>/<id>/<sha256[:32]>-<lado>.webp`.
- Antes de borrar, la API comprueba que la URL vieja empieza con su `MEDIA_PUBLIC_BASE_URL` y con
  el prefijo de la empresa y la entidad: nunca borra una llave que no armó.
- Cambiar de dominio de medios (#331) es un `update … set image_url = replace(…)`: reversible y
  barato (principio VIII: se puede después al mismo costo).

## R9. Borrados que fallan

- **Decision**: tabla `media_pending_deletions` (empresa, llave, intentos, último error). Al
  terminar con éxito cualquier escritura de foto de una empresa, se reintentan hasta 10 pendientes
  de ella. Sin goroutine ni cron (principio VI).
- **Alternatives**: reglas de ciclo de vida de R2 (no saben qué llave ya no se usa), un barrido
  periódico (proceso nuevo para un caso raro).

## R10. Pantalla de venta

- **Decision**: el mosaico de venta no cambia en esta etapa. Las fotos se ven en el catálogo (lista
  y editor de productos, nueva pestaña de categorías) y quedan listas para el menú público.
- **Rationale**: una foto en el mosaico le quita renglones al operador en la pantalla más usada; es
  una decisión de diseño aparte.
