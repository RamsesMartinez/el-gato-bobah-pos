-- Compras al almacén (spec 038): lo aplicado por renglón de compra, la corrección por diferencia,
-- lo que llega después y el costo del insumo. La empresa la pone RLS.

-- name: LockExpenseForStock :one
-- El gasto bloqueado mientras se suma o se corrige lo que llegó: dos tabletas que corrigen la misma
-- compra se forman, y la segunda compara contra lo ya aplicado por la primera (`seenApplied`).
select id, status, amount, received_at from expenses where id = $1 for update;

-- name: ListStockLines :many
-- Los renglones de almacén vivos de un gasto, con lo que hace falta para convertir a unidad base y
-- para saber cuánto falta: la unidad de compra, la base del artículo y lo ya aplicado.
select ei.id, ei.item_type, ei.ingredient_id, ei.product_id, ei.description, ei.quantity,
       ei.qty_received, ei.amount, ei.pack_qty_in_base, ei.presentation_id, ei.is_gift,
       ei.applied_qty_base, ei.not_coming_qty, ei.unit_id, ei.position,
       bu.kind as buy_kind, bu.to_base as buy_to_base,
       coalesce(iu.kind, 'pieza'::unit_kind) as base_kind
from expense_items ei
join units bu on bu.id = ei.unit_id
left join ingredients i on i.id = ei.ingredient_id
left join units iu on iu.id = i.base_unit_id
where ei.expense_id = $1 and ei.item_type is not null and ei.voided_at is null
order by ei.position, ei.id;

-- name: LockIngredientStockNowait :many
-- Las existencias de los insumos de una compra, en orden y SIN ESPERAR. Una venta las bloquea en el
-- orden de su ticket; si la compra esperara aquí podría cerrar un ciclo y Postgres cancelaría una
-- de las dos —a veces el cobro—. Tomadas antes de escribir, la compra ya no espera por nada (el
-- mismo arreglo del 037 en el cambio de modo).
select ingredient_id from stock_levels
 where item_type = 'ingrediente' and ingredient_id = any(sqlc.arg(ids)::bigint[]) and branch_id = current_branch_id()
 order by ingredient_id
   for update nowait;

-- name: LockIngredientCostNowait :one
-- El insumo bloqueado para recalcular su costo: dos compras del mismo insumo con promedio ponderado
-- no pueden promediar las dos sobre el costo viejo.
select current_cost from ingredients where id = $1 for no key update nowait;

-- name: IngredientOnHandTotal :one
-- Lo guardado de un insumo en TODA la empresa: el costo es de la empresa, no de la sucursal.
select coalesce(sum(on_hand), 0)::numeric(14,4) as on_hand from stock_levels where ingredient_id = $1;

-- name: CostingMethod :one
-- Sin fila de configuración, el método por omisión (última compra).
select coalesce((select costing_method from business_settings limit 1), 'last')::text as costing_method;

-- name: InsertLineMovement :exec
-- Un movimiento de compra ligado a su gasto y a su RENGLÓN, con la sucursal de la sesión: con dos
-- sucursales y sin selector, `current_branch_id()` truena (EGB01) en vez de adivinar.
insert into stock_movements (
  item_type, ingredient_id, product_id, movement_type, quantity, unit_cost, expense_id, expense_item_id,
  user_id, reason, branch_id
) values (
  sqlc.arg(item_type), sqlc.narg(ingredient_id), sqlc.narg(product_id), 'compra', sqlc.arg(quantity),
  sqlc.narg(unit_cost), sqlc.arg(expense_id), sqlc.arg(expense_item_id), sqlc.arg(user_id),
  sqlc.arg(reason), current_branch_id()
);

-- name: AddAppliedQty :exec
-- En la misma transacción que el movimiento: lo aplicado del renglón es la suma de sus movimientos.
update expense_items set applied_qty_base = applied_qty_base + sqlc.arg(delta) where id = sqlc.arg(id);

-- name: SetExpenseReceivedAt :exec
-- La fecha en que ya no quedó nada pendiente, o nula si todavía falta algo (R2b).
update expenses set received_at = sqlc.narg(received_at) where id = sqlc.arg(id);

-- name: UpdateStockLine :execrows
-- Corregir un renglón: lo pedido, lo que llegó, el empaque y el importe. Lo aplicado no se toca aquí:
-- se mueve con su movimiento.
update expense_items
   set item_type = sqlc.arg(item_type), ingredient_id = sqlc.narg(ingredient_id), product_id = sqlc.narg(product_id),
       description = sqlc.arg(description), quantity = sqlc.arg(quantity), unit_id = sqlc.arg(unit_id),
       qty_received = sqlc.narg(qty_received), amount = sqlc.arg(amount), unit_cost = sqlc.arg(unit_cost),
       pack_qty_in_base = sqlc.narg(pack_qty_in_base), presentation_id = sqlc.narg(presentation_id),
       is_gift = sqlc.arg(is_gift), not_coming_qty = sqlc.arg(not_coming_qty)
 where id = sqlc.arg(id) and expense_id = sqlc.arg(expense_id) and voided_at is null;

-- name: VoidStockLine :execrows
-- Quitar un renglón en una corrección: se marca, no se borra (sus movimientos lo referencian).
update expense_items set voided_at = now() where id = $1 and expense_id = $2 and voided_at is null;

-- name: GetPresentationForLine :one
-- El empaque elegido en un renglón: de qué insumo es y qué trae, para copiarlo al renglón.
select ingredient_id, qty_in_base, is_approximate from ingredient_presentations where id = $1 and active;

-- name: PieceUnitID :one
select id from units where code = 'pieza';

-- name: FillMissingReceived :exec
-- «Llegó todo» sobre un gasto capturado antes: lo que no tenía cantidad recibida llega completo,
-- menos lo que ya se dijo que no va a llegar.
update expense_items set qty_received = quantity - not_coming_qty
 where expense_id = $1 and item_type is not null and voided_at is null and qty_received is null;

-- name: LockIngredientsNowait :many
-- Los insumos de una compra, ANTES de escribir un solo renglón: la llave foránea del renglón toma
-- el insumo en modo compartido y esperaría a una venta que lo tiene tomado. En orden y sin esperar.
-- `no key update` y no `update`: la llave foránea de los movimientos de una venta toma el insumo en
-- `key share`, que no choca con éste; así una compra en curso no frena el cobro.
select id from ingredients where id = any(sqlc.arg(ids)::bigint[]) order by id for no key update nowait;
