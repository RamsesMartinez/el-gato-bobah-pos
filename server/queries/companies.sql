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
--
-- La matriz nace en la MISMA consulta: no puede existir un momento con empresa y sin sucursal
-- (FR-002). No es un trigger sobre `companies` porque `pg_restore` carga con COPY, que dispara
-- triggers, y duplicaría la matriz que el respaldo ya trae (0076).
with company as (
  insert into companies (slug, name) values ($1, $2) returning *
), headquarters as (
  insert into branches (company_id, branch_number, code, name, is_headquarters)
  select id, 1, headquarters_code(slug), left(name, 60), true from company
  returning company_id, id
), terminal as (
  -- Un negocio nace con una terminal por omisión (spec 032, punto 8).
  insert into card_terminals (company_id, branch_id, name)
  select company_id, id, 'Terminal' from headquarters
), concepts as (
  -- Y con los conceptos de salida frecuentes (punto 3).
  insert into cash_concepts (company_id, name)
  select c.id, n.name from company c cross join (values ('Basura'), ('Hielo'), ('Vigilancia'), ('Insumos')) as n(name)
), waste_reasons_seed as (
  -- Y con los motivos sugeridos de lo que se echa a perder (spec 039), los mismos que la 0094
  -- sembró a las empresas que ya existían. La empresa los renombra o archiva.
  insert into waste_reasons (company_id, name, description, is_staff_meal)
  select c.id, r.name, r.description, r.is_staff_meal from company c cross join (values
    ('Caducó',               'Pasó su fecha o se echó a perder guardado', false),
    ('Se cayó',              'Un accidente al servir o al moverlo',         false),
    ('Mal preparado',        'Salió mal y no se pudo vender',               false),
    ('Prueba de receta',     'Lo que se usó para probar una receta',        false),
    ('Consumo del personal', 'Lo que se toma o se come el equipo',          true)
  ) as r(name, description, is_staff_meal)
), count_limits_seed as (
  -- Y con sus límites del conteo (spec 044), con los defaults de la columna, como la 0098.
  insert into count_limits (company_id) select id from company
)
select * from company;

-- name: UpdateCompany :one
-- El admin de la empresa edita nombre/slug de SU empresa (RLS with-check impide tocar otra).
update companies set name = $1, slug = $2, updated_at = now() where id = $3 returning *;

-- name: CountUsersInCompany :one
select count(*) from users;
