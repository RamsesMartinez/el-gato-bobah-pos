-- La lista de compra (spec 084, historia 8): lo que se acabó o está en o bajo su mínimo en la sucursal,
-- con lo que se sabe para sugerir cuánto comprar. Una sola consulta para la pantalla y el correo.

-- name: ShoppingListItems :many
-- Consumo = lo que salió por venta menos lo que volvió por cancelación desde `since`. Un «se acabó»,
-- una merma o un traspaso no son consumo. used_days cuenta los días con venta, para no creerle a un
-- consumo de un solo día.
with cand as (
  select 'ingrediente'::stock_item_type as item_type, i.id, i.name, u.code::text as unit_code, i.coverage_days,
         coalesce(b.min_stock, i.min_stock) as min_stock, i.low_stock_enabled as enabled
    from ingredients i
    join units u on u.id = i.base_unit_id
    left join branch_stock_minimums b on b.branch_id = current_branch_id() and b.ingredient_id = i.id
   where i.is_active
  union all
  select 'producto'::stock_item_type, p.id, p.name, 'pieza'::text, p.coverage_days,
         coalesce(b.min_stock, p.min_stock), p.low_stock_enabled
    from products p
    left join branch_stock_minimums b on b.branch_id = current_branch_id() and b.product_id = p.id
   where p.is_active and p.track_stock
), base as (
  select c.*, coalesce(l.on_hand, 0)::numeric as on_hand,
         exists (select 1 from notifications n where n.status = 'open' and n.kind = 'stock_out' and n.branch_id = current_branch_id()
                   and n.item_key = case when c.item_type = 'ingrediente' then c.id else -c.id end) as out_open,
         exists (select 1 from notifications n where n.status = 'open' and n.kind = 'stock_low' and n.branch_id = current_branch_id()
                   and n.item_key = case when c.item_type = 'ingrediente' then c.id else -c.id end) as low_open
    from cand c
    left join stock_levels l on l.branch_id = current_branch_id()
     and ((c.item_type = 'ingrediente' and l.ingredient_id = c.id) or (c.item_type = 'producto' and l.product_id = c.id))
)
-- coalesce explícito: sqlc tipa estas columnas como no nulas (vienen de un CTE y de un lateral), y un
-- NULL tumbaría la consulta entera (AGENTS.md). 0 = sin mínimo; '' = sin proveedor conocido.
select b.item_type, b.id, b.name, b.unit_code, coalesce(b.coverage_days, 0)::int as coverage_days,
       coalesce(b.min_stock, 0)::numeric as min_stock, b.on_hand, b.out_open, b.low_open,
       coalesce(u.used, 0)::numeric as used, coalesce(u.days, 0)::int as used_days, coalesce(sup.name, '')::text as supplier
  from base b
  left join lateral (
    select -sum(m.quantity) as used, count(distinct (m.created_at at time zone 'UTC')::date) as days
      from stock_movements m
     where m.branch_id = current_branch_id() and m.movement_type in ('venta', 'cancelacion')
       and m.created_at >= sqlc.arg(since)
       and ((b.item_type = 'ingrediente' and m.ingredient_id = b.id) or (b.item_type = 'producto' and m.product_id = b.id))
  ) u on true
  left join lateral (
    select s.name from expense_items ei
      join expenses e on e.id = ei.expense_id and e.cancelled_at is null
      join suppliers s on s.id = e.supplier_id
     where b.item_type = 'ingrediente' and ei.ingredient_id = b.id and ei.voided_at is null
     order by e.expense_date desc, ei.id desc
     limit 1
  ) sup on true
 where b.out_open or b.low_open or (b.enabled and coalesce(b.min_stock, 0) > 0 and b.on_hand <= b.min_stock)
 order by coalesce(sup.name, '') = '', sup.name, b.name;

-- name: PacksWithLastCost :many
-- Los empaques activos de los insumos de la lista, con su costo por unidad base en la última compra.
-- -1 = el empaque nunca se compró (coalesce explícito: sqlc lo tipa como no nulo).
select p.ingredient_id, p.name, p.qty_in_base, coalesce(lc.cost, -1)::numeric as unit_cost
  from ingredient_presentations p
  left join lateral (
    select (ei.amount / nullif(ei.quantity * ei.pack_qty_in_base, 0))::numeric as cost
      from expense_items ei join expenses e on e.id = ei.expense_id and e.cancelled_at is null
     where ei.presentation_id = p.id and ei.voided_at is null and ei.quantity > 0
     order by e.expense_date desc, ei.id desc
     limit 1
  ) lc on true
 where p.active and p.ingredient_id = any(sqlc.arg(ids)::bigint[]);
