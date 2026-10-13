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
- **Por qué:** cubre sin reglas extra todos los caminos que ya descuentan (receta, existencia propia, paquete, extra obligatorio con receta). Es la definición que usó la auditoría del 13-oct, y por eso reproduce sus 31 productos (SC-001).
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
- El cambio de modo mueve existencias, una capacidad nueva sobre una ruta que hoy protege `RequireRole(admin, gerente)`. Para no colgar algo nuevo de un nombre de rol (puerta «roles por empresa» del principio VIII), las tres rutas `PUT …/composition` (producto, extra, insumo) y sus `confirm` pasan a `RequirePermission(PermMenuConfigure)` (`menu.configure`, admin y gerente). Mismo resultado hoy, y es lo que pasará a la base cuando lleguen los roles por empresa.
- **Descartado:** un permiso por cada sección del reporte. Nadie pidió verlas por separado.

## R6. Cambio de modo dentro de la captura de receta

- **Decisión:** `SaveComposition` deja de rechazar receta, combo o «No gasta insumos» en un producto con `track_stock`: en su lugar, la transición la decide `domain.PlanModeChange` (pura). **No** se agrega un modo `own`: la existencia propia ya no se elige (FR-013, decisión del dueño: los revendidos son receta), solo se sale de ella.
  - **existencia propia → receta de un insumo sin existencia:** traslada (`ajuste −N` del producto y `ajuste +N×cantidad` del insumo, ligados por la nota) y apaga `track_stock`;
  - **existencia propia → receta de un insumo con existencia:** la petición trae `actualStock` («¿Cuántas tienes de verdad?», FR-014a); el insumo queda en esa cifra con un `ajuste` por la diferencia, el producto en cero, y se apaga `track_stock`. Sin `actualStock` → 400;
  - **existencia propia → varios insumos, combo o «No gasta insumos»:** cierra la existencia del producto en cero con un `ajuste` y apaga `track_stock`; la pantalla confirma antes (FR-014b);
  - **cualquier cambio de modo:** un evento `product.consumption_mode` en `audit_events`, con antes y después.
- **Concurrencia con ventas (revisión de db-architect):** `LockCompositionEdits` solo serializa capturas entre sí; una venta no lo toma y el trigger de `stock_levels` aplica deltas. Por eso, dentro de la tx y después del candado, se leen las filas de `stock_levels` del producto y del insumo **con `FOR UPDATE`** (una por sucursal) y los ajustes se calculan sobre lo bloqueado, nunca sobre lo que vio la pantalla. La petición manda además lo que la pantalla mostró (`seenProductOnHand`, `seenIngredientOnHand`): si lo bloqueado es distinto, responde 409 y la hoja vuelve a preguntar con las cifras nuevas, porque «¿Cuántas tienes de verdad?» se contestó sobre números viejos.
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

## R8. Existencias con más de una sucursal

- **Decisión:** «Lo guardado en negativo» lee `stock_levels` por sucursal y muestra la sucursal solo si la empresa tiene más de una. No usa `current_branch_id()`, que con dos sucursales truena con `EGB01`.

## R9. Escala en la tableta

- **Decisión:**
  - cada sección muestra sus primeros 20 renglones y se pagina;
  - «Insumos sin compras registradas» abre como un renglón resumen que se despliega (aclaración del 13-oct);
  - la lista de «Por revisar» no se repite: se manda a Menú › Recetas con el filtro en la URL (`?status=review`), que hoy no existe y se agrega.
