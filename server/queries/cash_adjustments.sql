-- Correcciones de un corte cerrado (#290, spec 082 fase 3). Solo se agregan: no hay update ni delete.

-- name: LockSessionForAdjustment :one
-- El corte que se corrige, tomado para que dos tabletas no corrijan ni reviertan a la vez sobre la
-- misma cifra vigente (cada corrección guarda el «antes» que vio).
select id, status, float_left, register_id from register_sessions where id = $1 for update;

-- name: InsertSessionAdjustment :one
insert into cash_session_adjustments (session_id, kind, before_amount, after_amount, reason, note, reverses_id, created_by)
values ($1, $2, $3, $4, $5, $6, $7, $8)
returning id, created_at;

-- name: ListSessionAdjustments :many
-- Las correcciones de un corte en el orden en que se registraron, con quién las hizo.
select a.id, a.session_id, a.kind, a.before_amount, a.after_amount, a.reason, a.note, a.reverses_id,
       a.created_at, u.name as created_by_name
from cash_session_adjustments a
join users u on u.id = a.created_by
where a.session_id = $1
order by a.id;

-- name: ListAdjustmentsForSessions :many
-- Las correcciones de varios cortes a la vez: la lista del Histórico y la suma del periodo calculan
-- la diferencia corregida de cada corte sin una consulta por renglón.
select a.id, a.session_id, a.kind, a.after_amount, a.reverses_id
from cash_session_adjustments a
where a.session_id = any(@session_ids::bigint[])
order by a.session_id, a.id;
