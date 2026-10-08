---

description: "Tareas de la feature 030 — una sola puerta para cobrar"
---

# Tasks: Una sola puerta para cobrar — la cuenta vive en el servidor

**Input**: documentos de diseño en `specs/030-una-sola-puerta/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md), [quickstart.md](./quickstart.md)

**Tests**: obligatorios y **primero** (constitución IV). Cada tarea de código va precedida por la de
su test, que se ve **en rojo por la razón correcta** antes de escribir el código. Los tests van contra
el borde (tabla «Bordes → test» del plan), no contra el camino feliz. Ningún test se marca `skip`.

**Dónde se trabaja**: worktree `/home/ramy/git/egb-030-cuentas`, rama `030-una-sola-puerta`. La
integración corre contra `egb030-pg` (:5502), nunca contra `deploy-postgres-1`. Migraciones solo
desde 0082.

**Dos implementadores**: cada tarea lleva **BE** (`server/`) o **FE** (`web/`). Ninguna tarea BE toca
`web/` ni al revés. La frontera es [contracts/api.md](./contracts/api.md), congelado en T003. FE
trabaja contra mocks de `posApi` en vitest hasta que BE termina la tarea de handlers de cada
historia; las tareas FE que necesitan el backend real lo dicen («⇐ T0xx»).

**Abreviaturas**: `IT` = integración con tag `integration` en `server/internal/integration/`, bajo
`appRoleStore` + `AcquireTenant`. `IT HTTP` = contra el router real, sobre **JSON crudo**.
`3C` = `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa). `[P]` = no comparte
archivo ni depende de otra tarea abierta de la misma fase.

---

## Phase 1: Setup

- [ ] T001 Confirmar el punto de partida en verde en el worktree: `cd server && go build ./... && go test ./...`, integración contra `egb030-pg` (`TEST_DATABASE_URL=…:5502/gatobobah_test`), `cd web && bun run lint && bun run vitest run && bun run build`. Anotar cualquier rojo previo en este archivo antes de empezar
- [ ] T002 [P] Preparar `gatobobah_restored` en `egb030-pg` con un respaldo real de **dos empresas**, con dueños y GRANT (`PG_CONTAINER=egb030-pg POSTGRES_DB=gatobobah_restored bash scripts/restaurar-respaldo.sh`), y exportar `TEST_RESTORED_DATABASE_URL` (ver [quickstart.md](./quickstart.md))
- [ ] T003 Congelar el contrato: los dos implementadores leen [contracts/api.md](./contracts/api.md) y [data-model.md](./data-model.md); cualquier cambio posterior se hace en el mismo commit que lo implementa en los dos lados

---

## Phase 2: Foundational (bloquea todas las historias)

### Dominio puro — BE

- [ ] T004 [P] **BE** Test table-driven en server/internal/domain/draft_test.go:
  - `ValidateDraftLine`: qty 0, negativa, NaN, mayor que `MaxOrderQty` → error; modificador con `optionId ≤ 0`, `qty < 1`, `portion = "C"` → error; nota de 201 caracteres → error; 200 → pasa;
  - `MergeTarget`: sin modificadores ni nota se fusiona con el renglón igual; con modificadores, con nota, o producto distinto → renglón nuevo; el renglón igual con nota no recibe la fusión;
  - `DraftExpired`: 11h59m59s → no; 12h exactas → sí;
  - `MaxDraftLines = 200`
- [ ] T005 **BE** Implementar `server/internal/domain/draft.go` (`DraftLine`, `DraftModifier`, `ValidateDraftLine`, `MergeTarget`, `DraftIdleLimit`, `DraftExpired`, `MaxDraftLines`) con el porqué en los comentarios (research R-2, R-8)
- [ ] T006 [P] **BE** Test table-driven en server/internal/domain/account_test.go:
  - `AccountState`: abierta sin pagos → `in_kitchen`; abierta saldada → `paid_in_kitchen`; lista con pago parcial → `partly_paid`; entregada con deuda → `delivered_owes`; entregada saldada → cerrada (no listada); entregada de **$0** → cerrada; entregada con **$0.01** de diferencia → cerrada (misma tolerancia de `PedidoSaldado`); cancelada y reembolsada → no listada; borrador → `capturing`;
  - `AccountGroup`: entregada que debe de hoy → `delivered_owes`; de ayer → `previous_days`; abierta de ayer → `in_kitchen` (sigue en cocina)
- [ ] T007 **BE** Implementar `server/internal/domain/account.go` (`AccountState`, `AccountGroup` y sus constantes en inglés)
- [ ] T008 [P] **BE** Test en server/internal/domain/order_test.go para `CanReceiveLines` (reemplaza los casos de `PuedeRecibirLineas`): entregada y saldada → `ErrOrderClosed`; entregada que debe $5 → ok; abierta saldada → ok (pagada en cocina recibe, US1 AS2); de plataforma en cualquier estado → `ErrPlatformOrderNoLines`; cancelada y reembolsada → `ErrConflict`
- [ ] T009 **BE** Implementar `CanReceiveLines` y los sentinels `ErrOrderClosed`, `ErrPlatformOrderNoLines`, `ErrDraftChanged`, `ErrDraftDiscarded`, `ErrDraftAlreadySent`, `ErrDraftHasOrderHeader` en server/internal/domain/order.go y errors.go (textos para quien opera, sin internals). Borrar `PuedeRecibirLineas` y mover **en la misma tarea** a sus llamadores (`AddLines` en server/internal/app/orders.go y el destino de server/internal/app/move_lines.go); `go build ./...` y los tests existentes de agregar en verde
- [ ] T010 [P] **BE** Test en server/internal/domain/folio_test.go para `AvailableNames(lista, consumidos, usadosTurno, vivos)`: un nombre vivo nunca sale; tampoco cuando la bolsa se vacía (vuelta nueva); un propuesto que está vivo se rechaza; sin vivos se comporta igual que `DisponiblesDeLaBolsa`
- [ ] T011 **BE** Implementar `AvailableNames` en server/internal/domain/folio.go como envoltura de `DisponiblesDeLaBolsa` (un solo predicado, research R-3)
- [ ] T012 [P] **BE** Test en server/internal/httpapi/respond_test.go: cada sentinel nuevo → su HTTP y `code` de la tabla de contracts/api.md; un `ErrDraftChanged` envuelto con `%w` sigue mapeando
- [ ] T013 **BE** Mapear los códigos nuevos en server/internal/httpapi/respond.go

