-- Conteo de existencias (spec 038, historia 3). La empresa la pone RLS; la sucursal es la del conteo.

-- name: OpenStockCount :one
-- La sucursal la resuelve la base: con dos y sin selector, `current_branch_id()` truena (EGB01).
-- Un segundo conteo abierto en la sucursal choca con `stock_counts_one_open` (23505 → 409).
insert into stock_counts (branch_id, opened_by) values (current_branch_id(), sqlc.arg(opened_by))
returning id, opened_at;

-- name: CurrentStockCount :one
select id, opened_at, opened_by from stock_counts where status = 'open' and branch_id = current_branch_id();

-- name: LockStockCount :one
-- El conteo bloqueado: capturar y cerrar se forman, y una cifra no entra después del cierre.
select id, branch_id, status from stock_counts where id = $1 for update;

-- name: CountItems :many
-- Lo que se cuenta: los insumos activos que llevan existencia, por lugar (los sin lugar al final)
-- y por nombre. Lo que el sistema tiene NO viaja: el conteo es a ciegas.
select i.id, i.name, cp.name as count_place, i.count_place_id, u.code as base_unit_code, l.counted_qty, l.captured_at
  from ingredients i
  join units u on u.id = i.base_unit_id
  left join count_places cp on cp.id = i.count_place_id
  left join stock_count_lines l on l.ingredient_id = i.id and l.count_id = sqlc.arg(count_id)
 where i.is_active and i.track_stock
 order by cp.position is null, cp.position, i.name, i.id;

-- name: OnHandInBranch :one
select coalesce((select on_hand from stock_levels
                  where item_type = 'ingrediente' and ingredient_id = sqlc.arg(ingredient_id)
                    and branch_id = sqlc.arg(branch_id)), 0)::numeric(14,4) as on_hand;

-- name: LockStockLevelForCapture :exec
-- Toma la existencia del insumo antes de leerla: una merma o una venta en curso termina antes. Va
-- en una sentencia APARTE de la lectura: en una sola, Postgres relee la fila esperada pero el resto
-- de la consulta sale de la foto tomada antes de esperar, y el último movimiento quedaba viejo (la
-- merma que acababa de confirmar caía fuera del periodo del faltante).
select 1 from stock_levels
 where item_type = 'ingrediente' and ingredient_id = sqlc.arg(ingredient_id)::bigint and branch_id = sqlc.arg(branch_id)::bigint
   for share;

-- name: OnHandForCapture :one
-- Lo que el sistema tiene del insumo y el último movimiento que lo formó, en la misma foto (spec
-- 039, R2): el periodo del faltante se ancla por ese id y no por hora.
select coalesce((select sl.on_hand from stock_levels sl
                  where sl.item_type = 'ingrediente' and sl.ingredient_id = sqlc.arg(ingredient_id)::bigint
                    and sl.branch_id = sqlc.arg(branch_id)::bigint), 0)::numeric(14,4) as on_hand,
       coalesce((select max(sm.id) from stock_movements sm
                  where sm.ingredient_id = sqlc.arg(ingredient_id)::bigint
                    and sm.branch_id = sqlc.arg(branch_id)::bigint), 0)::bigint as last_movement_id;

-- name: UpsertCountLine :exec
-- Recapturar manda: la lectura nueva reemplaza cifra, lo que había en el sistema y su último
-- movimiento.
insert into stock_count_lines (count_id, ingredient_id, counted_qty, system_qty, captured_by, last_movement_id)
values (sqlc.arg(count_id), sqlc.arg(ingredient_id), sqlc.arg(counted_qty), sqlc.arg(system_qty), sqlc.arg(captured_by),
        sqlc.arg(last_movement_id))
on conflict (count_id, ingredient_id) do update
   set counted_qty = excluded.counted_qty, system_qty = excluded.system_qty,
       captured_by = excluded.captured_by, captured_at = now(), last_movement_id = excluded.last_movement_id;

-- name: SaveCountLineCosts :exec
-- Al cerrar, el costo de cada insumo contado: el reporte del faltante de este conteo no cambia
-- cuando después cambia el costo (spec 039, revisión de arquitectura).
update stock_count_lines l set unit_cost = i.current_cost
  from ingredients i where l.count_id = $1 and i.id = l.ingredient_id;

-- name: CountedLinesSince :many
-- Los insumos que capturó un conteo y cuándo, para cubrir las mermas por aprobar que vio.
select ingredient_id, captured_at from stock_count_lines where count_id = $1;

