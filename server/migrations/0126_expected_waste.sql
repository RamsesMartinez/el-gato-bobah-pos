-- +goose Up
-- Merma esperada (spec 096): un % general de la empresa, uno opcional por producto y uno por
-- artículo que distingue «sin captura» (nulo) de «capturado en 0». La precedencia vive en
-- domain.ResolveExpectedWaste.
--
-- Solo agrega (constitución IV): la versión anterior de la API sigue leyendo ingredients.waste_pct
-- durante la subida, así que esa columna se queda y se borra en la subida 2 de esta spec. Con el
-- general y el del producto vacíos y el artículo con el mismo % que tenía, ningún costo cambia.

set local lock_timeout = '3s';

alter table business_settings add column expected_waste_pct numeric(5,2)
  check (expected_waste_pct >= 0 and expected_waste_pct < 100);
alter table products add column expected_waste_pct numeric(5,2)
  check (expected_waste_pct >= 0 and expected_waste_pct < 100);
alter table ingredients add column expected_waste_pct numeric(5,2)
  check (expected_waste_pct >= 0 and expected_waste_pct < 100);

-- Lo que hoy está en 0 nadie lo capturó (lo dejó así la carga de FUDO): queda sin captura.
update ingredients set expected_waste_pct = waste_pct where waste_pct > 0;

-- Espejo hacia la columna vieja mientras exista: si se regresa a la imagen anterior sin correr el
-- Down (azul/verde revierte solo la imagen), esa versión recalcula con waste_pct y, sin el espejo,
-- deshacía en silencio cada merma capturada desde esta subida. Se va con la columna en la subida 2.
-- +goose StatementBegin
create function mirror_expected_waste_pct() returns trigger
language plpgsql as $$
begin
  if tg_op = 'INSERT' then
    if new.expected_waste_pct is not null then
      new.waste_pct := new.expected_waste_pct;
    end if;
  else
    new.waste_pct := coalesce(new.expected_waste_pct, 0);
  end if;
  return new;
end;
$$;
-- +goose StatementEnd
create trigger ingredients_mirror_expected_waste_insert before insert on ingredients
  for each row execute function mirror_expected_waste_pct();
create trigger ingredients_mirror_expected_waste_update before update of expected_waste_pct on ingredients
  for each row execute function mirror_expected_waste_pct();

-- +goose Down
set local lock_timeout = '3s';

drop trigger if exists ingredients_mirror_expected_waste_update on ingredients;
drop trigger if exists ingredients_mirror_expected_waste_insert on ingredients;
drop function if exists mirror_expected_waste_pct();

-- La versión anterior lee waste_pct: se le regresa lo capturado después de subir, o su primer
-- recálculo cambiaría costos con el % viejo. El % del producto y el general se pierden; tras bajar,
-- la API anterior recalcula los costos al arrancar.
update ingredients set waste_pct = coalesce(expected_waste_pct, 0)
 where waste_pct is distinct from coalesce(expected_waste_pct, 0);

alter table ingredients drop column expected_waste_pct;
alter table products drop column expected_waste_pct;
alter table business_settings drop column expected_waste_pct;
