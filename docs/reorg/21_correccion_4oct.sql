-- 21 — Corrección del 4-oct: una cuenta dividida a mano (pedidos 285, 286 y 287).
--
-- QUÉ PASÓ. Una cuenta se pagó por separado antes de que existiera «Dividir la cuenta». Para
-- lograrlo se quitaron 18 renglones de los pedidos 285 y 286 y se volvieron a capturar en el 286 y
-- el 287. Las bajas quedaron como cancelaciones con motivo
-- «Ya no lo quiere» y no regresaron nada al almacén, porque ya habían salido a cocina: lo que se
-- volvió a capturar se descontó dos veces, y lo que nunca se sirvió salió sin venderse.
--
-- DECISIÓN DEL DUEÑO (2026-10-13): se cobró bien una sola vez y no se cocinó nada de más. Por eso,
-- como excepción a «lo que salió a cocina se da por gastado»:
--   1. Se regresa al almacén lo que salió por esas 18 bajas, con un ajuste «Corrección 4-oct: cuenta
--      dividida». Menos el renglón 693, cuyo producto ya quedó en cero el 10-oct al pasar a receta
--      y dejó de llevar existencia propia: reponerlo inventaría piezas.
--   2. El motivo de las 18 bajas pasa a «Se capturó de más / se cobró en otro pedido».
-- No cambia lo cobrado ni las ventas: solo existencias y el motivo de las bajas.
--
-- Guardas: corre como gatobobah_app con la empresa 2 y aborta si los pedidos, las 18 bajas o los
-- 16 movimientos a reponer no son exactamente los esperados, o si ya se aplicó. Termina en
-- ROLLBACK; para aplicarlo se cambia la última línea por COMMIT. Reverso: 21_rollback.sql.

\set ON_ERROR_STOP on
begin;
select set_config('app.company_id', '2', true);
set local role gatobobah_app;
lock table stock_movements in share row exclusive mode;

create temp table bajas_4oct (id bigint primary key) on commit drop;
insert into bajas_4oct values
  (690), (691), (692), (693), (694), (695), (696), (697), (698),
  (699), (700), (701), (702), (703), (705), (706), (707), (708);

do $$
declare
  n int;
  quien bigint;
begin
  select count(*) into n from orders
   where business_date = '2026-10-04'
     and id in (285, 286, 287) and status = 'entregada';
  if n <> 3 then raise exception 'no están los tres pedidos del 4-oct (hay %): nada se cambió', n; end if;

  select count(*) into n from order_lines l join bajas_4oct b on b.id = l.id
   where l.order_id in (285, 286) and l.cancelled_at is not null
     and l.cancel_reason in ('Ya no lo quiere', 'Se capturó de más');
  if n <> 18 then raise exception 'se esperaban 18 bajas sin corregir, hay %: nada se cambió', n; end if;

  select count(*) into n from stock_movements where reason = 'Corrección 4-oct: cuenta dividida';
  if n <> 0 then raise exception 'la corrección ya se aplicó (% movimientos): nada se cambió', n; end if;

  select id into quien from users where username = 'admin' and is_active;
  if quien is null then raise exception 'no está el usuario admin de gatobobah'; end if;

  -- Lo que salió por las bajas, salvo el renglón 693: cada movimiento de venta se repone con su mismo
  -- artículo, costo y origen. El disparador de stock_movements suma a stock_levels.
  insert into stock_movements (company_id, item_type, ingredient_id, product_id, movement_type, quantity,
                               unit_cost, order_id, order_line_id, user_id, reason, note,
                               modifier_option_id, component_of_product_id)
  select m.company_id, m.item_type, m.ingredient_id, m.product_id, 'ajuste', -m.quantity,
         m.unit_cost, m.order_id, m.order_line_id, quien, 'Corrección 4-oct: cuenta dividida',
         'Baja de la cuenta dividida a mano; se cobró una sola vez (decisión del dueño, 2026-10-13).',
         m.modifier_option_id, m.component_of_product_id
    from stock_movements m join bajas_4oct b on b.id = m.order_line_id
   where m.movement_type = 'venta' and m.quantity < 0
     and m.order_line_id <> 693;
  get diagnostics n = row_count;
  if n <> 16 then raise exception 'se esperaban 16 movimientos a reponer, salieron %: se deshace', n; end if;

  update order_lines l set cancel_reason = 'Se capturó de más / se cobró en otro pedido'
    from bajas_4oct b where b.id = l.id;
  get diagnostics n = row_count;
  if n <> 18 then raise exception 'se esperaban 18 motivos, cambiaron %: se deshace', n; end if;
end $$;

-- Verificación: lo repuesto por artículo y los motivos nuevos.
select coalesce(i.name, p.name) as articulo, m.item_type, sum(m.quantity) as repuesto
  from stock_movements m left join ingredients i on i.id = m.ingredient_id left join products p on p.id = m.product_id
 where m.reason = 'Corrección 4-oct: cuenta dividida'
 group by 1, 2 order by 1;
select cancel_reason, count(*), sum(line_total) from order_lines
 where id in (select id from bajas_4oct) group by 1;

rollback;
