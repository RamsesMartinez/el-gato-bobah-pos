-- Avisos de inventario (spec 084): la campana, lo leído por persona, la configuración de alertas y la
-- evaluación. RLS pone la empresa.

-- name: ListNotifications :many
-- Abiertas (o cerradas) de la empresa, con lo leído por la persona. Los «No lo encuentro» solo para
-- quien configura el almacén. `banner`: el artículo pide franja, la persona no la ha visto y no fue
-- quien lo reportó.
select n.id, n.kind, n.status, n.item_type, n.ingredient_id, n.product_id, n.item_name, n.unit_code, n.query_text,
       n.on_hand_seen, n.min_seen, n.needs_count, n.opened_at, n.resolved_at, n.resolved_reason, n.opened_by,
       coalesce(ob.name, '')::text as opened_by_name, coalesce(rb.name, '')::text as resolved_by_name, b.name::text as branch_name,
       (r.read_at is not null)::bool as is_read,
       (n.status = 'open' and n.kind = 'stock_out' and coalesce(i.low_stock_banner, p.low_stock_banner, false)
         and r.banner_seen_at is null and n.opened_by is distinct from sqlc.arg(user_id)::bigint)::bool as banner,
       (select count(*) from stock_out_reports s
         where s.notification_id = n.id and s.kind in ('report', 'confirmation') and s.undone_at is null)::int as confirmations
  from notifications n
  join branches b on b.id = n.branch_id
  left join users ob on ob.id = n.opened_by
  left join users rb on rb.id = n.resolved_by
  left join ingredients i on i.id = n.ingredient_id
  left join products p on p.id = n.product_id
  left join notification_reads r on r.notification_id = n.id and r.user_id = sqlc.arg(user_id)
 where (n.status = 'open') = sqlc.arg(open)::bool
   and (sqlc.arg(include_not_found)::bool or n.kind <> 'not_found')
 order by coalesce(n.resolved_at, n.opened_at) desc
 limit sqlc.arg(lim);

-- name: UnreadNotifications :one
select count(*)::int
  from notifications n
  left join notification_reads r on r.notification_id = n.id and r.user_id = sqlc.arg(user_id)
 where n.status = 'open' and r.read_at is null
   and (sqlc.arg(include_not_found)::bool or n.kind <> 'not_found');

-- name: MarkNotificationsRead :exec
-- Solo ids de la empresa (el select va bajo RLS): uno ajeno no truena, se ignora.
insert into notification_reads (notification_id, user_id, read_at)
select n.id, sqlc.arg(user_id), now() from notifications n where n.id = any(sqlc.arg(ids)::bigint[])
on conflict (company_id, notification_id, user_id) do update set read_at = coalesce(notification_reads.read_at, excluded.read_at);

-- name: MarkBannerSeen :exec
insert into notification_reads (notification_id, user_id, banner_seen_at)
select n.id, sqlc.arg(user_id), now() from notifications n where n.id = sqlc.arg(id)
on conflict (company_id, notification_id, user_id) do update
  set banner_seen_at = coalesce(notification_reads.banner_seen_at, excluded.banner_seen_at);

-- name: GetNotificationForUpdate :one
select id, branch_id, kind, status, item_type, ingredient_id, product_id, query_text, opened_by
  from notifications where id = sqlc.arg(id) for update;

-- name: ResolveNotification :execrows
update notifications
   set status = sqlc.arg(status), resolved_at = now(), resolved_by = sqlc.narg(resolved_by),
       resolved_reason = sqlc.arg(resolved_reason)
 where id = sqlc.arg(id) and status = 'open';

-- name: DismissAsUndone :exec
-- Deshacer el último «se acabó» descarta su aviso. El ajuste de vuelta pudo haberlo cerrado ya en esta
-- misma transacción (el trigger, como «volvió a haber»): resolved_at = now() es esta transacción.
update notifications
   set status = 'dismissed', resolved_at = now(), resolved_by = sqlc.arg(resolved_by), resolved_reason = 'undone'
 where id = sqlc.arg(id) and (status = 'open' or resolved_at = now());

-- name: ResolveOpenOfArticle :execrows
-- Cierra lo abierto de un artículo en todas las sucursales (archivar, unir, cambiar de unidad,
-- contar). Con kind vacío cierra todos los tipos.
update notifications
   set status = 'resolved', resolved_at = now(), resolved_by = sqlc.narg(resolved_by), resolved_reason = sqlc.arg(resolved_reason)
 where status = 'open' and kind <> 'not_found'
   and item_key = coalesce(sqlc.narg(ingredient_id)::bigint, -sqlc.narg(product_id)::bigint, 0)
   and (sqlc.arg(kind)::text = '' or kind = sqlc.arg(kind)::text)
   and (sqlc.narg(branch_id)::bigint is null or branch_id = sqlc.narg(branch_id)::bigint);

