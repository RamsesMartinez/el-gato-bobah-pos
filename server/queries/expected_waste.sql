-- Merma real de 90 días (spec 096): solo informa, ningún costo sale de aquí. Las dos sumas se piden
-- para el artículo y los que se le unieron (las ventas y mermas de antes de una unión siguen con el id
-- del origen); el factor de cada uno lo aplica domain.MergeChain. Agrupado por artículo y almacén en
-- una sola consulta para todos los renglones de una receta, no una por renglón (R14 M2).

-- name: ListMergeEdges :many
select old_ingredient_id, new_ingredient_id, factor
  from ingredient_replacements
 where kind = 'merge';

-- name: ListRealWasteConsumption :many
-- Lo que las ventas descontaron, neto de cancelaciones que reponen. La merma esperada que descuente
-- la subida 2 no es consumo (R14 H2) y por eso el filtro nombra los dos tipos.
select sm.ingredient_id::bigint as ingredient_id, sm.warehouse_id::bigint as warehouse_id,
       (-sum(sm.quantity))::numeric(16,4) as consumed
  from stock_movements sm
 where sm.ingredient_id = any(sqlc.arg(ids)::bigint[])
   and sm.movement_type in ('venta', 'cancelacion')
   and sm.order_id is not null
   -- La 0120 llenó el almacén de todo lo anterior; el filtro existe porque el cast de arriba haría que
   -- un nulo tumbara la consulta entera en vez de ese renglón.
   and sm.warehouse_id is not null
   and sm.created_at >= now() - interval '90 days'
 group by sm.ingredient_id, sm.warehouse_id;

-- name: ListRealWasteRecords :many
-- Lo registrado como merma que restó: aplicada o cubierta después por un conteo. La comida del
-- personal no es pérdida; por aprobar, rechazada y deshecha nunca restaron. La de producto terminado
-- no tiene artículo y no entra.
select wr.ingredient_id::bigint as ingredient_id, wr.warehouse_id::bigint as warehouse_id,
       sum(wr.quantity)::numeric(16,4) as wasted
  from waste_records wr
 where wr.ingredient_id = any(sqlc.arg(ids)::bigint[])
   and wr.status in ('applied', 'covered')
   and not wr.is_staff_meal
   and wr.warehouse_id is not null
   and wr.recorded_at >= now() - interval '90 days'
 group by wr.ingredient_id, wr.warehouse_id;

-- name: ListIngredientLabels :many
-- Nombre y unidad base de los artículos de un desglose.
select i.id, i.name::text as name, u.code::text as base_unit
  from ingredients i
  join units u on u.id = i.base_unit_id
 where i.id = any(sqlc.arg(ids)::bigint[]);

-- name: ListProductLabels :many
select p.id, p.name::text as name from products p where p.id = any(sqlc.arg(ids)::bigint[]);

-- name: CountVsSalesLines :many
-- Conteo contra ventas (spec 096, subida 2): por insumo contado, lo que pasó en su almacén entre el
-- conteo cerrado anterior que lo vio y éste, con el mismo ancla por id de movimiento que el faltante
-- (CountLossLines). Cada suma va con coalesce: un insumo sin movimientos en el periodo da 0, y un NULL
-- tumbaría la consulta entera (AGENTS.md).
select l.ingredient_id, i.name::text as name, u.code::text as base_unit_code, l.counted_qty, l.system_qty,
       (prev.last_movement_id is not null and l.last_movement_id is not null)::bool as has_previous,
       coalesce(prev.counted_qty, 0)::numeric(14,4) as initial_qty,
       coalesce(mv.sold, 0)::numeric(14,4) as sold,
       coalesce(mv.expected_waste, 0)::numeric(14,4) as expected_waste,
       coalesce(mv.registered, 0)::numeric(14,4) as registered,
       coalesce(mv.transfers_out, 0)::numeric(14,4) as transfers_out,
       coalesce(mv.entries, 0)::numeric(14,4) as entries
  from stock_count_lines l
  join stock_counts c on c.id = l.count_id
  join ingredients i on i.id = l.ingredient_id
  join units u on u.id = i.base_unit_id
  left join lateral (
    select pl.last_movement_id, pl.counted_qty from stock_count_lines pl join stock_counts pc on pc.id = pl.count_id
     where pc.status = 'closed' and pc.warehouse_id = c.warehouse_id and pc.id <> c.id
       and pl.ingredient_id = l.ingredient_id and pl.captured_at < l.captured_at
     order by pl.captured_at desc limit 1
  ) prev on true
  left join lateral (
    select -sum(m.quantity) filter (where m.movement_type in ('venta', 'cancelacion')) as sold,
           -sum(m.quantity) filter (where m.movement_type = 'merma_esperada') as expected_waste,
           -sum(m.quantity) filter (where m.waste_record_id is not null) as registered,
           -sum(m.quantity) filter (where m.movement_type = 'traspaso' and m.quantity < 0) as transfers_out,
           sum(m.quantity) filter (where m.movement_type in ('compra', 'traspaso') and m.quantity > 0) as entries
      from stock_movements m
     where m.ingredient_id = l.ingredient_id and m.warehouse_id = c.warehouse_id
       and m.id > prev.last_movement_id and m.id <= l.last_movement_id
  ) mv on true
 where l.count_id = sqlc.arg(count_id) and c.status = 'closed'
 order by i.name, l.ingredient_id;

-- name: PreviousClosedCounts :many
-- Los dos conteos cerrados anteriores del mismo almacén, el más reciente primero, para ver si la
-- diferencia de un artículo es estable.
select pc.id from stock_counts pc join stock_counts c on c.id = sqlc.arg(count_id)
 where pc.status = 'closed' and pc.warehouse_id = c.warehouse_id and pc.id < c.id
 order by pc.id desc limit 2;
