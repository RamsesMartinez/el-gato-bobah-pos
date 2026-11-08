# Almacén, recetas y costo

Notas de mecánica de esta área: lo que no se ve en el código ni en la pantalla y ya costó un defecto. Se leen **antes** de tocar existencias, recetas y paquetes, costo vigente, compras, conteo, merma, lugares, artículos, unir insumos, nombres alternativos, densidad. Vienen de `AGENTS.md` (se movieron el 2026-11-07 porque las instrucciones que carga cada sesión pasaron de 150 mil caracteres); los principios siguen en la constitución.

- **El almacén descuenta lo que de verdad se vendió** (spec 028, 0078). Cuatro cosas que no se ven:
  - **Una sola función decide qué sale**: `domain.ExpandSale` (receta o existencias propias, cada
    extra, lo que lleva un paquete, y los insumos preparados por su rendimiento). La usan crear
    pedido, agregar renglones y aceptar un pedido de plataforma, por `descontarRenglon`. **Un camino
    nuevo que venda tiene que pasar por ahí**, o vende sin descontar y nada truena.
  - **Un paquete descuenta con la misma regla que lo costea** (`CostGraph`: el producto por omisión
    de cada hueco × `max(min_select, 1)`). Si cambia una, cambia la otra: si no, el costo y el
    almacén hablan de paquetes distintos.
  - **Cada movimiento guarda su origen** (`modifier_option_id`, `component_of_product_id`) y las
    cancelaciones reponen con el mismo origen. Los componentes vendidos se copian en
    `order_line_components`, que es de donde sale «unidades por producto» en Reportes.
  - **Cancelar repone solo lo que no se gastó** (decisión del dueño, 2026-10-11): lo que se prepara
    y ya salió a cocina se da por gastado y no vuelve; lo que no se ha mandado, sí. Lo que no se
    prepara (una lata) se repone aunque ya se haya mandado. Una sola regla para quitar un renglón y
    cancelar el pedido entero: `domain.ReponeInventario`. Un pedido entregado no repone nada.
  - **«Salud del inventario» mide lo que la venta dejó, no la configuración** (spec 037): un renglón
    «se vendió sin restar» si no dejó ningún movimiento `venta` con su `order_line_id` (y su
    `modifier_option_id`, en un extra). La excepción son las piezas de un paquete: el movimiento
    guarda en `component_of_product_id` el **paquete**, no la pieza, así que una pieza se marca por
    lo que tiene para restar. La pantalla y la exportación salen del mismo `report()`.
  - **Un producto con existencia propia ya puede recibir receta o volverse paquete** (spec 037): su
    existencia pasa al insumo (receta de un insumo) o se cierra en cero (lo demás), con las filas de
    `stock_levels` bloqueadas (`FOR UPDATE`) dentro de `SaveComposition`. El trigger de existencias
    suma deltas sin releer, y sin ese bloqueo una venta simultánea perdería o inventaría piezas. La
    hoja manda las cifras que mostró (`seenProductOnHand`, `seenIngredientOnHand`); si ya no son las
    bloqueadas, 409. La existencia propia ya no se ofrece en pantalla: solo se sale de ella.
  - **Un insumo no se da de alta con un nombre que ya existe** sin distinguir mayúsculas, acentos ni
    espacios (`IngredientNameTaken`, con candado por empresa 28037). El gasto y la receta reciben el
    que ya existe (`web/src/shared/createIngredient.ts`). Hoy ninguna ruta renombra insumos; la que
    se agregue aplica la misma regla.
  - **Corregir una receta no recalcula lo ya vendido** (decisión del dueño, 2026-10-13): el
    inventario de la empresa en producción es un histórico heredado de FUDO y se corrige de aquí en
    adelante, no hacia atrás. La diferencia acumulada la ajusta un conteo, nunca un script que
    reescriba `stock_movements` de ventas pasadas.
  - **La carga de FUDO** es `make fudo-composiciones empresa=gatobobah` (con `prueba=1` solo
    reporta). Llena solo lo que no tiene composición y lo marca **estimado**, que descuenta desde
    ese momento (decisión del dueño, 2026-10-07). No arma paquetes —sus cantidades en FUDO no
    siempre son piezas por paquete— y rechaza insumos circulares.
  - **Un producto es paquete porque su receta lleva productos (Menú › Recetas, «Es un combo»)**, no por un interruptor
    aparte: capturarle productos lo vuelve `combo` con un hueco por producto, y capturarle insumos lo
    regresa. El POS vende igual un paquete que un producto suelto; un paquete no lleva paquetes ni
    se lleva a sí mismo.
  - **Un insumo preparado se captura en Almacén → Insumos** (qué lleva y cuánto rinde, en su unidad
    base). Los ciclos se rechazan con `StockGraph.ValidatePrepIngredient` contra todo el catálogo,
    en la captura **y** en la carga de FUDO: una sola regla para los dos caminos.
