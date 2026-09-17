---
description: "Task list for feature implementation"
---

# Tasks: Recibir los pedidos de Uber Eats

**Input**: `specs/021-recibir-pedidos-uber/` — plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: **obligatorios y primero**. El principio IV de la constitución es no negociable: primero
el test que falla, luego el código. **Cada tarea de implementación tiene su tarea de test antes**, y
un test que nunca se vio en rojo no prueba nada.

**Arnés**: cada prueba de integración estrena su propia base clonada de una plantilla ya migrada.
Quien use `appRoleStore(t)` **tiene que llamar antes a `newTestStore(t)`**, y las conexiones propias
salen de `testURL(t)`.

**Corregido tras `/speckit-analyze` (2026-09-17)**: la captura de la llave de firma se movió de
Polish a fundacional —sin llave no se puede verificar ni una firma, así que el MVP no se podía
demostrar—, se arregló la ruta de la tarea de apertura de turno, se dijo de dónde salen
`client_uuid` y `daily_number`, y nueve tareas que juntaban implementación y test se partieron.

---

## Phase 1 · Setup

- [X] T001 Crear la migración vacía con `make migrate-new name=pedidos_de_plataforma` y confirmar que quedó como `server/migrations/0072_pedidos_de_plataforma.sql`

---

## Phase 2 · Foundational — bloquea todas las historias

**Nada de esto es de una historia en particular: sin esto no entra ni un aviso.**

### La migración, con su test antes

- [X] T002 Escribir `server/internal/integration/migracion_0072_test.go` con **dos empresas**, verificando: que los índices `users_tenant_key` y `platform_connections_tenant_key` existen, que las cuatro tablas nuevas tienen RLS activo y sus grants para `gatobobah_app`, y que el `check` de `orders` ya admite `para_llevar` con plataforma. **Verlo en rojo**
- [X] T003 Escribir en `server/migrations/0072_pedidos_de_plataforma.sql` la sección 0: `create unique index users_tenant_key on users (company_id, id)` y `platform_connections_tenant_key on platform_connections (company_id, id)`. Sin esto, toda FK compuesta de esta migración falla al aplicarse
- [X] T004 Agregar en `server/migrations/0072_pedidos_de_plataforma.sql` la tabla `platform_webhook_keys` con su unique, sus dos `check`, su FK compuesta con `on delete no action` explícito, RLS y grants
- [X] T005 Agregar en `server/migrations/0072_pedidos_de_plataforma.sql` la tabla `platform_webhook_events` con `raw_body text not null`, `unique (event_id)` **global**, los `check` de lista cerrada sobre `outcome` y `failure_kind`, FK compuesta a `platform_connections`, RLS y grants
- [X] T006 Agregar en `server/migrations/0072_pedidos_de_plataforma.sql` el enum de estado y la tabla `platform_incoming_orders` con `raw_detail`, `settled_at` separado de `decided_by`, las tres columnas nullable para la cancelación que llega primero y su `check`, las FK compuestas a `users`/`orders`/`platform_connections`, los dos índices y los grants. **La matriz estado→columnas va en el comentario**, no solo en Go
- [X] T007 Agregar en `server/migrations/0072_pedidos_de_plataforma.sql` la tabla `platform_incoming_order_lines` con `quantity numeric(8,2)` (misma precisión que su destino), `product_id` nullable con FK compuesta `on delete restrict`, RLS y grants
- [X] T008 Agregar en `server/migrations/0072_pedidos_de_plataforma.sql` el `set local lock_timeout = '3s'` y la relajación del `check` de `orders` para admitir `para_llevar` con plataforma
- [X] T009 Escribir el `Down` de `server/migrations/0072_pedidos_de_plataforma.sql`: **se detiene con un mensaje claro y sin tocar nada** si ya existe un pedido `para_llevar` con plataforma, como hace el de la 0061
- [X] T010 Correr `server/internal/integration/migracion_0072_test.go` en verde y `make api-test` completo

### La llave de firma: capturarla ANTES de poder verificar nada

