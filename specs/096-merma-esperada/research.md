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

**Empaques (decisión del dueño, 2026-11-23)**: un artículo marcado `is_packaging` (vaso, tapa, popote)
no recibe el fallback: solo su propio %. Al preparar no se «pierde» un vaso; la prueba con alguien
nuevo lo vio sumar la merma general y no lo entendió.

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
  `order_id is not null`, en los últimos 90 días (la cancelación que repone resta del consumo; la
  `merma_esperada` de S2 no es consumo);
- real = merma ÷ consumo, redondeada a 1 decimal; consumo ≤ 0 → «sin ventas»; ambos en cero → «sin
  datos».
Incluye los artículos que se unieron a éste, con su factor (R14 H1).

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

Consecuencia que no se ve: la sección «Costos» vive en Ajustes del negocio, que abre con
`business.configure`. Admin y gerente tienen los dos permisos; un rol propio con `menu.configure` y
sin `business.configure` captura el % de productos y artículos pero no llega al general. Es el mismo
caso que ya tiene la sección Inventario con `inventory.configure`; se resuelve dándole el permiso al
rol, no abriendo Ajustes a quien no configura el negocio.

## R7. Recalcular y no tocar el pasado

**Decision**: cada cambio de % llama a `recomputeCosts` en la misma transacción (como guardar una
receta). Las ventas guardan `order_lines.unit_cost` al vender (snapshot): ninguna se reescribe.
Test de integración que vende, cambia el general y comprueba que el renglón vendido no cambió.

## R8. Bitácora

**Decision**: tres acciones nuevas en el grupo de precio: `settings.expected_waste`,
`product.expected_waste`, `ingredient.expected_waste`, con antes y después (nulo = sin captura).

## R9. Migración de S1 — número y tolerancia

**Decision**: **0126**. Al traer develop (2026-11-23) ya estaban la 0121 a la 0124 de las specs 094 y
097, con dos 0123 repetidas (`0123_public_menu.sql` y `0123_stock_movement_transfer_type.sql`): goose
entra en pánico con «duplicate version 123» y la API de develop no arranca. Por la regla «la que llega
después renumera», a la 097 le toca la 0125; esta spec toma la 0126 para no chocar. Solo agrega
columnas nullable y copia `waste_pct` > 0: sin bloqueos largos (763 artículos).

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

**Medición de hoy (T052, 2026-11-23)**: build de producción servido en local (`vite preview`), base
restaurada del 23-nov (gatobobah), 1024×600, CPU 4× más lenta, mediana de tres cargas
([medir-carga.mjs](../../web/maquetas-096/medir-carga.mjs) no se versiona: arnés local).

| Pantalla | Primera pantalla | Lista completa | Renglones |
| --- | --- | --- | --- |
| Existencias | 0.74 s | 1.20 s | 438 |
| Conteo | 0.84 s | 1.35 s | 438 |

Hoy ya cumple la meta de SC-007 en local. Lo que no se mide aquí es la red del local hacia la VM: la
meta se vuelve a medir con la pantalla nueva (T068), contra estas mismas cifras, y la vista sencilla no
puede quedar más lenta que la de hoy.

**Medición con las pantallas nuevas (T068, 2026-11-23)**: mismo arnés, mismo build servido en local y
misma base, dos corridas por vista (la mediana de tres cargas en cada una). El conteo se midió sin
conteo abierto, igual que la cifra de hoy.

| Pantalla | Primera pantalla | Lista completa | Contra hoy |
| --- | --- | --- | --- |
| Existencias, sencilla (tablero) | 0.67 s | 1.15 s | más rápida |
| Existencias, avanzada (tabla) | 0.77–0.82 s | 1.24–1.28 s | +0.03 a +0.08 s, dentro del ruido entre corridas |
| Conteo | 0.78–0.91 s | 1.29–1.41 s | igual, dentro del ruido |

Las dos quedan bajo SC-007 (1 s y 2 s). No hubo nada que atacar: ninguna pantalla pide algo por
renglón (la avanzada del conteo agrega **una** petición, lo esperado, solo para quien configura) y las
consultas nuevas filtran por el almacén del conteo con los índices de la 094. Lo que sigue sin medir es
la red del local hacia la VM.

