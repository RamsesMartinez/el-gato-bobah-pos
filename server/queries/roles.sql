-- Roles por empresa (spec 068). RLS pone la empresa; ninguna consulta nombra users.company_id (sqlc
-- no la conoce: la agregó la 0023 con DDL dinámico).

-- name: LockCompanyRoles :exec
-- Candado de roles de la empresa (28068): toda escritura de un rol, de sus ajustes, del rol de un
-- empleado o de si está activo lo toma ANTES de leer. Sin él, inactivar un rol y asignárselo a
-- alguien a la vez, o dar de baja a los dos últimos que administran roles, pasan los dos.
select pg_advisory_xact_lock(28068, coalesce(nullif(current_setting('app.company_id', true), '')::int, 0));

-- name: UserEffectivePermissions :many
-- Lo que puede un usuario ahora: lo del sugerido menos lo quitado, más lo agregado; nada si el
-- usuario o su rol no están activos.
select p.code::text as code
  from users u
  join roles r on r.id = u.role_id and r.active
  join permissions p on (
        (exists (select 1 from system_role_permissions sp
                  where sp.system_role_id = r.system_role_id and sp.permission_id = p.id)
         and not exists (select 1 from role_permission_overrides o
                          where o.company_id = r.company_id and o.role_id = r.id
                            and o.permission_id = p.id and not o.granted))
     or exists (select 1 from role_permission_overrides o
                 where o.company_id = r.company_id and o.role_id = r.id
                   and o.permission_id = p.id and o.granted))
 where u.id = $1 and u.is_active
 order by p.id;

-- name: UserRole :one
-- El rol de un usuario para la sesión: id, nombre que se muestra y código del sugerido ("" si es
-- propio).
select r.id, coalesce(r.name, s.name)::text as name, coalesce(s.code, '')::text as system_code
  from users u join roles r on r.id = u.role_id
  left join system_roles s on s.id = r.system_role_id
 where u.id = $1;

-- name: ListPermissionCatalog :many
select id, code, area from permissions order by id;

-- name: ListRoles :many
select r.id, r.company_id, r.system_role_id, coalesce(r.name, s.name)::text as name,
       coalesce(s.code, '')::text as system_code, r.active, r.version,
       (select count(*) from users u where u.role_id = r.id and u.is_active) as active_users,
       (select count(*) from users u where u.role_id = r.id) as all_users
  from roles r left join system_roles s on s.id = r.system_role_id
 order by r.system_role_id nulls last, lower(r.name);

-- name: ListRoleOverrides :many
select o.company_id, o.role_id, p.code::text as code, o.granted
  from role_permission_overrides o join permissions p on p.id = o.permission_id
 order by o.role_id, p.id;

-- name: ListSystemRolePermissions :many
select sp.system_role_id, p.code::text as code
  from system_role_permissions sp join permissions p on p.id = sp.permission_id
 order by sp.system_role_id, p.id;

-- name: GetRoleForUpdate :one
select r.id, r.system_role_id, coalesce(s.code, '')::text as system_code,
       coalesce(r.name, s.name)::text as name, r.active, r.version
  from roles r left join system_roles s on s.id = r.system_role_id
 where r.id = $1
   for update of r;

-- name: RoleNamesExcept :many
-- Los nombres con los que no puede chocar un rol propio: los demás de la empresa y los sugeridos.
select coalesce(r.name, s.name)::text as name
  from roles r left join system_roles s on s.id = r.system_role_id
 where r.id <> $1;

-- name: BumpRoleVersion :execrows
-- Cada cambio de un rol sube su versión, y solo si la pantalla vio la vigente.
update roles set version = version + 1, updated_at = now() where id = $1 and version = $2;

-- name: InsertRole :one
insert into roles (name) values ($1) returning id;

-- name: RenameRole :exec
update roles set name = $2 where id = $1;

-- name: SetRoleActive :exec
update roles set active = $2 where id = $1;

-- name: DeleteRole :exec
delete from roles where id = $1;

-- name: DeleteRoleOverrides :exec
delete from role_permission_overrides where role_id = $1;

-- name: InsertRoleOverride :exec
insert into role_permission_overrides (role_id, permission_id, granted)
select $1, p.id, $3 from permissions p where p.code = $2;

-- name: CountActiveRoleAdmins :one
-- Cuántas personas activas pueden administrar roles. Se cuenta DESPUÉS de escribir, dentro de la
-- transacción y bajo el candado: si da cero, el cambio se revierte (ErrLastRoleAdmin).
select count(*)
  from users u join roles r on r.id = u.role_id and r.active
 where u.is_active
   and ((exists (select 1 from system_role_permissions sp join permissions p on p.id = sp.permission_id
                  where sp.system_role_id = r.system_role_id and p.code = 'roles.manage')
         and not exists (select 1 from role_permission_overrides o join permissions p on p.id = o.permission_id
                          where o.company_id = r.company_id and o.role_id = r.id and p.code = 'roles.manage' and not o.granted))
     or exists (select 1 from role_permission_overrides o join permissions p on p.id = o.permission_id
                 where o.company_id = r.company_id and o.role_id = r.id and p.code = 'roles.manage' and o.granted));

-- name: SuggestedRoleID :one
-- El id del sugerido `code` en la empresa de la sesión.
select r.id from roles r join system_roles s on s.id = r.system_role_id where s.code = $1;

-- name: EnsureCompanyRoles :exec
select ensure_company_roles($1);

-- name: CountActiveUsersWithRole :one
select count(*) from users where role_id = $1 and is_active;

-- name: CountUsersWithRole :one
select count(*) from users where role_id = $1;