-- name: EvaluateLowStock :exec
-- La misma función que corre el trigger, para lo que cambia sin movimiento (configuración, conteo
-- que confirma la cifra, archivar, unir, cambiar de unidad).
select evaluate_low_stock(nullif(current_setting('app.company_id', true), '')::bigint, sqlc.arg(branch_id)::bigint,
                          sqlc.arg(item_type)::stock_item_type, sqlc.narg(ingredient_id)::bigint, sqlc.narg(product_id)::bigint);

-- name: LowStockSnapshot :one
-- Lo que la vista previa de un cambio de alertas necesita para calcular el efecto con
-- domain.LowStockEffect (la misma tabla de verdad que evaluate_low_stock): la existencia en la
-- sucursal, el mínimo de la sucursal si tiene, y lo abierto.
select coalesce((select s.on_hand from stock_levels s
                  where s.branch_id = sqlc.arg(branch_id)
                    and (s.ingredient_id = sqlc.narg(ingredient_id) or s.product_id = sqlc.narg(product_id))), 0)::numeric as on_hand,
       m.min_stock as branch_min,
       coalesce((select array_agg(n.kind order by n.kind) from notifications n
                  where n.branch_id = sqlc.arg(branch_id) and n.status = 'open' and n.kind <> 'not_found'
                    and n.item_key = coalesce(sqlc.narg(ingredient_id)::bigint, -sqlc.narg(product_id)::bigint, 0)), '{}')::text[] as open_kinds,
       exists (select 1 from notifications n
                where n.branch_id = sqlc.arg(branch_id) and n.status = 'open' and n.kind = 'stock_out' and n.opened_by is not null
                  and n.item_key = coalesce(sqlc.narg(ingredient_id)::bigint, -sqlc.narg(product_id)::bigint, 0))::bool as manual_out
  -- Left join y no subconsulta: así sqlc lo tipa como nulable (una sucursal sin mínimo propio).
  from (select 1) one
  left join branch_stock_minimums m on m.branch_id = sqlc.arg(branch_id)
   and (m.ingredient_id = sqlc.narg(ingredient_id) or m.product_id = sqlc.narg(product_id));

-- name: CompanyBranches :many
select id from branches where is_active order by id;

-- name: NotifyNotificationsChanged :exec
select pg_notify('notifications', nullif(current_setting('app.company_id', true), '')::text);

-- name: CountedArticlesOf :many
-- Lo que un conteo incluyó (insumos y productos), contara diferencia o no.
select 'ingrediente'::stock_item_type as item_type, ingredient_id as id from stock_count_lines where count_id = sqlc.arg(cid)::bigint
union all
select 'producto'::stock_item_type, product_id from stock_count_product_lines where count_id = sqlc.arg(cid)::bigint;

-- name: MergeLowStockInto :exec
-- Al unir insumos, la casilla y los eventos del destino se fusionan con OR: si alguien pidió aviso de
-- cualquiera de los que se unen, lo sigue teniendo.
update ingredients t
   set low_stock_enabled = t.low_stock_enabled or s.enabled,
       low_stock_on_min = t.low_stock_on_min or s.on_min,
       low_stock_on_out = t.low_stock_on_out or s.on_out,
       low_stock_banner = t.low_stock_banner or s.banner
  from (select bool_or(low_stock_enabled) as enabled, bool_or(low_stock_enabled and low_stock_on_min) as on_min,
               bool_or(low_stock_enabled and low_stock_on_out) as on_out, bool_or(low_stock_banner) as banner
          from ingredients where id = any(sqlc.arg(sources)::bigint[])) s
 where t.id = sqlc.arg(target);

-- name: DeleteBranchMinimumsOf :exec
delete from branch_stock_minimums where ingredient_id = any(sqlc.arg(ids)::bigint[]);

-- name: ReplacementOf :one
-- El artículo nuevo de un reemplazo por cambio de unidad (0 si se convirtió el mismo).
select coalesce((select r.new_ingredient_id from ingredient_replacements r
                  where r.old_ingredient_id = sqlc.arg(old_id) and r.kind <> 'merge'
                  order by r.id desc limit 1), 0)::bigint;

-- name: MoveLowStockOnUnitChange :exec
-- Cambio de unidad (spec 084): el mínimo de cada sucursal se convierte con el mismo factor que el del
-- artículo, y con reemplazo pasa al nuevo junto con su casilla y sus eventos.
update branch_stock_minimums
   set ingredient_id = sqlc.arg(new_id), min_stock = round(min_stock * sqlc.arg(factor)::numeric, 4), updated_at = now()
 where ingredient_id = sqlc.arg(old_id);

