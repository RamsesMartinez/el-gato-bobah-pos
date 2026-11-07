-- Roles por empresa, entrega 3 (spec 068): borra users.role. Desde la 0108 el rol es role_id, y la
-- API que corre durante esta subida (la de la entrega 2, con el arreglo de #239) ya no nombra la
-- columna: TestTheAPIWorksWithoutUsersRole lo prueba borrándola.
--
-- El enum user_role se queda: es la dimensión «rol sugerido» de la medición de uso (usage_daily,
-- usage_touches_daily), y cambiar esas columnas de tipo reescribe las tablas bajo bloqueo exclusivo.

-- +goose Up
set local lock_timeout = '3s';

-- Lo que la API anterior pudo dejar a medias durante la subida de la 0108: una empresa creada sin sus
-- sugeridos o un usuario sin role_id. Idempotente.
select ensure_company_roles(id) from companies;
update users u set role_id = r.id
  from roles r join system_roles s on s.id = r.system_role_id
 where r.company_id = u.company_id and s.code = u.role::text and u.role_id is null;

-- not null sin barrer la tabla bajo bloqueo: check not valid, validate, y entonces set not null usa
-- el check ya validado.
alter table users add constraint users_role_id_present check (role_id is not null) not valid;
alter table users validate constraint users_role_id_present;
alter table users alter column role_id set not null;
alter table users drop constraint users_role_id_present;

drop trigger users_sync_role on users;
drop function sync_user_role();
alter table users drop column role;

-- +goose Down
-- Vuelve la columna con el código del sugerido; un rol propio queda como 'mesero' (lo mínimo), igual
-- que lo dejaba el trigger de la 0108.
alter table users add column role user_role;
update users u set role = coalesce(s.code, 'mesero')::user_role
  from roles r left join system_roles s on s.id = r.system_role_id
 where r.id = u.role_id;
alter table users alter column role set not null;
alter table users alter column role_id drop not null;
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
