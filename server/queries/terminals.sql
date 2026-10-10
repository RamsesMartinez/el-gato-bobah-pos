-- Terminales de tarjeta por sucursal (spec 032, puntos 8 y 9). RLS acota a la empresa.

-- name: ListCardTerminals :many
select t.id, t.branch_id, b.name as branch_name, t.name, (t.archived_at is not null)::boolean as archived
  from card_terminals t
  join branches b on b.id = t.branch_id
 order by b.branch_number, t.archived_at nulls first, lower(t.name);

-- name: InsertCardTerminal :one
insert into card_terminals (branch_id, name) values ($1, $2) returning id, branch_id, name;

-- name: RenameCardTerminal :one
update card_terminals set name = $2 where id = $1 and archived_at is null returning id, branch_id, name;

-- name: ArchiveCardTerminal :execrows
update card_terminals set archived_at = now() where id = $1 and archived_at is null;

-- name: ActiveTerminalsOfRegisterBranch :many
-- Las terminales activas de la sucursal de una caja: entre ellas se elige al cobrar.
select t.id, t.name
  from card_terminals t
  join cash_registers r on r.branch_id = t.branch_id
 where r.id = $1 and t.archived_at is null
 order by t.id;

-- name: SetPaymentTerminal :exec
update order_payments set card_terminal_id = $2, card_terminal_name = $3 where id = $1;

-- name: SetBranchCardCountMode :execrows
update branches set card_count_mode = $2 where id = $1;

-- name: ListBranchCardCountModes :many
select id, name, card_count_mode from branches where is_active order by branch_number;

-- name: TerminalCollectedForSession :many
-- Lo cobrado por terminal en el turno, menos lo devuelto con tarjeta en el turno. La devolución no
-- guarda terminal: se le atribuye la del último cobro con tarjeta de su pedido (donde se devuelve).
-- Cobros y devoluciones son dos 1:N: cada rama se pre-agrega por terminal antes de unirse.
with cobrado as (
  select op.card_terminal_id, max(op.card_terminal_name) as name, sum(op.amount + op.tip_amount) as total
    from order_payments op
   where op.register_session_id = sqlc.arg(session_id)::bigint and op.card_terminal_id is not null
   group by op.card_terminal_id
), devuelto as (
  select t.card_terminal_id, sum(r.amount + r.tip_amount) as total
    from order_refunds r
    join payment_methods pm on pm.id = r.payment_method_id and pm.kind = 'tarjeta'
    join lateral (select op.card_terminal_id from order_payments op
                   where op.order_id = r.order_id and op.payment_method_id = r.payment_method_id
                     and op.card_terminal_id is not null
                   order by op.id desc limit 1) t on true
   where r.register_session_id = sqlc.arg(session_id)::bigint and r.cash_movement_id is null
   group by t.card_terminal_id
)
select c.card_terminal_id::bigint as terminal_id, c.name::text as name,
       greatest(coalesce(c.total, 0) - coalesce(d.total, 0), 0)::numeric(10,2) as collected
  from cobrado c
  left join devuelto d on d.card_terminal_id = c.card_terminal_id
 order by c.card_terminal_id;

-- name: InsertSessionTerminalCount :exec
insert into session_terminal_counts (session_id, card_terminal_id, terminal_name, expected, declared, created_by)
values ($1, $2, $3, $4, $5, $6);

-- name: SessionTerminalCounts :many
select card_terminal_id, terminal_name, expected, declared, (declared - expected)::numeric(10,2) as difference
  from session_terminal_counts where session_id = $1 order by card_terminal_id;

-- name: PaymentTerminalForRefund :one
-- La terminal del cobro que se devuelve: la pantalla dice en cuál hacer la devolución.
select op.id, op.card_terminal_name, pm.kind
  from order_payments op
  join payment_methods pm on pm.id = op.payment_method_id
 where op.id = $1;
