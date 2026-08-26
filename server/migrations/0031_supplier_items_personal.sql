-- +goose Up
-- Un renglón puede no ser del local: en la misma compra del Sam's viene el shampoo de la casa.
--
-- Es un estado distinto de 'ignorado'. Una bolsa de plástico o el IVA son 'ignorado': no entran
-- al almacén pero SÍ son gasto del negocio. El shampoo no es ninguna de las dos cosas — no toca
-- el inventario y tampoco es gasto del local, así que su importe no puede sumar al gasto.
--
-- Se aprende por proveedor porque se repite: quien compra su shampoo en el Sam's lo vuelve a
-- comprar ahí, y sin memoria hay que volver a marcar los mismos ocho renglones cada visita.
alter table supplier_items drop constraint supplier_items_status_check;
alter table supplier_items add constraint supplier_items_status_check
  check (status in ('pendiente', 'mapeado', 'ignorado', 'personal'));

-- +goose Down
-- Los renglones marcados como personales vuelven a 'ignorado': es lo más cercano que existe
-- antes de este cambio (no inventariable) y así el rollback no viola el check ni borra el
-- aprendizaje del proveedor.
update supplier_items set status = 'ignorado' where status = 'personal';
alter table supplier_items drop constraint supplier_items_status_check;
alter table supplier_items add constraint supplier_items_status_check
  check (status in ('pendiente', 'mapeado', 'ignorado'));