### Esquema — BE

- [ ] T014 **BE** IT de la migración en server/internal/integration/migration_order_drafts_test.go, sobre `gatobobah_restored` (dos empresas): Up aplica; las FKs compuestas rechazan `product_id`, `opened_by`, `delivery_platform_id` y `order_id` de la otra empresa; dos cuentas vivas con el mismo nombre → `23505`; dos «Nuevo» vivas del mismo pedido → `23505`; una enviada y una viva con el mismo nombre conviven; checks de pareja (`status`/`sent_at`/`discarded_at`/`discard_reason`, descuento excluyente, `folio_name`/`folio_scheme`); `gatobobah_app` puede `select/insert/update` en `order_drafts` y **no** `delete`; Down se niega con una cuenta `capturando` y con una `enviada`; Down limpio sin filas. Verificar que el test corre (no `SKIP`)
- [ ] T015 **BE** Escribir server/migrations/0082_order_drafts.sql según [data-model.md](./data-model.md): tres tablas, FKs compuestas `no action` en el orden de los unique existentes (sin crear índices en tablas existentes), índices únicos parciales, RLS con `nullif`, grants explícitos, comentario del porqué (D-1, sin FK en `order_draft_adds.line_id`, techo de crecimiento) y Down que se niega
- [ ] T016 **BE** Correr `TestEveryCompanyTableIsIsolated` (server/internal/integration/rls_all_tables_test.go) y verlo verde con las tres tablas nuevas sin tocarlo
- [ ] T017 **BE** Crear server/queries/drafts.sql vacío (solo el encabezado) y verificar `make sqlc`. **Cada consulta se agrega en la tarea de implementación de su historia, después de la IT que la ejerce** (T030, T032, T034, T044, T062, T074, con `ReleaseFolioName` y su guarda `taken_at <= $created_at` en T074): escribir las consultas antes que sus tests violaría el principio IV

### Base del front — FE

- [ ] T018 [P] **FE** Test en web/src/api/pos.drafts.test.ts: cada función nueva de `posApi` arma la URL, el método y el cuerpo de contracts/api.md (`createDraft`, `getDraft`, `addDraftLine`, `changeDraftLine`, `removeDraftLine` con `expectedVersion` en query, `patchDraft`, `discardDraft`, `sendDraft`, `importDrafts`, `liveAccounts(olderDebts?)`); `liveAccounts()` sin `olderDebts` no manda el parámetro
- [ ] T019 **FE** Implementar los tipos `DraftView`, `DraftLineView`, `AccountItem`, `SendResult` en web/src/types/pos.ts (campos de arreglo **opcionales** donde el compilador deba obligar a la guarda, AGENTS.md §1) y las funciones en web/src/api/pos.ts; textos de `DRAFT_CHANGED`, `DRAFT_DISCARDED`, `DRAFT_SENT`, `ORDER_CLOSED`, `PLATFORM_ORDER_NO_LINES` en web/src/api/mensajes.ts (sin «borrador», «versión» ni códigos)
- [ ] T020 [P] **FE** Test en web/src/components/ConfirmSheet.test.tsx y ReasonSheet.test.tsx: botones ≥ 44 px; la acción destructiva separada de la principal; cerrar con el fondo o Escape = cancelar; `ReasonSheet` devuelve el texto o `null` y su campo no es obligatorio si así se pide
- [ ] T021 **FE** Implementar web/src/components/ConfirmSheet.tsx y ReasonSheet.tsx (hoja inferior de la app, `maxH` en dvh)
- [ ] T022 [P] **FE** Test estático web/src/sinDialogosDelSistema.test.ts: recorre `web/src` y falla nombrando archivo y línea ante `confirm(`, `prompt(` o `alert(` del navegador (excluye `shared/pwa/installPrompt.ts`, que es la API de PWA, y **los comentarios**: `PrintSettingsPage.tsx` nombra `confirm()` en uno). **Queda en rojo** hasta T088; es la vara de FR-015/SC-005
- [ ] T023 [P] **FE** Test en web/src/stores/pos.test.ts: persiste solo `{ selected }` bajo `egb:pos:v3`; un `selected` con forma rara se descarta al cargar (no tumba la pantalla, caso 18); un `selected` que el servidor no encuentra (otra empresa en la tableta, cuenta ya descartada) se limpia sin aviso; no hay renglones ni cabeceras en el almacenamiento
- [ ] T024 **FE** Implementar web/src/stores/pos.ts
- [ ] T025 [P] **FE** Test en web/src/features/pos/useSinConexion.test.ts: `offline` del navegador → sin conexión; un fallo de red en una mutación → sin conexión aunque `navigator.onLine` diga lo contrario; vuelve sola al primer éxito
- [ ] T026 **FE** Implementar web/src/features/pos/useSinConexion.ts

**Checkpoint**: dominio, esquema, consultas y base del front listos. BE y FE avanzan en paralelo.

---

## Phase 3: User Story 2 — La cuenta existe desde el primer producto (P1) 🎯 base del MVP

Va antes que US1 porque la fila de cuentas necesita que las cuentas existan en el servidor.

**Goal**: el primer producto crea la cuenta en el servidor con su nombre; todo lo capturado se guarda
ahí y sobrevive a recargas y a otras tabletas, sin contar como venta.

**Independent Test**: 3 productos, recargar y abrir otra tableta: la cuenta está en las dos con el
mismo nombre; no está en Ventas, corte, tablero ni almacén.

### Backend

- [ ] T027 [US2] **BE** IT en server/internal/integration/drafts_create_test.go:
  - crear con el primer renglón: nombre de la bolsa, `opened_by`, `folio_scheme`; reintento con el mismo `id` devuelve la misma cuenta sin duplicar renglones;
  - nombre propuesto libre → se respeta; propuesto vivo en otra cuenta → otro nombre;
  - `TestTwoDraftsNeverShareAName`: dos goroutines, mismo nombre propuesto, dos nombres distintos;
  - sin turno abierto la cuenta se crea igual (D-6) y no tiene turno, fecha ni movimientos de almacén;
  - `product_id` de la otra empresa → rechazado;
  - 3C sobre crear, leer y la consulta de nombres vivos
