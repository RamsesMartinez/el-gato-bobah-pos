-- +goose Up
-- Agrega company_id a TODAS las tablas de negocio (todo menos companies, units y goose).
-- Patrón uniforme por tabla: add nullable → backfill a la empresa por defecto → not null →
-- default current_setting (auto-sella el tenant en cada INSERT del app en runtime).
-- units queda GLOBAL (kg/ml/pieza son universales); companies es el tenant mismo.

-- +goose StatementBegin
do $$
declare
  t   text;
  cid bigint;
  tables text[] := array[
    'business_settings','categories','category_channels','channels','combo_slot_products',
    'combo_slots','delivery_platforms','expense_categories','expenses','fudo_import_map',
    'ingredient_categories','ingredient_purchase_formats','ingredients','modifier_groups',
    'modifier_options','order_counters','order_line_modifiers','order_lines','order_payments',
    'orders','product_channels','product_modifier_groups','products','recipe_items','recipes',
    'refresh_tokens','register_cash_movements','register_session_totals','register_sessions',
    'stock_levels','stock_movements','suppliers','user_preferences'
  ];
begin
  select id into cid from companies where slug = 'gatobobah';
  foreach t in array tables loop
    execute format('alter table %I add column company_id bigint references companies(id) on delete cascade', t);
    execute format('update %I set company_id = %L', t, cid);
    execute format('alter table %I alter column company_id set not null', t);
    execute format('alter table %I alter column company_id set default current_setting(''app.company_id'', true)::bigint', t);
    execute format('create index %I on %I (company_id)', t || '_company', t);
  end loop;
end $$;
-- +goose StatementEnd

-- Uniques que dejan de ser globales y pasan a ser por-empresa (dos empresas pueden repetir nombre/código).
alter table channels drop constraint channels_code_key;
alter table channels add constraint channels_company_code_key unique (company_id, code);

alter table delivery_platforms drop constraint delivery_platforms_name_key;
alter table delivery_platforms add constraint delivery_platforms_company_name_key unique (company_id, name);

alter table expense_categories drop constraint expense_categories_financial_group_name_key;
alter table expense_categories add constraint expense_categories_company_key unique (company_id, financial_group, name);

alter table ingredient_categories drop constraint ingredient_categories_name_key;
alter table ingredient_categories add constraint ingredient_categories_company_name_key unique (company_id, name);

alter table ingredients drop constraint ingredients_name_key;
alter table ingredients add constraint ingredients_company_name_key unique (company_id, name);

alter table modifier_groups drop constraint modifier_groups_name_key;
alter table modifier_groups add constraint modifier_groups_company_name_key unique (company_id, name);

alter table products drop constraint products_name_key;
alter table products add constraint products_company_name_key unique (company_id, name);
alter table products drop constraint products_sku_key;
alter table products add constraint products_company_sku_key unique (company_id, sku);

alter table suppliers drop constraint suppliers_name_key;
alter table suppliers add constraint suppliers_company_name_key unique (company_id, name);

alter table orders drop constraint orders_client_uuid_key;
alter table orders add constraint orders_company_client_uuid_key unique (company_id, client_uuid);
alter table orders drop constraint orders_business_date_daily_number_key;
alter table orders add constraint orders_company_daily_key unique (company_id, business_date, daily_number);

-- Contador diario de folios: uno por empresa por día.
alter table order_counters drop constraint order_counters_pkey;
alter table order_counters add primary key (company_id, business_date);

-- Una sola caja abierta POR EMPRESA (antes: una global).
drop index one_open_session;
create unique index one_open_session on register_sessions (company_id, status) where status = 'abierta';

-- business_settings era fila única global (truco id boolean). Ahora es una fila por empresa.
alter table business_settings drop constraint business_settings_pkey;
alter table business_settings drop column id; -- arrastra el check(id)
alter table business_settings add primary key (company_id);

-- +goose Down
alter table business_settings drop constraint business_settings_pkey;
alter table business_settings add column id boolean not null default true;
alter table business_settings add constraint business_settings_id_check check (id);
alter table business_settings add primary key (id);

drop index one_open_session;
alter table order_counters drop constraint order_counters_pkey;

alter table orders drop constraint orders_company_daily_key;
alter table orders drop constraint orders_company_client_uuid_key;
alter table suppliers drop constraint suppliers_company_name_key;
alter table products drop constraint products_company_sku_key;
alter table products drop constraint products_company_name_key;
alter table modifier_groups drop constraint modifier_groups_company_name_key;
alter table ingredients drop constraint ingredients_company_name_key;
alter table ingredient_categories drop constraint ingredient_categories_company_name_key;
alter table expense_categories drop constraint expense_categories_company_key;
alter table delivery_platforms drop constraint delivery_platforms_company_name_key;
alter table channels drop constraint channels_company_code_key;

-- +goose StatementBegin
do $$
declare
  t   text;
  tables text[] := array[
    'business_settings','categories','category_channels','channels','combo_slot_products',
    'combo_slots','delivery_platforms','expense_categories','expenses','fudo_import_map',
    'ingredient_categories','ingredient_purchase_formats','ingredients','modifier_groups',
    'modifier_options','order_counters','order_line_modifiers','order_lines','order_payments',
    'orders','product_channels','product_modifier_groups','products','recipe_items','recipes',
    'refresh_tokens','register_cash_movements','register_session_totals','register_sessions',
    'stock_levels','stock_movements','suppliers','user_preferences'
  ];
begin
  foreach t in array tables loop
    execute format('alter table %I drop column company_id cascade', t);
  end loop;
end $$;
-- +goose StatementEnd

-- Restaura uniques/pks globales originales.
alter table order_counters add primary key (business_date);
create unique index one_open_session on register_sessions (status) where status = 'abierta';
alter table channels add constraint channels_code_key unique (code);
alter table delivery_platforms add constraint delivery_platforms_name_key unique (name);
alter table expense_categories add constraint expense_categories_financial_group_name_key unique (financial_group, name);
alter table ingredient_categories add constraint ingredient_categories_name_key unique (name);
alter table ingredients add constraint ingredients_name_key unique (name);
alter table modifier_groups add constraint modifier_groups_name_key unique (name);
alter table products add constraint products_name_key unique (name);
alter table products add constraint products_sku_key unique (sku);
alter table suppliers add constraint suppliers_name_key unique (name);
alter table orders add constraint orders_client_uuid_key unique (client_uuid);
alter table orders add constraint orders_business_date_daily_number_key unique (business_date, daily_number);