**Movido aquí por `/speckit-analyze`.** Sin una llave en la base no se puede verificar una sola
firma, así que ni la fase fundacional ni US1 se pueden probar. Estaba en Polish y bloqueaba el MVP.

- [X] T011 Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de alta y rotación de llave: se guarda, se puede poner una secundaria, dos iguales se rechazan, una corta se rechaza. **Verlo en rojo**
- [X] T012 Escribir las consultas de alta, lectura y rotación de llave en `server/queries/pedidos_de_plataforma.sql` y correr `make sqlc`
- [ ] T013 Escribir el servicio de llaves en `server/internal/app/pedidos_de_plataforma.go` y su handler en `server/internal/httpapi/handlers_menus_plataforma.go`, con su ruta en `server/internal/httpapi/router.go`
- [ ] T014 Escribir en `web/src/features/admin/PlataformasPage.test.tsx` el test de la captura de llave: se guarda, y la llave **nunca se vuelve a mostrar** una vez guardada. **Verlo en rojo**
- [ ] T015 Escribir la captura de la llave en `web/src/features/admin/PlataformasPage.tsx`, con su texto para quien opera —dónde la saca del tablero de la aplicación—, no para quien programó

### La firma y la clasificación del aviso — lógica pura

- [X] T016 [P] Escribir `server/internal/domain/aviso_de_plataforma_test.go`: firma correcta contra la primaria, correcta contra la **secundaria**, incorrecta, ausente, con mayúsculas, con longitud distinta. **Verlo en rojo**
- [X] T017 Escribir `server/internal/domain/aviso_de_plataforma.go` con `VerificarFirma` usando `hmac.Equal` (**tiempo constante**, nunca `==`) sobre el cuerpo crudo, probando primaria y luego secundaria
- [X] T018 [P] Escribir en `server/internal/domain/aviso_de_plataforma_test.go` el test de `ClasificarEvento`: los seis tipos que manejamos, uno desconocido, uno con `event_type` vacío. **Verlo en rojo**
- [X] T019 Escribir en `server/internal/domain/aviso_de_plataforma.go` `ClasificarEvento` y los sentinels `ErrFirmaInvalida`, `ErrAvisoRepetido`, `ErrPedidoYaDecidido`, `ErrTiendaDesconocida`
- [X] T020 [P] Escribir en `server/internal/domain/aviso_de_plataforma_test.go` el test de la whitelist de motivos de rechazo: cada código admitido, uno inventado se rechaza y **no cae a `OTHER`**. **Verlo en rojo**
- [X] T021 Escribir la whitelist de motivos en `server/internal/domain/aviso_de_plataforma.go`

### Resolver la tienda sin saber la empresa

- [X] T022 Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test `TestUnAvisoNoCaeEnLaEmpresaEquivocada`: **dos empresas**, la misma tienda registrada en las dos, aviso firmado con la llave de A. Debe caer en A y **no dejar rastro en B**. **Verlo en rojo**
- [X] T023 Escribir en `server/queries/pedidos_de_plataforma.sql` la consulta que traduce `(plataforma, external_store_id)` → `(company_id, connection_id)` devolviendo **solo esos dos ids**, y correr `make sqlc`
- [X] T024 Escribir `resolverTienda` en `server/internal/app/pedidos_de_plataforma.go`: corre por el pool **sin tenant** —la empresa es el resultado, no la entrada— y cuando hay más de una candidata **verifica la firma contra cada una**. El comentario dice por qué es la única excepción a `store.QC(ctx)` y qué la acota
- [X] T025 Escribir `TestSoloUnaConsultaCorrePorFueraDelTenant` en `server/internal/app/pedidos_de_plataforma_test.go`: parsea el paquete `app` y falla si aparece un segundo uso de `store.Q` fuera de `resolverTienda`. Un comentario se olvida; un test que falla, no

### La puerta pública

