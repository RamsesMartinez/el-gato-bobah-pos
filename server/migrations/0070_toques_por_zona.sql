-- +goose Up

-- DÓNDE CAE EL DEDO (spec 019): cuántos toques cayeron en cada zona de una pantalla.
--
-- Es la mitad que la spec 017 dejó aplazada a propósito, y nace con las dos lecciones que aquélla
-- pagó en su auditoría:
--
-- 1. NO HAY FILA POR TOQUE. Un renglón por evento con su marca de tiempo se cruza con
--    `register_sessions.closed_by` o con `orders.opened_by` y etiqueta al empleado aunque su nombre
--    no esté: el identificador no es el rol, es el reloj. Aquí no hay instante que cruzar.
-- 2. NO HAY COLUMNA DE TIEMPO MÁS FINA QUE EL DÍA, ni siquiera un `updated_at`. Esa fila se toca
--    con cada lote; su hora de última escritura diría a qué hora estuvo activa esa zona, que es
--    medio camino de vuelta.
--
--    **Con una salvedad que hay que decir**: `xmin` es una columna de SISTEMA y se lee con el mismo
--    `select`. Como cada lote hace `on conflict do update`, el `xmin` de una fila es el
--    identificador de transacción del último lote que tocó esa zona — sondeándolo cada minuto se
--    reconstruye qué zonas se usaron en el último minuto. No se puede quitar; lo que se puede es no
--    mentir sobre ello. Hoy no cruza una frontera de confianza real —quien tiene la credencial de
--    plataforma tiene también la del dueño, las dos viven en el mismo `deploy/.env`— pero el grant
--    de esta tabla trae ese canal incluido, y el día que la consola corra con credenciales
--    separadas hay que contarlo.
--
-- Y el volumen queda acotado POR CONSTRUCCIÓN: mil toques en la misma zona suben un contador. Las
-- filas las fija la rejilla —84 celdas × 2 orientaciones × 5 cortes de rol × las pantallas
-- instrumentadas— sin importar cuántos dedos lleguen.

set local lock_timeout = '3s';

create table usage_touches_daily (
  -- El día del NEGOCIO, calculado en Go con la zona de la empresa. Con el servidor en UTC la
  -- medianoche cae a las 18:00 en México y la tarde-noche se contaría mañana (ver 0038 y 0069).
  day         date not null,
  screen      text not null,
  -- `horizontal` o `vertical`. La misma celda es OTRO LUGAR en cada forma, así que nunca se suman
  -- (FR-016): son filas distintas y la consola pide una.
  orientation text not null,
  -- El número de celda, ya calculado EN LA TABLETA. Aquí nunca llega un (x, y): un punto con
  -- precisión de píxel que viaja existe en el cuerpo del request, en el log de un proxy y en la
  -- memoria del servidor aunque después se redondee. Redondear en el origen es lo único que hace
  -- que el punto exacto no exista en ningún lado.
  --
  -- La rejilla es 12×7 en horizontal y 7×12 en vertical: 84 celdas en las dos, de unos 85×86 px en
  -- una tableta de 1024×600 — el tamaño de un botón. Más fina sería un mapa de puntos con otro
  -- nombre; y volver a una más fina después es IMPOSIBLE, porque el toque fino no se guarda.
  cell        smallint not null,
  -- Nulo = sin corte, con la misma regla de k-anonimato que la 017.
  role        user_role,
  hits        bigint not null default 0,
  company_id  bigint not null default current_setting('app.company_id', true)::bigint
              references companies(id) on delete cascade,

  -- `nulls not distinct` por lo mismo que en 0069: `role` es nulo en todo lo suprimido, y sin esta
  -- cláusula cada evento suprimido crearía una fila nueva en vez de sumar.
  constraint usage_touches_llave
    unique nulls not distinct (company_id, day, screen, orientation, cell, role),

  constraint usage_touches_celda_en_rango check (cell between 0 and 83),
  -- EL QUE MENOS SE NOTA Y MÁS DUELE: sin él, una versión vieja de la tableta que mande 'landscape'
  -- crea un BALDE INVISIBLE. La fila entra, pasa el rango de celda —0..83 vale en las dos formas— y
  -- la consola, que solo pide 'horizontal' y 'vertical', nunca la muestra. Los toques de esa zona
  -- desaparecen sin un solo error.
  constraint usage_touches_orientacion check (orientation in ('horizontal', 'vertical')),
  -- Los otros dos son los de la tabla gemela, que se habían caído al copiar el patrón: un conteo
  -- negativo solo puede venir de un bug de suma y en silencio se lee como "nadie tocó ahí".
  constraint usage_touches_hits_no_negativo check (hits >= 0),
  constraint usage_touches_pantalla_acotada check (char_length(screen) <= 40)
)
-- `fillfactor` bajo porque estas filas se ACTUALIZAN muchas veces al día —una por cada lote que cae
-- en esa celda— y cada `update` deja muerta la versión vieja. Dejar espacio libre en la página
-- permite que la versión nueva quepa al lado (actualización HOT) sin ensuciar el índice. Medido en
-- la 017: sin esto, las filas calientes de un día crecen 3.6× antes de que autovacuum llegue.
with (fillfactor = 70);

-- ESTE ÍNDICE SIRVE LAS DOS LECTURAS DE LA CONSOLA, no solo la de «todas las empresas». Medido con
-- `EXPLAIN` sobre el plan GENÉRICO —el que pgx usa a partir de la quinta ejecución del mismo
-- statement, o sea el caso normal de un endpoint—: el filtro opcional por empresa viaja como
-- `($5 is null or company_id = $5)`, que el planificador no puede convertir en condición de índice,
-- así que entra por `day` y filtra el resto. La llave única NUNCA sirve un `select`: es solo
-- unicidad y `on conflict`.
--
-- Se dice aquí porque el comentario anterior invitaba al error contrario —«para una empresa ya está
-- la llave»—: quitando este índice, la consulta de UNA empresa cae a `Parallel Seq Scan`, y ningún
-- test fija el plan.
create index usage_touches_daily_dia on usage_touches_daily (day);

alter table usage_touches_daily enable row level security;
create policy tenant_isolation on usage_touches_daily
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- El grant no es opcional: el de 0024 fue puntual y una tabla nueva no hereda nada. `select` va
-- porque el `upsert` lee `hits` dentro del `set`.
grant select, insert, update on usage_touches_daily to gatobobah_app;

-- La consola: solo leer.
grant select on usage_touches_daily to gatobobah_platform;

-- Y su política, por lo mismo que en 0068 y 0069: el grant SOLO no alcanza. `tenant_isolation`
-- también le aplica al rol de plataforma, y su conexión no fija `app.company_id` — sin esta línea
-- la rejilla sale vacía SIN QUE NADA FALLE, que es la peor forma de fallar.
create policy plataforma_lee_todos_los_toques on usage_touches_daily
  for select to gatobobah_platform using (true);

-- +goose Down

-- Revertir pierde los conteos y no se recuperan: no hay grano fino del cual reconstruirlos, a
-- propósito. Nada del negocio se pierde — esta feature no toca un pedido ni un peso.
drop table if exists usage_touches_daily;
