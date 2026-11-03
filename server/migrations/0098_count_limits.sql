-- +goose Up
-- Límites del conteo por empresa (spec 044): lo más que se puede registrar de un artículo en un
-- conteo y la cifra a partir de la cual se pide confirmar, por clase de unidad y en unidad base
-- (g, ml, cm, piezas). Antes eran fijos en el código; los defaults de columna son esos mismos
-- valores (domain.DefaultCountLimits), para que una empresa que nunca los toca cuente igual y la API
-- anterior, durante la subida, coincida con la nueva.
--
-- Ocho columnas fijas y no una fila por clase: son las cuatro clases de unit_kind, y una clase nueva
-- costaría una migración igual.

set local lock_timeout = '3s';

create table count_limits (
  company_id     bigint primary key
                 default nullif(current_setting('app.company_id', true), '')::bigint
                 references companies(id) on delete cascade,
  max_mass       numeric(14,4) not null default 1000000,
  max_volume     numeric(14,4) not null default 1000000,
  max_length     numeric(14,4) not null default 1000000,
  max_piece      numeric(14,4) not null default 1000000,
  confirm_mass   numeric(14,4) not null default 50000,
  confirm_volume numeric(14,4) not null default 50000,
  confirm_length numeric(14,4) not null default 5000,
  confirm_piece  numeric(14,4) not null default 5000,
  updated_at     timestamptz not null default now(),
  -- Mayores que cero y bajo el techo de almacenamiento (domain.MaxCountLimit).
  constraint count_limits_positive check (
    max_mass > 0 and max_volume > 0 and max_length > 0 and max_piece > 0 and
    confirm_mass > 0 and confirm_volume > 0 and confirm_length > 0 and confirm_piece > 0),
  constraint count_limits_ceiling check (
    max_mass <= 1000000000 and max_volume <= 1000000000 and max_length <= 1000000000 and max_piece <= 1000000000 and
    confirm_mass <= 1000000000 and confirm_volume <= 1000000000 and confirm_length <= 1000000000 and confirm_piece <= 1000000000),
  -- Un máximo que no pasa la cifra para confirmar rechazaría antes de que el aviso pudiera saltar.
  constraint count_limits_max_over_confirm check (
    max_mass > confirm_mass and max_volume > confirm_volume and
    max_length > confirm_length and max_piece > confirm_piece)
);

alter table count_limits enable row level security;
create policy tenant_isolation on count_limits
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update on count_limits to gatobobah_app;

-- Cada empresa que ya existe, con los defaults. Las nuevas las siembra CreateCompany.
insert into count_limits (company_id) select id from companies on conflict do nothing;

-- +goose Down
-- Pierde los límites que cada empresa haya personalizado: es configuración, no un hecho, y la API
-- anterior vuelve a sus valores fijos, que son los defaults.
set local lock_timeout = '3s';
drop table count_limits;
