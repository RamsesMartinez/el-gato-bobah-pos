-- +goose Up

-- CONTEO DE EFECTIVO POR DENOMINACIONES (spec 003).
--
-- El operador cuenta el cajón y captura un solo número; la suma la hace en una libreta o en la
-- calculadora del teléfono. Ese paso es la fuente de los faltantes: un billete mal sumado se
-- vuelve una diferencia que después nadie puede explicar. A partir de aquí se capturan PIEZAS y el
-- servidor suma.
--
-- El total sigue llegando a las columnas de siempre —`register_sessions.opening_cash` y
-- `register_session_totals.declared`—, así que el corte, los reportes y la columna generada
-- `difference` no se tocan. Estas tablas dicen de DÓNDE salió ese número, no lo reemplazan.
--
-- Va en una sola migración y no en dos como proponía el plan: los renglones referencian el
-- catálogo, así que sus `Down` solo funcionan en un orden. Dos migraciones que siempre se aplican
-- juntas y que no se pueden revertir por separado son una migración partida en dos.

-- lock_timeout: la migración crea tablas nuevas y no toca ninguna viva, así que no debería esperar
-- por nadie. Si aun así se queda esperando un lock, es que algo raro pasa y es mejor fallar rápido
-- que dejar el deploy colgado con la API a medio sustituir.
set local lock_timeout = '3s';

-- Los dos momentos en que se cuenta el cajón.
--
-- Va como `create type` suelto y NO dentro de un `do $$ ... $$`: el parser de sqlc no lee DDL
-- dinámico —es lo mismo que le pasa al `EXECUTE format()` de 0023 con company_id—, así que dentro
-- del bloque el tipo existe en Postgres y no existe para el código generado. goose no reaplica una
-- migración, así que la guarda de "si no existe" no hacía falta.
create type cash_count_moment as enum ('apertura', 'cierre');

-- EL CATÁLOGO. Qué piezas de dinero existen en cada moneda.
--
-- Es la primera tabla del sistema SIN `company_id` y sin RLS, y por eso conviene decirlo aquí: los
-- billetes de México son los mismos para todos los negocios, así que no hay nada que aislar. Lo
-- que sí se valida en el servicio es que la denominación pertenezca a la MONEDA de la sesión.
--
-- Que siga global no cierra la puerta a partirla por empresa el día que un cliente necesite su
-- propio catálogo: `payment_methods` nació así en 0002 y 0037 lo partió duplicando filas, al mismo
-- costo que si hubiera nacido partido.
create table cash_denominations (
  id        bigint generated always as identity primary key,
  currency  char(3) not null,
  -- numeric y no un entero de centavos: es el mismo tipo con el que el resto del sistema maneja
  -- dinero, y mezclar las dos escalas es cómo nace un error de factor 100.
  value     numeric(10,2) not null,
  -- Solo sirve para agrupar en pantalla, pero agrupar monedas y billetes es lo que hace que el
  -- operador encuentre la pieza que trae en la mano.
  is_coin   boolean not null,
  -- De mayor a menor, que es como se cuenta un cajón: primero los billetes grandes.
  sort_key  int not null,
  -- Una denominación se retira de circulación con esto, no borrándola: los arqueos que la usaron
  -- tienen que seguir mostrando qué se contó.
  is_active boolean not null default true,

  -- Dos filas con el mismo valor serían dos botones para la misma pieza, y el operador contaría en
  -- una y luego en la otra sin notarlo.
  constraint cash_denominations_unicas unique (currency, value),
  -- Una pieza que no vale dinero rompe en silencio toda suma que dependa de que cada renglón
  -- aporta algo. Un 0 sembrado por error ocuparía un botón y no sumaría nada.
  constraint cash_denominations_valor_positivo check (value > 0)
);

