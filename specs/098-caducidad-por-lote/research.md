# Research: Caducidad por lote (098)

## R1 — Dónde vive la regla de consumo: un trigger de `stock_movements`

- **Decisión**: la asignación a lotes la hace un trigger `AFTER INSERT` sobre `stock_movements`
  (`trg_stock_movement_lots`), que corre después de `trg_stock_movement` (orden alfabético de Postgres
  para triggers del mismo momento) y antes de `trg_stock_movement_notify`.
- **Por qué**: hay 15 consultas que insertan movimientos (venta, cancelación, compra, corrección,
  merma, deshacer merma, conteo, «se acabó», traspaso, unión, reemplazo, existencia inicial,
  compensación). Ponerlo en Go obliga a que cada una se acuerde («el hermano que no se movió») y deja
  a la versión anterior de la API, durante la subida, descontando sin lotes. En la base, toda salida
  pasa por la misma regla (FR-024).
- **Orden de candados**: el trigger de existencias ya hizo `insert … on conflict do update` sobre la
  fila de `stock_levels` (almacén, artículo), que queda tomada hasta el fin de la transacción. Todo lo
  que toca `stock_lots` de ese almacén y artículo lo hace después de esa fila, así que dos escritores
  se serializan en ella y no pueden cruzarse. Un camino que toque lotes sin movimiento (cierre de
  conteo con desglose, unión) toma primero la fila de `stock_levels` (ya lo hacen con `nowait`).
- **Alternativa rechazada**: asignar en Go en `descontarRenglon` y compañía: 15 lugares, y la API
  anterior quedaría fuera.

## R2 — El lote es (almacén, artículo, fecha); lo sin fecha se deriva

- **Decisión**: `stock_lots` guarda solo lo fechado, una fila por almacén + artículo + fecha. Lo sin
  fecha es `stock_levels.on_hand − Σ lots` y no se guarda.
- **Por qué**: así la existencia total no cambia de significado (FR-003), la API anterior sigue igual,
  y no hay dos números que se puedan desincronizar: un movimiento que la regla no asigna a lotes queda
  automáticamente en «sin fecha». «Dos lotes con la misma fecha» se resuelve por construcción
  (`on conflict` suma).
- **Ningún lote negativo**: `check (qty >= 0)`. Una salida que pide más de lo que tiene un lote lo deja
  en 0 y el resto cae a «sin fecha» (que puede quedar negativo). Las filas en 0 se conservan (la
  reposición las necesita); las listas filtran `qty > 0`.

## R3 — Orden: sin fecha primero, luego por fecha, nunca caducado

- **Decisión**: una salida sin lote indicado toma primero de «sin fecha» mientras sea positivo, luego
  de los lotes con `expires_on >= hoy del negocio` por fecha ascendente (y por id ante empate, que no
  ocurre porque la fecha es parte de la llave). Lo que no alcanza queda en «sin fecha» negativo.
- **Por qué sin fecha primero**: hoy toda la existencia es sin fecha y es la más vieja; y si la regla
  tomara primero lo fechado, el lote se vería más chico de lo que es en el estante y el aviso de
  caducidad se quedaría corto. Odoo hace lo contrario (sin fecha al final); aquí el error seguro es
  sobreestimar lo que va a caducar.
- **«Hoy del negocio»**: `business_today(company)` = `(now() at time zone business_settings.timezone)::date`
  con `America/Mexico_City` por omisión (patrón de la 0062). Caduca el día D = bueno todo el día D.

## R4 — Qué hace el trigger con cada movimiento

| Movimiento | Regla |
| --- | --- |
| Producto (no artículo) | Nada |
| Con `replacement_id` (unión, reemplazo) | Nada: los lotes los mueve el servicio (R7) |
| Con `expires_on` (compra fechada, su corrección y reubicación, merma desde Caducidades, compensación de un renglón fechado) | Directo a ese lote: positivo suma (crea el lote), negativo resta hasta 0 |
| `cancelacion` positiva | Repone lo que las ventas del mismo origen (pedido, renglón, extra, paquete, almacén) tomaron de lotes y aún no se repuso, lo último primero; el resto, sin fecha |
| `merma` positiva con `waste_record_id` (deshacer) | Igual, con las mermas del mismo registro |
| `traspaso` positivo | Crea en el destino lo que la pata de salida (mismo `transfer_id` y artículo) tomó de cada lote, con su fecha |
| `ajuste` de un conteo con desglose | Nada: el cierre fija los lotes (R6) |
| Cualquier otro negativo | Orden de R3 |
| Cualquier otro positivo | Nada (suma a sin fecha) |