-- name: CopyLowStockSettings :exec
update ingredients n
   set low_stock_enabled = o.low_stock_enabled, low_stock_on_min = o.low_stock_on_min,
       low_stock_on_out = o.low_stock_on_out, low_stock_banner = o.low_stock_banner,
       coverage_days = o.coverage_days, low_stock_asked_at = o.low_stock_asked_at
  from ingredients o
 where o.id = sqlc.arg(old_id) and n.id = sqlc.arg(new_id);

-- name: LowStockConfigOf :one
-- La configuración de alertas de un artículo, como la lee la ficha. Para un producto, solo si tiene
-- existencia propia: el «Se compra hecho» guarda la suya en su insumo.
select a.name, a.unit_code, a.is_active, a.low_stock_enabled, a.low_stock_on_min, a.low_stock_on_out, a.low_stock_banner,
       a.min_stock, a.coverage_days, a.low_stock_asked_at, a.updated_at
  from (
    select i.name, u.code::text as unit_code, i.is_active, i.low_stock_enabled, i.low_stock_on_min, i.low_stock_on_out,
           i.low_stock_banner, i.min_stock, i.coverage_days, i.low_stock_asked_at, i.updated_at
      from ingredients i join units u on u.id = i.base_unit_id
     where sqlc.arg(item_type)::stock_item_type = 'ingrediente' and i.id = sqlc.arg(id)
    union all
    select p.name, 'pieza'::text, p.is_active, p.low_stock_enabled, p.low_stock_on_min, p.low_stock_on_out,
           p.low_stock_banner, p.min_stock, p.coverage_days, p.low_stock_asked_at, p.updated_at
      from products p
     where sqlc.arg(item_type)::stock_item_type = 'producto' and p.id = sqlc.arg(id) and p.track_stock
  ) a;

-- name: SetIngredientLowStock :one
update ingredients
   set low_stock_enabled = sqlc.arg(enabled), low_stock_on_min = sqlc.arg(on_min), low_stock_on_out = sqlc.arg(on_out),
       low_stock_banner = sqlc.arg(banner), min_stock = sqlc.narg(min_stock), coverage_days = sqlc.narg(coverage_days),
       low_stock_asked_at = coalesce(low_stock_asked_at, now())
 where id = sqlc.arg(id)
returning updated_at;

-- name: SetProductLowStock :one
update products
   set low_stock_enabled = sqlc.arg(enabled), low_stock_on_min = sqlc.arg(on_min), low_stock_on_out = sqlc.arg(on_out),
       low_stock_banner = sqlc.arg(banner), min_stock = sqlc.narg(min_stock), coverage_days = sqlc.narg(coverage_days),
       low_stock_asked_at = coalesce(low_stock_asked_at, now())
 where id = sqlc.arg(id) and track_stock
returning updated_at;

-- name: MarkLowStockAsked :execrows
-- Ya se hizo la pregunta de alertas (la respuesta fue «No, por ahora»): no vuelve a salir.
update ingredients set low_stock_asked_at = coalesce(low_stock_asked_at, now()) where id = sqlc.arg(id);

-- name: UpsertBranchMinimum :exec
insert into branch_stock_minimums (branch_id, item_type, ingredient_id, product_id, min_stock, updated_by)
values (sqlc.arg(branch_id), sqlc.arg(item_type), sqlc.narg(ingredient_id), sqlc.narg(product_id), sqlc.arg(min_stock), sqlc.arg(updated_by))
on conflict (branch_id, ingredient_id) where ingredient_id is not null
do update set min_stock = excluded.min_stock, updated_by = excluded.updated_by, updated_at = now();

-- name: UpsertBranchProductMinimum :exec
insert into branch_stock_minimums (branch_id, item_type, product_id, min_stock, updated_by)
values (sqlc.arg(branch_id), 'producto', sqlc.arg(product_id), sqlc.arg(min_stock), sqlc.arg(updated_by))
on conflict (branch_id, product_id) where product_id is not null
do update set min_stock = excluded.min_stock, updated_by = excluded.updated_by, updated_at = now();

-- name: DeleteBranchMinimum :exec
delete from branch_stock_minimums
 where branch_id = sqlc.arg(branch_id)
   and (ingredient_id = sqlc.narg(ingredient_id) or product_id = sqlc.narg(product_id));