- [X] T026 Escribir `server/internal/httpapi/handlers_webhook_plataforma_test.go`: sin firma → 401, firma incorrecta → **401 con el mismo cuerpo**, cuerpo mayor al tope → 413, `X-Environment` equivocado → 400, tipo desconocido → **200** (para que Uber no reintente para siempre). **Verlo en rojo**
- [X] T027 Escribir `server/internal/httpapi/handlers_webhook_plataforma.go`: lee el cuerpo con `http.MaxBytesReader`, verifica ambiente, resuelve, verifica firma, y responde. Nunca dice qué falló ni si la tienda existe
- [X] T028 Montar `POST /api/v1/webhooks/uber-eats` en `server/internal/httpapi/router.go`, **fuera de `RequireAuth` y de `WithTenant`**, con su limitador de frecuencia. El comentario dice por qué está fuera de los dos grupos
- [X] T029 Escribir en `server/internal/httpapi/handlers_webhook_plataforma_test.go` el test de que el evento de seguridad no filtra: ni cuerpo, ni llave, ni datos del cliente. **Verlo en rojo**
- [X] T030 Escribir en `server/internal/httpapi/handlers_webhook_plataforma.go` el evento con `logging.SecurityEvent` y clave estable (`webhook_firma_invalida`, `webhook_tienda_desconocida`)
- [X] T031 Escribir en `server/internal/httpapi/handlers_webhook_plataforma_test.go` el test del limitador: el tope no deja fuera un volumen realista de pedidos, y con Redis caído **no bloquea** (fail-open, como el resto del repo)

### El cliente de Uber

- [X] T032 Escribir `server/internal/uber/sin_escrituras_test.go` actualizado: el transporte sigue rechazando todo verbo distinto de GET **salvo las dos rutas de la lista blanca**, y el `PUT` de menú y el `DELETE` de `pos_data` siguen siendo imposibles. **Verlo en rojo**
- [X] T033 Convertir el guard de `server/internal/uber/solo_lectura.go` en lista blanca de rutas para `accept_pos_order` y `deny_pos_order`
- [X] T034 [P] Escribir en `server/internal/uber/pedidos_test.go` el test de `TraerDetalleDePedido`: sigue el `resource_href` **del aviso** y no arma la ruta a mano, con tope de bytes y su presupuesto de tiempo. **Verlo en rojo**
- [X] T035 Escribir `TraerDetalleDePedido`, `AceptarPedido` y `RechazarPedido` en `server/internal/uber/pedidos.go`, copiando campo por campo para que lo que Uber agregue mañana no llegue solo a la pantalla, con el tope de bytes y el presupuesto de tiempo como **constantes en ese archivo** — no variables de entorno: son valores que no cambian (principio VI)

---

## Phase 3 · US1 — El pedido llega solo y se acepta de un toque (P1)

**Objetivo**: un aviso firmado se convierte en un pedido visible, y un toque lo mete al POS impreso.

**Prueba independiente**: disparar un evento firmado contra el ambiente de pruebas y ver el pedido
en la tableta en menos de 10 s; aceptarlo y ver el ticket.

### Backend — recibir y registrar

- [X] T036 [US1] Escribir el test de `MapearRenglones` en `server/internal/domain/pedido_entrante_test.go`: renglón con pareja, sin pareja, con opciones anidadas, cantidad fraccionaria, precio en centavos enteros. **Verlo en rojo**
- [X] T037 [US1] Escribir `server/internal/domain/pedido_entrante.go` con `MapearRenglones`, usando `Round2` en la frontera y tomando el precio **de la plataforma**
- [X] T038 [US1] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test `TestUnAvisoFirmadoDejaUnPedidoPendiente`: llega el aviso, se guarda `raw_body`, se trae el detalle, se guarda `raw_detail`, y quedan el pedido y sus renglones. **Verlo en rojo**
- [X] T039 [US1] Escribir las consultas de alta de aviso, pedido entrante y renglones en `server/queries/pedidos_de_plataforma.sql` y correr `make sqlc`
- [X] T040 [US1] Escribir `RecibirAviso` en `server/internal/app/pedidos_de_plataforma.go`: guarda el aviso crudo **en su propia transacción antes** de llamar a Uber, trae el detalle con presupuesto, registra, y **no confirma si algo falló**
- [X] T041 [US1] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test `TestUnDetalleQueNoSeTraeNoSeConfirma`: 5xx, no 200, con la clase de fallo registrada y ningún pedido. **Verlo en rojo**
- [ ] T042 [US1] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de que un pedido recibido publica `platform.order.received` por el broker. **Verlo en rojo**
- [ ] T043 [US1] Publicar `platform.order.received` por `realtime.Broker` desde `server/internal/httpapi/handlers_webhook_plataforma.go`

