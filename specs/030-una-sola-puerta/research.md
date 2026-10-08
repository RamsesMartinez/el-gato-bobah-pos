# Research: Una sola puerta para cobrar

Cada decisión dice qué se eligió, por qué y qué se descartó. Las decisiones D-1…D-12 del
[spec](./spec.md) no se reabren aquí: esto es el **cómo**.

## R-1. La cuenta en captura es una tabla propia, con sus renglones y una bitácora de agregados

- **Decisión**: tres tablas nuevas — `order_drafts` (la cuenta), `order_draft_lines` (sus renglones)
  y `order_draft_adds` (la llave de idempotencia de cada «agregar»). Detalle en
  [data-model.md](./data-model.md).
- **Por qué**: D-1 pide que el borrador nunca sea un pedido. Con tabla propia, las ~30 consultas de
  dinero que filtran `orders` por exclusión de estado no pueden contarlo **por construcción**; un
  estado nuevo en el enum `order_status` obligaría a revisar cada una (y a recordarlo en la consulta
  número 31).
- **Descartado**:
  - *Estado `capturando` en `order_status`*: rompe FR-002 en silencio en cualquier consulta que filtre
    por exclusión (`status not in ('cancelada','reembolsada')`), que son la mayoría.
  - *Renglones como `jsonb` dentro de `order_drafts`*: cada «+1» reescribiría el documento entero y
    dos tabletas agregando a la vez se pisarían (D-5 exige que los agregados se sumen). Con filas, el
    agregado es un `insert` o un `qty = qty + n` y no pisa nada.

## R-2. Idempotencia y concurrencia: agregar se suma, cambiar se versiona

D-5: «lo que agrega cada una se suma; cambiar o quitar algo que la otra ya cambió se rechaza».

| Operación | Llave | Conflicto |
|---|---|---|
| Crear la cuenta | `order_drafts.id` (uuid que pone la tableta) | Reintento con el mismo id → devuelve la misma cuenta |
| Agregar (producto nuevo o «+» sobre un renglón) | `opId` (uuid por toque) en `order_draft_adds` | Nunca: sumar es conmutativo. Reintento con el mismo `opId` → no-op |
| Cambiar cantidad, modificadores, nota; quitar renglón | `order_draft_lines.version` esperada | Versión distinta → `409 DRAFT_CHANGED`, nada se aplica |
| Editar cabecera (cliente, canal, servicio, envío, descuento, folio de plataforma) | `order_drafts.header_version` esperada | Igual que arriba |
| Descartar | estado | Ya descartada → 204; ya enviada → `409` |
| Enviar | `order_drafts.id` = `orders.client_uuid` (sin pedido) o `order_line_batches.client_uuid` (con pedido) | Reintento → devuelve el pedido ya creado |

- **Toda escritura de renglones toma `for update` sobre la cuenta primero**: serializa por cuenta,
  así la fusión («no había renglón igual») y `position` no se pisan entre tabletas.
- **El «+» de un renglón es un agregado, no un cambio de cantidad**: `POST …/lines` con
  `intoLineId`. Dos tabletas que tocan «+» sobre el mismo café suman 2. El «−» sí es cambio de
  cantidad con versión: restar lo que la otra tableta acaba de quitar es justo lo que D-5 rechaza.
- **Fusión de renglones iguales**: el servidor fusiona un agregado sin modificadores ni nota con el
  renglón vivo del mismo producto sin modificadores ni nota (la regla que hoy aplica `addLine` en
  `stores/ticket.ts`). La regla es pura y vive en `domain.MergeTarget`.
- **Un reintento de «cambiar» que sí se aplicó** vuelve con `409` (la versión ya avanzó). El front
  compara contra la cuenta recargada: si el renglón ya quedó como lo pidió, no avisa.
- **Descartado**: versión única por cuenta (cualquier agregado de la otra tableta invalidaría un «−»
  local: tasa de conflictos altísima en hora pico); última escritura gana (contradice D-5).

## R-3. El nombre se amarra al nacer y se suelta al descartar

- **Al crear**: el servicio elige el nombre con el MISMO predicado de hoy
  (`domain.DisponiblesDeLaBolsa`), pero con **los nombres de las cuentas en captura vivas** sumados a
  los `usados` del turno. Acepta el propuesto por la pantalla si está disponible; si no, sortea. Lo
  marca en `folio_consumido` y guarda en la cuenta el **esquema** con que lo sacó.
- **Choque entre dos tabletas a la vez**: índice único parcial
  `(company_id, folio_name) where status = 'capturando' and folio_name is not null`. La segunda recibe
  `23505`; el servicio reintenta la transacción entera hasta 3 veces excluyendo el nombre perdido.
