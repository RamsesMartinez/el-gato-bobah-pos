-- Medios de pago (lookup)

-- name: ListPaymentMethods :many
select id, name, kind, affects_cash_drawer from payment_methods where is_active order by sort_key, name;

-- Cortes de caja

-- name: GetOpenSession :one
select * from register_sessions where status = 'abierta' limit 1;

-- name: OpenSession :one
insert into register_sessions (business_date, opening_cash, opened_by)
values ($1, $2, $3)
returning *;

-- name: CloseSession :exec
update register_sessions set status = 'cerrada', closed_by = $2, closed_at = now(), notes = $3
where id = $1;

-- name: SaveSessionTotal :exec
insert into register_session_totals (session_id, payment_method_id, expected, declared)
values ($1, $2, $3, $4);

-- name: ListSessions :many
select id, business_date, status, opening_cash, opened_at, closed_at from register_sessions
order by opened_at desc limit $1;

-- Totales esperados por método desde la apertura de la sesión (ventana temporal).
-- name: ExpectedByMethodSince :many
select pm.id as payment_method_id, pm.name, pm.affects_cash_drawer,
       coalesce(sum(op.amount), 0)::numeric(10,2) as expected
from payment_methods pm
left join order_payments op on op.payment_method_id = pm.id and op.created_at >= $1
where pm.is_active
group by pm.id, pm.name, pm.affects_cash_drawer
order by pm.sort_key;
