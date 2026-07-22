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
insert into expenses (
  expense_date, category_id, supplier_id, amount, description, created_by,
  status, payment_method_id, register_session_id, paid_at, paid_by
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
returning id;

-- name: GetExpense :one
select * from expenses where id = $1;

-- name: ListExpenses :many
select e.id, e.expense_date, e.status, ec.name as category, ec.financial_group,
       s.name as supplier, e.amount, e.currency, e.description,
       pm.name as payment_method, e.paid_at, e.register_session_id, ub.name as created_by_name
from expenses e
join expense_categories ec on ec.id = e.category_id
left join suppliers s on s.id = e.supplier_id
left join payment_methods pm on pm.id = e.payment_method_id
left join users ub on ub.id = e.created_by
where (sqlc.narg('status')::expense_status is null or e.status = sqlc.narg('status'))
order by e.expense_date desc, e.id desc
limit sqlc.arg('lim') offset sqlc.arg('off');

-- name: CountExpenses :one
select count(*) from expenses e
where (sqlc.narg('status')::expense_status is null or e.status = sqlc.narg('status'));

-- name: PayExpense :execrows
-- El AND status='pendiente' es el guard de carrera (idempotente); el servicio ya valida el estado.
update expenses set status = 'pagada', payment_method_id = $2, register_session_id = $3,
       paid_at = now(), paid_by = $4
where id = $1 and status = 'pendiente';

-- name: CancelExpense :execrows
update expenses set status = 'cancelada', cancelled_at = now(), cancelled_by = $2, cancel_reason = $3
where id = $1 and status = 'pendiente';
