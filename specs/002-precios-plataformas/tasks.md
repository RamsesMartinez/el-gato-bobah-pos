---
description: "Task list for feature implementation"
---

# Tasks: Venta por plataformas digitales con listas de precios propias

**Input**: Design documents from `specs/002-precios-plataformas/`
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [research.md](research.md)

**Tests**: Obligatorios. El principio IV de la constitución es no negociable: primero el test que
falla, luego el código. Cada tarea de implementación va **después** de su tarea `**[test]**`.

**Entrega**: una sola, por decisión del dueño. El negocio empezó a operar hoy y tiene una venta
($159), así que el costo de un error es acotado y se corrige a mano contra el respaldo.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: puede correr en paralelo (archivo distinto, sin dependencia pendiente)
- **[US1]**: a qué historia sirve

---

## Phase 1: Setup

Sin tareas. El stack y la estructura ya existen; esta feature no agrega dependencias.

---

## Phase 2: Foundational — esquema, dinero y datos

**⚠️ BLOQUEA TODO LO DEMÁS**, y es la fase con dinero real apuntándole.

**Dos cosas de esta fase no se ven en local**: la base local tiene **una sola empresa**, así que todo
el camino de "copiar por cada otra empresa" es un no-op; y la API local sirve como **owner**, así que
RLS y grants no aplican. Lo que dependa de eso se prueba con test de integración, no a ojo.

### Respaldo, antes de nada

- [x] T001 Tomar respaldo de producción a `backups/prod/` siguiendo [backups/README.md](../../backups/README.md) y **verificar el sha256 en los dos lados**. Un respaldo no comparado no cuenta
- [x] T002 Restaurar ese respaldo en una base `gatobobah_ensayo` local y **crear ahí una segunda empresa de prueba**: sin ella la migración pasa verde y rompe en producción

### Migración 0037 — precios de plataforma

- [x] T003 `price_markup_pct numeric(5,2) not null default 0` con check 0..500 en `delivery_platforms`, y sembrar 35.00 en Didi/Uber Eats/Rappi (0 en Propio) **solo para las empresas que existen hoy**
- [x] T004 Crear `product_platform_prices` y `modifier_option_platform_prices` según [data-model.md](data-model.md): FK sin cascade hacia `delivery_platforms`, `updated_by bigint not null`, checks, índices `(platform_id, …)` y `(company_id)`, triggers `set_updated_at`, RLS con `tenant_isolation`, y **los dos `grant … to gatobobah_app`**

### Migración 0037 — `payment_methods` per-tenant (la parte con dinero)

- [x] T005 Antes de escribir el SQL, correr `select company_id, name from delivery_platforms order by 1,2` contra producción: el alta de los seis métodos depende de qué plataformas tiene cada empresa
- [x] T006 `add column company_id bigint` **nullable**, backfill a la empresa con `slug='gatobobah'` (el criterio que ya usa `0023`, no `min(created_at)`), con `raise exception` si sale NULL
- [x] T007 **Cambiar `unique(name)` por `unique(company_id, name)` AQUÍ**, antes de copiar nada. Al revés, la migración aborta con `23505` en la primera empresa extra — y no se ve en local, donde no hay una segunda
- [x] T008 Copiar los métodos por empresa con una columna temporal `src_id`, **sin listar `id`**: es `generated always as identity` y listarlo da `428C9` sin `overriding system value`
- [x] T009 Remapear `order_payments`, `expense_payments` y `register_session_totals` con join por `(company_id, src_id)`, **nunca por nombre**: `citext` ignora mayúsculas pero **no** acentos ni espacios, y el match de hoy funciona solo porque las copias son idénticas byte a byte
- [x] T010 Enumerar las tablas dependientes desde `pg_constraint` en vez de una lista fija de tres, y verificar **antes y después**: mismo `count(*)` por tabla, misma `sum(amount)`, mismo número de métodos por empresa, y cero filas apuntando al método de otra. Cualquier diferencia → `raise exception` y la transacción entera se va para atrás
- [x] T011 `set not null` + `set default current_setting('app.company_id', true)::bigint`, `create index payment_methods_company on payment_methods (company_id)` (el `grant` de `0024` **ya existe**; el índice no), RLS + política `tenant_isolation`, y `drop column src_id`
- [x] T012 `add column delivery_platform_id smallint` nullable sin `on delete`, más `unique (id, company_id)` en `delivery_platforms` y **FK compuesta** `(delivery_platform_id, company_id)`. Aquí sí vale la pena, a diferencia del riesgo residual del resto: esta columna **agrupa dinero real en el corte** y una fila cruzada rompe el subtotal sin dar error
- [x] T013 Renombrar los tres métodos a "en línea" y dar de alta los tres "efectivo" (`affects_cash_drawer=true`, `auto_declare=false`), **después del remapeo y para todas las empresas**, con `sort_key` 400/450, 500/550, 600/650 para que cada par no quede en orden indeterminado. El `delivery_platform_id` sale de `delivery_platforms` **de esa misma empresa**
- [x] T014 Escribir el `-- +goose Down` completo (remapear de vuelta por `src_id`, borrar copias y los tres nuevos, renombrar, revertir el unique, quitar columnas y RLS) **con el comentario de que solo es válido hasta el primer cobro con un método nuevo**: después, las FK `no action` lo hacen fallar y revertir pasa a ser un data-fix a mano con rollback gemelo