Cada toma o devolución queda en `stock_lot_allocations (movement_id, lot_id, qty)` con signo. Es lo
que permite reponer al mismo lote (FR-023) y llevar fechas en el traspaso (FR-030). Solo existen
filas para artículos con lotes: el resto del inventario no paga nada.

- **Atajo (corregido en R12 A1)**: un negativo sin fecha sale si el artículo no tiene ninguna fila en
  `stock_lots` en ese almacén (sin filtrar cantidad). Una reposición (`cancelacion`, `merma` positiva) y
  un traspaso positivo deciden con un `exists` sobre las asignaciones del origen (el pedido y renglón,
  el registro de merma o la pata de salida), nunca sobre los lotes del almacén destino. Un positivo
  sin fecha ni origen sale sin consulta. Un pedido de 90 movimientos sin lotes paga 90 `exists` por
  índice que empieza por `company_id`.
- **Lo sin fecha antes del movimiento (R12 M2)**: el trigger corre después del de existencias, que ya
  sumó el movimiento; lo sin fecha se mide como `on_hand − new.quantity − Σ lotes`.
- **La fecha se deriva de su origen (R12 A2)**: si el movimiento no trae `expires_on`, toma la de su
  fila fuente: `waste_records.lot_expires_on` por `waste_record_id`, `expense_items.expires_on` por
  `expense_item_id`. Una fecha explícita gana (así el par de reubicación por cambio de fecha funciona).
  Con eso la versión anterior de la API, que no conoce la columna, aprueba una merma de un lote o
  corrige un renglón fechado sobre el lote correcto.

## R5 — Aviso «lo que queda está caducado»

- **Decisión**: kind nuevo `expired_left` en `notifications`, uno abierto por almacén y artículo
  (índice `notifications_one_open_per_warehouse` ya existe). Se abre cuando, tras un movimiento que tocó
  lotes, la existencia buena (`on_hand − Σ caducados`) es < 0 y hay caducados > 0. Se cierra
  (`resolved_reason = 'expired_cleared'` o `'restocked'`) cuando deja de cumplirse. Va en su propio
  `begin … exception` como el de existencias: nunca tumba una venta.
- **Sin correo** (`email_state = 'skipped'`): el dueño pidió la campana; el issue deja fuera avisos
  externos.
- **Lo que no detecta**: un lote que caduca a medianoche sin ninguna venta después; se abre con la
  siguiente salida. La vista de Caducidades lo muestra desde el primer minuto.

## R6 — Conteo con desglose

- **Decisión**: `stock_count_line_lots (count_id, ingredient_id, expires_on, qty)`. La captura reemplaza
  el desglose del artículo completo. Al cerrar, después de insertar el ajuste, para cada artículo con
  desglose: los lotes de ese almacén quedan en `desglose − lo que salió de ese lote por movimientos
  posteriores a la captura` (los de `stock_lot_allocations` con `movement_id > last_movement_id` de la
  captura y que no son del propio conteo), con piso en 0; los lotes no mencionados, en 0. Luego evalúa
  el aviso de R5. «Lo que salió» es la suma **neta** de asignaciones (una cancelación posterior que
  repuso al lote suma).
- **Las fechas que ya tenía el artículo se ofrecen sin cantidad** (R12 UI-2): así contar con fechas no
  obliga a reteclear lotes conocidos ni enseña lo esperado. Una fecha sin cantidad no cuenta. Sin
  ninguna fecha con cantidad, el artículo no tiene desglose y sus lotes no se reescriben.
- **Límite conocido del traslape**: si la versión anterior cierra un conteo que la nueva capturó con
  desglose, ese cierre no fija los lotes; la diferencia se aplica con el orden de R3. Dura lo que la
  subida.
