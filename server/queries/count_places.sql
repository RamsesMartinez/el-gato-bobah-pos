-- Lugares para contar (spec 040). El lugar ordena el recorrido del conteo; no es un almacén.

-- name: LockCountPlaces :exec
-- Crear, mover y archivar renumeran el recorrido: sin este candado por empresa, dos altas a la vez
-- repetirían posición. Va antes de leer el orden, y el orden se lee en OTRA sentencia: un
-- `for update ... order by` en READ COMMITTED espera y conserva el orden de la foto vieja.
select pg_advisory_xact_lock(28040, coalesce(nullif(current_setting('app.company_id', true), '')::int, 0));

-- name: ListCountPlaces :many
-- Los que se ofrecen en el almacén, en el orden de su recorrido, con cuántos artículos tienen
-- (también los inactivos: al reactivarlos vuelven con su lugar).
select p.id, p.name, p.position,
       (select count(*) from ingredients i where i.count_place_id = p.id)::int as ingredients
  from count_places p
 where p.archived_at is null and p.warehouse_id = sqlc.arg(warehouse_id)
 order by p.position, p.id;

-- name: ActiveCountPlaceIDs :many
-- El recorrido del almacén del lugar: cada almacén se ordena aparte.
select id from count_places
 where archived_at is null and warehouse_id = (select c.warehouse_id from count_places c where c.id = sqlc.arg(place_id))
 order by position, id;

-- name: CreateCountPlace :one
insert into count_places (name, warehouse_id, position)
select sqlc.arg(name), sqlc.arg(warehouse_id)::bigint, coalesce(max(position) + 1, 0)
  from count_places where archived_at is null and warehouse_id = sqlc.arg(warehouse_id)::bigint
returning id;

-- name: CountPlaceByName :one
-- El que ya existe con ese nombre normalizado en el almacén (la misma expresión que el índice).
select id from count_places
 where archived_at is null and warehouse_id = sqlc.arg(warehouse_id)
   and regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
     = regexp_replace(translate(lower(btrim(sqlc.arg(name)::text)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g');

-- name: RenameCountPlace :execrows
update count_places set name = sqlc.arg(name) where id = sqlc.arg(id) and archived_at is null;

-- name: SetCountPlacePosition :exec
update count_places set position = sqlc.arg(position) where id = sqlc.arg(id);

-- name: LockCountPlaceForUpdate :one
-- Archivar lo toma así antes de mover sus artículos; asignar lo toma `for share`, así que uno espera
-- al otro y ningún artículo queda en un lugar archivado.
select id, warehouse_id, archived_at from count_places where id = $1 for update;

-- name: LockCountPlaceForShare :one
select id, warehouse_id, archived_at from count_places where id = $1 for share;

-- name: CountPlaceIngredients :one
select count(*)::int from ingredients where count_place_id = $1;

-- name: MoveCountPlaceIngredients :exec
-- Todos, activos o no: un inactivo que se quedara aquí reaparecería con un lugar archivado.
update ingredients set count_place_id = sqlc.narg(to_place_id), updated_at = now()
 where count_place_id = sqlc.arg(from_place_id);

-- name: ArchiveCountPlace :exec
update count_places set archived_at = now() where id = $1;
