# Research: 069 — Conteo con productos de existencia propia

## R1. Dónde se guarda la cifra de un producto en un conteo

- **Decision**: tabla nueva `stock_count_product_lines`, gemela recortada de `stock_count_lines`
  (conteo, producto, contado, sistema al capturar, quién, cuándo, último movimiento visto y costo al
  cerrar).
- **Rationale**:
  - `stock_count_lines` tiene `ingredient_id not null` dentro de su llave primaria, y la API anterior
    escribe con `on conflict (count_id, ingredient_id)`. Volverla nulable exige cambiar la llave
    primaria por índices únicos durante una subida con dos versiones vivas (constitución IV).
  - Las columnas de pesada (`gross_g`, `tare_g_used`, `density_used`, `closed_packs`), sus checks y
    triggers, y la guarda de la unión de insumos (0104) hablan de artículos. Mezclar productos
    obligaría a que cada una aprenda a ignorarlos.
  - Con tabla aparte la API anterior no ve nada distinto: ni lee ni escribe la tabla nueva.
- **Alternatives considered**: columna `product_id` en `stock_count_lines` (rechazada por lo de
  arriba); reusar `stock_movements` directo sin renglón de conteo (rechazada: el conteo es a ciegas
  y ajusta contra lo que había al capturar, eso es justo lo que guarda el renglón).

## R2. Ajuste contra lo capturado, y el bloqueo

- **Decision**: misma mecánica que los artículos. Capturar toma la existencia del producto en la
  sucursal `for share` y, en otra sentencia, lee `on_hand` y el último movimiento (spec 039, R2).
  Cerrar toma las existencias de los productos con diferencia `for update nowait`, en orden de id,
  después de las de los artículos; si una venta las tiene, responde «vuelve a intentar».
- **Rationale**: es la regla de `docs/mecanica/almacen.md` y evita esperar en ciclo con una venta.
  `domain.PlanCountClose` ya calcula el ajuste por renglón; se generaliza para recibir renglones de
  producto sin duplicar la regla (un solo cálculo, dos clases de renglón).
- **Alternatives considered**: ajustar contra lo que hay al cerrar (rechazado: borra lo vendido
  durante el conteo, el defecto que la spec 038 cerró).

## R3. Qué productos entran al conteo

- **Decision**: productos activos con `track_stock` y que no son paquete, es decir
  `domain.ModeOf(...) == ModeOwnStock`. Se listan aparte de los artículos (arreglo `products` en la
  respuesta del conteo) porque no tienen lugar ni empaques.
- **Rationale**: la decisión del dueño excluye los que restan por receta; `ModeOf` ya da la
  precedencia correcta (un paquete con `track_stock` puesto se expande por sus piezas).

## R4. Cifra entera y límites

- **Decision**: lo contado de un producto es entero ≥ 0 (`domain.ValidCountPieces`), con el máximo y
  la cifra de aviso de la clase `pieza` de `count_limits`, y la misma referencia del aviso
  (`domain.CountNeedsConfirm` con lo esperado y el último conteo cerrado de ese producto).
- **Rationale**: una lata no se parte; un decimal ahí es un dedazo que movería el valor del
  inventario.

## R5. Producto que deja de tener existencia propia con el conteo abierto

- **Decision**: al cerrar, el plan solo toma renglones cuyo producto **sigue** en `ModeOwnStock`;
  los demás salen en `skippedProducts` de la vista previa y del cierre, y no se ajustan.
- **Rationale**: el cambio de modo ya cerró ese contador a propósito; un ajuste lo reviviría con
  piezas que el sistema ya no lleva.

## R6. «Lo restan sus opciones»

- **Decision**: modo nuevo en la petición de composición, `mode: "options"`. Se guarda igual que
  `nothing` (sin receta, confirmado) y sale de la existencia propia por el mismo camino
  (`TargetNothing` en `domain.PlanModeChange`: cierra en cero por sucursal con el movimiento de
  cambio de modo y la bitácora `product.consumption_mode`). Lo que lo distingue es una validación
  del servidor: el producto debe tener al menos un grupo ligado con una opción activa que reste
  (receta con renglones, o producto ligado que resta). Sin eso, 422 «Ninguna de sus opciones resta
  del almacén».
- **El modo leído** (`ConsumptionMode`) gana un valor `options`, deducido: confirmado, sin receta,
  sin existencia propia, no paquete, y con opciones que restan. `ModeOf` recibe ese hecho como un
  booleano más (`OptionsDeduct`). Recetas (`mode: "options"`) lo muestra como «Lo restan sus
  opciones»; Salud del inventario no lo lista (está confirmado y sus ventas restan; spec FR-015).
