-- +goose Up
-- Interruptor de la comanda de cocina: un papel SIN precios que sale al mandar el pedido, con el
-- folio grande y las líneas con sus modificadores.
--
-- Nace APAGADO, y no es la elección tímida por default: en el local que estrena el sistema la
-- cocina está pegada al mostrador y el cocinero ve la misma pantalla, así que la comanda sería
-- papel que duplica lo que ya tiene enfrente. Sirve al negocio que tiene la cocina en otro cuarto,
-- y ese lo enciende a propósito.
--
-- Va como columna de business_settings y no como una constante porque es una decisión de cada
-- negocio: el mismo binario sirve a los dos. La intención declarada del dueño es que a medio plazo
-- este tipo de ajustes se vendan como paquetes; por eso vive junto a los demás ajustes del ticket y
-- no cableado en el código de impresión, que es lo que después habría que desenredar.
alter table business_settings
  add column print_kitchen_ticket boolean not null default false;

-- +goose Down
alter table business_settings drop column print_kitchen_ticket;
