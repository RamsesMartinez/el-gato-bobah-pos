-- Menús de plataforma (spec 020). Todo bajo RLS: ninguna consulta filtra por company_id porque la
-- política tenant_isolation ya lo hace, igual que el resto del repo.

-- name: ListPlatformConnections :many
-- Las tiendas conectadas de la empresa, con el nombre de su plataforma. Una empresa puede tener
-- VARIAS de la misma plataforma: una por sucursal.
select c.id, c.delivery_platform_id, p.name as platform_name,
       c.external_store_id, c.label, c.is_active, c.created_at
from platform_connections c
join delivery_platforms p on p.id = c.delivery_platform_id
order by p.name, c.label;

-- name: GetPlatformConnection :one
select c.id, c.delivery_platform_id, p.name as platform_name,
       c.external_store_id, c.label, c.is_active
from platform_connections c
join delivery_platforms p on p.id = c.delivery_platform_id
where c.id = $1;

-- name: CreatePlatformConnection :one
insert into platform_connections (delivery_platform_id, external_store_id, label)
values ($1, $2, $3)
returning id;

-- name: DeletePlatformConnection :execrows
-- :execrows para distinguir "se borró" de "no existía": el handler responde 404 en el segundo caso
-- en vez de fingir que hizo algo.
delete from platform_connections where id = $1;

-- name: CountLinksOfConnection :one
-- Cuántas parejas se pierden al dar de baja la tienda. La pantalla lo dice ANTES de confirmar:
-- son decisiones manuales de una sesión completa y no se reconstruyen.
select count(*) from platform_item_links where connection_id = $1;

-- ---------------------------------------------------------------------------------------------
-- Lecturas
-- ---------------------------------------------------------------------------------------------

-- name: StartMenuRead :one
-- Falla con 23505 si ya hay una en curso para esa tienda: lo impide el índice único parcial de la
-- 0071, no el chequeo previo del servicio, que es dos statements y se puede colar entre ambos.
insert into platform_menu_reads (connection_id) values ($1) returning id, started_at;

-- name: FailStaleRunningReads :execrows
-- Cierra las lecturas que quedaron en curso porque el proceso se cayó a media descarga.
--
-- SIN ESTO NO HAY SALIDA: `HasRunningMenuRead` sigue viéndolas, cada intento futuro responde «ya
-- hay una lectura en curso», y la pantalla dice «se está leyendo» para siempre — solo se arregla
-- tocando la base a mano. Corre al arrancar y con la poda.
update platform_menu_reads
   set status = 'fallida', finished_at = now(), failure_kind = 'tiempo_agotado'
 where status = 'en_curso' and started_at < $1;

-- name: FinishMenuReadOK :exec
update platform_menu_reads
   set status = 'ok', finished_at = now(), item_count = $2
 where id = $1;

-- name: FailMenuRead :exec
-- `failure_kind` es una clase cerrada, nunca el mensaje de la plataforma: ahí cabe un secreto.
update platform_menu_reads
   set status = 'fallida', finished_at = now(), failure_kind = $2
 where id = $1;

-- name: GetLastMenuRead :one
-- La última lectura de la conexión, EN CUALQUIER ESTADO. Es lo que permite distinguir los tres
-- casos que una pantalla mal hecha muestra igual: nunca leída (sin fila), fallida, y sin
-- diferencias. Devolver solo las 'ok' escondería una lectura rota detrás de una vieja buena.
select id, started_at, finished_at, status, item_count, failure_kind
from platform_menu_reads
where connection_id = $1
order by started_at desc
limit 1;

-- name: GetLastOKMenuRead :one
-- La última que sí sirvió, que es contra la que se compara y se empareja.
select id, started_at, finished_at, item_count
from platform_menu_reads
where connection_id = $1 and status = 'ok'
order by started_at desc
limit 1;

-- name: ListMenuReads :many
select id, started_at, finished_at, status, item_count, failure_kind
from platform_menu_reads
where connection_id = $1
order by started_at desc
limit $2;