### El dinero que el desdoble rompe si no se toca el código

- [x] T015 **[test]** Test de integración del faltante fantasma: caja con fondo de $1,500, sin ventas, con los seis métodos activos → el corte debe dar diferencia **0**, no −$1,500 por cada método de cajón
- [x] T016 Corregir [server/internal/app/backoffice.go](../../server/internal/app/backoffice.go): el fondo de apertura y el neto de movimientos se suman **una sola vez**, al método de `kind='efectivo'`, y no a cada `affects_cash_drawer`. Sin esto el desdoble reporta $4,500 de faltante inexistente y cuenta las entradas de efectivo cuatro veces
- [x] T017 **[test]** Test de integración: en una caja secundaria solo sobrevive el método de efectivo al filtro de `backoffice.go`, no los cuatro de cajón
- [x] T018 **[test]** Test de integración con **dos empresas**: `Create` con un `payment_method_id` de otra empresa devuelve 422 y no crea el pago
- [x] T019 Validar cada `MethodID` con `GetPaymentMethod` bajo RLS en `OrdersService.Create`, antes de la tx. Hoy no se valida: la FK salta RLS y un id cruzado hace que el pago **desaparezca** del corte y de los reportes, con el cajero viendo un faltante por el monto exacto

### Para que una empresa nueva pueda cobrar

- [x] T020 **[test]** Test de integración: una empresa recién aprovisionada tiene sus cuatro métodos base y puede cobrar
- [x] T021 Sembrar Efectivo, Tarjeta débito, Tarjeta crédito y Transferencia SPEI en `provisionCompany` ([server/cmd/api/main.go](../../server/cmd/api/main.go)), dentro del `WithTenant` que ya abre. Los de plataforma **no**: esos se dan de alta cuando ese negocio hace su propia vinculación

### El POS deja de asumir ids fijos

- [x] T022 **[test]** [P] Test de que la pantalla de cobro arma sus métodos desde la API y decide el default y la rama de efectivo por `kind`/`affectsCashDrawer`, no por id
- [x] T023 Quitar `METHODS` con ids quemados de [web/src/features/pos/CheckoutSheet.tsx](../../web/src/features/pos/CheckoutSheet.tsx): salen de `posApi.paymentMethods()`. Reemplazar `useState(2)` y los dos `methodId === 1` por decisiones sobre `kind`. **Va en el mismo release que la migración**: la empresa 1 conserva sus ids y no lo nota, pero cualquier otra recibe ids nuevos y se queda sin poder cobrar
- [x] T024 Corregir el harness: `paymentMethodID` resuelve por `name` **y `company_id`** ([server/internal/integration/cash_test.go](../../server/internal/integration/cash_test.go)). Con dos empresas habrá dos filas "Efectivo" y hoy `QueryRow` toma la que devuelva el plan, sin error

