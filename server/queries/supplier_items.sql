-- Mapeo aprendido proveedor → inventario, y las sugerencias.
--
-- El matching vive AQUÍ y no en el extractor a propósito: es determinista, gratis, instantáneo,
-- y sirve igual cuando se capturan las líneas a mano sin documento. El modelo lee papeles;
-- Postgres decide a qué artículo se parece cada renglón.

-- name: LookupSupplierItem :one
-- Paso 1 y 2 de la cascada: coincidencia EXACTA por la llave aprendida (código del proveedor
-- si identifica al artículo, y si no el nombre normalizado). Es la que autollena sin preguntar.
select si.id, si.status, si.item_type, si.ingredient_id, si.product_id,
       si.pack_qty_in_base, si.unit_id, si.last_cost, si.raw_name,
       i.name as ingredient_name, p.name as product_name
from supplier_items si
left join ingredients i on i.id = si.ingredient_id
left join products p on p.id = si.product_id
where si.supplier_id = sqlc.arg('supplier_id')
  and coalesce(si.raw_code, si.norm_name) = sqlc.arg('item_key')::text;

-- name: SuggestFromSupplierItems :many
-- Paso 3: parecido contra renglones YA MAPEADOS, de cualquier proveedor. Mapeas "Coca Cola
-- 600 ml" comprando en una tienda y el "COCA COLA 600ML" de otra pega solo.
-- El proveedor propio se excluye porque su coincidencia exacta ya la resolvió LookupSupplierItem.
select si.item_type, si.ingredient_id, si.product_id, si.pack_qty_in_base, si.unit_id,
       i.name as ingredient_name, p.name as product_name,
       si.raw_name as matched_via,
       similarity(si.norm_name, sqlc.arg('needle')::text) as score
from supplier_items si
left join ingredients i on i.id = si.ingredient_id
left join products p on p.id = si.product_id
where si.status = 'mapeado'
  and si.supplier_id <> sqlc.arg('supplier_id')
  and si.norm_name % sqlc.arg('needle')::text
order by score desc
limit sqlc.arg('lim');

-- name: SuggestArticlesByName :many
-- Paso 4: parecido contra el catálogo propio, para cuando el proveedor es nuevo y no hay nada
-- aprendido. Se compara contra lower(name) y no contra una versión sin acentos: pg_trgm es
-- difuso por diseño, así que "piña" contra "pina" sigue puntuando alto y evita depender de la
-- extensión unaccent solo para eso.
select 'ingrediente' as item_type, i.id, i.name::text as item_name,
       similarity(lower(i.name::text), sqlc.arg('needle')::text) as score
from ingredients i
where i.is_active and lower(i.name::text) % sqlc.arg('needle')::text
union all
select 'producto' as item_type, p.id, p.name::text as item_name,
       similarity(lower(p.name::text), sqlc.arg('needle')::text) as score
from products p
where p.is_active and p.track_stock and lower(p.name::text) % sqlc.arg('needle')::text
order by score desc
limit sqlc.arg('lim');

-- name: UpsertSupplierItem :one
-- El bucle de aprendizaje completo: confirmar una línea escribe (o actualiza) su fila, y la
-- próxima compra en ese proveedor cae en LookupSupplierItem.
insert into supplier_items (
  supplier_id, raw_code, raw_name, norm_name, status,
  item_type, ingredient_id, product_id, pack_qty_in_base, unit_id, last_cost
) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
on conflict (company_id, supplier_id, coalesce(raw_code, norm_name)) do update set
  raw_name = excluded.raw_name,
  status = excluded.status,
  item_type = excluded.item_type,
  ingredient_id = excluded.ingredient_id,
  product_id = excluded.product_id,
  -- El formato y el costo solo se sobreescriben si el nuevo trae dato: una captura sin gramaje
  -- no debe borrar el que ya se había aprendido.
  pack_qty_in_base = coalesce(excluded.pack_qty_in_base, supplier_items.pack_qty_in_base),
  unit_id = coalesce(excluded.unit_id, supplier_items.unit_id),
  last_cost = coalesce(excluded.last_cost, supplier_items.last_cost),
  last_seen_at = now()
returning id;

-- name: ListSupplierItems :many
-- La cola de revisión (status='pendiente') y la consulta del catálogo aprendido.
select si.id, si.supplier_id, s.name as supplier, si.raw_code, si.raw_name, si.status,
       si.item_type, si.ingredient_id, si.product_id, si.pack_qty_in_base, si.unit_id,
       si.last_cost, si.last_seen_at,
       i.name as ingredient_name, p.name as product_name
from supplier_items si
join suppliers s on s.id = si.supplier_id
left join ingredients i on i.id = si.ingredient_id
left join products p on p.id = si.product_id
where (sqlc.narg('status')::text is null or si.status = sqlc.narg('status'))
  and (sqlc.narg('supplier_id')::bigint is null or si.supplier_id = sqlc.narg('supplier_id'))
order by si.last_seen_at desc, si.id desc
limit sqlc.arg('lim') offset sqlc.arg('off');

-- name: CountSupplierItems :one
select count(*) from supplier_items si
where (sqlc.narg('status')::text is null or si.status = sqlc.narg('status'))
  and (sqlc.narg('supplier_id')::bigint is null or si.supplier_id = sqlc.narg('supplier_id'));

-- name: DeleteSupplierItem :execrows
-- Para deshacer un mapeo equivocado: se borra la fila y el siguiente documento vuelve a
-- sugerir desde cero, en vez de arrastrar el error.
delete from supplier_items where id = $1;
