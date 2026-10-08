# Research: Dividir la cuenta por productos

Tres investigaciones de solo lectura sobre el código de esta rama (dinero, renglones y pantalla)
alimentan estas decisiones. Cada una dice qué se eligió, por qué y qué se descartó. Las citas son
`archivo:símbolo`.

**Revisión de arquitectura (2026-10-05)**: `db-architect` y `tablet-ui-reviewer` pidieron cambios. Este
documento ya los incorpora, junto con la decisión del dueño de que los roles serán configurables
por empresa (D-16). Lo que cambió por la revisión se marca con **[rev]**.

**`/speckit-analyze` (2026-10-05)**: el dueño aprobó sus recomendaciones y tres decisiones (pasar
todos los productos, D-5; esperar a 021, D-11; la enmienda de la constitución en commit propio). Lo
que cambió por el análisis se marca con **[analyze]**.

## Hechos que condicionan el diseño

| Hecho | Dónde | Qué obliga |
|---|---|---|
| `order_payments` no tiene renglón, estado ni devolución; `amount > 0`; **ninguna FK apunta a él** | `0007_orders.sql`, verificado en el esquema vivo | La cobertura por renglón es una tabla nueva; borrar un pago no rompe referencias |
| Lo pagado (`SumOrderPayments`, `load`, `ListOpenOrders`, `UncollectedInSession`, `GetOrderParaDescuento`) **nunca resta devoluciones** | `queries/orders.sql`, `queries/cash.sql` | Un pago «devuelto» que siga en la tabla deja el pedido saldado |
| ~32 consultas suman `order_payments` directo: corte por método, propinas, ventas por método, por cajero | `cash.sql`, `sales.sql`, `reports.sql` | Marcar un pago como devuelto obliga a tocarlas todas, y la que se olvide cuenta el dinero dos veces sin fallar |
| `Charge` ya serializa por pedido (`GetOrderForUpdate`) y por caja (`LockOpenPrimarySession`) | `app/orders.go:Charge` | «Una pieza no se cobra dos veces» se valida bajo ese mismo candado |
| El trigger de existencias es **solo AFTER INSERT** | `0009`, `apply_stock_movement` | Mover un movimiento de pedido (UPDATE de `order_id`) no cambia existencias; partir se hace **insertando**, nunca con UPDATE de `quantity` |
| `RestockCancelledOrder` invierte toda `venta` del pedido, incluidas las de renglones ya quitados | `queries/orders.sql` | Ver D-8 |
| `products.needs_prep` existe y no se usa al capturar; todo renglón nace «enviado a cocina» | `0044`, `Create`, `AddLines` | La reposición se decide con `needs_prep` |
| `parent_line_id` no lo escribe nadie | `domain/order.go` | Partir o mover no arrastra hijos |
| `order_refunds.order_line_id` es `on delete restrict` y guarda su propio `order_id` | `0060` | Un renglón con devolución no se mueve ni se parte |
| goose corre sin `WithAllowMissing` | `server/internal/store/migrate.go:22` | Ver D-11 |
| `inTheThreeCases`, `TestEveryCompanyTableIsIsolated` y las políticas con `nullif` viven en la rama `021-recibir-pedidos-uber` | `three_cases_test.go`, `rls_all_tables_test.go`, `0074` (021) | Ver D-11 |
| «Cancelar pedido» se protege por nombre de rol | `router.go`: `RequireRole(admin, gerente)` en `/orders/{id}/cancel` | Ver D-16 |
| El prefijo «conflicto:» / «datos inválidos:» nace en `httpapi.Error`, que responde `err.Error()` de sentinels envueltos con `%w: …` | `server/internal/httpapi/respond.go`, `Error` (`msg := err.Error()`, línea 44) | Afecta a **todos** los 400 y 409; ver D-10 |
| Login, refresh y pin-switch responden por el mismo `writeSession`; `/auth/me` arma su propia respuesta | `server/internal/httpapi/handlers.go:244` (`writeSession`), `handlers.go:485` (`Me`) | Ver D-16 |
| Roles: enum `user_role` de 4 valores, 27 rutas con `RequireRole`, espejo en `web/src/app/roles.ts` | `0001_users.sql`, `router.go` | Ver D-16 |
| La hoja de cobro guarda lo cobrado solo en memoria (`yaCobrado`); la vista del pedido no trae pagos | `CobrarSheet.tsx`, `types/pos.ts:OrderView` | La lista de pagos viene del servidor |
| Con efectivo, propina y «Recibido», el chrome de la hoja se lleva ~450 px de 600; con el teclado del sistema el viewport baja a ~320 | Revisión de pantalla sobre A6 y `CobrarSheet.tsx` | Ver D-13 |
| Una tarjeta del tablero con 11 renglones mide ~620 px | `OrdersBoardPage.tsx:305-336` | Ver D-10 |

