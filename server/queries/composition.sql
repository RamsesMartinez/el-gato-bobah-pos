-- name: ListProductsForStock :many
-- Lo mínimo de cada producto para saber qué descuenta al venderse (spec 028). Sin filtro de empresa:
-- lo pone RLS.
select id, type, track_stock, recipe_id from products;

-- name: GetProductComposition :one
select p.id, p.type, p.track_stock, p.recipe_id, p.composition_status, p.composition_confirmed_at,
       u.name as confirmed_by_name
  from products p left join users u on u.id = p.composition_confirmed_by
 where p.id = $1;

-- name: GetOptionComposition :one
select o.id, o.recipe_id, o.linked_product_id, lp.name as linked_product_name,
       o.composition_status, o.composition_confirmed_at, u.name as confirmed_by_name
  from modifier_options o
  left join products lp on lp.id = o.linked_product_id
  left join users u on u.id = o.composition_confirmed_by
 where o.id = $1;

-- name: ListCompositionItems :many
select ri.ingredient_id, i.name as ingredient_name, ri.quantity, ri.unit_id, u.code as unit_code
  from recipe_items ri
  join ingredients i on i.id = ri.ingredient_id
  join units u on u.id = ri.unit_id
 where ri.recipe_id = $1
 order by ri.position, ri.id;

-- name: ListIngredientUnitKinds :many
-- El tipo de unidad base de cada insumo. Uno de otra empresa no sale (RLS), y eso es lo que impide
-- ligarlo: la FK de recipe_items es simple.
select i.id, u.kind::text as kind
  from ingredients i join units u on u.id = i.base_unit_id
 where i.id = any(sqlc.arg(ids)::bigint[]);

-- name: ListUnitKinds :many
select id, kind::text as kind from units where id = any(sqlc.arg(ids)::smallint[]);

-- name: GetLinkableProduct :one
select id, type, name::text as name from products where id = $1;

-- name: CreateCompositionRecipe :one
-- Receta nueva en cada captura y no editar la que había: `recipe_id` es único por tabla, pero un
-- producto y un extra cargados de FUDO pueden apuntar a la misma, y editarla en su lugar cambiaría
-- lo que descuenta el otro sin que nadie lo viera.
insert into recipes default values returning id;

-- name: InsertCompositionItem :exec
insert into recipe_items (recipe_id, ingredient_id, quantity, unit_id, position) values ($1, $2, $3, $4, $5);

-- name: SetProductComposition :execrows
-- El tipo va con la composición: un paquete es `combo` y no lleva receta (check de 0004).
update products
   set type = sqlc.arg(product_type)::product_type,
       recipe_id = sqlc.narg(recipe_id),
       composition_status = sqlc.narg(status),
       composition_confirmed_by = sqlc.narg(confirmed_by),
       composition_confirmed_at = case when sqlc.narg(status)::text = 'confirmed' then now() end
 where id = sqlc.arg(id);

-- name: SetOptionComposition :execrows
update modifier_options
   set recipe_id = sqlc.narg(recipe_id),
       linked_product_id = sqlc.narg(linked_product_id),
       composition_status = sqlc.narg(status),
       composition_confirmed_by = sqlc.narg(confirmed_by),
       composition_confirmed_at = case when sqlc.narg(status)::text = 'confirmed' then now() end
 where id = sqlc.arg(id);

-- name: ConfirmProductComposition :execrows
-- Confirmar sin composición es decidir «no lleva nada que se descuente» (un «Sin hielo»): así deja
-- de salir en «sin capturar», y lo pendiente no se confunde con lo decidido.
update products set composition_status = 'confirmed', composition_confirmed_by = $2, composition_confirmed_at = now()
 where id = $1 and composition_status is distinct from 'confirmed';

-- name: ConfirmOptionComposition :execrows
update modifier_options set composition_status = 'confirmed', composition_confirmed_by = $2, composition_confirmed_at = now()
 where id = $1 and composition_status is distinct from 'confirmed';

-- name: ListPackageComponents :many
-- Lo que lleva un paquete: el producto por omisión de cada hueco y cuántas piezas.
select csp.product_id, p.name::text as product_name, cs.min_select
  from combo_slots cs
  join combo_slot_products csp on csp.slot_id = cs.id and csp.is_default
  join products p on p.id = csp.product_id
 where cs.combo_id = $1
 order by cs.position, cs.id;

-- name: IsPackageComponent :one
-- Si el producto va dentro de algún paquete: entonces no puede volverse paquete él mismo.
select exists (select 1 from combo_slot_products where product_id = $1)::boolean as used;

-- name: DeletePackageSlots :exec
-- Los productos de cada hueco se van en cascada.
delete from combo_slots where combo_id = $1;

-- name: InsertPackageSlot :one
-- Un hueco por producto, con un solo producto: min = max = las piezas que lleva.
insert into combo_slots (combo_id, name, min_select, max_select, position)
values (sqlc.arg(combo_id), sqlc.arg(name), sqlc.arg(quantity), sqlc.arg(quantity), sqlc.arg(position))
returning id;

-- name: InsertPackageSlotProduct :exec
insert into combo_slot_products (slot_id, product_id, is_default) values ($1, $2, true);

-- name: InsertOrderLineComponent :exec
-- Lo que se vendió dentro de un paquete, copiado al vender: editar el paquete después no reescribe
-- el historial.
insert into order_line_components (order_line_id, product_id, quantity) values ($1, $2, $3);
