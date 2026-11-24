-- +goose Up
-- La merma esperada también descuenta del almacén (spec 096, subida 2): una opción de la empresa,
-- apagada por omisión, que la venta lee en su misma transacción. Y se quita la columna vieja de la
-- merma del artículo: desde la subida 1 ninguna versión en producción lee ingredients.waste_pct
-- (constitución IV: primero se agrega, después se quita). Con ella se va el espejo de la 0126, que
-- solo existía para un regreso a la versión anterior a la subida 1.

set local lock_timeout = '3s';

alter table business_settings add column expected_waste_deducts boolean not null default false;

drop trigger if exists ingredients_mirror_expected_waste_update on ingredients;
drop trigger if exists ingredients_mirror_expected_waste_insert on ingredients;
drop function if exists mirror_expected_waste_pct();
alter table ingredients drop column waste_pct;

-- +goose Down
set local lock_timeout = '3s';

-- La columna vuelve con lo capturado (una versión anterior a la subida 2 no la lee, pero la de antes de
-- la 0126 sí) y la opción se pierde: el Down la deja apagada, que es como nació.
alter table ingredients add column waste_pct numeric(5,2) not null default 0
  check (waste_pct >= 0 and waste_pct < 100);
update ingredients set waste_pct = coalesce(expected_waste_pct, 0) where expected_waste_pct is not null;
alter table business_settings drop column expected_waste_deducts;