### Verificación del esquema

- [x] T025 **[test]** Test de integración que, **bajo `appRoleStore`** (rol `gatobobah_app`, RLS real), hace `select`/`insert` sobre las tablas nuevas. Es el que atrapa un `grant` faltante y corre en CI
- [x] T026 **[test]** Test de integración de aislamiento: una empresa no ve los precios de plataforma ni los métodos de pago de la otra, y no puede insertar marcando el `company_id` ajeno
- [x] T027 Aplicar la migración sobre el ensayo (con sus dos empresas), correr el Down y el Up otra vez. Luego `make sqlc`; ojo con `sqlc vet`, que valida contra la Postgres local y falla si no está migrada
- [x] T028 **Pasar el subagente `db-architect` sobre la migración final** antes de aplicarla en producción

**Checkpoint**: esquema y dinero listos, verificados con dos empresas y bajo el rol de app.

---

## Phase 3: User Story 1 — capturar un pedido de plataforma con sus precios (P1) 🎯 MVP

**Goal**: cambiar la pantalla a Uber Eats, tocar productos y que cada uno entre ya con su precio de
esa lista, cobrar con el método de la plataforma e imprimir.

**Independent Test**: cambiar a una plataforma, agregar dos productos y un modificador, verificar que
los precios sean los de esa lista, cobrar y confirmar que el ticket impreso los trae.

### Dominio (lógica pura, sin I/O)

- [x] T029 **[test]** [P] [US1] `server/internal/domain/platform_price_test.go` table-driven para `PlatformPrice(base, markupPct, manual)`: sin manual aplica margen; con manual lo devuelve tal cual; margen 0 devuelve el base; **caso obligatorio 434.98 @ 35% → 587.22** (no 587.223) y `398.98 @ 35% → 538.62`; delta 0 con margen sigue en 0
- [x] T030 [US1] Implementar `PlatformPrice` en `server/internal/domain/platform_price.go`, con `Round2` sobre el **unitario** y el comentario de por qué ahí y no en el total de línea
- [x] T031 **[test]** [P] [US1] Test del sentinel `ErrPlatformNotFound` y de que se mapea a 422
- [x] T032 [US1] Agregar `ErrPlatformNotFound` en `server/internal/domain/errors.go` y su mapeo a `422 PLATFORM_NOT_FOUND` en `server/internal/httpapi/respond.go`

### Datos y servicio

- [x] T033 [US1] (queries escritas y generadas; falta cablearlas) Escribir `server/queries/platform_prices.sql`: `GetPlatformByID` (bajo RLS), `GetProductPlatformPrices` y `GetOptionPlatformPrices` por lista de ids, y correr `make sqlc`
- [x] T034 **[test]** [US1] Test de integración: `Create` con `deliveryPlatformId` **inexistente en la empresa** devuelve `ErrPlatformNotFound`, no crea la orden y **no** cae a margen 0
- [x] T035 **[test]** [US1] Test de integración: una venta con plataforma valúa cada línea con el precio de esa lista (calculado y manual), y `order_lines.unit_price` guarda ese precio, no el base
- [x] T036 [US1] En `server/internal/app/orders.go`, resolver la plataforma bajo RLS antes de armar el pedido y construir el mapa de `PricedProduct`/`PricedOption` con el precio efectivo ya redondeado
- [x] T037 **[test]** [US1] Test de integración: con plataforma, `deliveryFee` queda en 0 aunque el cliente mande otra cosa
- [x] T038 [US1] Forzar `deliveryFee = 0` cuando la venta trae plataforma
- [x] T039 **[test]** [US1] Test de integración: cobrar un pedido de plataforma con un método que no es de esa plataforma devuelve 422; con **cualquiera de los dos** (en línea o efectivo) pasa
- [x] T040 [US1] Validar el método de pago contra `delivery_platform_id` del método, no contra su nombre
- [x] T041 **[test]** [US1] Test de integración: vender el mismo producto con receta en mostrador y en las 3 plataformas descuenta **el mismo** inventario (FR-017)
- [x] T042 **[test]** [US1] Test de integración: la venta queda con su `delivery_platform_id` (FR-013)
- [x] T043 **[test]** [US1] Test de que un producto **sin** precio manual se agrega y se cobra sin bloquear nada (FR-004)