- [ ] T028 [US2] **BE** IT `TestADraftIsNeverASale` en server/internal/integration/draft_is_not_a_sale_test.go (SC-001): con una cuenta de 3 productos, cada consulta de dinero y de operación da lo mismo que sin ella — ventas del día, corte (`sessionWithExpected`), reportes, recetas/costeo, Top/populares, tablero (`Board`), `stock_movements`, existencias. Falla nombrando la consulta que la contó
- [ ] T029 [US2] **BE** IT en server/internal/integration/drafts_edit_test.go:
  - agregar es idempotente por `opId`; `TestConcurrentAddsMergeIntoOneLine` (dos goroutines, mismo producto: un renglón, qty 2, posiciones únicas); `intoLineId` suma; `intoLineId` de un renglón quitado → 404;
  - `TestStaleChangeIsRejected`: cambiar o quitar con versión vieja → `ErrDraftChanged` y nada se aplica;
  - quitar el último renglón deja la cuenta vacía y viva con su nombre;
  - cabecera con `headerVersion` vieja → `ErrDraftChanged`; poner descuento o folio de plataforma deja `discount_set_by`/`platform_ref_set_by` con **quien lo cambió**, no quien abrió la cuenta; quitar el descuento los limpia; cabecera en una «Nuevo» → `ErrDraftHasOrderHeader`; cambiar de plataforma reprecia y tira el folio de plataforma; descuento mayor que la venta → rechazado; `MaxDraftLines` → 400;
  - producto desactivado con la cuenta viva → `available=false`, no suma al total;
  - agregar o cambiar una cuenta enviada o descartada → 409;
  - 3C sobre cada escritura
- [ ] T030 [US2] **BE** Escribir en server/queries/drafts.sql las consultas que T027–T029 ejercen (`make sqlc`) e implementar `DraftsService` en server/internal/app/drafts.go: `Create` (barrido primero; nombre con `AvailableNames` dentro de la tx; reintento ×3 ante `23505` del nombre), `Get`, `AddLine`, `ChangeLine`, `RemoveLine`, `PatchHeader` y la vista con precios calculados por la lista de la cuenta (reusa `listaDePrecios`/`BuildOrder` sin guardar precios). **Toda escritura toma `for update` sobre la cuenta primero**
- [ ] T031 [US2] **BE** IT en server/internal/integration/folio_live_names_test.go: `GET /pos/folio-names` no ofrece nombres vivos; `TestMoveLinesSkipsLiveDraftNames`; `TestBagRefillSkipsLiveDrafts` (vaciar la bolsa con cuentas vivas no reparte sus nombres); `Create` directo no se lleva un nombre vivo
- [ ] T032 [US2] **BE** En server/internal/app/orders.go: `resolverFolio` y `NombresDisponibles` excluyen los nombres vivos leídos **dentro de la tx**; `CreateOrderCmd.BoundFolioName` acepta el nombre amarrado aunque esté en `folio_consumido` y lo vuelve a marcar; si ya se usó en el turno cae a `SiguienteFolioLibre`
- [ ] T033 [US2] **BE** IT en server/internal/integration/drafts_import_test.go: importar crea cuentas con el id de la pestaña y sus renglones; reintentar el mismo import no duplica; `TestImportDoesNotResendASentTab` (id ya en `orders.client_uuid` → `already_sent` con el pedido; id ya en `order_line_batches` → igual); pestaña sin renglones → `skipped_empty`; 21 cuentas → 400; 3C
- [ ] T034 [US2] **BE** Implementar `DraftsService.Import` en server/internal/app/drafts.go
- [ ] T035 [US2] **BE** IT HTTP en server/internal/integration/drafts_http_test.go: las rutas de esta fase (crear, leer, agregar, cambiar, quitar, cabecera, importar) con su status; `lines`, `modifiers` y `unavailable` salen `[]`, nunca `null`, sobre el JSON crudo; parámetros inválidos (uuid mal formado, `expectedVersion` no numérico) → 400; cada escritura publica `draft.updated` con `{id, orderId, status, updatedAt}` (broker suscrito en el test); `PATCH` de cabecera con descuento: el tope por usuario responde 429 al pasarse y deja `draft_discount_set` en el log con el anterior y el nuevo (el control del endpoint viejo no se salta por el camino nuevo)
- [ ] T036 [US2] **BE** Handlers finos en server/internal/httpapi/handlers_drafts.go y rutas en server/internal/httpapi/router.go (dentro del grupo con tenant, sin `RequireRole`, junto a `/pos/*`; `PATCH /pos/drafts/{id}` con `rateLimitUser(h.descuentoWrites)` y `SecurityEvent("draft_discount_set")`); publicar `draft.updated`

### Frontend

- [ ] T037 [P] [US2] **FE** Test en web/src/features/pos/useCuenta.test.tsx (mocks de `posApi`):
  - tocar un producto sin cuenta seleccionada crea la cuenta con un `id` y un `opId` nuevos y la selecciona;
  - el renglón se ve «guardando» al instante; si falla, sale de la cuenta y hay un toast «No se guardó · Reintentar» que reintenta con el **mismo** `opId`;
  - sin conexión, agregar no se aplica en silencio y Enviar/Cobrar quedan apagados (US2 AS4, FR-016);
  - «−» con 409 recarga la cuenta; si el renglón ya quedó como se pidió no avisa, si no, avisa «La cuenta cambió en otra tableta»;
  - el id devuelto por el servidor (otra «Nuevo» ya viva) reemplaza al propio
