-- 18_rollback.sql — revierte 18_desactivar_otros_envio.sql
-- Reactiva exactamente lo que 18 desactivó (categoría 30 y producto 389). No toca el resto de
-- productos de la 30, que ya estaban inactivos de antes.
update categories set is_active = true where id = 30;
update products  set is_active = true where id = 389;
