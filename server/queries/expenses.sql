-- ==== Categorías de gasto ====

-- name: ListExpenseCategories :many
-- Activas, para el formulario de gasto.
select id, name, financial_group from expense_categories where is_active order by financial_group, name;

-- name: ListAllExpenseCategories :many
-- Todas (incl. inactivas), para la gestión.
select id, name, financial_group, is_active from expense_categories order by financial_group, name;

-- name: CreateExpenseCategory :one
insert into expense_categories (name, financial_group) values ($1, $2)
returning id, name, financial_group, is_active;

-- name: UpdateExpenseCategory :one
update expense_categories set name = $2, financial_group = $3, is_active = $4 where id = $1
returning id, name, financial_group, is_active;

-- ==== Proveedores ====

-- name: ListSuppliers :many
-- Activos, para el formulario de gasto.
select id, name from suppliers where is_active order by name;

-- name: ListAllSuppliers :many
select id, name, phone, notes, is_active from suppliers order by name;

-- name: CreateSupplier :one
insert into suppliers (name, phone, notes) values ($1, $2, $3)
returning id, name, phone, notes, is_active;

-- name: UpdateSupplier :one
update suppliers set name = $2, phone = $3, notes = $4, is_active = $5 where id = $1
returning id, name, phone, notes, is_active;

-- ==== Gastos ====

