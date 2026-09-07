# Investigación previa al dashboard de gráficas

**2026-09-05.** Material de decisión para el spec de una pantalla de gráficas que complemente
[Reportes](../web/src/features/backoffice/ReportsPage.tsx). No es un spec ni un plan: es lo que se
midió y lo que se leyó, para que la decisión de qué construir se tome sobre datos y no sobre
intuición.

Cuatro investigaciones en paralelo: qué datos tiene hoy el sistema, qué KPIs usa la industria
restaurantera, qué muestran los dashboards de los sistemas contra los que este compite, y con qué
se dibuja una gráfica en una tableta táctil.

---

## 1. Lo que la auditoría encontró de paso

Tres cosas que no se fueron a buscar: salieron al inventariar qué consultas existen. Las tres están
en producción hoy.

### 1.1 La tabla de utilidad por producto suma más que el recuadro de ventas que tiene al lado

[`ProductMargins`](../server/queries/reports.sql) excluye los pedidos cancelados y reembolsados,
pero **no excluye los renglones cancelados dentro de un pedido vivo**: le falta
`and ol.cancelled_at is null`. El total del pedido sí los excluye
([`RecalcOrderTotals`](../server/queries/orders.sql)) y cancelar un renglón **conserva** su
`line_total` ([`CancelOrderLine`](../server/queries/orders.sql)).

