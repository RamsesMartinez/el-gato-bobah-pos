-- Fotos del catálogo (spec 091). La empresa la pone RLS: ninguna consulta nombra company_id salvo la
-- tabla nueva, que sqlc sí conoce.

-- name: LockProductImage :one
-- Bloquea la fila mientras se cambia la foto: dos subidas casi a la vez no borran una la de la otra.
select image_url from products where id = @id for update;

-- name: SetProductImage :exec
update products set image_url = sqlc.narg(image_url) where id = @id;

-- name: LockCategoryImage :one
select image_url from categories where id = @id for update;

-- name: SetCategoryImage :exec
update categories set image_url = sqlc.narg(image_url) where id = @id;

-- name: LockCategoryFamily :one
-- La familia de antes, para la bitácora.
select coalesce(food_family::text, '')::text as food_family from categories where id = @id for update;

-- name: SetCategoryFoodFamily :exec
update categories set food_family = sqlc.narg(food_family)::food_family where id = @id;

-- name: AdminCategoryList :many
-- La pestaña de categorías: foto, familia y cuántos productos activos del catálogo tiene cada una.
-- Con include_inactive (el MCP, spec 099) también las apagadas, para poder reactivarlas. «Producto
-- activo» es el mismo predicado que CategoryActiveProductCount: la cifra que se ve en la lista es la
-- que bloquea apagarla.
select c.id, c.name, c.parent_id, c.image_url, coalesce(c.food_family::text, '')::text as food_family,
       c.is_active,
       (select count(*) from products p
         where p.category_id = c.id and p.is_active and p.system_kind is null)::int as product_count,
       (select count(*) from products p
         where p.category_id = c.id and not p.is_active and p.system_kind is null)::int as inactive_product_count
  from categories c
 where c.is_active or @include_inactive::boolean
 order by c.parent_id nulls first, c.sort_key, c.name;

-- name: AddMediaPendingDeletion :exec
-- Un borrado que falló. Si ya estaba, suma el intento: el mismo archivo no se apila.
insert into media_pending_deletions (object_key, last_error)
values (@object_key, @last_error)
on conflict on constraint media_pending_deletions_key
do update set attempts = media_pending_deletions.attempts + 1, last_error = excluded.last_error, updated_at = now();

-- name: ListMediaPendingDeletions :many
select id, object_key from media_pending_deletions order by id limit @max;

-- name: DeleteMediaPendingDeletion :exec
delete from media_pending_deletions where id = @id;

-- name: BumpMediaPendingDeletion :exec
update media_pending_deletions set attempts = attempts + 1, last_error = @last_error, updated_at = now()
 where id = @id;

-- name: ProductImageInfo :one
-- Lo que la vista previa del MCP muestra antes de poner o quitar una foto.
select id, name::text as name, image_url from products where id = @id;

-- name: CategoryImageInfo :one
select id, name::text as name, image_url, coalesce(food_family::text, '')::text as food_family
  from categories where id = @id;

-- name: DeleteMediaPendingByKeys :exec
-- Una llave que vuelve a estar en uso (se regresó a la foto de antes) deja de estar pendiente: el
-- barrido no puede borrar una foto viva.
delete from media_pending_deletions where object_key = any(@keys::text[]);

-- name: LockMediaEntity :exec
-- Serializa las escrituras de foto de una misma entidad, desde la subida hasta el borrado de lo
-- viejo: dos subidas a la vez no se borran una a la otra. Candado de sesión, en la conexión de la
-- empresa que el request ya tiene apartada.
select pg_advisory_lock(hashtextextended(@entity::text, 91));

-- name: UnlockMediaEntity :exec
select pg_advisory_unlock(hashtextextended(@entity::text, 91));