-- name: CountLossLines :many
-- El faltante de un conteo cerrado, por insumo: lo contado, lo que el sistema tenía, su costo al
-- cerrar, y la merma que restó entre lo que vio el conteo anterior de ese insumo y lo que vio
-- éste. Sin conteo anterior comparable (`has_previous` falso), no hay periodo.
select l.ingredient_id, i.name, u.code as base_unit_code, l.counted_qty, l.system_qty,
       l.unit_cost, (prev.last_movement_id is not null and l.last_movement_id is not null)::bool as has_previous,
       coalesce((select -sum(m.quantity) from stock_movements m
                  where m.waste_record_id is not null and m.ingredient_id = l.ingredient_id
                    and m.branch_id = c.branch_id
                    and m.id > prev.last_movement_id and m.id <= l.last_movement_id), 0)::numeric(14,4) as registered
  from stock_count_lines l
  join stock_counts c on c.id = l.count_id
  join ingredients i on i.id = l.ingredient_id
  join units u on u.id = i.base_unit_id
  left join lateral (
    select pl.last_movement_id from stock_count_lines pl join stock_counts pc on pc.id = pl.count_id
     where pc.status = 'closed' and pc.branch_id = c.branch_id and pc.id <> c.id
       and pl.ingredient_id = l.ingredient_id and pl.captured_at < l.captured_at
     order by pl.captured_at desc limit 1
  ) prev on true
 where l.count_id = sqlc.arg(count_id) and c.status = 'closed'
 order by i.name, l.ingredient_id;

-- name: DeleteCountLine :execrows
delete from stock_count_lines where count_id = $1 and ingredient_id = $2;

-- name: CountLines :many
select l.ingredient_id, i.name, u.code as base_unit_code, l.counted_qty, l.system_qty
  from stock_count_lines l
  join ingredients i on i.id = l.ingredient_id
  join units u on u.id = i.base_unit_id
 where l.count_id = $1
 order by l.ingredient_id;

-- name: NotCountedIngredients :many
select i.id, i.name from ingredients i
 where i.is_active and i.track_stock
   and not exists (select 1 from stock_count_lines l where l.count_id = $1 and l.ingredient_id = i.id)
 order by i.name, i.id;

-- name: LockStockLevelsInBranchNowait :many
-- Las existencias que el cierre va a ajustar, en orden y sin esperar a una venta (mismo arreglo
-- que las compras): si una venta las tiene, el cierre se reintenta en vez de formar un ciclo.
select ingredient_id from stock_levels
 where item_type = 'ingrediente' and ingredient_id = any(sqlc.arg(ids)::bigint[]) and branch_id = sqlc.arg(branch_id)
 order by ingredient_id
   for update nowait;

-- name: InsertCountMovement :exec
-- Un ajuste de conteo, con su conteo, quién lo cerró y la sucursal del conteo (SC-003).
insert into stock_movements (item_type, ingredient_id, movement_type, quantity, stock_count_id, user_id, reason, branch_id)
values ('ingrediente', sqlc.arg(ingredient_id), 'ajuste', sqlc.arg(quantity), sqlc.arg(stock_count_id),
        sqlc.arg(user_id), sqlc.arg(reason), sqlc.arg(branch_id));

-- name: FinishStockCount :execrows
update stock_counts set status = sqlc.arg(status), closed_by = sqlc.arg(closed_by), closed_at = now()
 where id = sqlc.arg(id) and status = 'open';

-- name: LastClosedStockCount :one
-- Sin `max()`: sobre ningún conteo devuelve NULL y el scan a un tipo no nulable truena. Sin filas
-- es «nunca se ha cerrado uno».
select id, closed_at from stock_counts
 where status = 'closed' and branch_id = current_branch_id()
 order by closed_at desc limit 1;

-- name: SetCountPlace :execrows
update ingredients set count_place_id = sqlc.narg(place_id), updated_at = now() where id = sqlc.arg(id);

-- name: MarkCountPreviewed :exec
-- Solo la primera vez: lo que importa es qué cambió desde que se vieron las diferencias.
update stock_counts set previewed_at = coalesce(previewed_at, now()) where id = $1 and status = 'open';

-- name: CountChangedAfterPreview :one
select count(*)::int from stock_count_lines l
  join stock_counts c on c.id = l.count_id
 where l.count_id = $1 and c.previewed_at is not null and l.captured_at > c.previewed_at;