- [ ] T038 [US2] **FE** Implementar web/src/features/pos/useCuenta.ts (TanStack Query `['pos','draft',id]` / `['orders',id]` + mutaciones optimistas)
- [ ] T039 [P] [US2] **FE** Test en web/src/features/pos/subirCuentasViejas.test.ts: `egb:ticket:v2` con dos pestañas con productos → un `importDrafts` con los ids de pestaña y `opId = uuidv5(tab.id, line.lineId)` estables; todas con resultado → borra la llave; falla la red → la conserva; una pestaña con forma vieja que no parsea se salta con aviso y no bloquea a las demás; `already_sent` no deja cuenta viva; corre una sola vez por carga. Absorbe los casos de web/src/stores/cuentaGuardadaAntes.test.ts que sigan aplicando y borra ese archivo
- [ ] T040 [US2] **FE** Implementar web/src/features/pos/subirCuentasViejas.ts y llamarlo al montar el POS
- [ ] T041 [US2] **FE** Primero reescribir los tests (en rojo contra el código viejo) y después conectar web/src/features/pos/POSPage.tsx a `useCuenta` (tocar producto, modificadores, notas, cliente, canal, envío, descuento van al servidor), borrar web/src/stores/ticket.ts y sus usos; reescribir lo que dependía de él en POSPage.test.tsx, Ticket.test.tsx, idempotencia.test.tsx, elFolioLlegaAlServidor.test.tsx y agregarRecorta.test.tsx (cada test conserva el defecto que atrapaba, ahora contra el servidor simulado)

**Checkpoint**: la cuenta vive en el servidor; recargar no pierde nada; ninguna consulta de dinero la ve.

---

## Phase 4: User Story 1 — Todas las cuentas vivas en una fila (P1) 🎯 MVP

**Goal**: una fila con las cuentas vivas de todas las tabletas; «+N» con la lista agrupada; tocar
una la carga en el ticket.

**Independent Test**: cinco cuentas en cinco estados creadas desde otra tableta aparecen en la fila
o en «+N» con su estado; tocar cualquiera la carga.

### Backend

- [ ] T042 [US1] **BE** IT en server/internal/integration/accounts_live_test.go (reemplaza los casos de pedidos_en_curso_test.go y barra_solo_por_cobrar_test.go, que se borran al pasar sus casos aquí):
  - `TestLiveAccountsStates`: capturando, en cocina, pagada en cocina, pago parcial, entregada que debe de hoy y de hace 10 días (`previous_days`), con `state`, `group`, `outstanding` correctos; pagada y entregada no aparece; cancelada no aparece; plataforma aparece;
  - entregada que debe de hace 100 días: no aparece sin `olderDebts`; aparece con `olderDebts=true`;
  - pedido cerrado con «Nuevo» viva → `closedWithPending`;
  - `outstanding` total = suma de los `items` **de pedido**; una cuenta en captura de $500 no lo mueve (falla nombrando «la cuenta en captura se contó como deuda», constitución III);
  - el barrido descarta la cuenta de 13 h y suelta su nombre antes de listar;
  - 3C
- [ ] T043 [US1] **BE** IT `TestLiveAccountsStaysFast` en server/internal/integration/accounts_live_perf_test.go: 30 mil pedidos sintéticos en dos empresas; la consulta sin `olderDebts` ≤ 30 ms y su `EXPLAIN` usa `orders_company_date_status`; con `olderDebts` se anota el tiempo medido (techo del `ponytail`)
- [ ] T044 [US1] **BE** Consulta `ListLiveOrders` en server/queries/orders.sql (reemplaza `ListOpenOrders`; predicado redundante `>= $desde` en sus dos lugares; `// ponytail:` con techo y camino de subida, research R-7) y `ListLiveDrafts`/`ListPendingDraftsByOrder` en server/queries/drafts.sql; `make sqlc`
- [ ] T045 [US1] **BE** Implementar `AccountsService.Live(ctx, olderDebts bool)` en server/internal/app/accounts.go (barrido + unión + `domain.AccountState/AccountGroup`); borrar `OrdersService.Open`
- [ ] T046 [US1] **BE** IT HTTP en server/internal/integration/accounts_http_test.go: `GET /pos/accounts` con `items: []` en JSON crudo cuando no hay nada; `olderDebts=maybe` → 400; `GET /orders/open` ya no existe (404)
- [ ] T047 [US1] **BE** Handler `LiveAccounts` en server/internal/httpapi/handlers_drafts.go, ruta `GET /pos/accounts`; borrar `OpenOrders` y la ruta `/orders/open`

### Frontend

- [ ] T048 [P] [US1] **FE** Test en web/src/features/pos/useCuentasVivas.test.ts: consulta `['pos','accounts']` cada 30 s; un evento `draft.updated` u `order.*` la invalida; la tableta que vuelve de suspenderse refresca
- [ ] T049 [US1] **FE** Implementar web/src/features/pos/useCuentasVivas.ts y montar `useOrderEvents` (web/src/hooks/useOrderEvents.ts, extendido a `draft.updated`) en el POS
- [ ] T050 [P] [US1] **FE** Test en web/src/features/pos/FilaDeCuentas.test.tsx:
  - con un ancho dado pinta solo fichas completas de 120 px, luego «+N» y «+»; «+N» cuenta exactamente las que no se ven;
  - orden: seleccionada, las que deben, por antigüedad;
  - cada estado con su texto corto y color; ninguna ficha tiene ✕;
  - fichas y botones ≥ 44 px; «+» deja sin selección (la cuenta nace al primer producto)
- [ ] T051 [US1] **FE** Implementar web/src/features/pos/FilaDeCuentas.tsx
- [ ] T052 [P] [US1] **FE** Test en web/src/features/pos/TodasLasCuentasSheet.test.tsx: grupos con su conteo y en orden; un grupo vacío no se pinta; buscador solo con más de 8; al abrirse pide `olderDebts=true`; renglones de 56 px con hora, antigüedad y lo que falta; tocar uno lo selecciona y cierra la hoja
- [ ] T053 [US1] **FE** Implementar web/src/features/pos/TodasLasCuentasSheet.tsx (hoja `maxH="85dvh"`, scroll de la lista con `minH={0}`)
- [ ] T054 [US1] **FE** Primero el test en POSPage.test.tsx (la fila no desborda con 10 cuentas; no queda `TicketTabs` ni el botón naranja), después la fila 2 en web/src/features/pos/POSPage.tsx: `FilaDeCuentas` en lugar de `TicketTabs` y `PedidosEnCurso`; buscador `clamp(120px,20%,200px)` que se pliega a botón de 44 px con el panel abierto. Borrar web/src/features/pos/TicketTabs.tsx, PedidosEnCurso.tsx y PedidosEnCurso.test.tsx (sus casos vivos pasan a T050/T052) y `posApi.openOrders`
- [ ] T055 [P] [US1] **FE** Test en web/src/features/pos/abrirDesdeLaUrl.test.tsx: `/pos?pedido=12` selecciona ese pedido y limpia el parámetro; `?cuenta=<uuid>` igual; un id que no existe → «Esa cuenta ya no existe» y no selecciona otra; un valor malformado se rechaza igual (no cae a otra cuenta)
- [ ] T056 [US1] **FE** Implementar la lectura del parámetro en web/src/features/pos/POSPage.tsx
- [ ] T057 [P] [US1] **FE** Test en web/src/features/orders/OrdersBoardPage.test.tsx: cada tarjeta tiene «Abrir cuenta» (44 px) que navega a `/pos?pedido=<id>`; reemplaza la puerta de «Cobrar» del tablero como única ruta al cobro (caso 25)
- [ ] T058 [US1] **FE** Implementar «Abrir cuenta» en web/src/features/orders/OrdersBoardPage.tsx

