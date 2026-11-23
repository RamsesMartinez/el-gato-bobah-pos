-- +goose Up

-- TRASPASOS ENTRE ALMACENES (spec 094, tercera subida). El tipo de movimiento nuevo va solo en su
-- migración: Postgres no deja usar un valor de enum en la misma transacción que lo agrega, y la tabla
-- de traspasos y sus checks lo usan (R11 H3). Mientras ninguna versión de la API escribe traspasos,
-- el valor no aparece en ningún movimiento.
alter type stock_movement_type add value if not exists 'traspaso';

-- +goose Down
-- Un valor de enum no se quita sin recrear el tipo y reescribir la tabla más grande del inventario.
-- Se deja: sin movimientos que lo usen no estorba, y la migración siguiente (tablas de traspaso) se
-- niega a bajar si hay traspasos.
select 1;
