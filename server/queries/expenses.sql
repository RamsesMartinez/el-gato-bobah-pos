-- name: ListExpenseCategories :many
select id, name, financial_group from expense_categories where is_active order by financial_group, name;

-- name: CreateExpense :one
insert into expenses (expense_date, category_id, supplier_id, amount, payment_method_id, description, created_by)
values ($1, $2, $3, $4, $5, $6, $7)
returning id;

-- name: ListExpenses :many
select e.id, e.expense_date, ec.name as category, ec.financial_group, s.name as supplier,
       e.amount, e.currency, e.description
from expenses e
join expense_categories ec on ec.id = e.category_id
left join suppliers s on s.id = e.supplier_id
order by e.expense_date desc, e.id desc
limit $1;

-- name: ListSuppliers :many
select id, name from suppliers where is_active order by name;