## R14. Revisión de arquitectura (db-architect y tablet-ui-reviewer, 2026-11-23), ya incorporada

**Base de datos** — veredicto: cambios requeridos, 3 HIGH, ninguno bloquea la 0126 aditiva.

| Hallazgo | Decisión |
| --- | --- |
| H1 Merma real de un artículo unido: el trigger de la 0104 solo redirige movimientos **nuevos**; las ventas y mermas viejas del origen quedan con su id y en su unidad | Se resuelve la cadena de `ingredient_replacements` con `kind = 'merge'` (no `unit_change`: tras un cambio de unidad la ventana del artículo nuevo empieza en el cambio, como dice el spec), multiplicando factores como el trigger, para ventas, cancelaciones **y** mermas; el factor y la cadena se aplican en `domain` (`domain.MergeChain`), la consulta solo trae filas. Test de integración: unir con venta y merma previas y el % del destino es el del conjunto |
| H2 (S2) `merma_esperada` huérfano al cancelar o partir un renglón: `orders.sql` cancela renglón, parte y cuenta ventas filtrando `movement_type = 'venta'`; también `inventory_health.sql` y `shopping_list.sql` | Tareas de S2 que listan cada consulta y un test «cancelar renglón con merma esperada deja el almacén igual». El consumo de la merma real es **solo** `venta` + `cancelacion` (no `merma_esperada`); la spec se corrige |
| H3 La 0126 deja `waste_pct` desfasada si hay regreso a la versión anterior, y su Down pierde lo capturado | El Down copia de regreso `update ingredients set waste_pct = coalesce(expected_waste_pct, 0)` antes de quitar; dice que el % del producto y el general se pierden; tras bajar se recalculan costos (la API anterior recalcula al arrancar). Lo prueba la prueba de migración |
| M1 Unir insumos descarta el % del origen | Regla: si el destino está «sin captura» y el origen tenía %, el destino hereda el del origen; si los dos tienen, gana el destino. Test que la nombra. Ninguna inserción de prueba o del MCP lista `waste_pct` (verificar al implementar) |
| M2 La merma real por renglón del desglose serían N consultas | Una sola consulta agrupada `ingredient_id = any($1)` por `ingredient_id, warehouse_id`. La merma de producto terminado (`item_type = 'producto'`) no suma a ningún artículo. Techo anotado: con miles de mermas al año, índice parcial `(company_id, ingredient_id, recorded_at) where status in ('applied','covered')` |
| M3 (S2) Encender «descontar» durante el traslape de S2 | La bandera solo se enciende cuando ya no corre la versión anterior; el plan de S2 lo dice |
| M4 Números de migración | La prueba de la migración la nombra por archivo; si develop trae otra 0126, se renumera al traer develop (R9) |
| L1 El `update` de la 0126 sube `updated_at` de los artículos copiados | Una ficha de artículo abierta durante la subida recibe un 409 «alguien la cambió» una vez y recarga. Aceptado |

**Puerta (VIII)**: `order_lines` no guarda el % de merma aplicado en cada venta; el costo guardado ya lo incluye. Ningún requisito separa la merma del costo base en ventas pasadas (el reporte de S2 es de cantidades), así que **no se guarda**: separar ese dato en ventas anteriores a cuando se decida guardarlo no será posible. Se dice aquí como lo pide el principio VIII.

**Pantallas** — 4 HIGH a 1024×600, ninguno a 1280×854.

