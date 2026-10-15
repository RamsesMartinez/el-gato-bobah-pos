-- Lo que se echó a perder (spec 039). La empresa la pone RLS; la sucursal es la de la sesión
-- (`current_branch_id()`, que con dos y sin selector truena con EGB01 en vez de adivinar).

-- name: CurrentBranchID :one
select current_branch_id()::bigint as branch_id;

-- name: ListActiveWasteReasons :many
-- Los que se ofrecen, los más usados primero (los botones grandes de la captura).
select r.id, r.name, r.description, r.is_staff_meal,
       (select count(*) from waste_records w where w.reason_id = r.id)::int as uses
  from waste_reasons r
 where r.archived_at is null and r.merged_into_id is null
 order by uses desc, r.name, r.id;

-- name: ListAllWasteReasons :many
-- Para administrarlos: también los archivados y juntados.
select r.id, r.name, r.description, r.is_staff_meal, r.archived_at, r.merged_into_id,
       (select count(*) from waste_records w where w.reason_id = r.id)::int as uses
  from waste_reasons r
 order by r.archived_at is not null, r.merged_into_id is not null, r.name, r.id;

-- name: GetWasteReason :one
select id, name, is_staff_meal, archived_at, merged_into_id from waste_reasons where id = $1;

-- name: FindWasteReasonByName :one
-- El que ya existe con ese nombre, sin distinguir mayúsculas, acentos ni espacios (el mismo índice
-- único de la 0094), para ofrecerlo en vez de crear otro.
select id from waste_reasons
 where archived_at is null and merged_into_id is null
   and regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
     = regexp_replace(translate(lower(btrim(sqlc.arg(name)::text)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g');

-- name: InsertWasteReason :one
insert into waste_reasons (name, description, created_by) values ($1, $2, $3) returning id;

-- name: UpdateWasteReason :execrows
update waste_reasons
   set name = sqlc.arg(name), description = sqlc.arg(description), is_staff_meal = sqlc.arg(is_staff_meal),
       archived_at = case when sqlc.arg(archived)::bool then coalesce(archived_at, now()) end
 where id = sqlc.arg(id) and merged_into_id is null;

-- name: LockWasteReasonsForMerge :exec
-- Los dos motivos de un juntar, en orden de id: dos juntares cruzados (A en B y B en A) no pasan los
-- dos la validación ni dejan un ciclo.
select id from waste_reasons where id = any(sqlc.arg(ids)::bigint[]) order by id for update;

-- name: MoveMergedWasteReasons :exec
-- Lo que ya apuntaba al que se junta pasa al que queda: un solo nivel. Solo mueve ligas de motivos,
-- nunca los registros.
update waste_reasons set merged_into_id = sqlc.arg(into_id) where merged_into_id = sqlc.arg(id);

-- name: MergeWasteReason :execrows
-- Juntar no toca los registros (revisión de arquitectura): el reporte sigue esta liga.
update waste_reasons set merged_into_id = sqlc.arg(into_id)
 where id = sqlc.arg(id) and merged_into_id is null;

-- name: WasteApprovalThreshold :one
-- Sin cast: la columna es nulable (sin tope) y un cast haría que sqlc la tipara como no nulable y el
-- scan tronara con el tope vacío. Sin fila de configuración, ErrNoRows: tampoco hay tope.
select waste_approval_threshold from business_settings limit 1;

-- name: IngredientForWaste :one
-- El insumo, su costo por unidad base y cuánto de su unidad base es la unidad capturada; `same_kind`
-- falso si la unidad es de otro tipo (kg para un insumo en ml).
select i.id, i.current_cost,
       (u.to_base / bu.to_base)::numeric(20,6) as factor,
       (u.kind = bu.kind)::bool as same_kind
  from ingredients i
  join units bu on bu.id = i.base_unit_id
  join units u on u.id = coalesce(sqlc.narg(unit_id)::smallint, i.base_unit_id)
 where i.id = sqlc.arg(id);

-- name: ProductCostForWaste :one
select id, current_cost from products where id = $1;

-- name: InsertWasteRecord :one
insert into waste_records (
  branch_id, reason_id, item_type, ingredient_id, product_id, quantity, entered_qty, presentation_id,
  unit_cost, total_cost, is_staff_meal, note, status, recorded_by
) values (
  sqlc.arg(branch_id), sqlc.arg(reason_id), sqlc.arg(item_type), sqlc.narg(ingredient_id), sqlc.narg(product_id),
  sqlc.arg(quantity), sqlc.arg(entered_qty), sqlc.narg(presentation_id), sqlc.narg(unit_cost), sqlc.narg(total_cost),
  sqlc.arg(is_staff_meal), sqlc.narg(note), sqlc.arg(status), sqlc.arg(recorded_by)
) returning id, recorded_at;

-- name: DBNow :one
-- La hora de la base. Las decisiones de tiempo de la merma (mismo día, contado después, vio el
-- conteo) comparan contra `recorded_at` y `captured_at`, que pone la base: con el reloj de la API
-- se mezclarían dos relojes.
select clock_timestamp()::timestamptz as now;

-- name: InsertWasteMovement :exec
-- La sucursal es la del registro, explícita: el trigger genérico la tomaría de la empresa y truena
-- con dos sucursales (como la venta en la 0078). La FK compuesta impide otra.
insert into stock_movements (
  item_type, ingredient_id, product_id, movement_type, quantity, unit_cost, waste_record_id, user_id,
  reason, branch_id, modifier_option_id, component_of_product_id
) values (
  sqlc.arg(item_type), sqlc.narg(ingredient_id), sqlc.narg(product_id), 'merma', sqlc.arg(quantity),
  sqlc.narg(unit_cost), sqlc.arg(waste_record_id), sqlc.arg(user_id), sqlc.arg(reason), sqlc.arg(branch_id),
  sqlc.narg(modifier_option_id), sqlc.narg(component_of_product_id)
);

-- name: WasteMovements :many
-- Lo que restó un registro, para devolverlo exactamente al deshacer.
select item_type, ingredient_id, product_id, quantity, unit_cost, modifier_option_id, component_of_product_id
  from stock_movements where waste_record_id = $1 order by id;

-- name: LockWasteRecord :one
select id, branch_id, item_type, ingredient_id, product_id, quantity, unit_cost, total_cost, status,
       recorded_by, recorded_at
  from waste_records where id = $1 for update;

-- name: ResolveWasteRecord :execrows
-- Cambia de estado con quién y cuándo. Aprobar fija el costo si el registro no tenía.
update waste_records
   set status = sqlc.arg(status), resolved_by = sqlc.narg(resolved_by), resolved_at = clock_timestamp(),
       unit_cost = coalesce(unit_cost, sqlc.narg(unit_cost)), total_cost = coalesce(total_cost, sqlc.narg(total_cost)),
       covered_by_count_id = sqlc.narg(covered_by_count_id)
 where id = sqlc.arg(id) and status = sqlc.arg(from_status);

-- name: IngredientsCountedSince :one
-- ¿Algún conteo de la sucursal, cerrado o abierto, capturó alguno de estos insumos después de la
-- merma? Entonces el conteo ya corrige lo guardado contra lo que vio, y deshacer inventaría lo que
-- quitó; abierto también: su ajuste se calculó al capturar (revisión de código).
select exists (
  select 1 from stock_count_lines l join stock_counts c on c.id = l.count_id
   where c.status in ('open', 'closed') and c.branch_id = sqlc.arg(branch_id)
     and l.ingredient_id = any(sqlc.arg(ids)::bigint[]) and l.captured_at > sqlc.arg(since)
)::bool;

-- name: WasteOfDay :many
-- Lo registrado en la sucursal desde un momento (el inicio del día del local).
select w.id, w.item_type, coalesce(i.name, p.name)::text as item_name, w.quantity, w.entered_qty,
       coalesce(u.code, 'pza')::text as unit_code, w.total_cost, w.status, w.note,
       r.name as reason_name, w.recorded_by, uu.name as recorded_by_name, w.recorded_at
  from waste_records w
  join waste_reasons r on r.id = w.reason_id
  join users uu on uu.id = w.recorded_by
  left join ingredients i on i.id = w.ingredient_id
  left join units u on u.id = i.base_unit_id
  left join products p on p.id = w.product_id
 where w.branch_id = current_branch_id() and w.recorded_at >= sqlc.arg(since)
 order by w.recorded_at desc, w.id desc;

-- name: PendingWaste :many
select w.id, w.item_type, coalesce(i.name, p.name)::text as item_name, w.quantity,
       coalesce(u.code, 'pza')::text as unit_code, w.total_cost, r.name as reason_name,
       uu.name as recorded_by_name, w.recorded_at
  from waste_records w
  join waste_reasons r on r.id = w.reason_id
  join users uu on uu.id = w.recorded_by
  left join ingredients i on i.id = w.ingredient_id
  left join units u on u.id = i.base_unit_id
  left join products p on p.id = w.product_id
 where w.branch_id = current_branch_id() and w.status = 'pending'
 order by w.recorded_at, w.id;

-- name: OpenCountThatSaw :one
-- El conteo abierto de la sucursal que capturó alguno de estos insumos después de la merma, si lo
-- hay: ya ajustará esa pérdida al cerrar, así que aprobarla la restaría dos veces.
select coalesce((
  select c.id from stock_count_lines l join stock_counts c on c.id = l.count_id
   where c.status = 'open' and c.branch_id = sqlc.arg(branch_id)
     and l.ingredient_id = any(sqlc.arg(ids)::bigint[]) and l.captured_at >= sqlc.arg(since)
   order by c.id limit 1), 0)::bigint;

-- name: PendingWasteToCover :many
-- Las por aprobar de la sucursal, para que el cierre de un conteo cubra las que vio; cuáles vio lo
-- decide coverPendingWaste contra la hora de captura de cada insumo.
select id, item_type, ingredient_id, product_id, quantity, recorded_at
  from waste_records
 where branch_id = sqlc.arg(branch_id) and status = 'pending'
 order by id;

-- name: ListWaste :many
-- Los registros de un periodo para la pestaña; el motivo puede ser uno juntado: el filtro sigue la
-- liga para que «Caducó» incluya lo que se capturó con un motivo que se juntó con él.
select w.id, w.item_type, coalesce(i.name, p.name)::text as item_name, w.quantity,
       coalesce(u.code, 'pza')::text as unit_code, w.total_cost, w.status, w.is_staff_meal,
       r.name as reason_name, coalesce(r.merged_into_id, r.id)::bigint as report_reason_id,
       uu.name as recorded_by_name, w.recorded_at
  from waste_records w
  join waste_reasons r on r.id = w.reason_id
  join users uu on uu.id = w.recorded_by
  left join ingredients i on i.id = w.ingredient_id
  left join units u on u.id = i.base_unit_id
  left join products p on p.id = w.product_id
 where w.branch_id = current_branch_id()
   and w.recorded_at >= sqlc.arg(from_ts) and w.recorded_at < sqlc.arg(to_ts)
   and (sqlc.narg(reason_id)::bigint is null or coalesce(r.merged_into_id, r.id) = sqlc.narg(reason_id)::bigint)
 order by w.recorded_at desc, w.id desc;

-- name: WasteByReason :many
-- Totales del periodo por motivo (el que queda tras juntar). Solo lo que restó: ni la por aprobar,
-- ni la rechazada, ni la deshecha, ni la que cubrió un conteo. Por la fecha en que restó (la de la
-- aprobación si la tuvo), como el faltante. La comida del personal por la marca copiada al registro,
-- no por la del motivo de hoy.
select coalesce(r.merged_into_id, r.id)::bigint as reason_id, w.is_staff_meal,
       count(*)::int as records,
       coalesce(sum(w.total_cost), 0)::numeric(14,2) as total_cost,
       count(*) filter (where w.total_cost is null)::int as without_cost
  from waste_records w
  join waste_reasons r on r.id = w.reason_id
 where w.branch_id = current_branch_id() and w.status = 'applied'
   and coalesce(w.resolved_at, w.recorded_at) >= sqlc.arg(from_ts)::timestamptz
   and coalesce(w.resolved_at, w.recorded_at) < sqlc.arg(to_ts)::timestamptz
 group by 1, 2
 order by 4 desc;

-- name: SetWasteApprovalThreshold :execrows
update business_settings set waste_approval_threshold = sqlc.narg(threshold), updated_at = now();

-- name: WasteIngredients :many
-- Lo que se puede registrar como artículo: los insumos activos que llevan existencia, con su unidad
-- base. Ruta propia porque la captura la usan cajero y cocina, que no entran al catálogo de insumos.
select i.id, i.name, u.id as unit_id, u.code as unit_code, u.kind::text as unit_kind
  from ingredients i join units u on u.id = i.base_unit_id
 where i.is_active and i.track_stock
 order by i.name, i.id;

-- name: WastePresentations :many
select id, ingredient_id, name, qty_in_base from ingredient_presentations where active order by ingredient_id, name;

-- name: WasteUnits :many
select id, code, kind::text as kind, to_base from units order by kind, to_base;

-- name: WasteProducts :many
-- Los productos que se pueden registrar: los activos que no son del sistema. Si no tienen qué
-- restar, el registro lo rechaza con su motivo.
select id, name from products where is_active and system_kind is null order by name, id;

-- name: RecentWasteItems :many
-- Lo último registrado en la sucursal, sin repetir, para los «recientes» de la captura.
select item_type::text as item_type, item_id
  from (select distinct on (item_type, coalesce(ingredient_id, product_id))
               item_type, coalesce(ingredient_id, product_id)::bigint as item_id, recorded_at
          from waste_records where branch_id = current_branch_id()
         order by item_type, coalesce(ingredient_id, product_id), recorded_at desc) w
 order by recorded_at desc
 limit 8;
