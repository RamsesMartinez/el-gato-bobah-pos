# Tasks: Emparejar la tienda de plataforma conectada

**Input**: [plan.md](plan.md), [spec.md](spec.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV). Cada test se ve fallar antes del código. Integración
bajo `appRoleStore` con `inTheThreeCases` para lo que lee tablas de empresa; migración sobre
`restoredStore` (dos empresas).

## Phase 1: Setup

- [ ] T001 Medir sobre el respaldo restaurado (`gatobobah_restaurado`) y sobre el de producción más reciente: parejas con `kind = 'opcion'` en `platform_item_links`, filas de `product_platform_prices` / `modifier_option_platform_prices`, y si algún script de `docs/reorg/` escribe en esas tablas. Anotar el resultado en research.md; si hay parejas de opción, parar y preguntar al dueño
- [ ] T002 Confirmar que 0077 es el siguiente número libre en esta rama y en las ramas vivas

## Phase 2: Foundational

**Tests primero**

- [ ] T003 [P] Tests de dominio en server/internal/domain/menu_de_plataforma_test.go: `RankCandidates` (orden por tokens en común, empates estables, acentos y emoji), `GroupOf` (Listos / Por revisar / Sin pareja / excluido, y que los conteos salen de la misma función), `PriceSync` (cambios reales solamente; precio cero no se sincroniza; varias parejas al mismo producto usan la de captura; más de una tienda de la misma plataforma → error), `CapturePriceAfterUnlink` (pasa a la más reciente)
- [ ] T004 Test de migración en server/internal/integration/platform_pairing_migration_test.go sobre `restoredStore`: aborta con parejas de tipo opción; relleno de `is_capture_price`; `source = 'manual'` en los precios existentes; producto genérico creado una vez por empresa; Down limpio
- [ ] T005 Test de aislamiento de las tres tablas nuevas y de las consultas nuevas en los tres casos, en server/internal/integration/platform_pairing_test.go

**Implementación**

- [ ] T006 server/migrations/0077_platform_pairing.sql: columnas de `platform_item_links` (opción, captura, checks, FK compuesta, únicos parciales), tablas `platform_item_exclusions`, `local_item_exclusions` (restrict hacia productos y opciones), `platform_price_changes` (cascade con la lectura), `source`/`synced_at` en las dos tablas de precios, `products.system_kind`, categoría «Plataformas» y producto genérico por empresa; RLS con `nullif` y grants; guarda que aborta con parejas de opción; Down
- [ ] T007 `CreateCompany` crea también la categoría y el producto genérico (server/queries/companies.sql), `make sqlc`; `docs/corte-produccion/01_nueva_empresa.sql` igual
- [ ] T008 [P] Funciones de dominio de T003 en server/internal/domain/menu_de_plataforma.go y sentinels `ErrPlatformPriceManaged`, `ErrCapturePriceRequired`, `ErrLinkKindMismatch`, `ErrSeveralStoresSamePlatform` en server/internal/domain/errors.go; mapeo en server/internal/httpapi/respond.go con sus códigos

**Checkpoint**: `go build ./... && go test ./...` en verde.

## Phase 3: US1 — Ver el universo (P1)

- [ ] T009 [P] [US1] Test de integración: `GET /pairing` trae platillos y opciones con grupo, conteos que cuadran con la lista, precios con 2 decimales, arreglos nunca nulos (JSON crudo), en server/internal/integration/platform_pairing_test.go
- [ ] T010 [US1] Consultas de lectura (pareja con destino producto u opción, exclusiones, últimos cambios de precio) en server/queries/menus_plataforma.sql; servicio `Pairing` en server/internal/app/menus_de_plataforma.go; handler y ruta en server/internal/httpapi/handlers_menus_plataforma.go y router.go
- [ ] T011 [P] [US1] Tests de la pantalla: grupos con conteo, buscador, scroll propio de la lista (contenedor con alto), toggles de 44 px, sin `<select>`, aviso fijo de precios, `moneyExact`, en web/src/features/admin/EmparejarPage.test.tsx
- [ ] T012 [US1] `moneyExact` en web/src/utils/format.ts; cliente en web/src/api/plataformas.ts; `EmparejarPage` con `PairingList` (diseño B1) en web/src/features/admin/

## Phase 4: US2 — Revisar propuestas (P1)

- [ ] T013 [P] [US2] Test de integración: lote idempotente, confirma solo propuestas vigentes, quién y cuándo, dos sesiones a la vez, en platform_pairing_test.go
- [ ] T014 [US2] `POST /links/batch` (servicio en una transacción, handler, ruta)
- [ ] T015 [P] [US2] Tests de pantalla: modo uno por uno avanza solo y deshace; modo lote confirma N con un toque y desmarcar deja en Sin pareja
- [ ] T016 [US2] `ReviewPanel` (B2b) y `BatchPanel` (B2) con su selector de modo

