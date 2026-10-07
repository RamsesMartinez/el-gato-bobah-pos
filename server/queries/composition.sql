-- name: ListProductsForStock :many
-- Lo mínimo de cada producto para saber qué descuenta al venderse (spec 028). Sin filtro de empresa:
-- lo pone RLS.
select id, type, track_stock, recipe_id from products;

-- name: InsertOrderLineComponent :exec
-- Lo que se vendió dentro de un paquete, copiado al vender: editar el paquete después no reescribe
-- el historial.
insert into order_line_components (order_line_id, product_id, quantity) values ($1, $2, $3);
