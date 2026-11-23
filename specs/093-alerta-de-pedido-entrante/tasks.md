---
description: "Tareas de la spec 093: alerta continua de pedido entrante y sección «Por aceptar»"
---

# Tasks: Alerta continua de pedido entrante y sección «Por aceptar»

**Input**: `specs/093-alerta-de-pedido-entrante/` — plan.md, spec.md, research.md, data-model.md,
contracts/incoming-orders.md, quickstart.md

**Tests**: obligatorios (constitución IV, TDD). Cada tarea de test va ANTES de su implementación y se
ve fallar por la razón correcta.

**Organización**: por historia de usuario del spec. Las pantallas citan su fila de «Maquetas» del
plan: **M-Indicador** (franja en fila), **M-PorAceptar** (B2, el más urgente abierto, origen como
ícono), **M-Pedidos** (ticket junto al total + pestaña «Cerrados»).

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

- [X] T001 Verificar que 0118 sigue libre en `origin/develop` (`git fetch && git ls-tree origin/develop server/migrations/`) y que el id de permiso 39 no lo tomó otra rama; si no, renumerar en plan.md, data-model.md y aquí
- [X] T002 [P] Script de desarrollo `scripts/entrantes-de-prueba.sh` (y su objetivo `make entrantes-de-prueba n=…` en `Makefile`) que inserta N pedidos de plataforma pendientes en la base LOCAL del worktree, con renglones y `decide_before` a 11 min; se niega fuera de un worktree con `.env.worktree.local` y nunca toca `deploy-postgres-1`; su test en `scripts/tests/entrantes-de-prueba-test.sh`

---

## Phase 2: Foundational (bloquea a todas las historias)

### Dominio (puro)

- [X] T003 [P] Test table-driven `server/internal/domain/incoming_test.go`: `IsPending` (pendiente con plazo futuro, vencido, sin plazo, aceptado/rechazado/cancelado), `SortByUrgency` (plazo cercano primero, sin plazo al final por antigüedad, desempate por id), `ParseIncomingSource` (desconocido = `ErrValidation`), `IncomingRejectReason` (los 5 códigos válidos, vacío y desconocido = `ErrValidation`, nunca cae a `other`) y su traducción a código de la plataforma
- [X] T004 Implementar `server/internal/domain/incoming.go`: `IncomingOrder`, `IncomingLine` (slices nunca nil), `IncomingSource` (`platform`), `IsPending`, `SortByUrgency`, `IncomingRejectReason` con texto en español y `PlatformRejectCode`
- [X] T005 [P] Agregar `PermOrdersAcceptIncoming = "orders.accept_incoming"` a `server/internal/domain/permission.go` (y a `AllPermissions`), con su test en `permission_test.go` si el catálogo se prueba ahí

### Migración 0118

- [X] T006 Test de integración `server/internal/integration/migration_0118_test.go` contra base restaurada con dos empresas, cada una con un rol propio sembrado: tras Up, los 4 sugeridos tienen 39 por herencia y cada rol propio existente su override `granted=true` (`company_id` correcto, sin cruzar empresas); un rol propio creado después del Up NO lo tiene; Up repetido no duplica; Down borra todo; Down se niega con un override `granted=false`; `UserEffectivePermissions` de la API con el 39 no falla
- [X] T007 Migración `server/migrations/0118_incoming_orders_permission.sql` según data-model.md (company_id explícito, `on conflict do nothing`, Down con guarda y orden de borrado) y renglón `118` en `guardedDownCleanup` de `server/internal/integration/harness_restaurado_test.go`

### Consultas (sqlc)

- [X] T008 En `server/queries/pedidos_de_plataforma.sql`: `ListPendingIncomingOrders` filtra `decide_before is null or decide_before > now()` y ordena `decide_before nulls last, placed_at, id`; nueva `CancelIncomingOrder :execrows` (`state='cancelado', settled_at=now()` por `connection_id, external_order_id` con `state='pendiente'`); `make sqlc`

### Configuración de desarrollo

