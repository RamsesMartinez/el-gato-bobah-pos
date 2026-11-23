# Research: Varios almacenes por sucursal

Medido en producción el 2026-11-23 (solo lectura): 2 empresas, 1 sucursal activa cada una;
`stock_movements` 7,219 filas (2.4 MB), `stock_levels` 407, `expense_items` 75. El relleno de la
migración es de milisegundos; el riesgo no es el tamaño sino la convivencia con la API anterior.

## R1. Cómo convive la migración con la versión anterior

**Decisión**: patrón de la 0076 (sucursales). `warehouse_id` entra **nullable**, se rellena con el
almacén de venta de la sucursal de cada fila, y un trigger `BEFORE INSERT` lo llena cuando el insert
no lo trae. Así la versión anterior —que no conoce la columna— sigue escribiendo y su fila cae en el
almacén de venta. El `not null` va en la subida siguiente (2), con `check … not valid` + `validate` +
`set not null` para no tomar `ACCESS EXCLUSIVE` durante un escaneo.

**Por qué no `not null` de una vez**: el dueño lo fijó así (#65, 2026-11-23) y es la regla de la
constitución IV para un `not null` nuevo.

**Alternativas**: default de columna (no sirve: el default no puede depender de la sucursal de la
fila); llenar desde Go (no cubre a la versión anterior).

## R2. Las llaves únicas por sucursal (expandir y después contraer)

Hoy hay tres upserts con destino por sucursal, dos en Go y uno en la base:

| Upsert | Destino de hoy | Dónde |
| --- | --- | --- |
| `apply_stock_movement()` | `(branch_id, ingredient_id)` / `(branch_id, product_id)` | trigger de la base |
| `EnsureProductStockLevel` | `(branch_id, product_id) do nothing` | `queries/stock_counts.sql` |
| `UpsertBranchMinimum` / `…ProductMinimum` | `(branch_id, ingredient_id|product_id)` | `queries/notifications.sql` |
| `OpenManualOut` | `(company_id, branch_id, item_key, kind) where …` | `queries/stock_out_reports.sql` |
| `evaluate_low_stock()` | el mismo índice de avisos | función de la base |

Si la 0120 cambia las llaves, el `on conflict` de la API anterior truena con «no unique or exclusion
constraint matching» durante el traslape (500 en capturar producto en conteo y en guardar mínimos).

**Decisión**:
- **Subida 1 (0120)**: se **agregan** las llaves por almacén junto a las de sucursal. El trigger de
  la base pasa a usar la de almacén. El código de la subida 1 usa la de almacén en sus dos upserts.
- **Consecuencia**: mientras vivan las dos llaves, una sucursal solo puede tener existencias en **un**
  almacén por artículo. Por eso la subida 1 **no escribe en otro almacén que el de venta** y **no
  permite cambiar cuál es el de venta**: dar de alta «Bodega» sí, llenarla no.
- **Subida 2 (0121)**: cuando la versión anterior ya es la subida 1, se borran las llaves por sucursal
  y se pone el `not null`. Desde ahí hay compras con destino (E3) y cambiar el de venta.

## R3. Qué se puede subir en cada subida

| Subida | Migración | Entregas | Por qué juntas |
| --- | --- | --- | --- |
| 1 | 0120 | E1 almacenes y almacén en cada registro + E2 ventas | La venta ya descuenta del almacén de venta por el trigger (R1); E2 es leer por almacén en Existencias y la regla de «hay existencia» |
| 2 | 0121 | contraer llaves + `not null` + E3 compras con destino | E3 es la primera escritura en otro almacén y necesita la llave contraída |
| 3 | 0122 | E4 traspasos | Tabla nueva de traspasos |
| 4 | — o 0120 | E5 conteo, merma, mínimos y lugares por almacén | Las columnas ya existen desde 0120 |
| 5 | — | E6 reportes y movimientos | Solo lectura |

Si dos subidas caben en una sola sin romper R2 (p. ej. 4 y 5), se juntan; nunca se juntan 1 y 2.

## R4. Renglones del conteo

**Decisión**: `stock_count_lines` y `stock_count_product_lines` **no** llevan columna propia. Heredan
el almacén de su conteo (`count_id` con llave compuesta), igual que hoy heredan la sucursal. Una
columna propia permitiría un renglón de otro almacén que su conteo, que es justo el estado inválido.
Los paquetes cerrados (0106) son una columna de `stock_count_lines`: mismo caso.

**Alternativa rechazada**: columna en el renglón (lo pedía el requerimiento literal): duplica un dato
que ya fija el conteo y abre la inconsistencia.

## R5. Integridad: el almacén es de la sucursal de la fila

**Decisión**: `warehouses` lleva `unique (company_id, branch_id, id)` y cada tabla con `branch_id`
apunta con llave compuesta `(company_id, branch_id, warehouse_id)`. Un almacén de otra sucursal o de
otra empresa no se puede guardar aunque RLS fallara (los chequeos de llave foránea saltan RLS; la
llave compuesta no). `expense_items` y `count_places` no tienen sucursal: `(company_id, warehouse_id)`.

En `stock_movements`, el trigger que llena la sucursal toma la del almacén cuando el insert trae
almacén y no sucursal; así el código nuevo puede mandar solo el almacén.

## R6. Almacén de venta

**Decisión**: `warehouses.is_sales` con índice único parcial `(company_id, branch_id) where is_sales`.
Una función `sales_warehouse_of(company, branch)` lo resuelve y truena (`EGB03`) si no hay ninguno,
en lugar de escoger otro en silencio. `CreateCompany` crea «Local» en la misma consulta que la
matriz, por la razón de la 0076 (un trigger sobre `branches` duplicaría el que trae un respaldo al
restaurar con COPY). Hoy ninguna otra ruta crea sucursales.

## R7. Traspasos (subida 3)

**Decisión**: `stock_transfers` (cabecera: sucursal, origen, destino, nota, quién, cuándo, `client_key`
único por empresa) y `stock_transfer_lines` (artículo, cantidad en unidad base y, opcional, la
presentación y su cantidad). Cada renglón produce dos movimientos de tipo nuevo `traspaso`
(`stock_movement_type` gana el valor), uno negativo en el origen y uno positivo en el destino, con
`transfer_id`. Al costo vigente del artículo: el costo es del artículo, no del almacén, así que el
traspaso no cambia costos. No es costo de ventas: el reporte de costo de ventas sale de `venta`.

`ALTER TYPE … ADD VALUE` no puede usarse en la misma transacción que lo agrega: la 0122 lo agrega en
su propia sentencia sin transacción (`-- +goose NO TRANSACTION` en un archivo aparte si hace falta) y
el código que lo usa llega en la misma subida, después de migrar.

**Idempotencia**: `client_key` (uuid del equipo) único por empresa; el reintento devuelve el traspaso
existente si el cuerpo coincide, 409 si no.

## R8. La función compartida con el #326

El #326 deja «¿hay existencia de este producto en este lugar de venta?». Aún no está en develop
(2026-11-23). **Decisión**: esta feature no la duplica. Mientras no llegue, la regla de E2 vive en la
lectura de existencias por almacén de venta (`sales_warehouse_of`); cuando llegue a develop, se
traen sus cambios y se le cambia el interior para que mida `stock_levels` del almacén de venta, con
un test que tenga existencia en «Bodega» y cero en «Local» y espere «sin existencia».

## R9. Permisos

`inventory.warehouse_manage` («Administrar almacenes») e `inventory.transfer` («Traspasar entre
almacenes»), ids 39 y 40 (si otra rama toma esos ids, la que llega después renumera, igual que la
migración). Herencia: `warehouse_manage` a admin y gerente (los que hoy tienen `inventory.configure`);
`transfer` a admin y gerente (los que hoy tienen `inventory.count`). Contar sigue con
`inventory.count` para cualquier almacén de la sucursal.

## R10. Lo que se descubrió y no estaba en el requerimiento

- **Movimientos no filtra por sucursal** (`ListStockMovements`): con dos sucursales mezcla las dos.
  Se corrige en E6 al filtrar por almacén.
- **Sumas de toda la empresa** (`IngredientOnHandTotal` para el costo, `GetProductOnHand`,
  `per_sale_on_hand` de recetas, `on_hand` de renglones de gasto, candidatos a unir): son totales a
  propósito (el costo promedio es del artículo). Se quedan como suma de todos los almacenes; el
  `per_sale_on_hand` de recetas pasa al almacén de venta en E2 porque responde «¿alcanza para vender?».
- **No existe reporte de valor del inventario** (spec 038 lo dejó fuera). E6 lo crea: cantidad ×
  costo vigente por almacén y total.

## R11. Revisión del db-architect sobre este diseño (2026-11-23), ya incorporada

Veredicto: 0 CRITICAL, 6 HIGH, 7 MEDIUM. Lo que cambia del diseño de arriba:

| Hallazgo | Decisión |
| --- | --- |
| H1 `low_stock_state` hace un subselect de una fila sobre `stock_levels`; con dos almacenes por artículo truena «more than one row» **fuera** del `exception`, y tumba la venta | La 0120 lo reemplaza por la existencia del **almacén de venta** (no la suma de la sucursal: con Local en 0 y Bodega en 50 la suma no avisaría «se acabó»). Con un almacén es idéntico. En E5 pasa a por almacén |
| H2 La compuerta «solo el de venta» vivía solo en Go | La 0120 la pone en la base: el trigger de llenado de `stock_movements` lanza `EGB04 warehouse_not_writable_yet` si `warehouse_id` no es el de venta, y `warehouses_before_update` rechaza cambiar `is_sales`. La 0121 quita las dos, en la misma migración que contrae las llaves |
| H2b En la subida 2 la API vieja es la de la subida 1 y ya ve existencias en dos almacenes | Todo lo que la subida 1 lee o escribe por sucursal sobre existencias se vuelve por almacén **en la subida 1**: `RecomputeArticleStock`, `InsertMergeMove`, `InsertReplacementMove`, `LockArticleStockNowait` (por almacén), `ArticleOnHandByBranch`, los `join stock_levels … current_branch_id()` (sumados por artículo o filtrados al de venta), y los de conteo y merma que comparan `branch_id`. Cada uno con un test que siembra existencias en un segundo almacén por SQL (apagando el trigger de H2 solo en el test) |
| H3 `ADD VALUE 'traspaso'` y su uso en la misma transacción | Migración propia solo con el `ADD VALUE` (Down no-op documentado); las tablas de traspaso en la siguiente |
| H4 `stock_transfer_lines` | En `MergeTables` como «lo pasado se queda; después, rechaza», con las dos guardas de la 0104; en `UnitChangeTables` «obliga al reemplazo» |
| H5 `expense_items` | Se llena todo renglón con `item_type` (excluido o no); check en la 0121: `item_type is null or warehouse_id is not null`. Relleno por la sucursal del movimiento ligado y, si no hay, la matriz explícita (no `branch_for_company`) |
| H6 Downs | 0120: se niega si hay almacén que no sea de venta o fila fuera del de venta; restaura `apply_stock_movement` (0076) y el llenado de sucursal (0078) antes de soltar columnas; borra overrides, `system_role_permissions` y `permissions` de los dos permisos. 0121: recrea las llaves por sucursal y se niega con duplicados. Tablas de traspaso: se niega con traspasos. Cada guarda con su renglón en `guardedDownCleanup`. **Después de la 0121 ya no se puede volver a una imagen anterior a esta feature** |
| M1 Bloqueos | `set local lock_timeout = '3s'` y un solo `lock table stock_movements, stock_levels, …` en el orden en que opera la venta, antes del relleno |
| M2 Una empresa creada por la API anterior durante la subida 1 nace sin «Local» | La 0121 crea «Local» donde falte antes de validar. Helper de pruebas que crea el almacén junto con la sucursal |
| M3 Un ajuste de conteo o de merma podía llevar otro almacén que su conteo o su merma | `unique (id, company_id, warehouse_id)` en `stock_counts` y `waste_records` y llaves compuestas desde `stock_movements`; ídem `(transfer_id, company_id, branch_id)` |
| M4 Lugares de conteo | Decisión: el lugar pertenece a un almacén (requerimiento, punto 4); nombre único por `(company_id, warehouse_id)`. El artículo conserva un solo lugar: al contar un almacén, los artículos cuyo lugar es de otro almacén salen al final en «Sin lugar en este almacén». Sin tabla puente (se agrega después desde cero, sin hecho perdido) |
| M5 `ON DELETE` | `no action` en las llaves nuevas; `cascade` a `companies` en las tablas nuevas |
| M6 Tablas nuevas | `stock_transfer_lines` con `company_id` propio y RLS; `warehouses` con RLS y grant; `warehouses.name` y `stock_transfers.client_key` a `noPersonalData`, `stock_transfers.note` se borra en el respaldo anónimo |
| M7 Numeración | Las ramas 091, 092 y 093 ya tienen una 0120, y la 093 toma el permiso 39. Los números de migración y de permiso se fijan al llegar a develop: la que llega después renumera |
| L1 | El llenado de almacén en `stock_movements` va dentro de `fill_stock_movement_branch_id` (una sola función), no en un trigger aparte que dependa del orden alfabético |
| L3 | `(warehouse_id, ingredient_id)` sin `company_id` es a propósito: `warehouse_id` es único global |
| L6 | Los únicos parciales de mínimos repiten el predicado exacto (`ingredient_id is not null`) para que el `on conflict … where` los infiera |

## R12. Segunda vuelta de revisión (db-architect y tablet-ui-reviewer, 2026-11-23), ya incorporada

**Base de datos**

| Hallazgo | Decisión |
| --- | --- |
| HIGH `evaluate_low_stock` hace `on conflict` sobre `notifications_one_open`, que la 0121 borra; el `exception when others` se traga el error y los avisos dejan de crearse sin log | La 0120 redefine `evaluate_low_stock` con `on conflict` sobre `notifications_one_open_per_warehouse` (mismo predicado exacto) y el almacén del movimiento. Test después de la 0121: vender cruzando el mínimo y **afirmar que la fila de aviso existe** |
| HIGH FR-011 sin diseño: las reposiciones de cancelación (`RestockCancelledOrder`, `RestockCancelledLine` y las otras dos de `orders.sql`) no traen almacén y el trigger las mandaría al de venta de hoy | En la subida 1 copian `sm.warehouse_id` del movimiento original (y lo agregan al `group by`). Test: vender, cambiar el de venta (desde la subida 2), cancelar pedido y renglón, verificar el almacén de cada reposición. La 0121 hace que el llenado rechace (`EGB04`) un movimiento `cancelacion` sin almacén |
| MEDIUM `low_stock_state` sumando la sucursal | Corregido en R11 H1: mide el almacén de venta |
| MEDIUM tres versiones del check de `expense_items` | Una sola: **todo renglón con `item_type` lleva almacén, excluido o no**; check `item_type is null or warehouse_id is not null` en la 0121. Un excluido guarda su almacén pero no mueve existencias (`PlanStockLines` ya le pone objetivo cero) |
| MEDIUM traspaso sin costo guardado | Las dos patas del traspaso guardan `unit_cost` = costo vigente del artículo (snapshot; principio VIII) |
| Menor llave del traspaso | `(transfer_id, company_id, branch_id)` desde `stock_movements`; un test verifica que cada pata está en el origen o el destino de su traspaso |
| Puerta VIII | Se cruza con la 0120 (el hecho irrecuperable es el almacén de cada registro): la tabla de puertas de la constitución se edita en la subida 1. `sales_warehouse_of(company, branch)` conserva esa firma para poder ganar después un parámetro de lugar de venta (puerta «más de una caja») |
| Enum nuevo `traspaso` | Las tareas revisan que ningún mapa de tipos de movimiento en Go o en `web/` truene con un valor desconocido antes de la subida 3 |

**Pantallas**

| Hallazgo | Decisión |
| --- | --- |
| HIGH Traspaso B teclea con el teclado del sistema | La cantidad usa `NumericKeypad` fijo al pie con `useKeypadField` (como `AddLineSheet`) |
| HIGH «De / A» del traspaso parecen selector y no lo son | Cada uno es un `Picker` de 44 px que excluye el otro extremo; el botón de invertir se queda |
| MEDIUM el activo en `brand` (rojo) se confunde con error | Componente común nuevo `web/src/components/Segmented.tsx` (44 px, `aria-pressed`, un solo color de activo que no es el de error, `overflowX="auto"`), usado en Existencias, Compra y Traspaso |
| MEDIUM buscador del traspaso de 40 px | `minH="44px"` en el `Input` |
| MEDIUM a 1024 px las pestañas nuevas quedan fuera de la vista | «Almacenes» no es pestaña: vive en Negocio › Inventario junto a «Lugares para contar» (es configuración, la usa quien tiene `inventory.warehouse_manage`). «Traspaso» va inmediatamente después de «Existencias» |
| MEDIUM «Quitar renglón» sin confirmar junto a los conmutadores | `ConfirmSheet` y deshabilitado con un solo renglón |
| MEDIUM Compra R3 a 1024×600 recorta el detalle | La fila «N renglones / ¿Dónde se guarda? / Agregar» sube al encabezado; por debajo de 1100 px de ancho el detalle se abre como hoja inferior (R1) |
| LOW Almacenes A2/A3 con estilo propio | La opción que elija el dueño se construye ampliando `ListRow` (ícono opcional), sin etiquetas en mayúsculas que el POS no usa; «Archivar» con `ConfirmSheet` |
| LOW nombres largos en el segmentado | `overflowX="auto"` en el contenedor; con 5+ almacenes pasa a botón con hoja |
| Textos | El resumen «Se guarda en Local: N · en Bodega: M» y «¿Dónde se guarda?» solo aparecen con 2+ almacenes |

## R13. Riesgo aceptado en la subida 1

Una empresa que dé de alta la API anterior durante el traslape (o tras un regreso a la versión
anterior) nace sin almacén de venta, y su primer movimiento truena con `EGB03`. No se arregla con un
trigger sobre `branches`: un restore con COPY lo dispararía y chocaría con el «Local» del respaldo
(la razón de la 0076). La ventana es la del traslape, la API nueva lo crea en la misma consulta que la
matriz, y la 0121 crea «Local» donde falte (R11 M2).

## R14. Puertas con lo que viene encima (#146 caducidad por lote, #226 merma esperada)

- **Lotes (#146)**: un lote guardará su almacén. El traspaso no asume que una existencia es solo
  cantidad: `stock_transfer_lines` es un renglón por artículo y cantidad al que se le puede agregar
  después el lote de origen (una columna nullable y, si hace falta, varios renglones por artículo);
  los dos movimientos del traspaso llevan `transfer_id` y su `unit_cost`, así que un lote podrá
  colgarse de cada pata sin reescribir traspasos pasados. Nada de esta feature guarda «la existencia
  del almacén» como un número suelto fuera de `stock_levels`.
- **Merma esperada (#226)**: su reporte de conteo contra ventas se construye por almacén encima de E5;
  las ventas ya salen del almacén de venta y el conteo es por almacén.