**Checkpoint (MVP)**: US2 + US1 — una sola fila, todas las tabletas, nada se pierde.

---

## Phase 5: User Story 3 — Agregar después de cocina manda solo lo nuevo (P1)

**Goal**: en un pedido enviado, lo nuevo queda en «Nuevo» (en el servidor) hasta «Enviar N a cocina»;
cocina recibe solo eso.

**Independent Test**: pedido con 3 productos; agregar 1; «Enviar 1 a cocina» imprime solo ese y el
tablero lo muestra pendiente.

### Backend

- [ ] T059 [US3] **BE** Extraer `createInTx` y `addLinesInTx` en server/internal/app/orders.go sin cambiar comportamiento: agregar_a_pedido_test.go, agregar_es_idempotente_test.go, comanda_del_agregado_test.go, folio_propuesto_test.go, bolsa_de_folios_test.go y cobrar_exige_confirmar_test.go siguen verdes **antes** de escribir `Send` (refactor con red)
- [ ] T060 [US3] **BE** IT en server/internal/integration/drafts_send_test.go:
  - enviar una cuenta sin pedido crea el pedido con folio del turno, fecha del reloj, inventario por renglón, todo marcado enviado a cocina, el **nombre amarrado**, `opened_by` de la cuenta y `discount_set_by`/`platform_ref_set_by` de quien los puso en la cuenta (no de quien tocó «Enviar»); `printLineIds` = todos;
  - `TestSendIsIdempotent`: dos envíos → un pedido, el segundo con `printLineIds` vacío;
  - dos tabletas envían la misma cuenta a la vez → un pedido;
  - sin turno → `NO_OPEN_REGISTER` y la cuenta intacta;
  - producto desactivado → 422 con su nombre; opción de modificador borrada → 422 con el producto, nunca 500;
  - plataforma capturada a mano sin folio → 400;
  - `TestDraftAcrossShiftsKeepsItsAnimal`: cuenta creada en un turno, turno cerrado, otro pedido toma el mismo nombre en el turno nuevo → la cuenta sale como «Persa 2», nunca otro animal;
  - el barrido y el envío a la vez → la cuenta queda enviada;
  - SC-006: una cuenta descartada no consume `daily_number`; los folios del turno no tienen huecos ni repetidos;
  - 3C
- [ ] T061 [US3] **BE** IT en server/internal/integration/drafts_new_lines_test.go:
  - crear «Nuevo» de un pedido en cocina; una segunda tableta que crea otra «Nuevo» del mismo pedido recibe la misma con su renglón sumado;
  - enviarla pasa por `AddLines`: `printLineIds` solo lo nuevo, el tablero lo muestra pendiente;
  - entregada que debe → vuelve a `abierta` (US3 AS4);
  - `TestClosedOrderReceivesNothing`: pagado y entregado → crear «Nuevo» y `POST /orders/{id}/lines` → `ORDER_CLOSED`;
  - `TestPlatformOrderReceivesNothing` → `PLATFORM_ORDER_NO_LINES`;
  - pedido que se cerró con su «Nuevo» viva → enviar `ORDER_CLOSED`, la cuenta sigue viva y listada con `closedWithPending`;
  - `TestCancelledOrderDiscardsItsNew`: cancelar el pedido → el barrido descarta su «Nuevo» (`order_closed`);
  - `TestSendWhileAddingNeverLosesALine`: enviar y agregar a la vez → el renglón quedó en el pedido o en una «Nuevo» viva, nunca se pierde
- [ ] T062 [US3] **BE** Implementar `DraftsService.Send` en server/internal/app/drafts.go (preparar fuera, una tx: `for update` de la cuenta antes de mirar llaves, verificar `updated_at`, `createInTx`/`addLinesInTx` con `clientUuid = draft.id`, marcar enviada con `order_id`) y la regla nueva en `AddLines` y en la creación de «Nuevo»
- [ ] T063 [US3] **BE** Handler `SendDraft` y ruta `POST /pos/drafts/{id}/send` en server/internal/httpapi/handlers_drafts.go; publica `order.created`/`order.updated` y `draft.updated`. IT HTTP en drafts_http_test.go: `printLineIds` siempre arreglo

### Frontend

- [ ] T064 [P] [US3] **FE** Test en web/src/features/pos/Ticket.test.tsx:
  - cuenta en captura: solo «Nuevo» con −/+;
  - pedido con pagos y en cocina: «Nuevo» primero con −/+, «En cocina» compacto (40 px, marca de entregado, sin −/+, quitar en el ⋮ del renglón), «Pagado» con candado y sin ⋮; «En cocina» y «Pagado» se pliegan solos con más de 3 renglones;
  - el pie solo tiene totales y dos botones (caso 30): «Enviar N a cocina» con N = renglones de «Nuevo»;
  - con algo guardando o sin conexión, los dos botones apagados y el motivo en una línea
- [ ] T065 [US3] **FE** Reescribir web/src/features/pos/Ticket.tsx con las tres secciones y el ⋮ de cabecera (cliente, canal, descuento, envío)
- [ ] T066 [P] [US3] **FE** Test en web/src/features/pos/useEnviarCuenta.test.tsx: enviar imprime la comanda con `printLineIds` (`KitchenTicket` con `soloLineas`); el reintento con `printLineIds` vacío no reimprime; `ORDER_CLOSED` ofrece «Empezar cuenta nueva con estos productos» (crea cuenta nueva con los mismos renglones y descarta la vieja); `PLATFORM_ORDER_NO_LINES` lo dice y no ofrece agregar; la cuenta enviada se queda seleccionada y en la fila (caso 5)
- [ ] T067 [US3] **FE** Implementar web/src/features/pos/useEnviarCuenta.ts (⇐ T063 para la integración real) y borrar useMandarPedido.ts y useAgregarAPedido.ts