## Decisiones

### D-1 · La cobertura vive en una tabla `order_payment_lines`

- **Decisión**: una fila por (pago, renglón) con cuántas piezas y cuánto dinero de ese renglón pagó.
  Σ piezas cubiertas de un renglón ≤ su cantidad; se valida en `app` bajo el `FOR UPDATE` del pedido
  que `Charge` ya toma.
- **Por qué**: es el único hecho que no se registra hoy y que no se puede reconstruir después
  (principio VIII). Bajo el candado del pedido, dos tabletas no pueden cubrir la misma pieza.
- **Descartado**: un arreglo JSON en el pago (no se suma por renglón ni se protege con FK); una
  columna `paid_qty` en `order_lines` (pierde qué pago la cubrió y no se deshace al devolverlo).

### D-2 · Devolver un pago lo **saca** de `order_payments` a una bitácora `order_payment_voids`

- **Decisión**: «Devolver este pago» copia el pago **completo** a `order_payment_voids`, con quién,
  cuándo y por qué, y borra su fila de `order_payments`; su cobertura cae en cascada y la copia la
  conserva en `covered`. Se copian método, monto, propina, `reference`, turno, quién lo cobró,
  `client_uuid`, parte y hora.
- **Cuándo se permite**: solo si el `register_session_id` **del pago** es un turno con
  `status = 'abierta'`, y el pedido no está cancelado ni reembolsado (`entregada` sí se permite: un
  pedido saldado y entregado puede tener el cobro equivocado). Un pago con sesión nula (hay 55 viejos) o de un
  turno cerrado se rechaza y va por la devolución de siempre.
- **[rev] Candado**: `VoidPayment` toma `GetOrderForUpdate` del pedido **antes** de leer el pago.
  Es el mismo candado de `Charge`. Sin él, un reintento del cobro con la misma llave podía ver el
  pago, perderlo por el borrado y volver a insertarlo: el pago revivía.
- **[rev] Una sola vez**: `unique (company_id, original_payment_id)` en la bitácora. La migración
  inversa se niega a correr si la bitácora tiene filas, porque es la única copia de ese dinero.
- **Por qué sacarlo**: deja correctas **por construcción** las ~32 consultas que suman
  `order_payments`. Un pago devuelto en su mismo turno y por el mismo método es, para el cajón, un
  pago que no ocurrió. Ese peso se clasifica una vez (principio III).
- **Consecuencia que no se ve**: el corte no lo muestra como salida, sino en una lista aparte de
  pagos devueltos del turno (FR-017 ya lo dice).
- **Revivir por la llave**: `Charge` consulta también la bitácora; un `client_uuid` de un pago
  devuelto responde «Ese pago ya se devolvió. Vuelve a cobrar».
- **Descartado**:
  - Marcar el pago (`voided_at`): exige filtrar en cada consulta que suma pagos.
  - `order_refunds` con `order_payment_id`: lo pagado no resta devoluciones y los reportes tampoco.

### D-3 · El monto de una selección lo calcula `domain`

- **Decisión**: `domain.SelectionAmount` recibe los renglones (precio y modificadores por
  unidad, cantidad, piezas ya cubiertas), la selección, el descuento, el subtotal, el envío y lo que
  falta. Devuelve el monto total y el monto por renglón:
  - bruto = Σ piezas × (precio + modificadores por unidad);
  - descuento de la selección = `Round2(bruto × descuento / subtotal)`;
  - si la selección cubre **todas** las piezas aún no cubiertas, el monto es exactamente lo que
    falta: absorbe envío, redondeo y lo pagado sin productos;
  - si el monto pasa de lo que falta (pasa tras pagos por monto), se rechaza.
- **[analyze] Residuo por renglón en toda selección**: no solo en la que cubre todo. El monto de
  cada renglón se redondea por separado y la diferencia contra el monto del pago cae en el último
  renglón, para que Σ `order_payment_lines.amount` = `amount` del pago, exacto, en cualquier pago.
