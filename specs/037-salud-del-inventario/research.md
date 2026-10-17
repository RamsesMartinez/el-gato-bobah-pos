# Research: Salud del inventario

Decisiones de diseño, cada una con lo que se descartó. Base: mapa del código de develop al
2026-10-13 (rama 037, migración más reciente 0090).

## R1. Sin migración: todo se calcula o ya tiene dónde vivir

- **Decisión:** la feature no agrega tablas ni columnas.
  - El **modo de consumo** se deduce de lo que ya existe en `products`: `track_stock` (existencia propia), `recipe_id` con renglones (receta), `type = 'combo'` (paquete) y `composition_status = 'confirmed'` sin receta («No gasta insumos»).
  - El **historial de cambios de modo** va a `audit_events` (spec 033) con una acción nueva.
  - El **traslado de existencia** usa `stock_movements` con `movement_type = 'ajuste'`, que ya existe.
  - El **reporte** es una consulta; no se guarda.
- **Por qué:** un modo guardado aparte se desincroniza del que de verdad usa `ExpandSale`. Ese desacuerdo sería justo un problema de salud que el reporte no vería.
- **Descartado:** una columna `consumption_mode`. Duplica `track_stock` + `recipe_id` y obliga a mantener las dos alineadas.

## R2. «Se vende sin restar» se mide sobre lo que pasó, no sobre la configuración

- **Decisión:** un renglón vendido cuenta como «sin restar» si no dejó **ningún** movimiento `venta` con su `order_line_id`. Un extra vendido cuenta si no dejó movimiento con su `order_line_id` **y** su `modifier_option_id`.
- **Por qué:** cubre sin reglas extra todos los caminos que ya descuentan (receta, existencia propia, paquete, extra obligatorio con receta). Coincide con la columna «Sin descontar» de la auditoría del 13-oct. Los 31 productos de esa auditoría salían de la configuración (también los no vendidos); esos los cuenta «Recetas pendientes» con el criterio de Menú › Recetas (SC-001, aclaración del 13-oct).
- **Costo estimado:** piezas × `current_cost` del producto o del extra, el mismo costo de hoy que muestra el catálogo.
- **Piezas de paquete (descubierto al implementar, 13-oct):** no se pueden medir por sus movimientos. `stock_movements.component_of_product_id` guarda el **paquete**, no la pieza, y un insumo no dice de qué pieza salió. Una pieza vendida se marca si no tiene con qué restar, con la misma regla que `ExpandSale`: sin existencia propia y sin receta con insumos (y sin estar confirmada «No gasta insumos»).
- **Estado NULL (descubierto al implementar):** `composition_status` es NULL en lo nunca decidido; `not (status = 'confirmed' and …)` daba NULL y el `where` descartaba justo eso. Va con `coalesce`, y la prueba lo cubre.
- **Medido (T021, 13-oct):** sobre la copia de producción del 13-oct, como `gatobobah_app` y con 30 días de ventas, la consulta principal tarda 7 ms (12 ms de planeación). Desde el 11-oct marca Shot Expreso, los dos Yopokki, Crema Irlandesa y «Caliente» (este último no está confirmado «No gasta insumos»), lo mismo que la auditoría.
- **Excepciones:**
  - lo confirmado «No gasta insumos» (FR-004);
  - el producto genérico de plataforma (`system_kind = 'platform_unpaired'`), que sale en su propio renglón y no se suma al catálogo.
- **Descartado:** clasificar por configuración («no tiene receta»). Marca productos cuyo extra obligatorio sí descuenta (bebidas con sabor a elegir) y no ve un extra que dejó de descontar.

## R3. Unidades por canal

- **Decisión:** el canal es `orders.delivery_platform_id` (nulo = mostrador). Las unidades son la suma de `order_lines.quantity` de renglones no cancelados, en pedidos no cancelados ni reembolsados, por `business_date`, con el mismo predicado que `ListProductRecipes`.
- Para extras: `order_line_modifiers.quantity × order_lines.quantity`.

## R4. Exportación: la primera del sistema

