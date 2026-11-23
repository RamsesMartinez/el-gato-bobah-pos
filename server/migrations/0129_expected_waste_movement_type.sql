-- +goose Up

-- MERMA ESPERADA QUE DESCUENTA INVENTARIO (spec 096, subida 2). El tipo de movimiento nuevo va solo en
-- su migración: Postgres no deja usar un valor de enum en la misma transacción que lo agrega. Mientras
-- la empresa no encienda «la merma esperada también descuenta del almacén» (apagado por omisión),
-- ninguna versión de la API escribe este tipo.
alter type stock_movement_type add value if not exists 'merma_esperada';

-- +goose Down
-- Un valor de enum no se quita sin recrear el tipo y reescribir stock_movements, la tabla más grande
-- del inventario. Se deja: sin movimientos que lo usen no estorba.
select 1;
