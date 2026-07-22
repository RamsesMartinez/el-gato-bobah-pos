-- +goose Up
-- Ajustes de negocio en una sola fila. Arranca con el costo de envío configurable; se agregan
-- columnas cuando exista otro ajuste real (YAGNI: no un key/value genérico para un valor).
-- El truco id boolean+check(id) fuerza como máximo una fila (solo 'true' cabe en el PK).
create table business_settings (
  id           boolean primary key default true check (id),
  delivery_fee numeric(10,2) not null default 20 check (delivery_fee >= 0),
  updated_at   timestamptz not null default now(),
  updated_by   bigint references users(id)
);
insert into business_settings (id) values (true); -- fila única con defaults ($20)

-- Snapshot del costo de envío cobrado en cada orden a domicilio. Aparte del subtotal para que
-- reportes y reembolsos lo vean; total = subtotal + delivery_fee. Default 0 (no-domicilio).
alter table orders add column delivery_fee numeric(10,2) not null default 0 check (delivery_fee >= 0);

-- +goose Down
alter table orders drop column delivery_fee;
drop table business_settings;
