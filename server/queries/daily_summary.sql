-- Resumen diario del cierre por correo (spec 032, punto 7). Todo bajo RLS: corre dentro de la
-- transacción de UNA empresa.

-- name: GetSummaryEmails :one
select daily_summary_emails from business_settings;

-- name: SetSummaryEmails :execrows
update business_settings set daily_summary_emails = $1, updated_at = now();

-- name: DaysReadyForSummary :many
-- Días de negocio recientes con al menos un turno cerrado, ninguno abierto, y sin resumen enviado ni
-- agotado. Con dos cajas el día espera a que cierren las dos.
select distinct s.business_date
  from register_sessions s
 where s.status = 'cerrada' and s.business_date >= sqlc.arg(since)::date
   and not exists (select 1 from register_sessions o where o.status = 'abierta' and o.business_date = s.business_date)
   and not exists (select 1 from daily_summary_sends d
                    where d.business_date = s.business_date and (d.status = 'sent' or d.attempts >= 5))
 order by s.business_date;

-- name: ClaimSummarySend :one
-- La fila se toma ANTES de mandar: un reintento o una pasada paralela no manda dos veces. Solo se
-- vuelve a tomar una fallida (o una pendiente que quedó colgada más de 10 minutos).
insert into daily_summary_sends (business_date, status, attempts) values ($1, 'pending', 1)
on conflict (company_id, business_date) do update
   set status = 'pending', attempts = daily_summary_sends.attempts + 1, last_error = null
 where daily_summary_sends.attempts < 5
   and (daily_summary_sends.status = 'failed'
        or (daily_summary_sends.status = 'pending' and daily_summary_sends.created_at < now() - interval '10 minutes'))
returning id;

-- name: MarkSummarySent :exec
update daily_summary_sends set status = 'sent', sent_at = now() where id = $1;

-- name: MarkSummaryFailed :exec
update daily_summary_sends set status = 'failed', last_error = left($2, 500) where id = $1;

-- name: DaySummaryMoney :one
-- Las cifras del día, cada rama pre-agregada por su cuenta (1:N con los turnos). Ventas sin
-- propinas; propinas cobradas, entregadas y por entregar aparte: no son del negocio.
with t as (
  select rs.id, rs.tips_carried_over from register_sessions rs where rs.business_date = sqlc.arg(day)::date and rs.status = 'cerrada'
), pagos as (
  select coalesce(sum(op.amount), 0) as ventas, coalesce(sum(op.tip_amount), 0) as propinas
    from order_payments op where op.register_session_id in (select id from t)
), mov as (
  select coalesce(sum(m.amount) filter (where m.kind = 'propina'), 0) as entregadas,
         count(*) filter (where m.kind = 'salida' and m.concept_id is null and m.expense_id is null
                          and m.transfer_id is null
                          and not exists (select 1 from order_refunds r where r.cash_movement_id = m.id)
                          and not exists (select 1 from register_cash_movements x where x.reverses_id = m.id)) as sin_concepto
    from register_cash_movements m where m.session_id in (select id from t)
), cajon as (
  select coalesce(sum(c.difference), 0) as diferencia
    from session_cash_counts c where c.moment = 'cierre' and c.session_id in (select id from t)
), term as (
  select coalesce(sum(declared - expected), 0) as diferencia from session_terminal_counts where session_id in (select id from t)
)
select (select count(*) from t)::int as shifts,
       pagos.ventas::numeric(12,2) as sales, pagos.propinas::numeric(12,2) as tips,
       mov.entregadas::numeric(12,2) as tips_paid_out,
       (select coalesce(sum(tips_carried_over), 0) from t)::numeric(12,2) as tips_pending,
       mov.sin_concepto::int as cash_outs_without_concept,
       cajon.diferencia::numeric(12,2) as drawer_difference,
       term.diferencia::numeric(12,2) as terminal_difference
  from pagos, mov, cajon, term;

-- name: DaySummaryCashOuts :many
-- Salidas a mano por concepto (sin gastos, traspasos, devoluciones ni propinas), netas de reversos.
select coalesce(c.name, 'Sin concepto') as concept,
       (sum(m.amount) - coalesce(sum(rv.amount), 0))::numeric(12,2) as total
  from register_cash_movements m
  join register_sessions s on s.id = m.session_id
  left join cash_concepts c on c.id = m.concept_id
  left join register_cash_movements rv on rv.reverses_id = m.id
 where s.business_date = sqlc.arg(day)::date and s.status = 'cerrada' and m.kind = 'salida'
   and m.expense_id is null and m.transfer_id is null
   and not exists (select 1 from order_refunds r where r.cash_movement_id = m.id)
 group by coalesce(c.name, 'Sin concepto')
 order by 2 desc;