- **Al enviar**: `resolverFolio` recibe el nombre amarrado (`CreateOrderCmd.BoundFolioName`) y lo
  acepta **aunque esté en `folio_consumido`** (lo consumió esta misma cuenta) mientras no esté usado
  en el turno; lo vuelve a marcar (`on conflict do nothing`). Si en un caso patológico ya está usado
  en el turno, cae a `SiguienteFolioLibre` («Persa 2»): nunca cambia de animal.
- **Todos los que reparten nombre excluyen las cuentas vivas**: `resolverFolio` (lo usan `Create` y
  `move_lines`) y `NombresDisponibles`. Sin eso, un pedido creado por otro camino se llevaría el
  nombre de una cuenta que ya se le dijo al cliente. Los nombres vivos se leen **dentro de la misma
  tx** que inserta el pedido (no antes), para que la ventana con una cuenta que nace a la vez sea de
  milisegundos; si aun así choca, el pedido nuevo cae al siguiente libre, nunca la cuenta.
- **Cuenta que cruza turnos** (vive hasta 12 h; el cierre no la bloquea): al enviarse en el turno
  nuevo puede chocar con un nombre ya usado ahí y cae a «Persa 2». Comportamiento documentado, con
  test. `VaciarBolsaDeFolios` borra también las filas de las cuentas vivas: es inofensivo porque la
  exclusión por vivos no depende de `folio_consumido`; tiene su test («vaciar con cuentas vivas»).
- **Al descartar** (a mano o por 12 h): `delete from folio_consumido where scheme = $esquema and
  name = $nombre and taken_at <= $draft.created_at`. La guarda de `taken_at` evita soltar un nombre
  que, tras vaciarse la bolsa, volvió a tomar un pedido nuevo.
- **La cuenta que apunta a un pedido** («Nuevo» de un pedido enviado) no tiene nombre propio: es el
  del pedido.
- **Descartado**: amarrar el nombre al enviar (contradice D-2); reservar en una tabla aparte (dos
  fuentes para lo mismo).

## R-4. Enviar es una transacción: la cuenta y el pedido cambian juntos

- **Decisión**: `OrdersService.Create` y `AddLines` se parten en *preparar* (lecturas: precios,
  composición, turno) y *escribir dentro de una tx dada* (`createInTx`, `addLinesInTx`). El envío de
  una cuenta (`DraftsService.Send`) prepara fuera y luego, en **una** tx: bloquea la cuenta
  (`for update`, **antes** de mirar `orders.client_uuid`/`order_line_batches`: dos tabletas enviando
  la misma cuenta se serializan y la segunda ve `enviada`), verifica que sigue `capturando` y que su
  `updated_at` es el que se preparó (si no, vuelve a preparar, máx. 3; el reintento no re-sortea el
  nombre: `BoundFolioName` lo fija), escribe el pedido o el agregado y marca la cuenta `enviada` con su
  `order_id`.
- **Por qué**: dos transacciones dejarían una ventana donde el pedido existe y la cuenta sigue viva;
  otra tableta vería las dos y podría mandarla otra vez como agregado. La idempotencia por
  `client_uuid` atrapa el reintento de la misma cuenta, no la cuenta duplicada a la vista.
- **Idempotencia**: si la cuenta ya está `enviada`, `Send` devuelve el pedido sin escribir. Si un
  pedido con `client_uuid = draft.id` ya existe (cuenta importada de la versión anterior que sí se
  envió, R-10), se marca enviada contra ese pedido y no se crea otro.
- **Lo que no cambia**: turno exigido, folio por turno, fecha del reloj, inventario por renglón,
  `MarcarTodoElPedidoEnviadoACocina`, `agregados` para la comanda del agregado, reapertura de la
  entregada que debe.

## R-5. Cobrar: el front envía primero y luego abre la hoja sobre el pedido

- **Decisión**: «Cobrar» sobre una cuenta con algo sin enviar llama a `POST /pos/drafts/{id}/send`
  y, con el pedido que regresa, abre `CobrarSheet` con los tres modos. No hay endpoint
  «enviar-y-cobrar».
- **Por qué**:
  - «Por productos» necesita ids de renglón de **pedido**; antes de enviar no existen. Un endpoint
    combinado tendría que aceptar ids de borrador, traducirlos a ids de pedido dentro de `Charge` y
    tener su propio `quote` para borradores: tres caminos nuevos de dinero para el mismo cobro.
  - FR-007 y US3 AS3 dicen *primero cocina, después el pago*; el estado intermedio (en cocina, debe)
    es un estado legítimo que ya existe, se ve en la fila y el cierre de caja lo cuenta.
  - La hoja de cobro, `quote`, `pay`, sus llaves de idempotencia y sus tests de la 027 no se tocan.