- **Prorrateo por renglón** (`order_payment_lines.amount`): si la selección cubre todo lo pendiente
  pero cobra **menos** que su bruto descontado (porque antes hubo pagos por monto), el monto de cada
  renglón se escala en proporción a su bruto descontado, con `Round2`, y el residuo cae en el último
  renglón, para que Σ `amount` = `amount` del pago. Si cobra más (envío, residuo), cada renglón lleva
  su bruto descontado y la diferencia queda como lo cubierto sin producto.
- **Devolver un pago por monto no toca ninguna cobertura**: no tenía; el saldo vuelve a deberse sin
  piezas asociadas.
- **`allRemaining` sin piezas por cubrir**: si ya no queda pieza sin cubrir y el saldo es positivo
  (p. ej. tras devolver un pago por monto), cobra lo que falta sin cobertura. No es una selección
  vacía y no se rechaza.
- **Dónde no aplica**: con `lines`, `allRemaining` o `split`, un pedido de plataforma se rechaza («Los
  pedidos de plataforma no se dividen») y uno de un turno cerrado también; `amount` solo sigue como
  hoy.
- **Por qué**: la regla del residuo es dinero y vive en un solo lugar; el monto de la pantalla se
  ignora (III).
- **[analyze] El monto se ve antes de cobrar: `POST /orders/{id}/quote`** (decisión del
  2026-10-06). Endpoint de solo lectura con el mismo cuerpo que `/pay` (`lines` | `split` |
  `allRemaining`), sin `methodId`. No escribe; lee bajo un `SELECT` sin `FOR UPDATE` y llama a la
  **misma** función de `domain` que `Charge`, así que no hay una segunda regla de dinero en la
  pantalla. Devuelve `{ amount, lines, outstandingAfter }` y rechaza lo mismo que `/pay`, salvo la
  idempotencia. `/pay` recalcula siempre: si otra tableta cambió el pedido entre la cotización y el
  cobro, cobra lo que calcula entonces y lo devuelve, y la pantalla se refresca con ese monto.
  - Descartado: calcular en el front con una copia de la regla. Es justo la duplicación que D-4
    quita de `web/src/domain/cobro.ts`.
  - Descartado: rechazar el cobro si difiere de lo cotizado. Obligaría a mandar el monto desde la
    pantalla, que el servidor ignora a propósito.

### D-4 · «Entre personas» persiste en el pago

- **Decisión**: `split_part` y `split_of` en `order_payments`. El servidor calcula la parte sobre lo
  que falta en ese momento, y la última absorbe el centavo. **[rev]** El servidor rechaza cobrar una
  parte ya cobrada de la misma serie: es la idempotencia de este modo, que la llave no cubre si se
  rota al cambiar de intención.
- El tope de 12 vive en `domain`, no en un check: **[rev]** la base solo exige `split_of >= 2`.
- `dividirEnPartes` y `montoDeLaParte` pasan de `web/src/domain/cobro.ts` a `domain` en Go, como
  `domain.SplitParts` y `domain.SplitPartAmount`. **[analyze]** La pantalla pide el monto de cada
  parte a `quote` con `split`; ya no lo calcula.

### D-5 · Pasar productos a otro pedido mueve el renglón

- **Decisión**: `OrdersService.MoveLines(origen, destino|nuevo, [{renglón, piezas}], clientUUID)`.
  - Bloquea los dos pedidos en orden ascendente de id y luego sus renglones (mismo orden que
    `DeliverLine` y `CancelarRenglon`). **Es lo que serializa de verdad.**
  - Cambia `order_lines.order_id` y el `order_id` de **los movimientos de ese renglón**
    (`stock_movements where order_line_id = …`; **[rev]** nunca filtrando por `order_id`).
  - Conserva `enviado_a_cocina_at` (no hay comanda nueva) y `delivered_qty`.
  - Registra el lote y cada renglón movido (D-12b).
  - Recalcula totales y corre `cerrarSiYaSeEntregoTodo` en los dos.
- **[rev] Turno y día**: el pedido nuevo **copia del origen** `register_session_id`,
  `business_date`, `service_type` y `opened_by`; no pide «la caja abierta» (eso cerraría la puerta
  de más de una caja). Folio y nombre salen del contador y la bolsa **de ese turno**. Un destino
  existente con otro `register_session_id` u otro `business_date` se rechaza. El actor queda en
  `moved_by`.
- **Rechazos**: piezas pagadas; renglones con devolución; pedidos de plataforma; origen de un turno
  cerrado; destino que no esté abierto o listo; con descuento (D-6); dejar el origen con más pagado
  que su total; **[rev]** un renglón de producto con receta cuyos movimientos no tengan
  `order_line_id` (anteriores a 0060).
