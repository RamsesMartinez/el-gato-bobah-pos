-- +goose Up
-- Si el producto necesita prepararse. Es lo que decide si un pedido viaja por el tablero.
--
-- Nace en TRUE para los 1004 productos existentes: el día que esto entra, nada cambia de
-- comportamiento. El negocio va apagando los que no lo necesitan —el refresco de la nevera, el
-- dulce empaquetado— y a partir de ahí un pedido que solo lleva de esos nace entregado.
--
-- Vive en el PRODUCTO y no en el pedido, que es donde estuvo un rato: preguntárselo al operador en
-- cada cobro es pedirle que decida algo que el catálogo ya sabe, y equivocarse tenía consecuencia
-- cara — un ticket con un refresco y unas alitas marcado como "no pasa por cocina" escondía las
-- alitas del tablero y nadie las preparaba.
alter table products
  add column needs_prep boolean not null default true;

-- +goose Down
alter table products drop column needs_prep;
