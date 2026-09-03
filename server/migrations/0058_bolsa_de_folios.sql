-- +goose Up
-- El nombre del pedido deja de ser una función del día y pasa a salir de una BOLSA que se agota.
--
-- Antes el nombre se calculaba: `animal[(folio-1) % 100]` sobre una lista barajada por (empresa,
-- día). Eso reparte bien dentro de un día pero no entre días: con 40 pedidos diarios, los mismos
-- nombres salían una y otra vez y media lista no se usaba nunca. Ahora se sortea entre los que
-- todavía no salen, y solo al acabarse todos se empieza otra vuelta.

create type folio_scheme as enum ('animales', 'razas');

-- Con qué se nombran los pedidos de este negocio. Default 'razas' —razas de gato— y ese default
-- alcanza también a las empresas que ya existen: corre como owner, así que RLS no aplica y el
-- `add column ... not null default` sella el valor en las filas actuales.
alter table business_settings
  add column folio_scheme folio_scheme not null default 'razas';

-- Los nombres que YA SALIERON en la vuelta en curso. Solo eso.
--
-- La lista completa vive en el binario (internal/domain/folio.go) y NO se copia aquí: agregarle un
-- nombre lo pone disponible en la siguiente vuelta sin tocar datos, y sembrar 88 filas por empresa
-- las dejaría obsoletas en el primer cambio de la lista. Al acabarse la vuelta se BORRAN estas
-- filas y empieza otra.
--
-- Una fila por (empresa, esquema, nombre): los dos esquemas llevan bolsas independientes, así que
-- cambiar de razas a animales y volver no pierde la vuelta que iba a medias.
create table folio_consumido (
  scheme     folio_scheme not null,
  name       text not null,
  taken_at   timestamptz not null default now(),
  company_id bigint not null default current_setting('app.company_id', true)::bigint
             references companies(id) on delete cascade,
  -- Arranca por company_id a propósito: RLS le pega ese predicado a toda consulta del rol de la
  -- app, y un índice que empiece por scheme se quedaría descartando filas de otras empresas dentro
  -- del scan. Cubre las tres consultas que existen (listar, insertar, vaciar) sin índice extra.
  primary key (company_id, scheme, name)
);

alter table folio_consumido enable row level security;
create policy tenant_isolation on folio_consumido
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- El grant de 0024 fue `on all tables in schema public`, que es PUNTUAL: no hay default privileges,
-- así que cada tabla creada después necesita el suyo. Sin esto la migración pasa, los tests pasan y
-- `make start` pasa —dev sirve como owner— y en producción el primer pedido devuelve 42501.
--
-- Sin `update`: una fila de esta tabla se inserta y se borra, nunca se modifica. Un grant que nadie
-- usa es superficie que alguien acaba usando.
grant select, insert, delete on folio_consumido to gatobobah_app;

-- +goose Down
-- Revertir NO pierde ninguna venta: los nombres ya cantados viven en orders.folio_name, que no se
-- toca. Lo que se pierde es en qué punto de la vuelta iba cada empresa, y la lógica vieja no lo usa.
drop table if exists folio_consumido;
alter table business_settings drop column folio_scheme;
drop type folio_scheme;