- **Pasar todos los productos** (decisión del dueño, 2026-10-05):
  - **hacia un pedido nuevo** se rechaza: «Ya es su propio pedido; no hace falta pasarlo». Es un
    no-op: el folio, el turno y los productos serían los mismos;
  - **hacia un pedido existente** (caso D, cuenta equivocada): el origen vacío se cierra como
    **juntado con otro pedido**: `status = cancelada`, motivo fijo «Se juntó con otro pedido» y
    `orders.merged_into_order_id` = destino, sin reponer inventario (el consumo viajó con los
    productos). Solo pasa si no tiene pagos: con pagos ya lo detiene la regla de no dejar el origen
    sobrepagado.
  - **Origen con envío**: pasar todo hacia un pedido existente se rechaza («Ese pedido tiene envío;
    cóbralo o quítalo antes de juntarlo»). Igual que con el descuento (D-6): mover el envío entre
    pedidos es una regla de dinero que nadie pidió, y cualquier versión silenciosa cobra de más o de
    menos.
  - **Los reportes no lo cuentan como cancelación**: `SalesTotalsByStatus` y las gemelas que cuentan
    pedidos `cancelada` (`server/queries/sales.sql`, `domain.SummarizeSales` en
    `server/internal/domain/sales.go:262`) excluyen el pedido juntado, con el **mismo predicado** en
    lista, conteo y resumen (principio III). **[analyze]** Lo mismo en las ventas del turno del
    corte: `SessionSales` y `CountSessionSales` (`server/queries/cash.sql`, pintadas en la lista
    de ventas del turno de `CashPage.tsx`) usan un solo predicado en lista y conteo.
  - **[analyze] Lo quitado antes de juntar sí es cancelación**: hoy `SalesCancelledLines` y su
    gemela filtran `o.status not in ('cancelada', 'reembolsada')`, así que **no** contarían nada del
    pedido juntado, que queda `cancelada`. Decisión: las dos agregan `or o.merged_into_order_id is
    not null` a ese filtro, para que los productos que se **quitaron** del origen antes de juntarlo
    sigan contando como productos cancelados, aunque el folio no aparezca como pedido cancelado.
    `SalesCancelledLines` es un total (no hay pantalla que liste renglones cancelados por folio), así
    que no hace falta etiqueta. Lo vigila
    `TestAMergedOrderIsNotACancellation`.
  - **Por qué una columna y no solo el motivo**: el motivo es texto para quien opera; un reporte que
    filtre por texto se rompe al primer cambio de redacción. La columna además dice con cuál se
    juntó.
- **[rev] En pantalla**: tocar un destino lo marca y el pie de la vista dice «Pasar a Singapura #6»;
  se confirma con un toque más. Los rechazos que se pueden saber antes (descuento, piezas pagadas o
  mixtas, plataforma) deshabilitan «Pasar a otro pedido» con su motivo **antes** de abrir la lista.
  Tras «+ Pedido nuevo», la hoja ofrece «Cobrar #N».

### D-6 · Pasar productos con descuento se rechaza por ahora

- Si el origen o el destino tienen descuento: «Quita el descuento antes de pasar productos».
- **¿Se puede agregar después al mismo costo?** Sí. No cierra la puerta *Descuentos*.

### D-7 · Partir un renglón inserta movimientos, nunca los edita

- **Decisión**: quitar o mover `k` piezas de un renglón de `n` crea un renglón nuevo de `k` piezas
  que copia precio, modificadores (por unidad), costo y estado de cocina.
- **Inventario**: por cada movimiento `venta` del original se insertan dos de tipo `venta` con
  `reason = 'renglón partido'` (distinguible en el kárdex):
  - `+r` en el original, con el `order_id` del origen;
  - `−r` en el nuevo, con el `order_id` de su dueño final (el destino al mover);
  - **[rev]** con `r = q − Round4(q·(n−k)/n)` para que las dos mitades sumen exacto.
- **Entrega**: el nuevo se lleva primero piezas pendientes. Si se piden más piezas de las pendientes
  en un renglón mixto, se rechaza con «Ese producto tiene piezas entregadas y otras sin entregar.
  Pásalas todas juntas».
- **Cobertura**: lo pagado se queda en el original; `k ≤ n − cubiertas`.
- **Reglas en `domain`**: `domain.SplitLine` y `domain.MovablePieces`.

