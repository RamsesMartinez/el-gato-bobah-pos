-- Cuántos empaques cerrados de cada uno se contaron en una pesada con empaque (#204). Sin esto, al
-- reabrir la captura solo se sabía el total de lo cerrado, y con dos tamaños de empaque («1 bolsa de 15
-- kg y 1 de 5 kg») no hay forma de saber cómo se repartió: la pantalla lo adivinaba mal. Es un hecho de
-- la captura que no se puede reconstruir después (constitución VIII), así que se guarda.
--
-- Arreglo de {presentationId, qty}. Nulo en todo lo anterior y en lo que capture la API anterior
-- durante la subida. La API anterior recaptura con un upsert que no conoce esta columna: sin pesar,
-- pone la pesada en nulo y deja el reparto; el trigger lo suelta junto con la pesada (con un check que
-- lo exigiera, esa captura respondía 500). Recapturando con pesada lo deja como estaba, así que quien
-- lo lee comprueba que sume lo cerrado antes de usarlo.

-- +goose Up
alter table stock_count_lines
  add column closed_packs jsonb,
  add constraint stock_count_lines_closed_packs_shape check (closed_packs is null or jsonb_typeof(closed_packs) = 'array');

-- +goose StatementBegin
create function drop_closed_packs_without_gross() returns trigger
language plpgsql as $$
begin
  if new.gross_g is null then
    new.closed_packs := null;
  end if;
  return new;
end $$;
-- +goose StatementEnd

create trigger stock_count_lines_closed_packs_need_gross
  before insert or update on stock_count_lines
  for each row execute function drop_closed_packs_without_gross();

-- +goose Down
-- Se pierde el reparto de los cerrados: la captura sigue con su total y su pesada, y la pantalla
-- anterior no lo usa. Por eso no se niega.
drop trigger stock_count_lines_closed_packs_need_gross on stock_count_lines;
drop function drop_closed_packs_without_gross();
alter table stock_count_lines drop constraint stock_count_lines_closed_packs_shape, drop column closed_packs;
