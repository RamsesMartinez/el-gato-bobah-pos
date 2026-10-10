-- Conceptos de salida de caja (spec 032, punto 3). RLS acota a la empresa.

-- name: ListCashConcepts :many
select c.id, c.name, c.expense_category_id, c.supplier_id, ec.name as category_name, sp.name as supplier_name
  from cash_concepts c
  left join expense_categories ec on ec.id = c.expense_category_id
  left join suppliers sp on sp.id = c.supplier_id
 where c.archived_at is null
 order by lower(c.name);

-- name: FindActiveCashConceptByName :one
select id, name from cash_concepts where archived_at is null and lower(name) = lower(sqlc.arg(name)::text);

-- name: InsertCashConcept :one
insert into cash_concepts (name) values ($1) returning id, name;

-- name: GetCashConcept :one
select id, name, archived_at from cash_concepts where id = $1;

-- name: UpdateCashConcept :one
update cash_concepts set name = $2, expense_category_id = sqlc.narg('expense_category_id'),
       supplier_id = sqlc.narg('supplier_id')
 where id = $1 and archived_at is null
returning id, name;

-- name: ArchiveCashConcept :execrows
update cash_concepts set archived_at = now() where id = $1 and archived_at is null;

-- name: MergeCashConcept :execrows
-- El duplicado queda archivado y apuntando al que queda; sus salidas pasan a ese concepto.
update cash_concepts set archived_at = now(), merged_into_id = sqlc.arg(into_id)
 where id = sqlc.arg(id) and archived_at is null;

-- name: MoveCashOutsToConcept :exec
update register_cash_movements set concept_id = sqlc.arg(into_id) where concept_id = sqlc.arg(from_id);

-- name: GetMovementForCorrection :one
-- La salida que se corrige, con lo que decide si se puede: su caja (para el turno abierto donde cae
-- el reverso) y si ya tiene reverso.
select m.id, m.kind, m.amount, m.concept, m.session_id, s.register_id,
       (m.expense_id is not null)::boolean as is_expense,
       (m.transfer_id is not null)::boolean as is_transfer,
       exists (select 1 from order_refunds r where r.cash_movement_id = m.id) as is_refund,
       exists (select 1 from register_cash_movements x where x.reverses_id = m.id) as already_reversed
  from register_cash_movements m
  join register_sessions s on s.id = m.session_id
 where m.id = $1
 for update of m;

-- name: InsertCashOut :one
insert into register_cash_movements (session_id, kind, amount, concept, user_id, concept_id)
values ($1, 'salida', $2, $3, $4, $5)
returning id;

-- name: InsertCashReversal :one
insert into register_cash_movements (session_id, kind, amount, concept, user_id, reverses_id)
values ($1, 'reverso', $2, $3, $4, $5)
returning id;

-- name: CashOutsWithoutConceptInSession :one
-- Salidas a mano sin concepto del turno: las de antes de los conceptos. Gastos, traspasos y
-- devoluciones tienen el suyo.
select count(*)::int from register_cash_movements m
 where m.session_id = $1 and m.kind = 'salida' and m.concept_id is null
   and m.expense_id is null and m.transfer_id is null
   and not exists (select 1 from order_refunds r where r.cash_movement_id = m.id);
