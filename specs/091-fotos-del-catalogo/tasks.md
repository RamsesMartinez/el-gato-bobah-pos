---
description: "Tareas de la feature 091: fotos de productos y categorías"
---

# Tasks: Fotos de productos y categorías

**Input**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md),
[data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md)

**Tests**: TDD obligatorio (constitución IV): cada tarea de test va antes de su implementación y se
ve fallar por la razón correcta.

**Maquetas**: las tareas de pantalla citan la fila de «Maquetas» del plan que implementan
(M1 = Editor de producto con foto, opción A; M2 = Recorte, opción B; M3 = Categorías, tres vistas
con selector).

## Phase 1: Setup (infraestructura local)

- [ ] T001 Agregar `versitygw` (servicio `media`, backend posix, volumen `media_dev`, puerto `${MEDIA_PORT:-9390}:7070`, credenciales de desarrollo) en deploy/docker-compose.dev.yml
- [ ] T002 Agregar `MEDIA_PORT=9300+NN` a scripts/worktree-env.sh y su caso en scripts/tests (si el script tiene prueba), y que scripts/start.sh resuelva el puerto, levante `media` y cree el bucket `media` con su política pública de lectura usando `curl --aws-sigv4` (idempotente)
- [ ] T003 Exportar `MEDIA_S3_ENDPOINT`, `MEDIA_S3_BUCKET`, `MEDIA_S3_REGION`, `MEDIA_S3_ACCESS_KEY_ID`, `MEDIA_S3_SECRET_ACCESS_KEY` y `MEDIA_PUBLIC_BASE_URL` del almacenamiento local en scripts/dev-api.sh
- [ ] T004 [P] Documentar las variables `MEDIA_*` sin valores en deploy/.env.example

## Phase 2: Foundational (bloquea todas las historias)

### Tests primero

- [ ] T005 [P] Tests table-driven de `ValidateWebP` (VP8, VP8L, VP8X, animado, RIFF con tamaño mentido, truncado, PNG renombrado, chunk desconocido, dimensiones distintas, peso de más), `MediaObjectKey`, `MediaURLFor`/`ParseMediaURL` (base ajena, empresa ajena, entidad ajena, variante 400 derivada) y `ValidFoodFamily` en server/internal/domain/media_test.go
- [ ] T006 [P] Test de la firma SigV4 contra el ejemplo publicado por AWS (GET de `examplebucket`, canonical request, string to sign y firma esperados) y de `Put`/`Delete` contra un `httptest.Server` (cabeceras `Content-Type`, `Cache-Control`, `x-amz-content-sha256`, ruta con bucket, error de 403 sin filtrar el secreto) en server/internal/objstore/s3_test.go
- [ ] T007 [P] Tests de `config.Validate` para `MEDIA_*`: todas o ninguna, `https` obligatorio en producción para endpoint y base pública, base sin `/` final, en server/internal/config/config_test.go
- [ ] T008 Test de la migración 0117 sobre respaldo restaurado con dos empresas: checks de `image_url` (rechaza sin `-1200.webp`, acepta null y `http://localhost…`), fila vieja con `image_url` ajena no impide migrar, check de tabla de `object_key` contra `company_id`, cascade de `companies`, Down que se niega con pendientes, Down+Up reproducible, `update` de `food_family` como `gatobobah_app`, en server/internal/integration/migration_0117_test.go

### Implementación

