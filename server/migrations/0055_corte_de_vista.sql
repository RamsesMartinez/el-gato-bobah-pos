-- +goose Up
-- Hasta cuándo se sigue viendo en pantalla un pedido ya entregado.
--
-- Decide SOLO QUÉ SE MUESTRA. El día al que pertenece una venta lo sigue decidiendo el turno y lo
-- guarda `orders.business_date`; esta columna no lo toca ni puede moverlo de arqueo. Es importante
-- que quede dicho: un ajuste que suena a "cuándo cierra el día" invita a creer que mueve dinero.
--
-- El default es `medianoche` porque es lo que un operador espera sin que nadie se lo explique, y el
-- único de los tres que no depende de que alguien se acuerde de cerrar la caja.
--
-- Texto con check y no un enum de Postgres: agregar un valor a un enum es una migración que no se
-- puede revertir dentro de una transacción, y aquí son tres valores que probablemente no crezcan.
alter table business_settings
  add column corte_de_vista text not null default 'medianoche'
  constraint business_settings_corte_de_vista_valido
    check (corte_de_vista in ('medianoche', 'turno', 'cierre_de_caja'));

-- +goose Down
alter table business_settings drop column corte_de_vista;
