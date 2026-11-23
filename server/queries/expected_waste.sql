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