### D-8 · Quitar devuelve lo que no se consumió, sin reponer dos veces

- **Decisión**:
  - `domain.ReponeInventario(needsPrep, enviado)` devuelve `!needsPrep || enviado == nil`.
    `GetOrderLineForCancel` trae `products.needs_prep`.
  - **[rev]** `RestockCancelledOrder` excluye todo movimiento cuyo renglón ya esté cancelado
    (`cancelled_at is not null`), se haya repuesto o no. Cubre las dos formas de reponer de más: el
    renglón ya repuesto y el que se quitó **ya consumido**, que hoy se repone al cancelar el pedido.
    Los movimientos sin `order_line_id` (anteriores a 0060) siguen entrando como hoy.
- **Descartado**: dejar de marcar «enviado a cocina» lo que no se prepara (rompe la comanda del
  agregado).

### D-9 · Quitar un renglón respeta lo pagado

- `CancelarRenglon` gana `qty` (por omisión, todas las pendientes) y parte el renglón con D-7.
- Rechaza piezas cubiertas y dejar el total bajo lo pagado. Antes se escribe el test que confirma
  si hoy deja dinero de más (FR-023 no verificado).
- **Verificado el 2026-10-07** con `TestRemovingAPaidLineIsRejected` contra el código de antes del
  arreglo: quitar el producto pagado de un pedido se aceptaba sin error, y el total bajaba por
  debajo de lo cobrado sin devolver nada. FR-023 era un defecto real.
- `CancelPending(pedido, motivo)`: quitar lo que falta, en una transacción. Responde
  `{ removed, restocked }`: cuántos productos quitó y cuántos repusieron inventario.
- **[analyze] Los mismos rechazos que quitar un producto, también los de pagos**: `CancelPending` y
  `CancelarRenglon` llaman a **una sola** función que rechaza piezas cubiertas por un pago y dejar el
  total bajo lo pagado. Si `CancelPending` no la llamara, quitar lo que falta sería el camino nuevo
  que se salta el control viejo (constitución IV).
- **[analyze] Partir llega con `CancelPending`**: un renglón con entrega parcial se parte para quitar
  solo lo pendiente, así que las consultas de partir (copiar el renglón y sus modificadores,
  insertar los pares `venta` con `reason = 'renglón partido'`) nacen con `CancelPending`, antes del
  rebase sobre 021: no tocan tablas nuevas. Su aislamiento entra en la prueba tras el rebase, y
  `CancelarRenglon` con `qty` las reusa.
- **[rev] En pantalla**: el bote de un producto pagado aparece deshabilitado con «Pagado».
  **[analyze]** Para saberlo, cada `BoardLine` del tablero trae sus piezas cubiertas por pagos
  (`paidQty`, siempre presente), con su consulta aislada en los tres casos.

### D-10 · Ningún pedido sin salida

- `DeliverAll` rechaza un pedido sin renglones vivos: «Este pedido ya no tiene productos: ciérralo».
- **[analyze] Pedido sin productos vivos** (decisión del 2026-10-06): «Quitar lo que falta»
  (`orders.cancel_pending`, todos los roles) sobre un pedido sin productos vivos y **sin pagos** lo
  cancela: `status = cancelada`, motivo fijo «Sin productos», sin reponer inventario (cada renglón
  ya se resolvió al quitarlo). Es una cancelación verdadera y **sí** cuenta en los reportes. Con
  pagos se rechaza («Tiene pagos: hay que devolverlos primero»), y devolverlos ya tiene su permiso.
  Así cualquier rol puede cerrar un pedido vacío sin que «Cancelar pedido» (`orders.cancel`) deje de
  ser de admin y gerente. En ese caso «Cerrar pedido» manda la petición sin motivo: el motivo es
  fijo.
- **[analyze] Dos ítems en el menú de la tarjeta**: «Quitar lo que falta» con
  `can('orders.cancel_pending')` (hoy todos los roles), que abre la hoja de quitar lo que falta, y
  «Cancelar pedido» con `can('orders.cancel')`, aparte. Así quien no puede cancelar el pedido sí puede
  quitar lo que no se va a entregar.
