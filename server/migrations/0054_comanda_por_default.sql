-- +goose Up
-- La comanda de cocina pasa a nacer ENCENDIDA en las empresas nuevas.
--
-- Nació apagada cuando la comanda era una función que se estaba estrenando. Ahora es parte del flujo
-- —confirmar el pedido ES mandarlo a cocina—, así que un negocio que se da de alta y no encuentra el
-- ajuste tiene una cocina que no recibe papel y no sabe por qué.
--
-- Cambiar el DEFAULT no toca NINGUNA fila existente, y eso es lo que se quiere: las empresas en
-- operación conservan lo que tengan configurado. Un despliegue que le enciende una impresora a un
-- negocio sin que nadie la pida es un defecto, no una mejora.
alter table business_settings alter column print_kitchen_ticket set default true;

-- +goose Down
alter table business_settings alter column print_kitchen_ticket set default false;
