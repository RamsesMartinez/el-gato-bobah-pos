-- +goose Up
-- Gestión de artículos del almacén (spec 042). Solo agrega: dos tablas, una columna nullable en los
-- movimientos y dos índices. La API anterior no las lee ni las escribe, así que sigue funcionando
-- mientras la nueva arranca a su lado (constitución IV, tolerancia a la versión anterior).

set local lock_timeout = '3s';

-- Correcciones de costo a mano. Junto con los movimientos de compra forman el historial de costos de
-- un artículo; no se copia aquí lo que ya guardan las compras.
create table ingredient_cost_corrections (
  id            bigint generated always as identity primary key,
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                references companies(id) on delete cascade,
  ingredient_id bigint not null,
  previous_cost numeric(12,6) not null check (previous_cost >= 0),
  new_cost      numeric(12,6) not null check (new_cost >= 0),
  user_id       bigint not null,
  created_at    timestamptz not null default now(),
  constraint ingredient_cost_corrections_id_company_key unique (id, company_id),
  -- Compuestas: una llave foránea salta RLS, y sin company_id una corrección podría apuntar al
  -- artículo de otra empresa. no action y no restrict: al borrar una empresa todo cae en la misma
  -- sentencia, y restrict truena según el orden en que llegue la cascada.
  constraint ingredient_cost_corrections_ingredient_fkey
    foreign key (ingredient_id, company_id) references ingredients (id, company_id),
  constraint ingredient_cost_corrections_user_fkey
    foreign key (company_id, user_id) references users (company_id, id)
);
create index ingredient_cost_corrections_by_ingredient
  on ingredient_cost_corrections (company_id, ingredient_id, created_at desc, id desc);

-- Un artículo reemplazado por otro en otra unidad (el que ya tenía compras o ventas). Un artículo se
-- reemplaza una sola vez: después queda archivado.
create table ingredient_replacements (
  id                bigint generated always as identity primary key,
  company_id        bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                    references companies(id) on delete cascade,
  old_ingredient_id bigint not null,
  new_ingredient_id bigint not null,
  -- Cuánto del nuevo vale una unidad base del viejo («1 pieza = 5,000 g» → 5000).
  factor            numeric(14,6) not null check (factor > 0),
  user_id           bigint not null,
  created_at        timestamptz not null default now(),
  constraint ingredient_replacements_id_company_key unique (id, company_id),
  constraint ingredient_replacements_old_key unique (company_id, old_ingredient_id),
  constraint ingredient_replacements_not_itself check (old_ingredient_id <> new_ingredient_id),
  constraint ingredient_replacements_old_fkey
    foreign key (old_ingredient_id, company_id) references ingredients (id, company_id),
  constraint ingredient_replacements_new_fkey
    foreign key (new_ingredient_id, company_id) references ingredients (id, company_id),
  constraint ingredient_replacements_user_fkey
    foreign key (company_id, user_id) references users (company_id, id)
);
create index ingredient_replacements_by_new on ingredient_replacements (company_id, new_ingredient_id);

alter table ingredient_cost_corrections enable row level security;
create policy tenant_isolation on ingredient_cost_corrections
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
alter table ingredient_replacements enable row level security;
create policy tenant_isolation on ingredient_replacements
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Hechos: se registran y no se borran. Las correcciones admiten update por una sola razón: el cambio
-- directo de unidad (decisión del dueño, 2026-10-19) las pasa a la unidad nueva, con los originales en
-- la bitácora. Los reemplazos no se editan.
grant select, insert, update on ingredient_cost_corrections to gatobobah_app;
grant select, insert on ingredient_replacements to gatobobah_app;

-- Los dos ajustes del traspaso de un reemplazo (salida en el viejo, entrada en el nuevo, por sucursal)
-- se ligan por columna y no por una nota de texto: reconstruir o revertir uno no exige parsear texto.
alter table stock_movements add column replacement_id bigint;
alter table stock_movements add constraint stock_movements_replacement_fkey
  foreign key (replacement_id, company_id) references ingredient_replacements (id, company_id);
create index stock_movements_by_replacement on stock_movements (replacement_id) where replacement_id is not null;

-- El historial de costos lee las compras de un artículo; RLS agrega company_id a toda consulta de la
-- app, y un índice que no empieza por ahí descarta filas de la otra empresa dentro del scan.
create index stock_movements_purchases_by_ingredient
  on stock_movements (company_id, ingredient_id, created_at desc, id desc)
  where movement_type = 'compra';

-- La bitácora de un artículo filtra por entidad; los índices de 0089 son por fecha, pedido, actor y
-- aparato, y recorrerían la bitácora entera de la empresa.
create index audit_events_by_entity
  on audit_events (company_id, entity_type, entity_id, occurred_at desc, id desc);

-- +goose Down
-- Se niega con datos, como 0089, 0092 y 0094: una corrección de costo o un reemplazo son hechos que
-- solo viven aquí. Tras registrarlos, el rollback es restaurar el respaldo.
set local lock_timeout = '3s';
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from ingredient_cost_corrections) or exists (select 1 from ingredient_replacements) then
    raise exception '0096: hay correcciones de costo o reemplazos registrados; restaura el respaldo en vez de bajar';
  end if;
end $$;
-- +goose StatementEnd
drop index audit_events_by_entity;
drop index stock_movements_purchases_by_ingredient;
drop index stock_movements_by_replacement;
alter table stock_movements drop constraint stock_movements_replacement_fkey;
alter table stock_movements drop column replacement_id;
drop table ingredient_replacements;
drop table ingredient_cost_corrections;
