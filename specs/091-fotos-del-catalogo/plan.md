# Implementation Plan: Fotos de productos y categorías

**Branch**: `091-fotos-del-catalogo` | **Date**: 2026-11-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/091-fotos-del-catalogo/spec.md`

## Summary

Subir, reemplazar y quitar la foto de un producto y de una categoría (pantalla y MCP), guardada en
Cloudflare R2 con dominio propio, más una ilustración por familia de comida para que nada se vea
con un hueco. El navegador prepara dos WebP cuadrados (400 y 1200); la API los valida por contenido,
los sube con una firma SigV4 propia y guarda la URL en `image_url`. Detalle de decisiones en
[research.md](research.md).

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query. **Sin
dependencias nuevas** (SigV4 y lectura de cabecera WebP sobre stdlib; recorte con canvas).

**Storage**: Postgres (URL y familia); Cloudflare R2 (archivos); versitygw en local.

**Testing**: `go test` (domain, objstore), integración con Postgres real bajo `appRoleStore` y un
S3 falso en proceso; vitest; Playwright contra el `make start` del worktree.

**Target Platform**: API en e2-micro (1 vCPU, 1 GB); frontend en Cloudflare Pages; equipo de
mostrador táctil.

**Project Type**: web (server + web)

**Performance Goals**: miniatura ≤ 100 KB; las 11 ilustraciones ≤ 40 KB en una descarga (medido:
1.3 KB gzip); la API no convierte imágenes.

**Constraints**: cuerpo ≤ 1 MiB (router); ninguna imagen sale de la API ni de la VM; R2 caído no
detiene la venta.

**Scale/Scope**: ~180 productos y ~30 categorías por negocio; subida en lote de ~200 fotos por MCP.

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I. Layering | `domain/media.go` (WebP, llaves, familias, puro); `objstore/` (cliente S3, I/O); `app/catalog_media.go` (orquesta, tx, borrados); handlers finos; SQL por sqlc |
| II. Errores | Sentinels `ErrImageInvalid`, `ErrImageTooLarge`, `ErrMediaDisabled`, `ErrMediaUnavailable` mapeados solo en `httpapi.Error` |
| III. Dinero | No toca dinero |
| IV. TDD | Tests primero: cabecera WebP (casos de borde abajo), vector SigV4 de AWS, servicio con S3 falso, aislamiento en los tres casos (`inTheThreeCases`) para la tabla nueva y las rutas; migración con su test sobre respaldo con dos empresas |
| IV. Traslape | 0117 solo agrega (tipo, columna nullable, tabla, checks que hoy cumplen todas las filas) |
| V. Seguridad | Ver sección propia |
| VI. YAGNI | Sin SDK, sin cron, sin tabla de medios; `image_url` simple; una foto por entidad |
| VII. Idioma | Identificadores en inglés (`food_family`, `media_pending_deletions`); textos en español |
| VIII. Puertas | ¿Se puede agregar después al mismo costo? Varias fotos por producto: sí (tabla nueva que se llena desde `image_url`). Cambiar de dominio: sí (`replace` sobre `image_url`). Familias nuevas: sí (`add value`). Ninguna puerta de la tabla se cierra |
| Restricciones: tableta | Botones ≥ 44 px, sin `<select>` nativo (familia con `Picker`), sin `<input type=file>` visible (lo dispara un botón), un solo estilo |
| Restricciones: texto UI | Mensajes de acción («Elige una foto de al menos 400 px»), nada de WebP/R2/base64 en pantalla |

Sin violaciones; sin Complexity Tracking.

## Seguridad (principio V)

- **Autorización**: rutas nuevas con `RequirePermission(menu.configure)`; el front solo esconde.
  `TestEveryToolMatchesItsRouteGate` cubre las herramientas nuevas.
- **Validación en el servidor**: tipo por contenido, dimensiones exactas, peso por variante,
  WebP animado rechazado; `Content-Type` y `Cache-Control` del objeto los fija la API. Huella la
  calcula el servidor.
- **Aislamiento**: la llave lleva la empresa de la SESIÓN (`store.CompanyFrom`), nunca un dato del
  cliente; la entidad se comprueba bajo RLS antes de subir. Borrar solo llaves que empiezan con la
  base de medios + empresa + entidad: una `image_url` manipulada no puede borrar archivos ajenos.
- **Secretos**: `MEDIA_S3_ACCESS_KEY_ID` y `MEDIA_S3_SECRET_ACCESS_KEY` en `deploy/.env` (como
  `SMTP_PASS` o `ANTHROPIC_API_KEY`), token de R2 con permiso «Object Read & Write» solo sobre su
  bucket. `config.Validate`: todas o ninguna de las `MEDIA_*`; en producción endpoint y base
  `https`. Nunca se loguean. **No va en KMS**: KMS cifra secretos de terceros *por empresa* que
  viven en la base; éste es del despliegue y vive en el `.env`, como el resto.
- **DoS**: `maxBody` 1 MiB ya existe; límite por usuario de subidas (`rateLimiter`, 60/min) para
  que una sesión no queme operaciones de escritura de R2; el MCP ya pasa por `s.limit`.
- **Lectura pública**: las fotos de un menú no son secretas; la llave no expone nada más que ids.
- **CSP**: `img-src` de `web/public/_headers` agrega el dominio de medios y `blob:` (vista previa
  local); CI reescribe el dominio para el ambiente de pruebas igual que hoy con la API.

## Casos de borde (antes del código)

| Familia | Caso | Dónde se prueba |
|---|---|---|
| Valor vacío | `image400` vacío o `""` en base64 | handler test |
| Valor vacío | `foodFamily: ""` vs `null` vs ausente | domain + handler |
| Archivo falso | PNG con extensión .webp; `RIFF` con tamaño mentido; chunk desconocido; truncado a 20 bytes | `domain/media_test.go` |
| Archivo falso | WebP VP8X animado; VP8L 1200×1199 | domain |
| Estado | Red caída tras subir y antes del `commit`: objetos huérfanos se borran | servicio con S3 falso |
| Estado | Borrado viejo falla: foto nueva queda, renglón en pendientes, siguiente escritura lo reintenta | servicio |
| Estado | Misma foto dos veces: misma llave, no borra | servicio |
| Hermano | El editor de producto que ya existe y el que trae #326 (merge) | e2e |
| Hermano | `DuplicateProduct` copia `image_url`: dos productos con la misma URL; quitar la de uno borraría la del otro | **Se corrige**: duplicar NO copia la foto (la llave lleva el id del producto); test de integración |
| Aislamiento | Empresa B pone foto al id de un producto de A → 404 y ningún objeto en el bucket | integración `inTheThreeCases` |
| Aislamiento | `image_url` de A con prefijo de B escrita por SQL → la API de B no la borra | servicio |
| Traslape | La API vieja escribe un producto mientras la nueva ya migró: checks solo sobre `image_url`, que la vieja no escribe | test de migración |
| Pantalla | Safari sin WebP en canvas → mensaje, no un PNG subido | vitest |
| Pantalla | Foto de 15 MB: se reduce en el navegador antes de mandarla | vitest |

## Pantallas y maquetas

Dos pantallas cambian. Las maquetas se miden a **1024×600** (pedido del dueño para esta feature) y
a 1280×854 (constitución).

1. **Editor de producto** (`shared/ProductEditDialog.tsx`): un bloque de foto arriba del nombre, y
   el flujo de recorte (diálogo con la foto, arrastre, acercamiento y «Guardar foto»).
2. **Pestaña nueva «Categorías»** en el hub Menú (`CatalogPage`): lista con miniatura, nombre,
   familia y cantidad de productos; tocar abre el editor de la categoría (foto + familia con
   `Picker`). Hoy no existe pantalla de categorías.

La lista de productos solo agrega una columna de miniatura de 40 px (sin cambiar la disposición).

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Editor de producto con foto | [maqueta](https://claude.ai/artifact/BM3i3c2r8gNe4n4fpHkWvX) | (espera al dueño) | |
| Recorte de la foto | [maqueta](https://claude.ai/artifact/1U5DLLrioJ5prXPQPSsypN) | (espera al dueño) | |
| Categorías | [maqueta](https://claude.ai/artifact/1MaoeVygkugHvXKBpYUw4N) | (espera al dueño) | |

## Project Structure

### Documentation (this feature)

```text
specs/091-fotos-del-catalogo/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
├── contracts/mcp.md
└── tasks.md
```

### Source Code

```text
server/
├── migrations/0117_catalog_media.sql
├── queries/catalog_media.sql                 # set/clear image, family, pendientes
├── queries/admin.sql                         # AdminCategories y AdminListProducts amplían columnas
├── internal/domain/media.go (+_test)         # ValidateWebP, MediaKey, ParseMediaURL, FoodFamily, sentinels
├── internal/objstore/s3.go (+_test)          # Client{Put, Delete}, firma SigV4
├── internal/config/config.go                 # MEDIA_* + Validate
├── internal/app/catalog_media.go             # CatalogMediaService
├── internal/app/products_admin.go            # Duplicate sin foto
├── internal/httpapi/handlers_catalog_media.go
├── internal/httpapi/router.go                # rutas + límite
├── internal/httpapi/mcp_tools_091_media.go   # cinco herramientas + redacción de Args
├── internal/domain/apitoken.go               # catálogo de herramientas
└── internal/integration/catalog_media_test.go, migration_0117_test.go, mcp_091_test.go

