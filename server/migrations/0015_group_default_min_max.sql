-- +goose Up
-- Default min/max de selección por grupo (heredable). En el enlace por-producto, NULL = hereda
-- el default del grupo; un valor = override. Las filas existentes conservan su valor (override explícito).
alter table modifier_groups
  add column default_min_select smallint not null default 0,
  add column default_max_select smallint not null default 1
    check (default_max_select >= default_min_select);

alter table product_modifier_groups
  alter column min_select drop not null,
  alter column max_select drop not null,
  alter column min_select drop default,
  alter column max_select drop default,
  -- override es todo-o-nada: ambos NULL (hereda) o ambos con valor.
  add constraint pmg_override_both check ((min_select is null) = (max_select is null));

-- +goose Down
alter table product_modifier_groups
  drop constraint pmg_override_both;
update product_modifier_groups
  set min_select = coalesce(min_select, 0), max_select = coalesce(max_select, 1);
alter table product_modifier_groups
  alter column min_select set default 0,
  alter column max_select set default 1,
  alter column min_select set not null,
  alter column max_select set not null;
alter table modifier_groups
  drop column default_min_select,
  drop column default_max_select;