### Backend — mostrar y aceptar

- [X] T044 [US1] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de `GET /orders/platform/pending` **sobre el JSON crudo**, verificando que `lines` y `options` salen como `[]` y nunca `null` — deserializar a una estructura de Go borra justo esa diferencia, y eso ya tumbó la pantalla de pedidos de producción una vez. **Verlo en rojo**
- [X] T045 [US1] Escribir el handler `GET /orders/platform/pending` en `server/internal/httpapi/handlers_orders.go` y su ruta en `server/internal/httpapi/router.go`
- [X] T046 [US1] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de aceptar: crea la fila en `orders` con `opened_by` = quien aceptó, **ya pagada por la plataforma**, sin aparecer como deuda por cobrar, y el total **cuadra al centavo**. **Verlo en rojo**
- [X] T047 [US1] Escribir `AceptarPedido` en `server/internal/app/pedidos_de_plataforma.go`. Tres columnas `not null` de `orders` que hay que llenar y que el plan no nombraba: **`client_uuid`** se genera nuevo (`uuid` de la stdlib), **`daily_number`** sale de `order_counters`, que es por `business_date` y por lo tanto **no necesita turno abierto**, y **`opened_by`** es quien aceptó. `platform_order_ref` guarda el folio de Uber y **no sustituye** a `daily_number`: son columnas distintas. Si Uber rechaza la aceptación, **no queda aceptado de nuestro lado**
- [X] T048 [US1] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test `TestAceptarSinTurnoDeCajaFunciona`: `register_session_id` queda NULL, el pedido existe y tiene su `daily_number`. **Verlo en rojo**
- [X] T049 [US1] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de que abrir turno reclama los pedidos huérfanos **de ese `business_date`** y no los de días anteriores. **Verlo en rojo**
- [X] T050 [US1] Escribir el `update` que reclama los pedidos huérfanos dentro de la transacción de `OpenSession` en `server/internal/app/backoffice.go`, con su consulta en `server/queries/pedidos_de_plataforma.sql`
- [X] T051 [US1] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test del 409 al aceptar dos veces. **Verlo en rojo**
- [X] T052 [US1] Escribir el handler `POST /orders/platform/{id}/accept` en `server/internal/httpapi/handlers_orders.go` con su ruta en `server/internal/httpapi/router.go`

### Pantalla