## Phase 5: US3 — Emparejar sin limitaciones (P1)

- [ ] T017 [P] [US3] Test de integración: varios platillos al mismo producto con elección de captura obligatoria (`CAPTURE_PRICE_REQUIRED`); opción → opción del POS; opción → producto rechazado (`LINK_KIND_MISMATCH`); opción de otra empresa rechazada por la FK (como owner); candidatos ordenados con `linkedCount`
- [ ] T018 [US3] `GuardarPareja` escribe `product_id` o `modifier_option_id` según el tipo, con `capturePrice` y `replace`; `GET /candidates`; quitar `unlinkedLocal` como filtro del buscador
- [ ] T019 [P] [US3] Tests de pantalla: buscador muestra productos ya ligados con su conteo; al ligar el segundo aparece la elección de captura (B5) y no deja terminar sin ella
- [ ] T020 [US3] Panel de candidatos y paso de precio de captura (B5)

## Phase 6: US4 — Corregir y decidir (P2)

- [ ] T021 [P] [US4] Test de integración: cambiar y quitar pareja (la captura pasa a la más reciente); exclusiones sobreviven a otra lectura; emparejar borra la exclusión en la misma transacción; borrar la tienda cuenta parejas y decisiones
- [ ] T022 [US4] Rutas de exclusiones, `ParejasQueSePierden` con las tres cuentas, `CorrectPanel` (B3) y filtro «Solo en Uber»

## Phase 7: US5 — El precio lo pone Uber (P2)

- [ ] T023 [P] [US5] Test de integración: tras una lectura buena se sincronizan los precios de lo emparejado con `source = platform` y quedan los cambios; lectura fallida no toca nada; precio cero no se escribe; dos tiendas de la misma plataforma rechazan la sincronización; `PUT /platform-prices/product` y de opción responden 409 `PLATFORM_PRICE_MANAGED` sobre una fila sincronizada
- [ ] T024 [US5] Sincronización dentro del `WithTenant` de `correrLectura` en server/internal/app/menus_de_plataforma.go; rechazo en server/internal/app/platform_prices.go; consultas en server/queries/platform_prices.sql
- [ ] T025 [P] [US5] Tests de pantalla del POS: `PlatformPriceDialog` y `OptionPriceFields` muestran «Lo pone Uber · se actualizó …» y no dejan editar
- [ ] T026 [US5] web/src/features/pos/PlatformPriceDialog.tsx y ModifierSheet.tsx; aviso de precios cambiados en la pantalla de emparejar

## Phase 8: US6 — Platillo sin pareja (P1)

- [ ] T027 [P] [US6] Test de integración: aceptar un pedido con un renglón sin pareja lo liga al producto genérico con nombre de Uber y opciones en la nota; uno con pareja de opción no se trata como producto (`pedidos_de_plataforma.go:419`); el genérico no descuenta almacén
- [ ] T028 [US6] `copiarRenglones` y el armado de parejas en server/internal/app/pedidos_de_plataforma.go
- [ ] T029 [P] [US6] Test del ticket: un renglón del producto genérico (inactivo) se imprime en la comanda, en web/src/features/pos/Ticket.test.tsx (o el que exista)
- [ ] T030 [US6] Ajustar el ticket o la vista de vender si filtran por activo

## Phase 9: Polish

- [ ] T031 [P] AGENTS.md: el emparejamiento guarda producto u opción; el precio de una plataforma conectada lo escribe la lectura y la captura a mano lo rechaza; el producto genérico se encuentra por `system_kind`
- [ ] T032 [P] docs/emparejamiento-de-plataformas.md: estado de lo construido
- [ ] T033 Gates: `go build`, `go test`, integración, `make lint`, `bun run lint`, vitest, build
- [ ] T034 Ensayo en `pos-vps-dev` con el respaldo de producción según quickstart.md; cobrar lo creado; regresar el ambiente y apagarlo
- [ ] T035 `/revision-de-codigo` sobre el diff

## Dependencies

Phase 2 bloquea todo. US1 antes que US2, US3 y US4 (comparten la pantalla). US5 y US6 dependen solo
de Phase 2. T034 y T035 al final.

## Implementation Strategy

MVP = Phase 2 + US1 + US3 + US6: ver todo, poder emparejar bien (opciones y varios a uno) y que
ningún pedido de Uber quede sin producto. Después US2 (velocidad), US5 (precio) y US4 (cerrar).
