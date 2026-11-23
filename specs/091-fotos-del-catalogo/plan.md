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
- **DoS**: `maxBody` 1 MiB ya existe; límite por usuario de subidas (`rateLimiter`, 120/min; también lo cuenta el despacho del MCP, así que un lote de 200 fotos tarda ~2 min) para
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

Dos pantallas cambian. Maquetas medidas a **1024×600** (pedido del dueño) y a 1280×854 y 2560×1440.

1. **Editor de producto** (`shared/ProductEditDialog.tsx`, opción A): el diálogo pasa a `size="lg"`
   (~512 px) y el bloque de foto va a la izquierda de Nombre y Categoría; Precio/Costo (con el botón
   largo de costo) quedan a todo el ancho debajo. Foto de 132 px; el renglón de estado bajo la foto
   solo aparece cuando hay algo que decir, y así «Receta» sigue a la vista a 1024×600 (lo mide la
   e2e F1). Sin foto: la ilustración (o la foto de la categoría) con la
   etiqueta «Sin foto» y el botón «Agregar foto». Con foto: dos botones de ícono de 44 px, «Cambiar
   foto» y «Quitar foto». Quitar confirma con `ConfirmSheet` destructivo («Volver» como acción
   segura). La foto se guarda al tocar «Guardar foto», sin esperar al «Guardar» del producto (como
   los grupos): el bloque dice «La foto se guarda aparte» y, al guardar, «Foto guardada».
   El selector de archivo es un `<input type=file>` invisible (`opacity:0`, no `display:none`) que
   dispara el botón; `accept="image/jpeg,image/png,image/webp"`; su `value` se limpia tras elegir.
   Una foto que el navegador no abre (HEIC) dice «Esta foto no se puede abrir. Elige una en JPG o
   PNG». Si el navegador no puede preparar fotos, se detecta al montar: «Agregar foto» queda
   deshabilitado con «Este navegador no puede preparar la foto. Ábrelo en Chrome o Edge».
2. **Recorte** (opción B): al elegir el archivo se abre un diálogo «Así se verá» con la foto centrada
   en cuadrado (`min(340px, 52dvh)`; `44dvh` con el ajuste abierto), «Ajustar encuadre» (rejilla,
   arrastre y botones −/+ de 44 px), «Cancelar» y «Guardar foto». Cuerpo con scroll interno y pie
   siempre visible. Recorte y confirmación se montan dentro del `DialogRoot` del editor (como
   `CompositionSheet`): cerrarlos no cierra el editor ni pierde lo escrito.
3. **Pestaña «Categorías»** en el hub Menú (`CatalogPage`), con **tres vistas** y selector de tres
   botones de 44 px en la barra de pestañas (solo en esta pestaña), mosaico por omisión y la última
   vista recordada en el equipo (`localStorage`, con respaldo a mosaico si falla):
   - **Mosaico**: tarjetas cuadradas (`repeat(auto-fill, minmax(150px, 1fr))`: 6 columnas a 1024)
     con foto o ilustración, nombre y número de productos; scroll dentro de su caja (`Page fill`).
   - **Tabla**: miniatura de 40 px, nombre, familia y productos, como la lista de productos.
   - **Lista y panel**: lista a la izquierda (44 %) y el editor a la derecha, sin diálogo. Todo se
     aplica al momento (foto con «Guardar foto», familia al elegirla): no hay cambios pendientes que
     perder al tocar otra categoría.
   En mosaico y tabla, tocar abre el diálogo de la categoría: foto (mismo bloque que el producto) y
   «Familia de comida» con `Picker`, que también se aplica al elegirla. La confirmación de quitar la
   foto de una categoría dice «Los productos sin foto propia dejarán de mostrar ésta».
   El selector va en el hueco que hoy empuja la ayuda a la derecha (las pestañas no se mueven),
   con `aria-label`, `title` y `aria-pressed`. La pestaña lleva su `what`/`steps` de ayuda y su
   ruta en `routes.ts` con `menu.configure`.

La lista de productos agrega una columna de miniatura de 32 px (con `width`/`height` fijos), para
no subir la altura de cada renglón.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Editor de producto con foto | [maqueta](https://claude.ai/artifact/BM3i3c2r8gNe4n4fpHkWvX) | A · Junto al nombre | Mismos 5 controles visibles que hoy a 1024×600; agregar foto en 3 toques |
| Recorte de la foto | [maqueta](https://claude.ai/artifact/1U5DLLrioJ5prXPQPSsypN) | B · Centrada, ajustar opcional | El caso común se guarda viendo el resultado; ajustar cuesta 1 toque |
| Categorías | [maqueta](https://claude.ai/artifact/1MaoeVygkugHvXKBpYUw4N) | Las tres vistas con selector, mosaico por omisión | Decisión del dueño (2026-11-23): mosaico, tabla y lista con panel; selector de 3 botones de 44 px en la barra de pestañas; cada equipo recuerda la última vista |

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
├── public/ilustraciones/food-families.v1.svg      # sprite, avisos MIT/ISC
├── public/_headers                           # img-src
├── src/media/preparePhoto.ts (+test)         # recorte → dos WebP con tope
├── src/media/foodFamilies.ts                     # lista, etiquetas, símbolo
├── src/components/CatalogImage.tsx (+test) # img con srcset / ilustración (cadena FR-017)
├── src/components/PhotoField.tsx (+test)      # agregar/reemplazar/quitar + recorte
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

## Revisión de arquitectura (2026-11-23)

`db-architect` y `tablet-ui-reviewer`: aprobado con cambios; todos aplicados arriba y en
[data-model.md](data-model.md).

- Base: `CloneProductRow` deja de copiar `image_url`; checks de `image_url` `not valid`; Down que
  se niega con pendientes y sus renglones de limpieza; check de tabla de `object_key` contra
  `company_id`; clasificación del respaldo anónimo.
- Pantalla: editor `size="lg"`; «La foto se guarda aparte»; navegador sin soporte detectado al
  montar; recorte y confirmación dentro del mismo `DialogRoot` con `ConfirmSheet`; recorte con pie
  siempre visible; miniatura de 32 px; mosaico con columnas mínimas de 150 px; autoguardado en la
  vista de panel; selector con `aria-pressed`; input de archivo invisible y reiniciado.

## Complexity Tracking

Sin violaciones.