- [ ] T053 [US1] Escribir `web/src/api/pedidosDePlataforma.ts` con sus tipos, declarando `lines`/`options` **opcionales** para que el compilador obligue a la guarda
- [ ] T054 [US1] Escribir en `web/src/features/pos/AvisoDePedidoEntrante.test.tsx` el test de que **el aviso se ve con una hoja abierta encima**. Es el escenario que más importa y el que se pierde si se pinta como parte normal de la pantalla. **Verlo en rojo**
- [ ] T055 [US1] Escribir `web/src/features/pos/AvisoDePedidoEntrante.tsx` en su **propio overlay**, por encima de cualquier hoja del POS, como franja horizontal **fuera** de la barra superior — esa fila ya se desborda ~55 px en 1024×600
- [ ] T056 [US1] Escribir en `web/src/features/pos/AvisoDePedidoEntrante.test.tsx` el test del reloj: se pintan **minutos y color**, nunca segundos, con umbral a los 90 s y otro cerca del final. **Verlo en rojo**
- [ ] T057 [US1] Escribir la cuenta regresiva con sus tres estados en `web/src/features/pos/AvisoDePedidoEntrante.tsx`
- [ ] T058 [US1] Escribir en `web/src/features/pos/AvisoDePedidoEntrante.test.tsx` el test de que Aceptar cuesta **un toque** y de que Aceptar y Rechazar **no son dos botones iguales**: Aceptar dominante, Rechazar chico y separado. **Verlo en rojo**
- [ ] T059 [US1] Escribir los botones con su separación en `web/src/features/pos/AvisoDePedidoEntrante.tsx`, copiando el criterio de `web/src/features/pos/PedidosEnCurso.tsx`
- [ ] T060 [US1] Escribir en `web/src/features/pos/AvisoDePedidoEntrante.test.tsx` el test de tres pendientes a la vez: se ve el más urgente con sus acciones y un contador con los demás. **Verlo en rojo**
- [ ] T061 [US1] Escribir `web/src/features/pos/PedidosEntrantesSheet.tsx` con el contador «+N», copiando el patrón de `web/src/features/pos/PedidosEnCurso.tsx`
- [ ] T062 [US1] Cablear el aviso en `web/src/features/pos/POSPage.tsx` y suscribirlo a `platform.order.received`
- [ ] T063 [US1] Escribir en `web/src/features/pos/AvisoDePedidoEntrante.test.tsx` el test de que aceptar dispara la impresión del ticket de cocina respetando `PrintSettings`. **Verlo en rojo**
- [ ] T064 [US1] Escribir la impresión al aceptar en `web/src/features/pos/AvisoDePedidoEntrante.tsx`

---

## Phase 4 · US2 — Rechazar un pedido que no se puede preparar (P1)

- [ ] T065 [US2] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de rechazar: queda rechazado con su motivo, con quién y cuándo, **no cuenta como venta**, y un código inventado da 422 sin caer a «otro». **Verlo en rojo**
- [ ] T066 [US2] Escribir `RechazarPedido` en `server/internal/app/pedidos_de_plataforma.go` y el handler `POST /orders/platform/{id}/deny` en `server/internal/httpapi/handlers_orders.go`
- [ ] T067 [US2] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de que un pedido ya aceptado no se puede rechazar, y que lo dice en vez de fallar callado
- [ ] T068 [US2] Escribir en `web/src/features/pos/motivosDeRechazo.test.ts` el test de que cada código tiene su frase en español y **el código nunca se pinta**. **Verlo en rojo**
- [ ] T069 [US2] Escribir `web/src/features/pos/motivosDeRechazo.ts` — el único lugar fuera de `server/internal/domain/` donde ese código existe
- [ ] T070 [US2] Escribir en `web/src/features/pos/AvisoDePedidoEntrante.test.tsx` el test de que el motivo se elige con `Picker` y **nunca con un desplegable del sistema**. **Verlo en rojo**
- [ ] T071 [US2] Escribir el selector de motivo con `Picker` (`web/src/components/Picker.tsx`) en `web/src/features/pos/AvisoDePedidoEntrante.tsx`

---

## Phase 5 · US3 — Un pedido no se pierde aunque algo falle (P1)

**Es P1 porque el fallo es silencioso**: nada truena, simplemente hay un pedido de más o de menos.

- [X] T072 [US3] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test `TestElMismoAvisoCincoVecesEsUnPedido`: cinco entregas idénticas, 200 las cinco, **un** pedido. **Verlo en rojo**
- [X] T073 [US3] Escribir la deduplicación por `event_id` en `server/internal/app/pedidos_de_plataforma.go`, confirmando la recepción igual
- [ ] T074 [US3] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test `TestUnaCancelacionQueLlegaPrimeroDejaElPedidoCancelado`: el `orders.cancel` antes que su `orders.notification`, sin detalle que traer. **Verlo en rojo**
- [ ] T075 [US3] Escribir en `server/internal/app/pedidos_de_plataforma.go` el camino de cancelación que nace sin notificación previa, con las tres columnas nullable
- [ ] T076 [US3] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de que una cancelación **sobre un pedido ya aceptado** cancela el pedido del POS sin borrar nada. **Verlo en rojo**
- [ ] T077 [US3] Escribir ese camino en `server/internal/app/pedidos_de_plataforma.go`
- [ ] T078 [US3] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de `expirado`: el sistema lo marca al pasar `decide_before` y **no llama a la plataforma** — quien cancela es Uber. **Verlo en rojo**
- [ ] T079 [US3] Escribir el camino de `expirado` en `server/internal/app/pedidos_de_plataforma.go`

