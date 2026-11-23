---
description: "Tareas del menú público v1 (spec 097)"
---

# Tasks: Menú público v1 con enlace y QR

**Input**: `specs/097-menu-publico/` — spec.md, plan.md, research.md, data-model.md, contracts/api.md, quickstart.md

**Tests**: TDD obligatorio (constitución IV): cada tarea de código va precedida de su tarea de test, que
se ve fallar por la razón correcta antes de escribir el código.

**Formato**: `[ID] [P?] [Story] Descripción con ruta`. Rutas desde la raíz del repo.

## Phase 1: Setup

- [X] T001 Agregar `uqr@0.1.3` a `web/package.json` (con `bun add`, lockfile `web/bun.lock`)
- [X] T002 Crear el tercer producto de `web/`: `web/tsconfig.menu.json` (incluye `web/menu/**`, sin React), y los scripts `typecheck:menu`, `build:menu` (copia `web/menu/public` a `web/dist-menu` y deja `functions/` junto) y `dev:menu` (`wrangler pages dev` en `MENU_PORT` con `API_ORIGIN`/`MEDIA_ORIGIN` locales) en `web/package.json`
- [X] T003 Barrera de eslint en las dos direcciones entre el POS (`web/src/**`) y el menú (`web/menu/**`), con su prueba como la de la consola, en `web/eslint.config.js`
- [X] T004 [P] `MENU_PORT=8700+NN` en `scripts/worktree-env.sh`, y `MENU_BASE_URL=http://localhost:$MENU_PORT` en `scripts/dev-api.sh`; actualizar la tabla de puertos de `AGENTS.md` §2
- [X] T005 Test primero en `server/internal/config/config_test.go`: `MENU_BASE_URL` ausente → `https://menu.elgatobobah.com`; en producción exige `https`; `MENU_EDGE_KEY` presente con < 32 caracteres → error; ausente → arranca
- [X] T006 Implementar `MENU_BASE_URL` y `MENU_EDGE_KEY` en `server/internal/config/config.go` (`Validate`), log de arranque si falta el secreto, y `.env.example` sin valores en `deploy/`

## Phase 2: Foundational (bloquea a todas las historias)

- [X] T007 Tests de tabla primero en `server/internal/domain/public_menu_test.go`: `SlugifyMenuName` (acentos de la tabla única, «Doña Lupe»→`dona-lupe`, emoji→vacío, 40 y 41 caracteres, guion en el corte), `ValidMenuNameShape`, `ReservedMenuNames`, `ValidPalette`, `PlanMenuRename` (igual, inválido, reservado, de otra empresa, revivir, nuevo, tope de 20) y `FeaturePublicMenu == "public_menu"`
- [X] T008 Implementar `server/internal/domain/public_menu.go` con sentinels `ErrMenuNameUnavailable` y `ErrMenuNameLimit`, y su mapeo (409 `MENU_NAME_UNAVAILABLE` / `MENU_NAME_LIMIT`) en `server/internal/httpapi/respond.go`
- [X] T009 Test de migración primero en `server/internal/integration/migracion_0121_test.go` sobre base restaurada con dos empresas: cada empresa con menú y nombre vigente derivado del nombre del negocio (no del slug), token de 12 hex, `free_name` con empate largo y base vacía, vista sin `slug`, el rol `gatobobah_public_menu` sin login, Down y vuelta a subir
- [X] T010 Escribir `server/migrations/0121_public_menu.sql` según data-model (tablas, RLS sin `force`, rol y vista `security_barrier`, `grant select (id, is_active)`, funciones, relleno en loop, Down que no borra el rol)
- [X] T011 [P] Test primero: SQL `public_menu_slugify` y Go `SlugifyMenuName` dan lo mismo sobre la misma tabla de casos, en `server/internal/integration/public_menu_slugify_test.go`
- [X] T012 [P] Clasificar las columnas nuevas en `noPersonalData` del respaldo anónimo (test `TestEveryColumnIsCoveredByTheAnonymousBackup`) y anotar la 0121 donde lo pida `TestEveryGuardedDownHasItsTestCleanup`, en `server/internal/integration/`
- [X] T013 Consultas sqlc en `server/queries/public_menu.sql` (resolver por nombre y por token sobre la vista, `LockPublicMenu`, `GetPublicMenu`, `EnsurePublicMenu`, `PublicMenuNames`, `CountRetiredMenuNames`, retirar, revivir, insertar, `SetMenuPalette`, `BusinessNameForMenu`, `PublicMenuCategories`) y `CreateCompany` con su menú y nombre en `server/queries/companies.sql`; `make sqlc`
- [X] T014 Test primero en `server/internal/integration/web_sales_test.go`: `WebOfferProducts` ampliada trae nombre, descripción, foto, categoría y orden; excluye categoría inactiva; `WebSalesCounts` sigue contando sobre la misma base
- [X] T015 Ampliar `WebOfferProducts` y `WebSalesCounts` en `server/queries/web_sales.sql` y extraer `offer(ctx, q)` en `server/internal/app/web_sales.go` (lo usan `Offer` y el menú público)
- [X] T016 Aislamiento: `public_menus` y `public_menu_names` pasan `TestEveryCompanyTableIsIsolated` (correr `make test-integration` y corregir lo que pida)

