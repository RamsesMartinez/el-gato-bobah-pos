-- Nombres alternativos de productos y artículos (spec 063). La llave con que dos nombres son el mismo
-- es catalog_name_key (0107), gemela de domain.CatalogNameKey. El choque se decide bajo el candado de
-- nombres de cada catálogo, tomado ANTES de leer: sin él, dos tabletas que guardan «café frío» en dos
-- productos a la vez leen las dos «no existe» y quedan las dos.

-- ==== Productos ====

-- name: LockProductNames :exec
-- Un guardado de nombres de producto a la vez por empresa, hasta el fin de la transacción. Lo toman
-- los alternativos, el alta, el renombre y el duplicado.
select pg_advisory_xact_lock(28063, coalesce(nullif(current_setting('app.company_id', true), '')::int, 0));

-- name: GetProductForAltNames :one
-- El nombre propio, con la fila tomada. El genérico de plataforma no es del catálogo: no lleva
-- alternativos y responde como inexistente, igual que su edición.
select name::text as name from products where id = sqlc.arg(id) and system_kind is null for update;

-- name: FirstProductNameClash :one
-- El primer nombre propuesto (en el orden capturado) que ya es el nombre o un alternativo de OTRO
-- producto de la empresa. El nombre propio lo descarta el dominio antes de llegar aquí.
select pr.n::text as name, p.id as owner_id, p.name::text as owner_name, p.is_active as owner_active,
       (catalog_name_key(p.name) <> catalog_name_key(pr.n))::bool as via_alt
  from unnest(sqlc.arg(names)::text[]) with ordinality pr(n, ord)
  join products p on p.id <> sqlc.arg(id) and p.system_kind is null
   and (catalog_name_key(p.name) = catalog_name_key(pr.n)
        or exists (select 1 from unnest(p.alt_names) a where catalog_name_key(a) = catalog_name_key(pr.n)))
 order by pr.ord, via_alt, p.id
 limit 1;

-- name: ProductAltNameOwner :one
-- Al dar de alta o renombrar: ¿el nombre ya es alternativo de un producto? (FR-005). También del
-- propio: llamarse igual que uno de sus alternativos deja a ese alternativo sobrando. Solo
-- alternativos: nombre contra nombre lo sigue decidiendo el índice único de siempre.
select p.id, p.name::text as name, p.is_active from products p
 where p.system_kind is null
   and exists (select 1 from unnest(p.alt_names) a where catalog_name_key(a) = catalog_name_key(sqlc.arg(name)::text))
 order by p.id
 limit 1;

-- name: GetProductName :one
select name::text as name from products where id = sqlc.arg(id);

-- name: SetProductAltNames :execrows
-- La lista completa. coalesce: una lista vacía de Go puede llegar como NULL. Sube la versión del
-- catálogo por el trigger de la 0093: el menú de las tabletas se rearma solo.
update products set alt_names = coalesce(sqlc.arg(names)::text[], '{}')
 where id = sqlc.arg(id) and system_kind is null;

-- name: ProductAltNameCandidates :many
-- Nombre y alternativos de todos los productos del catálogo, para el aviso de parecidos.
select p.id, p.name::text as name, p.alt_names from products p where p.system_kind is null order by p.id;

-- ==== Artículos del almacén ====

-- name: GetArticleForAltNames :one
select name::text as name, alt_names from ingredients where id = sqlc.arg(id) for update;

-- name: FirstArticleNameClash :one
-- Como en productos. Un artículo que es origen de una unión no cuenta: no puede volver, y si
-- contara, después de unir «Leche Lala» en «Leche entera» no se podría capturar «Leche Lala» como
-- alternativo del destino, que es justo para lo que se une.
select pr.n::text as name, i.id as owner_id, i.name::text as owner_name, i.is_active as owner_active,
       (catalog_name_key(i.name) <> catalog_name_key(pr.n))::bool as via_alt
  from unnest(sqlc.arg(names)::text[]) with ordinality pr(n, ord)
  join ingredients i on i.id <> sqlc.arg(id)
   and not exists (select 1 from ingredient_replacements r where r.old_ingredient_id = i.id and r.kind = 'merge')
   and (catalog_name_key(i.name) = catalog_name_key(pr.n)
        or exists (select 1 from unnest(i.alt_names) a where catalog_name_key(a) = catalog_name_key(pr.n)))
 order by pr.ord, via_alt, i.id
 limit 1;

-- name: SetArticleAltNames :one
-- Devuelve la versión nueva de la ficha: el trigger de la 0009 mueve updated_at en todo update.
update ingredients set alt_names = coalesce(sqlc.arg(names)::text[], '{}') where id = sqlc.arg(id)
returning updated_at;

-- name: ArticleAltNameCandidates :many
select i.id, i.name::text as name, i.alt_names from ingredients i
 where not exists (select 1 from ingredient_replacements r where r.old_ingredient_id = i.id and r.kind = 'merge')
 order by i.id;
