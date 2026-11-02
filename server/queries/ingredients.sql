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
       ic.name as category, coalesce(sl.on_hand, 0)::numeric(14,4) as on_hand,
       i.is_prep, i.composition_status,
       (select count(*) from recipe_items ri where ri.ingredient_id = i.id)::int as recipe_uses,
       i.count_place_id, cp.name as count_place
from ingredients i
join units u on u.id = i.base_unit_id
left join count_places cp on cp.id = i.count_place_id
left join ingredient_categories ic on ic.id = i.category_id
-- Existencias de la sucursal (0076): sin el filtro, con dos sucursales cada insumo saldría dos
-- veces sin decir de cuál es.
left join stock_levels sl on sl.ingredient_id = i.id and sl.branch_id = current_branch_id()
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

-- name: LockIngredientNames :exec
-- Un alta de insumo a la vez por empresa, hasta el fin de la transacción (spec 037, research R7d).
-- Llave propia, distinta de la 28028 de las recetas: guardar una receta y dar de alta un insumo no
-- tienen por qué esperarse. Sin el candado, dos altas casi simultáneas de «Lata» y «lata» pasaban las
-- dos: el `unique (company_id, name)` solo atrapa el nombre idéntico.
select pg_advisory_xact_lock(28037, coalesce(nullif(current_setting('app.company_id', true), '')::int, 0));

-- name: IngredientNameTaken :one
-- ¿Hay otro insumo de la empresa con el mismo nombre sin distinguir mayúsculas, acentos ni espacios
-- de más? La misma normalización que «duplicado» en Salud del inventario.
select exists (
  select 1 from ingredients
   where regexp_replace(translate(lower(trim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
       = regexp_replace(translate(lower(trim(sqlc.arg(name)::text)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
)::bool;

-- name: ListPresentations :many
-- Empaques de compra (spec 038), los activos primero. Sin filtro, todos los de la empresa: la
-- columna «Cómo llega» los pinta de una vez en vez de pedirlos insumo por insumo.
select p.id, p.ingredient_id, p.name, p.qty_in_base, p.active, p.is_approximate, p.purchase_unit_id,
       u.code as base_unit_code, u.kind as base_unit_kind, p.brand
from ingredient_presentations p
join ingredients i on i.id = p.ingredient_id
join units u on u.id = i.base_unit_id
where (sqlc.narg('ingredient_id')::bigint is null or p.ingredient_id = sqlc.narg('ingredient_id'))
order by p.ingredient_id, p.active desc, p.name, p.id;

-- name: CreatePresentation :one
insert into ingredient_presentations (ingredient_id, name, qty_in_base, purchase_unit_id, is_approximate, brand)
values ($1, $2, $3, $4, $5, $6)
returning id;

-- name: UpdatePresentation :execrows
update ingredient_presentations
set name = $2, qty_in_base = $3, purchase_unit_id = $4, is_approximate = $5, active = $6, brand = $7
where id = $1;

-- name: GetPresentationIngredient :one
-- El insumo, el estado y la marca actuales: editar sin mandar `active` o `brand` conserva lo que tiene.
select ingredient_id, active, brand from ingredient_presentations where id = $1;

-- name: GetUnit :one
select id, code, name, kind, to_base from units where id = $1;