---

## Phase 6 · US4 — Saber qué pasó sin abrir la consola (P2)

- [ ] T080 [US4] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de `GET /admin/platform-orders` con su `Count` gemela **usando el mismo `where`**, la whitelist de orden en el dominio y el rechazo de un `sort` desconocido. **Verlo en rojo**
- [ ] T081 [US4] Escribir la consulta paginada y su gemela en `server/queries/pedidos_de_plataforma.sql`, con `sqlc.narg` para los filtros opcionales y **nunca SQL concatenado**
- [ ] T082 [US4] Escribir el handler `GET /admin/platform-orders` en `server/internal/httpapi/handlers_backoffice.go` y su ruta en `server/internal/httpapi/router.go`
- [ ] T083 [US4] Escribir en `web/src/features/admin/PedidosDePlataformaPage.test.tsx` el test de la lista y su ordenamiento. **Verlo en rojo**
- [ ] T084 [US4] Escribir `web/src/features/admin/PedidosDePlataformaPage.tsx` con `SortHead`, copiando el patrón de listas que ya existe
- [ ] T085 [US4] Instrumentar la pantalla en los **TRES** lugares: `pantallasMedibles` y `rolesPorPantalla` en `server/internal/domain/uso.go`, más `PANTALLAS` en `web/src/app/rutas-medidas.ts`. Falta el mapa de roles y los eventos se descartan todos, en silencio
- [ ] T086 [US4] Rutear la pantalla en `web/src/app/App.tsx`, `web/src/app/AppShell.tsx` y `web/src/app/roles.ts` — no solo en `rutas-medidas.ts`

---

## Phase 7 · US5 — Enterarse de que la plataforma cerró la tienda (P2)

- [ ] T087 [US5] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de que **consultar el estado de la tienda** detecta que la plataforma la cerró y lo registra con su hora. **Verlo en rojo**. El aviso `store.status.changed` NO es el camino: exige un permiso que el dueño decidió no pedir todavía, y construir sobre un aviso que nunca llega deja el requisito sin cumplir **sin que nada falle**
- [ ] T088 [US5] Escribir la consulta de estado en `server/internal/uber/pedidos.go` y su manejo en `server/internal/app/pedidos_de_plataforma.go`, de modo que el día que exista el permiso **manejar el aviso sea agregar un caso**, no rehacer el camino
- [ ] T089 [US5] Escribir en `web/src/features/pos/POSPage.test.tsx` el test de que el aviso de tienda cerrada no se puede pasar por alto y desaparece al reabrir. **Verlo en rojo**
- [ ] T090 [US5] Escribir el aviso de tienda cerrada en `web/src/features/pos/POSPage.tsx`

---

## Phase 8 · US6 — Conectaron o desconectaron la tienda (P3)

- [ ] T091 [US6] Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de `store.provisioned` y `store.deprovisioned`: la conexión queda activa o inactiva. **Verlo en rojo**
- [ ] T092 [US6] Escribir el manejo de los dos eventos en `server/internal/app/pedidos_de_plataforma.go` y reflejarlo en `web/src/features/admin/PlataformasPage.tsx`

---

## Phase 9 · Polish