- **El botón lo dice**: con renglones en «Nuevo», el botón del pie dice **«Enviar y cobrar $X»**; sin
  nada nuevo, «Cobrar $X». Si el envío falla o no hay red, la hoja no se abre y el motivo sale en el
  pie.
- **Consecuencia que se ve**: cerrar la hoja sin cobrar deja la cuenta «En cocina» (ya se mandó),
  no «Capturando». Nada se cancela (US4 AS2). La comanda ya salió: es lo que hoy pasa con «Enviar».
- **Descartado**: endpoint combinado en una transacción (ver arriba); abrir la hoja sobre el borrador
  y enviar al confirmar el primer pago (la hoja tendría dos fuentes de renglones).

## R-6. La lista de cuentas vivas: endpoint nuevo, estado derivado en `domain`

- **Decisión**: `GET /pos/accounts` reemplaza a `GET /orders/open?porCobrar=true` (único consumidor:
  `PedidosEnCurso`, que se elimina; el endpoint viejo se borra con él). Une:
  1. cuentas en captura sin pedido (`capturando`, `order_id is null`);
  2. pedidos `abierta` o `lista` de cualquier fecha;
  3. pedidos `entregada` que deben (`total − pagado > 0.01`, el mismo centavo de `PedidoSaldado`)
     de los **últimos 90 días**, o de cualquier fecha con `olderDebts=true` (ver R-7);
  4. pedidos ya cerrados que conservan una «Nuevo» viva (`closedWithPending`, R-9): sin esto lo
     capturado quedaría en una cuenta que nadie ve;
  5. a cada pedido se le pega su cuenta «Nuevo» viva, si tiene, con su número de renglones.
- **Estado de UI**: `domain.AccountState(kind, status, paid, total)` → `capturando · en_cocina ·
  pagada_en_cocina · pago_parcial · entregada_debe`, y `domain.AccountGroup(state, businessDate,
  today)` → `capturando · en_cocina · entregadas_deben · dias_anteriores`. Puras y table-driven; el
  front no reimplementa la regla.
- **Antes de listar** corre el barrido perezoso (R-8).

## R-7. «Entregadas que deben» de días anteriores: la fila mira 90 días, la hoja mira todo

- **Problema**: la consulta de hoy acota la deuda al día (`business_date = hoy`) por rendimiento:
  medido con 30 mil pedidos, sin ese filtro el `lateral` de pagos recorre todo el histórico (175 ms,
  90 mil buffers), y cada tableta la pide cada 30 s. Ningún índice parcial puede expresar
  `total − pagado > 0.01`.
- **Decisión**: dos alcances de la **misma** consulta, con el mismo predicado:
  - `GET /pos/accounts` (lo que se pide cada 30 s y en cada evento) acota la rama de deuda a
    `business_date >= hoy − 90` — range scan sobre `orders_company_date_status (company_id,
    business_date, status)` (0042). Techo medido en test: ≤ 30 ms con 30 mil pedidos.
  - `GET /pos/accounts?olderDebts=true` agrega las entregadas que deben de **cualquier** fecha.
    Solo lo piden la hoja «+N» al abrirse y la vista del cierre de caja: una lectura por acción del
    operador, no por intervalo. Así FR-009 («de cualquier día») se cumple sin pagar el barrido
    completo cada 30 s.
  - El predicado redundante de hoy (`o.status in ('abierta','lista') or o.business_date = $1`, en dos
    lugares de `ListOpenOrders`) pasa a `>= $desde`; los dos se mueven juntos.
- `// ponytail:` en la consulta: techo (el modo `olderDebts` crece lineal con el histórico; ~175 ms a
  30 mil pedidos) y camino de subida (`orders.owes` mantenida por trigger sobre `order_payments` y
  `orders.total`, con índice parcial; se rellena después desde `order_payments` al mismo costo).
- **Consecuencia que se ve**: un fiado de más de 90 días no está en la fila ni cuenta en su «+N»,
  pero sí aparece al abrir «+N» (grupo «De días anteriores») y en el cierre de caja.
- **Descartado**: sin ventana en la consulta de cada 30 s (crece lineal); columna con trigger ahora
  (toca todos los caminos de dinero para un caso que hoy no existe).
