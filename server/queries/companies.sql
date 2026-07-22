-- Empresas (tenants). Ver migraciones 0022/0024. companies es global con RLS de "solo mi fila".

-- name: ResolveCompanyBySlug :one
-- Login: resuelve slug→company_id ANTES de tener contexto de tenant. Usa el resolver
-- SECURITY DEFINER (salta RLS de forma acotada). 0 si el slug no existe o está inactivo
-- (coalesce: un escalar NULL rompería el scan a int64; 0 nunca es un id real).
select coalesce(app_resolve_company($1), 0)::bigint as company_id;

-- name: GetCompany :one
-- La propia empresa del tenant (RLS ya la acota a la fila del GUC).
select * from companies where id = $1;

-- name: CreateCompany :one
-- Provisioning de plataforma (corre como owner en bootstrap; el rol del app no puede insertar).
insert into companies (slug, name) values ($1, $2) returning *;

-- name: UpdateCompany :one
-- El admin de la empresa edita nombre/slug de SU empresa (RLS with-check impide tocar otra).
update companies set name = $1, slug = $2, updated_at = now() where id = $3 returning *;

-- name: CountUsersInCompany :one
select count(*) from users;
