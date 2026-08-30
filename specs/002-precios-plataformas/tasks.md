---
description: "Task list for feature implementation"
---

# Tasks: Venta por plataformas digitales con listas de precios propias

**Input**: Design documents from `specs/002-precios-plataformas/`
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [research.md](research.md)

**Tests**: Obligatorios. El principio IV de la constitución es no negociable: primero el test que
falla, luego el código. Cada tarea `[impl]` va **después** de su tarea de test.

**Organization**: Por historia de usuario, para que cada corte deje algo usable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: puede correr en paralelo (archivo distinto, sin dependencia pendiente)
- **[US1]**: a qué historia sirve

---

## Phase 1: Setup

Sin tareas. El stack, el linting y la estructura ya existen; esta feature no agrega dependencias
(principio VI: nada de librería nueva para lo que ya hace el repo).

---

## Phase 2: Foundational — esquema y datos

**⚠️ BLOQUEA TODO LO DEMÁS.** Sin los grants, la feature funciona en dev y se cae en producción.

- [ ] T001 Escribir la migración `server/migrations/0037_platform_prices.sql` con: `price_markup_pct numeric(5,2) not null default 0` + check 0..500 en `delivery_platforms`; las tablas `product_platform_prices` y `modifier_option_platform_prices` tal como las define [data-model.md](data-model.md) (FK sin cascade hacia `delivery_platforms`, `updated_by bigint not null`, checks `price > 0` y `price_delta >= 0`); índices `(platform_id, …)` y `(company_id)`; triggers `set_updated_at`; RLS con política `tenant_isolation` (`using` + `with check`); y **los dos `grant select, insert, update, delete … to gatobobah_app`**
- [ ] T002 En la misma migración, sembrar `price_markup_pct = 35.00` en Didi, Uber Eats y Rappi (0 en Propio), con el comentario de que corre como owner y por eso alcanza a todas las empresas
- [ ] T003 En la misma migración, desdoblar los métodos de pago: renombrar ids 4/5/6 a `Didi en línea`, `Uber Eats en línea`, `Rappi en línea` (conservando id, `affects_cash_drawer=false`, `auto_declare=true`) e insertar `Didi efectivo`, `Uber Eats efectivo`, `Rappi efectivo` con `kind='plataforma'`, **`affects_cash_drawer=true`** y **`auto_declare=false`**
- [ ] T004 Escribir el `-- +goose Down` que revierte T001–T003: `drop table` de las dos tablas, `drop column` del margen, y devolver los métodos de pago a su estado anterior (borrar los tres de efectivo, renombrar los tres de vuelta)
- [ ] T005 **[test]** Test de integración `server/internal/integration/platform_prices_test.go` que, **usando `appRoleStore` (rol `gatobobah_app`, con RLS real)**, hace `select`/`insert` sobre las dos tablas nuevas. Es el que atrapa un `grant` faltante — corre en CI y bloquea el deploy
- [ ] T006 **[test]** Test de integración que verifica el aislamiento: una empresa no ve los precios de plataforma de la otra, y no puede insertar una fila marcada con el `company_id` ajeno (rechazo por `with check`)
- [ ] T007 Aplicar la migración en local y correr `make sqlc`; ojo con `sqlc vet` (regla `db-prepare`), que valida contra la Postgres local: hay que migrarla antes o el vet falla contra el esquema viejo
- [ ] T008 **Pasar el subagente `db-architect` sobre la migración final** antes de aplicarla en cualquier lado que no sea local

**Checkpoint**: esquema listo y verificado bajo el rol de app.

---

## Phase 3: User Story 1 — capturar un pedido de plataforma con sus precios (P1) 🎯 MVP

**Goal**: cambiar la pantalla a Uber Eats, tocar productos y que cada uno entre ya con su precio de
esa lista, cobrar con el método de la plataforma e imprimir.

**Independent Test**: cambiar a una plataforma, agregar dos productos y un modificador, verificar que
los precios sean los de esa lista, cobrar y confirmar que el ticket impreso los trae.

### Dominio (lógica pura, sin I/O)

- [ ] T009 **[test]** [P] [US1] `server/internal/domain/platform_price_test.go` table-driven para `PlatformPrice(base, markupPct, manual)`: sin manual aplica margen; con manual lo devuelve tal cual; margen 0 devuelve el base; **caso obligatorio 434.98 @ 35% → 587.22** (no 587.223) y `398.98 @ 35% → 538.62`; delta 0 con margen sigue en 0
- [ ] T010 [US1] Implementar `PlatformPrice` en `server/internal/domain/platform_price.go`, con `Round2` aplicado al **unitario** y el comentario de por qué ahí y no en el total de línea
- [ ] T011 **[test]** [P] [US1] Test en `server/internal/domain/errors_test.go` (o el que corresponda) de que existe el sentinel `ErrPlatformNotFound` y que envuelve lo necesario para mapear a 422
- [ ] T012 [US1] Agregar `ErrPlatformNotFound` en `server/internal/domain/errors.go` y su mapeo a `422 PLATFORM_NOT_FOUND` en `server/internal/httpapi/respond.go`