- [X] T009 [P] Test en `server/internal/config/config_test.go`: `PLATFORM_ORDERS_SIMULATED=true` con `APP_ENV=production` hace fallar `Validate`; con `development` pasa
- [X] T010 Implementar el campo y el check en `server/internal/config/config.go`, y en `server/cmd/api/main.go` pasar un decisor simulado (confirma aceptar y rechazar sin red, valida el motivo con la lista del dominio) a `PedidosDePlataformaService` cuando está encendido; documentar la variable en `deploy/.env.example` como solo desarrollo

### Servicio de plataforma: rechazar, cancelar, plazo

- [X] T011 Tests de integración en `server/internal/integration/pedidos_de_plataforma_test.go` (decisor falso): (a) `Rechazar` con motivo válido deja `rechazado`, `deny_reason`, `decided_by`; (b) la plataforma no confirma el rechazo → sigue `pendiente`; (c) rechazar un aceptado → `ErrPedidoYaDecidido`; (d) aceptar o rechazar vencido → `ErrPedidoYaDecidido` SIN llamar a la plataforma; (e) webhook `orders.cancel` de un pendiente → `cancelado` y `AvisoRecibido` marca cambio; (f) cancelación de un ya aceptado → cero filas, sin error, sigue `aceptado`; (g) dos `Aceptar` concurrentes, repetido 20 veces con pedidos distintos (SC-004) → en cada uno un pedido del POS, un cobro, el otro `ErrPedidoYaDecidido`; (h) aceptar mientras otra rechaza → solo una gana
- [X] T012 Implementar en `server/internal/app/pedidos_de_plataforma.go`: `Rechazar(ctx, id, usuario, motivo)`, la verificación de plazo antes de llamar a la plataforma en `Aceptar`/`Rechazar`, la rama `AvisoPedidoCancelado` en `procesar` con `CancelIncomingOrder`, y `AvisoRecibido.Cambio`

### Servicio común y rutas