| Hallazgo | Decisión |
| --- | --- |
| Dos «Costo» en la ficha del producto | La pestaña Costo con costo manual o de compra muestra una línea «Este producto tiene su costo a mano: la merma no aplica» y no ofrece el campo; con combo, la suma de sus productos. En General, junto al costo de receta, «Ver cómo se calcula ›» abre la pestaña Costo |
| La tabla de 6 columnas no cabe a 1024 | 5 columnas: artículo (fija), lleva, cuesta, merma (% en negritas y debajo, en pequeño, de dónde vino y la merma real), suma; `overflowX="auto"` y encabezado fijo. Campo de % y las tres cifras en una sola fila |
| El panel del artículo bajo el pliegue | Va inmediatamente después de «Ubicación» (antes de nombres alternativos y Alertas), como en la maqueta A2 |
| La tarjeta de merma real se come el historial | La tarjeta conserva un renglón de 32 px por almacén (texto, no botón) y el aviso en el mismo renglón del almacén («pasa lo esperado»), no en renglón aparte; a 1024×600 el historial conserva al menos 2 renglones (lo mide la e2e) |
| Guardado distinto por pantalla | Ajustes: botón Guardar. Producto: el Guardar del diálogo, y un % inválido marca el punto rojo de la pestaña Costo con `erroresDe()`. Artículo: autoguardado de la ficha (`auto.commit` al salir del campo) con su indicador |
| Selector de vista duplicado (S2) | Se usa `Segmented` con su prop `icon` (ya existe): un solo componente de «elegir entre pocas», con íconos como pidió el dueño |
| `CostBreakdownSheet` mal nombrado | `CostBreakdownTable.tsx`: el desglose vive solo en la pestaña |
| Textos | Origen: «general del negocio», «de este producto», «de este artículo». El 409 del recálculo: «Se está recalculando el costo; intenta en un momento». La sección de Ajustes no repite su título; la línea dice «Sube el costo de lo que preparas…» |
| «Sin capturar / Capturada» suena a base de datos y suma un toque | El dueño eligió y aprobó la A2 con ese segmentado: se queda. Sin cambio |

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

## R15. Revisión de código de la subida 1 (2026-11-23): veredicto consolidado

Corrieron los cinco revisores que aplican al diff (`go-backend-reviewer`, `security-auditor`,
`tablet-ui-reviewer`, `db-architect` y el de lógica del front). Ninguno encontró bloqueantes.
**Veredicto: cambios requeridos, todos aplicados antes del PR #357**, cada uno con su prueba:

| Revisor | Hallazgo | Corrección |
| --- | --- | --- |
| Seguridad | Las 4 rutas nuevas no estaban en `route_gates.golden`: la integración quedaba en rojo | Referencia regenerada: admin y gerente; cajero y mesero sin acceso |
| Seguridad | El desglose entregaba la merma real a quien no ve el almacén | Solo con `inventory.view`; sin él, `realWaste: []` (prueba) |
| Seguridad | El aislamiento solo exigía «error», no 404 | `errors.Is(err, ErrNotFound)` en los tres casos |
| Base de datos | Un regreso de imagen sin Down dejaba `waste_pct` viejo | Trigger espejo en la 0126 (prueba sobre el respaldo) |
| Base de datos | Down sin probar con nulo y check sin código | Prueba del espejo y de `23514` |
| Go | Contrato de respuesta de los PUT | Contrato corregido: decimal como texto |
| Go | Unir insumos heredaba el % sin bitácora | Bitácora `ingredient.expected_waste` con `inheritedFromMerge` |
| Lógica del front | Carrera al tocar «Sin capturar» con el % en camino; borrar el % no guardaba; error sin reintento | Peticiones en fila contra lo último pedido; vacío = sin capturar; error regresa lo guardado (4 pruebas) |
| Lógica del front | Vista previa del desglose distinta del servidor sin cambios | Sin cambio de %, se muestra lo del servidor tal cual |
| Tableta | Encabezado de la tabla no quedaba fijo; dato solo en `title`; título de la tarjeta | `maxH` en dvh; texto visible; `Heading` |

Después: `make ci-local` verde y CI del PR verde. La prueba táctil posterior encontró un defecto más
(el aviso «Guardado» que se borraba), corregido con su prueba en #366 y en producción con el #368.

**`/speckit-analyze`, última corrida (cuarta, 2026-11-23)**: 27 de 27 requisitos con tarea, CRITICAL 0,
HIGH 0, MEDIUM 0, LOW 1 (redacción de FR-010).

## R16. Revisión de código de la subida 2 (2026-11-23), ya incorporada

