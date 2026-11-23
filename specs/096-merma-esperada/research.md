# Research: Merma esperada, conteo contra ventas y rediseño del inventario

## R1. «Sin captura» distinto de 0 % en el artículo

**Decision**: columna nueva `ingredients.expected_waste_pct numeric(5,2)` **nullable** (nulo = sin
captura). La migración la llena con `waste_pct` donde es mayor a 0. El código nuevo solo lee y
escribe la nueva; `waste_pct` se queda en S1 (la versión anterior la lee durante el traslape) y se
borra en la migración de S2, cuando ninguna versión en producción la lea.

**Rationale**: es el cambio aditivo de la constitución IV («renombrar es agregar y quitar»). Volver
nullable `waste_pct` en la misma subida rompe a la API anterior: sqlc la tipó `decimal.Decimal` y
un `NULL` truena el escaneo de `ListIngredientsForCosting` — el 500 sería el recálculo de costos de
la versión vieja.

**Alternatives considered**: bandera `waste_pct_set boolean` junto a `waste_pct` (tri-estado en dos
columnas, se desincroniza si la API anterior escribe un % sin la bandera); convertir los 0 en nulo
sobre la misma columna (rompe el traslape).

Durante el traslape la versión anterior recalcula con `waste_pct`, que tiene los mismos valores, y
con el general vacío: el costo que escribe es el mismo. `fudo-import` pasa a escribir la columna
nueva en S1.

## R2. Dónde aplica el % del producto y el general

**Decision**: el % que se aplica a un renglón es `artículo ?? fallback ?? 0`, donde el fallback es
`producto ?? general` en la receta de un producto, `general` en la de un extra, y **nulo** dentro de
un preparado. Vive en una función pura `domain.ExpectedWastePct(article, fallback *float64)` que S2
reusa para descontar inventario.

**Rationale**: el criterio del issue («5 % general → sube 5 %») solo se cumple si el general no se
compone por nivel. Además el costo de un preparado se guarda por artículo y es compartido por todos
los productos: si dependiera del producto que lo usa, no tendría un solo costo.

**Alternatives considered**: aplicar el general también dentro de los preparados (compone: un
producto con jarabe sube ~10 % con 5 % general, y el costo guardado del jarabe cambia al subir S1,
contra SC-001).

## R3. Caché del motor con fallback

**Decision**: `recCache` se queda para recetas de preparado (fallback nulo); el costo de la receta de
un producto o un extra no se cachea por receta (lo cachea `prodCache` por producto). `recipeCost`
recibe el fallback.

**Rationale**: una misma receta con dos fallbacks distintos daría dos costos; la llave por receta
ya no basta. Medido el recálculo completo de hoy: ~750 productos en < 50 ms (comentario de
`RecomputeAll`); quitar el caché de las recetas de producto no cambia el orden (cada una se calcula
una vez por producto).

## R4. Desglose del costo

**Decision**: `CostGraph.ProductBreakdown(id)` devuelve por renglón de la receta (o por componente
del combo): cantidad base, costo unitario, base = cantidad × costo, %, origen (`article`,
`product`, `general`, `none`) y merma = base × %. Cada importe se redondea a 4 decimales
(`domain.Round4`, el mismo redondeo de `current_cost`) y el total es la **suma de los renglones
redondeados**; la pantalla muestra 2 decimales de esos mismos valores.

**Rationale**: SC-003 pide que la suma de renglones sea el total al centavo; sumar redondeados y
mostrar el total así lo garantiza por construcción, y la diferencia contra el costo guardado queda
bajo 0.0001 × renglones.

El endpoint existente `GET /products/{id}/costing` (con `menu.configure`) crece con el desglose; hoy
ninguna pantalla lo usa (verificado con grep en `web/src`), así que ampliar su respuesta no rompe a
nadie.

## R5. Merma real de 90 días

**Decision**: al pedirla, por artículo y almacén:
- merma = Σ `waste_records.quantity` con `status in ('applied','covered')` y `not is_staff_meal`,
  `recorded_at` en los últimos 90 días;
- consumo = −Σ `stock_movements.quantity` con `movement_type in ('venta','cancelacion')` y
  `order_id is not null`, en los últimos 90 días (la cancelación que repone resta del consumo);
- real = merma ÷ consumo, redondeada a 1 decimal; consumo ≤ 0 → «sin ventas»; ambos en cero → «sin
  datos».