-- MXN: monedas de 0.50, 1, 2, 5 y 10; billetes de 20, 50, 100, 200, 500 y 1000.
--
-- Fuera quedan las de 10¢ y 20¢ —prácticamente no circulan— y la moneda de $20, que existe pero es
-- conmemorativa y rara en caja. Si aparece una, se captura por el camino del total manual: once
-- botones que el operador tiene que saltarse todos los días cuestan más que el caso raro.
insert into cash_denominations (currency, value, is_coin, sort_key) values
  ('MXN', 1000, false, 1),
  ('MXN',  500, false, 2),
  ('MXN',  200, false, 3),
  ('MXN',  100, false, 4),
  ('MXN',   50, false, 5),
  ('MXN',   20, false, 6),
  ('MXN',   10, true,  7),
  ('MXN',    5, true,  8),
  ('MXN',    2, true,  9),
  ('MXN',    1, true, 10),
  ('MXN',  0.5, true, 11);

-- EL ROL DE LA APP LEE EL CATÁLOGO, Y SOLO LEE.
--
-- El grant de LECTURA no es opcional: el de 0024 fue `on all tables in schema public`, que es
-- PUNTUAL —no hay default privileges—, así que una tabla creada después no hereda ni el select. Sin
-- esta línea la pantalla de conteo recibe 42501 en producción, donde la API sí corre con este rol,
-- mientras en dev funciona porque ahí se conecta como owner.
grant select on cash_denominations to gatobobah_app;

-- Y el revoke de escritura, explícito aunque hoy sea redundante: esta tabla NO tiene company_id, así
-- que un endpoint futuro mal filtrado —o un `grant all` de mañana— que apague el billete de $1000
-- para un negocio que no lo acepta lo apagaría para TODAS las empresas de la base. Mientras siga
-- global, el catálogo se cambia como operación deliberada de owner, igual que `units`.
revoke insert, update, delete on cash_denominations from gatobobah_app;

-- EL CONTEO. Cuántas piezas se contaron en un momento del turno.
create table session_cash_counts (
  id            bigint generated always as identity primary key,
  session_id    bigint not null,
  moment        cash_count_moment not null,
  -- DERIVADO, PERO GUARDADO. Recalcularlo desde el catálogo exigiría que los valores nunca
  -- cambiaran, y una denominación retirada rompería los arqueos viejos. Es el mismo snapshot que
  -- `order_lines.unit_price`: el pasado deja de reescribirse cuando alguien edita el catálogo.
  total         numeric(10,2) not null,
  -- Por qué se capturó el total a mano en vez de contar. Nulo = se contó por denominaciones.
  -- Es lo que hace auditable un arqueo sin desglose: FR-016 exige que todo arqueo tenga una cosa
  -- o la otra, nunca una cifra suelta.
  manual_reason text,
  created_by    bigint not null references users(id),
  created_at    timestamptz not null default now(),
  company_id    bigint not null default current_setting('app.company_id', true)::bigint
                references companies(id) on delete cascade,

  -- Un turno tiene un conteo de apertura y uno de cierre, no más. Es lo que impide que un segundo
  -- cierre pise el del primero — y con dos tabletas compartiendo cuenta eso no es hipotético. El
  -- servicio traduce este rechazo a un mensaje que dice qué pasó, no a un 500.
  constraint session_cash_counts_un_momento unique (session_id, moment),
  -- No previene duplicados: existe para ser el destino de la FK compuesta de los renglones, igual
  -- que `products_id_company_key` en 0040.
  constraint session_cash_counts_id_company_key unique (id, company_id),

  -- FK COMPUESTA. Los chequeos de integridad referencial de Postgres SALTAN RLS por diseño, así que
  -- una FK simple aceptaría sin protestar un conteo cuyo company_id es de una empresa y cuyo
  -- session_id es de otra: la fila quedaría invisible para las dos y el turno ajeno aparecería con
  -- piezas que nadie contó ahí. Es la razón textual de 0041 y 0061.
  constraint session_cash_counts_session_fkey
    foreign key (company_id, session_id) references register_sessions (company_id, id)
    on delete cascade,

  -- FR-016 en el esquema, no solo en el servicio: o hay desglose, o hay motivo. Un conteo con total
  -- y sin renglones tiene que decir por qué; el servicio lo exige, y esto lo hace imposible de
  -- saltar desde un data-fix.
  constraint session_cash_counts_motivo_acotado check (
    manual_reason is null
    or (manual_reason = btrim(manual_reason) and length(manual_reason) between 1 and 200)
  ),
  constraint session_cash_counts_total_no_negativo check (total >= 0)
);

