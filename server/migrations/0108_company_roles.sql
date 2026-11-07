-- Roles por empresa (spec 068, issue #61; decisiones del dueño del 2026-11-06). Cruza la puerta de
-- roles del principio VIII.
--
-- Universal, sin empresa y con el mismo id en todos los ambientes: el catálogo de permisos, los
-- cuatro roles sugeridos y lo que el producto le da a cada uno. Por empresa, con RLS y llaves
-- compuestas: sus roles (un renglón por sugerido, más los propios) y lo que la empresa les agregó o
-- quitó. Lo que puede un sugerido = lo del producto + agregado − quitado; un permiso nuevo del
-- producto es una fila en system_role_permissions y llega a todas las empresas (herencia).
--
-- CONVIVE CON LA API ANTERIOR: todo es aditivo. La anterior sigue leyendo y escribiendo
-- `users.role`; la nueva, `users.role_id`. El trigger sync_user_role mantiene los dos coherentes
-- hasta la 0109, que borra `users.role` cuando ninguna versión en producción lo lee.

-- +goose Up
set local lock_timeout = '3s';

create table permissions (
  id   smallint primary key,
  code text not null unique check (code ~ '^[a-z_]+\.[a-z_]+$'),
  area text not null
);

-- Los ids no se reusan nunca: los guardan los ajustes de cada empresa. El orden es el de
-- domain.AllPermissions y lo vigila TestThePermissionCatalogMatchesTheDomain.
insert into permissions (id, code, area) values
  (1, 'payments.void', 'orders'),
  (2, 'orders.cancel', 'orders'),
  (3, 'orders.move_lines', 'orders'),
  (4, 'orders.cancel_pending', 'orders'),
  (5, 'attendance.configure', 'team'),
  (6, 'attendance.report', 'team'),
  (7, 'attendance.correct_others', 'team'),
  (8, 'attendance.correct_own', 'team'),
  (9, 'audit.view', 'team'),
  (10, 'payments.change_terminal', 'orders'),
  (11, 'cash.configure', 'cash'),
  (12, 'inventory.health_view', 'inventory'),
  (13, 'menu.configure', 'menu'),
  (14, 'inventory.receive', 'inventory'),
  (15, 'inventory.configure', 'inventory'),
  (16, 'inventory.count', 'inventory'),
  (17, 'inventory.write_off', 'inventory'),
  (18, 'inventory.write_off_approve', 'inventory'),
  (19, 'inventory.view', 'inventory'),
  (20, 'cash.close_without_clock_out', 'cash'),
  (21, 'cash.adjust_count', 'cash'),
  (22, 'expenses.edit', 'expenses'),
  (23, 'expenses.cancel', 'expenses'),
  (24, 'orders.delivered_view', 'orders'),
  (25, 'cash.operate', 'cash'),
  (26, 'orders.platform_ref', 'orders'),
  (27, 'platform_prices.capture', 'platforms'),
  (28, 'sales.view', 'sales'),
  (29, 'platform_settlements.capture', 'sales'),
  (30, 'reports.view', 'sales'),
  (31, 'expenses.capture', 'expenses'),
  (32, 'platforms.configure', 'platforms'),
  (33, 'platforms.credentials', 'platforms'),
  (34, 'business.configure', 'business'),
  (35, 'employees.manage', 'team'),
  (36, 'roles.manage', 'team');

create table system_roles (
  id   smallint primary key,
  code text not null unique,
  name text not null unique
);
insert into system_roles (id, code, name) values
  (1, 'admin', 'Admin'), (2, 'gerente', 'Gerente'), (3, 'cajero', 'Cajero'), (4, 'mesero', 'Mesero');

create table system_role_permissions (
  system_role_id smallint not null references system_roles (id) on delete restrict,
  permission_id  smallint not null references permissions (id) on delete restrict,
  primary key (system_role_id, permission_id)
);
insert into system_role_permissions (system_role_id, permission_id) select 1, id from permissions;
insert into system_role_permissions (system_role_id, permission_id)
  select 2, id from permissions where code in ('payments.void', 'orders.cancel', 'orders.move_lines', 'orders.cancel_pending', 'attendance.report', 'attendance.correct_others', 'audit.view', 'payments.change_terminal', 'cash.configure', 'inventory.health_view', 'menu.configure', 'inventory.receive', 'inventory.configure', 'inventory.count', 'inventory.write_off', 'inventory.write_off_approve', 'inventory.view', 'cash.close_without_clock_out', 'cash.adjust_count', 'expenses.cancel', 'orders.delivered_view', 'cash.operate', 'orders.platform_ref', 'platform_prices.capture', 'sales.view', 'platform_settlements.capture', 'reports.view', 'expenses.capture', 'platforms.configure', 'business.configure');
insert into system_role_permissions (system_role_id, permission_id)
  select 3, id from permissions where code in ('orders.move_lines', 'orders.cancel_pending', 'inventory.write_off', 'orders.delivered_view', 'cash.operate', 'orders.platform_ref', 'platform_prices.capture');
insert into system_role_permissions (system_role_id, permission_id)
  select 4, id from permissions where code in ('orders.move_lines', 'orders.cancel_pending', 'inventory.write_off');

grant select on permissions, system_roles, system_role_permissions to gatobobah_app;

create table roles (
  id             bigint generated always as identity primary key,
  company_id     bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                 references companies (id) on delete cascade,
  system_role_id smallint references system_roles (id) on delete restrict,
  name           text,
  active         boolean not null default true,
  version        integer not null default 1,
  created_at     timestamptz not null default now(),
  updated_at     timestamptz not null default now(),
  constraint roles_company_id_key unique (company_id, id),
  constraint roles_system_key unique (company_id, system_role_id),
  -- Un sugerido toma el nombre del producto (system_roles.name); un propio trae el suyo.
  constraint roles_kind check ((system_role_id is null) = (name is not null)),
  constraint roles_name_length check (name is null or char_length(name) between 1 and 40),
  -- El admin no se inactiva: una empresa sin quien administre roles no tiene salida.
  constraint roles_admin_active check (system_role_id is distinct from 1 or active)
);
-- El choque con los nombres de los sugeridos lo revisa la API bajo el candado 28068.
create unique index roles_name_key on roles (company_id, catalog_name_key(name)) where name is not null;
alter table roles enable row level security;
create policy tenant_isolation on roles
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update, delete on roles to gatobobah_app;

-- Lo que la empresa agregó (granted) o quitó (not granted) a uno de sus roles. En un rol propio solo
-- hay agregados: no tiene base de la cual quitar.
create table role_permission_overrides (
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                references companies (id) on delete cascade,
  role_id       bigint not null,
  permission_id smallint not null references permissions (id) on delete restrict,
  granted       boolean not null,
  primary key (company_id, role_id, permission_id),
  constraint role_permission_overrides_role_fkey
    foreign key (company_id, role_id) references roles (company_id, id) on delete cascade
);
alter table role_permission_overrides enable row level security;
create policy tenant_isolation on role_permission_overrides
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update, delete on role_permission_overrides to gatobobah_app;

-- Nullable hasta la 0109. La llave compuesta impide asignar un rol de otra empresa aunque RLS
-- fallara: las llaves foráneas no pasan por RLS.
alter table users add column role_id bigint;
alter table users add constraint users_role_fkey
  foreign key (company_id, role_id) references roles (company_id, id) on delete restrict;
create index users_role_idx on users (company_id, role_id);

-- Los cuatro sugeridos de una empresa, sin duplicar. La llaman esta migración, CreateCompany y el
-- trigger de abajo. Security definer con search_path fijo: corre igual como owner, como la
-- aplicación y dentro de pg_restore, que vacía el search_path.
-- +goose StatementBegin
create function ensure_company_roles(p_company bigint) returns void
language sql security definer set search_path = pg_catalog, public, pg_temp as $$
  insert into public.roles (company_id, system_role_id)
  select p_company, s.id from public.system_roles s
  on conflict (company_id, system_role_id) do nothing;
$$;
-- +goose StatementEnd
revoke execute on function ensure_company_roles(bigint) from public;
grant execute on function ensure_company_roles(bigint) to gatobobah_app;

select ensure_company_roles(id) from companies;

update users u set role_id = r.id
  from roles r join system_roles s on s.id = r.system_role_id
 where r.company_id = u.company_id and s.code = u.role::text and u.role_id is null;

-- sync_user_role mantiene `role` y `role_id` coherentes mientras conviven las dos APIs. Filtra por
-- la empresa de la FILA, nunca por app.company_id (un script como owner no lo tiene).
--   - Insert con role_id y sin role (la API nueva): role = código del sugerido, o 'mesero' si es
--     propio (lo mínimo para la API anterior, que no conoce los roles propios).
--   - Insert con role y sin role_id (la API anterior): role_id = el sugerido equivalente; si la
--     empresa no tiene sus roles (la creó la API anterior durante la subida), se crean.
--   - Insert con los dos (pg_restore): se respetan.
--   - Update: si cambia role_id, gana role_id; si solo cambia role, role_id lo sigue.
-- +goose StatementBegin
create function sync_user_role() returns trigger
language plpgsql security definer set search_path = pg_catalog, public, pg_temp as $$
declare
  v_code text;
  from_role_id boolean;
  from_role boolean;
begin
  if tg_op = 'INSERT' then
    from_role_id := new.role_id is not null and new.role is null;
    from_role := new.role_id is null and new.role is not null;
  else
    from_role_id := new.role_id is distinct from old.role_id and new.role_id is not null;
    from_role := not from_role_id and new.role is distinct from old.role;
  end if;

  if from_role_id then
    select s.code into v_code
      from public.roles r left join public.system_roles s on s.id = r.system_role_id
     where r.company_id = new.company_id and r.id = new.role_id;
    new.role := coalesce(v_code, 'mesero')::public.user_role;
  elsif from_role then
    perform public.ensure_company_roles(new.company_id);
    select r.id into new.role_id
      from public.roles r join public.system_roles s on s.id = r.system_role_id
     where r.company_id = new.company_id and s.code = new.role::text;
  end if;
  return new;
end;
$$;
-- +goose StatementEnd
revoke execute on function sync_user_role() from public;
create trigger users_sync_role before insert or update of role, role_id on users
  for each row execute function sync_user_role();

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from roles where system_role_id is null)
     or exists (select 1 from role_permission_overrides) then
    raise exception '0108: hay roles propios o ajustes de permisos; bajar los perdería';
  end if;
end $$;
-- +goose StatementEnd
drop trigger users_sync_role on users;
drop function sync_user_role();
alter table users drop column role_id;
drop table role_permission_overrides;
drop table roles;
drop function ensure_company_roles(bigint);
drop table system_role_permissions;
drop table system_roles;
drop table permissions;