**Checkpoint**: la mesa que sigue pidiendo funciona sin dos cuentas.

---

## Phase 6: User Story 4 — Cobrar siempre ofrece los tres modos (P1)

**Goal**: «Cobrar» desde el ticket abre los tres modos para toda cuenta de mostrador; si hay algo
nuevo, lo envía primero.

**Independent Test**: cuenta capturándose con 4 productos → «Enviar y cobrar» → «Por productos» → 2
productos → la fila la muestra «Pago parcial · falta $X».

- [ ] T068 [US4] **BE** IT en server/internal/integration/drafts_charge_test.go: enviar y cobrar 2 de 4 productos por productos → la lista la da `partly_paid` con el `outstanding` correcto; dos tabletas «Enviar y cobrar» la misma cuenta a la vez → un pedido, el segundo cobro solo cobra lo que falta (edge case del spec); 3C de la lista tras el cobro
- [ ] T069 [P] [US4] **FE** Test en web/src/shared/CobrarSheet.test.tsx y CobrarSheet.split.test.tsx: cuenta de mostrador con id de pedido → los tres modos siempre; de plataforma → solo completo con su método; cerrar la hoja sin cobrar no cancela nada y la cuenta sigue en la fila (US4 AS2)
- [ ] T070 [US4] **FE** Ajustar web/src/shared/cobro/ModePicker.tsx y web/src/shared/CobrarSheet.tsx (`splittable` ya no depende de que exista el pedido: siempre lo hay al abrir)
- [ ] T071 [P] [US4] **FE** Test en web/src/features/pos/cobrarDesdeElTicket.test.tsx: con «Nuevo» el botón dice «Enviar y cobrar $X» y envía antes de abrir la hoja; sin nada nuevo dice «Cobrar $X» y abre directo; si el envío falla la hoja no se abre y el motivo queda en el pie; sin conexión apagado
- [ ] T072 [US4] **FE** Implementar el flujo en web/src/features/pos/Ticket.tsx y POSPage.tsx (⇐ T063)

---

## Phase 7: User Story 5 — Nada se cierra ni se pierde por accidente (P1)

**Goal**: vacía se descarta sola; con productos pregunta en una hoja de la app; enviada solo se
cancela desde ⋮; ningún diálogo del sistema.

**Independent Test**: cerrar cada tipo de cuenta; el test estático no encuentra diálogos del sistema.

- [ ] T073 [US5] **BE** IT en server/internal/integration/drafts_discard_test.go: `TestDiscardReturnsTheName` (el nombre vuelve a `GET /pos/folio-names`); descartar dos veces → 204; descartar una enviada → `DRAFT_SENT`; `TestDiscardAfterBagRefillKeepsTheNewOwner` (bolsa vaciada y nombre retomado por un pedido → descartar no lo suelta); descartar no consume folio; `discarded_by` y `discard_reason` quedan; IT HTTP: `POST /pos/drafts/{id}/discard` → 204 y 409 en JSON crudo; 3C
- [ ] T074 [US5] **BE** Escribir `ReleaseFolioName` en server/queries/folios.sql y la consulta de descartar; implementar `DraftsService.Discard` en server/internal/app/drafts.go, handler y ruta `POST /pos/drafts/{id}/discard`
- [ ] T075 [P] [US5] **FE** Test en web/src/features/pos/DescartarCuentaSheet.test.tsx: cuenta vacía → se descarta sin hoja; con productos → «¿Descartar la cuenta de Levkoy? Se pierden 2 productos ($74)…»; «Seguir capturando» es la principal y no cambia nada; «Descartar» rojo, separado, descarta y deja sin selección. El ⋮ de cabecera ofrece «Descartar cuenta» solo en captura y «Cancelar pedido» solo en enviada (y solo con permiso `orders.cancel`); una enviada no tiene ninguna otra forma de cerrarse
- [ ] T076 [US5] **FE** Implementar web/src/features/pos/DescartarCuentaSheet.tsx y el ⋮ de cabecera del ticket: «Descartar cuenta» solo en captura, «Cancelar pedido» (permiso `orders.cancel`, flujo existente) solo en enviada; ninguna otra forma de cerrar una enviada (US5 AS3). Quitar el `confirm('¿Vaciar pedido?')` de Ticket.tsx
- [ ] T077 [P] [US5] **FE** Test en web/src/features/backoffice/ExpensesPage.test.tsx: cancelar un gasto pide el motivo en `ReasonSheet` (opcional) y no llama a `prompt`
- [ ] T078 [US5] **FE** Reemplazar el `prompt()` de web/src/features/backoffice/ExpensesPage.tsx por `ReasonSheet`

---

## Phase 8: User Story 7 — Red, recarga y otra tableta (P2)

**Goal**: sin conexión se dice y se reintenta; lo que cambia en otra tableta se ve con aviso.

- [ ] T079 [P] [US7] **FE** Test en web/src/features/pos/cambioEnOtraTableta.test.ts (función pura): cobrada, cancelada, enviada, descartada y editada en otra tableta → aviso con el nombre («Siamés se cobró en otra tableta»); las operaciones propias en vuelo no avisan; un agregado ajeno suma sin aviso de conflicto (US7 AS2)
- [ ] T080 [US7] **FE** Implementar web/src/features/pos/cambioEnOtraTableta.ts y conectarlo a `useCuenta`
- [ ] T081 [P] [US7] **FE** Test en web/src/features/pos/AvisoSinConexion.test.tsx: aparece en la franja del encabezado, superpuesto (no cambia el alto de nada), con el panel abierto y cerrado; desaparece solo al volver
- [ ] T082 [US7] **FE** Implementar web/src/features/pos/AvisoSinConexion.tsx y montarlo en POSPage.tsx

---

## Phase 9: User Story 6 — Quitar algo que ya está en cocina (P2)

