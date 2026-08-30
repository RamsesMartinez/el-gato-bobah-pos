-- Precios por plataforma digital (spec 002). Todo bajo RLS: las queries no filtran por company_id
-- porque la política tenant_isolation ya lo hace, igual que el resto del repo.

-- name: GetPlatformByID :one
-- Resuelve la plataforma de un pedido DENTRO de la empresa. Va por aquí y no por la llave foránea
-- de orders: los chequeos de FK de Postgres saltan RLS, así que un id de otra empresa pasaría el
-- insert y el pedido se cobraría a precio de mostrador con el ticket bien impreso.
select id, name, price_markup_pct, is_active from delivery_platforms where id = $1;

-- name: ListPlatformsWithMarkup :many
-- Las plataformas que el POS ofrece como lista de precios. 'Propio' queda fuera: es reparto del
-- propio negocio, sin comisión que absorber ni depósito que conciliar, y no tiene método de pago.
select id, name, price_markup_pct from delivery_platforms
where is_active and name <> 'Propio' order by name;

-- name: GetProductPlatformPrices :many
-- Solo las EXCEPCIONES de una plataforma. Un producto ausente usa el precio calculado.
select product_id, price from product_platform_prices where platform_id = $1;

-- name: GetOptionPlatformPrices :many
select option_id, price_delta from modifier_option_platform_prices where platform_id = $1;

-- name: UpsertProductPlatformPrice :exec
insert into product_platform_prices (product_id, platform_id, price, updated_by)
values ($1, $2, $3, $4)
on conflict (product_id, platform_id)
do update set price = excluded.price, updated_by = excluded.updated_by;

-- name: DeleteProductPlatformPrice :exec
delete from product_platform_prices where product_id = $1 and platform_id = $2;

-- name: UpsertOptionPlatformPrice :exec
insert into modifier_option_platform_prices (option_id, platform_id, price_delta, updated_by)
values ($1, $2, $3, $4)
on conflict (option_id, platform_id)
do update set price_delta = excluded.price_delta, updated_by = excluded.updated_by;

-- name: DeleteOptionPlatformPrice :exec
delete from modifier_option_platform_prices where option_id = $1 and platform_id = $2;

-- name: CountProductPlatformPrices :one
-- Para los tests de aislamiento: cuenta lo que la empresa activa alcanza a ver.
select count(*) from product_platform_prices;
