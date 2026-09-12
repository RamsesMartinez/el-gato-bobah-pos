-- +goose Up

-- EL USO DEL SISTEMA (spec 017): cuántas veces se abre cada pantalla y cuántas veces se dispara
-- cada acción con nombre.
--
-- Dos tablas y no una: el grano fino es lo que mantiene abierta la puerta de las coordenadas del
-- toque (FR-013) y lo que permite recontar si mañana se cuenta distinto; el agregado es lo que hace
-- que el volumen no crezca (FR-010) y lo único que la consola lee.
--
-- LO QUE NO ESTÁ EN NINGUNA DE LAS DOS: quién fue. No hay `user_id`, ni FK a `users`, ni nada de
-- donde deducirlo. No es que la aplicación no lo escriba — es que no hay dónde. Lo que no existe no
-- se llena por descuido ni aparece en un `select *` dentro de seis meses.

-- Crea tablas nuevas y no toca ninguna viva, así que no debería esperar por nadie. Si aun así se
-- queda esperando un lock, es mejor fallar rápido que dejar el deploy colgado a media sustitución.
set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 1. El grano fino.
-- ---------------------------------------------------------------------------------------------

create table usage_events (
  id          bigint generated always as identity primary key,
  -- LA FECHA LA PONE EL SERVIDOR. El reloj de la tableta no manda, igual que en la venta (spec
  -- 008): una tableta con la fecha corrida escribiría eventos en un día que no ocurrió.
  occurred_at timestamptz not null default now(),
  -- De la lista blanca de `domain`. Lo que no está en la lista no llega hasta aquí.
  screen      text not null,
  -- NULO = fue una apertura de pantalla. Las aperturas y las acciones viven en la misma tabla
  -- porque son el mismo hecho: alguien hizo algo en una pantalla.
  action      text,
  -- NULO = sin corte. Cuando el rol tiene menos de dos usuarios activos en esa empresa, decir
  -- "el gerente hizo 40 acciones" es decir su nombre, así que no se escribe (FR-009). La supresión
  -- ocurre al ESCRIBIR y por eso no se puede deshacer leyendo.
  role        user_role,
  -- LA PUERTA DE FR-013, hoy siempre nula. El día que se midan coordenadas del toque entran aquí
  -- como {"x":…,"y":…} sin migrar una sola fila.
  --
  -- Y la cierra del lado de afuera el handler: el `detail` que venga en el cuerpo del POST se
  -- DESCARTA sin mirarlo. Mientras el cliente pueda escribir aquí, esta puerta es también un campo
  -- libre por donde entra lo que FR-003 prohíbe — y un jsonb con datos de más no avisa.
  detail      jsonb,
  company_id  bigint not null default current_setting('app.company_id', true)::bigint
              references companies(id) on delete cascade,

  -- La lista blanca de Go es la barrera buena, pero estas dos columnas reciben lo que venga en el
  -- cuerpo. Un control que solo vive en Go se rodea por otra ruta; uno en la columna, no.
  constraint usage_events_nombres_acotados check (
    char_length(screen) <= 40 and (action is null or char_length(action) <= 60))
);

-- Empieza por company_id: RLS agrega ese predicado a toda consulta del rol de la app, y un índice
-- que arranque por la fecha se queda descartando filas de otras empresas dentro del scan (ver 0042).
create index usage_events_company_fecha on usage_events (company_id, occurred_at);

-- ---------------------------------------------------------------------------------------------
-- 2. El agregado, que es lo único que lee la consola.
-- ---------------------------------------------------------------------------------------------

create table usage_daily (
  day        date not null,
  screen     text not null,
  action     text,
  role       user_role,
  hits       bigint not null default 0,
  company_id bigint not null default current_setting('app.company_id', true)::bigint
             references companies(id) on delete cascade,

  -- `NULLS NOT DISTINCT` NO ES UN DETALLE DE ESTILO.
  --
  -- `action` es nula en TODA apertura de pantalla y `role` es nulo en todo evento suprimido por
  -- k-anonimato, que juntos son la mayoría. En SQL `null <> null`, así que con una llave única
  -- normal cada uno de esos eventos crearía una FILA NUEVA en vez de sumar: el agregado dejaría de
  -- agregar en silencio y esta tabla crecería como la de eventos, que es justo lo que FR-010
  -- promete que no pasa. Es de Postgres 15+ y la base corre 16.
  constraint usage_daily_llave unique nulls not distinct (company_id, day, screen, action, role),
  constraint usage_daily_nombres_acotados check (
    char_length(screen) <= 40 and (action is null or char_length(action) <= 60)),
  -- Un conteo negativo solo puede venir de un bug de suma, y en silencio se lee como "se usó poco".
  constraint usage_daily_hits_no_negativo check (hits >= 0)
);

-- ---------------------------------------------------------------------------------------------
-- 3. Quién puede qué.
-- ---------------------------------------------------------------------------------------------

alter table usage_events enable row level security;
create policy tenant_isolation on usage_events
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

alter table usage_daily enable row level security;
create policy tenant_isolation on usage_daily
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- EL GRANT NO ES OPCIONAL: el de 0024 fue puntual (`on all tables`), sin default privileges, así que
-- una tabla creada después no hereda ni el select. Sin estas líneas la migración pasa, los tests
-- pasan y `make start` pasa —dev sirve como owner, sin RLS ni grants— y en producción el primer
-- evento devuelve 42501.
--
-- El POS solo ESCRIBE el grano fino: no lo lee nadie desde la aplicación.
grant insert on usage_events to gatobobah_app;

-- Y en el agregado necesita las tres: `insert` y `update` por el upsert, y `select` porque Postgres
-- lo exige para leer `hits` dentro del `set`. No es acceso de lectura para una pantalla del
-- negocio: no existe tal pantalla, el mapa es de la consola.
grant select, insert, update on usage_daily to gatobobah_app;

-- LA CONSOLA DE PLATAFORMA: solo el agregado, y solo para leer.
--
-- Ningún grant sobre `usage_events`, a propósito y para siempre: la consola mira conteos, no
-- hechos. El día que el grano fino lleve coordenadas del toque, seguirá sin alcanzarlo salvo que
-- alguien lo decida a propósito y lo escriba en una migración.
grant select on usage_daily to gatobobah_platform;

-- Y su política, por lo mismo que la de `companies` en la 0068: el grant SOLO no alcanza, porque
-- `tenant_isolation` también le aplica al rol de plataforma. Sin ella el mapa sale mal en las dos
-- formas posibles y ninguna falla: si la conexión no trae `app.company_id` —lo normal en
-- producción— ve CERO filas, y si lo trae heredado ve UNA EMPRESA DE VARIAS. Medido con el test.
--
-- Acotada a `select` y a ese rol; las políticas se suman, así que el rol del negocio sigue viendo
-- solo lo suyo.
create policy plataforma_lee_todo_el_uso on usage_daily
  for select to gatobobah_platform using (true);

-- +goose Down

-- Revertir PIERDE los conteos, y no se recuperan volviendo a aplicar la migración: el grano fino
-- también se va, así que no hay de dónde reconstruirlos. Lo que no se pierde es nada del negocio —
-- esta feature no toca un solo pedido ni un solo peso.
drop table if exists usage_daily;
drop table if exists usage_events;