### Menú

- [x] T044 **[test]** [US1] Test de que `GET /pos/menu` incluye `platforms` (con `markupPct`), `platformPrices` y `platformModPrices`, y que **"Propio" NO aparece**
- [x] T045 [US1] Extender el documento del menú en `server/internal/app/menu.go` y sus queries; **la llave del caché sigue siendo `pos:menu:<companyID>`**, sin la plataforma

### Frontend

- [x] T046 **[test]** [P] [US1] `web/src/features/pos/precioPlataforma.test.ts`: espejo de la regla del servidor, con los mismos casos de redondeo
- [x] T047 [US1] Implementar `web/src/features/pos/precioPlataforma.ts`
- [x] T048 **[test]** [P] [US1] Test del store: la plataforma activa vive en la **cuenta** y una cuenta nueva arranca en mostrador
- [x] T049 [US1] Agregar la plataforma activa a `web/src/stores/ticket.ts`, con `null` = mostrador
- [x] T050 **[test]** [P] [US1] Test del selector: muestra las 3 plataformas y mostrador, e indica siempre con cuál se está cobrando
- [x] T051 [US1] `web/src/features/pos/PlatformPicker.tsx`
- [x] T052 **[test]** [P] [US1] Test de que el grid y el detalle de modificadores pintan los precios de la lista activa
- [x] T053 [US1] Pintar los precios de la lista activa en `ProductGrid`/`ProductTile` y `ModifierSheet`
- [x] T054 **[test]** [P] [US1] Test de `CheckoutSheet`: con plataforma ofrece **solo** sus dos métodos y **no** muestra costo de envío
- [x] T055 [US1] En `CheckoutSheet.tsx`: mandar `deliveryPlatformId`, filtrar métodos por plataforma y ocultar el envío
- [x] T056 **[test]** [US1] **El riesgo #1**: armar un ticket en mostrador, cambiar a plataforma **con líneas ya agregadas**, y verificar que el total de pantalla coincide con el que devuelve el servidor al cobrar (FR-011 + FR-012)
- [x] T057 [US1] Re-preciar las líneas ya agregadas al cambiar de lista
- [x] T058 **[test]** [US1] Test de que el ticket impreso de una venta de plataforma trae los precios de la lista (FR-016)

**Checkpoint**: US1 completa — se captura, cobra e imprime un pedido de plataforma.

---

## Phase 4: User Story 2 — corregir un precio y que persista (P1)

**Goal**: corregir el precio desde la pantalla de venta y que la próxima vez ya entre corregido.

**Independent Test**: sobrescribir un precio, cerrar la venta, empezar otra en la misma plataforma y
ver el precio corregido.

- [x] T059 [US2] Agregar el upsert y el delete de precio de producto a `server/queries/platform_prices.sql`, y correr `make sqlc`
- [x] T060 **[test]** [US2] Test de integración: el `PUT` persiste con `updated_by`; el mismo producto en otra plataforma y en mostrador **no cambia** (FR-007)
- [x] T061 **[test]** [US2] Test de integración: precio ≤ 0 o fuera de `ValidMoney` → 422; producto o plataforma de otra empresa → 404
- [x] T062 [US2] Implementar `server/internal/app/platform_prices.go` (upsert + delete) con `ValidMoney` en la frontera
- [x] T063 **[test]** [US2] Test de que escribir o borrar un precio **invalida** `pos:menu:<companyID>` (FR-020)
- [x] T064 [US2] Invalidar el caché del menú en las dos rutas de escritura
- [x] T065 **[test]** [US2] Test del `DELETE`: quitar la excepción devuelve el producto al precio calculado, y borrar lo inexistente responde 204 (FR-019)
- [x] T066 [US2] `server/internal/httpapi/handlers_platform_prices.go` + rutas en `router.go`
- [x] T067 **[test]** [P] [US2] Test de que el diálogo de captura solo aparece con una plataforma activa
- [x] T068 [US2] UI para capturar y quitar el precio sin perder el ticket en curso
- [x] T069 **Pasar el subagente `security-auditor`**: endpoint de escritura nuevo accesible a cajero (principio V)

