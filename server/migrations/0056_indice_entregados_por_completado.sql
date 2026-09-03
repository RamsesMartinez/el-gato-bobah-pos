-- +goose NO TRANSACTION
-- +goose Up
-- El índice que sostiene la lista de entregados desde el corte.
--
-- Esa lista pasó de filtrar por `business_date` —que sí tenía índice— a filtrar por `completed_at`,
-- que no lo tenía: se volvió un scan de toda la tabla. Medido con 60 mil pedidos: 7.9 ms y 1,098
-- buffers, contra 0.48 ms y 100 con este índice. Hoy no duele, pero la tabla solo crece y la
-- consulta la pide el tablero cada vez que alguien lo abre.
--
-- Empieza por company_id porque RLS agrega ese predicado a toda consulta del rol de aplicación: un
-- índice que arrancara por la fecha se quedaría descartando filas de otras empresas dentro del scan.
--
-- CONCURRENTLY: `orders` es una tabla viva —cada venta, cada pago, cada entrega escribe en ella— y
-- un create index normal toma ACCESS EXCLUSIVE mientras construye. El patrón de lock_timeout de las
-- migraciones 0040-0042 asumía una tabla de decenas de filas; ésta ya va en decenas de miles.
create index concurrently if not exists orders_company_status_completed
  on orders (company_id, status, completed_at desc);

-- +goose Down
drop index concurrently if exists orders_company_status_completed;