- **[rev] La tarjeta, por combinación**:

  | Nada pendiente | Debe | El tablero cobra | Qué ofrece |
  |---|---|---|---|
  | sí | no | — | «Cerrar pedido» |
  | sí | sí | sí | «Cobrar $X» |
  | sí | sí | no | el texto «Falta cobrar $X en caja» |
  | no | — | — | «Entregar todo» (y «Cobrar» si debe y cobra) |
  | sin productos vivos | no (sin pagos) | — | «Cerrar pedido», que llama a `lines/cancel-pending` |
  | sin productos vivos | tiene pagos | — | el texto «Tiene pagos por devolver», y el acceso a devolverlos con `payments.void` |

  «Cerrar pedido» con lo vivo entregado y sin deuda llama a `deliver`; sin productos y sin pagos, a
  `lines/cancel-pending` (contracts/api.md). Un test recorre las diez combinaciones: las ocho de
  pendiente × debe × el tablero cobra, más las dos sin productos vivos (SC-003 en pantalla).
- **[rev] Tarjetas largas**: con más de 5 renglones pendientes, la lista lleva alto máximo en dvh
  y scroll propio, y los botones quedan siempre a la vista.
- **[rev] Cancelar pedido**:
  - Con algo entregado y nada pendiente, ofrece «Cerrar pedido» en vez de una hoja vacía.
  - El texto de `ErrCancelarConEntregas` deja de sugerir un reembolso.
  - El prefijo del sentinel no llega a la pantalla. Nace en `httpapi.Error`
    (`respond.go`, `Error`: `msg := err.Error()` en la línea 44, sobre un sentinel envuelto con `%w: …`) y afecta a **todos** los
    409 («conflicto:») y 400 («datos inválidos:»), no solo a cancelar. Se quita ahí, para todas las
    respuestas; el radio se mide con los tests existentes antes de cambiarlo
    (`web/src/shared/CobrarSheet.test.tsx:190` espera `'conflicto: …'`) y se actualizan.
  - «Cancelar pedido» se ofrece solo con `can('orders.cancel')` (D-16).

### D-11 · Migración sin número reservado, RLS como en 021, y lo que toca tablas espera a 021

- **[rev] Número**: la migración se escribe con un nombre de trabajo y **toma el siguiente número
  libre de `develop` al fusionar**. No se reserva 0078. goose corre sin `AllowMissing`: si 027 entra
  antes que 0073–0077, la API no arranca cuando lleguen. No se activa `AllowMissing` para taparlo,
  porque cambia el runner de producción. El orden de fusión (021, 025, 026, 027) se comprueba al
  fusionar.
- **RLS**: default y política con `nullif(current_setting('app.company_id', true), '')::bigint`,
  como 021. Como la migración se escribe ya rebasada sobre 021, `TestEveryCompanyTableIsIsolated`
  la recorre desde el primer día, y 0074 salta las políticas que ya traen `nullif`.
- **[analyze] Dependencia de 021** (decisión del dueño, 2026-10-05, opción a): `inTheThreeCases`
  (`three_cases_test.go`), `TestEveryCompanyTableIsIsolated` (`rls_all_tables_test.go`) y las
  políticas con `nullif` (0074) llegan con 021. **No se copia nada.**
  - Se construye ya lo que no toca tablas nuevas: US7, US8, el dominio puro y los permisos. Las
    consultas existentes que esas fases modifican quedan cubiertas por la prueba de aislamiento
    tras el rebase (T032).
  - Lo que sí las toca —la migración, las consultas nuevas y los servicios nuevos con sus pruebas
    de aislamiento— **espera a que 021 llegue a `develop`**, y 027 se rebasa encima.
  - Ningún test se marca `skip`. El aislamiento de lo construido antes (`CancelPending`,
    `RestockCancelledOrder`, `GetOrderLineForCancel`) se escribe tras el rebase y se fuerza a verse
    en rojo. Es la única excepción al orden test-primero, es consecuencia de esta decisión y queda
    registrada en *Complexity Tracking* del plan.
  - **Al rebasar**, el conflicto en la constitución se resuelve reaplicando la enmienda de roles
    sobre la 1.13.0 de 021, que queda en 1.14.0 (T002).
  - Descartado: copiar el helper byte por byte. Dos ramas manteniendo el mismo archivo y una prueba
    de catálogo que no existiría aquí hasta fusionar.

### D-12 · Los pagos viajan en la vista del pedido, siempre como arreglo

- `OrderView.payments` y sus `lines` son arreglos, nunca `null` (test sobre JSON crudo). `ChargeResult`
  devuelve el id del pago.
- **[rev] Numeración estable**: cada pago tiene su número fijo dentro del pedido. Los devueltos
  aparecen tachados con «Devuelto» (de la bitácora), para que el «Pago 2» impreso siga siendo el 2
  en pantalla.