-- name: HasRunningMenuRead :one
-- Dos lecturas simultáneas de la misma tienda gastan dos tokens para escribir la misma foto, y
-- Uber invalida el más viejo a partir del 101 en una hora.
select exists (
  select 1 from platform_menu_reads where connection_id = $1 and status = 'en_curso'
);

-- name: PruneMenuReads :execrows
-- Poda por retención. **Conserva SIEMPRE la más reciente de cada conexión**, sin importar su edad:
-- sin esa excepción, una tienda que nadie vuelve a leer se queda sin ninguna fila y la pantalla la
-- muestra igual que una que nunca se leyó — dos estados que FR-003 exige distinguir.
delete from platform_menu_reads r
where r.started_at < $1
  and r.id <> (
    select r2.id from platform_menu_reads r2
    where r2.connection_id = r.connection_id
    order by r2.started_at desc limit 1
  );

-- ---------------------------------------------------------------------------------------------
-- La foto
-- ---------------------------------------------------------------------------------------------

-- name: InsertMenuItem :exec
insert into platform_menu_items (read_id, external_id, kind, name, price_cents, available)
values ($1, $2, $3, $4, $5, $6);

-- name: ListMenuItemsOfRead :many
select external_id, kind, name, price_cents, available
from platform_menu_items
where read_id = $1 and kind = $2
order by name;

-- name: MenuItemExistsInRead :one
-- Sustituye a la FK que `platform_item_links` deliberadamente NO tiene. Sin esta validación se
-- puede guardar una pareja contra un id que no existe arriba: el PUT responde 200 y la pantalla
-- sigue diciendo «sin pareja», sin que nadie vea un error.
select exists (
  select 1 from platform_menu_items where read_id = $1 and external_id = $2 and kind = $3
);

-- ---------------------------------------------------------------------------------------------
-- Emparejamiento
-- ---------------------------------------------------------------------------------------------

-- name: ListItemLinks :many
-- `confirmed_at` viaja hasta la pantalla y no es adorno: sin él, «deshacer el último» tiene que
-- adivinar cuál fue, y la lista llega ordenada por NOMBRE. Adivinar ahí no falla ruidoso — deshace
-- otra pareja.
select external_id, kind, product_id, local_kind, confirmed_at, confirmed_by
from platform_item_links
where connection_id = $1;

-- name: GetItemLink :one
-- Con qué está emparejado hoy ese item. Se consulta ANTES de escribir: el upsert pisa en silencio
-- una pareja confirmada, y eso es trabajo manual que no se reconstruye.
select external_id, kind, product_id, local_kind, confirmed_at
from platform_item_links
where connection_id = $1 and external_id = $2;

-- name: UpsertItemLink :exec
insert into platform_item_links (connection_id, external_id, kind, product_id, local_kind, confirmed_at, confirmed_by)
values ($1, $2, $3, $4, $5, now(), $6)
on conflict (connection_id, external_id)
do update set product_id    = excluded.product_id,
              kind          = excluded.kind,
              local_kind    = excluded.local_kind,
              confirmed_at  = excluded.confirmed_at,
              confirmed_by  = excluded.confirmed_by;

-- name: DeleteItemLink :execrows
delete from platform_item_links where connection_id = $1 and external_id = $2;

-- ---------------------------------------------------------------------------------------------
-- El lado del POS de la comparación
-- ---------------------------------------------------------------------------------------------

-- name: ListActiveProductsForCompare :many
-- Nombre y precio base de lo que el negocio vende hoy. El precio POR PLATAFORMA no sale de aquí:
-- se calcula con el markup y las excepciones, igual que en el menú del POS, para no duplicar esa
-- regla en dos lugares que se desincronizan.
select id, name, price from products where is_active order by name;

-- name: ListActiveOptionsForCompare :many
-- `g.is_active` ADEMÁS de `o.is_active`, igual que la consulta que arma el menú del POS. Desactivar
-- el grupo entero es la forma normal de ocultar algo estacional sin tocar opción por opción; sin
-- este filtro, esas opciones se siguen reportando como vendidas y salen como diferencia contra un
-- menú que ya no las ofrece.
select o.id, o.name, o.price_delta
from modifier_options o
join modifier_groups g on g.id = o.group_id
where o.is_active and g.is_active
order by o.name;