- **Decisión:** el servidor genera el archivo (`GET /stock/health.csv`) con `encoding/csv` de la stdlib, con BOM UTF-8 para que las hojas de cálculo lean los acentos, y `Content-Disposition: attachment`. La pantalla y el archivo llaman al **mismo** servicio con el mismo filtro (FR-011, SC-005).
- **Descartado:**
  - Armar el CSV en el navegador: serían dos cálculos que pueden divergir.
  - Una librería: la stdlib basta (principio VI).

## R5. Permiso nuevo para ver la salud del inventario

- **Decisión:** `inventory.health_view`, con admin y gerente por omisión (FR-012), siguiendo la regla de la 027: todo control nuevo cuelga de un permiso, no de un rol.
- El cambio de modo mueve existencias, una capacidad nueva sobre una ruta que hoy protege `RequireRole(admin, gerente)`. Para no colgar algo nuevo de un nombre de rol (puerta «roles por empresa» del principio VIII), las rutas `PUT …/composition` de producto y de extra, y sus `confirm`, pasan a `RequirePermission(PermMenuConfigure)`. Las de insumo **no**: preparar un insumo es configurar el almacén, no el menú, y no cambia el modo de ningún producto; se quedan en su grupo hasta que el issue #61 les dé su permiso (`menu.configure`, admin y gerente). Mismo resultado hoy, y es lo que pasará a la base cuando lleguen los roles por empresa.
- **Los grupos del router todavía preguntan por rol** (constitución 1.19.0, «Roles: modelo híbrido»): `/stock` (`router.go:397`) y `/admin/products`, `/admin/modifier-options`, `/stock/ingredients` cuelgan de `RequireRole(admin, gerente)`. Un `RequirePermission` anidado ahí no basta: el rol se sigue exigiendo antes. Por eso las rutas que toca esta feature (`GET /stock/health`, `GET /stock/health.csv` y los `PUT …/composition` y `…/confirm` de producto y de extra) se registran **fuera** de esos grupos, solo con su permiso. El resto de cada grupo se queda como está y se migra cuando alguien lo toque (issue #61).
- **Descartado:** un permiso por cada sección del reporte. Nadie pidió verlas por separado.

## R6. Cambio de modo dentro de la captura de receta

- **Decisión:** `SaveComposition` deja de rechazar receta, combo o «No gasta insumos» en un producto con `track_stock`: en su lugar, la transición la decide `domain.PlanModeChange` (pura). **No** se agrega un modo `own`: la existencia propia ya no se elige (FR-013, decisión del dueño: los revendidos son receta), solo se sale de ella.
  - **existencia propia → receta de un insumo sin existencia:** traslada (`ajuste −N` del producto y `ajuste +N×cantidad` del insumo, ligados por la nota) y apaga `track_stock`;
  - **existencia propia → receta de un insumo con existencia:** la petición trae `actualStock` («¿Cuántas tienes de verdad?», FR-014a); el insumo queda en esa cifra con un `ajuste` por la diferencia, el producto en cero, y se apaga `track_stock`. Sin `actualStock` → 400;
  - **existencia propia → varios insumos, combo o «No gasta insumos»:** cierra la existencia del producto en cero con un `ajuste` y apaga `track_stock`; la pantalla confirma antes (FR-014b);
  - **cualquier cambio de modo:** un evento `product.consumption_mode` en `audit_events`, con antes y después.
- **Concurrencia con ventas (revisión de db-architect):** `LockCompositionEdits` solo serializa capturas entre sí; una venta no lo toma y el trigger de `stock_levels` aplica deltas. Por eso, dentro de la tx y después del candado, se leen las filas de `stock_levels` del producto y del insumo **con `FOR UPDATE`** (una por sucursal) y los ajustes se calculan sobre lo bloqueado, nunca sobre lo que vio la pantalla. La petición manda además lo que la pantalla mostró (`seenProductOnHand`, `seenIngredientOnHand`): si lo bloqueado es distinto, responde 409 y la hoja vuelve a preguntar con las cifras nuevas, porque «¿Cuántas tienes de verdad?» se contestó sobre números viejos.
- **Sin bloqueo mutuo con las ventas (revisión de código, 13-oct):** una venta bloquea las existencias en el orden de su ticket, así que «insumo y luego producto» contra la conversión «producto y luego insumo» era un deadlock, y Postgres cancelaba una de las dos (a veces el cobro). El insumo se bloquea con `FOR UPDATE NOWAIT`: si una venta lo tiene, la conversión responde 409 al instante y la venta sigue. Lo prueba `TestModeChangeNeverDeadlocksWithASale`.
- **Con más de una sucursal:** el traslado y el cierre se hacen por sucursal, sobre cada `stock_levels` del producto. «¿Cuántas tienes de verdad?» se pregunta solo si hay una sucursal con existencia en los dos; con más de una, el servidor responde 409 y la pantalla pide hacerlo en un conteo (038). Hoy ninguna empresa tiene dos sucursales con existencia.
- **Ventas anteriores:** ningún camino reescribe movimientos `venta` (FR-002a, criterio del dueño del 13-oct).
- Todo en la transacción de `SaveComposition`, con `LockCompositionEdits`.
- **Descartado:** un endpoint aparte para el modo. Partiría en dos la decisión de cómo descuenta un producto.

## R7. «Se compra hecho»: atajo de pantalla, no endpoint

- **Decisión:** en la hoja de receta del producto, «Se compra hecho» propone crear el insumo con el nombre del producto, por pieza. Usa `POST /stock/ingredients`, que ya existe, y precarga 1 pieza en la receta (FR-014).
- **Descartado:** un endpoint «convertir a revendido». Haría lo mismo que dos llamadas que ya existen, con su propia validación.

## R7b. Combos con un hueco de varios productos

- **Decisión:** el reporte los señala (FR-002b) leyendo los huecos activos con más de un producto. No se cambia `ExpandSale` ni el POS: el punto de venta no deja elegir el componente de un combo, y lo que el cliente elige se configura como extra ligado a su producto, que ya descuenta.
- **Descartado:** guardar el componente elegido en cada renglón. Sería una pantalla nueva en el POS para algo que el extra ligado ya resuelve.

## R7c. Costo estimado de lo que no se restó

- **Decisión:** piezas sin restar × el costo de hoy del producto o extra (el mismo que muestra Menú › Recetas). Es una cifra para ordenar, no contable: la pantalla la llama «costo estimado» y no la suma a ningún total (constitución III).

## R7d. Nombres de insumo sin duplicar

- **Decisión:** alta y renombre de insumo rechazan un nombre igual a otro de la empresa sin distinguir mayúsculas ni acentos, con una función de normalización en `domain` (la misma que detecta «duplicado» en FR-007). La búsqueda va dentro de la transacción después de un candado por empresa (`pg_advisory_xact_lock` con una llave propia, distinta de la 28028 de `LockCompositionEdits`, para que guardar una receta y dar de alta un insumo no se esperen entre sí), y así dos altas casi simultáneas no pasan las dos.
- **Descartado:** un índice único sobre el nombre normalizado. Producción ya tiene insumos así (es lo que FR-007 marca), y la migración fallaría al crearlo. Primero se corrigen con «Salud del inventario»; el índice puede venir después.
- **Alcance:** afecta también el alta desde el diálogo del gasto, que ya muestra el 409 de nombre repetido.

## R8. Existencias con más de una sucursal

- **Decisión:** «Lo guardado en negativo» lee `stock_levels` por sucursal y muestra la sucursal solo si la empresa tiene más de una. No usa `current_branch_id()`, que con dos sucursales truena con `EGB01`.

## R9. Escala en la tableta

- **Decisión:**
  - cada sección muestra sus primeros 25 renglones y se pagina;
  - «Insumos sin compras registradas» abre como un renglón resumen que se despliega (aclaración del 13-oct);
  - las listas de recetas pendientes no se repiten: «Falta decir qué resta» y «Las cargó el sistema» mandan a Menú › Recetas con el filtro en la URL (`?status=pending` y `?status=review`), que hoy no existe y se agrega.