- **[analyze] La regla del número**: `payment_number` = (pagos vivos + devueltos del pedido,
  contando los viejos sin número) + 1, calculado bajo el candado del pedido. La vista numera los
  viejos sin número por `created_at` con la misma regla, y al devolver uno la bitácora guarda el
  número que la vista le daba. Así un pago viejo y uno nuevo nunca comparten número.

### D-12b · Idempotencia de «Pasar» por lote **[rev]**

- `order_line_move_batches (company_id, client_uuid, from_order_id, to_order_id)` con PK
  `(company_id, client_uuid)`, igual que `order_line_batches` (0063): insert con
  `on conflict do nothing returning`.
  - Si ya existía y coincide en origen y destino: no-op con la respuesta de antes.
  - Si difiere: `ErrConflict`.
- `order_line_moves` cuelga del lote.
- Se descartó la llave por renglón: un renglón partido tiene id nuevo en cada intento.

### D-13 · La hoja de cobro crece por dentro, no hacia abajo

- **[rev] Selector**: aparece **solo al tocar «Dividir»**, que sigue en el encabezado. Quien cobra
  a una sola persona no paga 52 px.
- **[rev] Pie fijo mínimo**: solo la fila de método (4 botones de 48 px), «Esta persona: N
  productos» y los botones (Todo lo que falta, Pasar, Cobrar), unos 130 px. Propina y «Recibido» van
  dentro de la zona con scroll, debajo de la lista, y solo con Efectivo o si se pide propina. El pie
  se encoge con `maxH` en dvh para que el teclado del sistema no lo expulse. La hoja usa `100dvh`.
- **[rev] Lista**: los pendientes arriba y los pagados al final, en gris y compactos; con más de 4
  pagados se agrupan en «N pagados ▸». El orden vive en un helper con test.
- **[rev] Fichas de pagos**: en una sola fila con scroll horizontal, de 44 px de alto. Pestañas del
  selector y contadores también ≥ 44 px.
- **[rev] Vistas, no hojas apiladas**: un estado `view: 'charge' | 'detail' | 'move'` cambia el
  contenido del mismo `DrawerContent`. Las vistas inactivas se ocultan, no se desmontan (conservan
  scroll y selección). No se anidan Drawers ni se usa `Picker` dentro. «Reimprimir» abre el
  `Dialog` que ya existe.
- **[rev] Fila de lista propia**: se saca `PickerRow` a `web/src/components/ListRow.tsx`
  (`ListRow`) con alto parametrizable (56 px para pedidos), y `Picker` la importa. El buscador de la lista de pedidos no
  se enfoca solo, para no abrir el teclado sobre la lista.
- **[rev] Envío**: la llave de idempotencia rota solo tras un cobro exitoso; mientras se envía, la
  selección, las partes y el monto quedan deshabilitados.
- **[analyze] Monto del botón**: viene de `quote` (D-3), pedido con debounce al cambiar la selección
  o las partes, para no mandar una petición por toque. Mientras llega, el botón conserva el monto
  anterior deshabilitado. Si `/pay` responde otro monto, el botón y «Falta» se refrescan con él.
- **[rev] Descuento con pagos**: «Descuento» se oculta en cuanto el pedido tiene pagos, y el
  servidor lo rechaza (D-17).
- **Prueba de alto**: E7 bis mide con Efectivo, propina, la lista de productos **y con el teclado
  del sistema abierto**.

### D-14 · Ticket por pago

- `buildReceiptHtml` gana `opts.payment`: solo los renglones (y piezas) de ese pago, «Pago N», método,
  propina, cambio y «Del pedido quedan por pagar $X».
- La impresión automática se recuerda por id de pago y corre tras cada pago de un pedido dividido,
  si `autoPrintOnClose` está encendido.
- `VerTicket` pasa de la clave `['order', id]` al prefijo `['orders', …]`.

### D-15 · Medición de uso

- Acciones nuevas en los dos lados a la vez (`uso.go` y `medirAccion`), **en inglés** porque se
  guardan como dato (VII):
  - `pos` y `pedidos`: `split-by-products`, `move-lines`, `void-payment`;
  - `pedidos`: `close-order`, `cancel-pending`.
- Se actualiza la lista `ARCHIVOS` de `uso-orden.test.ts`.

### D-16 · Lo nuevo se pregunta por permiso, no por rol (decisión del dueño, 2026-10-05)

- **Contexto**: los clientes podrán crear roles propios y decidir qué puede cada uno. La
  constitución gana la puerta *Roles y permisos configurables por empresa*.
