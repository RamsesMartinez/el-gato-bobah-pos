-- +goose Up
-- Interruptor de si el ticket imprime los adicionales que NO cuestan ("leche entera", "sin
-- cebolla"). Nace encendido: cocina y el cliente necesitan ver qué se pidió, y la ausencia de cifra
-- junto a los que sí cuestan es lo que hace legible el desglose.
--
-- Existe porque el ticket pasa a mostrar el precio de cada adicional: sin apagarlo, un pedido con
-- muchos extras gratis alarga el papel sin aportar. Es decisión del negocio, no del sistema.
alter table business_settings
  add column print_free_modifiers boolean not null default true;

-- +goose Down
alter table business_settings drop column print_free_modifiers;