- [ ] T009 [P] Implementar server/internal/domain/media.go (constantes 400/1200, topes 100 KB/400 KB, `FoodFamily`, sentinels `ErrImageInvalid`, `ErrImageTooLarge`, `ErrMediaDisabled`, `ErrMediaUnavailable`, `ValidateWebP`, llaves y URLs) hasta que pase T005
- [ ] T010 [P] Implementar server/internal/objstore/s3.go (`Client{Put, Delete}`, firma SigV4 con stdlib, timeouts, errores sin secretos) hasta que pase T006
- [ ] T011 [P] Agregar `MEDIA_*` a server/internal/config/config.go con `MediaEnabled()` y su validación hasta que pase T007
- [ ] T012 Escribir server/migrations/0117_catalog_media.sql (tipo `food_family`, `categories.food_family`, checks `not valid` de `image_url`, tabla `media_pending_deletions` con RLS, grants, checks, Down que se niega) según data-model.md, hasta que pase T008
- [ ] T013 Agregar renglones de `guardedDownCleanup` para la 0117 en server/internal/integration/harness_restaurado_test.go y `media_pending_deletions.object_key`/`.last_error` a `noPersonalData` en server/internal/integration/respaldo_anonimo_test.go
- [ ] T014 Mapear los sentinels nuevos (422 `IMAGE_INVALID`, 413 `IMAGE_TOO_LARGE`, 503 `MEDIA_DISABLED`, 502 `MEDIA_UNAVAILABLE`) en server/internal/httpapi/respond.go con su test en server/internal/httpapi/respond_test.go
- [ ] T015 Escribir server/queries/catalog_media.sql (lock y lectura de `image_url` de producto y categoría, set/clear, `SetCategoryFoodFamily`, insertar/listar/actualizar/borrar pendientes por empresa) y correr `make sqlc`
- [ ] T016 Crear `CatalogMediaService` vacío y su cableado (cliente S3 desde config, nil si apagado) en server/internal/app/catalog_media.go y server/cmd/api

**Checkpoint**: dominio, cliente, config y esquema listos.

## Phase 3: User Story 1 — Foto de un producto desde el catálogo (P1) 🎯 MVP

**Goal**: subir, reemplazar y quitar la foto de un producto desde «Editar producto».
**Independent Test**: quickstart §1.

### Tests primero

- [ ] T017 [US1] Tests de integración del servicio con S3 falso en proceso: poner (sube 400 y 1200 y guarda la URL), reemplazar (borra las viejas), misma foto dos veces (no borra), quitar, borrado que falla (queda en pendientes y la siguiente escritura lo reintenta), commit que falla (borra las recién subidas), producto de otra empresa (404 sin objeto subido), `image_url` con prefijo ajeno escrita por SQL (no se borra), bitácora de auditoría, en server/internal/integration/catalog_media_test.go
- [ ] T018 [US1] Tests de aislamiento `inTheThreeCases` bajo `appRoleStore` para las consultas de catalog_media.sql y la tabla de pendientes, en server/internal/integration/catalog_media_test.go
- [ ] T019 [P] [US1] Test de handler: cuerpo sin `image400`, base64 inválido, cuerpo de más de 1 MiB (413), 503 con medios apagados, en server/internal/httpapi/handlers_catalog_media_test.go
- [ ] T020 [P] [US1] Test de que duplicar un producto con foto deja al duplicado sin foto y la del original intacta, en server/internal/integration/catalog_media_test.go

### Implementación

