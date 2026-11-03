-- Deshace 21_correccion_4oct.sql.
--
-- Las existencias no se deshacen borrando: el disparador de stock_movements ya sumó a stock_levels.
-- Se inserta el movimiento contrario de cada ajuste, y los motivos vuelven a los de origen (la 690
-- decía «Se capturó de más»; las demás, «Ya no lo quiere»). Aborta si la corrección no está aplicada
-- o ya se revirtió. Termina en ROLLBACK; para aplicarlo se cambia la última línea por COMMIT.

\set ON_ERROR_STOP on
begin;
select set_config('app.company_id', '2', true);
set local role gatobobah_app;
lock table stock_movements in share row exclusive mode;

do $$
declare n int;
begin
  select count(*) into n from stock_movements where reason = 'Corrección 4-oct: cuenta dividida';
  if n <> 16 then raise exception 'la corrección no está aplicada (% movimientos)', n; end if;
  select count(*) into n from stock_movements where reason = 'Reverso de la corrección 4-oct';
  if n <> 0 then raise exception 'ya se revirtió (% movimientos)', n; end if;

  insert into stock_movements (company_id, item_type, ingredient_id, product_id, movement_type, quantity,
                               unit_cost, order_id, order_line_id, user_id, reason, note,
                               modifier_option_id, component_of_product_id)
  select company_id, item_type, ingredient_id, product_id, 'ajuste', -quantity, unit_cost, order_id,
         order_line_id, user_id, 'Reverso de la corrección 4-oct', 'Deshace 21_correccion_4oct.sql.',
         modifier_option_id, component_of_product_id
    from stock_movements where reason = 'Corrección 4-oct: cuenta dividida';

  update order_lines set cancel_reason = case when id = 690 then 'Se capturó de más' else 'Ya no lo quiere' end
   where id in (690, 691, 692, 693, 694, 695, 696, 697, 698, 699, 700, 701, 702, 703, 705, 706, 707, 708)
     and cancel_reason = 'Se capturó de más / se cobró en otro pedido';
  get diagnostics n = row_count;
  if n <> 18 then raise exception 'se esperaban 18 motivos, cambiaron %: se deshace', n; end if;
end $$;

rollback;