### Datos y servicio

- [ ] T013 [US1] Escribir `server/queries/platform_prices.sql`: `GetPlatformByID` (bajo RLS, para resolver el margen), `GetProductPlatformPrices` y `GetOptionPlatformPrices` por lista de ids, y correr `make sqlc`
- [ ] T014 **[test]** [US1] Test de integración: `OrdersService.Create` con `deliveryPlatformId` de una plataforma **inexistente en la empresa** devuelve `ErrPlatformNotFound`, y **no** crea la orden ni cae a margen 0
- [ ] T015 **[test]** [US1] Test de integración: una venta con plataforma valúa cada línea con el precio de esa lista (calculado y manual), y `order_lines.unit_price` guarda ese precio, no el base
- [ ] T016 [US1] En `server/internal/app/orders.go`, resolver la plataforma bajo RLS antes de armar el pedido y construir el mapa de `PricedProduct`/`PricedOption` con el precio efectivo ya redondeado
- [ ] T017 **[test]** [US1] Test de integración: con plataforma, `deliveryFee` queda en 0 aunque el cliente mande otra cosa
- [ ] T018 [US1] Forzar `deliveryFee = 0` cuando la venta trae plataforma, en `server/internal/app/orders.go`
- [ ] T019 **[test]** [US1] Test de integración: cobrar un pedido de plataforma con un método que no es de esa plataforma devuelve 422; con **cualquiera de los dos** (en línea o efectivo) pasa
- [ ] T020 [US1] Validar el método de pago contra la plataforma del pedido en `server/internal/app/orders.go`
- [ ] T021 **[test]** [US1] Test de integración: vender el mismo producto con receta en mostrador y en las 3 plataformas descuenta **el mismo** inventario (FR-017)

### Menú

- [ ] T022 **[test]** [US1] Test de que `GET /pos/menu` incluye `platforms` (con `markupPct`), `platformPrices` y `platformModPrices`, y que **"Propio" NO aparece** en `platforms`
- [ ] T023 [US1] Extender el documento del menú en `server/internal/app/menu.go` y sus queries; **la llave del caché sigue siendo `pos:menu:<companyID>`**, sin la plataforma

### Frontend

- [ ] T024 **[test]** [P] [US1] `web/src/features/pos/precioPlataforma.test.ts` para la función pura que el front usa al pintar: espejo de la regla del servidor, con los mismos casos de redondeo
- [ ] T025 [US1] Implementar `web/src/features/pos/precioPlataforma.ts`
- [ ] T026 **[test]** [P] [US1] Test del store: la plataforma activa vive en la **cuenta** y una cuenta nueva arranca en mostrador (`web/src/stores/ticket.test.ts`)
- [ ] T027 [US1] Agregar la plataforma activa a `web/src/stores/ticket.ts`, con `null` = mostrador y cada cuenta nueva en `null`
- [ ] T028 [US1] `web/src/features/pos/PlatformPicker.tsx`: selector con las 3 plataformas + mostrador, e **indicador siempre visible** de con qué lista se está cobrando
- [ ] T029 [US1] Pintar los precios de la lista activa en el grid (`ProductGrid`/`ProductTile`) y en el detalle de modificadores (`ModifierSheet`)
- [ ] T030 [US1] En `CheckoutSheet.tsx`: mandar `deliveryPlatformId`, ofrecer solo los dos métodos de esa plataforma y ocultar el costo de envío

**Checkpoint**: US1 completa — ya se captura, cobra e imprime un pedido de plataforma.

---

## Phase 4: User Story 2 — corregir un precio y que persista (P1)

**Goal**: corregir el precio desde la pantalla de venta, sin salir, y que la próxima vez ya entre
corregido.

**Independent Test**: sobrescribir un precio, cerrar la venta, empezar otra en la misma plataforma y
ver el precio corregido.

- [ ] T031 [US2] Agregar a `server/queries/platform_prices.sql` el upsert y el delete de precio de producto, y correr `make sqlc`
- [ ] T032 **[test]** [US2] Test de integración: `PUT` de un precio lo persiste con `updated_by`; el mismo producto en otra plataforma y en mostrador **no cambia** (FR-007)
- [ ] T033 **[test]** [US2] Test de integración: `PUT` con precio ≤ 0 o fuera de `ValidMoney` → 422; con producto o plataforma de otra empresa → 404
- [ ] T034 [US2] Implementar `server/internal/app/platform_prices.go` (upsert + delete) con `ValidMoney` en la frontera
- [ ] T035 **[test]** [US2] Test de que escribir o borrar un precio **invalida** `pos:menu:<companyID>` (FR-020)
- [ ] T036 [US2] Invalidar el caché del menú en las dos rutas de escritura
- [ ] T037 **[test]** [US2] Test de integración del `DELETE`: quitar la excepción devuelve el producto al precio calculado, y borrar lo que no existe responde 204 (FR-019)
- [ ] T038 [US2] `server/internal/httpapi/handlers_platform_prices.go` + rutas en `router.go` (`PUT`/`DELETE /platform-prices/product`), accesibles a cualquier rol que pueda vender
- [ ] T039 **[test]** [P] [US2] Test del front de que el diálogo de captura solo aparece con una plataforma activa y no en mostrador
- [ ] T040 [US2] UI para capturar y quitar el precio desde la pantalla de venta, sin perder el ticket en curso
- [ ] T041 **Pasar el subagente `security-auditor`**: endpoint de escritura nuevo accesible a cajero (principio V)