- [ ] T093 Escribir en `web/src/features/backoffice/CashPage.test.tsx` el test de la señal de FR-023 al abrir turno. **Verlo en rojo**
- [ ] T094 Escribir esa señal en `web/src/features/backoffice/CashPage.tsx`: **UNA línea** con conteo y monto, nunca una tabla — esa pantalla ya mide 1,494 px en 1024×600
- [ ] T095 Escribir en `server/internal/integration/pedidos_de_plataforma_test.go` el test de la poda: a los **60 días** se borra el cuerpo y **la fila se queda**. **Verlo en rojo**
- [ ] T096 Escribir la poda de `raw_body` y `raw_detail` en `server/internal/app/pedidos_de_plataforma.go`
- [ ] T097 [P] Documentar en `AGENTS.md` las trampas de esta feature: la consulta que no usa `QC` y por qué, que el aviso no trae el pedido, que el detalle crudo es lo único desde lo cual se corrige un mapeo, que `platform_order_ref` no sustituye a `daily_number`, y que instrumentar la pantalla nueva son tres lugares
- [ ] T098 [P] Agregar la llave de firma al respaldo de variables y secretos **fuera del repositorio** (`~/.gatobobah-backups/`, modo 600) — el repositorio es público
- [ ] T099 Correr `specs/021-recibir-pedidos-uber/quickstart.md` completo contra el ambiente de pruebas, incluido el caso de dos empresas con el mismo id de tienda
- [ ] T100 **Cerrar los pedidos de prueba que la validación haya creado**: entregar (`POST /orders/:id/deliver`) y cobrar (`POST /orders/:id/pay`, **no** `/charge`), como hace `web/e2e/limpiar-lo-que-cree.ts`
- [ ] T101 Correr todos los gates desde la raíz del repositorio: `go build`, `go test`, la suite de integración, `bun run lint`, `vitest` y `bun run build`

---

## Dependencias

```
Phase 1 (setup)
   └─→ Phase 2 (foundational)  ← BLOQUEA TODO
          ├─→ Phase 3 · US1  ← el MVP
          │      ├─→ Phase 4 · US2   (necesita el pedido pendiente de US1)
          │      └─→ Phase 5 · US3   (necesita el camino de recepción de US1)
          ├─→ Phase 6 · US4  (su pantalla se puede construir con datos sembrados)
          ├─→ Phase 7 · US5  (independiente: otro tipo de evento)
          └─→ Phase 8 · US6  (independiente: otro tipo de evento)
                 └─→ Phase 9 · Polish
```

**Dentro de la Phase 2 el orden importa más de lo que parece**: la migración (T002-T010) va primero
porque la captura de llave (T011-T015) escribe en una tabla que ella crea, y sin llave capturada no
se puede verificar una firma — o sea, el resolutor (T022-T025) y la puerta pública (T026-T031) no se
pueden probar. La lógica pura (T016-T021) y el cliente de Uber (T032-T035) sí son independientes.

## Paralelismo

| Se pueden hacer a la vez | Por qué |
|---|---|
| T016, T018, T020 | Tres pruebas distintas del mismo paquete puro, sin dependencia entre ellas |
| T034 y la rama de la migración | `internal/uber/` no toca el esquema |
| T097, T098 | Documentación y respaldo: nada en común |

**Lo que NO se paraleliza**: todo lo que toca `server/migrations/0072_pedidos_de_plataforma.sql`
(T003-T009) va en orden, y todo lo que toca `server/queries/pedidos_de_plataforma.sql` va en orden —
mismo archivo.

## Alcance del MVP

**Phase 1 + Phase 2 + Phase 3 (US1)**: un pedido de Uber entra solo y se acepta de un toque. Es la
feature; lo demás es lo que pasa cuando algo sale mal.

Aun así, **US3 no es opcional para producción**. Su fallo es silencioso —un pedido duplicado hace
que la cocina prepare dos veces— y no se descubre hasta que un cliente reclama. Se puede demostrar
sin ella; no se puede operar sin ella.

## Lo que NO se puede verificar todavía, y hay que decirlo al reportar

- **Que `accept_pos_order` funcione contra Uber**: exige ser la app gestora de pedidos de la tienda,
  permiso que hoy no tenemos y que está en el ticket abierto.
- **La forma real del detalle del pedido**: los avisos de prueba son nuestros, con la forma
  documentada. Para eso existe `raw_detail`.
- **La robollamada a los 90 s y el corte a los 11.5 minutos**: los dispara Uber.
- **SC-001 (menos de 10 segundos)** solo se mide a mano en T099. Ninguna prueba automática lo vigila.