- [ ] T083 [P] [US6] **FE** Test en web/src/features/pos/Ticket.test.tsx: en «En cocina», el ⋮ del renglón abre `CancelarRenglonDialog` con contador, motivos sin preselección y el texto de qué pasa; quitar 1 de 2 deja ×1 y baja el total; un renglón pagado no tiene ⋮ (candado). El servidor ya lo rechaza (dividir_la_cuenta_test.go); no hay BE nuevo
- [ ] T084 [US6] **FE** Conectar el ⋮ del renglón a la hoja existente en web/src/features/pos/Ticket.tsx

---

## Phase 10: User Story 8 — Cerrar caja con cuentas vivas (P2)

- [ ] T085 [US8] **BE** IT `TestCloseLiveAccountsDoNotBlock` en server/internal/integration/cierre_con_cuentas_vivas_test.go: con un pedido en cocina el cierre sigue bloqueado (`OPEN_ORDERS`, `Pending` sin cambio); con solo cuentas en captura y entregadas que deben (una de hace 100 días) cierra, `liveAccounts` las lista, y al día siguiente las que deben están en `previous_days` y las capturando siguen vivas; JSON crudo `liveAccounts: []`
- [ ] T086 [US8] **BE** Agregar `LiveAccounts` a `SessionView` en server/internal/app/backoffice.go desde `AccountsService.Live(ctx, true)` filtrado a los grupos no bloqueantes; `sinPedidosPendientes` no se toca
- [ ] T087 [P] [US8] **FE** Test en web/src/features/backoffice/CashPage.test.tsx: bloqueantes arriba con «Abrir» (navega a `/pos?pedido=`); sección plegada «Cuentas pendientes (N)» con «Abrir» y «Descartar» de 44 px y separados; «Descartar» confirma con `ConfirmSheet`; «Cerrar caja» confirma con `ConfirmSheet` y no llama a `confirm`
- [ ] T088 [US8] **FE** Implementar en web/src/features/backoffice/CashPage.tsx; con esto `sinDialogosDelSistema.test.ts` (T022) queda **verde**

---

## Phase 11: Verificación final (BE + FE juntos)

### e2e contra el ambiente de pruebas, 1024×600

- [ ] T089 **FE** web/e2e/limpiar-lo-que-cree.ts aprende a descartar las cuentas en captura que creó la suite (las que ya estaban se anotan en el `globalSetup` y no se tocan) y sigue entregando y cobrando sus pedidos
- [ ] T090 **FE** Reescribir las referencias a «Cuenta 1» / «Pedidos por cobrar» en web/e2e/medir-no-estorba.spec.ts, folio-de-plataforma.spec.ts, contar-el-cajon.spec.ts, split-bill-incident.spec.ts, deuda-de-especificaciones.spec.ts, cobro-en-pantalla.spec.ts y cabe-en-la-tableta.spec.ts al flujo nuevo, conservando el defecto que cada una atrapa; usan `tokenDeApi`/`tokenDeRequest` (web/e2e/ambiente.ts)
- [ ] T091 **FE** Medición en web/e2e/cabe-en-la-tableta.spec.ts a 1024×600: fichas visibles con panel abierto ≥ 2 y cerrado ≥ 4; la fila no desborda con 10 cuentas; ticket con las tres secciones muestra ≥ 4 renglones y el pie visible; la hoja «+N», `DescartarCuentaSheet`, el aviso sin conexión y el cierre de caja caben con su botón de confirmar visible; todo control ≥ 44 px (SC-004)
- [ ] T092 **FE** web/e2e/una-sola-puerta.spec.ts: un test por caso del lienzo y por historia, con el número del caso en el título (mapa abajo). Dos contextos de navegador = dos tabletas
- [ ] T093 Validar como usuario nuevo: capturas reales a 1024×600 de la fila, la hoja «+N», el ticket con tres secciones, la hoja de descartar y el cierre; un agente que nunca vio el POS intenta los recorridos de las 8 historias y reporta dónde se atora. Lo que encuentre deja su test antes del arreglo

### Mapa caso del lienzo → test

| # | Caso | Test |
|---|---|---|
| 1 | Cuenta en curso se pierde de vista | e2e `caso 1` + IT `TestLiveAccountsStates` |
| 2 | Pagado en cocina no recibía | e2e `caso 2` + unit `CanReceiveLines` (abierta saldada → ok) |
| 3 | Deuda de días anteriores | e2e `caso 3` + IT `TestLiveAccountsStates` (`previous_days`, `olderDebts`) |
| 4 | Entregada y debe vs cerrada | unit `AccountState` + e2e `caso 4` (ficha roja) |
| 5 | Enviar borraba la cuenta | vitest `useEnviarCuenta` + e2e `caso 5` |
| 6 | Agregar después: solo lo nuevo | IT `drafts_new_lines_test` + e2e `caso 6` (comanda) |
| 7 | Agregar a entregada llega a cocina | IT `drafts_new_lines_test` (reabre) + e2e `caso 7` |
| 8 | Quitar 1 de 2 | vitest T083 + e2e `caso 8` |
| 9 | Quitar pagado | vitest T083 (candado) + IT existente `dividir_la_cuenta_test` |
| 10 | `confirm()` nativo | vitest `sinDialogosDelSistema` |
| 11 | Cerrar la hoja de cobro | vitest T069 + e2e `caso 11` |
| 12 | Pedido atorado | IT existente `no_way_out_test` + e2e `caso 12` (el cierre abre cada cuenta) |
| 13 | Cancelar con algo entregado | IT existente de `CancelPending` + e2e `caso 13` (desde ⋮) |
| 14 | Tableta se apaga | e2e `caso 14` (cerrar contexto, abrir otro) |
| 15 | Red caída al confirmar | IT `TestSendIsIdempotent` + e2e `caso 15` (ruta que corta la respuesta) |
| 16 | Otra tableta cobró o canceló | vitest `cambioEnOtraTableta` + e2e `caso 16` |
| 17 | Dos tabletas editan | IT `TestConcurrentAddsMergeIntoOneLine`, `TestStaleChangeIsRejected` + e2e `caso 17` |
| 18 | Pantalla en blanco por cuenta vieja | vitest `subirCuentasViejas` (forma rota) + vitest `pos.test` + e2e `caso 18` (localStorage sembrado) |
| 19 | Plataforma | IT `TestPlatformOrderReceivesNothing` + vitest T069 + e2e `caso 19` |
| 20 | Uber aceptado sin caja | **Fuera: 031** |
| 21 | Cerrar caja con cuentas vivas | IT `TestCloseLiveAccountsDoNotBlock` + e2e `caso 21` |
| 22 | Turno largo: fecha y folio | IT `drafts_send_test` (fecha del reloj al enviar) + IT existente `el_dia_de_la_venta_no_cambia_test` |
| 23 | El nombre cambia / bolsa agotada | IT `TestTwoDraftsNeverShareAName`, `TestDraftAcrossShiftsKeepsItsAnimal`, `TestBagRefillSkipsLiveDrafts` + e2e `caso 23` |
| 24 | Borradores inflan folio y reportes | IT `TestADraftIsNeverASale` + SC-006 en `drafts_send_test` |
| 25 | Tres puertas | vitest T057 + e2e `caso 25` |
| 26 | «Por productos» desde el ticket | vitest T069 + e2e `caso 26` |
| 27 | Descuento tras pagos / productos movidos | IT existentes de la 027 (`dividir_la_cuenta_test`, `move_lines_test`), sin cambio |
| 28 | Devoluciones dobles | **Fuera: 031** |
| 29 | La fila no cabe | e2e T091 |
| 30 | Iconos junto a Enviar/Cobrar | vitest T064 (pie con dos botones) |