- **Corregido al implementar (medido, `accounts_live_perf_test`)**: con 30 mil pedidos la ventana de
  90 días tarda ~6 ms y `olderDebts` ~21 ms. El plan **no** entra por `orders_company_date_status`:
  la rama de cocina es de cualquier fecha, así que Postgres entra por un índice de empresa
  (`orders_branch`) y filtra. Sigue siendo lineal con el histórico de la empresa (lee sus pedidos por
  índice antes de filtrar), pero sin calcular pagos fuera de la ventana. Partirla en `UNION ALL` se
  midió y fue más lenta (~22 ms). El test topa el tiempo y exige un índice de empresa, no un nombre.

## R-8. Las 12 horas: barrido perezoso, no un job

- **Decisión**: `DraftsService.sweep(ctx, q)` descarta (con `where status = 'capturando'` en el
  propio `update`, para no pisar un envío concurrente que ya cambió el estado) (`status = 'descartada'`,
  `discard_reason = 'expired'`) las cuentas `capturando` con `updated_at < now() − 12 h`, suelta sus
  nombres (R-3), y descarta (`discard_reason = 'order_closed'`) las «Nuevo» cuyo pedido quedó
  `cancelada` o `reembolsada`. Corre dentro de la tx de `GET /pos/accounts`, de crear cuenta y de la
  vista del turno del cierre de caja.
- **Por qué**: las tabletas piden la lista cada 30 s (y con SSE); un job es otra pieza que vigilar
  para un efecto que nadie ve hasta que mira la fila. El umbral es `domain.DraftIdleLimit = 12 *
  time.Hour` (constante, principio VI).
- **Borde**: el barrido corre bajo RLS, así que solo toca la empresa de la sesión. Una empresa sin
  tabletas abiertas no barre, y no importa: nadie ve su fila.

## R-9. Qué pasa con la «Nuevo» de un pedido que se cierra o cambia

| Caso | Qué pasa |
|---|---|
| Pedido cancelado o reembolsado con «Nuevo» viva | El barrido la descarta (`order_closed`); enviar → `409` |
| Pedido que se cerró (pagado y entregado) con «Nuevo» viva | Enviar → `409 ORDER_CLOSED`; el front ofrece «Empezar cuenta nueva con estos productos» (crea cuenta nueva con los mismos renglones y descarta la vieja) |
| Pedido de plataforma | Crear una «Nuevo» se rechaza (`422`); nunca existe |
| Dos tabletas abren «Nuevo» del mismo pedido | Índice único parcial por pedido: la segunda recibe la cuenta existente con su agregado aplicado |

## R-10. Subir las cuentas de la versión anterior (D-12)

- **Decisión**: `POST /pos/drafts/import` recibe las pestañas de `egb:ticket:v2` con productos y
  devuelve un resultado por pestaña. Usa el id de la pestaña como id de la cuenta y un `opId`
  derivado (`uuidv5(tab.id, line.lineId)`, calculado en el front) por renglón: reintentar es inocuo.
- **Antes de crear**, por cada pestaña: si ya existe un pedido con `client_uuid = tab.id` o un lote en
  `order_line_batches` con esa llave, la pestaña **ya se había enviado** (la red se cayó después de
  que el servidor confirmó): no se crea cuenta y el resultado dice `already_sent` con el pedido.
  Crear una cuenta ahí mandaría la comida dos veces.
- **El front** borra `egb:ticket:v2` solo cuando todas las pestañas regresan con resultado; si la red
  falla, lo conserva y reintenta en la siguiente carga. Una pestaña que ya no parsea (la forma vieja
  que tumbaba la pantalla) se descarta con aviso en vez de bloquear las demás.
- **Descartado**: dejar las pestañas viejas como borradores locales (vuelve a tener dos fuentes).

## R-11. `PuedeRecibirLineas` deja de ser solo del estado

- **Decisión**: `domain.CanReceiveLines(o OrderForAdd) error` con `Status`, `Paid`, `Total`,
  `PlatformID`. Devuelve `ErrOrderClosed` para entregada **y** saldada (D-9), `ErrPlatformOrderNoLines`
  para plataforma (D-11) y `ErrConflict` para cancelada/reembolsada. Reemplaza a
  `PuedeRecibirLineas`; sus llamadores (`AddLines`, `move_lines` destino) se mueven juntos (el
  hermano que no se movió, constitución IV).
- **Corregido al implementar**: `PuedeRecibirLineas` tenía seis llamadores, no dos. Los otros cuatro
  (entregar un renglón, cancelar un renglón, devolver un pago y el **origen** de pasar productos) no
  agregan nada: preguntan solo si el dinero del pedido sigue sin decidirse. Para ellos queda
  `domain.OrderNotVoided(status)` con la misma semántica de antes; el destino de pasar productos sí
  pasa a `CanReceiveLines` (un pedido cerrado ya no recibe por ningún camino).
