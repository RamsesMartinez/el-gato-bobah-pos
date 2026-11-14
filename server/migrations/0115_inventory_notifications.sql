-- +goose Up
-- Notificaciones de inventario (spec 084, issue #291): alertas de stock bajo opt-in por artículo,
-- «Se acabó» y «No lo encuentro», la campana, el correo y la lista de compra.
--
-- Aditiva: solo agrega columnas con default, tablas, funciones, un trigger y un permiso. La API
-- anterior sigue sirviendo durante la subida; el trigger aplica también a sus movimientos.
-- En el código todo se llama «notification»: «alert» es de los avisos de error del servidor (070).

-- El trigger nuevo toma ShareRowExclusive sobre stock_movements un instante, con el negocio vendiendo:
-- si una venta larga lo tiene tomado, la migración falla en vez de formar una fila detrás de ella.
set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 1. La configuración de alertas vive en el artículo. Defaults constantes: sin reescribir la tabla.
-- ---------------------------------------------------------------------------------------------
alter table ingredients
  add column low_stock_enabled  boolean not null default false,
  add column low_stock_on_min   boolean not null default true,
  add column low_stock_on_out   boolean not null default true,
  add column low_stock_banner   boolean not null default false,
  add column coverage_days      smallint constraint ingredients_coverage_days_range check (coverage_days between 1 and 90),
  add column low_stock_asked_at timestamptz;
alter table products
  add column low_stock_enabled  boolean not null default false,
  add column low_stock_on_min   boolean not null default true,
  add column low_stock_on_out   boolean not null default true,
  add column low_stock_banner   boolean not null default false,
  add column coverage_days      smallint constraint products_coverage_days_range check (coverage_days between 1 and 90),
  add column low_stock_asked_at timestamptz;

alter table business_settings add column notifications_morning_at time not null default '08:00';

-- ---------------------------------------------------------------------------------------------
-- 2. El mínimo de una sucursal (decisión del dueño: cada sucursal puede definir el suyo). Gana sobre
--    el del artículo. Los únicos no llevan company_id a propósito, como stock_levels: sucursal e
--    insumo ya son identidades de una sola empresa. Nombres explícitos: la futura columna de almacén
--    (#65) los reconstruye por nombre.
-- ---------------------------------------------------------------------------------------------
create table branch_stock_minimums (
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                references companies(id) on delete cascade,
  branch_id     bigint not null,
  item_type     stock_item_type not null,
  ingredient_id bigint,
  product_id    bigint,
  min_stock     numeric(14,4) not null constraint branch_stock_minimums_min_nonneg check (min_stock >= 0),
  updated_by    bigint not null,
  updated_at    timestamptz not null default now(),
  constraint branch_stock_minimums_one_item check (
    (item_type = 'ingrediente' and ingredient_id is not null and product_id is null) or
    (item_type = 'producto' and product_id is not null and ingredient_id is null)),
  constraint branch_stock_minimums_branch_fkey foreign key (company_id, branch_id) references branches (company_id, id),
  constraint branch_stock_minimums_ingredient_fkey foreign key (ingredient_id, company_id) references ingredients (id, company_id),
  constraint branch_stock_minimums_product_fkey foreign key (company_id, product_id) references products (company_id, id),
  constraint branch_stock_minimums_user_fkey foreign key (company_id, updated_by) references users (company_id, id)
);
create unique index branch_stock_minimums_ingredient_key on branch_stock_minimums (branch_id, ingredient_id) where ingredient_id is not null;
create unique index branch_stock_minimums_product_key on branch_stock_minimums (branch_id, product_id) where product_id is not null;

-- ---------------------------------------------------------------------------------------------
-- 3. Cada correo enviado (bloque de 5 min o resumen de la mañana): la fila es el candado que impide
--    mandarlo dos veces, también con dos APIs vivas o tras un reinicio.
-- ---------------------------------------------------------------------------------------------
create table notification_email_sends (
  id         bigint generated always as identity primary key,
  company_id bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
             references companies(id) on delete cascade,
  kind       text not null constraint notification_email_sends_kind check (kind in ('window', 'morning')),
  period_key text not null constraint notification_email_sends_period_len check (char_length(period_key) between 1 and 40),
  status     text not null default 'pending'
             constraint notification_email_sends_status check (status in ('pending', 'sent', 'failed', 'empty')),
  attempts   smallint not null default 0,
  last_error text constraint notification_email_sends_error_len check (char_length(last_error) <= 200),
  recipients smallint not null default 0,
  created_at timestamptz not null default now(),
  sent_at    timestamptz,
  constraint notification_email_sends_tenant_key unique (company_id, id),
  constraint notification_email_sends_period_key unique (company_id, kind, period_key)
);

-- ---------------------------------------------------------------------------------------------
-- 4. Los avisos. Una tabla genérica: kind crece con otros tipos (por eso text + check y no enum).
-- ---------------------------------------------------------------------------------------------
create table notifications (
  id              bigint generated always as identity primary key,
  company_id      bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                  references companies(id) on delete cascade,
  branch_id       bigint not null,
  kind            text not null constraint notifications_kind check (kind in ('stock_low', 'stock_out', 'not_found')),
  item_type       stock_item_type,
  ingredient_id   bigint,
  product_id      bigint,
  -- La llave del artículo para el índice único: ids de identidad, positivos, y el producto en negativo.
  item_key        bigint generated always as (coalesce(ingredient_id, -product_id, 0)) stored,
  -- Copias al abrir: un aviso viejo dice lo que decía aunque el artículo se renombre o cambie de unidad.
  item_name       text constraint notifications_item_name_len check (char_length(item_name) <= 200),
  unit_code       text constraint notifications_unit_code_len check (char_length(unit_code) <= 20),
  query_text      text constraint notifications_query_text_len check (char_length(query_text) between 1 and 80),
  status          text not null default 'open' constraint notifications_status check (status in ('open', 'resolved', 'dismissed')),
  on_hand_seen    numeric(14,4),
  min_seen        numeric(14,4),
  needs_count     boolean not null default false,
  -- Nulo = lo abrió el sistema (un movimiento). Nunca «se borró la persona»: la llave no lo permite.
  opened_by       bigint,
  opened_at       timestamptz not null default now(),
  resolved_at     timestamptz,
  resolved_by     bigint,
  resolved_reason text constraint notifications_resolved_reason check (resolved_reason in (
                    'above_min', 'restocked', 'counted', 'arrived', 'undone', 'alert_off', 'archived',
                    'merged', 'unit_changed', 'dismissed', 'created', 'alias')),
  email_state     text not null default 'pending' constraint notifications_email_state check (email_state in ('pending', 'sent', 'skipped')),
  email_send_id   bigint,
  constraint notifications_tenant_key unique (company_id, id),
  constraint notifications_item_shape check (
    case kind
      when 'not_found' then query_text is not null and item_type is null and ingredient_id is null and product_id is null
      else query_text is null and (
        (item_type = 'ingrediente' and ingredient_id is not null and product_id is null) or
        (item_type = 'producto' and product_id is not null and ingredient_id is null))
    end),
  constraint notifications_closed_shape check ((status = 'open') = (resolved_at is null)),
  constraint notifications_branch_fkey foreign key (company_id, branch_id) references branches (company_id, id),
  constraint notifications_ingredient_fkey foreign key (ingredient_id, company_id) references ingredients (id, company_id),
  constraint notifications_product_fkey foreign key (company_id, product_id) references products (company_id, id),
  constraint notifications_opened_by_fkey foreign key (company_id, opened_by) references users (company_id, id),
  constraint notifications_resolved_by_fkey foreign key (company_id, resolved_by) references users (company_id, id),
  constraint notifications_email_send_fkey foreign key (company_id, email_send_id)
    references notification_email_sends (company_id, id) on delete set null (email_send_id)
);
-- Un aviso abierto por sucursal, artículo y tipo: dos ventas que cruzan el mismo mínimo abren uno.
create unique index notifications_one_open on notifications (company_id, branch_id, item_key, kind)
  where status = 'open' and kind <> 'not_found';
-- Cerrar todo lo de un artículo (archivar, unir, cambiar de unidad) en todas las sucursales.
create index notifications_by_item on notifications (company_id, item_key);
create index notifications_open_recent on notifications (company_id, opened_at desc) where status = 'open';
create index notifications_closed_recent on notifications (company_id, resolved_at desc) where status <> 'open';
create index notifications_email_pending on notifications (company_id, opened_at) where email_state = 'pending';
create index notifications_email_send on notifications (email_send_id) where email_send_id is not null;

-- ---------------------------------------------------------------------------------------------
-- 5. Leído por persona (decisión del dueño), y si ya vio la franja.
-- ---------------------------------------------------------------------------------------------
create table notification_reads (
  company_id      bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                  references companies(id) on delete cascade,
  notification_id bigint not null,
  user_id         bigint not null,
  read_at         timestamptz,
  banner_seen_at  timestamptz,
  primary key (company_id, notification_id, user_id),
  constraint notification_reads_notification_fkey foreign key (company_id, notification_id)
    references notifications (company_id, id) on delete cascade,
  constraint notification_reads_user_fkey foreign key (company_id, user_id) references users (company_id, id)
);
create index notification_reads_by_user on notification_reads (company_id, user_id);

-- ---------------------------------------------------------------------------------------------
-- 6. «Se acabó» y «No lo encuentro»: cada reporte, su ajuste y su deshacer. client_key es la
--    idempotencia de la pantalla (un doble envío devuelve el primero).
-- ---------------------------------------------------------------------------------------------
create table stock_out_reports (
  id               bigint generated always as identity primary key,
  company_id       bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                   references companies(id) on delete cascade,
  branch_id        bigint not null,
  kind             text not null constraint stock_out_reports_kind check (kind in ('report', 'confirmation', 'not_found')),
  item_type        stock_item_type,
  ingredient_id    bigint,
  product_id       bigint,
  notification_id  bigint not null,
  -- stock_movements no tiene unique(company_id, id) y crearlo en la tabla más grande no compra nada:
  -- se escribe bajo RLS. Llave simple.
  movement_id      bigint references stock_movements (id),
  delta            numeric(14,4),
  client_key       uuid not null,
  reported_by      bigint not null,
  reported_at      timestamptz not null default now(),
  undone_at        timestamptz,
  undo_movement_id bigint references stock_movements (id),
  constraint stock_out_reports_item_shape check (
    case kind
      when 'not_found' then item_type is null and ingredient_id is null and product_id is null
      else (item_type = 'ingrediente' and ingredient_id is not null and product_id is null) or
           (item_type = 'producto' and product_id is not null and ingredient_id is null)
    end),
  constraint stock_out_reports_movement_shape check ((movement_id is null) = (delta is null)),
  -- Deshacer una confirmación (o un reporte con existencia en 0) no mueve nada: no hay ajuste de vuelta.
  constraint stock_out_reports_undo_shape check (undo_movement_id is null or undone_at is not null),
  constraint stock_out_reports_client_key unique (company_id, client_key),
  constraint stock_out_reports_branch_fkey foreign key (company_id, branch_id) references branches (company_id, id),
  constraint stock_out_reports_ingredient_fkey foreign key (ingredient_id, company_id) references ingredients (id, company_id),
  constraint stock_out_reports_product_fkey foreign key (company_id, product_id) references products (company_id, id),
  constraint stock_out_reports_notification_fkey foreign key (company_id, notification_id) references notifications (company_id, id),
  constraint stock_out_reports_user_fkey foreign key (company_id, reported_by) references users (company_id, id)
);
create index stock_out_reports_by_user on stock_out_reports (company_id, reported_by, reported_at);
create index stock_out_reports_by_notification on stock_out_reports (company_id, notification_id);
create index stock_out_reports_by_ingredient on stock_out_reports (company_id, ingredient_id) where ingredient_id is not null;
create index stock_out_reports_by_product on stock_out_reports (company_id, product_id) where product_id is not null;

-- ---------------------------------------------------------------------------------------------
-- 7. Quién recibe el correo, además de la omisión (quien configura el almacén y tiene correo):
--    enabled = false quita a alguien de la omisión; un correo extra no necesita usuario.
-- ---------------------------------------------------------------------------------------------
create table notification_recipients (
  id         bigint generated always as identity primary key,
  company_id bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
             references companies(id) on delete cascade,
  user_id    bigint,
  email      citext constraint notification_recipients_email_len check (char_length(email) between 3 and 254),
  kind       text not null constraint notification_recipients_kind check (kind in ('stock', 'not_found')),
  enabled    boolean not null,
  updated_by bigint not null,
  updated_at timestamptz not null default now(),
  constraint notification_recipients_one_target check ((user_id is null) <> (email is null)),
  constraint notification_recipients_user_fkey foreign key (company_id, user_id) references users (company_id, id),
  constraint notification_recipients_updated_by_fkey foreign key (company_id, updated_by) references users (company_id, id)
);
create unique index notification_recipients_user_key on notification_recipients (company_id, user_id, kind) where user_id is not null;
create unique index notification_recipients_email_key on notification_recipients (company_id, email, kind) where email is not null;

-- ---------------------------------------------------------------------------------------------
-- 8. Aislamiento por empresa y permisos. Sin delete en avisos ni reportes: se cierran por estado.
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
do $$
declare
  tbl text;
begin
  foreach tbl in array array['branch_stock_minimums', 'notification_email_sends', 'notifications',
                             'notification_reads', 'stock_out_reports', 'notification_recipients'] loop
    execute format('alter table %I enable row level security', tbl);
    execute format($p$create policy tenant_isolation on %I
      using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
      with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint)$p$, tbl);
  end loop;
end $$;
-- +goose StatementEnd
grant select, insert, update on notifications, notification_reads, stock_out_reports, notification_email_sends to gatobobah_app;
grant select, insert, update, delete on notification_recipients, branch_stock_minimums to gatobobah_app;

-- Unión de insumos (0104): nada nuevo cuelga de un origen unido.
-- +goose StatementBegin
do $$
declare
  tbl text;
begin
  foreach tbl in array array['stock_out_reports', 'branch_stock_minimums'] loop
    execute format('create trigger %I before insert or update of ingredient_id on %I
                      for each row execute function reject_merged_ingredient()', tbl || '_merged_guard', tbl);
    execute format('create constraint trigger %I after insert or update of ingredient_id on %I
                      deferrable initially deferred for each row execute function reject_merged_ingredient()',
                   tbl || '_merged_check', tbl);
  end loop;
end $$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------------------------
-- 9. La regla. low_stock_state lee el estado (una sentencia, sin subtransacción); el trigger solo
--    llama a evaluate_low_stock cuando lo que debería estar abierto difiere de lo que está. La tabla
--    de verdad es domain.LowStockShould y la prueba compara las dos.
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
create function low_stock_state(p_company bigint, p_branch bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint)
returns table (active boolean, enabled boolean, on_min boolean, should_low boolean, should_out boolean, on_hand numeric, min_stock numeric,
               low_open boolean, out_open boolean, out_manual boolean, item_name text, unit_code text)
language sql stable as $$
  with art as (
    select i.is_active and not exists (
             select 1 from ingredient_replacements r
              where r.company_id = i.company_id and r.old_ingredient_id = i.id and r.kind = 'merge') as active,
           i.low_stock_enabled as enabled, i.low_stock_on_min as on_min, i.low_stock_on_out as on_out,
           coalesce(b.min_stock, i.min_stock) as min_stock, i.name, u.code as unit_code
      from ingredients i
      join units u on u.id = i.base_unit_id
      left join branch_stock_minimums b on b.branch_id = p_branch and b.ingredient_id = i.id
     where p_type = 'ingrediente' and i.id = p_ingredient and i.company_id = p_company
    union all
    select p.is_active, p.low_stock_enabled, p.low_stock_on_min, p.low_stock_on_out,
           coalesce(b.min_stock, p.min_stock), p.name, 'pieza'
      from products p
      left join branch_stock_minimums b on b.branch_id = p_branch and b.product_id = p.id
     where p_type = 'producto' and p.id = p_product and p.company_id = p_company
  ), lvl as (
    -- Dos ramas con guarda y no un case: así cada una usa su índice único (sucursal, artículo); con
    -- el case, el plan genérico recorría stock_levels completa en cada movimiento.
    select coalesce((select s.on_hand from stock_levels s
                      where p_type = 'ingrediente' and s.branch_id = p_branch and s.ingredient_id = p_ingredient
                     union all
                     select s.on_hand from stock_levels s
                      where p_type = 'producto' and s.branch_id = p_branch and s.product_id = p_product), 0) as on_hand
  ), open as (
    select bool_or(n.kind = 'stock_low') as low_open, bool_or(n.kind = 'stock_out') as out_open,
           bool_or(n.kind = 'stock_out' and n.opened_by is not null) as out_manual
      from notifications n
     where n.company_id = p_company and n.branch_id = p_branch and n.status = 'open' and n.kind <> 'not_found'
       and n.item_key = coalesce(p_ingredient, -p_product, 0)
  )
  select a.active, a.enabled, a.on_min,
         a.active and a.enabled and a.on_min and coalesce(a.min_stock, 0) > 0 and l.on_hand <= a.min_stock,
         a.active and a.enabled and a.on_out and l.on_hand <= 0,
         l.on_hand, a.min_stock,
         coalesce(o.low_open, false), coalesce(o.out_open, false), coalesce(o.out_manual, false),
         a.name, a.unit_code
    from art a cross join lvl l cross join open o
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- p_count_id: el conteo cuyo ajuste dispara la evaluación (el trigger lo pasa). Ese aviso no pide
-- conteo, y lo que cierra lo cierra «contado».
create function evaluate_low_stock(p_company bigint, p_branch bigint, p_type stock_item_type, p_ingredient bigint, p_product bigint,
                                   p_count_id bigint default null)
returns void language plpgsql as $$
declare
  s record;
  v_needs boolean;
  v_changed integer := 0;
  v_n integer;
begin
  select * into s from low_stock_state(p_company, p_branch, p_type, p_ingredient, p_product);
  if not found then
    return;
  end if;

  -- Cerrar lo que ya no aplica. El «se acabó» que reportó una persona no se cierra por apagar la
  -- alerta: solo cuando vuelve a haber.
  update notifications n
     set status = 'resolved', resolved_at = now(),
         resolved_reason = case
           when not s.active then 'archived'
           when p_count_id is not null then 'counted'
           when n.kind = 'stock_low' then
             case when s.enabled and s.on_min and coalesce(s.min_stock, 0) > 0 then 'above_min' else 'alert_off' end
           when s.on_hand > 0 then 'restocked'
           else 'alert_off' end
   where n.company_id = p_company and n.branch_id = p_branch and n.status = 'open' and n.kind <> 'not_found'
     and n.item_key = coalesce(p_ingredient, -p_product, 0)
     and (not s.active
          or (n.kind = 'stock_low' and not s.should_low)
          or (n.kind = 'stock_out' and not s.should_out and (n.opened_by is null or s.on_hand > 0)));
  get diagnostics v_n = row_count;
  v_changed := v_changed + v_n;

  if s.should_low or s.should_out then
    -- «Pedir conteo»: con la existencia en negativo, o sin un conteo cerrado en 30 días, la cifra del
    -- sistema no es para comprar.
    v_needs := s.on_hand < 0 or (p_count_id is null and not exists (
      select 1 from stock_count_lines l join stock_counts c on c.id = l.count_id and c.company_id = l.company_id
       where p_type = 'ingrediente' and l.ingredient_id = p_ingredient and c.branch_id = p_branch
         and c.status = 'closed' and c.closed_at > now() - interval '30 days'
      union all
      select 1 from stock_count_product_lines l join stock_counts c on c.id = l.count_id and c.company_id = l.company_id
       where p_type = 'producto' and l.product_id = p_product and c.branch_id = p_branch
         and c.status = 'closed' and c.closed_at > now() - interval '30 days'));
    insert into notifications (company_id, branch_id, kind, item_type, ingredient_id, product_id, item_name, unit_code,
                               on_hand_seen, min_seen, needs_count)
    select p_company, p_branch, k, p_type, p_ingredient, p_product, s.item_name, s.unit_code, s.on_hand, s.min_stock, v_needs
      from unnest(array[case when s.should_low then 'stock_low' end, case when s.should_out then 'stock_out' end]) k
     where k is not null
    on conflict (company_id, branch_id, item_key, kind) where status = 'open' and kind <> 'not_found' do nothing;
    get diagnostics v_n = row_count;
    v_changed := v_changed + v_n;
  end if;

  -- NOTIFY solo si algo cambió: toma al confirmar un candado global de la cola, y en cada venta
  -- serializaría los cobros. Es transaccional: si la venta se revierte, el aviso en vivo no sale.
  if v_changed > 0 then
    perform pg_notify('notifications', p_company::text);
  end if;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function notify_stock_movement() returns trigger language plpgsql as $$
declare
  s record;
begin
  -- Los traspasos de una unión o de un cambio de unidad no son consumo: los evalúa el servicio que
  -- los hace, al final, con todo ya convertido.
  if new.replacement_id is not null then
    return null;
  end if;
  select * into s from low_stock_state(new.company_id, new.branch_id, new.item_type, new.ingredient_id, new.product_id);
  if not found then
    return null;
  end if;
  -- Solo cuando lo que debería estar abierto difiere de lo que está. Un artículo ya bajo el mínimo
  -- con su aviso abierto (cada venta de una mala racha) no entra al bloque: cada bloque con
  -- excepción es una subtransacción, y con más de 64 vivas en una sesión la base entera se alenta.
  if (not s.active and (s.low_open or s.out_open))
     or s.should_low <> s.low_open
     or (s.should_out and not s.out_open)
     or (s.out_open and not s.should_out and (not s.out_manual or s.on_hand > 0)) then
    begin
      perform evaluate_low_stock(new.company_id, new.branch_id, new.item_type, new.ingredient_id, new.product_id, new.stock_count_id);
    exception when others then
      -- La venta nunca falla por el aviso. Como la regla es por estado, el siguiente movimiento del
      -- artículo (o el barrido de la mañana) lo vuelve a intentar.
      raise warning 'notification_failed: % (%)', sqlerrm, sqlstate;
    end;
  end if;
  return null;
end;
$$;
-- +goose StatementEnd

-- Corre después de trg_stock_movement (mismo evento, orden por nombre): ve la existencia ya sumada.
create trigger trg_stock_movement_notify after insert on stock_movements
  for each row execute function notify_stock_movement();

revoke execute on function low_stock_state(bigint, bigint, stock_item_type, bigint, bigint) from public;
revoke execute on function evaluate_low_stock(bigint, bigint, stock_item_type, bigint, bigint, bigint) from public;
grant execute on function low_stock_state(bigint, bigint, stock_item_type, bigint, bigint) to gatobobah_app;
grant execute on function evaluate_low_stock(bigint, bigint, stock_item_type, bigint, bigint, bigint) to gatobobah_app;

-- ---------------------------------------------------------------------------------------------
-- 10. Permiso nuevo: reportar que algo se acabó sin poder ajustar existencias. Lo traen los cuatro
--     roles sugeridos, como «Se echó a perder».
-- ---------------------------------------------------------------------------------------------
insert into permissions (id, code, area) values (38, 'inventory.report_out', 'inventory');
insert into system_role_permissions (system_role_id, permission_id) values (1, 38), (2, 38), (3, 38), (4, 38);

-- +goose Down
-- Se niega con avisos, reportes o configuración: bajar perdería quién reportó qué y las alertas que
-- alguien encendió.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from notifications) or exists (select 1 from stock_out_reports)
     or exists (select 1 from notification_recipients) or exists (select 1 from branch_stock_minimums)
     or exists (select 1 from notification_email_sends)
     or exists (select 1 from ingredients where low_stock_enabled or coverage_days is not null or low_stock_asked_at is not null
                                             or low_stock_banner or not low_stock_on_min or not low_stock_on_out)
     or exists (select 1 from products where low_stock_enabled or coverage_days is not null or low_stock_asked_at is not null
                                          or low_stock_banner or not low_stock_on_min or not low_stock_on_out)
     or exists (select 1 from business_settings where notifications_morning_at <> '08:00') then
    raise exception 'hay avisos o configuración de alertas: bajar los perdería';
  end if;
end $$;
-- +goose StatementEnd
drop trigger trg_stock_movement_notify on stock_movements;
drop function notify_stock_movement();
drop function evaluate_low_stock(bigint, bigint, stock_item_type, bigint, bigint, bigint);
drop function low_stock_state(bigint, bigint, stock_item_type, bigint, bigint);
drop table notification_reads;
drop table stock_out_reports;
drop table notifications;
drop table notification_email_sends;
drop table notification_recipients;
drop table branch_stock_minimums;
alter table business_settings drop column notifications_morning_at;
alter table products drop column low_stock_enabled, drop column low_stock_on_min, drop column low_stock_on_out,
  drop column low_stock_banner, drop column coverage_days, drop column low_stock_asked_at;
alter table ingredients drop column low_stock_enabled, drop column low_stock_on_min, drop column low_stock_on_out,
  drop column low_stock_banner, drop column coverage_days, drop column low_stock_asked_at;
delete from role_permission_overrides where permission_id = 38;
delete from system_role_permissions where permission_id = 38;
delete from permissions where id = 38;
