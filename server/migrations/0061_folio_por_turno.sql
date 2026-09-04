-- +goose Up
-- EL FOLIO SE CUENTA POR TURNO. LA FECHA SE CUENTA APARTE, Y NINGUNO LEE AL OTRO.
--
-- Hasta aquí las dos cosas colgaban de `orders.business_date`: el contador vivía en
-- `order_counters` con PK (company_id, business_date), y la venta HEREDABA esa fecha del turno
-- abierto. Esa herencia resolvía la numeración de un turno nocturno —abre 11pm, cierra 3am, numera
-- corrido— pero no tenía techo: un turno que nadie cerraba seguía estampando su fecha días después.
-- Medido el 2026-09-04 en el ambiente de pruebas: 158 pedidos y $6,664 archivados como 31 de
-- agosto, con la pantalla de Ventas del día saliendo vacía mientras el negocio vendía.
--
-- Separarlos deja arreglar la fecha sin reabrir el defecto de los dos tickets #1 en la misma noche.

-- Destino de la llave compuesta de abajo. `register_sessions` no lo tenía: su única unicidad por
-- empresa era la del turno abierto por caja.
create unique index register_sessions_tenant_key on register_sessions (company_id, id);

create table folio_counters (
  register_session_id bigint not null,
  last_number         int not null,
  company_id          bigint not null default current_setting('app.company_id', true)::bigint
                      references companies(id) on delete cascade,

  -- LLAVE COMPUESTA, no `references register_sessions(id)` a secas. Los chequeos de integridad
  -- referencial de Postgres SALTAN RLS, así que con una llave simple nada en el esquema impediría
  -- que el contador de una empresa colgara del turno de otra. Es el mismo hueco que cerró 0041, y
  -- el escritor que lo abre es justo el de aquí abajo: un backfill de migración corre como owner.
  --
  -- `on delete cascade` porque el único borrado real es el de una empresa entera, que ya arrastra
  -- sus turnos. Un turno suelto no se puede borrar: los pedidos que lo referencian lo impiden
  -- antes de llegar aquí.
  foreign key (company_id, register_session_id)
    references register_sessions (company_id, id) on delete cascade,
  -- Arranca por company_id como todo índice de este esquema: RLS le pega ese predicado a cada
  -- consulta del rol de la app, y una PK que empiece por el turno se queda descartando filas de
  -- otras empresas dentro del scan.
  --
  -- El candado de fila sobre ESTA llave es lo que serializa la numeración concurrente. No es un
  -- detalle del índice: es la garantía de que dos cobros simultáneos no reciben el mismo número, y
  -- cualquier cambio aquí tiene que conservarla.
  primary key (company_id, register_session_id)
);

alter table folio_counters enable row level security;
create policy tenant_isolation on folio_counters
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- El grant de 0024 fue `on all tables in schema public`, que es PUNTUAL: no hay default privileges,
-- así que cada tabla creada después necesita el suyo. Sin esto la migración pasa, los tests pasan y
-- `make start` pasa —dev conecta como owner— y en producción el primer pedido devuelve 42501.
--
-- Sin `delete`: un contador se crea y se incrementa, nunca se borra.
grant select, insert, update on folio_counters to gatobobah_app;

-- La semilla NO es opcional. Corre como owner, así que ve todas las empresas.
--
-- Los turnos que ya están abiertos traen folios repartidos: el del ambiente de pruebas tiene 158.
-- Sin sembrar, su siguiente venta pediría el número 1 y chocaría con uno que ya existe. Se siembran
-- también los turnos cerrados —cuestan nada— para no tener que razonar sobre cuáles hacían falta.
insert into folio_counters (company_id, register_session_id, last_number)
select company_id, register_session_id, max(daily_number)
from orders
where register_session_id is not null
group by company_id, register_session_id;

-- LAS DOS RESTRICCIONES DE UNICIDAD SE MUEVEN CON EL FOLIO, o el esquema sigue exigiendo la regla
-- vieja y la venta se cae con 23505 en cuanto haya dos turnos el mismo día.
--
-- No es teoría: al cambiar solo el contador, `TestElCorteSumaPorTurnoYNoPorHora` se puso rojo con
-- "duplicate key value violates unique constraint orders_company_daily_key". El alcance del folio
-- pasó a ser el turno, así que su unicidad tiene que medirse contra el turno.
--
-- Lo que se pierde: dos tickets del mismo día pueden llevar el número 1 y hasta el mismo nombre, si
-- salieron de turnos distintos. Se acepta porque lo que el folio tiene que distinguir son pedidos
-- VIVOS, y cerrar un turno ya exige que no quede ninguno: los dos "Tigre" nunca coexisten. Se
-- distinguen además por fecha y hora.
--
-- Verificado contra los datos reales antes de aplicarlo: cero números y cero nombres duplicados
-- dentro de un mismo turno en producción.
alter table orders drop constraint orders_company_daily_key;
alter table orders add constraint orders_folio_turno_key
  unique (company_id, register_session_id, daily_number);

-- Parcial, igual que el de 0047: los pedidos anteriores a 0046 tienen folio_name nulo y no deben
-- chocar entre sí.
drop index orders_folio_dia;
create unique index orders_folio_turno
  on orders (company_id, register_session_id, folio_name)
  where folio_name is not null;

-- `order_counters` SE QUEDA EN PIE, sin usar y sin tocar. Dos razones:
--
-- 1. El rollback por imagen sigue funcionando. Si le cambiáramos la PK, el binario anterior dejaría
--    de poder numerar y volver atrás exigiría restaurar la base.
-- 2. El Down de esta migración es entonces trivialmente correcto: un `drop`. Un Down que devuelve
--    una PK a su estado anterior tendría además que reconstruir las filas, y eso ya no es
--    reversible de verdad.
--
-- ponytail: se borra en una migración propia cuando esta feature lleve un ciclo en producción.

-- +goose Down
-- ADVERTENCIA, y se dice aquí en vez de descubrirse al revertir: volver a estrechar la unicidad al
-- DÍA falla si entretanto se vendió con dos turnos el mismo día, porque entonces existen dos #1 con
-- la misma fecha. Es inherente a revertir una restricción que se ensanchó, no un descuido. Si pasa,
-- el `Down` se detiene sin haber tocado nada y hay que decidir a mano qué pedido se renumera.
drop index if exists orders_folio_turno;
create unique index orders_folio_dia
  on orders (company_id, business_date, folio_name)
  where folio_name is not null;

alter table orders drop constraint if exists orders_folio_turno_key;
alter table orders add constraint orders_company_daily_key
  unique (company_id, business_date, daily_number);

drop table if exists folio_counters;
drop index if exists register_sessions_tenant_key;
