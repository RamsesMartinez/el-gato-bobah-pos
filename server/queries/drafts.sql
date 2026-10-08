-- La cuenta en captura (spec 030). company_id no se nombra en ninguna consulta: lo pone RLS en el
-- where y el default en el insert, como en el resto del repo.

-- name: InsertDraft :execrows
-- `on conflict (id) do nothing`: un reintento de crear con el mismo id no es un error, es la misma
-- cuenta. Cero filas lo dice. Los únicos parciales de nombre y de pedido NO entran en el `on
-- conflict`: esos choques son de otra cuenta, y tienen que llegar como 23505 para resolverse.
insert into order_drafts (id, order_id, folio_name, folio_scheme, service_type, customer_name,
                          delivery_platform_id, platform_order_ref, delivery_fee, discount_amount,
                          discount_percent, discount_set_by, platform_ref_set_by, opened_by)
values (@id, sqlc.narg('order_id'), sqlc.narg('folio_name'), sqlc.narg('folio_scheme'), @service_type,
        sqlc.narg('customer_name'), sqlc.narg('delivery_platform_id'), sqlc.narg('platform_order_ref'),
        @delivery_fee, sqlc.narg('discount_amount'), sqlc.narg('discount_percent'),
        sqlc.narg('discount_set_by'), sqlc.narg('platform_ref_set_by'), @opened_by)
on conflict (id) do nothing;

-- name: GetDraft :one
select d.*, u.name as opened_by_name
from order_drafts d
join users u on u.id = d.opened_by
where d.id = $1;

-- name: LockDraft :one
-- Toda escritura de renglones toma esto PRIMERO: serializa por cuenta, así la fusión («no había un
-- renglón igual») y la posición no se pisan entre dos tabletas.
select * from order_drafts where id = $1 for update;

-- name: GetLiveDraftOfOrder :one
-- La «Nuevo» viva de un pedido, si tiene. Una sola por el único parcial.
select id from order_drafts where order_id = $1 and status = 'capturando';

-- name: ListDraftLines :many
select * from order_draft_lines where draft_id = $1 order by position;

-- name: InsertDraftLine :exec
-- La posición es la siguiente de la cuenta. Segura porque quien llama ya tiene la cuenta bloqueada.
insert into order_draft_lines (id, draft_id, product_id, qty, modifiers, notes, position)
values (@id, @draft_id, @product_id, @qty, @modifiers, sqlc.narg('notes'),
        (select coalesce(max(position), 0) + 1 from order_draft_lines where draft_id = @draft_id));

-- name: AddToDraftLine :execrows
-- El «+»: suma y avanza la versión. Avanzarla es a propósito: el «−» manda la cantidad ABSOLUTA, y
-- aplicarlo sobre un «+» que la otra tableta acaba de hacer borraría lo que agregó (D-5).
update order_draft_lines set qty = qty + @qty, version = version + 1, updated_at = now()
where id = @id and draft_id = @draft_id;

-- name: CountDraftLines :one
select count(*)::int from order_draft_lines where draft_id = $1;

-- name: ChangeDraftLine :execrows
-- Cambiar con la versión esperada: cero filas = otra tableta lo cambió o lo quitó (D-5).
update order_draft_lines
set qty = @qty, modifiers = @modifiers, notes = sqlc.narg('notes'), version = version + 1, updated_at = now()
where id = @id and draft_id = @draft_id and version = @expected_version;

-- name: DeleteDraftLine :execrows
delete from order_draft_lines where id = @id and draft_id = @draft_id and version = @expected_version;

-- name: GetDraftAdd :one
select draft_id from order_draft_adds where op_id = $1;

-- name: InsertDraftAdd :execrows
insert into order_draft_adds (op_id, draft_id, line_id) values (@op_id, @draft_id, @line_id)
on conflict do nothing;

-- name: TouchDraft :exec
update order_drafts set updated_at = now() where id = $1;

-- name: UpdateDraftHeader :execrows
-- La cabecera completa, con la versión esperada (D-5). Quien llama arma los valores finales: aquí no
-- se decide nada, para que el «quién puso el descuento» y su par no se decidan en dos lugares.
update order_drafts
set service_type = @service_type, customer_name = sqlc.narg('customer_name'),
    delivery_platform_id = sqlc.narg('delivery_platform_id'), platform_order_ref = sqlc.narg('platform_order_ref'),
    delivery_fee = @delivery_fee, discount_amount = sqlc.narg('discount_amount'),
    discount_percent = sqlc.narg('discount_percent'), discount_set_by = sqlc.narg('discount_set_by'),
    platform_ref_set_by = sqlc.narg('platform_ref_set_by'),
    header_version = header_version + 1, updated_at = now()
where id = @id and header_version = @expected_version and status = 'capturando';

-- name: ListLiveDraftNames :many
-- Los nombres que ya se le dijeron a un cliente y siguen vivos. Los lee quien reparte un nombre
-- DENTRO de su transacción, para que la ventana con una cuenta que nace a la vez sea mínima.
select folio_name::text from order_drafts where status = 'capturando' and folio_name is not null;

-- name: ListDraftsToSweep :many
-- Las cuentas vivas con lo que el barrido necesita para decidir: cuándo se tocaron (contra el reloj
-- de la BASE, el mismo que escribió `updated_at`) y si su pedido ya se canceló o reembolsó.
select d.id, d.updated_at, now()::timestamptz as db_now,
       coalesce(o.status in ('cancelada', 'reembolsada'), false)::boolean as order_voided
from order_drafts d
left join orders o on o.id = d.order_id
where d.status = 'capturando';

-- name: DiscardDraft :one
-- `status = 'capturando'` en el propio update: un barrido que corre mientras otra tableta manda la
-- cuenta a cocina espera el candado y, al verla enviada, no la toca. Con `seen` además exige que
-- nadie la haya tocado desde que el barrido la vio (las 12 horas se cuentan desde el último cambio).
update order_drafts
set status = 'descartada', discarded_at = now(), discarded_by = sqlc.narg('discarded_by'),
    discard_reason = @reason, updated_at = now()
where id = @id and status = 'capturando'
  and (sqlc.narg('seen')::timestamptz is null or updated_at = sqlc.narg('seen')::timestamptz)
returning folio_name, folio_scheme, created_at;