-- LOS RENGLONES. Cuántas piezas de cada denominación.
create table session_cash_count_lines (
  id              bigint generated always as identity primary key,
  count_id        bigint not null,
  -- SIN `on delete cascade`, a propósito y explícito: el default de Postgres es NO ACTION, que es lo
  -- correcto, pero heredarlo por omisión invita a copiar el `cascade` del renglón de arriba. El
  -- catálogo se retira con `is_active`; si alguien BORRA una fila —limpiando una semilla mal cargada
  -- al agregar otra moneda— un cascade se llevaría en silencio piezas de un arqueo ya firmado.
  -- Dinero contado y declarado no desaparece por una limpieza de catálogo.
  denomination_id bigint not null references cash_denominations(id) on delete restrict,
  -- SOLO SE GUARDA LO QUE HAY. Una denominación en cero no genera renglón: "no hay" y "no se
  -- capturó" son lo mismo en un arqueo, y once ceros por conteo son ruido que hay que filtrar al
  -- leer. FR-009 acepta el cero de ENTRADA; el filtro ocurre antes de escribir.
  pieces          int not null check (pieces > 0),
  company_id      bigint not null default current_setting('app.company_id', true)::bigint
                  references companies(id) on delete cascade,

  constraint session_cash_count_lines_unicas unique (count_id, denomination_id),
  -- Compuesta por lo mismo que la de arriba: un renglón de la empresa A colgado del conteo de B
  -- sumaría piezas ajenas a un arqueo, y ninguna consulta bajo RLS lo vería.
  constraint session_cash_count_lines_count_fkey
    foreign key (company_id, count_id) references session_cash_counts (id, company_id)
    on delete cascade
);

-- El índice de soporte empieza por company_id: RLS agrega ese predicado a toda consulta del rol
-- `gatobobah_app`, y un índice que arranque por session_id se queda descartando filas de otras
-- empresas dentro del scan (ver 0042).
create index session_cash_counts_company on session_cash_counts (company_id, session_id);

alter table session_cash_counts enable row level security;
create policy tenant_isolation on session_cash_counts
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

alter table session_cash_count_lines enable row level security;
create policy tenant_isolation on session_cash_count_lines
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- EL GRANT NO ES OPCIONAL: el de 0024 fue puntual (`on all tables`), sin default privileges, así que
-- cada tabla creada después necesita el suyo. Sin esto la migración pasa, los tests pasan y
-- `make start` pasa —dev sirve como owner, sin RLS ni grants— y en producción el primer request
-- devuelve 42501.
--
-- SIN `update` NI `delete`: un arqueo firmado no se edita ni se borra desde la app. Corregir un
-- conteo mal capturado es una decisión de dueño, no un botón.
grant select, insert on session_cash_counts to gatobobah_app;
grant select, insert on session_cash_count_lines to gatobobah_app;

-- +goose Down

-- En orden inverso de FK: los renglones apuntan al conteo y al catálogo, el conteo al catálogo de
-- momentos. Revertir esto NO pierde nada que no se pueda volver a contar: el total de cada arqueo
-- vive en `register_sessions.opening_cash` y en `register_session_totals.declared`, que esta
-- migración nunca tocó. Lo que se pierde es el DESGLOSE, o sea la capacidad de explicar un faltante
-- viejo — y eso no se recupera volviendo a aplicar la migración.
drop table if exists session_cash_count_lines;
drop table if exists session_cash_counts;
drop table if exists cash_denominations;
drop type if exists cash_count_moment;