- **Rationale**: no hace falta columna: el modo ya es deducido (spec 037) y así no puede contradecir
  a la venta. `ExpandSale` ya resta las opciones sin importar el modo del producto, por eso «al
  vender solo baja el artículo del sabor» es cierto en cuanto se apaga el contador.
- **Compatibilidad**: la pantalla anterior nunca manda `options`; la API anterior recibiría
  `options` como modo desconocido (422) solo si una tableta ya actualizada habla con ella durante
  los segundos del traslape: la persona reintenta. Un valor nuevo de `mode` en Recetas se
  pinta en la pantalla anterior como «No gasta insumos»/«none», que es lo de hoy.
- **Alternatives considered**: renombrar «No se cuenta» (rechazado: sigue siendo correcto para un
  producto que de verdad no resta nada); permitir `items` vacío (rechazado: «Se prepara aquí» sin
  nada guardaría «nunca decidido», FR-014b de la 037).

## R7. Número de migración

- **Decision**: `0110_count_product_lines.sql`. Revisado el 2026-11-08: las ramas y worktrees de
  065, 066 y 067 no tienen migración nueva (todas terminan en 0109). Se vuelve a revisar antes de
  empujar; la que llegue primero a develop se queda con el número y la otra renumera.

## R8. Revisión de arquitectura (db-architect y tablet-ui-reviewer, 2026-11-08)

Lo que cambió el diseño:

- **Fila de existencia garantizada antes de bloquear** (HIGH). `for share`/`for update nowait` no
  bloquean una fila que no existe, y un producto que nunca se vendió en la sucursal no tiene fila en
  `stock_levels`. Capturar un producto inserta primero su fila en cero (`insert … on conflict
  (branch_id, product_id) do nothing`, la llave `stock_levels_product_per_branch` de la 0076) y
  después la toma `for share`. Así el cierre siempre encuentra la fila y la venta concurrente espera
  o hace esperar. Una consulta propia de bloqueo para productos (`LockProductStockLevelsInBranchNowait`,
  por `product_id` y `branch_id`, en orden de id). Test: producto sin fila + venta concurrente.
- **El Down se niega con datos** (HIGH). `drop table` borraba cifras de conteos cerrados sin avisar.
  El Down hace `raise exception` si hay renglones, con su renglón en `guardedDownCleanup` (borra las
  filas en la copia de prueba). Queda escrito que sus ajustes en `stock_movements` sobreviven y que el
  Down de 0092 ya se niega con ellos.
- **`last_movement_id` sin FK, a propósito** (MEDIUM). Igual que en `stock_count_lines` (0094):
  `OnHandForCapture` devuelve 0 cuando no hay movimientos y un 0 no puede ser FK; la columna solo
  ancla un periodo por comparación de ids. Se dice en la migración.
- **Cierre por la API anterior durante el traslape** (MEDIUM). Ese cierre deja renglones de producto
  sin ajustar en un conteo cerrado. El marcador ya existe: `unit_cost` es nulo hasta que la API nueva
  cierra; «conteo cerrado + renglón con `unit_cost` nulo» = «no se ajustó». Se documenta en
  `almacen.md` para el reporte futuro. No se agrega bloqueo de arranque: la ventana son segundos.
- **El modo `options` es deducido del catálogo vivo** (MEDIUM). Si después alguien apaga la última
  opción que resta, el producto se lee como «No se cuenta» sin que nada falle; la validación del
  servidor es al guardar, no una invariante. Se documenta en `almacen.md`; Salud del inventario lo
  hace visible solo si se vende sin restar. El conteo de «sin decidir» de Salud no cambia (un `none`
  pasa a `options`, los dos son decididos).
- **El cierre lee el modo DESPUÉS de bloquear las existencias de los productos** (MEDIUM). El cambio
  de modo (`applyModeChange`) bloquea esas mismas filas `FOR UPDATE`: si está en curso, el cierre
  responde «vuelve a intentar»; si ya confirmó, el cierre ve el modo nuevo y lo salta. Test de
  integración con cambio de modo a media cerrada.
- **Pantalla**: el aviso de productos va dentro del renglón de avance (0 px extra); la vista previa
  pone primero las diferencias grandes y esconde los nombres sin contar tras «Ver cuáles»; «Producto
  del menú» apagado; textos de existencia en palabras de quien opera. Detalle en plan.md.

Lo que se dejó como está: índice `(company_id, product_id)` suficiente; sin check de máximo en la
base (el máximo es por empresa y vive en `count_limits`); `captured_by` hereda la ambigüedad de
tabletas compartidas.