- [ ] T021 [US1] Implementar `SetProductImage`/`RemoveProductImage` y el barrido de pendientes en server/internal/app/catalog_media.go hasta que pasen T017 y T018
- [ ] T022 [US1] Quitar `image_url` de `CloneProductRow` en server/queries/admin.sql y `make sqlc` hasta que pase T020
- [ ] T023 [US1] Handlers `PUT`/`DELETE /admin/products/{id}/image` en server/internal/httpapi/handlers_catalog_media.go y rutas con `menu.configure` y límite por usuario de 120/min (cuenta también lo que despacha el MCP; un lote de 200 fotos tarda ~2 min) en server/internal/httpapi/router.go hasta que pase T019
- [ ] T024 [US1] Agregar `imageUrl`, `categoryImageUrl` y `categoryFoodFamily` a `AdminListProducts` (server/queries/admin.sql, servicio y tipo) y su test de integración existente ampliado
- [ ] T025 [P] [US1] Tests de `preparePhoto` (recorte centrado y desplazado, salida 400 y 1200, baja de calidad hasta caber, navegador sin WebP detectado, archivo que no abre) en web/src/media/preparePhoto.test.ts
- [ ] T026 [US1] Implementar web/src/media/preparePhoto.ts (`createImageBitmap` con orientación, canvas, `toBlob('image/webp')`, `canPreparePhotos()`) hasta que pase T025
- [ ] T027 [P] [US1] Tests de `CatalogImage` (cadena foto propia → foto de categoría → familia → genérica, `srcset` 400/1200, `width`/`height`, `loading="lazy"`) en web/src/components/CatalogImage.test.tsx
- [ ] T028 [US1] Implementar web/src/components/CatalogImage.tsx hasta que pase T027
- [ ] T029 [P] [US1] Tests de `PhotoField` (M1, M2): botón de 44 px dispara el input invisible, `value` reiniciado, diálogo «Así se verá» y «Ajustar encuadre», «Guardar foto» llama a la API, «Foto guardada», quitar con `ConfirmSheet` que no cierra el editor, navegador sin soporte deshabilita con su mensaje, HEIC con su mensaje, en web/src/components/PhotoField.test.tsx
- [ ] T030 [US1] Implementar web/src/components/PhotoField.tsx y las llamadas `setProductImage`/`removeProductImage` en web/src/api/admin.ts hasta que pase T029
- [ ] T031 [US1] Integrar `PhotoField` en web/src/shared/ProductEditDialog.tsx (M1: `size="lg"`, bloque de 132 px junto a Nombre y Categoría, Precio/Costo a todo el ancho) con su test en web/src/features/admin/ProductEditDialog.test.tsx
- [ ] T032 [US1] Miniatura de 32 px con `CatalogImage` en web/src/features/admin/ProductsAdminPage.tsx y su test
- [ ] T033 [US1] Agregar el dominio de medios y `blob:` a `img-src` en web/public/_headers y la reescritura del dominio del ambiente de pruebas en .github/workflows/ci.yml

**Checkpoint**: US1 funciona sola en local (quickstart §1 y §3).

## Phase 4: User Story 2 — Nunca un hueco: ilustraciones por familia (P1)

**Goal**: cada categoría elige familia; producto sin foto muestra foto de categoría o ilustración.
**Independent Test**: quickstart §2.

- [ ] T034 [P] [US2] Sprite web/public/ilustraciones/food-families.v1.svg con un `<symbol>` por familia (paths de Tabler y `LuPopcorn`), dos tonos con `currentColor` y `var(--illu-bg)`, avisos MIT e ISC en el encabezado; test que comprueba que hay un símbolo por cada familia de web/src/media/foodFamilies.ts y que el archivo pesa < 40 KB en web/src/media/foodFamilies.test.ts
- [ ] T035 [P] [US2] web/src/media/foodFamilies.ts (ids iguales a `domain.FoodFamily`, etiquetas, símbolo) y test espejo que compara contra la lista del servidor (server/internal/domain/media_test.go exporta la lista en un test de contrato, o el front la lee de un JSON generado)
- [ ] T036 [US2] Tests de integración: `PATCH /admin/categories/{id}` con familia válida, `null`, `""` y desconocida; `GET /admin/categories` con `imageUrl`, `foodFamily` y `productCount`; menú del POS con `foodFamily`; renglón `category.family_set` en la bitácora de auditoría con antes y después, en server/internal/integration/catalog_media_test.go
- [ ] T037 [US2] Implementar `SetCategoryFamily`, el handler `PATCH /admin/categories/{id}`, la ampliación de `AdminCategories` y del menú (server/queries/admin.sql, server/queries/menu.sql, servicio, handler y ruta) hasta que pase T036
- [ ] T038 [P] [US2] Tests de `CategoriesAdminPage` (M3): mosaico por omisión, selector de tres botones de 44 px con `aria-pressed`, vista recordada y respaldo a mosaico si `localStorage` falla, tabla con familia, panel que aplica la familia al elegirla, Picker de familia (sin `<select>`), en web/src/features/admin/CategoriesAdminPage.test.tsx
- [ ] T039 [US2] Implementar web/src/features/admin/CategoriesAdminPage.tsx (tres vistas, diálogo de categoría con familia), la pestaña en web/src/features/admin/CatalogPage.tsx (con `what`/`steps` y el selector en la barra) y la ruta en web/src/app/routes.ts con `menu.configure`, hasta que pase T038

