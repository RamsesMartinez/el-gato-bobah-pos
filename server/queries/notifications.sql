-- Avisos de inventario (spec 084): la campana, lo leído por persona, la configuración de alertas y la
-- evaluación. RLS pone la empresa.

-- name: ListNotifications :many
-- Abiertas (o cerradas) de la empresa, con lo leído por la persona. Los «No lo encuentro» solo para
-- quien configura el almacén. `banner`: el artículo pide franja, la persona no la ha visto y no fue
-- quien lo reportó.
select n.id, n.kind, n.status, n.item_type, n.ingredient_id, n.product_id, n.item_name, n.unit_code, n.query_text,
       n.on_hand_seen, n.min_seen, n.needs_count, n.opened_at, n.resolved_at, n.resolved_reason, n.opened_by,
       ob.name::text as opened_by_name, rb.name::text as resolved_by_name, b.name::text as branch_name,
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
       (select m.min_stock from branch_stock_minimums m
         where m.branch_id = sqlc.arg(branch_id)
           and (m.ingredient_id = sqlc.narg(ingredient_id) or m.product_id = sqlc.narg(product_id))) as branch_min,
       coalesce((select array_agg(n.kind order by n.kind) from notifications n
                  where n.branch_id = sqlc.arg(branch_id) and n.status = 'open' and n.kind <> 'not_found'
                    and n.item_key = coalesce(sqlc.narg(ingredient_id)::bigint, -sqlc.narg(product_id)::bigint, 0)), '{}')::text[] as open_kinds,
       exists (select 1 from notifications n
                where n.branch_id = sqlc.arg(branch_id) and n.status = 'open' and n.kind = 'stock_out' and n.opened_by is not null
                  and n.item_key = coalesce(sqlc.narg(ingredient_id)::bigint, -sqlc.narg(product_id)::bigint, 0))::bool as manual_out;

-- name: CompanyBranches :many
select id from branches where is_active order by id;

-- name: NotifyNotificationsChanged :exec
select pg_notify('notifications', nullif(current_setting('app.company_id', true), '')::text);
