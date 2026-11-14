-- «Se acabó» y «No lo encuentro» (spec 084). RLS pone la empresa; la sucursal la resuelve
-- current_branch_id(), que con dos y sin selector truena en vez de adivinar.

-- name: SearchOutArticles :many
-- Artículos activos cuyo nombre u otro nombre tiene una palabra que empieza con CADA llave (ya sin
-- acentos ni plural: domain.SearchKeys). Insumos y productos con existencia propia.
with keys as (select unnest(sqlc.arg(keys)::text[]) as k)
select a.item_type, a.id, a.name, a.unit_code, a.open_out
  from (
    select 'ingrediente'::stock_item_type as item_type, i.id, i.name, u.code::text as unit_code,
           exists (select 1 from notifications n
                    where n.status = 'open' and n.kind = 'stock_out' and n.ingredient_id = i.id
                      and n.branch_id = current_branch_id()) as open_out,
           string_to_array(catalog_name_key(i.name || ' ' || array_to_string(i.alt_names, ' ')), ' ') as words
      from ingredients i join units u on u.id = i.base_unit_id
     where i.is_active
    union all
    select 'producto'::stock_item_type, p.id, p.name, 'pieza'::text,
           exists (select 1 from notifications n
                    where n.status = 'open' and n.kind = 'stock_out' and n.product_id = p.id
                      and n.branch_id = current_branch_id()),
           string_to_array(catalog_name_key(p.name || ' ' || array_to_string(p.alt_names, ' ')), ' ')
      from products p
     where p.is_active and p.track_stock
  ) a
 where not exists (select 1 from keys where not exists (select 1 from unnest(a.words) w where w like keys.k || '%'))
 order by a.name
 limit 20;

-- name: FrequentOutArticles :many
-- Lo que más se reportó desde `since` (activos), para los mosaicos.
select r.item_type, coalesce(r.ingredient_id, r.product_id)::bigint as id,
       max(coalesce(i.name, p.name))::text as name, count(*)::int as times
  from stock_out_reports r
  left join ingredients i on i.id = r.ingredient_id
  left join products p on p.id = r.product_id
 where r.kind in ('report', 'confirmation') and r.reported_at >= sqlc.arg(since)
   and coalesce(i.is_active, p.is_active)
 group by r.item_type, coalesce(r.ingredient_id, r.product_id)
 order by count(*) desc, 3
 limit 8;

-- name: OutReportsSince :many
-- Lo reportado en la sucursal desde `since`: la columna «Reportado hoy».
select r.id, r.kind, r.item_type, r.ingredient_id, r.product_id,
       coalesce(i.name, p.name, n.query_text)::text as name, r.reported_at, r.undone_at, r.reported_by,
       u.name::text as reported_by_name, (r.delta is not null)::bool as moved, n.status::text as notification_status
  from stock_out_reports r
  join notifications n on n.id = r.notification_id
  join users u on u.id = r.reported_by
  left join ingredients i on i.id = r.ingredient_id
  left join products p on p.id = r.product_id
 where r.branch_id = current_branch_id() and r.reported_at >= sqlc.arg(since)
 order by r.reported_at desc
 limit 50;

-- name: CountUserReportsSince :one
select count(*)::int from stock_out_reports where reported_by = sqlc.arg(user_id) and reported_at >= sqlc.arg(since);

-- name: GetOutReportByClientKey :one
-- Con lo que se reportó: una llave repetida solo devuelve el reporte si es del mismo artículo o texto
-- y de la misma persona.
select r.id, r.kind, r.notification_id, r.delta, r.reported_at, r.reported_by,
       coalesce(r.ingredient_id, 0)::bigint as ingredient_key, coalesce(r.product_id, 0)::bigint as product_key,
       coalesce(n.query_text, '')::text as query_text
  from stock_out_reports r join notifications n on n.id = r.notification_id
 where r.client_key = sqlc.arg(client_key);

-- name: LockReporter :exec
-- Un reporte a la vez por persona: así el tope por hora cuenta lo que de verdad pasó y la misma llave
-- mandada dos veces a la vez encuentra el primer reporte en vez de chocar con el único.
select pg_advisory_xact_lock(84291, sqlc.arg(user_id)::int);

-- name: PendingOutMovementOf :one
-- El ajuste a 0 de un aviso que nadie ha repuesto: el reporte que lo hizo se deshizo mientras otro
-- sostenía el aviso. Al deshacer el último, se repone éste.
select id, movement_id::bigint as movement_id, delta::numeric as delta, reported_at
  from stock_out_reports
 where notification_id = sqlc.arg(notification_id) and movement_id is not null and undo_movement_id is null
   and undone_at is not null
 order by id limit 1;

-- name: SetOutReportUndoMovement :exec
update stock_out_reports set undo_movement_id = sqlc.arg(undo_movement_id) where id = sqlc.arg(id);

-- name: OutArticle :one
-- Lo que «Se acabó» necesita del artículo: nombre, unidad y si sigue activo.
select a.name, a.unit_code, a.is_active
  from (
    select i.name, u.code::text as unit_code, i.is_active from ingredients i join units u on u.id = i.base_unit_id
     where sqlc.arg(item_type)::stock_item_type = 'ingrediente' and i.id = sqlc.arg(id)
    union all
    select p.name, 'pieza'::text, p.is_active and p.track_stock from products p
     where sqlc.arg(item_type)::stock_item_type = 'producto' and p.id = sqlc.arg(id)
  ) a;

