---
description: "Task list for feature implementation"
---

# Tasks: el folio con el que llegó el pedido, y lo que la plataforma se quedó

**Input**: Design documents from `specs/014-folio-y-comision-de-plataforma/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

**Tests**: NO son opcionales en este repo. El principio IV de la constitución es **NO NEGOCIABLE**
("primero el test que falla, luego el código"; "un task de implementación sin su task de test antes
está mal ordenado"), y el pre-commit [migracion-con-test.sh](../../scripts/hooks/migracion-con-test.sh)
rechaza una migración que llega sin su test de integración. Por eso cada bloque de implementación va
después de su bloque de tests, y **cada test se ve fallar por la razón correcta antes de escribir el
código**.

**Organization**: por historia de usuario, en el orden de prioridad del spec. Cada una entrega valor
sola.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: se puede correr en paralelo (archivos distintos, sin dependencias pendientes)
- **[Story]**: US1 / US2 / US3
- Rutas exactas en cada tarea

## Convenciones de este repo

- **Backend**: `cd server && go build ./... && go test ./...` antes de dar algo por bueno.
- **Integración**: `go test -tags=integration ./internal/integration/...` con `TEST_DATABASE_URL`.
- **Frontend**: siempre `bun`, nunca npm.
- **sqlc**: se editan `server/queries/*.sql` y se corre `make sqlc`; **nunca** se edita el `.go` generado.
- **sqlc no conoce `company_id`** en `orders`: nombrarla en una consulta rompe `sqlc generate`. RLS la aplica sola.
- **En la interfaz nunca se dice "folio" a secas** para el identificador de la plataforma: en esta
  pantalla esa palabra ya es el número del turno. Va *Folio de Uber Eats* / *Folio de la plataforma* (D-18).

---

## Phase 1: Setup

**Purpose**: el esqueleto, la base con datos reales y el rastro documental, antes de tocar nada.

- [X] T001 Generar el esqueleto de la migración con `make migrate-new name=folio_y_liquidacion_de_plataforma`, que crea `server/migrations/0065_folio_y_liquidacion_de_plataforma.sql`
- [X] T002 Escribir `scripts/respaldo-anonimo.sh` y su target en el `Makefile`: baja un dump de la base de producción apoyándose en el acceso que ya existe (`make prod-db-tunnel`, `gcloud compute ssh` a `pos-vps`), **borra los datos personales** (`orders.customer_name`, `orders.notes` y cualquier otro texto libre capturado por el operador) y lo restaura en `TEST_DATABASE_URL`. Importes, fechas, estados, sesiones de caja y plataformas se conservan intactos: es justo lo que los tests vienen a probar (D-17)
- [X] T003 [P] Agregar los renglones de esta feature a `docs/matriz-de-pantallas.md` (filtro rechazado, lista y resumen del mismo predicado, campo ausente en mostrador, búsqueda por folio, alto medido) y a `docs/matriz-de-cobro.md` (registrar liquidación no mueve el cobro), marcados **sin test** — se cierran conforme cada test exista

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: el esquema y las reglas puras de las que cuelgan las tres historias.

**⚠️ CRÍTICO**: ninguna historia puede empezar hasta que esta fase esté completa.

### Tests primero

- [X] T004 Escribir el test de integración de la migración en `server/internal/integration/migracion_folio_y_liquidacion_test.go`, **contra la base restaurada de T002** y con **al menos dos empresas**, bajo `appRoleStore`: las tres columnas de `orders`, los tres checks, el índice único por `(company_id, delivery_platform_id, platform_order_ref)`, el índice parcial de pendientes, el índice de búsqueda, la FK compuesta de `platform_settlements`, la policy de RLS, el `grant` a `gatobobah_app` y que el `Down` deja el esquema como estaba. **Verlo fallar** — las columnas todavía no existen
- [X] T005 [P] Escribir `server/internal/domain/platform_ref_test.go` table-driven contra los bordes: recorte de extremos, cadena vacía, solo espacios, 64 y 65 caracteres, mayúsculas y guiones que **no** se transforman
- [X] T006 [P] Agregar a `server/internal/domain/limits_test.go` los casos de `ValidSignedMoney`: negativo aceptado, `±MaxMoney`, fuera de cota, y exponente absurdo (`1e100000000`) rechazado sin quemar CPU
- [X] T007 [P] Escribir `server/internal/domain/settlement_test.go`: tasa fuera de 0–100, comisión negativa, `discountPlatform > discountTotal`, neto negativo **aceptado**, comisión mayor que la venta **aceptada**, y `DiscountRestaurant()` como derivado

### Implementación

- [X] T008 Escribir la migración completa en `server/migrations/0065_folio_y_liquidacion_de_plataforma.sql` según [data-model.md](data-model.md): `lock_timeout`, las 3 columnas, los 3 checks, `orders_id_company_key`, el índice único parcial, el índice de pendientes, el índice de búsqueda, la tabla `platform_settlements` con FK compuesta, trigger, RLS, policy y **`grant`**, más el `Down` completo. **Va en el mismo commit que T004** — el pre-commit rechaza una migración sin su test
- [X] T009 Comentar en la migración que **cambiar la plataforma de un pedido que ya tiene folio dejaría basura silenciosa**, y que hoy es imposible porque ningún endpoint muta `delivery_platform_id`. Quien agregue ese camino decide ahí qué pasa con el folio
- [X] T010 Aplicar la migración sobre la base restaurada y dejar T004 en verde; correr `goose down` y volver a subir para probar la reversibilidad de verdad
- [X] T011 [P] Implementar `NormalizePlatformRef` y `MaxPlatformRefLen` en `server/internal/domain/platform_ref.go` (T005 en verde)
- [X] T012 [P] Implementar `ValidSignedMoney` en `server/internal/domain/limits.go`, con el comentario de por qué `ValidMoney` no sirve para el neto (T006 en verde)
- [X] T013 [P] Implementar el tipo `Settlement`, su `Validate` y `DiscountRestaurant` en `server/internal/domain/settlement.go` (T007 en verde)
- [X] T014 Agregar el sentinel `ErrPlatformRefTaken` (envolviendo `ErrConflict` con `%w`) en `server/internal/domain/errors.go` y su caso `409 PLATFORM_REF_TAKEN` en `server/internal/httpapi/respond.go`, con su test en `server/internal/httpapi/respond_test.go`

**Checkpoint**: esquema aplicado y reversible sobre datos reales, reglas puras probadas sin base de datos. Las tres historias pueden empezar.

---

## Phase 3: User Story 1 — el pedido de plataforma nace con su folio (P1) 🎯 MVP

**Goal**: el POS pide el identificador mientras el operador lo tiene enfrente en la tablet de la plataforma, y lo guarda tal cual.

**Independent Test**: se captura un pedido de plataforma con su folio, se cierra la sesión y **se busca ese pedido por el folio** del documento de pago. La búsqueda vive en la US-2, así que la prueba completa de esta historia se cierra ahí; sola, la US-1 entrega el dato irrecuperable guardado.

### Tests primero

- [X] T015 [P] [US1] Escribir `server/internal/integration/folio_de_plataforma_test.go`: se guarda **tal cual** con los extremos recortados; duplicado en la misma empresa y plataforma → `ErrPlatformRefTaken` **nombrando el pedido que ya lo tiene**; el mismo folio en dos empresas **pasa**; el mismo folio en dos plataformas de la misma empresa **pasa**; vacío, solo espacios y 65 caracteres → `ErrValidation`; folio en pedido de mostrador → `ErrValidation`
- [X] T016 [P] [US1] Escribir en el mismo archivo el test de que un pedido **cancelado conserva su folio** — la plataforma también lo canceló y su documento lo trae
- [X] T017 [P] [US1] Escribir en `web/src/features/pos/PlatformPicker.test.tsx`: el campo **no existe en el árbol** con la lista en Mostrador (no "existe oculto"), aparece al elegir plataforma, y su etiqueta nombra la plataforma en vez de decir "folio" a secas
- [X] T018 [P] [US1] Escribir en `web/src/features/pos/POSPage.test.tsx`: mandar un pedido de plataforma con el campo vacío abre la hoja; tomar la salida explícita manda el pedido sin folio; con el campo lleno **no** se interpone nada

### Implementación — backend

- [X] T019 [US1] Agregar `platform_order_ref`, `platform_ref_set_by` y `platform_ref_set_at` a `CreateOrder` y escribir `FindOrderByPlatformRef` (devuelve id, `folio_name`, `daily_number`, `business_date`) en `server/queries/orders.sql`; **sin nombrar `company_id`** — RLS la aplica
- [X] T020 [US1] Correr `make sqlc` y compilar
- [X] T021 [US1] Agregar `PlatformOrderRef *string` a `CreateOrderCmd` y a `OrderView` en `server/internal/app/orders.go`: normalizar con `domain.NormalizePlatformRef`, rechazar folio sin plataforma, y ante violación del índice único resolver el pedido dueño con `FindOrderByPlatformRef` para construir el mensaje que lo nombra
- [X] T022 [US1] Agregar `platformOrderRef` al `createOrderBody` y a la respuesta en `server/internal/httpapi/handlers_orders.go`

### Implementación — pantalla

- [X] T023 [P] [US1] Agregar `platformOrderRef` al ticket activo y limpiarlo junto con la plataforma en `web/src/stores/ticket.ts`
- [X] T024 [P] [US1] Agregar `platformOrderRef` al `CreateOrderBody` en `web/src/api/pos.ts`
- [X] T025 [US1] Poner el campo de folio **en el mismo `HStack` de los botones** de `web/src/features/pos/PlatformPicker.tsx`, visible solo con plataforma activa, etiquetado con el nombre de la plataforma, y subir los botones de `minH="40px"` a `44px` (D-9, D-13, D-18)
- [X] T026 [US1] Crear `web/src/features/pos/FolioPlataformaSheet.tsx`: campo enfocado y **footer fijo** con *Guardar y mandar* / *Mandar sin folio* dentro de un contenedor en `dvh`, para que el teclado no tape la salida (D-10)
- [X] T027 [US1] Interponer esa hoja en el envío del pedido en `web/src/features/pos/POSPage.tsx` solo cuando hay plataforma activa y el folio está vacío

### Medición

- [ ] T028 [US1] Medir a 1024×600 con Playwright en `web/e2e/` los renglones del mosaico con plataforma activa (con y sin el aviso de caja) y el alto del campo, y **anotar el resultado** en `docs/presupuesto-de-pantalla-1024x600.md`. Si el `flexWrap` bajó el campo de renglón, declararlo como el renglón que SC-007 permite

**Checkpoint**: el dato irrecuperable ya se está guardando.

---

## Phase 4: User Story 2 — ver qué pedidos quedaron sin folio, y completarlos (P2)

**Goal**: encontrar un pedido por su folio, quedarse solo con los que no lo tienen, y completarlos con el documento en la mano.

**Independent Test**: se capturan tres pedidos de plataforma, uno sin folio; se filtra y aparece exactamente ese; se le escribe el folio y desaparece del filtro. Y con un folio en la mano, el pedido se encuentra en un solo paso.

### Tests primero

- [X] T029 [P] [US2] Agregar a `server/internal/domain/sales_test.go` la whitelist del filtro nuevo (ausente no filtra, `pendiente` filtra, cualquier otro valor → `ErrValidation` **sin caer a un default**) y la cota de la búsqueda (más de 64 caracteres → `ErrValidation`)
- [X] T030 [P] [US2] Escribir `server/internal/integration/buscar_por_folio_test.go`: pegar el folio del documento devuelve **ese** pedido y solo ese; un folio que nadie tiene devuelve lista vacía y resumen en ceros, no un error; la búsqueda **no** encuentra un pedido por su número interno ni por su nombre de folio; el mismo folio en otra empresa **no** aparece (SC-002, D-16)
- [X] T031 [P] [US2] Escribir `server/internal/integration/corregir_folio_no_mueve_dinero_test.go`, **contra la base restaurada de T002**: fotografía del corte de caja, del arqueo **ya cerrado** y del resumen de ventas antes y después de escribirle el folio a un pedido cobrado de ese arqueo. Falla **nombrando la cifra que se movió**, no "esperaba X obtuve Y" (SC-005, FR-006)
- [X] T032 [P] [US2] Escribir el test de que la lista y el resumen filtrados a pendientes describen **el mismo conjunto**: `total` de la lista == `count` del resumen, sobre datos donde divergirían si el filtro faltara en alguna de las cinco consultas
- [X] T033 [P] [US2] Escribir el test de aislamiento del endpoint de corrección: un usuario de la empresa A recibe **404** al intentar escribirle el folio a un pedido de la empresa B, bajo `appRoleStore`
- [X] T034 [P] [US2] Escribir el test del **limitador por usuario** del endpoint de corrección en `server/internal/httpapi/`: pasado el tope responde `429`, y el tope cuenta solo a quien pasó el gate de rol. Responder por escrito, en el comentario del test, "¿un atacante lo evade?" con un caso concreto (principio V)
- [X] T035 [P] [US2] Escribir en `web/src/features/sales/SalesPage.test.tsx`: el buscador manda el folio exacto; el toggle de pendientes se enciende y se apaga con un tap cada uno; el folio se pinta truncado en una sola línea bajo el nombre de la plataforma

### Implementación — filtro y búsqueda

- [X] T036 [US2] Convertir las cinco consultas de `server/queries/sales.sql` en **cinco pares**, con el predicado `o.delivery_platform_id is not null and o.platform_order_ref is null` **literal** en la variante filtrada (D-8: con el patrón `narg … is null or (…)` el planner no usa el índice parcial). Agregar `platform_order_ref` al select de `ListSales`
- [X] T037 [US2] Agregar la búsqueda exacta `o.platform_order_ref = sqlc.narg('folio')` a las cinco consultas — aquí **sí** va el patrón `narg`, porque es igualdad sobre una columna indexada y no depende de un índice parcial condicionado por el parámetro (D-16)
- [X] T038 [US2] Correr `make sqlc` y compilar
- [X] T039 [US2] Agregar los dos campos y su validación a `SalesFilter.Validate` en `server/internal/domain/sales.go` (T029 en verde)
- [X] T040 [US2] Elegir la variante de consulta según el filtro y pasar la búsqueda en `server/internal/app/sales.go`, y agregar `PlatformOrderRef` a `SaleRow`
- [X] T041 [US2] Leer `folioPlataforma` y `folio` en `filtroDeVentas` de `server/internal/httpapi/handlers_sales.go`, con `valorODefault` aplicado **solo al parámetro ausente**
- [X] T042 [US2] Verificar con `EXPLAIN (ANALYZE, BUFFERS)` y `plan_cache_mode = force_generic_plan` que la consulta de pendientes usa `orders_plataforma_sin_folio` y la de búsqueda usa `orders_platform_ref_busqueda`, sobre la ejecución **vía pgx** y no con psql (SC-008)

### Implementación — la corrección

- [X] T043 [US2] Escribir `SetPlatformRef` en `server/queries/orders.sql` (escribe las tres columnas del rastro juntas) y correr `make sqlc`
- [X] T044 [US2] Implementar `OrdersService.SetPlatformRef` en `server/internal/app/orders.go`: normaliza, rechaza `null`/vacío (**no hay borrado**, D-12), rechaza pedido sin plataforma, resuelve el duplicado con el mensaje que nombra al dueño, y **no toca ninguna otra columna**
- [X] T045 [US2] Agregar el handler `PATCH /orders/{id}/platform-ref` en `server/internal/httpapi/handlers_orders.go`
- [X] T046 [US2] Cablear la ruta en `server/internal/httpapi/router.go` con `RequireRole(admin, gerente, cajero)` y `rateLimitUser`, con el tope **después** del gate de rol (T033 y T034 en verde)

### Implementación — pantalla

- [X] T047 [P] [US2] Agregar el filtro, la búsqueda y `platformOrderRef` a `web/src/api/sales.ts`
- [X] T048 [US2] Agregar a `web/src/features/sales/SalesPage.tsx` el buscador de folio, el **chip/toggle** de pendientes (no un `Picker`: es booleano y el ciclo costaría cuatro toques) y el folio **truncado con elipsis** bajo el nombre de la plataforma en la celda "Tipo" — nunca una columna nueva, porque el contenedor no tiene `overflowX` (D-13). Los rótulos nombran la plataforma, no dicen "folio" a secas (D-18)
- [X] T049 [US2] Mostrar el folio completo y permitir escribirlo o corregirlo desde `web/src/features/sales/SaleDetailDialog.tsx`

**Checkpoint**: con el documento de pago en la mano, cualquier renglón se encuentra en un paso, y lo que falta se ve y se completa.

---

## Phase 5: User Story 3 — registrar lo que la plataforma se quedó (P3)

**Goal**: con el documento de pago en la mano, el dueño anota lo que dice; el sistema puede responder cuánto vendió el pedido, cuánto se quedó la plataforma y cuánto llegó al banco.

**Independent Test**: con un documento real, se registra la liquidación de un pedido y salen tres cifras distintas que hoy no existen.

### Tests primero

- [X] T050 [P] [US3] Escribir `server/internal/integration/liquidacion_de_plataforma_test.go`: el upsert **reemplaza y no duplica**; `GET` sin liquidación devuelve `404` y con ceros devuelve `200` con ceros (FR-015); neto negativo aceptado; comisión negativa, tasa fuera de rango y `discountPlatform > discountTotal` → `400`; RLS aísla entre dos empresas; una liquidación cruzada la rechaza la **FK compuesta**
- [X] T051 [P] [US3] Escribir en el mismo archivo el test de FR-016, **contra la base restaurada de T002**: `/sales/summary`, el corte de caja y el arqueo devuelven **exactamente lo mismo** antes y después de registrar una liquidación
- [X] T052 [P] [US3] Escribir el test de clasificación del resumen del periodo en `server/internal/domain/settlement_test.go`, que falla **nombrando el concepto que se duplicó** si alguien hace que `llegoAlBanco` sea la resta de las otras dos, o si mete la comisión en un total de ventas (FR-017)
- [X] T053 [P] [US3] Escribir el test de autorización: con sesión de **cajero**, `PUT /orders/{id}/settlement` responde `403` (FR-019)
- [X] T054 [P] [US3] Escribir `web/src/features/sales/LiquidacionSheet.test.tsx`: los nueve campos presentes, el derivado *lo que puso el restaurante* se recalcula al teclear, y guardar manda lo que el documento dice sin calcular nada

### Implementación — backend

- [X] T055 [US3] Escribir `server/queries/settlements.sql`: `UpsertSettlement` (`insert … on conflict (order_id) do update`), `GetSettlement` y `PlatformSettlementSummary` por rango de fecha de negocio; **sin nombrar `company_id`** en los `where`
- [X] T056 [US3] Correr `make sqlc` y compilar
- [X] T057 [US3] Implementar `SettlementsService` (`Upsert`, `Get`, `Summary`) en `server/internal/app/settlements.go`: `Round2` en cada importe antes de tocar `numeric`, validación por `domain.Settlement.Validate`, y rechazo si el pedido no es de plataforma
- [X] T058 [US3] Implementar la clasificación del resumen del periodo en `server/internal/domain/settlement.go`, con cada cifra declarando **qué incluye y qué excluye** y su propio conteo de pedidos (T052 en verde)
- [X] T059 [US3] Escribir `server/internal/httpapi/handlers_settlements.go` con `PUT`/`GET /orders/{id}/settlement` y `GET /platform-settlements/summary`
- [X] T060 [US3] Cablear las tres rutas en `server/internal/httpapi/router.go` con `RequireRole(admin, gerente)`

### Implementación — pantalla

- [X] T061 [P] [US3] Crear `web/src/api/settlements.ts`
- [X] T062 [US3] Crear `web/src/features/sales/LiquidacionSheet.tsx`: los **siete campos de dinero en dos columnas**, los dos de texto al final a ancho completo, y **footer de guardar fijo** en un contenedor `dvh` — apilados en una columna son ~630 px y no caben en 600 (D-14)
- [X] T063 [US3] Abrir esa hoja desde `web/src/features/sales/SaleDetailDialog.tsx` y distinguir a la vista "sin liquidación" de "liquidación en ceros"
- [X] T064 [US3] Agregar el grupo de plataformas a `web/src/features/sales/SalesSummaryTiles.tsx`, que **solo se renderiza si el periodo tiene liquidaciones**, con el **conteo de pedidos como nota de cada tile** (como ya hacen Canceladas y Reembolsadas) y el `incluye`/`excluye` detrás de un **icono de ayuda**, no como texto permanente (D-15)

**Checkpoint**: las tres historias funcionan de forma independiente.

---

## Phase 6: Polish & Cross-Cutting

- [ ] T065 Medir por primera vez la pantalla de **Ventas** a 1024×600 con el método del POS y anotar el conteo de renglones antes y después en `docs/presupuesto-de-pantalla-1024x600.md`. Hoy ese documento solo cubre el POS, y esta feature le agregó a Ventas un buscador, un toggle, una fila de tiles y texto en una celda
- [ ] T066 Escribir los casos de e2e en `web/e2e/` para el flujo completo a 1024×600: capturar con folio, tomar la salida explícita, encontrar el pedido pegando el folio, completar un pendiente, y **la salida visible con el teclado abierto**. La suite **cobra los pedidos que crea**
- [ ] T067 Cronometrar en el mismo e2e cuánto toma registrar una liquidación desde que se abre la hoja hasta que se guarda, y declarar el número medido contra los 30 segundos de SC-004
- [ ] T068 Verificar SC-001 con una consulta sobre la base restaurada: **cero** pedidos de plataforma que no tengan folio ni aparezcan en el filtro de pendientes
- [ ] T069 [P] Cerrar los renglones de `docs/matriz-de-pantallas.md` y `docs/matriz-de-cobro.md` con el nombre del test que sostiene cada uno; lo que quede sin test se **declara** sin test
- [ ] T070 Marcar como cruzadas las puertas *conciliar el depósito contra los pedidos que lo formaron* y *cuánto deja cada plataforma* en la tabla del principio VIII de `.specify/memory/constitution.md`, con su bump de versión — **en un commit propio**, como manda Governance
- [ ] T071 Correr el [quickstart.md](quickstart.md) completo de punta a punta
- [ ] T072 Correr los gates: `cd server && go build ./... && go test ./...`, `go test -tags=integration ./internal/integration/...`, `make lint`, `make vuln`, y en `web/` `bun run lint && bun run vitest run && bun run build`

---

## Dependencies & Execution Order

### Entre fases

- **Setup (1)**: T002 (la base restaurada) bloquea al test de la migración, así que va primero de verdad, no como trámite.
- **Foundational (2)**: bloquea **todo**. Nada de las tres historias compila sin la migración y los tipos de dominio.
- **US1 (3)** → **US2 (4)** → **US3 (5)**: es el orden de prioridad del spec, y no es arbitrario. US1 es lo único irrecuperable; US2 es lo que hace que el folio sirva para algo y lo que impide que la salida de escape lo vuelva un campo opcional; US3 es recuperable mientras el pedido tenga folio.
- **Polish (6)**: después de las historias que se vayan a entregar.

### Dependencias reales entre historias

- **La US-1 no se puede probar entera sin la US-2**: su prueba independiente dice "se busca ese pedido por el folio", y la búsqueda vive en la US-2. Sola, la US-1 entrega el dato guardado — que es lo irrecuperable y por eso sigue siendo el MVP.
- **US2 depende de US1**: el filtro y la búsqueda no tienen nada que listar si la columna no se está llenando.
- **US3 depende de US1** solo por el gate de "el pedido es de plataforma"; su tabla y sus endpoints son independientes.

### Dentro de cada historia

Tests → dominio → queries (`make sqlc`) → servicio → handler → ruta → pantalla → medición.

### Paralelizables

- Fase 2: T005, T006 y T007 juntos; después T011, T012 y T013 juntos.
- US1: T015–T018 juntos; T023 y T024 juntos.
- US2: T029–T035 juntos.
- US3: T050–T054 juntos.
- El backend y la pantalla de una misma historia, una vez que el contrato de `contracts/api.md` está implementado.

**Lo que NO se paraleliza**: T036, T037 y T043 tocan `server/queries/*.sql` y disparan `make sqlc`, que regenera el mismo paquete. Dos `make sqlc` a la vez se pisan.

---

## Implementation Strategy

### MVP — solo US1

1. Fase 1 y Fase 2 completas.
2. Fase 3 (US1).
3. **Parar y validar**: capturar un pedido de plataforma con folio y ver que quedó guardado tal cual.
4. Desplegar. A partir de ese momento el dato irrecuperable deja de perderse, que es lo que fija la urgencia de esta feature.

### Entrega incremental

Cada historia se despliega sola. US1 detiene la pérdida; US2 hace que el folio guardado sirva —encontrar el pedido y completar lo que falta—; US3 hace visible una pérdida que lleva años siendo invisible.

---

## Notes

- **Cada test se ve fallar antes de escribir el código**, y por la razón correcta. Un test que nunca estuvo en rojo no prueba nada.
- El nombre de un test dice qué se rompía y por qué importaba, no "esperaba X obtuve Y".
- Commits chicos, firmados, **sin `Co-Authored-By`**.
- `--no-verify` no se usa: si un hook falla, se arregla la causa.
- Antes de aplicar la migración corre el `db-architect`; ya revisó [data-model.md](data-model.md) y sus hallazgos están aplicados, pero el archivo `.sql` real todavía no existe.
- **No quedan desviaciones abiertas entre el spec y el plan**: FR-012 se enmendó el 2026-09-07 y los dos CRITICAL de `/speckit-analyze` (la búsqueda por folio y la base restaurada) están resueltos con tarea.