Por historia: US1 `historia 1` · US2 `historia 2` (recargar y otra tableta) · US3 `historia 3` ·
US4 `historia 4` · US5 `historia 5` · US6 `historia 6` · US7 `historia 7` · US8 `historia 8`, en
web/e2e/una-sola-puerta.spec.ts.

### Documentación y gates

- [ ] T094 [P] Actualizar docs/matriz-de-cobro.md y docs/matriz-de-pantallas.md con los casos nuevos y lo que **no** queda cubierto (casos 20 y 28; fiados de más de 90 días fuera de la fila)
- [ ] T095 [P] Actualizar la leyenda del mapa de toques en web/src/consola/zonas-del-pos.ts y su `FECHA_DEL_LAYOUT`: la fila 2 y el ticket cambian de disposición (AGENTS.md, «la leyenda se actualiza con cada rediseño del POS»). Documentar en AGENTS.md la mecánica nueva: la cuenta en captura vive en `order_drafts` y nunca es pedido; `egb:pos:v3` solo guarda la selección; `GET /pos/accounts` reemplaza a `/orders/open`; el nombre se amarra al nacer
- [ ] T096 Gates completos en el worktree: `go build ./... && go test ./...`; integración contra `egb030-pg` con `TEST_DATABASE_URL` y `TEST_RESTORED_DATABASE_URL` (incluido `TestEveryCompanyTableIsIsolated`); `go test -tags=integration -v -run MigrationOrderDrafts ./internal/integration/... | grep -c SKIP` da 0; golangci-lint (`scripts/hooks/golangci-lint.sh`); `bun run lint && bun run vitest run && bun run build`; `bun audit --audit-level=high`
- [ ] T097 Correr la suite e2e completa en contenedor contra la imagen de la rama en el ambiente de pruebas (quickstart §3) y verificar al final que no queda nada de la suite en `GET /pos/accounts`
- [ ] T098 Correr `/revision-de-codigo` sobre el diff (hook `after_implement`) y resolver lo que encuentre, con su test antes del arreglo
- [ ] T099 Al fusionar: renumerar la migración al siguiente libre de `develop` (0080–0081 son de otra rama) y volver a correr T096

---

## Dependencies & Execution Order

- **T001–T003** → todo.
- **Fase 2**: BE (T004–T017) y FE (T018–T026) en paralelo. T009 antes que cualquier cambio a `AddLines`; T015 antes que T016–T017; T017 bloquea todo BE de historias.
- **US2 (Fase 3)** bloquea US1, US3, US4, US5, US7 y US8 en los dos lados (las cuentas tienen que existir).
- **US1 (Fase 4)**: BE necesita T030; FE necesita T038. T054 (fila) después de T051 y T053.
- **US3 (Fase 5)**: T059 (refactor con red) antes de T062. FE T067 ⇐ T063 para integrar.
- **US4 (Fase 6)** necesita T063 y T065.
- **US5 (Fase 7)**: T076 necesita T065 (⋮ de cabecera).
- **US7 (Fase 8)** necesita T038 y T049.
- **US6 (Fase 9)** necesita T065.
- **US8 (Fase 10)**: T086 necesita T045; T088 cierra T022.
- **Fase 11** al final; T092 necesita todas las historias desplegadas en la imagen de la rama.

### Carril BE (implementador 1)

T004–T017 → T027–T036 → T042–T047 → T059–T063 → T068 → T073–T074 → T085–T086 → (T096, T098 con FE)

### Carril FE (implementador 2)

T018–T026 → T037–T041 → T048–T058 → T064–T067 → T069–T072 → T075–T078 → T079–T084 → T087–T088 → T089–T093

Puntos de sincronía: T036 (handlers de cuentas) para integrar T041; T047 para T054; T063 para T067 y
T072; T074 para T076; T086 para T088.

## Parallel Opportunities

- Fase 2: T004, T006, T008, T010, T012 (tests BE) y T018, T020, T022, T023, T025 (tests FE) a la vez.
- Dentro de cada historia, los tests FE marcados [P] van en paralelo con todo el carril BE.
- Varias IT de una misma historia comparten helpers pero no archivo: van en paralelo si su fixture
  no comparte empresa.

## Implementation Strategy

1. **Base**: Fase 2 en los dos carriles.
2. **MVP**: US2 + US1 — la cuenta vive en el servidor y hay una sola fila. Ya resuelve los casos 1, 3,
   14, 15, 18, 24 y 25.
3. US3 + US4: la mesa que sigue pidiendo y la puerta única de cobro (casos 2, 5–7, 11, 26).
4. US5: nada se pierde por accidente (caso 10).
5. US7, US6, US8 (P2).
6. **La rama no se fusiona antes de terminar US4**: entre US1 y US4 el POS tendría la fila nueva con
   el cobro viejo, y «Por productos» seguiría sin salir desde el ticket.
7. Cada fase cierra con sus gates en verde y su checkpoint.
