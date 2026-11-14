# Implementation Plan: Notificaciones de inventario

**Branch**: `084-notificaciones-de-inventario` | **Date**: 2026-11-14 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/084-notificaciones-de-inventario/spec.md` (issue #291)

## Summary

Avisar a tiempo de lo que se acaba. Tres fuentes abren notificaciones en una tabla genérica por
empresa: el reporte manual «Se acabó» (ajuste a 0 sin cantidad, idempotente, deshacer 90 s), «No lo
encuentro» (texto sin tocar existencias) y la alerta por mínimo opt-in de cada artículo, que detecta un
trigger sobre `stock_movements` con una función SQL compartida (`evaluate_low_stock`). Go publica en
vivo con `LISTEN/NOTIFY` → `realtime.Broker`, la campana al pie del riel muestra lo no leído por
persona, una franja no bloqueante anuncia lo que se acabó, y un despachador manda un correo por bloque
de 5 min y un resumen de la mañana con la lista de compra, con plantillas MJML compiladas en el build.

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript + React 19 (web), bun para el build de correos
**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query; `mjml` 5.4.1 solo
como dependencia de desarrollo de `web/` (compila plantillas; nunca en el binario)
**Storage**: PostgreSQL (migración 0115, aditiva), Redis sin uso nuevo
**Testing**: `go test` (domain, table-driven), integración contra Postgres real bajo `appRoleStore`
con respaldo de dos empresas, vitest, Playwright a 1280×854 y 2560×1440
**Target Platform**: API Linux (VM e2-micro) + PWA en el equipo de mostrador
**Project Type**: web-service + web-app (monorepo `server/` + `web/`)
**Performance Goals**: el trigger agrega < 50 ms a guardar un pedido de 30 renglones (medido y
reportado); campana en vivo < 5 s; correo ≤ 5 min tras el aviso
**Constraints**: una falla del aviso nunca rompe venta, compra, conteo ni merma; subida con la API
anterior corriendo; correo < 102 KB; 1280×854 sin desborde
**Scale/Scope**: 2 empresas, ~400 insumos, cientos de movimientos por día; 7 pantallas o piezas de
pantalla nuevas o cambiadas

## Constitution Check

| Principio | Cómo se cumple | Estado |
|---|---|---|
| I. Layering | Ver «Capas» abajo: la política y el cálculo puro en `domain`, orquestación y transacciones en `app`, handlers finos, SQL por sqlc; el trigger solo detecta y escribe la fila | OK |
| II. Errores | Sentinels nuevos en `domain` (`ErrReportLimit`, `ErrUndoExpired`, `ErrUndoAfterMovement`, `ErrNotificationClosed`) mapeados en `httpapi.Error`; ctx propagado; el despachador y el listener terminan con el ctx del proceso | OK |
| III. Dinero | No mueve dinero. Cantidades de existencia con `Round4` y `ValidStockQtyOrZero`; el mínimo con `MaxStockQty` | OK |
| IV. TDD | Tests primero por cada FR; casos de borde enumerados abajo; integración bajo `appRoleStore` con `inTheThreeCases`; migración probada sobre respaldo real con dos empresas; prueba con dos conexiones; reloj falso | OK |
| IV. Tolera la versión anterior | Solo agrega columnas con default, tablas, funciones, trigger y un permiso; el trigger aplica también a los movimientos de la API vieja; nada se renombra ni se quita | OK |
| V. Seguridad | Permiso nuevo en backend (`RequirePermission`), golden de rutas; tope de 20 reportes por hora y persona; `query_text` y correos fuera de los logs; la pantalla de destinatarios no puede cambiar el correo de recuperación de nadie; parámetros inválidos (status, ids, hora) → 400 | OK |
| VI. YAGNI | Sin push ni WhatsApp; sin selector de sucursal; plantillas solo para el correo nuevo (los viejos migran después); una tabla de envíos propia en vez de generalizar la del cierre | OK |
| VII. Comentarios / idioma | Identificadores en inglés (`notifications`, nunca `alerts`); textos en español | OK |
| VIII. Puertas | Ver la sección siguiente | OK |
| Restricciones: 1280×854, 44 px, sin select nativo, un solo estilo | Maquetas elegidas con componentes reales; `Picker` para la hora; `ConfirmSheet`, hoja inferior y `Page` existentes | OK |
| Quien opera no decide en el momento | «Se acabó» no pregunta cantidad ni motivo; quién recibe y qué se avisa se configura antes | OK |

### Principio VIII: ¿esto se puede agregar después al mismo costo?

- **Inventario independiente por sucursal**: `stock_levels` ya es por sucursal. Las notificaciones,
  los reportes «Se acabó» y su ajuste guardan `branch_id` desde el primer renglón, y el mínimo admite
  sobrescritura por sucursal (`branch_stock_minimums`). Lo que **no** se construye: el selector de
  sucursal y la pantalla para capturar el mínimo de cada una (con una sola sucursal activa no se
  muestra). Agregarlos después cuesta una pantalla, no una migración: el hecho («en qué sucursal se
  acabó») ya queda registrado. **La puerta queda abierta.**
- **Más de un almacén por sucursal (#65)**: el aviso se guarda por sucursal, igual que la existencia
  hoy. Cuando exista el almacén, `stock_levels`, los movimientos, `notifications` y
  `branch_stock_minimums` reciben una columna `warehouse_id` y lo existente se asigna al principal;
  el índice único del aviso abierto se rehace con esa columna. No hay hecho irrecuperable: un aviso
  pasado de una sucursal con un solo almacén pertenece al principal sin adivinar. Lo que se evita a
  propósito: usar el «lugar» del insumo (`count_places`) como si fuera almacén. **La puerta queda
  abierta.**
- **Saber de quién es un pedido / estación**: cada reporte guarda `reported_by` y la bitácora el
  dispositivo (`X-Device-Key`); no se cierra nada.
- **Más de una caja vendiendo**: no aplica (nada asume una caja).
- Lo que sí se puede agregar después al mismo costo y por eso no se construye: avisos por push o
  WhatsApp (la tabla es genérica y el despachador se amplía), otros tipos de aviso, migrar los correos
  existentes a las plantillas.

### Capas

| Pieza | Capa |
|---|---|
| Tablas, índices, trigger `trg_stock_movement_notify`, función `evaluate_low_stock`, `pg_notify` | `server/migrations/0115_inventory_notifications.sql` |
| Consultas (`notifications.sql`, `stock_out_reports.sql`, `notification_recipients.sql`, `shopping_list.sql`) | `server/queries/` → sqlc |
| Reglas puras: efecto de un cambio de alertas (`LowStockEffect`), texto de la confirmación, bloque de 5 min, enfriamiento, hora de la mañana, destinatarios efectivos, cantidad sugerida y su porqué (`SuggestPurchase`), llaves de búsqueda (`SearchKeys`), tope de reportes, ventana de deshacer | `server/internal/domain/notifications*.go`, `shopping_list.go` |
| `StockOutService` (reportar, deshacer, no lo encuentro, búsqueda), `NotificationsService` (lista, leídas, resolver, configuración, destinatarios), `NotificationMailer` (bloques y mañana), `NotificationListener` (LISTEN → Broker) | `server/internal/app/` |
| Plantillas compiladas + render con `html/template` | `server/internal/mailtpl/` (nuevo) |
| Fuente MJML + script de compilación | `emails/` y `web/scripts/compilar-correos.ts` |
| Handlers y rutas | `server/internal/httpapi/handlers_notifications.go`, `handlers_stock_out.go`, `router.go` |
| Herramienta MCP `low_stock_update` | `server/internal/httpapi/mcp_tools_084.go` |
| Pantallas | `web/src/features/notificaciones/`, `web/src/features/se-acabo/`, `ArticleSheet.tsx`, `NewArticleSheet.tsx`, `ProductEditDialog.tsx`, `AppShell.tsx`, `nav.ts` |

### Casos de borde enumerados antes del código (IV)

| Familia | Caso | Test que lo atrapa |
|---|---|---|
| Valor vacío que significa algo | Mínimo vacío o 0 ⇒ sin mínimo, no «avisar en 0» | `TestLowStockEffect` (domain), `TestZeroMinNeverOpensLowStock` |
| | Campo de mínimo borrado a media captura no apaga la alerta | `ArticleSheet` vitest: vacío ≠ 0 en el payload |
| | `coverage_days` vacío ⇒ 14 | `TestSuggestPurchase` |
| Estado que no sobrevive | Recargar con un «Deshacer» visible: el servidor decide la ventana, la pantalla lo vuelve a leer de `/suggestions` (recent) | `TestUndoUsesTheDatabaseClock`, e2e |
| | La franja vista no regresa tras recargar (se guarda `banner_seen_at`) | `TestBannerSeenIsPerPerson` |
| | El resumen de la mañana no se repite tras reiniciar | `TestMorningSummaryOncePerDayAcrossRestarts` |
| Camino nuevo que se salta el control viejo | El cajero con `report_out` no ajusta existencias por la ruta genérica | `TestCashierCanReportOutButNotAdjust` |
| | «Es otro nombre de…» pasa por el candado y la regla de choque de otros nombres | `TestAliasFromNotFoundUsesTheAltNamesRules` |
| | La unión y el cambio de unidad no abren avisos con sus traspasos; la unión fusiona la casilla | `TestMergeOrsLowStockAndClosesSourceNotifications` |
| | Un movimiento redirigido a un destino unido abre (si toca) en el destino, nunca en el origen | `TestRedirectedMovementNotifiesOnTheTarget` |
| | El `PATCH` viejo de la ficha que cambia el mínimo también evalúa | `TestOldArticlePatchEvaluatesLowStock` |
| El hermano que no se movió | Todas las listas que ofrecen insumos filtran archivados: la búsqueda de «Se acabó» también | `TestArchivedIngredientLeavesCountPurchasesAndShortages` (caso nuevo) |
| | Cada camino que inserta movimientos lo cubre el trigger, no un llamado en Go | `TestEveryMovementPathEvaluates` (venta, compra, conteo, merma, ajuste, cancelación) |
| Concurrencia | Dos conexiones cruzan el mismo mínimo | `TestTwoConnectionsCrossingTheSameMinimumOpenOne` |
| | Dos «Se acabó» simultáneos del mismo artículo: un ajuste, una confirmación | `TestConcurrentStockOutReports` |
| Falla del aviso | El aviso revienta (tabla sin grant forzado en prueba) y la venta se guarda | `TestANotificationFailureNeverBreaksASale` |
| Correo | Abre y se resuelve antes del bloque: no sale | `TestResolvedBeforeTheWindowIsNotEmailed` |
| | Un bloque, un correo; bloque vacío, ninguno; enfriamiento 12 h | `TestOneEmailPerWindow`, `TestCooldownPerItem` |

## Pantallas y maquetas

Presupuesto 1280×854 con el riel de 76 px; 2560×1440 como ampliación. Componentes: `Page`, la hoja
inferior, `Picker`, `QtyWithUnit`, botones y tarjetas del tema.

Elegidas por el dueño el 2026-11-14 y confirmadas en su versión 2 (ajustes de la revisión de diseño) el mismo día, sobre capturas de la app real (arnés con los componentes del
POS, datos inventados) a 1280×854 y 2560×1440.

| Pantalla | Dónde vive | Cómo queda (opción elegida) |
|---|---|---|
| 1. Alertas en la ficha | Ficha del insumo (`ArticleSheet`) y de un producto con existencia propia | Renglón «Alertas: *estado en palabras* · Configurar ›» en lugar del campo «Mínimo»; abre una hoja inferior con la casilla, el mínimo (`QtyWithUnit`), los eventos, la franja y los días de cobertura, y su «Guardar», que abre la confirmación (`ConfirmSheet`) con el efecto inmediato. Al crear o al abrir una ficha que nunca preguntó: `ConfirmSheet` «¿Quieres recibir alertas de stock bajo de *X*?» (No, por ahora / Sí, configurar) |
| 2. Se acabó | Riel, debajo de «Se echó a perder» (`/se-acabo`, permiso `inventory.report_out`) | Dos columnas: buscador grande (64 px) y mosaicos de lo que más se acaba; el mosaico o renglón de resultado tocado se pone rojo («Toca otra vez: se acabó *X*», mismo alto) y el segundo toque confirma (≥ 0.4 s después del primero); se desarma solo a los 4 s, al teclear o al desplazar, y tocar otro mueve el armado. El aviso verde con «Deshacer · 90 s» vive en un hueco de alto fijo (76 px) sobre los mosaicos: nada se mueve bajo el dedo al reportar varios seguidos. «No lo encuentro · avisar *texto*» va anclado al pie de los resultados, separado 24 px, en tono neutro y con el mismo toque-y-confirma. Columna derecha de 340 px: «Reportado hoy». El segundo toque se queda aunque haya deshacer: deshacer no retira el aviso que las otras tabletas ya vieron |
| 3. Campana | Pie del riel, encima del tema: icono de 44 px sin rótulo (aria «Avisos, N sin leer»), con la misma insignia que «Se echó a perder» para no quitarle a la lista del riel más de un renglón | Panel «Avisos» de 440 px que sale junto al riel, pestañas «Abiertas · N» y «Cerradas», cada aviso con su acción («Ya llegó», «Dar de alta», «Es otro nombre de…», «Descartar», «Ver en la lista de compra ›») y el motivo de cierre |
| 4. Franja | Una sola capa de avisos (un portal, capa 2000, columna) que apila el aviso de pedido de plataforma primero y la franja después, sin encimarse aunque el aviso crezca | Tarjeta roja de 560 px arriba al centro con «Ver» y «×» y barra de 90 s, a 8 px del borde; no sale en la tableta de quien reportó; espera mientras haya una hoja de captura abierta |
| 5. Destinatarios | Almacén › pestaña «Avisos» (`inventory.configure`) | Tabla dentro de `Page`: una fila por persona (con su correo o «Sin correo: captúralo en Empleados») o correo extra (con «Quitar» como botón de 44 px), una columna por tipo de aviso con interruptores de área de 44 px; arriba la hora del resumen de la mañana con `Picker` en pasos de 15 min; «Agregar» correo extra abajo |
| 6. Lista de compra | Almacén › pestaña «Lista de compra» (`inventory.view`) | Dentro de `Page` (mismo ancho que las demás pestañas): tarjetas por lugar de compra (proveedor de la última compra; «Sin lugar de compra» al final), cada renglón con estado, hay/mínimo, qué comprar y su porqué |
| 7. Correo | Correo de bloque y de la mañana | Encabezado de marca con logo, tarjetas por aviso agrupadas por tipo con qué comprar y dónde, botón «Abrir en el sistema», pie con por qué te llega |

### Lo que cambió con la revisión de arquitectura (2026-11-14)

Del revisor de tableta se tomaron: hueco fijo del deshacer, reglas del armado, «No lo encuentro»
neutro y separado, una sola capa para franja y pedido, franja fuera de la tableta de quien reportó,
campana de icono, `Page` en Destinatarios y Lista, «Quitar» y casillas de 44 px, panel titulado
«Avisos». Se dejaron, por ser decisión del dueño en el issue: la confirmación al guardar siempre (no
solo cuando hay efecto), la pregunta al abrir por primera vez una ficha (con «No, por ahora» ya no
vuelve), y los textos «Enviar alerta de stock bajo», «Mostrar franja en pantalla» y el porqué de la
lista («se usan ~X al día, quedan Y, objetivo Z días»). Un test con admin a 1280×854 fija cuántas
opciones del riel se ven sin desplazar.

Del db-architect se tomaron todos los hallazgos; están en [data-model.md](data-model.md) (llaves,
índices, filtro sin subtransacción, Down, guardas de unión, cambio de unidad, snapshot) y en
[research.md](research.md) R12 (orden de candados) y R13 (la API anterior y el permiso nuevo).

## Maquetas

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| M1 Alertas en la ficha | [lienzo](https://claude.ai/artifact/1ZsK5kA6fNPqrrhpLA8eXP#m1) | B | La ficha se guarda sola; la hoja propia trae su Guardar y de ahí la confirmación, sin empujar nada fuera de la pantalla |
| M2 Se acabó / No lo encuentro | [lienzo](https://claude.ai/artifact/1ZsK5kA6fNPqrrhpLA8eXP#m2) | B | Dos toques y lo ya reportado a la vista; el deshacer cubre el error |
| M3 Campana y panel | [lienzo](https://claude.ai/artifact/1ZsK5kA6fNPqrrhpLA8eXP#m3) | A | No tapa la pantalla de trabajo y crece bien en 2K |
| M4 Franja | [lienzo](https://claude.ai/artifact/1ZsK5kA6fNPqrrhpLA8eXP#m4) | B | Patrón del aviso de pedido de plataforma; deja libre el buscador de Vender |
| M5 Destinatarios | [lienzo](https://claude.ai/artifact/1ZsK5kA6fNPqrrhpLA8eXP#m5) | A | Cada persona una vez; se lee de un vistazo quién recibe qué |
| M6 Lista de compra | [lienzo](https://claude.ai/artifact/1ZsK5kA6fNPqrrhpLA8eXP#m6) | B | La vuelta de compras sale ya armada por lugar |
| M7 Correo | [lienzo](https://claude.ai/artifact/1ZsK5kA6fNPqrrhpLA8eXP#m7) | B | Se lee en el teléfono; 23.7 KB contra el límite de 102 KB |

## Project Structure

### Documentation (this feature)

```text
specs/084-notificaciones-de-inventario/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
├── checklists/requirements.md
└── tasks.md            # /speckit-tasks
```

### Source Code

```text
server/
├── migrations/0115_inventory_notifications.sql
├── queries/{notifications,stock_out_reports,notification_recipients,shopping_list}.sql
├── internal/domain/{notifications.go,notifications_test.go,shopping_list.go,shopping_list_test.go,permission.go,audit.go}
├── internal/app/{stock_out.go,notifications.go,notification_mailer.go,notification_listener.go}
├── internal/mailtpl/{mailtpl.go,mailtpl_test.go,templates/*.html.tmpl,testdata/*.golden.html}
├── internal/httpapi/{handlers_stock_out.go,handlers_notifications.go,mcp_tools_084.go,router.go}
└── internal/integration/{notifications_test.go,notifications_concurrency_test.go,notification_mailer_test.go,migration_0115_test.go}
emails/{layout.mjml,stock-window.mjml,stock-morning.mjml,logo.png}
web/
├── scripts/compilar-correos.ts
├── src/features/notificaciones/{Campana.tsx,PanelDeNotificaciones.tsx,FranjaDeAgotado.tsx,Destinatarios.tsx,ListaDeCompra.tsx,api.ts}
├── src/features/se-acabo/{SeAcaboPage.tsx,api.ts}
├── src/features/backoffice/articles/{ArticleSheet.tsx,NewArticleSheet.tsx,SeccionAlertas.tsx}
├── src/app/{AppShell.tsx,nav.ts,routes.ts,permissions.ts}
└── e2e/notificaciones.spec.ts
```

**Structure Decision**: monorepo existente; una carpeta de pantallas por área nueva y un paquete
`mailtpl` en el servidor. `emails/` en la raíz porque no es código del front ni del servidor.

## Orden de entrega (una sola entrega, por dependencias del issue)

1. Migración, permiso, función compartida y trigger (con sus tests).
2. «Se acabó» y «No lo encuentro» (servicio, rutas, pantalla).
3. Detección conectada a configuración, conteo, unión y archivado; listener y despachador.
4. Campana, panel y franja.
5. Sección «Alertas», pregunta y confirmación; lote y MCP.
6. Plantillas y correo, destinatarios, resumen de la mañana, lista de compra.
7. e2e a 1280×854 y 2K.

## Complexity Tracking

| Decisión | Por qué | Alternativa más simple descartada |
|---|---|---|
| `LISTEN/NOTIFY` con una conexión dedicada | Lo que abre el trigger (una venta) tiene que llegar en vivo y solo si confirmó | Sondeo cada 60 s: la campana tardaría un minuto y la franja perdería sentido |
| Tabla `notification_email_sends` aparte | Agregar `kind` a `daily_summary_sends` cambiaría la llave única que la API anterior usa en `on conflict` | Generalizar la del cierre: rompe la subida |
| `mjml` en el build | Decisión del dueño (plantillas profesionales); cero dependencias en el servidor | Tablas a mano: queda como plan B si `bun audit` marca la dependencia |
