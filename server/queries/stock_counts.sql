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
select i.id, i.name, i.count_place, u.code as base_unit_code, l.counted_qty, l.captured_at
  from ingredients i
  join units u on u.id = i.base_unit_id
  left join stock_count_lines l on l.ingredient_id = i.id and l.count_id = sqlc.arg(count_id)
 where i.is_active and i.track_stock
 order by i.count_place is null, i.count_place, i.name, i.id;

-- name: OnHandInBranch :one
select coalesce((select on_hand from stock_levels
                  where item_type = 'ingrediente' and ingredient_id = sqlc.arg(ingredient_id)
                    and branch_id = sqlc.arg(branch_id)), 0)::numeric(14,4) as on_hand;

-- name: UpsertCountLine :exec
-- Recapturar manda: la lectura nueva reemplaza cifra y lo que había en el sistema.
insert into stock_count_lines (count_id, ingredient_id, counted_qty, system_qty, captured_by)
values (sqlc.arg(count_id), sqlc.arg(ingredient_id), sqlc.arg(counted_qty), sqlc.arg(system_qty), sqlc.arg(captured_by))
on conflict (count_id, ingredient_id) do update
   set counted_qty = excluded.counted_qty, system_qty = excluded.system_qty,
       captured_by = excluded.captured_by, captured_at = now();

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
select closed_at from stock_counts
 where status = 'closed' and branch_id = current_branch_id()
 order by closed_at desc limit 1;

-- name: SetCountPlace :execrows
update ingredients set count_place = sqlc.narg(place), updated_at = now() where id = sqlc.arg(id);
