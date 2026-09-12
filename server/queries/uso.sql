-- EL USO DEL SISTEMA (spec 017).
--
-- Dos caminos que no se cruzan: el POS ESCRIBE (las tres primeras, con el rol de la aplicación) y
-- la consola LEE (la última, con el rol de plataforma, que no tiene permiso sobre el grano fino).

-- name: InsertUsageEvent :exec
-- El grano fino, un hecho por fila. Sin quién lo hizo: la tabla no tiene esa columna.
--
-- `detail` NO se recibe de nadie: existe para las coordenadas del futuro (FR-013) y se queda nulo.
-- Aceptarlo desde el cuerpo del POST convertiría esa puerta en un campo libre.
--
-- ponytail: una fila por evento, hasta 50 por lote y por transacción. A 3 pedidos/día medidos (y
-- 100 en el escenario de diseño) son milisegundos; si algún día un cliente mete miles de eventos
-- por minuto, esto pasa a un `copy`.
insert into usage_events (screen, action, role)
values ($1, $2, $3);

-- name: UpsertUsageDaily :exec
-- El conteo del día. Recibe el lote YA PRE-AGREGADO: suma `excluded.hits`, no `+1`.
--
-- Cada `update` deja la versión vieja de la fila muerta, y estas filas son pocas y calientes:
-- medido, 135 filas con 2,000 incrementos de a uno pasan de 64 kB a 232 kB antes de que autovacuum
-- llegue. Pre-agregar convierte hasta 50 escrituras físicas en una.
insert into usage_daily (day, screen, action, role, hits)
values (current_date, $1, $2, $3, $4)
on conflict on constraint usage_daily_llave
do update set hits = usage_daily.hits + excluded.hits;

-- name: CountActiveUsersByRole :one
-- Cuántas personas activas tiene ese rol en la empresa del request (RLS acota la consulta).
--
-- Es lo que decide si el rol se guarda o se deja en blanco: con menos de dos, escribirlo es escribir
-- un nombre. Se pregunta al ESCRIBIR porque es el único momento en que la decisión no se puede
-- deshacer — y porque quien lee no podría: la consola no tiene permiso sobre `users`.
select count(*)::bigint from users where role = $1 and is_active;

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

-- name: DeleteOldUsageEvents :execrows
-- El recorte del grano fino. Corre con conexión de DUEÑO: el rol de la aplicación está bajo RLS y
-- solo borraría lo de su empresa, dejando el recorte a medias sin que nada fallara.
delete from usage_events where occurred_at < now() - ($1::int * interval '1 day');

-- name: DeleteOldUsageDaily :execrows
-- Y el del agregado, con su propia retención (más larga: es lo que se mira).
delete from usage_daily where day < current_date - ($1::int * interval '1 day');
