-- +goose Up
-- Ventana de disponibilidad (temporada): un producto solo aparece en el POS entre
-- estas fechas. NULL = sin límite por ese lado (siempre disponible).
alter table products
  add column available_from  date,
  add column available_until date;

-- +goose Down
alter table products
  drop column available_from,
  drop column available_until;