## Phase 3: User Story 1 — Un cliente abre el menú en su celular (P1) MVP

**Independent test**: `GET /api/v1/public/menus/el-gato-bobah` y la Function local muestran categorías, productos, precios, fotos o ilustraciones y «Agotado».

- [X] T017 [US1] Tests de integración primero en `server/internal/integration/public_menu_test.go`: JSON crudo con lista blanca de llaves (`TestThePublicMenuExposesOnlyThePublicFields`: sin costo, existencia, slug, `allow_oversell`, precio de mostrador cuando hay web), `categories` y `products` nunca `null`, precio web vs mostrador, oculto en web no sale, agotado sale marcado, categoría sin productos no sale, empresa inactiva = 404 igual a inexistente, y `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa)
- [X] T018 [US1] `PublicMenuService.Menu(ctx, name)` en `server/internal/app/public_menu.go`: resolver sin tenant con la vista, `WithTenant`, `offer` + categorías + nombre del negocio + paleta; arreglos normalizados a `[]`
- [X] T019 [US1] Test primero en `server/internal/httpapi/ratelimit_test.go`: con `X-Menu-Edge-Key` válido la llave es `X-Menu-Client-IP`; sin secreto o con uno inválido, una IP falsa en ese encabezado se ignora y cuenta la IP real; comparación de tiempo constante
- [X] T020 [US1] `publicMenuKey` y `publicMenuMax = 120` con su propio contador (`publicMenuIPs`, separado del de login y del webhook: un abuso del menú no frena al POS) en `server/internal/httpapi/ratelimit.go`; handlers `PublicMenu` y `PublicMenuByToken` en `server/internal/httpapi/handlers_public_menu.go` (forma validada antes de la base, `Cache-Control: public, max-age=30`); rutas fuera del grupo de tenant en `server/internal/httpapi/router.go`
- [X] T021 [P] [US1] Tests primero en `web/menu/plantilla/*.test.ts`: la plantilla escapa todo texto del negocio (`<script>` en un nombre sale inerte), cero `<script>` en la salida, `og:` en el HTML inicial con foto chica del primer producto con foto o la imagen por omisión, «Agotado» sin cantidades, ilustración por familia (null → `dish`), precios «$45» y «$45.50», `width`/`height` en toda imagen, primeras 4 con prioridad y el resto diferidas, mosaico con sus enlaces (fila «Menú público» de Maquetas)
- [X] T022 [US1] Plantilla real en `web/menu/plantilla/` separada en `datos.ts` (tipos del contrato), `marca.ts` (paletas → tonos), `formato.ts`, `html.ts` (escape) y `plantillas/classic.ts` (A + mosaico, con los ajustes de la revisión: encabezado en 700, `--m100` oscuro, pie de 14 px, `overscroll-behavior` y `:has` en la hoja), y el sprite de familias en línea
- [X] T023 [P] [US1] Tests primero de la Function en `web/menu/borde/handler.test.ts` con `fetch` y `caches` falsos: menú 200 con CSP sin scripts y `img-src` de `MEDIA_ORIGIN`; 404 HTML; nombre con forma imposible no llama a la API; manda `X-Menu-Edge-Key` y `X-Menu-Client-IP`; ignora la query string en la llave de caché
- [X] T024 [US1] Function `web/menu/borde/{handler,worker}.ts` + `web/menu/public/{_routes.json,_headers,robots.txt,og-default.png}` y la portada `/` sin lista de negocios
- [X] T025 [US1] e2e `web/e2e/menu-publico.spec.ts` contra `make start` + `dev:menu`: abrir el menú a 360, 768 y 1280; abrir el mosaico y saltar a una categoría; abrir un producto; ningún control < 44 px; sin desplazamiento horizontal

## Phase 4: User Story 2 — La dueña comparte su menú por enlace y QR (P1)

**Independent test**: desde Ajustes › Venta en web, copiar el enlace y descargar el QR en PNG, SVG y tarjeta; el QR decodificado apunta a `/q/<token>`.

- [X] T026 [US2] Tests primero en `server/internal/integration/public_menu_test.go`: `GET /admin/public-menu` con `url` y `qrUrl` de `MENU_BASE_URL`, crea el menú si falta (empresa nacida en el traslape), `CreateCompany` deja menú y nombre derivado del nombre del negocio a una empresa nueva, 403 sin `business.configure`, `inTheThreeCases` del servicio de Ajustes; `TestEveryToolMatchesItsRouteGate`/permisos de ruta sin regresión
- [X] T027 [US2] `PublicMenuService.Settings` + handler `GetPublicMenuSettings` y ruta con `RequirePermission(business.configure)` en `server/internal/app/public_menu.go`, `server/internal/httpapi/handlers_public_menu.go`, `server/internal/httpapi/router.go`
- [X] T028 [P] [US2] Tests primero en `web/src/features/admin/settings/menuPublico/*.test.tsx`: la tarjeta muestra la dirección guardada, Copiar cambia a «Copiado», el SVG del QR codifica `qrUrl`, PNG desde el SVG, la tarjeta para imprimir trae QR, nombre y dirección; Compartir cae a copiar sin `navigator.share` (fila «Ajustes › Venta en web» de Maquetas, opción B)
- [X] T029 [US2] Tarjeta «Comparte tu menú» (QR de 144 px, acciones y descargas en sus filas) en `web/src/features/admin/settings/menuPublico/CompartirMenu.tsx`, `qr.ts` (carga diferida de `uqr`), `TarjetaImprimible.tsx`; cliente en `web/src/api/admin.ts`; montada en `web/src/features/admin/settings/VentaEnWebSection.tsx` con el resumen en un renglón

## Phase 5: User Story 5 — El menú carga rápido (P1)

**Independent test**: Lighthouse móvil contra la Function local con el catálogo restaurado y fotos: LCP < 2500 ms, CLS < 0.1.

- [X] T030 [US5] Tests primero de caché en `web/menu/borde/handler.test.ts`: fresco < 60 s sin ir a la API; vencido va a la API; API con 5xx o caída sirve la copia de hasta 24 h; sin copia → 503 con `Retry-After`; el id del build está en la llave
- [X] T031 [US5] Caché del borde en `web/menu/borde/handler.ts` (Cache API, marca de frescura, `waitUntil`)
- [X] T032 [US5] Medir con Lighthouse (perfil móvil por omisión) contra `dev:menu` con el catálogo restaurado de El Gato Bobah y al menos 20 fotos subidas en local; anotar LCP, CLS y bytes en `specs/097-menu-publico/quickstart.md` (cifras, sin capturas)

## Phase 6: User Story 3 — Cambiar el nombre sin romper lo compartido (P2)

**Independent test**: `a → b → c → a`; `/b`, `/c` y `/q/<token>` llegan a `/a` en un salto.

- [X] T033 [US3] Tests primero en `server/internal/integration/public_menu_test.go`: PATCH de nombre (normaliza en el servidor), historial `a→b→c→a` sin ciclos, `kind: moved` del nombre viejo, nombre de otra empresa → 409, reservado → 409, tope de 20 → `MENU_NAME_LIMIT`, dos PATCH simultáneos de la misma empresa sin «no disponible» falso, dos empresas por el mismo nombre → una gana, PATCH sin `business.configure` → 403
- [X] T034 [US3] `PublicMenuService.Rename` con `LockPublicMenu` y `PlanMenuRename`, traducción de 23505 por restricción en `server/internal/store/` y handler `UpdatePublicMenuSettings` (`name`) en `server/internal/httpapi/handlers_public_menu.go`
- [X] T035 [P] [US3] Tests primero de la Function: nombre anterior y `/q/<token>` → 302 con `Cache-Control: public, max-age=300`; mayúsculas o diagonal final → 302 a la forma canónica; `<link rel=canonical>` al vigente
- [X] T036 [US3] Redirecciones en `web/menu/borde/handler.ts`
- [X] T037 [P] [US3] Tests primero del campo de nombre en `web/src/features/admin/settings/menuPublico/NombreDelMenu.test.tsx`: «Quedará» en vivo con la misma normalización, aviso en vivo con menos de 3, errores 409 con su texto para quien opera, la dirección mostrada cambia solo al guardar (fila «Ajustes › Venta en web» de Maquetas, opción B)
- [X] T038 [US3] `web/src/features/admin/settings/menuPublico/NombreDelMenu.tsx` y `slug.ts` (espejo de `SlugifyMenuName`, misma tabla de acentos)

## Phase 7: User Story 4 — Colores del negocio y cómodo en cualquier pantalla (P2)

**Independent test**: cambiar a verde y ver el menú con esos acentos a 360, 768 y 1280.

- [X] T039 [US4] Tests primero: PATCH `palette` (desconocida → 400) en `server/internal/integration/public_menu_test.go`; en `web/menu/plantilla/marca.test.ts`, las 8 paletas dan ≥ 4.5:1 en blanco sobre el fondo del encabezado y en el color de precio sobre blanco, y el test nombra la que falle
- [X] T040 [US4] `SetMenuPalette` en `server/internal/app/public_menu.go` y el campo `palette` del handler
- [X] T041 [P] [US4] Test primero de `web/src/features/admin/settings/menuPublico/ColorDelMenu.test.tsx`: `radiogroup` con nombre, foco visible, nombre de la paleta elegida a la vista, aviso «Color guardado» (fila «Ajustes › Venta en web» de Maquetas, opción B)
- [X] T042 [US4] `web/src/features/admin/settings/menuPublico/ColorDelMenu.tsx`

## Phase 8: Polish y cierre

- [X] T043 Medir la sección Venta en web a 1280×854 con el error del nombre visible: cabe sin desplazar; en 2K; ningún control < 44 px (Playwright, cifras en el reporte)
- [X] T044 [P] Jobs `deploy-menu` (main) y `deploy-menu-staging` (develop) copiados de `deploy-consola*`, y `typecheck:menu` + tests del menú en el job `frontend`, en `.github/workflows/ci.yml` (filtro de cambios incluido)
- [X] T045 [P] Runbook `docs/menu-publico.md` (proyectos de Pages, dominios, variables, regla de límite de peticiones, rotar `MENU_EDGE_KEY`, el QR atado a `menu.elgatobobah.com` cuando llegue el #331), renglón en `docs/README.md` y en la tabla de notas por área de `AGENTS.md`
- [ ] T046 `/revision-de-codigo` sobre el diff y corrección de lo que encuentre con su test
- [ ] T047 `/security-review` (obligatorio) y corrección con su test
- [ ] T048 Prueba con alguien nuevo: la dueña de una fonda que comparte su menú por WhatsApp (Ajustes a 1280×854 y en 2K) y un cliente que lo abre en el celular (360); corregir lo que encuentren
- [ ] T049 `make ci-local` verde; PR a develop al día (develop traída en local con commit firmado), CI verde y merge; release a main con respaldo previo, deploy, `readyz` 200 y journal sin errores propios
- [ ] T050 Cerrar: `make cerrar-worktree datos=1`, borrar la rama remota, comentar y cerrar el #316

## Dependencies

- Setup (T001–T006) → Foundational (T007–T016) → historias.
- US1 (T017–T025) es el MVP y lo necesitan US3 y US5 (la Function).
- US2 (T026–T029) depende solo de Foundational.
- US5 (T030–T032) depende de T024.
- US3 depende de T018 y T024; US4 de T022 y T029.
- Polish al final.

## Parallel

- T004 con T005–T006; T011 y T012 con T013; T021 y T023 juntos (archivos distintos); T028 con T026; T035 con T033; T037, T039, T041 entre sí; T044 y T045 juntos.

## Implementation Strategy

MVP = Foundational + US1 (el menú se ve en una dirección pública). Después US2 (compartir), US5
(caché y medición), US3 (renombrar) y US4 (color). Todo se sube junto en una release: la v1 no tiene
sentido sin enlace y QR.
