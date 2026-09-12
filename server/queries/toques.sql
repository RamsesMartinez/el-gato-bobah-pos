-- DÓNDE CAE EL DEDO (spec 019).
--
-- Gemelas de las de la 017 y por las mismas razones. Lo que cambia: la celda y la orientación, que
-- **nunca se suman entre sí** (FR-016) porque la celda 37 es otro lugar en cada forma de pantalla.

-- name: UpsertTouchesDaily :exec
-- El conteo de una zona. Recibe el lote YA PRE-AGREGADO desde Go: suma `excluded.hits`, no `+1`.
--
-- Es lo que acota el volumen por construcción. Mil toques en la misma celda durante un turno son
-- una fila y un contador, no mil renglones — y pre-agregar convierte además esas mil escrituras
-- físicas en una, que es lo que mantiene la tabla chica entre pasadas de autovacuum.
--
-- El DÍA viene calculado en Go con la zona del negocio, nunca de `current_date`: con el servidor en
-- UTC la medianoche cae a las 18:00 en México y toda la tarde-noche se contaría mañana.
insert into usage_touches_daily (day, screen, orientation, cell, role, hits)
values ($1, $2, $3, $4, $5, $6)
on conflict on constraint usage_touches_llave
do update set hits = usage_touches_daily.hits + excluded.hits;

-- name: SumTouchesForGrid :many
-- Lo que lee la consola: cuántos toques cayeron en cada celda de UNA pantalla en UNA orientación.
--
-- La orientación es parámetro obligatorio y no un `group by`: mezclarlas pintaría una rejilla que
-- nadie tocó nunca. Pedir una devuelve solo la suya.
--
-- **Devuelve solo las celdas con conteo**, no las 84. Las que faltan valen cero y las rellena quien
-- arma la respuesta: traer 84 filas de las cuales 60 son ceros es mover nada por la red, y el cero
-- lo sabe el servidor sin preguntarlo.
--
-- `company` nulo = todas las empresas juntas, que aquí es el caso interesante: el layout es el mismo
-- software para todos, así que juntar tabletas da mejor muestra para decidir dónde va un control.
-- Lo hace posible la política `plataforma_lee_todos_los_toques`; sin ella devuelve cero filas sin
-- fallar.
select cell,
       role,
       sum(hits)::bigint as veces
from usage_touches_daily
where day between sqlc.arg('desde')::date and sqlc.arg('hasta')::date
  and screen = sqlc.arg('screen')::text
  and orientation = sqlc.arg('orientation')::text
  and (sqlc.narg('company')::bigint is null or company_id = sqlc.narg('company')::bigint)
group by cell, role
order by cell, role nulls first;

-- name: DeleteOldTouchesDaily :execrows
-- El recorte, con su PROPIA retención y no la del agregado.
--
-- Son 92 días contra los 396 de `usage_daily`, y la diferencia es deliberada: el conteo por pantalla
-- se mira año contra año —«¿se usa más el corte de caja que la temporada pasada?»— mientras que la
-- rejilla solo sirve para decidir un rediseño, y una rejilla de hace un año describe un layout que
-- ya no existe. Guardarla más tiempo es guardar una referencia que miente.
delete from usage_touches_daily where day < current_date - ($1::int * interval '1 day');
