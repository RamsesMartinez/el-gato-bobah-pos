-- +goose Up
-- Aislamiento de tenant a nivel BASE DE DATOS (defensa en profundidad): aunque una query del
-- app olvide filtrar por company_id, Postgres RLS rechaza filas de otro tenant. Se apoya en el
-- GUC de sesión app.company_id, fijado por request en store.WithTenant (SET LOCAL).
--
-- Clave del modelo: el app se conecta como gatobobah_app (NO owner, NO superuser) → RLS aplica.
-- Las migraciones/bootstrap corren como el owner (gatobobah) → RLS NO aplica (sin FORCE), por eso
-- backfills y DDL funcionan. Los superusuarios siempre saltan RLS; por eso el app NO es superuser.

-- Rol del app. Sin password aquí (no versionar secretos): el bootstrap le fija el password desde
-- APP_DB_PASSWORD al arrancar. Un LOGIN sin password no puede conectarse hasta entonces.
-- +goose StatementBegin
do $$ begin
  if not exists (select 1 from pg_roles where rolname = 'gatobobah_app') then
    create role gatobobah_app login;
  end if;
end $$;
-- +goose StatementEnd

-- RLS + policy de aislamiento en cada tabla per-tenant (todas menos units/companies/goose).
-- +goose StatementBegin
do $$
declare
  t text;
  tables text[] := array[
    'business_settings','categories','category_channels','channels','combo_slot_products',
    'combo_slots','delivery_platforms','expense_categories','expenses','fudo_import_map',
    'ingredient_categories','ingredient_purchase_formats','ingredients','modifier_groups',
    'modifier_options','order_counters','order_line_modifiers','order_lines','order_payments',
    'orders','product_channels','product_modifier_groups','products','recipe_items','recipes',
    'refresh_tokens','register_cash_movements','register_session_totals','register_sessions',
    'stock_levels','stock_movements','suppliers','user_preferences','users'
  ];
begin
  foreach t in array tables loop
    execute format('alter table %I enable row level security', t);
    execute format($f$create policy tenant_isolation on %I
      using (company_id = current_setting('app.company_id', true)::bigint)
      with check (company_id = current_setting('app.company_id', true)::bigint)$f$, t);
  end loop;
end $$;
-- +goose StatementEnd

-- companies: el app solo ve/edita SU propia fila (id = tenant actual). Crear empresas es una
-- operación de plataforma (superuser/CLI), por eso abajo se le revoca insert/delete.
alter table companies enable row level security;
create policy company_self on companies
  using (id = current_setting('app.company_id', true)::bigint)
  with check (id = current_setting('app.company_id', true)::bigint);

-- Resolvers SECURITY DEFINER: las ÚNICAS lecturas que ocurren ANTES de conocer el tenant
-- (login por slug, refresh por hash de token). Corren como el owner → saltan RLS de forma
-- acotada y controlada (hay que poseer ya el slug/token). search_path fijado por seguridad.
-- +goose StatementBegin
create function app_resolve_company(p_slug citext) returns bigint
  language sql stable security definer set search_path = public as $$
  select id from companies where slug = p_slug and is_active limit 1;
$$;
-- +goose StatementEnd

-- Privilegios del rol del app. RLS es la barrera; los grants dan el acceso base.
grant usage on schema public to gatobobah_app;
grant select, insert, update, delete on all tables in schema public to gatobobah_app;
grant usage, select on all sequences in schema public to gatobobah_app;
-- units es referencia global compartida (kg/ml/pieza): el app la lee, no la modifica.
revoke insert, update, delete on units from gatobobah_app;
-- companies: crear/borrar empresas es de plataforma; el app solo edita su propia fila (RLS).
revoke insert, delete on companies from gatobobah_app;
grant execute on function app_resolve_company(citext) to gatobobah_app;

-- +goose Down
drop function if exists app_resolve_company(citext);
drop policy if exists company_self on companies;
alter table companies disable row level security;
-- +goose StatementBegin
do $$
declare
  t text;
  tables text[] := array[
    'business_settings','categories','category_channels','channels','combo_slot_products',
    'combo_slots','delivery_platforms','expense_categories','expenses','fudo_import_map',
    'ingredient_categories','ingredient_purchase_formats','ingredients','modifier_groups',
    'modifier_options','order_counters','order_line_modifiers','order_lines','order_payments',
    'orders','product_channels','product_modifier_groups','products','recipe_items','recipes',
    'refresh_tokens','register_cash_movements','register_session_totals','register_sessions',
    'stock_levels','stock_movements','suppliers','user_preferences','users'
  ];
begin
  foreach t in array tables loop
    execute format('drop policy if exists tenant_isolation on %I', t);
    execute format('alter table %I disable row level security', t);
  end loop;
end $$;
-- +goose StatementEnd
-- +goose StatementBegin
do $$ begin
  if exists (select 1 from pg_roles where rolname = 'gatobobah_app') then
    execute 'drop owned by gatobobah_app';
    drop role gatobobah_app;
  end if;
end $$;
-- +goose StatementEnd