**Checkpoint**: US1 + US2 dan el MVP: fotos por pantalla y cero huecos.

## Phase 5: User Story 4 — Foto de una categoría (P2)

**Goal**: subir, reemplazar y quitar la foto de una categoría.
**Independent Test**: subir la foto de una categoría; sus productos sin foto la muestran.

- [ ] T040 [US4] Tests de integración de `SetCategoryImage`/`RemoveCategoryImage` (mismos casos que T017 para categoría) en server/internal/integration/catalog_media_test.go
- [ ] T041 [US4] Implementar el servicio y `PUT`/`DELETE /admin/categories/{id}/image` hasta que pase T040
- [ ] T042 [US4] `PhotoField` en el diálogo y el panel de categoría, con el texto de confirmación de categoría, y su test en web/src/features/admin/CategoriesAdminPage.test.tsx

## Phase 6: User Story 3 — Subir en lote por el MCP (P2)

**Goal**: cinco herramientas con vista previa, idempotencia y bitácora sin base64.
**Independent Test**: quickstart §4.

- [ ] T043 [US3] Tests de las herramientas (vista previa valida sin subir, imagen inválida falla en la vista previa, aplicar, repetición con la misma clave, conflicto con otra foto y la misma clave, token sin alcance, bitácora del MCP sin base64) en server/internal/integration/mcp_091_test.go
- [ ] T044 [US3] Agregar las cinco herramientas al catálogo de server/internal/domain/apitoken.go (alcance `write:catalog`, permiso `menu.configure`)
- [ ] T045 [US3] Implementar server/internal/httpapi/mcp_tools_091_media.go (rutas en `mcpToolRoutes`, `writeTool` con `strictPreview`, redacción de `Args` a `{bytes, sha256}`) hasta que pasen T043 y `TestEveryToolMatchesItsRouteGate`

## Phase 7: Polish

- [ ] T046 [P] docs/medios-r2.md (buckets, dominios, token por bucket, variables, rotación del token, cómo limpiar pendientes) y su renglón en docs/README.md
- [ ] T047 [P] Notas de lo que no se ve en docs/mecanica/catalogo-y-sucursales.md (duplicar no copia la foto, guarda de prefijo, URL de 400 derivada, el sprite versionado)
- [ ] T048 [P] Script de preparación en lote de la computadora del catálogo documentado en el quickstart y en docs/medios-r2.md
- [ ] T049 Prueba e2e de subir (con un JPEG cuya orientación EXIF lo gira: la foto guardada sale derecha), reemplazar y quitar una foto contra el `make start` del worktree, capturas en `test-results/`, en web/e2e/catalog-photos.spec.ts
- [ ] T050 `/revision-de-codigo` y `/security-review`; corregir lo que encuentren con su test
- [ ] T051 Prueba con alguien nuevo de una fonda a 1024×600 (y 1280×854) en local; corregir dónde se atore
- [ ] T052 `make ci-local` verde

## Dependencies & Execution Order

- Setup (T001–T004) → Foundational (T005–T016) → US1 → US2 → US4 → US3 → Polish.
- US2 depende de `CatalogImage` (T028) de US1. US4 depende de `PhotoField` (T030) y de la pestaña
  (T039). US3 depende de las rutas de US1 y US4.

## Parallel Opportunities

- T005, T006, T007 en paralelo; luego T009, T010, T011 en paralelo.
- En US1: T019, T020, T025, T027 y T029 en paralelo (archivos distintos).
- T034 y T035 en paralelo con el backend de US2.
- T046–T048 en paralelo.

## Implementation Strategy

MVP = US1 + US2 (fotos por pantalla y cero huecos). Luego US4 y US3. Todo entra en una sola subida:
la migración solo agrega, y con `MEDIA_*` vacías la API arranca con las fotos apagadas.