- [X] T013 Tests de integración `server/internal/integration/incoming_orders_test.go` bajo `appRoleStore` con `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa) para `IncomingOrdersService.Pending`, `Accept` y `Reject`; y test del handler sobre el JSON crudo: `orders`, `lines` y `options` son `[]` y nunca `null`
- [X] T014 Implementar `server/internal/app/incoming_orders.go` (`IncomingOrdersService`: junta lo pendiente de cada origen ya ordenado, despacha `Accept`/`Reject` por `source` con `switch`)
- [X] T015 Tests en `server/internal/httpapi/handlers_incoming_test.go`: sin permiso 403 en las tres rutas y en `/orders/platform/pending` y `/platform/{id}/accept`; `source` desconocido y `id` no positivo 400; motivo fuera de lista 400; aceptar y rechazar publican `incoming.changed` (y los viejos `platform.order.decided`)
- [X] T016 Implementar `server/internal/httpapi/handlers_incoming.go` y las rutas en `server/internal/httpapi/router.go` con `RequirePermission(PermOrdersAcceptIncoming)` (nuevas y viejas de plataforma); el webhook en `handlers_webhook_plataforma.go` publica `incoming.changed` al registrar un nuevo y al cancelar
- [X] T017 `cd server && go build ./... && go test ./...` y `make test-integration` en verde

### Front común

- [X] T018 [P] Agregar `'orders.accept_incoming'` a `web/src/app/permissions.ts` y su etiqueta en la pantalla de roles (donde se nombran los permisos), con el test que ya cubre el catálogo
- [X] T019 [P] `web/src/api/pedidosEntrantes.ts`: tipos del contrato (`lines?`/`options?` opcionales), `pendientes()`, `aceptar(source,id)`, `rechazar(source,id,reason)`
- [X] T020 [P] Test `web/src/shared/entrantes/capas.test.ts` y `capas.ts`: hojas 1400 < toaster 1700 < bloqueo 2000 < franja 2100 (sustituye al test de `CAPA_DEL_AVISO`)

**Checkpoint**: backend completo y probado; front con contrato y capas.

---

## Phase 3: User Story 1 — Suena y se ve en cualquier pantalla (P1) 🎯 MVP

**Independent Test**: con un pendiente sembrado, la tableta en Almacén suena y muestra la franja; al aceptarlo, se calla.

- [X] T021 [P] [US1] Test `web/src/shared/entrantes/alarma.test.ts` con `AudioContext` falso: suena con ≥1 pendiente y una sola vez con varios; se calla con 0; `bloqueada` si el contexto está `suspended`; `pointerdown` llama `resume()`; el tono entra como función generadora del búfer y el test usa uno propio (FR-013)
- [X] T022 [US1] Implementar `web/src/shared/entrantes/alarma.ts` (búfer generado en bucle, desbloqueo global, estado observable)
- [X] T023 [P] [US1] Test `web/src/shared/entrantes/useEntrantes.test.ts`: consulta inicial; refetch con `incoming.changed`; sin permiso no consulta ni abre `/events`
- [X] T024 [US1] Implementar `web/src/shared/entrantes/useEntrantes.ts` (TanStack Query, `refetchInterval` 15 s, `EventSource` con `incoming.changed`)
- [X] T025 [P] [US1] Test `web/src/shared/entrantes/FranjaDeEntrantes.test.tsx` (M-Indicador): «N pedidos por aceptar» con el más urgente (origen, cliente, minutos); «Ver pedido» con 1 y «Ver pedidos» con varios lleva a `/pedidos`; capa `FRANJA` encima de una hoja abierta; con bloqueo por PIN solo dice cuántos (sin cliente); no se pinta en Pedidos mientras la columna está a la vista y sí en la pestaña «Cerrados»; sin pendientes no ocupa alto
- [X] T026 [US1] Implementar `web/src/shared/entrantes/FranjaDeEntrantes.tsx` y `AlertaDeEntrantes.tsx` (hook + alarma + franja) y montarla en `web/src/app/AppShell.tsx` como fila de la columna de trabajo, antes de la franja de agotados
- [X] T027 [US1] Quitar `AvisoDePlataforma` de `web/src/features/pos/POSPage.tsx`; borrar `AvisoDePlataforma`, `AvisoDePedidoEntrante`, `PedidosEntrantesSheet`, `usePedidosDePlataforma`, `api/pedidosDePlataforma.ts` y sus tests; mover `urgencia.ts` (+test) a `web/src/shared/entrantes/`; test en `POSPage.test.tsx` de que Vender no monta un segundo aviso

---

## Phase 4: User Story 2 — Aceptar o rechazar desde «Por aceptar» (P1)

**Independent Test**: dos pendientes; aceptar uno lo manda a cocina con comanda; rechazar el otro con «Se acabó un producto» lo deja `rechazado`.

- [X] T028 [P] [US2] Test `web/src/shared/entrantes/ColumnaPorAceptar.test.tsx` (M-PorAceptar): el más urgente abierto con hasta 3 renglones y «+N más»; los demás plegados en «N productos» (avisa si alguno no está en el menú) que abre con un toque; ícono de origen junto al nombre; «quedan N min» con plazo y «hace N min» sin plazo; teléfono, dirección y pago cuando vienen; Aceptar y Rechazar ≥ 44 px; sin pendientes no se pinta
- [X] T029 [P] [US2] Test `web/src/shared/entrantes/RechazarSheet.test.tsx`: 5 motivos en palabras de quien opera; «Rechazar pedido» deshabilitado sin motivo; manda el código propio
- [X] T030 [US2] Implementar `ColumnaPorAceptar.tsx`, `TarjetaEntrante.tsx` y `RechazarSheet.tsx` (sobre `ConfirmSheet`) en `web/src/shared/entrantes/`
- [X] T031 [US2] Montar la columna en `web/src/features/orders/OrdersBoardPage.tsx` a la izquierda del tablero, columnas sobre panel `bg.muted`; aceptar imprime la comanda solo en esta tableta (`KitchenTicket`) y avisa en palabras del operador si falla; test en `OrdersBoardPage.test.tsx` de que con pendientes el tablero conserva sus columnas

---

## Phase 5: User Story 3 — Sobrevive a recargas, red y suspensión (P1)

**Independent Test**: offline, se siembra; al volver la red suena en < 5 s.

- [X] T032 [P] [US3] Tests en `useEntrantes.test.ts`: error de red tras datos → los datos siguen y la alarma sigue; `alReconectar` → refetch; `visibilitychange` a visible → refetch y `resume()`; montar con pendientes en la primera respuesta → suena o queda `bloqueada`
- [X] T033 [US3] Implementar los tres caminos en `useEntrantes.ts` y `alarma.ts`

---

## Phase 6: User Story 4 — Varias tabletas (P1)

- [X] T034 [US4] Test en `ColumnaPorAceptar.test.tsx`/`AlertaDeEntrantes.test.tsx`: respuesta 409 al aceptar → toast «Otra tableta ya lo atendió», refetch y sin comanda; `incoming.changed` sin el pedido → se calla
- [X] T035 [US4] Implementar el manejo de 409 en la mutación de aceptar/rechazar

---

## Phase 7: User Story 5 — Sonido bloqueado (P2)

- [X] T036 [US5] Test en `FranjaDeEntrantes.test.tsx`: con alarma `bloqueada` y pendientes, botón «Toca para activar el sonido»; tras el toque, si sigue bloqueada el botón sigue; sin pendientes no hay botón
- [X] T037 [US5] Implementar el botón en la franja

---

## Phase 8: User Story 6 — Permiso (P2)

- [X] T038 [US6] Test en `AlertaDeEntrantes.test.tsx` y `OrdersBoardPage.test.tsx`: sin `orders.accept_incoming` no hay alarma, franja ni columna (la barrera real ya la cubren T013 y T015)
- [X] T039 [US6] Gatear con `can('orders.accept_incoming', user)` en `AlertaDeEntrantes` y en la columna

---

## Phase 9: User Story 7 — Ticket a la mano y «Cerrados» (P3)

- [X] T040 [P] [US7] Test en `OrdersBoardPage.test.tsx` (M-Pedidos): pestañas «En curso» (con número de pendientes por aceptar) y «Cerrados» en el renglón del título; «Cerrados» lista TODAS las entregadas de hoy con Ticket y Devolver; la impresora junto al total mide 44×44 y no se encima con el total; «Ver ticket» ya no está en el ⋮
- [X] T041 [US7] Implementar pestañas e impresora en `web/src/features/orders/OrdersBoardPage.tsx` (`Tabs` como en Caja; `ReprintTicket`)

---

## Phase 10: Polish y cierre

- [X] T042 [P] e2e `web/e2e/entrantes.spec.ts` contra el `make start` del worktree con `PLATFORM_ORDERS_SIMULATED=true`: suena (estado de la alarma expuesto como `data-` en la franja) en otra pantalla; aceptar desde la columna; rechazar con motivo; dos contextos de navegador: aceptar en uno apaga el otro; offline → online; cierra (entrega) todo pedido que acepte
- [X] T043 [P] Anotar en `docs/mecanica/plataformas.md` lo que no se ve: «pendiente» lo decide el servidor (plazo + cancelación), la simulación solo en desarrollo, y que las rutas `/orders/platform/*` y eventos `platform.order.*` se retiran en una subida posterior
- [X] T044 `/revision-de-codigo` sobre el diff y corregir con test cada hallazgo
- [X] T045 Prueba con alguien nuevo de una fonda a 1024×600 (y 1280×854) contra el `make start` del worktree, con quickstart.md; sin pedidos de prueba abiertos al terminar
- [X] T046 `make ci-local` en verde

---

## Dependencies & Execution Order

- Setup → Foundational (T003–T020) → historias.
- US1 (T021–T027) es el MVP y base de US3, US4, US5 y US6 (mismo hook y alarma).
- US2 depende de T019/T020; independiente de US1 salvo la franja que lleva a Pedidos.
- US7 solo toca `OrdersBoardPage` y puede ir en paralelo a US1.
- Polish al final.

### Parallel Opportunities

- T003, T005, T009 en paralelo (archivos distintos).
- T018, T019, T020 en paralelo.
- Dentro de US1: T021, T023, T025 (tests) en paralelo; luego sus implementaciones.
- US7 (T040–T041) en paralelo con US1–US2 si se coordina `OrdersBoardPage.tsx`.

## Implementation Strategy

MVP = Foundational + US1 + US2 (sonar, ver, aceptar y rechazar). Después US3–US6 (bordes y
permiso), US7 y el cierre. Commits pequeños por tarea o par test+código.
