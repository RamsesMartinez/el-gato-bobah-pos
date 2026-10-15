# Research: Merma y faltante sin explicar

## R1. Cómo se parte el faltante

- **Decisión**: por insumo y periodo, `perdido = registrado + sin explicar`, donde *registrado* es la
  merma que restó en el periodo y *sin explicar* es `system_qty − counted_qty` del renglón del conteo
  (si es positivo; si es negativo, «sobraba»).
- **Por qué**: la merma registrada ya restó de lo que el sistema tenía al capturar el conteo. La
  fórmula del issue («diferencia del conteo menos la merma») la contaría dos veces. Encontrado al
  planear; el spec se corrigió (FR-014, SC-003).
- **Alternativas**: guardar la merma fuera del almacén y restarla solo en el reporte; descartado
  porque el almacén dejaría de decir lo que hay mientras tanto.

## R2. El periodo de un insumo

- **Decisión** (corregida tras la revisión de arquitectura): el periodo se ancla por **id de
  movimiento**, no por hora. Al capturar un insumo, el conteo guarda el último movimiento del insumo
  en la sucursal (`last_movement_id`), leído con el insumo tomado. El periodo es
  `(anterior.last_movement_id, este.last_movement_id]`.
- **Por qué no la hora**: `now()` es la hora de inicio de la transacción. Una merma que empezó antes
  de la captura y confirmó después tendría hora anterior y caería en el periodo equivocado, aunque
  el conteo no la vio. El id lo asigna la base al insertar y el candado del insumo ordena las dos.
- **Por qué**: el conteo ajusta contra lo que había al capturar (038); una merma posterior a la
  captura ya queda fuera de este conteo y entra en el siguiente, sin perderse ni duplicarse.
- **Alternativas**: por fecha de cierre del conteo (descartado: una merma entre captura y cierre
  quedaría en el periodo equivocado).

## R3. Lo que resta un producto

- **Decisión**: `StockGraph.ExpandSale` con una línea sin extras y la cantidad registrada; los
  movimientos llevan `waste_record_id` y no `order_id`.
- **Por qué**: una sola regla de qué sale (AGENTS.md, spec 028). Un camino nuevo que restara a su
  manera divergiría de la venta.
- **Costo**: `products.current_cost` × piezas para el snapshot y el tope; por insumo,
  `ingredients.current_cost` × cantidad base.

## R4. Concurrencia

- **Decisión**: registrar y aprobar toman los insumos con `lockIngredients` del 038 (`for no key
  update nowait` + existencias `nowait`) antes de escribir; 55P03 → 409. Deshacer, aprobar y
  rechazar toman el registro `for update` y validan la transición en `domain`.
- **Por qué**: el spec pide no esperar a una venta ni hacerla esperar; es el mismo contrato que
  compras. Orden de candados: registro, luego insumos ordenados por id, igual que el 038.

## R5. Cobertura por el conteo

- **Decisión**: `CloseCount` pasa a `covered` las pendientes de la sucursal cuyos insumos que
  restarían incluyan alguno contado **y que se registraron antes de la captura de ese insumo**,
  dentro de la misma transacción del cierre. Una registrada entre la captura y el cierre no la vio
  el conteo: sigue por aprobar.
- **Por qué**: un conteo descartado no corrige nada, así que no debe cubrir; y hacerlo al capturar
  dejaría pendientes cubiertas por un conteo que nunca se cerró.

## R6. Motivos sugeridos

- **Decisión**: la migración siembra los cinco motivos genéricos (caducó, se cayó, mal preparado,
  prueba de receta, consumo del personal) para cada empresa existente; `CreateCompany` los siembra
  en la misma consulta para una nueva (CTE, como la matriz de 0076).
- **Por qué**: no son datos de un negocio sino de cualquier negocio de comida (constitución,
  «producto para negocios de comida»), y no se puede sembrar con un trigger sobre `companies`
  porque `pg_restore` lo duplicaría.
- «Consumo del personal» se distingue por una marca `is_staff_meal` en el motivo, no por su nombre:
  la empresa puede renombrarlo y el reporte lo sigue poniendo aparte.

## R7. Nombre de motivo repetido

- **Decisión**: la misma normalización que `IngredientNameTaken` (sin mayúsculas, acentos ni
  espacios) y un candado por empresa al crear o renombrar.

## R8. Quién registra

- **Decisión**: `inventory.write_off` para admin, gerente, cajero, mesero y cocina (decisión del
  9-oct: cualquiera del local). `inventory.write_off_approve` para admin y gerente. Motivos y tope
  bajo `inventory.configure`. Reportes bajo `inventory.health_view`.

## R9. Lo que cambió con la revisión de arquitectura (2026-10-15)

- Juntar motivos no reescribe `reason_id`: el registro guarda su motivo y el reporte sigue
  `merged_into_id` (un nivel). Reescribirlo cerraba la puerta de reconstruir el motivo original.
- El faltante guarda su costo al cerrar (`stock_count_lines.unit_cost`) y la merma copia
  `is_staff_meal`. Así un reporte cerrado no cambia cuando cambia un costo o una marca.
- La sucursal del movimiento de merma es explícita y está amarrada al registro con FK compuesta.
  El trigger genérico de 0076 la tomaría de la empresa y truena con dos sucursales, como le pasó a
  la venta en 0078.
- Unicidad del nombre de motivo con índice en la base, no solo con candado.
- Pantalla: franja de 44 px y nunca en Vender ni Pedidos; el riel filtra por permiso; la vista «Una
  pantalla» con columnas que se desplazan solas y el artículo en `Picker`. Detalle en el plan.
