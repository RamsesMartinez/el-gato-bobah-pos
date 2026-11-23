-- Lo que se vende en la web (spec 092). La regla vive aquí y en app/web_sales.go, y la usa todo lo
-- que muestre o cuente la web: el resumen de Ajustes hoy y el menú público (#316) después.

-- name: WebOfferProducts :many
-- Activos, dentro de su temporada (como el menú del POS) y sin fila «oculto» para Web. Sin fila de
-- lugar de venta = se ofrece, al precio de mostrador.
select p.id, p.price, p.allow_oversell,
       (select pc.price from product_channels pc join channels ch on ch.id = pc.channel_id
         where pc.product_id = p.id and ch.code = 'online') as web_price
from products p
where p.is_active and p.system_kind is null
  and (p.available_from is null or p.available_from <= current_date)
  and (p.available_until is null or p.available_until >= current_date)
  and not exists (select 1 from product_channels pc join channels ch on ch.id = pc.channel_id
                   where pc.product_id = p.id and ch.code = 'online' and pc.visibility = 'oculto')
order by p.id;

-- name: ActiveCatalogCount :one
select count(*)::int from products where is_active and system_kind is null;