Incluye los artículos que se unieron a éste (`ingredient_replacements` con `new_id` = el artículo):
los movimientos ya se redirigen solos, las mermas no.

**Rationale**: `covered` es merma real que después cubrió un conteo; `pending`, `rejected` y `undone`
nunca restaron. La comida del personal no es pérdida (spec 039). Se usa `warehouse_id` que la 0120
ya guarda en mermas y movimientos; con un almacén, una cifra.

**Índices**: el consumo usa `sm_ingredient (ingredient_id, created_at desc)`; la merma no tiene índice
por artículo (`waste_records` mide cientos de filas). No se agrega índice en S1; S2 lo reevalúa con
la medición de SC-007.

## R6. Permisos y rutas

**Decision**: capturar los tres porcentajes pide `menu.configure` (decisión del #226). Rutas aparte
de la ficha (`PUT .../expected-waste`), porque la ficha del artículo se guarda con
`inventory.configure` y el % es decisión del menú. La merma real del artículo se lee con
`inventory.view` (la ficha ya muestra su costo con ese permiso).

## R7. Recalcular y no tocar el pasado

**Decision**: cada cambio de % llama a `recomputeCosts` en la misma transacción (como guardar una
receta). Las ventas guardan `order_lines.unit_cost` al vender (snapshot): ninguna se reescribe.
Test de integración que vende, cambia el general y comprueba que el renglón vendido no cambió.

## R8. Bitácora

**Decision**: tres acciones nuevas en el grupo de precio: `settings.expected_waste`,
`product.expected_waste`, `ingredient.expected_waste`, con antes y después (nulo = sin captura).

## R9. Migración de S1 — número y tolerancia

**Decision**: la siguiente libre al traer develop: **0121** (develop trae hasta la 0120 de la spec
094). La 094 reservó 0121 para su subida 2: la que llegue después a develop renumera (instrucción del
dueño). Solo agrega columnas nullable y copia `waste_pct` > 0: sin bloqueos largos (763 artículos).

## R10. S2 — conteo vs ventas

**Decision**: el periodo de un conteo cerrado de un almacén es desde el conteo cerrado anterior del
mismo almacén (ancla por id, como el faltante de la spec 039). Por artículo: inicial (contado
anterior), entradas (compras + traspasos entrantes + ajustes positivos), ventas (−venta +
cancelación), merma esperada descontada (`merma_esperada`), merma registrada, traspasos salientes,
esperado = inicial + entradas − todo lo demás, contado, diferencia = contado − esperado, % =
diferencia ÷ ventas. Se calcula en `domain` (puro) sobre las sumas de una consulta agrupada por
artículo y tipo de movimiento.

Sugerencia de %: tres conteos cerrados con ventas > 0, diferencia negativa entre 2 y 4 % en los tres
y rango (máx − mín) ≤ 1.5 puntos → sugiere el promedio redondeado a 0.5. Mayor a 10 % o con rango
mayor → «revisar receta o captura».

## R11. S2 — la merma esperada descuenta inventario

**Decision**: `business_settings.expected_waste_deducts boolean not null default false`; valor nuevo
`merma_esperada` en `stock_movement_type` (aditivo). Con la opción activa, la venta agrega por
artículo un movimiento `merma_esperada` = cantidad de receta × % (precedencia de R2) en el almacén de
venta; la cancelación con reposición repone por `order_id` todos los movimientos de la venta,
incluidos éstos. La opción se lee en la misma transacción de la venta.

## R12. S2 — vista sencilla y avanzada

**Decision**: lo capturado del conteo ya vive en el servidor (`stock_count_lines`, autoguardado por
renglón): cambiar de vista es solo cambiar qué columnas y acciones se ven, así que nada se pierde
(SC-008) y sobrevive a F5. La vista elegida se recuerda por equipo (`localStorage` con try/catch);
quien tiene `inventory.count` pero no `inventory.configure` (el que hoy administra lugares y cierra con ajustes) entra siempre a la
sencilla y el servidor no le manda lo esperado (conteo a ciegas real, no escondido en la pantalla).

## R13. S2 — tiempo de carga

**Decision**: primero se mide (tarea propia) en el build servido en local con la base restaurada, a
1024×600 con CPU 4×, y se registra la cifra de hoy en este archivo. Después se ataca lo que la
medición señale, en este orden de sospecha: (1) una petición por artículo o por lugar, (2) la lista
completa pintada de golpe sin paginar ni virtualizar, (3) consultas sin índice que empiece por
`company_id, warehouse_id`. Meta: SC-007. No se fija la técnica antes de medir.

## Otros POS

Investigado el 2026-11-23 en las ayudas oficiales. **No verificado** (fuente caída o sin
documentación pública): MarketMan (solo su blog), Lightspeed O-Series (403), Craftable (solo
resultados de búsqueda), ciclos de conteo de Toast Retail (404), Soft Restaurant (nada público).
**Ninguna** ayuda publica una cifra de tiempo de carga: la meta de SC-007 se fija contra la medición
propia, no contra un competidor.

| Producto | Conteo | Real vs teórico | Merma esperada en costo |
| --- | --- | --- | --- |
| Toast Inventory / xtraCHEF | Lista por zona de guardado; app móvil; cada presentación se cuenta aparte. La hoja se arma con un permiso y se cuenta con otro | Variación = consumo real − teórico por receta; la merma se registra aparte; avisa «producto sin receta» | «Yield %» por ingrediente (100 % por omisión) y uno por omisión en el producto que la receta puede ajustar; solo costo |
| Restaurant365 | Por zona con % contado; no cierra hasta 100 %; varios contadores en tiempo real; **no muestra lo esperado**; avisa contra el conteo anterior («menos de la mitad», «más del doble»); plantillas diaria/semanal/mensual | Begin, Purch, Trnsfer, End, Actual, Theo, Var, **Waste**, **UnExpl Var**; % sobre la venta; tope de variación aceptable por categoría con la pérdida en rojo | No encontrado |
| Square | Completo o cíclico (cada variante al menos cada 90 días); varios equipos; cualquier unidad convertida | Ordena por variación en cantidad y en costo | No encontrado |
| Apicbase | Subconteos con total combinado; por paquete o «parte»; **ocultar la existencia vigente** es ajuste del administrador; lo no contado conserva su existencia | — | No encontrado |
| Craftable | Zona y número de anaquel por artículo; app sin conexión; auditoría completa o parcial | — | No encontrado |
| Loyverse | Completo o parcial por categoría o proveedor; **sí** muestra lo esperado | — | No encontrado |

Fuentes: support.toasttab.com (Get-Started-with-Toast-Inventory, xtraCHEF-Inventory-Counts,
xtraCHEF-Inventory-Analytics, xtraCHEF-Recipe-Yield-and-Usable-Yield); docs.restaurant365.com
(mobile-inventory-counts-overview, inventory-count-screens-mobile, actual-vs-theoretical-analysis,
food-variance-report); squareup.com/help/us/en/article/7401; support.apicbase.com
(counting-your-inventory); support.craftable.com (Taking-an-Audit); help.loyverse.com
(how-work-inventory-count).

**Lo que se adopta y dónde**:
1. *La zona es la hoja de conteo, en el orden del recorrido* → la vista sencilla recorre por lugar
   (ya existen los lugares de la spec 040, por almacén en la 094). FR-024.
2. *Contar en cualquier presentación y convertir* → la sencilla deja elegir la presentación. FR-024.
3. *No mostrar lo esperado al contar; solo en la revisión y con permiso* → conteo a ciegas real en el
   servidor (R12). FR-025.
4. *Avisar contra el conteo anterior* («menos de la mitad», «más del doble»), sin revelar lo
   esperado → aviso en la sencilla. FR-024.
5. *Merma registrada en su columna y «no explicada» aparte, con tope aceptable* → columnas del
   reporte de S2 y la marca «revisar». FR-020, FR-021.
6. *Separar contar de configurar con permisos* → vistas sencilla y avanzada por permiso. FR-025.
7. *% por omisión que el nivel más específico reemplaza* → precedencia artículo → producto → general.
   A diferencia de xtraCHEF, **reemplaza y no se suma** (criterio del #226: 10 % del producto y 5 %
   general dan 10 %).
8. *Avisar falta de configuración* (producto sin receta) → el desglose dice cuando un producto no
   sale de receta. FR-009.

Lo que no se copia: el conteo completo de Square que pone en cero lo no contado (borra existencias
sin que nadie las vea); se mantiene «lo no contado no se toca».