web/
├── public/ilustraciones/familias.v1.svg      # sprite, avisos MIT/ISC
├── public/_headers                           # img-src
├── src/media/prepararFoto.ts (+test)         # recorte → dos WebP con tope
├── src/media/familias.ts                     # lista, etiquetas, símbolo
├── src/components/FotoDeCatalogo.tsx (+test) # img con srcset / ilustración (cadena FR-017)
├── src/components/FotoField.tsx (+test)      # agregar/reemplazar/quitar + recorte
├── src/shared/ProductEditDialog.tsx
├── src/features/admin/ProductsAdminPage.tsx  # miniatura
├── src/features/admin/CategoriesAdminPage.tsx (+test)
├── src/features/admin/CatalogPage.tsx        # pestaña
└── src/api/admin.ts                          # llamadas nuevas

deploy/docker-compose.dev.yml                 # versitygw
scripts/worktree-env.sh, start.sh, dev-api.sh # MEDIA_PORT y variables
deploy/.env.example                           # MEDIA_* sin valores
docs/medios-r2.md + docs/README.md            # bucket, token, dominio, rotación
docs/mecanica/catalogo-y-sucursales.md        # lo que no se ve de las fotos
.github/workflows/ci.yml                      # dominio de medios del ambiente de pruebas en _headers
```

**Structure Decision**: monorepo existente, `server/` + `web/`.

## Producción: lo que hace el dueño en Cloudflare

Se pide cuando la implementación llegue a ese paso (local avanza sin esto):

1. Bucket R2 `gatobobah-media` (producción) y `gatobobah-media-dev` (ambiente de pruebas).
2. Dominio propio por bucket: `media.elgatobobah.com` y `media-dev.elgatobobah.com`
   (R2 › bucket › Settings › Custom Domains). Acceso por `r2.dev` apagado.
3. Un API token de R2 por bucket, permiso «Object Read & Write», aplicado solo a ese bucket.
   Entregar: Account ID, Access Key ID y Secret Access Key de cada uno.
4. Las variables van al `deploy/.env` de cada VM; el `_headers` de Pages lleva el dominio.

## Complexity Tracking

Sin violaciones.