- **Por qué**: es la misma regla con la que el conteo ya ajusta la existencia («contra lo que había al
  capturar»); la venta durante el conteo no se cuenta dos veces.
- **Validación** en `domain`: cada cantidad > 0, fechas distintas, Σ ≤ contado, fecha entre 2000-01-01
  y hoy + 10 años.

## R7 — Unión y cambio de unidad

- **Unión / reemplazo**: después de mover la existencia, el servicio pasa los lotes del origen al destino
  almacén por almacén con el factor (`on conflict` suma) y deja los del origen en 0. Toma las filas de
  `stock_levels` de los dos artículos en orden de id (R12 M2). El trigger ignora
  los movimientos con `replacement_id` (R4) para no tomarlos dos veces.
- **Cambio de unidad directo**: `stock_lots` y `stock_count_line_lots` entran en `app.UnitChangeTables`
  (se multiplican por el factor). `stock_lot_allocations` no apunta al artículo; se reescala igual para
  que una reposición posterior devuelva en la unidad nueva.
- Las dos tablas nuevas que apuntan a `ingredients` se clasifican en `app.MergeTables` (lo vigila
  `TestEveryTableThatPointsToAnArticleIsClassified`).

## R8 — Fecha en la compra

- `expense_items.expires_on date null`. `applyStockLines` pasa la fecha del renglón a cada movimiento
  de compra. Cambiar la fecha de un renglón ya aplicado inserta el par de reubicación que ya existe
  para el cambio de almacén (`warehouse_relocation = true`): −aplicado con la fecha vieja, +aplicado con
  la nueva. Así el historial de recibido no lo ve y la regla de R4 lo aplica.
- La compensación «no mover» (`InsertCompensationMovement`) lleva la fecha del renglón para deshacer
  el mismo lote.

## R9 — Merma desde Caducidades

- `waste_records.lot_expires_on date null`; el movimiento de merma lleva esa fecha (también al aprobar
  una merma pendiente y al deshacer, que copia la del movimiento original). Una merma de un producto o
  de un paquete no lleva lote.
- Endpoint: el `POST` de merma que ya existe acepta `lotExpiresOn` opcional; solo para un artículo.

## R10 — Consumo diario y proyección

- Consumo = `−Σ quantity` de movimientos `venta` y `cancelacion` del artículo en la sucursal en los
  últimos 30 días / 30 (un traspaso no es consumo, como la lista de compra).
- Proyección en `domain.ProjectLots(undated, lots, dailyUse, today)`, pura: recorre en el orden de
  consumo (sin fecha positivo, luego lotes buenos por fecha) acumulando; para cada lote,
  `finishesInDays = acumulado / consumo`, `daysLeft = fecha − hoy`, `leftover = min(qty, acumulado −
  consumo × (daysLeft + 1))` si es > 0. Con consumo 0: `noRecentUse`, leftover = qty. Un lote
  caducado: leftover = qty y no entra al acumulado. La proyección se hace con los almacenes de la
  sucursal juntos (Bodega alimenta a Local); la lista se filtra por almacén.

## R11 — Pantallas: alinearse con la spec 096

- La subida 2 de la 096 rediseña Conteo (maqueta B, lista del lugar con teclado fijo) y Existencias
  (maqueta B, tablero por estado con cambio de vista con íconos), y no está implementada. Esta entrega
  no rediseña: agrega al conteo de hoy una sección «Fechas» dentro de la hoja de captura del artículo
  (la misma hoja vivirá en la vista sencilla de la 096) y a Existencias un aviso de una línea arriba
  del contenido, que cabe igual sobre el tablero de la 096.

## R12 — Ajustes de la revisión de diseño (db-architect y tablet-ui-reviewer, 2026-11-23)

Base de datos (ya reflejado arriba y en data-model.md):
- **A1** atajo del trigger por tipo de movimiento; pruebas «vender hasta 0 y cancelar» y «traspasar un
  lote con fecha».
- **A2** fecha derivada del origen; el cierre de conteo de la versión anterior queda nombrado.
- **M1** `stock_movements` gana `unique (id, company_id)` (~7.2 mil filas, milisegundos con
  `lock_timeout`) para la llave compuesta de las asignaciones; `on delete no action`.