**Checkpoint**: US2 completa — el precio se corrige una vez y queda.

---

## Phase 5: User Story 3 — el corte distingue cada plataforma (P2)

**Goal**: verificar, no construir. `ExpectedByMethodSince` ya agrupa por método activo.

**Independent Test**: cobrar por cada plataforma en un turno, cerrar caja y leer los renglones.

- [ ] T042 **[test]** [US3] Test de integración: ventas por las 3 plataformas en un turno → el corte muestra cada método en su renglón con su total (FR-015)
- [ ] T043 **[test]** [US3] Test de integración: lo cobrado **en línea** no cuenta al efectivo esperado del cajón y se autodeclara; lo cobrado **en efectivo** sí cuenta y **exige conteo** (FR-015a, FR-015b). Es el que atrapa el sobrante inexplicable del efectivo del repartidor
- [ ] T044 [US3] Si algún test de arriba falla, corregir; si pasan, **no se escribe código**: se deja constancia en el commit de que la historia ya estaba cubierta

**Checkpoint**: US3 verificada.

---

## Phase 6: User Story 4 — precio manual por opción de modificador (P3)

**Goal**: lo mismo que US2, para los extras.

**Independent Test**: sobrescribir el precio de una opción en una plataforma y ver que persiste ahí
y solo ahí.

- [ ] T045 [US4] Agregar el upsert y el delete de `modifier_option_platform_prices` a `server/queries/platform_prices.sql` y correr `make sqlc`
- [ ] T046 **[test]** [US4] Test de integración: el delta manual persiste por plataforma y **acepta 0** (un extra sin costo es normal), pero rechaza negativos
- [ ] T047 [US4] Implementar el servicio y los handlers `PUT`/`DELETE /platform-prices/modifier-option`
- [ ] T048 [US4] UI para corregir el precio de una opción desde `ModifierSheet`

**Checkpoint**: US4 completa.

---

## Phase 7: Polish

- [ ] T049 **[test]** [P] Test end-to-end del riesgo principal: armar un ticket en mostrador, cambiar a plataforma **con líneas ya agregadas**, y verificar que el total de pantalla coincide con el que devuelve el servidor al cobrar (FR-011 + FR-012)
- [ ] T050 [P] Verificar el ticket impreso de una venta de plataforma: trae los precios de la lista y no los base (FR-016)
- [ ] T051 [P] Correr [quickstart.md](quickstart.md) completo contra local, incluida la verificación 1 (grants con `gatobobah_app`)
- [ ] T052 [P] Revisar los textos de la pantalla contra el principio de UI de la constitución: nada que solo entienda quien leyó el código
- [ ] T053 Gates: `make api-build && make api-test`, `bun run test`, `bun run typecheck`, `bun run build`, y los hooks de lefthook en verde
- [ ] T054 Pasar el subagente `go-backend-reviewer` sobre el diff de backend

---

## Dependencies

- **Phase 2 bloquea todo.** Sin la migración y sus grants no hay dónde guardar ni leer un precio.
- **US1** depende solo de Phase 2. Es el MVP.
- **US2** depende de US1 (necesita la pantalla con plataforma activa para capturar desde ahí).
- **US3** depende de US1 (necesita ventas de plataforma que contar). Solo verifica.
- **US4** depende de US2 (repite su patrón para opciones).
- **Phase 7** al final.

## Parallel opportunities

- **T005 y T006** (los dos tests de integración del esquema) tras T007.
- **T009, T011, T024, T026**: tests puros en archivos distintos, sin base de datos.
- **T028, T029, T030**: tres componentes distintos del front, una vez que T027 dejó la plataforma en el store.
- **T049–T052**: cuatro verificaciones independientes.

## Implementation Strategy

**MVP = Phase 2 + US1** (T001–T030). Con eso ya se captura, cobra e imprime un pedido de plataforma
con precios automáticos, que es el problema que la feature vino a resolver. US2 lo hace sostenible,
US3 solo confirma lo que ya funciona y US4 es refinamiento.

**El orden dentro de Phase 2 no es negociable**: la migración y sus grants primero, y el test bajo el
rol de app antes de escribir cualquier servicio. Un grant faltante no se ve en dev —la API sirve como
owner— y tumba producción entera en el primer request.
