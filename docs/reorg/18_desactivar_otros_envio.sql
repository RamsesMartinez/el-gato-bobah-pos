-- 18_desactivar_otros_envio.sql
-- Desactiva la categoría "Otros" (id 30, top-level) y su único producto activo, "Envio" (389).
-- El envío ahora se cobra con el ajuste de negocio (business_settings.delivery_fee) al tipo
-- "domicilio" en el cobro, no como producto manual.
--
-- Desactivar, NO borrar: hay órdenes históricas que referencian a Envio (order_lines.product_id)
-- y a la categoría; un DELETE rompería FKs y falsearía reportes.
--
-- OJO: solo la id 30. Existe otra "Otros" (id 51) que es la SUBcategoría de ramen (Carbonara,
-- Udon) colgando de id 23 — esa NO se toca. Al 2026-07 el único producto activo en la 30 es
-- Envio (389); el resto (Donas, Papas, Juego Gonggi, …) ya está inactivo.

\echo '--- Antes ---'
select id, name, is_active from categories where id = 30;
select id, name, is_active from products where id = 389;

update categories set is_active = false where id = 30;
update products  set is_active = false where id = 389;

\echo '--- Después (ambos deben quedar is_active=f) ---'
select id, name, is_active from categories where id = 30;
select id, name, is_active from products where id = 389;