- **M2** lo sin fecha antes del movimiento; prueba con un `insert` de varias filas del mismo artículo;
  orden de candados por id en la unión; suma neta en el cierre.
- **M3** el resumen de la mañana excluye `expired_left` (el asunto contaba avisos que el cuerpo no
  mostraba), con prueba.
- Bajo: el Down cierra y borra los `expired_left` antes de devolver los checks y va a la lista del
  harness de Downs; sin el índice parcial redundante; grants de las tres tablas.

Pantalla (ya reflejado en plan.md):
1. El selector de caducidad muestra los 12 meses desde el actual y pagina de 12 en 12 (una página por
   año calendario costaba un toque extra de noviembre a marzo); en el conteo deja ir atrás (lo ya
   caducado); al recibir no ofrece días anteriores a hoy, en vez de confirmar (FR-012).
2. Las fechas conocidas se ofrecen sin cantidad (ver R6).
3. El campo activo del teclado se marca con borde; el error «las fechas suman más de lo contado» es
   del artículo, no bloquea el cierre.
4. Una merma pendiente de aprobación se ve en el renglón («Merma pendiente de aprobación») y apaga «A
   merma»: la vista trae lo pendiente por lote.
5. «Caduca…» compacto en la misma línea del renglón de entrega y solo con cantidad > 0; el botón del
   encabezado pasa a «Día de llegada por renglón».
6. En el conteo, las fechas son chips en una línea (tocar reabre el selector), «quitar» de 44×44
   aparte, y la lista tiene su propio scroll (`flex=1 minH=0`). La vista avanzada de la 096 no lleva
   desglose.
7. Caducidades con vacío («Aún no hay fechas de caducidad. Regístralas al recibir una compra o al
   contar»), carga y error con «Reintentar»; los grupos vacíos no se muestran; lo de más de 60 días
   va en «Más adelante», cerrado.
8. La pestaña va después de «Existencias», solo con `inventory.view`; el aviso va a la derecha del
   selector de almacén cuando cabe.
9. Hoja de merma: motivo «Caducado» por omisión en lo caducado; en un lote vigente, sin motivo y
   «Mandar a merma» apagado hasta elegirlo; decimales según la unidad.
10. Grupos: «Ya caducó», «Esta semana», «8 a 30 días», «31 a 60 días», «Más adelante». «No se ha
   vendido en 30 días» en vez de «sin consumo reciente».

## R13 — Revisión de código (2026-11-23)

Corregido, cada uno con su prueba de regresión vista en rojo:
- El código `LOTS_EXCEED_COUNTED` nunca salía: su caso iba después del `VALIDATION` genérico.
- La campana de la versión anterior del front truena con un tipo de aviso que no conoce: la lista y la
  cuenta solo traen `expired_left` con `withExpired=1`, que manda la pantalla nueva.
- Fechar después un renglón recibido sin fecha y ya vendido en parte metía a lote más que la existencia:
  `relocate_lot` solo pasa lo que queda sin fecha.
- Deshacer una merma de un lote que tomó parte de lo sin fecha devolvía todo al lote: la fecha del
  registro solo se deriva para la salida; la devolución va por asignaciones.
- Conteo: el caché no guardaba el desglose (reabrir enseñaba fechas viejas y una fecha nueva borraba las
  guardadas); «Sí, no hay» y recontar menos que las fechas guardadas chocaban con el servidor; agregar o
  cambiar a una fecha ya puesta dejaba el teclado sin casilla o la fecha repetida.
- Tope de 20 fechas por artículo contado (una captura podía mandar miles).
- Pantalla: «Caduca…» compacto en la hoja de entrega, sin «No trae fecha» cuando no hay fecha, el
  almacén ya no se corta con el nombre en Caducidades, sin la línea de cuentas repetida, sin hueco en
  Existencias con un solo almacén.

Aceptado sin cambio: `stock_movements.expires_on` no la escribe hoy ningún camino (la fecha se deriva del
renglón o de la merma); se deja porque es la vía explícita que las pruebas usan y que un camino futuro
(la merma esperada, un ajuste fechado) puede tomar sin migración. La unión hecha por la versión anterior
durante la subida deja los lotes en el artículo archivado (acotado a la subida).