- `ReabreAlAgregar` no cambia: la entregada que debe vuelve a `abierta`.

## R-12. Tiempo real: un evento nuevo y el POS escucha

- **Decisión**: el broker publica `draft.updated` `{id, orderId, status, updatedAt}` en cada
  escritura de cuenta (crear, agregar, cambiar, cabecera, descartar, enviar). El POS monta
  `useOrderEvents` (ya existe para el tablero) e invalida `['pos','accounts']` y la cuenta abierta con
  `order.*` y `draft.*`. Refresco periódico de 30 s como red (SSE se cae en silencio en tabletas
  suspendidas).
- **El aviso de «cambió en otra tableta»** (FR-017) se decide en el front comparando el estado de la
  cuenta abierta antes y después de recargar, con una función pura testeada
  (`cambioEnOtraTableta(antes, despues, misOperaciones)`). Las operaciones propias en vuelo no
  avisan.

## R-13. El front deja de guardar cuentas

- **Decisión**: la verdad es TanStack Query (`['pos','accounts']`, `['pos','draft',id]`,
  `['orders',id]`). Zustand persistido queda en `egb:pos:v3` con solo `{ selected: {kind, id} | null }`
  y las preferencias que ya vivían ahí. Lo que está «guardando» es estado de mutación en memoria: un
  renglón optimista con `pending: true` que se quita si falla, con toast «No se guardó · Reintentar».
- **Sin conexión** (`navigator.onLine` + fallo de red en una mutación): banner fijo, «Enviar» y
  «Cobrar» apagados, agregar apagado con su motivo; reintento automático al volver.

## R-14. Diálogos del sistema fuera

- `ConfirmSheet` (hoja inferior de la app, botones de 44 px, acción destructiva separada) reemplaza
  `confirm()` en `TicketTabs`, `Ticket` y `CashPage`; `ReasonSheet` reemplaza el `prompt()` de
  `ExpensesPage`. Un test estático (`sinDialogosDelSistema.test.ts`) recorre `web/src` y falla con
  `confirm(`, `prompt(` o `alert(` de `window` (excluye `installPrompt.ts`, que es la API de PWA).

## R-15. «Abrir cuenta» desde el tablero y el cierre

- Ruta `/pos?pedido=<id>` y `/pos?cuenta=<uuid>`. `POSPage` lee el parámetro, selecciona la cuenta y
  limpia el parámetro (para que F5 no la vuelva a forzar).

## R-16. Esquema: ¿se puede agregar después al mismo costo? (principio VIII)

| Decisión | ¿Mismo costo después? | Consecuencia |
|---|---|---|
| Cuenta en captura sin `branch_id` | **Sí**: vive ≤ 12 h; agregar la columna mañana no tiene histórico que rellenar. La sucursal ya existe (0076) y la sella el pedido al enviarse (`trg_orders_branch`) | No se agrega (VI) |
| Cuenta sin turno ni fecha de negocio (D-6) | **Sí**: los sella el pedido al enviarse | No se agregan |
| `opened_by` en la cuenta | **No**: quién capturó se pierde al enviarse si no se guardó desde el nacimiento | Se guarda (FR-020) y pasa al pedido. **No resuelve la puerta «de quién es un pedido»**: dos tabletas con la misma cuenta de usuario siguen sin distinguirse; eso pide una estación, que no se construye hoy |
| Conservar las descartadas (estado, no `delete`) | **No**: el rastro de lo abandonado no se reconstruye | Se conservan con `discarded_at`, `discarded_by`, `discard_reason` |
| Precio del renglón en captura | **Sí**: se reprecia al enviar como hoy; un snapshot aquí no protege nada | No se guarda |
| Modificadores como `jsonb` | **Sí**: el renglón muere al enviarse, y el pedido sí los guarda en `order_line_modifiers` | `jsonb` validado en `domain` |
| Nombre único por empresa entre cuentas vivas (no por sucursal) | **Sí**: el índice se rehace sin datos que mover. Con dos sucursales, un nombre vivo en una bloquea el mismo nombre en la otra (más estricto, no ambiguo) | Por empresa |
| Ventana de 90 días en la consulta frecuente | **Sí** (consulta, no esquema) | Ver R-7 |
| `modifiers` sin FK a opciones | **Sí** | Una opción borrada o desactivada con la cuenta viva se descubre al enviar: `422` que nombra el producto, nunca `500` (test) |
