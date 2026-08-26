-- Catálogo de insumos y buscador de artículos.
--
-- Hasta ahora los ingredientes solo entraban por el importador de FUDO (cmd/fudo-import): no
-- había forma de listarlos ni crearlos desde la app, así que el almacén era un ledger sin
-- catálogo consultable. Esto es el prerrequisito del buscador de artículos del gasto.

-- name: ListUnits :many
-- Unidades con su kind y factor a base: el front necesita el kind para no ofrecer kg a un
-- ingrediente que se lleva en ml, y el servicio el to_base para convertir.
select id, code, name, kind, to_base from units order by kind, to_base;

-- name: ListIngredientCategories :many
select id, name from ingredient_categories where is_active order by name;

-- name: CreateIngredientCategory :one
insert into ingredient_categories (name) values ($1) returning id, name;

-- name: ListIngredients :many
select i.id, i.name, i.is_active, i.track_stock, i.is_packaging, i.min_stock,
       i.current_cost, i.base_unit_id, u.code as base_unit_code, u.kind as base_unit_kind,
       ic.name as category, coalesce(sl.on_hand, 0)::numeric(14,4) as on_hand
from ingredients i
join units u on u.id = i.base_unit_id
left join ingredient_categories ic on ic.id = i.category_id
left join stock_levels sl on sl.ingredient_id = i.id
where (sqlc.narg('only_active')::boolean is not true or i.is_active)
order by i.name;

-- name: CreateIngredient :one
-- Alta mínima: nombre + unidad base. Todo lo demás (receta, merma, proveedor) se edita después;
-- el alta tiene que caber en el diálogo del gasto sin sacar al operador del flujo.
insert into ingredients (name, base_unit_id, category_id, min_stock, track_stock, is_packaging)
values ($1, $2, $3, $4, coalesce(sqlc.narg('track_stock')::boolean, true), coalesce(sqlc.narg('is_packaging')::boolean, false))
-- Devuelve los valores REALES tras aplicar los defaults: con un returning parcial la respuesta
-- reportaba track_stock=false cuando en la base había quedado true.
returning id, name, base_unit_id, is_active, track_stock, is_packaging, min_stock, current_cost;

-- name: UpdateIngredient :one
update ingredients
set name = $2, base_unit_id = $3, category_id = $4, min_stock = $5,
    track_stock = $6, is_packaging = $7, is_active = $8, updated_at = now()
where id = $1
returning id, name, base_unit_id, is_active;

-- name: GetIngredientUnit :one
-- El kind y el factor de la unidad base de un artículo: lo que domain.BaseQty necesita para
-- decidir si la unidad de compra es convertible.
select i.id, u.kind as base_unit_kind, u.to_base as base_to_base
from ingredients i join units u on u.id = i.base_unit_id
where i.id = $1;

-- name: SearchArticles :many
-- Buscador ÚNICO sobre los dos catálogos que el almacén sabe mover: ingredientes y productos
-- con control de stock (una bebida que se compra y se revende tal cual). Un solo picker sobre
-- un union en vez de obligar al operador a elegir primero "¿es ingrediente o producto?".
select 'ingrediente' as item_type, i.id, i.name::text as name,
       u.code as unit_code, u.kind as unit_kind
from ingredients i
join units u on u.id = i.base_unit_id
where i.is_active
  and (sqlc.narg('q')::text is null or i.name::text ilike '%' || sqlc.narg('q')::text || '%')
union all
select 'producto' as item_type, p.id, p.name::text as name,
       'pieza' as unit_code, 'pieza'::unit_kind as unit_kind
from products p
where p.is_active and p.track_stock
  and (sqlc.narg('q')::text is null or p.name::text ilike '%' || sqlc.narg('q')::text || '%')
order by name
limit sqlc.arg('lim');