- **El costo vigente (`current_cost` de insumos preparados, productos y extras) es un caché que
  escribe solo el motor** (`recomputeCosts`, con la regla de `domain.CostGraph`). Lo que no se ve
  (issue #145):
  - **Todo camino que cambie algo de lo que sale un costo lo llama dentro de su transacción**: una
    compra, guardar o confirmar una receta (producto, extra o preparado), corregir el costo de un
    insumo y cambiar su unidad. Antes guardar la receta de un extra no lo hacía, y el extra se
    quedaba en $0 o con el costo de su receta anterior: el margen salía mal sin que nada fallara.
  - **Necesita el candado de captura de la empresa** (`LockCompositionEdits`), y **siempre antes
    que cualquier fila**: dos correcciones a la vez escribían cada una su cálculo sin el cambio de la
    otra. Quien no tiene filas tomadas lo pide al empezar su transacción (receta, confirmar, corregir
    costo, cambiar unidad). Quien llega con filas tomadas (la compra, con el almacén del insumo) solo
    lo intenta (`TryLockCompositionEdits`) y responde «vuelve a intentar»: esperarlo con filas en la
    mano se cruzaba con una captura de receta y Postgres abortaba una con `deadlock detected`. Lo
    cubren `TestTwoConcurrentCostCorrectionsDoNotOverwriteEachOther`,
    `TestCorrectingACostWhileARecipeIsSavedDoesNotDeadlock` y
    `TestAPurchaseDoesNotWaitForTheCompositionLockHoldingStock`.
  - **Un costo que no cabe en su columna se deja como estaba** (`domain.StorableCost`, log
    `cost_out_of_range`): con el recálculo en cada guardado, una sola receta absurda dejaba a toda la
    empresa sin poder guardar recetas.
  - **La API recalcula todas las empresas activas al arrancar**, en segundo plano, y solo escribe lo
    que cambió (así no sube la versión del catálogo en balde). Lo que deje un script se corrige en
    la siguiente subida. Las ventas pasadas no cambian: `order_lines.unit_cost` es copia.
- **Unir insumos** (spec 056, 0104). Varios insumos que son lo mismo con otra marca pasan a uno
  (`POST /stock/articles/{id}/merge`, `?preview=1` es la misma transacción revertida, con huella). Cada
  origen queda en `ingredient_replacements` con `kind = 'merge'` y se archiva. Cuatro cosas que no se ven:
  - **Nada nuevo cuelga de un insumo unido, y lo garantiza la base**: un movimiento de existencia que
    llega a un origen (la cancelación de un pedido vendido antes de unir, una pantalla abierta desde
    antes, la API anterior durante la subida) se redirige al destino con la equivalencia
    (`redirect_merged_ingredient_movement`); cualquier otra fila que apunte a un origen se rechaza con
    `EGB02` → 409 `INGREDIENT_MERGED` nombrando el destino. Cada guarda va dos veces: inmediata y como
    constraint trigger diferido, que atrapa la fila que esperó la llave foránea mientras la unión
    confirmaba. **Una tabla nueva que apunte a `ingredients`** va en `app.MergeTables` (y en
    `app.UnitChangeTables`); si la clasificas como «rechaza», lleva las dos guardas o truena
    `TestEveryRejectingTableHasTheMergeGuard`.
  - **`EGB02` puede nacer en `tx.Commit`**: por eso los commits de `store.go` pasan por `DomainError`.
    Sin eso, la venta que pierde la carrera contra una unión respondía 500.
  - **La marca de la presentación se copia en la compra** (`expense_items.presentation_brand`, por
    trigger): editarla después no reescribe de qué marca fue cada compra. La presentación pasa al
    destino conservando su id por la misma razón.
  - **Un destino con insumos unidos no cambia de unidad**: la equivalencia de cada unión habla de la
    unidad de hoy. Unir no reescribe ventas, compras ni conteos pasados; el historial de costos y la
    bitácora del destino leen los de sus orígenes con un CTE recursivo dentro de la consulta (bajo RLS).
- **Nombres alternativos** (spec 063, 0107). `products.alt_names` e `ingredients.alt_names` (hasta
  10, `text[]`) sirven solo para buscar: ticket, comanda, reportes y plataformas leen `name`. Cuatro
  cosas que no se ven:
  - **La llave con que dos nombres son el mismo es `catalog_name_key` (SQL) y `domain.CatalogNameKey`
    (Go)**, gemelas con prueba de paridad (también con `standard_conforming_strings=off`). Las
    expresiones viejas (`IngredientNameTaken`, `FindArticleByName` por nombre) no recortan un tabulador
    en la orilla; lo nuevo usa la función.
  - **El choque se decide bajo el candado de nombres del catálogo, tomado antes de leer**: 28063 para
    productos (alternativos, alta, renombre, duplicado) y el 28037 de siempre para artículos. Un
    alternativo choca con el nombre de cualquiera de su catálogo y con los alternativos de los demás,
    archivados incluidos, salvo los **orígenes de una unión**, que no reservan nada. Un camino nuevo
    que dé de alta o renombre pasa por `productNameFree` o por `FindArticleByName`.
  - **Van por su propia ruta (`PUT …/alt-names`, lista completa), nunca en el `PATCH` de la ficha**:
    una tableta con la pantalla anterior manda el `PATCH` sin ellos y no puede borrarlos. En el
    artículo, guardar mueve `updated_at` (trigger de la 0009) y la respuesta trae la versión nueva.
  - **El cambio de unidad por reemplazo los pasa al artículo nuevo**; duplicar un producto no los
    copia. La carga de FUDO no toma el candado: no trae alternativos.
- **Densidad y tara** (spec 058, 0105). La densidad (g por ml) del artículo es lo único que pasa
  entre peso y líquido; nunca se asume 1 g = 1 ml. Cuatro cosas que no se ven:
  - **Cada renglón de compra fija la densidad con que convirtió la primera vez**
    (`expense_items.density_g_per_ml`, en `applyStockLinesOn` con el valor exacto que usó `BaseQty`).
    `PlanStockLines` recalcula lo que el renglón debió sumar en cada escritura: sin esa copia, rehacer
    la pesada y luego corregir o editar el gasto movería la existencia (decisión del dueño,
    2026-11-02: «de aquí en adelante»). `ListStockLines` lee `coalesce(fija, vigente)`; cambiar el
    artículo del renglón la vuelve a nulo.
  - **Quitar la densidad se niega con una compra pendiente entre peso y líquido** sin densidad fija
    (409 `DENSITY_IN_USE`): `PlanStockLines` convierte también lo pedido y el gasto entero dejaría de
    poder tocarse. Durante la subida, la API anterior responde 400 a ese gasto (anular sí funciona).
  - **«Solo g o ml tiene densidad y tara» lo impone la base**: pasar un artículo a pieza o cm limpia
    su densidad y las taras de sus empaques (trigger en `ingredients`), y un empaque que llega a un
    artículo en piezas (una unión) pierde la tara en vez de tronar.
  - **Entre peso y líquido la equivalencia del cambio de unidad la pone la densidad**
    (`domain.UnitChangeFactor`); un factor distinto del derivado es 409 `DENSITY_CHANGED`. El conteo
    pesando con empaque guarda lo pesado (`stock_count_lines.gross_g`, empaques, tara y densidad
    usadas) y responde 409 `COUNT_INPUTS_CHANGED` si la tara o la densidad ya no son las que usó la
    pantalla.
- **Compras al almacén, por recibir y conteo** (spec 038, 0091/0092). Cinco cosas que no se ven:
  - **El almacén se mueve por diferencia, nunca editando un movimiento.** Cada renglón de compra
    guarda en `applied_qty_base` lo que ya sumó; recibir, corregir, quitar (`voided_at`) y anular
    calculan lo que debería haber sumado e insertan solo la diferencia (`domain.PlanStockLines`).
    Un camino nuevo que toque lo recibido pasa por `applyStockLines`, o duplica o pierde mercancía.
  - **Los insumos se toman sin esperar, ANTES de escribir** (`lockIngredients`, `for no key update
    nowait`): la llave foránea del renglón tomaría el insumo y esperaría a una venta. Una venta en
    curso responde 409 «vuelve a intentar», no una espera. `no key update` y no `update`: así la
    compra no frena la llave foránea de los movimientos de la venta.
  - **`ValidQty` rechaza el cero**, y lo recibido («todavía no llega») y lo contado («no hay») lo
    usan a menudo: para eso está `domain.ValidStockQtyOrZero`. Usar `ValidQty` ahí responde 400.
  - **El conteo ajusta contra lo que había AL CAPTURAR**, no al cerrar: lo vendido mientras se cuenta
    queda restado. Lo no contado no se toca. Mientras está abierto, lo que el sistema tiene no viaja.
  - **El «lugar» de un insumo no es un almacén**: solo ordena el conteo. Más de un almacén por
    sucursal es una puerta del principio VIII (issue #65). Las compras todavía no guardan sucursal:
    con dos y sin selector, «Por recibir» responde `BRANCH_AMBIGUOUS`.
- **Lo que se echó a perder** (spec 039, 0094). Cuatro cosas que no se ven:
  - **El periodo del faltante se cuenta por id de movimiento, no por hora.** Al capturar un artículo,
    el conteo toma su existencia (`for share`) y DESPUÉS, en otra sentencia, lee lo que había y el
    último movimiento (`last_movement_id`). En una sola consulta, Postgres relee la fila esperada pero
    el resto sale de la foto vieja, y una merma que confirmaba a media captura se restaba dos veces.
  - **La merma registrada YA restó de lo que el sistema tenía**: lo que el conteo encuentra de menos es
    lo sin explicar tal cual (`domain.SplitLoss`). Restarle otra vez la merma la cuenta dos veces.
  - **Juntar motivos no reescribe `reason_id`**: el reporte sigue `merged_into_id` (un nivel). La marca
    «no cuenta como pérdida» se copia al registro; cambiarla en el motivo no reclasifica el pasado.
  - **Las horas de la merma son de la base** (`recorded_at`, `DBNow`): se comparan con `captured_at`
    del conteo, que también pone la base. Con el reloj de la API se mezclarían dos relojes. Y
    `CreateCompany` siembra los motivos: una prueba que baje el esquema debajo de la 0094 crea sus
    empresas ANTES de bajarlo.
- **Lugares para contar** (spec 040, 0095). Tres cosas que no se ven:
  - **El lugar es una liga a `count_places`, no texto.** Ordena el recorrido del conteo y nada más:
    no es un almacén (#65) y no guarda existencias. La pantalla no lo llama «almacén».
  - **Crear, mover y archivar toman primero `pg_advisory_xact_lock(28040, empresa)`** y después, en
    otra sentencia, leen los activos y renumeran 0..n-1. Asignar toma el lugar `for share` y archivar
    `for update`: así ningún artículo queda en un lugar archivado. Un camino nuevo que toque el orden
    o archive pasa por el mismo candado.
  - **La normalización de nombres junta espacios con `strings.Fields`, no con `\s` de regexp**: el de
    Go no ve el espacio duro y el de Postgres sí (lo encontró la prueba de paridad de la 0095; también
    afectaba a los motivos de merma). La Down de la 0095 pierde el orden y los lugares sin artículos.
- **Límites del conteo** (spec 044, 0098). Tres cosas que no se ven:
  - **Los defaults viven en la columna de `count_limits`, una sola vez**: el relleno y la siembra de
    `CreateCompany` solo pasan el id, y una prueba compara la fila con `domain.DefaultCountLimits`.
    Cambiar un default es una migración que cambia la columna, no una constante en Go.
  - **Solo el conteo los usa**: compras, recetas y ajustes siguen con `MaxStockQty`. El máximo de una
    empresa puede pasar de ese tope (hasta `MaxCountLimit`, 1e9 en unidad base).
  - **Leer nunca escribe**: una empresa sin fila (respaldo viejo, alta a mano) recibe los defaults del
    dominio; la fila se crea al guardar.
- **Artículos del almacén** (spec 042, 0096). Cinco cosas que no se ven:
  - **Un artículo nace solo dentro de un guardado**: la lista (`createArticleTx`), el gasto y la receta
    (`newArticle` dentro de su transacción). Nada lo crea al tocar una unidad; cancelar o fallar no
    deja artículos sueltos. `POST /ingredients` ya no existe (#115): toda alta pasa por `createArticleTx`.
  - **La unidad base solo la reescribe `SetArticleUnit`** (cambio de unidad), y lo vigila
    `TestNoQueryRewritesTheBaseUnitOfAnIngredient`. Sin compras, ventas ni mermas el cambio es directo
    y reescribe ajustes, renglones de conteo, recetas, empaques y correcciones con los originales en la
    bitácora (`ingredient.unit_changed.detail`, un `batchId`); con ellas, se reemplaza por otro artículo
    ligado en `ingredient_replacements` y el viejo se archiva. Toda tabla que apunte a `ingredients` va
    en `app.UnitChangeTables` o truena `TestEveryTableThatPointsToAnArticleIsClassified`.
  - **Tras convertir, la existencia es la suma de los movimientos** (`RecomputeArticleStock`): redondear
    cada ajuste por separado deja residuo y el trigger de existencias es solo de insert.
  - **El aviso del conteo lo decide el servidor y solo si la pantalla manda `confirmable`**: una tableta
    con la pantalla anterior guarda como antes. Responde 409 `NEEDS_CONFIRM` sin decir la referencia
    (`domain.CountNeedsConfirm`): el conteo sigue a ciegas.
  - **Las sugerencias de empaque son por clase de unidad** (`PresentationSuggestions`), nunca por nombre
    suelto: así «Bolsa de 15 kg» no aparece en un artículo en litros.
- **Productos con existencia propia en el conteo y «Lo restan sus opciones»** (spec 069, 0110). Cinco
  cosas que no se ven:
  - **Sus cifras van en `stock_count_product_lines`, no en `stock_count_lines`**: ahí `ingredient_id`
    es parte de la llave y la API anterior escribe con `on conflict (count_id, ingredient_id)`. Solo
    entran los productos activos con `track_stock` que no son paquete (`domain.CountsOwnStock`), en
    piezas enteras (`domain.ValidCountPieces`). La pantalla los pinta con id negativo para reusar la
    hoja de captura; el servidor nunca ve ese negativo.
  - **Capturar asegura la fila de existencia antes de tomarla** (`EnsureProductStockLevel`): un
    producto que nunca se movió en la sucursal no la tiene, y sin fila el `for share` no toma nada.
  - **El cierre toma las existencias de los productos contados ANTES de leer su modo** (`nowait`): un
    cambio de modo en curso las tiene tomadas y el cierre responde «vuelve a intentar». El producto
    que dejó de contarse con el conteo abierto se salta (`skippedProducts`): ajustarlo le reviviría
    el contador.
  - **`unit_cost` nulo en un conteo cerrado = «no se ajustó»**: lo llena solo la API nueva al cerrar.
    Así se reconocen los renglones que cerró la API anterior durante una subida y los saltados.
  - **«Una opción resta» tiene una sola definición: `option_deducts` y `product_options_deduct`
    (0110)**, que usan la validación al guardar, la hoja de composición y Recetas. El modo `options`
    se deduce del catálogo de hoy: si alguien apaga la última opción que resta, el producto se lee
    «No se cuenta» sin que nada falle (Salud del inventario lo hace visible si vende sin restar).
    Ninguna regla depende de nombres de productos, grupos o categorías.
