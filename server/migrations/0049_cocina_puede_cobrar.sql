-- +goose Up
-- Si el tablero de Pedidos puede cobrar, además de preparar y entregar.
--
-- Nace APAGADO, y es la elección correcta para el diseño y no la tímida: cobrar es del punto de
-- venta, y una pantalla de cocina con botón de cobrar le da acceso al dinero a quien solo tiene
-- que preparar comida. Con esto apagado, /pedidos hace una sola cosa.
--
-- Existe encendido porque en el local que estrena el sistema la cocina está pegada al mostrador y
-- es la misma persona en la misma máquina: ahí, obligarla a cambiar de pantalla para cobrar un
-- pedido que tiene enfrente son taps que no compran nada. El día que haya un dispositivo de cocina
-- de verdad, o un rol de cocinero, se apaga y la separación queda hecha sin tocar código.
--
-- Va como columna de business_settings y no como constante porque es una decisión de cada negocio:
-- el mismo binario sirve a los dos, que es lo que este sistema necesita para escalar a N locales.
alter table business_settings
  add column kitchen_can_charge boolean not null default false;

-- +goose Down
alter table business_settings drop column kitchen_can_charge;
