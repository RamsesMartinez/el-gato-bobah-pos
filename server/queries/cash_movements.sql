-- Movimientos de efectivo: entradas, salidas, traspasos, retiros y la salida de un gasto (spec 082,
-- fase 2: partido de cash.sql).

-- name: InsertExpenseCashMovement :exec
-- Salida de efectivo del cajón al pagar un gasto en efectivo (liga el gasto al corte). Lleva su pago
-- (spec 048): la base no deja una segunda salida del mismo pago. El monto lo fija el trigger de la
-- 0102 con importe + drawer_extra del pago; el que se manda aquí se ignora.
-- El 0 nunca queda: el trigger lo reemplaza, y si no corriera, el check de monto lo rechazaría.
insert into register_cash_movements (session_id, kind, amount, concept, expense_id, user_id, expense_payment_id)
values ($1, 'salida', 0, $2, $3, $4, $5);

-- name: DeleteExpenseSalidaOfPayment :execrows
-- Quitar un pago del arqueo de un corte ABIERTO borra su salida (spec 048, R2b): el corte no se ha
-- contado, y la bitácora guarda el cambio. Así volver a marcarlo no choca con la unique.
delete from register_cash_movements where expense_payment_id = $1 and kind = 'salida';

-- Movimientos de efectivo (entradas/salidas del cajón durante la sesión).
-- name: InsertCashMovement :one
insert into register_cash_movements (session_id, kind, amount, concept, user_id)
values ($1, $2, $3, $4, $5)
returning *;

-- name: ListCashMovements :many
-- drawer_extra: de la salida de un pago de gasto, la parte que no es del negocio (spec 050). El
-- corte la presenta en «No es del negocio» y no en «Gastos».
--
-- expense_id: no-null si el movimiento es la salida de un gasto → el front lo excluye de la tabla
-- de efectivo (los gastos van en su propia sección) para no contarlos dos veces.
--
-- is_refund: la salida de caja de una devolución (spec 029). El corte la presenta en
-- «Devoluciones» de su medio y no en «Salidas de efectivo»; el esperado no cambia, porque el neto
-- de movimientos la sigue restando.
select m.id, m.kind, m.amount, m.concept, m.created_at, u.name as user_name, m.transfer_id, m.expense_id, m.reverses_id,
       m.recipient_name,
       exists (select 1 from register_cash_movements x where x.reverses_id = m.id) as reversed,
       exists (select 1 from order_refunds r where r.cash_movement_id = m.id) as is_refund,
       coalesce(ep.drawer_extra, 0)::numeric(10,2) as drawer_extra
from register_cash_movements m
join users u on u.id = m.user_id
left join expense_payments ep on ep.id = m.expense_payment_id and m.kind = 'salida'
where m.session_id = $1
order by m.created_at;

-- Neto de efectivo movido en la sesión (entradas − salidas); suma al efectivo esperado al cerrar.
-- name: NetCashMovements :one
-- Un reverso solo corrige salidas (domain.CanReverse): devuelve al cajón lo que la salida restó.
-- Cada tipo con su signo y ninguno por descarte: el retiro al cierre (spec 035) se registra DESPUÉS
-- de contar, y restarlo dejaría el arqueo con un sobrante igual al retiro.
select coalesce(sum(case when kind in ('entrada', 'reverso') then amount
                         when kind in ('salida', 'propina') then -amount
                         else 0 end), 0)::numeric(10,2) as net
from register_cash_movements where session_id = $1;

-- Traspasos entre cajas: la fila de traspaso + cada pierna como movimiento ligado.
-- name: CreateCashTransfer :one
insert into cash_transfers (from_session_id, to_session_id, amount, note, created_by)
values ($1, $2, $3, $4, $5)
returning id;

-- name: InsertTransferMovement :exec
insert into register_cash_movements (session_id, kind, amount, concept, user_id, transfer_id)
values ($1, $2, $3, $4, $5, $6);

-- name: InsertWithdrawal :one
-- El retiro al cierre (spec 035): quién se lo llevó, con su nombre copiado para que renombrar al
-- usuario no reescriba un corte firmado. El índice único deja uno por turno.
insert into register_cash_movements (session_id, kind, amount, concept, user_id, recipient_user_id, recipient_name)
select sqlc.arg(session_id), 'retiro', sqlc.arg(amount), 'Retiro al cierre', sqlc.arg(user_id), u.id, u.name
  from users u where u.id = sqlc.arg(recipient_user_id) and u.is_active
returning id, recipient_name;