| Hallazgo | Decisión |
| --- | --- |
| Cancelar un renglón o un pedido reponía la merma esperada como `cancelacion`: el reporte contra ventas leía vendido negativo y la merma real un consumo negativo (el total sí cuadraba) | La reposición conserva el tipo `merma_esperada`; el pedido completo agrupa también por esa clase. Test que lo nombra (`TestCancellingALineGivesBackTheExpectedWaste`) |
| Quien solo cuenta leía lo esperado en la vista previa del cierre (capturar absurdo, leer `system`, recapturar para dejar el faltante en 0). Venía de antes de la 096, pero rompe el conteo a ciegas que promete FR-024/025 | Sin `inventory.configure`, la vista previa y el cierre llegan sin `system` ni `delta` (`blind: true`); sigue pudiendo cerrar. También por MCP, que usa la misma ruta. Test por HTTP con un rol al que la empresa solo le dio contar |
| El aviso de cifra rara sirve de oráculo para sondear lo esperado | **No se cambia**: es el riesgo que el dueño aceptó en la spec 042 (2026-10-19) a cambio de que cada aviso quede en la bitácora (`AuditCountConfirmAsked`) |
| Un conteo que no existe respondía 200 vacío en `/expected` y `/vs-sales` («primer conteo») | 404; `/vs-sales` de un conteo abierto, 409 |
| `CountVsSalesLines` recorre el historial de cada insumo (`sm_ingredient`) | Medido en la base restaurada: 295 insumos con movimientos, 23 en promedio y 300 como máximo por insumo; cuesta milisegundos. Techo: con años de historial, un índice `(company_id, warehouse_id, ingredient_id, id)` creado `concurrently`. No se agrega hoy |
| Durante el traslape de la subida, la API anterior no repone `merma_esperada` al cancelar | La opción nace apagada y solo la enciende la versión nueva: el riesgo es de segundos y solo si alguien la enciende y la versión vieja cancela en esa ventana. Aceptado (constitución IV: se declara aquí) |
| Front: la sencilla sin buscador dejaba a quien solo cuenta sin poder contar en una Bodega nueva; «Saltar» al final de un lugar no se movía; volver a un almacén ya visto podía quedar vacío; `?q=` abría el primero que empezaba igual; el interruptor parpadeaba; el reporte con error se quedaba esperando | Lo que no es del almacén va en un lugar «De otro almacén» al final; el recorrido da la vuelta en el lugar y pasa al siguiente, y avisa «Ya se contó todo»; `key` por conteo; nombre exacto primero; la caché se escribe con lo guardado; mensajes de error. Un test por cada uno |
| Pantalla: el nombre de lo contado se cortaba en la lista angosta; el tablero tenía su alto en píxeles fijos | La cantidad va debajo del nombre; el alto lo da la pestaña |

## R17. Prueba con alguien nuevo de una fonda, subida 2 (T069, 2026-11-23)

Un agente en el papel de cocinera y cajera de una fonda, sin ver el código, con toques reales a
1024×600 contra la API local y la base restaurada. Contó 30 artículos en la vista sencilla **sin
ayuda** (SC-006): 52.5 s y 116 toques medidos con Playwright, de 0.7 a 1.9 s por artículo (tiempo
de máquina; una persona buscando en el anaquel tardaría más, estimado). Cambiar a la avanzada y
regresar conservó los 30 (SC-008). Descartó los conteos que abrió; no cerró ninguno.

| Lo que encontró | Qué se hizo |
| --- | --- |
| «No hay» metía su pregunta en la fila de botones y sacaba el teclado de la pantalla (1024 y 1280) | La pregunta va en una hoja inferior, como el aviso de cifra rara |
| «Cuánto hay» salía cortado a 1024×600 | Menos relleno en la tarjeta debajo de 1280 px; medido, se ve completo |
| Ir a la avanzada y regresar la ponía otra vez en el primer lugar | La sencilla regresa al artículo donde iba |
| El aviso de cifra rara saltó 4 veces en 30 con cifras normales (la regla de FR-024: menos de la mitad o más del doble del conteo anterior) y no saltó con «2 ml» en un artículo que se cuenta en litros con existencia 0 | **Sin cambio**: la regla es la que pide FR-024 y la del 0 es la de la spec 042. Se le avisa al dueño |
| El nombre largo se corta a un renglón a 1024; los lugares se llaman como el negocio los configuró («Sin definir», «Almacén principal») | Se deja: el nombre completo está en la lista y al tocarlo; los nombres de lugar los pone cada empresa |
