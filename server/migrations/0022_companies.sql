-- +goose Up
-- Multi-tenant: cada empresa es un tenant con un slug único y editable por su admin. El slug
-- forma el identificador de login username@slug y es la llave que particiona TODOS los datos.
-- El aislamiento real lo hace RLS (ver 0024); este archivo introduce la identidad de tenant.
create table companies (
  id         bigint generated always as identity primary key,
  slug       citext not null unique,   -- parte derecha de username@slug; editable por el admin de la empresa
  name       text not null,
  is_active  boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- Empresa por defecto: todos los datos que ya existían pertenecen a ella (backfill en 0023/0024).
-- Es el único tenant hoy; el admin puede renombrar el slug desde el panel.
insert into companies (slug, name) values ('gatobobah', 'El Gato Bobah');

-- users pasa a ser multi-tenant. company_id se agrega nullable → backfill → not null → default,
-- para no evaluar current_setting durante el reescaneo de la columna (fallaría sin GUC).
alter table users add column company_id bigint references companies(id) on delete cascade;
update users set company_id = (select id from companies where slug = 'gatobobah');
alter table users alter column company_id set not null;
-- default en runtime: el INSERT del app auto-sella el tenant desde el GUC de sesión (ver store.WithTenant).
-- missing_ok=true → NULL cuando no hay GUC (migraciones/bootstrap), esos caminos pasan company_id explícito.
alter table users alter column company_id set default current_setting('app.company_id', true)::bigint;

-- username es único DENTRO de la empresa, no global: dos empresas pueden tener 'admin'.
-- Llave compuesta indexada (company_id, username) → login resuelve slug→company y luego username.
alter table users drop constraint users_username_key;
alter table users add constraint users_company_username_key unique (company_id, username);

-- Email externo REAL para recuperación de contraseña (distinto del handle interno username@slug).
-- Opcional: sin él, no se puede autoservir el reset y el admin lo resetea a mano.
alter table users add column recovery_email citext;
-- Fuerza cambio de contraseña en el próximo login (tras alta/reset por admin).
alter table users add column must_change_password boolean not null default false;

-- +goose Down
alter table users drop column must_change_password;
alter table users drop column recovery_email;
alter table users drop constraint users_company_username_key;
alter table users add constraint users_username_key unique (username);
alter table users drop column company_id;
drop table companies;