-- name: CreateExpense :one
-- expense_date la manda el llamador (antes se forzaba a hoy): es la fecha del DOCUMENTO, y una
-- factura se captura días después de emitirse. received_at va aparte, al recibir la mercancía.
insert into expenses (
  expense_date, category_id, supplier_id, amount, description, created_by,
  status, paid_at, paid_by, received_at, doc_kind, doc_folio, doc_raw
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
returning id;

-- name: GetExpense :one
select * from expenses where id = $1;

-- name: GetExpenseView :one
-- El encabezado ya resuelto (categoría, proveedor, quién lo capturó) para la pantalla de
-- detalle. GetExpense devuelve la fila cruda y la usa el servicio para decidir transiciones.
select e.id, e.expense_date, e.received_at, e.status, ec.name as category, ec.financial_group,
       s.name as supplier, e.amount, e.currency, e.description, e.doc_kind, e.doc_folio,
       e.paid_at, ub.name as created_by_name
from expenses e
join expense_categories ec on ec.id = e.category_id
left join suppliers s on s.id = e.supplier_id
left join users ub on ub.id = e.created_by
where e.id = $1;

-- name: ListExpenses :many
-- payment_method se agrega desde expense_payments: un gasto puede tener varios medios
-- ("Tarjeta + Efectivo"), así que ya no es una columna del encabezado.
-- Orden por columna: @sort ∈ (''|date|status|category|supplier|description|amount) × @dir
-- (asc|desc); default fecha desc. Va en SQL y no en el cliente porque la lista está paginada:
-- ordenar solo las 20 filas visibles daría un orden falso.
select e.id, e.expense_date, e.received_at, e.status, ec.name as category, ec.financial_group,
       s.name as supplier, e.amount, e.currency, e.description, e.doc_kind, e.doc_folio,
       e.paid_at, ub.name as created_by_name,
       (select string_agg(distinct pm.name, ' + ' order by pm.name)
          from expense_payments ep join payment_methods pm on pm.id = ep.payment_method_id
         where ep.expense_id = e.id) as payment_method,
       (select count(*) from expense_items ei where ei.expense_id = e.id) as item_count
from expenses e
join expense_categories ec on ec.id = e.category_id
left join suppliers s on s.id = e.supplier_id
left join users ub on ub.id = e.created_by
where (sqlc.narg('status')::expense_status is null or e.status = sqlc.narg('status'))
  and (sqlc.narg('pending_receipt')::boolean is not true or e.received_at is null)
order by
  case when @sort::text = 'amount'      and @dir::text = 'asc'  then e.amount end asc  nulls last,
  case when @sort::text = 'amount'      and @dir::text <> 'asc' then e.amount end desc nulls last,
  case when @sort::text = 'status'      and @dir::text = 'asc'  then e.status::text end asc  nulls last,
  case when @sort::text = 'status'      and @dir::text <> 'asc' then e.status::text end desc nulls last,
  case when @sort::text = 'category'    and @dir::text = 'asc'  then ec.name end asc  nulls last,
  case when @sort::text = 'category'    and @dir::text <> 'asc' then ec.name end desc nulls last,
  case when @sort::text = 'supplier'    and @dir::text = 'asc'  then s.name end asc  nulls last,
  case when @sort::text = 'supplier'    and @dir::text <> 'asc' then s.name end desc nulls last,
  case when @sort::text = 'description' and @dir::text = 'asc'  then e.description end asc  nulls last,
  case when @sort::text = 'description' and @dir::text <> 'asc' then e.description end desc nulls last,
  case when @sort::text = 'date'        and @dir::text = 'asc'  then e.expense_date end asc,
  e.expense_date desc, e.id desc
limit sqlc.arg('lim') offset sqlc.arg('off');

-- name: CountExpenses :one
select count(*) from expenses e
where (sqlc.narg('status')::expense_status is null or e.status = sqlc.narg('status'))
  and (sqlc.narg('pending_receipt')::boolean is not true or e.received_at is null);

-- name: CancelExpense :execrows
update expenses set status = 'cancelada', cancelled_at = now(), cancelled_by = $2, cancel_reason = $3
where id = $1 and status = 'pendiente';

-- ==== Líneas del gasto (mercancía) ====

-- name: CreateExpenseItem :one
insert into expense_items (
  expense_id, item_type, ingredient_id, product_id, description,
  quantity, unit_id, qty_received, unit_cost, amount, pack_qty_in_base, position
) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
returning id;

-- name: ListExpenseItems :many
select ei.id, ei.item_type, ei.ingredient_id, ei.product_id, ei.description,
       ei.quantity, ei.unit_id, u.code as unit_code, u.kind as unit_kind,
       ei.qty_received, ei.unit_cost, ei.amount, ei.pack_qty_in_base, ei.position,
       i.name as ingredient_name, p.name as product_name
from expense_items ei
left join units u on u.id = ei.unit_id
left join ingredients i on i.id = ei.ingredient_id
left join products p on p.id = ei.product_id
where ei.expense_id = $1
order by ei.position, ei.id;

-- name: SetExpenseItemReceived :execrows
update expense_items set qty_received = $2 where id = $1 and expense_id = $3;

-- name: DeleteExpenseItems :exec
delete from expense_items where expense_id = $1;

-- ==== Recepción de mercancía ====

-- name: MarkExpenseReceived :execrows
-- El AND received_at is null es el guard de idempotencia: un doble-tap no puede generar dos
-- veces los movimientos de almacén de la misma compra.
update expenses set received_at = $2 where id = $1 and received_at is null;

-- name: ItemsToDeplete :many
-- Líneas inventariables de un gasto con lo que hace falta para convertir a unidad base:
-- el kind/factor de la unidad de compra y el kind de la unidad base del artículo.
-- Las líneas con qty_received null o 0 quedan fuera: no llegaron.
select ei.id, ei.item_type, ei.ingredient_id, ei.product_id, ei.description,
       ei.qty_received, ei.amount, ei.pack_qty_in_base,
       bu.kind as buy_kind, bu.to_base as buy_to_base,
       coalesce(iu.kind, 'pieza'::unit_kind) as base_kind
from expense_items ei
join units bu on bu.id = ei.unit_id
left join ingredients i on i.id = ei.ingredient_id
left join units iu on iu.id = i.base_unit_id
where ei.expense_id = $1 and ei.item_type is not null
  and ei.qty_received is not null and ei.qty_received > 0
order by ei.position, ei.id;

-- name: InsertPurchaseMovement :exec
-- Movimiento de compra ligado al gasto. expense_id ya existía en stock_movements: el gancho
-- gasto→almacén estaba en el esquema desde 0008 y esto es lo que finalmente lo usa.
insert into stock_movements (
  item_type, ingredient_id, product_id, movement_type, quantity, unit_cost, expense_id, user_id, reason
) values ($1,$2,$3,'compra',$4,$5,$6,$7,$8);

-- ==== Pagos del gasto ====

-- name: CreateExpensePayment :one
insert into expense_payments (
  expense_id, payment_method_id, amount, paid_on, register_session_id, reference, paid_by
) values ($1,$2,$3,$4,$5,$6,$7)
returning id;

-- name: ListExpensePayments :many
select ep.id, ep.payment_method_id, pm.name as method, pm.affects_cash_drawer,
       ep.amount, ep.paid_on, ep.register_session_id, ep.reference, ep.created_at
from expense_payments ep
join payment_methods pm on pm.id = ep.payment_method_id
where ep.expense_id = $1
order by ep.paid_on, ep.id;

-- name: SumExpensePayments :one
select coalesce(sum(amount), 0)::numeric(10,2) as paid from expense_payments where expense_id = $1;

-- name: MarkExpensePaid :execrows
-- Sin payment_method_id: con qué se pagó vive en expense_payments desde 0029. El guard de
-- estado sigue siendo la carrera entre el GET y el UPDATE.
update expenses set status = 'pagada', paid_at = now(), paid_by = $2
where id = $1 and status = 'pendiente';