**Checkpoint**: US2 completa.

---

## Phase 5: User Story 3 — el corte distingue cada plataforma (P2)

**Goal**: cada método en su renglón **y** el subtotal de la plataforma.

**Independent Test**: cobrar por cada plataforma y en las dos formas, cerrar caja y leer.

- [x] T070 **[test]** [US3] Test de integración: ventas por las 3 plataformas → el corte muestra cada método en su renglón con su total (FR-015)
- [x] T071 **[test]** [US3] Test de integración: lo cobrado **en línea** no cuenta al efectivo esperado y se autodeclara; lo cobrado **en efectivo** sí cuenta y **exige conteo** (FR-015a, FR-015b). Es el que atrapa el sobrante inexplicable del efectivo del repartidor
- [x] T072 **[test]** [US3] Test del subtotal por plataforma: agrupa por `delivery_platform_id`, **no** por nombre
- [x] T073 [US3] Agrupar por plataforma en el cierre (backend) y mostrarlo en la pantalla de corte — PENDIENTE: los renglones ya salen separados y correctos; falta el subtotal por plataforma

**Checkpoint**: US3 completa.

---

## Phase 6: User Story 4 — precio manual por opción de modificador (P3)

- [x] T074 [US4] Agregar el upsert y el delete de `modifier_option_platform_prices` y correr `make sqlc`
- [x] T075 **[test]** [US4] Test de integración: el delta manual persiste por plataforma, **acepta 0** y rechaza negativos
- [x] T076 [US4] Servicio y handlers `PUT`/`DELETE /platform-prices/modifier-option`
- [x] T077 **[test]** [P] [US4] Test de la UI de corrección de una opción
- [x] T078 [US4] UI para corregir el precio de una opción desde `ModifierSheet`

**Checkpoint**: US4 completa.

---

## Phase 7: Polish

- [ ] T079 [P] Correr [quickstart.md](quickstart.md) completo, incluida la verificación de grants con `gatobobah_app`
- [x] T080 [P] Revisar los textos de la pantalla contra el principio de UI de la constitución: nada que solo entienda quien leyó el código
- [x] T081 Gates: `make api-build && make api-test`, `bun run test`, `bun run typecheck`, `bun run build`, y los hooks de lefthook en verde
- [x] T082 Pasar el subagente `go-backend-reviewer` sobre el diff de backend
- [ ] T083 **Después de desplegar**: verificar en producción que los 55 pagos históricos y el pedido #1 ($159) siguen apuntando al método de su empresa, y que el corte del día cuadra

---

## Dependencies

- **Phase 2 bloquea todo.** Y dentro de ella el orden es estricto: respaldo → segunda empresa de ensayo → unique per-company **antes** de copiar → remapeo por `src_id` → verificación de sumas → el resto.
- **T016 (fondo de caja) y T023 (ids del POS) van en el mismo release que la migración.** Sin T016 el corte reporta faltantes que no existen; sin T023, cualquier empresa que no sea la primera no puede cobrar.
- **US1** depende solo de Phase 2. Es el MVP.
- **US2** depende de US1; **US3** depende de US1; **US4** depende de US2.

## Parallel opportunities

- **T025 y T026** tras T027.
- **T029, T031, T046, T048**: tests puros, sin base de datos.
- **T050/T052/T054** y sus implementaciones, una vez que T049 dejó la plataforma en el store.
- **T079 y T080**.

## Implementation Strategy

Entrega única, pero el orden interno no es negociable. **La fase 2 se prueba contra un ensayo con
dos empresas**: con una sola, la mitad de sus riesgos son invisibles y aparecen en producción.

**MVP funcional = Phase 2 + US1** (T001–T058). US2 lo hace sostenible, US3 cierra el corte y US4 es
refinamiento.
