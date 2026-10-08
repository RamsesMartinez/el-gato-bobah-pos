# Research: Ventas netas y devoluciones a la vista

## R1. De dónde sale el Total

- **Decision**: Total = Σ `byMethod[].total`, donde cada medio ya es «cobrado en el periodo por día del
  pago − devuelto de la venta en el periodo por día de la devolución» (`SalesTotalsByMethod`, 031).
  Se calcula en `domain.NetCollected` y no con otra consulta.
- **Rationale**: SC-001 exige que el Total sea la suma de los medios al centavo; derivarlo de las
  mismas filas lo hace cierto por construcción. Una segunda consulta sería una segunda derivación
  de la misma cifra.
- **Alternatives**: sumar `orders.total − refund_amount` por día del pedido — descartado: viola la
  decisión del dueño (la devolución cuenta en su mes) y suma pedidos sin cobrar.

## R2. «Ventas» y «Promedio» con el Total nuevo

- **Decision**: el conteo sigue siendo pedidos del periodo no cancelados; el promedio pasa a
  rotularse «Ticket promedio» y se calcula sobre el importe de esos pedidos (como hoy). El recuadro
  dice «sin canceladas» y el pie de la tabla «N pedidos en la lista».
- **Rationale**: dividir el cobro neto entre pedidos mezclaría cobros de otros días con pedidos de
  este. El ticket promedio es una cifra de venta, y rotulado así no se confunde con lo facturable.
- **Alternatives**: quitar el promedio — descartado: el dueño lo usa y no pidió quitarlo.

## R3. Por cobrar

- **Decision**: consulta `SalesPending` (+ gemela `…SinFolio`): pedidos del periodo (por día del
  pedido), vivos (no cancelados ni reembolsados, no juntados), con `total − Σ pagos > 0.00`; devuelve
  pedidos y monto. Mismo filtro de tipo que el resumen.
- **Rationale**: un pedido sin cobrar no tiene día de pago; su día es el del pedido, que es el de la
  lista. Mismo predicado de fecha que la lista → la marca «Por cobrar» del renglón y el recuadro
  describen el mismo conjunto.

## R4. Marca de devolución en la lista

- **Decision**: las listas de Ventas agregan `paid` (Σ pagos vivos) y `last_refund_at`
  (máx. `order_refunds.created_at`) como subconsultas correlacionadas. El renglón pinta «Devuelto $X
  · 8 oct 09:08» con `refund` (ya viaja) y «Por cobrar $Y» con `total − paid`.
- **Rationale**: subconsulta correlacionada, no join: pagos y devoluciones son dos 1:N.

## R5. El corte: devoluciones en un solo lugar y propina neta

- **Decision**: `ExpectedByMethodForSession` agrega `refunded_tips` (propina de devoluciones fuera del
  cajón), `drawer_refunded` y `drawer_refunded_tips` (las que salieron del cajón), por medio.
  `ListCashMovements` agrega `is_refund`. En el desglose: «Devoluciones» = solo la parte de venta de
  todas las devoluciones del medio; «Propinas» = propinas − propina devuelta; las salidas de caja que
  son devolución dejan de sumarse a «Salidas de efectivo». El **esperado no cambia**: la salida de
  caja sigue restando por el neto de movimientos del efectivo.
- **Rationale**: cumple FR-007/008 sin tocar lo firmado. La clasificación vive en
  `domain.ClassifyRefunds` para probarla sin base.
- **Totals[].tips**: se muestra neta (tips − propina devuelta del medio). Los cortes cerrados guardaron
  la propina bruta; la neta se calcula en vivo al leerlos, igual que «Cobros de otros turnos».

## R6. Nota del negativo

- **Decision**: un medio del desglose con total < 0 lleva `note`: «Se devolvió dinero de ventas
  cobradas en otro turno». Regla en `domain`.

## R7. Detalle de un turno abierto desde Histórico

- **Decision**: `SessionDetail` de un turno abierto toma `Totals`, `Breakdown` y `Drawer` de la misma
  función que Cajas (`sessionWithExpected`), y conserva el ocultamiento del arqueo ciego.

## R8. Validaciones de frontera

- Pedido de plataforma en mostrador: `domain.ValidPlatformServiceType` en `Create` antes de la base.
- Cobro menor a un centavo: rechazo en `domain` antes de redondear (`$0.005` redondeaba a `$0.01`).
- Nada por devolver: `ValidarDevolucion` dice «ya se devolvió todo lo cobrado» cuando lo que queda es
  0, y la ruta de renglón dice «de ese producto ya no queda nada por devolver», antes de validar el
  monto.

## R9. Presentación

- `money()` muestra dos decimales cuando hay centavos y ninguno cuando es entero.
- Ventas: la fila de medios de pago se integra a la fila de tiles (tras un separador) y los tiles
  bajan su relleno para dejar ≥4 renglones a 600 px.
- Utilidad por producto: `ProductMargins` agrega `uncosted_revenue`; el margen solo cuenta los
  renglones con costo > 0. La pantalla dice «sin costo capturado» si todo el renglón es sin costo.
- Propinas por día: fecha con el formato local de la pantalla.
- Corte: concepto sin `maxW` fijo, lista de devoluciones sin scroll propio, controles a 44 px.

## R10. Lo que cambió con la revisión de arquitectura (db-architect + tablet-ui-reviewer)

Veredicto consolidado: cambios requeridos, sin bloqueante de puertas ni de RLS. Se aceptan todos:

- **El desglose cuadra con el esperado por construcción, y un test lo vigila**: test de integración con
  devolución en efectivo con propina y otra en tarjeta, que afirma fondo + ingresos − egresos =
  esperado del efectivo y falla nombrando el concepto que no cuadra. `is_refund` = `exists (select 1
  from order_refunds r where r.cash_movement_id = m.id)`. Sin índice por `cash_movement_id`: los
  movimientos de un turno son pocos; anotado.
- **Utilidad**: `uncosted_revenue` con la misma expresión prorrateada que `revenue`, filtrada a
  `unit_cost = 0`; `margin = revenue − uncosted_revenue − cost`. `unit_cost = 0` es centinela («gratis»
  y «sin capturar» no se distinguen; es un hecho ya guardado). Los paquetes con costo 0 en su renglón
  salen como sin costo, que es lo cierto: su costo no se capturó en el renglón.
- **Agregados vacíos**: `coalesce` en `SalesPending`, `paid`, y las columnas nuevas del esperado;
  `last_refund_at` nulable a propósito, con test sobre el JSON crudo (`null`).
- **Ventas a 600 px**: el título «Ventas» va en la fila del rango; el orden de los tiles es Total →
  medios (chips compactos) → Por cobrar → separador → resto; las marcas de devolución y por cobrar van
  como segunda línea de la celda de Estado (no crece el renglón). Un test vitest no mide píxeles en
  jsdom; la medida va en la captura a 1024×600 (SC-004).
- **«Por cobrar»** dice a la vista «no entra al total · N pedidos», sin tooltip. El Total lleva la
  nota «cobrado − devuelto» y el promedio «de los pedidos».
- **Corte**: a 44 px también «Editar», el `Collapsible`, «Traspaso», los Input de movimiento y las
  pestañas; «Pagos devueltos» y «Devoluciones» sin scroll propio.