-- name: StockOnHandForUpdate :many
-- La existencia del artículo en la sucursal, tomada: el ajuste a 0 resta exactamente lo que hay.
select on_hand from stock_levels
 where branch_id = sqlc.arg(branch_id)
   and (ingredient_id = sqlc.narg(ingredient_id) or product_id = sqlc.narg(product_id))
 for update;

-- name: InsertOutAdjustment :one
-- El ajuste de «se acabó» (o su deshacer). Es un ajuste, no una venta: no cuenta como consumo.
insert into stock_movements (item_type, ingredient_id, product_id, movement_type, quantity, user_id, reason, branch_id)
values (sqlc.arg(item_type), sqlc.narg(ingredient_id), sqlc.narg(product_id), 'ajuste', sqlc.arg(quantity),
        sqlc.arg(user_id), sqlc.arg(reason), sqlc.arg(branch_id))
returning id;

-- name: OpenManualOut :many
-- Abre el «se acabó» reportado a mano. Si ya hay uno abierto (de alguien más o del sistema), no abre
-- otro: el índice único lo impide también con dos tabletas a la vez.
insert into notifications (branch_id, kind, item_type, ingredient_id, product_id, item_name, unit_code, on_hand_seen, opened_by)
values (sqlc.arg(branch_id), 'stock_out', sqlc.arg(item_type), sqlc.narg(ingredient_id), sqlc.narg(product_id),
        sqlc.arg(item_name), sqlc.arg(unit_code), sqlc.arg(on_hand_seen), sqlc.arg(opened_by))
on conflict (company_id, branch_id, item_key, kind) where status = 'open' and kind <> 'not_found' do nothing
returning id;

-- name: OpenOutNotification :one
select id from notifications
 where branch_id = sqlc.arg(branch_id) and status = 'open' and kind = 'stock_out'
   and item_key = coalesce(sqlc.narg(ingredient_id)::bigint, -sqlc.narg(product_id)::bigint, 0);

-- name: ClaimTriggerOut :one
-- El aviso que abrió el trigger al dejar la existencia en 0 no sabe quién fue: el reporte que se
-- cuelga de él lo deja a nombre de quien reportó (lo vuelve manual: espera a que haya existencia).
update notifications set opened_by = sqlc.arg(opened_by)
 where branch_id = sqlc.arg(branch_id) and status = 'open' and kind = 'stock_out'
   and item_key = coalesce(sqlc.narg(ingredient_id)::bigint, -sqlc.narg(product_id)::bigint, 0)
   and opened_by is null
returning id;

-- name: OpenNotFound :one
insert into notifications (branch_id, kind, query_text, opened_by)
values (sqlc.arg(branch_id), 'not_found', sqlc.arg(query_text), sqlc.arg(opened_by))
returning id;

-- name: InsertOutReport :one
insert into stock_out_reports (branch_id, kind, item_type, ingredient_id, product_id, notification_id, movement_id, delta,
                               client_key, reported_by)
values (sqlc.arg(branch_id), sqlc.arg(kind), sqlc.narg(item_type), sqlc.narg(ingredient_id), sqlc.narg(product_id),
        sqlc.arg(notification_id), sqlc.narg(movement_id), sqlc.narg(delta), sqlc.arg(client_key), sqlc.arg(reported_by))
returning id, reported_at;

-- name: GetOutReportForUndo :one
select id, branch_id, kind, item_type, ingredient_id, product_id, notification_id, movement_id, delta, reported_by,
       reported_at, undone_at
  from stock_out_reports where id = sqlc.arg(id) for update;

-- name: ArticleMovedSince :one
-- ¿Algo movió el artículo después del reporte? Un movimiento posterior o un renglón de conteo
-- capturado después: reponer el delta original lo pisaría.
select (exists (select 1 from stock_movements m
                 where m.branch_id = sqlc.arg(branch_id) and m.id > sqlc.arg(movement_id)::bigint
                   and (m.ingredient_id = sqlc.narg(ingredient_id) or m.product_id = sqlc.narg(product_id)))
     or exists (select 1 from stock_count_lines l join stock_counts c on c.id = l.count_id
                 where c.branch_id = sqlc.arg(branch_id) and l.ingredient_id = sqlc.narg(ingredient_id)
                   and l.captured_at > sqlc.arg(reported_at))
     or exists (select 1 from stock_count_product_lines l join stock_counts c on c.id = l.count_id
                 where c.branch_id = sqlc.arg(branch_id) and l.product_id = sqlc.narg(product_id)
                   and l.captured_at > sqlc.arg(reported_at)))::bool;

-- name: MarkOutReportUndone :exec
update stock_out_reports set undone_at = now(), undo_movement_id = sqlc.arg(undo_movement_id) where id = sqlc.arg(id);

-- name: LiveReportsOnNotification :one
-- Cuántos reportes vivos (sin deshacer) sostienen un aviso: deshacer el último lo descarta.
select count(*)::int from stock_out_reports
 where notification_id = sqlc.arg(notification_id) and kind in ('report', 'confirmation') and undone_at is null;
