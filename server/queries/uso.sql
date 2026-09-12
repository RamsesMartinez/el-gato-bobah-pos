-- EL USO DEL SISTEMA (spec 017).
--
-- Dos caminos que no se cruzan: el POS ESCRIBE con el rol de la aplicación y la consola LEE con el
-- de plataforma. Una sola tabla: el conteo. Ver el porqué en la migración 0069 — un renglón por
-- toque se podía cruzar con `register_sessions` por la marca de tiempo y deshacía el anonimato.

-- name: UpsertUsageDaily :exec
-- El conteo del día. Recibe el lote YA PRE-AGREGADO: suma `excluded.hits`, no `+1`.
--
-- El DÍA viene calculado desde Go con la zona del negocio, no de `current_date`: con el servidor en
-- UTC la medianoche cae a las 18:00 en México y todo lo de la tarde-noche —la franja de más
-- movimiento— se contaría mañana. Es el mismo defecto que 0038 arregló para la venta.
--
-- Cada `update` deja la versión vieja de la fila muerta, y estas filas son pocas y calientes:
-- medido, 135 filas con 2,000 incrementos de a uno pasan de 64 kB a 232 kB antes de que autovacuum
-- llegue. Pre-agregar convierte hasta 50 escrituras físicas en una.
insert into usage_daily (day, screen, action, role, hits)
values ($1, $2, $3, $4, $5)
on conflict on constraint usage_daily_llave
do update set hits = usage_daily.hits + excluded.hits;

-- name: CountActiveUsersByRoleAll :many
-- Cuántas personas activas tiene CADA rol en la empresa del request (RLS acota la consulta).
--
-- La plantilla ENTERA y no solo la del rol que mide, y eso lo cambió una auditoría: la supresión se
-- decide rol por rol, pero todo lo suprimido cae en el mismo balde `role is null`. Cuando
-- exactamente un rol queda por debajo del umbral, ese balde ES esa persona —con 1 admin, 2
-- gerentes, 3 cajeros y 2 meseros, `null` es el dueño— y la consola lo pinta como «sin corte», que
-- promete lo contrario. Para saberlo hay que ver a todos.
--
-- Se pregunta al ESCRIBIR porque es el único momento en que la decisión no se puede deshacer — y
-- porque quien lee no podría: la consola no tiene permiso sobre `users`.
select role, count(*)::bigint as activos
from users
where is_active
group by role;

-- name: SumUsageForMap :many
-- Lo que lee la consola: el conteo del periodo por pantalla, acción y rol.
--
-- Suma en la base y no en Go: son decenas de filas por día y por empresa, pero un año de 13 meses
-- por varias empresas son decenas de miles, y no hay razón para moverlas.
--
-- `company` nulo = todas las empresas juntas (FR-008). Lo que hace posible ese "todas" es la
-- política `plataforma_lee_todo_el_uso`; sin ella esta misma consulta devuelve una empresa, o cero.
select screen,
       action,
       role,
       sum(hits)::bigint as veces
from usage_daily
where day between sqlc.arg('desde')::date and sqlc.arg('hasta')::date
  and (sqlc.narg('company')::bigint is null or company_id = sqlc.narg('company')::bigint)
group by screen, action, role
order by screen, action nulls first, role nulls first;

-- name: DeleteOldUsageDaily :execrows
-- Y el del agregado, con su propia retención (más larga: es lo que se mira).
delete from usage_daily where day < current_date - ($1::int * interval '1 day');