-- name: ListLowStockConfigs :many
-- La tabla de «Alertas por lote»: insumos activos y productos con existencia propia, con lo que hay
-- en la sucursal.
select a.item_type, a.id, a.name, a.unit_code, a.low_stock_enabled, a.min_stock,
       coalesce((select s.on_hand from stock_levels s
                  where s.branch_id = current_branch_id()
                    and ((a.item_type = 'ingrediente' and s.ingredient_id = a.id) or (a.item_type = 'producto' and s.product_id = a.id))), 0)::numeric as on_hand
  from (
    select 'ingrediente'::stock_item_type as item_type, i.id, i.name, u.code::text as unit_code, i.low_stock_enabled, i.min_stock
      from ingredients i join units u on u.id = i.base_unit_id where i.is_active
    union all
    select 'producto'::stock_item_type, p.id, p.name, 'pieza'::text, p.low_stock_enabled, p.min_stock
      from products p where p.is_active and p.track_stock
  ) a
 order by a.name;

-- name: CompanyBranchCount :one
select count(*)::int from branches where is_active;

-- name: PendingEmailNotifications :many
-- Lo que todavía no sale por correo y se abrió antes de `before` (el inicio del bloque en curso).
select n.id, n.kind, n.status, n.item_key, n.branch_id, coalesce(n.item_name, '')::text as item_name,
       coalesce(n.unit_code, '')::text as unit_code, coalesce(n.query_text, '')::text as query_text, n.opened_at,
       coalesce(u.name, '')::text as opened_by_name, coalesce(n.on_hand_seen, 0)::numeric as on_hand_seen,
       coalesce(n.min_seen, 0)::numeric as min_seen, n.needs_count
  from notifications n
  left join users u on u.id = n.opened_by
 where n.email_state = 'pending' and n.opened_at < sqlc.arg(before)
 order by n.opened_at, n.id;

-- name: LastEmailedAt :one
-- La última vez que un artículo salió en un correo (para el enfriamiento de 12 h). Época si nunca.
select coalesce(max(s.sent_at), 'epoch'::timestamptz)::timestamptz
  from notifications n join notification_email_sends s on s.id = n.email_send_id
 where n.item_key = sqlc.arg(item_key) and n.kind <> 'not_found' and n.email_state = 'sent';

-- name: ClaimNotificationSend :many
-- La fila se toma ANTES de mandar: dos APIs vivas o un reinicio no mandan dos veces. Solo se vuelve a
-- tomar una fallida o una pendiente colgada más de 10 minutos, hasta 5 intentos.
insert into notification_email_sends (kind, period_key, status, attempts) values (sqlc.arg(kind), sqlc.arg(period_key), 'pending', 1)
on conflict (company_id, kind, period_key) do update
   set status = 'pending', attempts = notification_email_sends.attempts + 1, last_error = null
 where notification_email_sends.attempts < 5
   and (notification_email_sends.status = 'failed'
        or (notification_email_sends.status = 'pending' and notification_email_sends.created_at < now() - interval '10 minutes'))
returning id;

-- name: FinishNotificationSend :exec
update notification_email_sends
   set status = sqlc.arg(status), recipients = sqlc.arg(recipients), last_error = sqlc.narg(last_error),
       sent_at = case when sqlc.arg(status)::text = 'sent' then now() end
 where id = sqlc.arg(id);

-- name: MarkNotificationsEmailed :exec
update notifications set email_state = sqlc.arg(state), email_send_id = sqlc.narg(send_id)
 where id = any(sqlc.arg(ids)::bigint[]) and email_state = 'pending';

-- name: MorningSendExists :one
select exists (select 1 from notification_email_sends where kind = 'morning' and period_key = sqlc.arg(period_key) and status in ('sent', 'empty'))::bool;

-- name: OpenNotificationsForMorning :many
select n.id, n.kind, coalesce(n.item_name, '')::text as item_name, coalesce(n.unit_code, '')::text as unit_code,
       coalesce(n.query_text, '')::text as query_text, n.opened_at, coalesce(u.name, '')::text as opened_by_name,
       coalesce(n.on_hand_seen, 0)::numeric as on_hand_seen, coalesce(n.min_seen, 0)::numeric as min_seen, n.needs_count
  from notifications n left join users u on u.id = n.opened_by
 where n.status = 'open'
 order by n.kind, n.opened_at;

-- name: EnabledLowStockItems :many
-- Para el barrido de la mañana: si un aviso se perdió (una falla del trigger), se vuelve a evaluar.
select 'ingrediente'::stock_item_type as item_type, id from ingredients where is_active and low_stock_enabled
union all
select 'producto'::stock_item_type, id from products where is_active and track_stock and low_stock_enabled;

-- name: BusinessNameForEmail :one
select coalesce(nullif(business_name, ''), 'Tu negocio')::text from business_settings limit 1;
