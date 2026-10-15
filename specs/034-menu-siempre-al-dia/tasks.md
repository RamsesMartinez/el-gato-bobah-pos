# Tasks: El menú de la tableta siempre al día

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/menu-api.md](contracts/menu-api.md)

TDD estricto (constitución IV): cada test se escribe y se ve fallar antes de su implementación.

## Phase 1: Setup

- [X] T001 Levantar un Postgres desechable para integración (puerto propio, nunca el de dev) y confirmar que `0089` (hoy `0093`, ver plan) es la siguiente migración libre en `server/migrations/`

## Phase 2: Foundational

(ninguna: cada historia trae lo suyo)

## Phase 3: User Story 1 — el catálogo cambiado por cualquier camino llega (P1)

**Independent test**: un `update` directo como owner sin empresa cambia la versión de las empresas tocadas y solo ésas; el menú servido cambia de llave.

- [X] T002 [US1] Tests de integración E1–E4, E15 y la regla «todas las tablas del menú suben la versión» en server/internal/integration/catalog_versions_test.go (verlos fallar)
- [X] T003 [US1] Migración server/migrations/0093_catalog_versions.sql (escrita como 0089; renumerada al retomar, ver plan): tabla, RLS, grant, función `security definer`, 30 triggers, Down
- [X] T004 [US1] Consulta `CatalogVersion :one` (versión + `current_date`) en server/queries/menu.sql y `make sqlc`
- [X] T005 [US1] `MenuService.Version(ctx)` en server/internal/app/menu.go (tipo `MenuVersion{Catalog, Day}`)
- [X] T006 [P] [US1] Test unitario de `MenuETag` (E6, E7: día y empresa cambian el ETag; schema incluido) en server/internal/cache/menu_test.go
- [X] T007 [US1] `cache.MenuETag` y llave versionada; quitar `Invalidate` en server/internal/cache/menu.go
- [X] T008 [US1] Quitar las invalidaciones del menú en server/internal/httpapi/{handlers_admin.go,handlers_modifiers.go,handlers_platform_prices.go,handlers.go,handlers_menu.go} conservando `menu.updated` y lo de popularidad/recomendador

## Phase 4: User Story 2 — la tableta se entera sola y barato (P2)

**Independent test**: `If-None-Match` con el ETag vigente → 304 sin cuerpo; la tableta revalida cada 60 s y al volver al frente, y una falla de red no quita el menú.

- [X] T009 [US2] Test de handler (E8, E9): 200 con ETag, 304 con `If-None-Match` igual, versión leída antes de armar, en server/internal/httpapi/handlers_menu_test.go
- [X] T010 [US2] `PosMenu` condicional en server/internal/httpapi/handlers_menu.go (interfaz mínima para probarlo sin base si hace falta)
- [X] T011 [P] [US2] Test vitest (E11): `posApi.menu` manda `If-None-Match` y en 304 devuelve el menú anterior, en web/src/api/menu.test.ts
- [X] T012 [US2] Soporte de petición condicional en web/src/api/client.ts y `posApi.menu` con ETag en web/src/api/pos.ts
- [X] T013 [P] [US2] Test vitest de `useMenu` (60 s, foco) y de que un refetch fallido no tumba la pantalla (E10) en web/src/hooks/useMenu.test.tsx
- [X] T014 [US2] `useMenu` con `refetchInterval: 60_000` y `refetchOnWindowFocus: 'always'` en web/src/hooks/useMenu.ts; `POSPage` solo muestra «Error cargando el menú» si no hay menú en web/src/features/pos/POSPage.tsx

## Phase 5: User Story 3 — red de seguridad ante un menú viejo (P3)

**Independent test**: un rechazo `MENU_CHANGED` al agregar muestra el aviso neutro, recarga el menú, reabre la hoja de extras y conserva los demás renglones.

- [X] T015 [US3] Test en server/internal/httpapi/respond_test.go: `ErrOptionNotFound`, `ErrProductNotSell`/`ProductUnavailable`, `ErrOptionOverMax` → 422 `MENU_CHANGED`; `ErrEmptyOrder` sigue `UNPROCESSABLE`
- [X] T016 [US3] Mapeo `MENU_CHANGED` en server/internal/httpapi/respond.go
- [X] T017 [P] [US3] Tests vitest (E12–E14) en web/src/features/pos/useCuenta.test.tsx: `onMenuCambiado` se llama con el producto, toast `info` con id fijo, sin toast rojo, la cuenta conserva sus renglones; al cambiar un renglón, mismo aviso
- [X] T018 [US3] `onMenuCambiado` en web/src/features/pos/useCuenta.ts (agregar y `alRechazo`)
- [X] T019 [US3] En web/src/features/pos/POSPage.tsx: al recibirlo, refetch del menú y reabrir `ModifierSheet` con el producto fresco si sigue a la venta y tiene extras (E13)

### Ampliación: mandar a cocina y «Enviar y cobrar» (FR-013)

- [X] T023 [US3] Test en server/internal/domain/order_test.go: `BuildOrder` dice en qué renglón rechazó (`LineRejected`) sin dejar de ser el sentinel
- [X] T024 [US3] `LineRejected` y `buildLine` en server/internal/domain/order.go
- [X] T025 [US3] Test de integración server/internal/integration/drafts_send_menu_changed_test.go: grupo cambiado y producto apagado → el rechazo de `Send` nombra el renglón de la cuenta
- [X] T026 [US3] `onDraftLine` en server/internal/app/drafts.go (y el rechazo propio de `orderLinesOf`)
- [X] T027 [US3] Test y `details.lineId` en server/internal/httpapi/respond.go
- [X] T028 [P] [US3] Tests vitest en web/src/features/pos/useEnviarCuenta.test.tsx y cobrarDesdeElTicket.test.tsx (mandar y cobrar): aviso neutro, sin rojo, renglón señalado, cobro sin abrir
- [X] T029 [US3] `onMenuCambiado` en useEnviarCuenta.ts, `reelegir` en Ticket.tsx y reapertura en POSPage.tsx; aviso compartido en menuCambiado.ts
- [X] T030 [US3] Revisión de tableta: el renglón por volver a elegir no se tacha ni entra en «Ya no están en el menú · Quitar» (su producto sigue a la venta); test en Ticket.test.tsx

## Phase 6: Polish

- [X] T020 Regla y nota del incidente en AGENTS.md (sin números del negocio): un cambio de catálogo por SQL ya no necesita borrar el menú guardado
- [X] T021 Correr `make ci-local`
- [X] T022 `/revision-de-codigo` (hook after_implement)

## Dependencies

US1 (T002–T008) → US2 (T009–T010 usan `Version` y `MenuETag`). Frontend de US2 (T011–T014) y US3 (T015–T019) son independientes entre sí. Polish al final.

## Parallel

T006 ∥ T002; T011 ∥ T013 ∥ T015 ∥ T017.

## Implementation Strategy

MVP = US1 (cierra la causa del incidente). US2 y US3 en el mismo PR, porque el dueño aprobó B + D + F juntas.
