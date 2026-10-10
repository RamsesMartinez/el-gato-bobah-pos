-- Propinas por entregar (spec 032). El pendiente se calcula siempre desde los hechos —cobros,
-- devoluciones y entregas— y nunca se guarda: un saldo guardado es una segunda verdad que diverge.

-- name: TipPaymentsForSession :many
-- Cada cobro del turno con propina, con lo que ya se entregó de él. Las entregas se pre-agregan por
-- cobro antes de unirse: son 1:N y unirlas tal cual multiplicaría la propina.
with entregado as (
  select s.order_payment_id, sum(s.amount) as paid_out
    from tip_payout_sources s
   where s.order_payment_id is not null
   group by s.order_payment_id
)
select op.id, op.order_id, pm.is_cash,
       op.tip_amount, coalesce(e.paid_out, 0)::numeric(10,2) as paid_out
  from order_payments op
  join payment_methods pm on pm.id = op.payment_method_id
  left join entregado e on e.order_payment_id = op.id
 where op.register_session_id = sqlc.arg(session_id)::bigint
   and op.tip_amount > 0
 order by op.created_at, op.id;

-- name: RefundedTipsByOrderForSession :many
-- Propina devuelta de los pedidos que cobraron propina en este turno, en el turno que sea: lo
-- devuelto deja de ser propina del personal aunque la devolución ocurra mañana.
select r.order_id, sum(r.tip_amount)::numeric(10,2) as refunded_tips
  from order_refunds r
 where r.tip_amount > 0
   and r.order_id in (select op.order_id from order_payments op
                       where op.register_session_id = sqlc.arg(session_id)::bigint and op.tip_amount > 0)
 group by r.order_id;

-- name: InheritedTipsForSession :one
-- Lo que el turno anterior de la MISMA caja dejó en caja, menos lo que este turno ya entregó de
-- ello. Por caja y no por empresa: con dos cajas abiertas cada una hereda solo lo suyo.
select coalesce((select p.tips_carried_over
                   from register_sessions p
                  where p.register_id = cur.register_id
                    and p.id <> cur.id
                    and p.opened_at < cur.opened_at
                  order by p.opened_at desc, p.id desc
                  limit 1), 0)::numeric(10,2) as carried_in,
       coalesce((select sum(s.amount)
                   from tip_payout_sources s
                   join register_cash_movements m on m.company_id = s.company_id and m.id = s.movement_id
                  where m.session_id = cur.id and s.order_payment_id is null), 0)::numeric(10,2) as used
  from register_sessions cur
 where cur.id = sqlc.arg(session_id)::bigint;

-- name: InsertTipMovement :one
insert into register_cash_movements (session_id, kind, amount, concept, user_id, recipient_user_id, recipient_name)
values (sqlc.arg(session_id), 'propina', sqlc.arg(amount), sqlc.arg(concept), sqlc.arg(user_id),
        sqlc.arg(recipient_user_id), sqlc.arg(recipient_name))
returning id;

-- name: InsertTipSource :exec
insert into tip_payout_sources (movement_id, order_payment_id, amount)
values ($1, sqlc.narg('order_payment_id'), $2);

-- name: LockSessionForTips :one
-- Dos tabletas repartiendo a la vez leerían el mismo pendiente y entregarían dos veces lo mismo;
-- el candado del turno las pone en fila.
-- Solo si sigue abierto: un cierre que confirmó mientras se esperaba ya heredó esta propina.
select id from register_sessions where id = $1 and status = 'abierta' for update;

-- name: SetTipsCarriedOver :exec
update register_sessions set tips_carried_over = $2 where id = $1;

-- name: TipPayoutTotalsForSession :one
-- Lo entregado en el turno y cuánto de eso fue propina de un medio que no es efectivo, pagada con
-- efectivo del cajón (punto 2). Lo heredado cuenta como efectivo: ya estaba en el cajón.
select coalesce(sum(s.amount), 0)::numeric(10,2) as paid_out,
       coalesce(sum(s.amount) filter (where pm.is_cash is false), 0)::numeric(10,2) as non_cash_paid_in_cash
  from tip_payout_sources s
  join register_cash_movements m on m.company_id = s.company_id and m.id = s.movement_id
  left join order_payments op on op.id = s.order_payment_id
  left join payment_methods pm on pm.id = op.payment_method_id
 where m.session_id = sqlc.arg(session_id)::bigint;
