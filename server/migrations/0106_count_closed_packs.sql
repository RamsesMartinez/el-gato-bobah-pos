-- Cuántos empaques cerrados de cada uno se contaron en una pesada con empaque (#204). Sin esto, al
-- reabrir la captura solo se sabía el total de lo cerrado, y con dos tamaños de empaque («1 bolsa de 15
-- kg y 1 de 5 kg») no hay forma de saber cómo se repartió: la pantalla lo adivinaba mal. Es un hecho de
-- la captura que no se puede reconstruir después (constitución VIII), así que se guarda.
--
-- Arreglo de {presentationId, qty}. Nulo en todo lo anterior y en lo que capture la API anterior
-- durante la subida; quien lo lee comprueba que sume lo cerrado antes de usarlo, porque un recapturar
-- de la API anterior no lo limpia.

-- +goose Up
alter table stock_count_lines
  add column closed_packs jsonb,
  add constraint stock_count_lines_closed_packs_shape check (
    closed_packs is null or (jsonb_typeof(closed_packs) = 'array' and gross_g is not null));

-- +goose Down
alter table stock_count_lines drop constraint stock_count_lines_closed_packs_shape, drop column closed_packs;