Consecuencia: en cuanto un pedido cobrado tenga un renglón cancelado, la columna "Venta" de
*Utilidad por producto* suma más que el recuadro "Ventas" de la misma pantalla — las dos cifras
están a diez centímetros una de otra en
[ReportsPage.tsx:93](../web/src/features/backoffice/ReportsPage.tsx#L93) y
[ReportsPage.tsx:120](../web/src/features/backoffice/ReportsPage.tsx#L120), y quien las lea no tiene
forma de saber cuál miente.

Es el modo de falla que nombra el principio III de la constitución: *la lista y el resumen de una
misma pantalla se derivan del mismo predicado*. Que sea descuido y no decisión lo confirma su
hermana [`PopularProducts`](../server/queries/menu.sql), que sí filtra `ol.cancelled_at is null`, y
[`SalesCancelledLines`](../server/queries/sales.sql), que existe justo para contar esa merma aparte.

**Medido contra producción el 2026-09-05: cero renglones cancelados en pedidos vivos.** Las dos
cifras coinciden hoy por casualidad, no por construcción. Ningún test cubre el caso —
[reporte_un_solo_periodo_test.go](../server/internal/integration/reporte_un_solo_periodo_test.go)
solo prueba la cota de fechas.

Arreglo: una línea de SQL más su prueba de regresión.

### 1.2 Una consulta de devoluciones escrita, probada y sin endpoint

[`RefundsByDay`](../server/queries/reports.sql) tiene código generado por sqlc y su test de
integración ([refund_test.go](../server/internal/integration/refund_test.go)), pero ningún handler
la expone: `ReportSales` solo devuelve `byDay` y `byMethod`
([handlers_backoffice.go](../server/internal/httpapi/handlers_backoffice.go)). Una serie de
devoluciones por día cuesta un método de servicio y un campo en el JSON.

### 1.3 La columna de descuentos existe y siempre vale cero

`orders.discount_total` está desde [0007](../server/migrations/0007_orders.sql) y no aparece en
ninguna consulta, servicio ni regla de dominio. Cualquier gráfica de descuentos graficaría una línea
plana en cero. No es un hueco del dashboard: es una feature que nunca se construyó, y conviene
decidir si se construye o si la columna se va.

---

## 2. El presupuesto de pantalla cambió

Las tabletas ya no son de 7 pulgadas sino de **10**. En alto de pantalla la diferencia es toda la
conversación: una tableta de 10" en horizontal reporta típicamente cerca de **1280x800** px de CSS
contra los 1024x600 con los que se ha diseñado hasta hoy. Son ~200 px más de alto útil — de una
gráfica y media por pantalla a tres.

Dos consecuencias que hay que resolver **antes** de escribir el spec:

- **La constitución y la suite de Playwright siguen fijadas en 1024x600.** Las pruebas corren a esa
  medida a propósito y la regla de los 44 px vive en la constitución. Si el objetivo sube, se cambian
  los dos juntos, o el spec dice una cosa y las pruebas verifican otra.
- **Falta el número real, no el nominal.** Las pulgadas no dicen el viewport: dos tabletas de 10"
  pueden reportar 1280x800 o 1024x768. Mientras no se mida en el equipo, la recomendación es diseñar
  contra 1280x800 y **mantener 1024x600 como piso** — el dashboard aprovecha el alto extra cuando lo
  hay y no se rompe cuando no.

---

## 3. Lo que el sistema ya sabe

Casi todo el trabajo pesado ya está escrito en SQL.

| Lo que se puede graficar | Estado | De dónde sale |
|---|---|---|
| Ventas y número de pedidos, día a día | Listo | [`SalesByDay`](../server/queries/reports.sql) — ya se pide y solo se usa para sumar los recuadros; la serie llega a la pantalla sin pintarse |
| Cobrado por método de pago | Listo | [`SalesByMethod`](../server/queries/reports.sql), [`SalesTotalsByMethod`](../server/queries/sales.sql) |
| Propinas por día y por persona | Listo | [`TipsByDay`](../server/queries/reports.sql), [`TipsByEmployee`](../server/queries/reports.sql) |
| Utilidad y venta por producto | Listo, con el defecto de §1.1 | [`ProductMargins`](../server/queries/reports.sql) |
| Merma por renglón cancelado | Listo | [`SalesCancelledLines`](../server/queries/sales.sql) |
| Diferencia de arqueo por corte | Listo | `register_session_totals.difference`, columna generada ([0006](../server/migrations/0006_cash_expenses.sql)) |
| Devoluciones por día | Consulta sí, endpoint no | [`RefundsByDay`](../server/queries/reports.sql) |
| Venta por hora del día | Falta la consulta | El dato está en `orders.opened_at`; hay que convertir a la zona del negocio con [`GetBusinessTimezone`](../server/queries/cash.sql) |
| Venta por tipo de servicio y por plataforma, en el tiempo | Falta la consulta | `service_type` y `delivery_platform_id` ya existen; hoy son filtro, no serie |
| Venta por categoría | Falta la consulta, y con salvedad | Se une `order_lines -> products -> categories`, pero el renglón **no guarda copia** de la categoría: recategorizar un producto reescribe el pasado |
| Gasto por día y por categoría | Falta la consulta | [`ListExpenses`/`CountExpenses`](../server/queries/expenses.sql) ni siquiera aceptan rango de fechas |
| Venta de modificadores y extras | Falta la consulta | `order_line_modifiers` guarda nombre, precio y costo snapshot; ninguna consulta la agrega |

### Dimensiones disponibles

| Dimensión | De dónde | Granularidad |
|---|---|---|
| Día de negocio | `orders.business_date` | Día, homogéneo en todo el histórico desde [0062](../server/migrations/0062_fecha_de_venta_del_reloj.sql) |
| Hora | `orders.opened_at`, `order_payments.created_at` | Instante en UTC; convertir a la zona del negocio |
| Tipo de servicio | `orders.service_type` (`mostrador`, `para_llevar`, `domicilio`) | Por pedido |
| Plataforma | `orders.delivery_platform_id` | Por pedido, solo si `service_type='domicilio'` |
| Método de pago | `order_payments.payment_method_id` | Por pago — **1:N con el pedido** |
| Producto | `order_lines.product_name` (snapshot) o `product_id` | Por renglón |
| Categoría | `products.category_id` | Solo "categoría de hoy": no hay snapshot |
| Quién cobró | `order_payments.received_by` | Por pago |
| Quién capturó | `orders.opened_by` | Por pedido — **no distingue tableta**: las dos comparten cuenta |
| Turno / caja | `orders.register_session_id` | Por pedido y por pago |
| Gasto | `expenses` + `expense_categories.financial_group` | Por gasto |
| Movimiento de inventario | `stock_movements.movement_type` | Por movimiento |

**No es dimensión:** `orders.currency` es constante `MXN`; y aunque existe el catálogo `channels`
(pos/qr/online), **`orders` no lo referencia** — no hay canal por venta.

---

## 4. Lo que no se puede calcular, y qué costaría

Esta lista vale tanto como la anterior: evita especificar una pantalla que no se puede llenar.

| Lo que no se puede | Por qué | Qué faltaría |
|---|---|---|
| **Cuánto deja de verdad cada plataforma** | No hay columna de comisión. `delivery_platforms.price_markup_pct` ([0037](../server/migrations/0037_platform_prices.sql)) es el sobreprecio con el que se **vende**, no lo que la plataforma **cobra** | Una columna de comisión por plataforma, o una tabla de liquidaciones |
| Tiempo de preparación | `orders.ready_at` solo se escribe si el pedido pasa por `lista`, y [`CanTransition`](../server/internal/domain/order.go) permite `abierta -> entregada` directo — que es el camino que se usa | Cambio de flujo, no de esquema |
| Comensales, ticket por persona, rotación de mesa | El negocio no tiene mesas ni cuenta comensales | **No aplica.** RevPASH y rotación son KPIs de otro modelo de negocio, no huecos de éste |
| Food cost real y prime cost | Exigen inventario inicial, compras e inventario final. Hay costo unitario por producto, no costeo de receta | Módulo de inventario de insumos |
| Costo de nómina como % de ventas | El sistema no registra horas trabajadas | Módulo de turnos y nómina |
| Canal de venta (mostrador / QR / en línea) | `channels` es catálogo de visibilidad de menú; `orders` no lo referencia | Columna `channel_id` en el pedido |
| Serie de un producto renombrado | `ProductMargins` agrupa por el nombre copiado en el renglón: renombrar parte la historia en dos | Agrupar por `product_id` — cambia la semántica y hay que decidirla |
| Descuentos | Ver §1.3 | La feature completa |
| Merma valorizada por insumo | `stock_movements` tiene `movement_type='merma'` y `unit_cost`, pero ninguna consulta los agrega | Solo la consulta; el dato ya está |

### El hueco más caro

La **comisión de plataforma** es, según la literatura de la industria, la decisión de mayor impacto
financiero disponible para un restaurante que vende por Uber Eats, DiDi y Rappi: con comisiones del
orden de 25-30%, el margen de contribución de un pedido de plataforma puede caer a 0-7%. Sin esa
columna el dashboard puede decir *cuánto* se vendió por plataforma, pero no *si conviene* seguir en
ella — que es la única pregunta que el dueño realmente tiene.

Es una columna y un campo en la pantalla de plataformas. Conviene meterla en el mismo spec.

---

## 5. Contra qué se compite

Se revisó la documentación pública de Toast, Square for Restaurants, Lightspeed Restaurant, Fudo,
Clover, TouchBistro, SpotOn, Parrot, Soft Restaurant y Wansoft. Lo verificable, y lo que no, está
detallado al final.

### El mínimo que tienen todos

1. Recuadros de KPI arriba: ventas netas, número de pedidos, ticket promedio.
2. **Una** serie de tiempo de ventas, con % de cambio contra un periodo de referencia.
3. Tabla de productos más vendidos, rankeada.
4. Desglose por medio de pago.
5. Botones de periodo prearmados — ninguno obliga a escoger un rango libre.
6. Un reporte de cierre diario **separado** del dashboard de tendencia.

El punto 6 ya está resuelto aquí: la pantalla de [Caja](../web/src/features/backoffice/CashPage.tsx)
es exactamente ese reporte de cierre, y el dashboard nuevo **no debe duplicarla**.

### Lo que separa a los buenos

| Qué | Quién | ¿Aplica aquí? |
|---|---|---|
| Comparar contra varias bases a la vez (semana pasada, mismo periodo del año pasado, de hace dos años) | Toast | Todavía no: no hay un año de historia comparable |
| Mapa de calor por hora, pensado para **programar turnos** y no para mirar números | Fudo — el sistema que este reemplaza | Sí, y es la mejor relación valor/esfuerzo |
| Costo de mano de obra en el mismo dashboard que las ventas | Toast, Lightspeed | No: no hay datos de nómina |
| Meta de ventas con % de avance | Soft Restaurant — el único que lo hace | Sí, y es barato: una constante contra una suma que ya existe |
| Consolidado de varias sucursales con desglose | Clover, Revel, Soft Restaurant | Todavía no, pero condiciona el diseño si el sistema va a venderse |

### Lo que los operadores dicen que de verdad usan

El hallazgo más incómodo: el dueño típico revisa **un solo número al día** —las ventas de ayer— y
sigue con su vida. El desglose por canal y por franja horaria existe en todos los productos y es de
lo que menos se abre. Y en las reseñas, lo que los dueños valoran no es la pantalla: es *recibir el
resumen* sin tener que entrar a buscarlo.

Traducido a este spec: la primera pantalla contesta "¿cómo vamos?" **sin un toque**, y todo lo demás
vive un nivel abajo. Un dashboard con nueve gráficas de igual peso es un dashboard que nadie abre
dos veces.

---

## 6. Lo que cambia por ser este negocio

| Qué | Cuánto | Fuente |
|---|---|---|
| Pedidos por día | 4 a 10 | [spec 005](../specs/005-confirmar-antes-de-cobrar/spec.md), [spec 013](../specs/013-la-orden-nace-al-primer-producto/spec.md) |
| Renglones por pedido | 2.2 promedio, 6 máximo | spec 005 |
| Productos en catálogo | ~502 | [platform_price.go](../server/internal/domain/platform_price.go) |
| Cajas que reciben ventas | 1 | spec 013 |
| Tabletas | 2, **compartiendo la misma cuenta** | spec 013 |

Implicaciones directas:

- **Una línea de 30 días es ruido.** Treinta puntos que valen entre 4 y 10 no dibujan una tendencia,
  dibujan una sierra. Barras por día, con su etiqueta legible, sí se leen. La línea empieza a servir
  agregando por semana.
- **El top de productos tiene poca señal.** ~450 renglones al mes repartidos entre 502 productos: la
  diferencia entre el primero y el décimo cabe en el margen de error de una semana rara. Ordenar por
  **margen** vale más que por cantidad, y la ventana tiene que ser mensual o trimestral.
- **"Cobrado por persona" distingue a dos personas**, y las dos tabletas comparten cuenta. Quien
  cobró sí se distingue (`order_payments.received_by`); quien capturó, no.
- **Ningún agregado necesita caché ni tabla materializada.** El costo de este dashboard es de diseño
  de pantalla, no de consulta. Conviene no gastar esa ventaja en complejidad que no hace falta.

---

## 7. Cómo se dibuja

### Lo que la evidencia descarta

- **El pastel y la dona, para comparar.** El ojo compara longitudes mucho mejor que ángulos, y en
  táctil las rebanadas chicas desaparecen. Se salvan solo como foto de parte-del-todo con 4-5
  categorías y el valor escrito encima. Para comparar, barra horizontal.
- **Los medidores y carátulas.** Gastan una cantidad enorme de espacio para un solo número. La misma
  doctrina que los descarta propone el reemplazo: el número grande con un sparkline al lado.
- **Más de tres series en una línea.** El techo general de la literatura son 4-6; en esta pantalla y
  con este público, 3.
- **El mapa de calor de 7x24 como tarjeta chica.** 168 celdas comprimidas dejan de leerse. O se
  agrupa en franjas de 3 horas, o el mapa se lleva su propia vista.

### En táctil no hay hover

Todo el patrón de "acerca el mouse y sale el dato" no existe. La regla que sale de la investigación
es más fuerte que "poner el tooltip con tap": **el número importante nunca vive en un tooltip.** Va
escrito en la gráfica o en el eje, y el toque se reserva para el detalle adicional.

### La librería

Hoy **no hay ninguna** en [web/package.json](../web/package.json) — ni gráficas SVG escritas a mano.
Cualquier camino es una decisión nueva. De las ocho opciones comparadas, dos quedaron fuera por
mantenimiento (Tremor lleva 19+ meses sin publicar y su propia comunidad lo da por abandonado; Nivo,
15+ meses) y las de canvas arrastran un problema concreto: **no pueden leer `var(--chakra-colors-…)`
directamente** y hay que resolverlas con `getComputedStyle` y redibujar a mano en cada cambio de
tema.

| Opción | React 19 | Peso gzip | CSP estricta | Tema Chakra | Mantenimiento |
|---|---|---|---|---|---|
| **SVG propio + d3-scale** | N/A | 5-15 KB | Sin ambigüedad: tú escribes cada línea que toca el DOM | Directo, `var()` en cualquier atributo | N/A |
| **Recharts v3.10.1** | Nativo | ~150 KB | Sí, verificado leyendo su código actual | SVG: acepta `var()`; sin dark-mode nativo | Release cada 1-4 semanas |
| Chart.js 4 | Sí | ~68 KB | Parcial: inyecta un `<style>` salvo que se desactive y sirvas tu propia hoja | Canvas: no lee `var()` | Activo |
| ECharts 5 | Sí | 335 KB completo | Sí en uso normal | Canvas: no lee `var()` | Activo |
| visx v4 | Nativo | Modular, ~10 KB por paquete | Sí | Directo | Irregular; el soporte de React 19 tardó 7 meses |
| Nivo 0.99 | Sí | 143 KB + un paquete por tipo | Probablemente | Directo | **Sin release desde 2025-05** |
| Tremor 3.18 | **No** | Recharts + envoltura | Requiere **Tailwind**, que este proyecto no tiene | No | **Sin release desde 2025-01** |
| Observable Plot | No es React | 67 KB | Sí (atributos SVG, ni caen bajo `style-src`) | Requiere re-montar al cambiar tema | Activo |

**Recomendación: SVG propio con `d3-scale`.** Con 4-10 pedidos al día no hay volumen que justifique
el peso de una librería pensada para miles de puntos; el catálogo que hace falta (línea, barra,
apilada, dona, mapa de calor, sparkline) es exactamente el rango donde armar a mano no cuesta más
que aprender la API de configuración; y el principio VI ya se aplicó en este repo a cosas más
simples que ésta.

**El contra honesto:** no hay tooltip, leyenda, contenedor responsivo ni accesibilidad de regalo —
cada uno se escribe y se prueba. Y si algún día hacen falta zoom, brushing o miles de puntos, se
termina adoptando una librería de todos modos y migrar no es gratis.

Si el criterio cambiara hacia velocidad de desarrollo, **Recharts v3** es la única librería completa
que pasa todos los filtros sin reservas. Su hueco son mapa de calor y sparkline — los dos que más
falta hacen aquí — así que ni eligiéndola se evita escribir SVG a mano.

---

## 8. Lo que se propone especificar

No es el spec: es la propuesta concreta para corregirla antes de que lo sea.

| # | Qué | Por qué |
|---|---|---|
| 1 | **Cómo vamos** — ventas, pedidos y ticket promedio del periodo, cada uno con sparkline y cambio contra el periodo anterior. Sin un solo toque | Es lo único que la investigación confirma que un dueño mira todos los días |
| 2 | **Ventas día a día** — barras, no línea | El dato ya llega a la pantalla y hoy se tira: no necesita ni una consulta nueva |
| 3 | **Mapa de calor por hora** — franjas de 3 horas por día de la semana, no 24 columnas | Dice a qué hora abrir y cuánta gente poner. La única gráfica de Fudo que resuelve una decisión operativa |
| 4 | **Por canal, en neto** — mostrador vs. domicilio propio vs. cada plataforma, con la comisión adentro | Sin la comisión, el dashboard dice cuánto se vendió y no si conviene vender ahí. **Requiere la columna nueva** |
| 5 | **Popularidad contra margen** — la matriz clásica, cortada por el promedio de **cada categoría** y no por un valor fijo, en ventana mensual, con el margen rotulado como aproximado en la propia pantalla | El costo unitario no cubre empaque ni merma: sirve para saber dónde investigar, no como veredicto |
| 6 | **Excepciones** — devoluciones, cancelaciones y diferencia de caja en una sola tarjeta | Un cero es la respuesta correcta casi siempre, y por eso destaca cuando no lo es. Dos de las tres ya están en SQL; a una solo le falta el endpoint |

### Tres decisiones pendientes del dueño

1. **¿La comisión de plataforma entra en este spec o va aparte?** Es lo que convierte al dashboard
   de informativo en útil, pero toca la pantalla de plataformas y no solo la nueva.
2. **¿Se mueve el objetivo de pantalla a 1280x800, o se mantiene 1024x600 como piso?** Cambia la
   constitución y la suite de pruebas, no solo esta pantalla.
3. **El defecto de `ProductMargins` (§1.1): ¿se arregla ya, aparte, o entra con el dashboard?** Está
   latente en producción y es una línea de SQL más su prueba de regresión.

---

## 9. Restricciones que hereda cualquier pantalla nueva

- **El índice de soporte empieza por `company_id`** (`AGENTS.md` §1). Los que ya sirven:
  `orders_company_date_status` ([0042](../server/migrations/0042_sales_index.sql)) y
  `orders_company_status_completed` ([0056](../server/migrations/0056_indice_entregados_por_completado.sql)).
  **No hay índice que empiece por `company_id` en `order_lines` ni en `order_payments`** más allá del
  plano de [0023](../server/migrations/0023_tenant_columns.sql): un agregado por producto o por
  método sobre un rango largo escanea.
- **Un agregado no se une a dos tablas 1:N en la misma consulta.** El patrón ya resuelto es el CTE de
  [`SalesTotalsByStatus`](../server/queries/sales.sql), que pre-agrega `order_payments` por
  `order_id` antes de unir. Cualquier gráfica que cruce método de pago con producto necesita ese
  doble paso, o dos consultas.
- **Cada peso se clasifica una sola vez** (principio III). Toda serie nueva declara en pantalla qué
  incluye y qué excluye, como ya hace [SalesSummaryTiles](../web/src/features/sales/SalesSummaryTiles.tsx)
  con "no entra al total".
- **sqlc no ve `company_id`** en las tablas que alteró [0023](../server/migrations/0023_tenant_columns.sql).
  Nombrarla rompe `sqlc generate`. RLS la aplica sola.
- **Un parámetro malformado se rechaza, nunca cae a un default** (principio V). Ya implementado y
  reutilizable tal cual: [`domain.ResolveRange`](../server/internal/domain/sales.go) con tope de 366
  días, y [`rangoDeReporte`](../server/internal/httpapi/handlers_backoffice.go), que garantiza que
  los tres endpoints contesten el mismo periodo. El dashboard pasa por ahí, no arma el suyo.
- **Nada de `<select>` nativo.** Un selector de métrica o de "agrupar por" va con
  [`Picker`](../web/src/components/Picker.tsx) o con chips como los de
  [`RangoDeFechas`](../web/src/components/RangoDeFechas.tsx).
- **Los ejes y las leyendas no nombran columnas ni endpoints.** El texto es para quien opera.

---

## Fuentes y su grado de verificación

**Medido en este repo y contra producción**: todo lo de §1, §3, §4, §6 y §9. Cada afirmación cita
archivo o migración.

**Documentación pública de los fabricantes** (§5): Toast, Square, Lightspeed y Fudo tienen
documentación de soporte que describe su dashboard. Clover, TouchBistro, SpotOn, Parrot, Soft
Restaurant y Wansoft solo tienen material de marketing: el dato se documenta, el **tipo de gráfica**
casi nunca. De Revel no se encontró documentación técnica pública. **No se localizó ninguna captura
pública del dashboard de Fudo** — lo de §5 sale de su texto de soporte, no de imagen.

**Lo que los operadores usan** (§5): se sostiene en dos blogs de fabricante (Otter, Toast) y en
fragmentos de reseñas de G2/Capterra sin acceso a la cita completa. **Las búsquedas en Reddit no
devolvieron hilos reales.** Es la sección con menos respaldo del documento.

**Literatura de visualización** (§7): Stephen Few sobre el pastel, Tufte sobre data-ink y
sparklines, IBM Carbon sobre paletas y contraste 3:1, Okabe-Ito para color seguro a daltonismo. El
límite de líneas por gráfica es consenso citado en varias fuentes, no un estudio único.

**Versiones y peso de las librerías** (§7): verificados contra el registro de npm y la API de
GitHub, y los tamaños medidos comprimiendo los `dist` reales — no de memoria. El comportamiento
frente a CSP de Recharts y Chart.js se verificó **leyendo su código fuente actual**, no su
documentación ni sus issues viejos.

**Criterio propio, no de fuente**: toda la §8, la recomendación de librería, la de mantener
1024x600 como piso, y la adaptación del menu engineering a ventana mensual por el volumen bajo.
