-- +goose Up
-- Los FK al artículo eran ON DELETE SET NULL, lo que contradice el CHECK de la propia tabla:
-- item_type='ingrediente' EXIGE ingredient_id no nulo. Poner el id en null deja la fila violando
-- el check, así que borrar un ingrediente falla con un error incomprensible en vez de funcionar.
--
-- CASCADE es además la semántica correcta: si el artículo ya no existe, el mapeo aprendido no
-- apunta a nada y no hay nada que conservar — el próximo documento vuelve a sugerir.
--
-- Hoy la app solo desactiva artículos (is_active), así que no había forma de llegar aquí desde la
-- UI; se corrige porque el día que exista un borrado real, el error saldría en producción.
alter table supplier_items drop constraint supplier_items_ingredient_id_fkey;
alter table supplier_items add constraint supplier_items_ingredient_id_fkey
  foreign key (ingredient_id) references ingredients (id) on delete cascade;
alter table supplier_items drop constraint supplier_items_product_id_fkey;
alter table supplier_items add constraint supplier_items_product_id_fkey
  foreign key (product_id) references products (id) on delete cascade;

-- +goose Down
alter table supplier_items drop constraint supplier_items_ingredient_id_fkey;
alter table supplier_items add constraint supplier_items_ingredient_id_fkey
  foreign key (ingredient_id) references ingredients (id) on delete set null;
alter table supplier_items drop constraint supplier_items_product_id_fkey;
alter table supplier_items add constraint supplier_items_product_id_fkey
  foreign key (product_id) references products (id) on delete set null;
