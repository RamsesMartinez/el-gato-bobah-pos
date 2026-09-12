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
-- UNA SOLA TABLA: el conteo. No hay tabla de eventos, y eso es una decisión, no un olvido.
-- ---------------------------------------------------------------------------------------------
--
-- El plan tenía dos tablas: ésta y una de grano fino, un renglón por toque, con `detail jsonb`
-- vacío para las coordenadas del futuro. La auditoría adversarial la tumbó por dos caminos
-- independientes, y los dos son del tipo que no se ve mirando:
--
-- 1. SE PODÍA DESHACER EL ANONIMATO CON EL RELOJ. Un renglón por toque con marca de microsegundos
--    se cruza con `register_sessions.closed_by` o con `orders.opened_by`: el evento "sin corte" de
--    las 22:03:11.412 es de quien cerró el turno a las 22:03:11.6. Y como los eventos de un turno
--    son un flujo contiguo de la misma tableta, esa sesión etiqueta TODO lo que cayó en medio. El
--    identificador no era el rol: era el reloj. Suprimir el rol no protegía de nada.
-- 2. EL VOLUMEN NO TENÍA TECHO REAL. Con el limitador intacto, una cuenta puede mandar 2.16
--    millones de eventos al día: medido, 298 MB diarios, 4 GB en los 14 días de retención — 167×
--    el techo declarado. Un bucle en una tableta llenaba el disco del VPS y Postgres se detenía,
--    o sea: el mostrador dejaba de cobrar por una feature de analítica.
--
-- Con solo el conteo, las dos desaparecen por construcción: no hay instante que cruzar, y el
-- número de FILAS está acotado por la lista blanca (pantallas × acciones × roles), sin importar
-- cuántos eventos lleguen. Lo que crece es un contador, no la tabla.
--
-- Y la puerta de FR-013 sigue abierta al mismo costo: el día que se midan coordenadas, nace una
-- tabla nueva para ellas. Crear una tabla no migra nada — que es literalmente lo que el requisito
-- pide. Lo que se pierde es poder recontar distinto los últimos 14 días, y no vale una tabla que
-- hoy nadie lee y que solo el recorte toca.

create table usage_daily (
  -- EL DÍA DEL NEGOCIO, calculado en Go con la zona de la empresa, no `current_date`.
  --
  -- Con el servidor en UTC la medianoche cae a las 18:00 en México: todo lo que pasa de las 6pm en
  -- adelante —la franja donde más se mueve un lugar de comida— se contaría en el día SIGUIENTE.
  -- Es el mismo defecto que 0038 ya corrigió para la venta (ver domain.BusinessDate); repetirlo
  -- aquí habría dado un mapa que miente de noche y acierta de día.
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

-- El único índice de la tabla es el de la llave única, y empieza por `company_id`: le sirve al
-- filtro de UNA empresa y no al de TODAS, que es el default del mapa. Sin éste, pedir "todas" con
-- 13 meses encima recorre la tabla entera.
create index usage_daily_dia on usage_daily (day);

-- ---------------------------------------------------------------------------------------------
-- Quién puede qué.
-- ---------------------------------------------------------------------------------------------

alter table usage_daily enable row level security;
create policy tenant_isolation on usage_daily
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- EL GRANT NO ES OPCIONAL: el de 0024 fue puntual (`on all tables`), sin default privileges, así que
-- una tabla creada después no hereda ni el select. Sin estas líneas la migración pasa, los tests
-- pasan y `make start` pasa —dev sirve como owner, sin RLS ni grants— y en producción el primer
-- evento devuelve 42501.
--
-- El POS necesita las tres: `insert` y `update` por el upsert, y `select` porque Postgres
-- lo exige para leer `hits` dentro del `set`. No es acceso de lectura para una pantalla del
-- negocio: no existe tal pantalla, el mapa es de la consola.
grant select, insert, update on usage_daily to gatobobah_app;

-- LA CONSOLA DE PLATAFORMA: solo el agregado, y solo para leer.
--
-- Conteos, que es todo lo que hay. El día que nazca una tabla de coordenadas, alcanzarla será una
-- decisión que alguien tenga que escribir en una migración, no algo que herede.
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

-- Revertir PIERDE los conteos y no se recuperan volviendo a aplicar la migración: no hay grano fino
-- del cual reconstruirlos, a propósito. Lo que no se pierde es nada del negocio — esta feature no
-- toca un solo pedido ni un solo peso.
drop table if exists usage_daily;
