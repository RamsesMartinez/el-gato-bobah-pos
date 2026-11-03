-- Deshace 20_sin_y_no_aplica.sql: los 13 extras vuelven a «pendiente».
--
-- Solo regresa los que siguen como los dejó el 20 (confirmados, sin receta y sin producto ligado):
-- si alguien les capturó una receta después, ese trabajo no se pierde. Termina en ROLLBACK; para
-- aplicarlo se cambia la última línea por COMMIT.

\set ON_ERROR_STOP on
begin;
select set_config('app.company_id', '2', true);
set local role gatobobah_app;
select pg_advisory_xact_lock(28028, 2);

update modifier_options
   set composition_status = null, composition_confirmed_by = null, composition_confirmed_at = null
 where id in (596, 603, 1101, 1100, 555, 1110, 690, 682, 1126, 636, 1137, 664, 650)
   and composition_status = 'confirmed' and recipe_id is null and linked_product_id is null;

select id, name, composition_status from modifier_options
 where id in (596, 603, 1101, 1100, 555, 1110, 690, 682, 1126, 636, 1137, 664, 650)
 order by name, id;

rollback;