- **Decisión**:
  - `domain.Permission` (string estable en inglés: `payments.void`, `orders.cancel`,
    `orders.move_lines`, `orders.cancel_pending`).
  - `domain.PermissionsFor(role)`: el **mapa fijo de hoy**. `payments.void` y `orders.cancel` para
    admin y gerente; `orders.move_lines` y `orders.cancel_pending` para todos.
  - **[analyze]** `orders.cancel` existe porque la tarjeta del tablero tiene que preguntar por
    permiso si ofrece «Cancelar pedido», y la ruta `/orders/{id}/cancel` pasa de `RequireRole` a
    `RequirePermission(orders.cancel)`. Sin él, el tablero seguiría preguntando por nombre de rol.
  - Middleware `RequirePermission(p)` junto a `RequireRole`.
  - `domain.PermissionDeniedMessage(p)`: el texto «Tu usuario no puede …» de cada permiso, junto a
    `PermissionsFor`. Lo usa `RequirePermission` y lo espeja la pantalla.
  - La sesión (login, refresh y pin-switch, que comparten `writeSession`) y `/auth/me` devuelven
    `permissions: []`; la pantalla pregunta por permiso (`can('payments.void')`), nunca por nombre
    de rol.
  - El texto sin permiso no nombra roles («Tu usuario no puede devolver pagos»): con roles propios,
    «gerente» puede no existir.
- **Por qué**: cuando lleguen los roles por empresa, lo único que cambia es de dónde sale el mapa
  (la base, por empresa). Ningún control de esta feature se reescribe.
- **¿Se puede agregar después al mismo costo?** Los roles por empresa sí: no se construyen hoy.
  Escribir controles nuevos por nombre de rol **no**: cada uno sería una reescritura después. Por
  eso se decide hoy.
- **Fuera de alcance**: migrar las 27 rutas existentes de `RequireRole` a permisos y cambiar el
  enum `user_role`. Ver *Hallazgos fuera de alcance*.

### D-17 · Descuento con pagos hechos **[rev]**

- `SetDiscount` rechaza aplicar o cambiar el descuento de un pedido que ya tiene pagos: «Ya hay
  pagos; el descuento se pone antes de cobrar». Hoy lo permite si el total no baja de lo pagado.
- **Por qué**: con pagos por productos ya hechos, cambiar el descuento reescribe el monto de lo
  pendiente y el último pago absorbería un residuo que nadie vio.

### D-18 · Textos para quien opera **[rev]**

Todo rechazo tiene su texto, y ninguno cae al mensaje genérico. Los de
[contracts/api.md](./contracts/api.md) ya están reescritos: dicen qué tocar, usan «producto» y no
«renglón», y no nombran roles. El toast «Renglón quitado» pasa a «Producto quitado». Los motivos
de quitar se unifican en una sola lista: Ya no lo quiere · Se capturó de más · Sin insumos · Se
equivocó el pedido. Sin preselección y sin texto libre.

## Hallazgos fuera de alcance (no se arreglan aquí; quedan para el dueño)

- Un pedido cobrado, devuelto y vaciado sigue respondiendo «Tiene pagos» al cerrarlo: lo pagado
  (`SumOrderPayments`) no resta devoluciones, así que `CancelPending` lo ve con dinero. Falla hacia
  lo seguro —no se cierra un pedido con pagos sin rastro— y lo cierra quien tiene `orders.cancel`.
- Los reportes no restan `order_refunds`: una devolución hecha con `Devolver` sigue contando como
  venta y su pago como ingreso del método. `RefundsByDay` solo ve `status='reembolsada'`, que ese
  camino ya no pone.
- `RepartirDevolucion` reparte contra lo cobrado bruto por método, sin restar lo ya devuelto.
- `OrdersService.Refund` solo lo llaman los tests.
- En el corte, una devolución que no es en efectivo no baja el esperado de su método.
- `order_refunds.order_line_id` es `restrict` y puede abortar el borrado en cascada de una empresa;
  las tablas nuevas usan `no action` por eso.
- Las 27 rutas con `RequireRole` y el enum `user_role` siguen por nombre de rol (D-16): son la
  migración pendiente de la puerta de roles.
- «Cancelar pedido» se ofrecía a todos los roles y rebotaba con 403 a quien no tenía el rol. Esta
  feature lo pasa a `orders.cancel` en la ruta y en la tarjeta del tablero; otras pantallas que lo
  ofrezcan siguen como están.
