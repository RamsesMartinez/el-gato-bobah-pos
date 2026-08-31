-- +goose Up
-- Índice de soporte para la pantalla de Ventas (análisis por período).
--
-- lock_timeout como en 0040 y 0041, y por el mismo motivo: `create index` toma un ACCESS EXCLUSIVE
-- sobre `orders`, la tabla que toca cada venta. Con ~70 filas construirlo tarda microsegundos, así
-- que el riesgo no es el tamaño sino la COLA: si al momento del deploy hay una transacción larga
-- abierta, el ALTER se encola detrás de ella y arrastra toda lectura nueva. Es preferible que la
-- migración falle limpio en 3 segundos a que el POS se quede mudo esperando.
set local lock_timeout = '3s';

-- La columna líder es `company_id` y no la fecha porque RLS agrega `company_id = <tenant>` como
-- predicado a TODA consulta del rol `gatobobah_app`. Con la fecha al frente, Postgres entra por
-- (business_date, status) y descarta filas de OTRAS empresas ya dentro del scan, en vez de no
-- tocarlas nunca.
--
-- El orden de las otras dos no es intercambiable: `business_date` es un RANGO y siempre viene;
-- `status` es una igualdad y es opcional. Con el estado antes de la fecha, una consulta sin filtro
-- de estado perdería la fecha como condición de índice y la aplicaría como filtro sobre todo el
-- histórico de la empresa.
create index orders_company_date_status on orders (company_id, business_date, status);

-- `orders_company (company_id)` queda estrictamente contenido en el compuesto de arriba: es su
-- prefijo exacto, así que cualquier plan que lo usara puede usar el nuevo igual de bien. No es una
-- hipótesis que haya que medir con tráfico —como sí lo es `orders_date_status`, que empieza por
-- otra columna y se conserva—, es aritmética de prefijos. Mantener un índice de más sobre la tabla
-- que crece con cada venta cuesta escritura por cero beneficio.
drop index orders_company;

-- +goose Down
create index orders_company on orders (company_id);
drop index if exists orders_company_date_status;
