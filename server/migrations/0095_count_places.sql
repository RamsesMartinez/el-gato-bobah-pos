-- +goose Up
-- Lugares para contar (spec 040): el lugar de un artículo deja de ser texto libre (0092) y pasa a ser
-- una liga a la lista de lugares de su empresa, con orden de recorrido y archivo. Lo escrito a mano
-- pasa a la lista sin que ningún artículo pierda su lugar; lo que solo difería en mayúsculas,
-- acentos o espacios se junta.
--
-- El lugar NO es un almacén (issue #65): solo ordena el recorrido del conteo y no guarda existencias.

set local lock_timeout = '3s';

create table count_places (
  id          bigint generated always as identity primary key,
  company_id  bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
              references companies(id) on delete cascade,
  name        text not null check (char_length(btrim(name)) between 1 and 30),
  -- Ordinal del recorrido entre los activos (0 = primero). El servidor lo renumera bajo un candado
  -- por empresa: dos altas o un alta y un mover a la vez no pueden repetir posición.
  position    int not null check (position >= 0),
  archived_at timestamptz,
  created_at  timestamptz not null default now(),
  constraint count_places_id_company_key unique (id, company_id)
);
-- Un nombre por empresa sin distinguir mayúsculas, acentos ni espacios: la misma expresión que
-- waste_reasons_name_key y domain.NormalizeReasonName. Solo entre los que se ofrecen.
create unique index count_places_name_key on count_places
  (company_id, regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g'))
  where archived_at is null;

alter table count_places enable row level security;
create policy tenant_isolation on count_places
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update on count_places to gatobobah_app;

-- La liga va compuesta: una llave foránea salta RLS, y sin company_id un artículo podría apuntar al
-- lugar de otra empresa. no action, como las demás ligas dentro de una empresa: los lugares no se
-- borran, se archivan, y el borrado de la empresa cae en las dos tablas.
alter table ingredients add column count_place_id bigint;
alter table ingredients add constraint ingredients_count_place_fkey
  foreign key (count_place_id, company_id) references count_places (id, company_id);
create index ingredients_by_count_place on ingredients (count_place_id) where count_place_id is not null;

-- Lo escrito a mano: un lugar por texto normalizado y empresa. El nombre es el texto más usado del
-- grupo, sin espacios de más (en empate, el primero en orden de bytes, para que no dependa de la
-- intercalación); el orden inicial es alfabético.
-- Los espacios se juntan ANTES del btrim: btrim solo quita espacios, y un tab o un espacio duro en
-- la orilla («\tRefri») daba otro grupo, chocaba con el índice o dejaba al artículo sin lugar.
with cleaned as (
  select company_id, btrim(regexp_replace(count_place, '\s+', ' ', 'g')) as clean
    from ingredients where count_place is not null
), written as (
  select company_id, clean, translate(lower(clean), 'áéíóúüñ', 'aeiouun') as norm from cleaned
), chosen as (
  select distinct on (company_id, norm) company_id, norm, clean
    from (select company_id, norm, clean, count(*) as uses from written group by 1, 2, 3) g
   order by company_id, norm, uses desc, clean collate "C"
)
insert into count_places (company_id, name, position)
select company_id, clean, (row_number() over (partition by company_id order by norm) - 1)::int
  from chosen;

update ingredients i set count_place_id = p.id
  from count_places p
 where i.count_place is not null and p.company_id = i.company_id
   and translate(lower(p.name), 'áéíóúüñ', 'aeiouun')
     = translate(lower(btrim(regexp_replace(i.count_place, '\s+', ' ', 'g'))), 'áéíóúüñ', 'aeiouun');

-- Ningún artículo que tenía lugar se queda sin él: si el paso falló para alguno, se aborta antes de
-- borrar la única copia del texto.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from ingredients where count_place is not null and count_place_id is null) then
    raise exception '0095: hay artículos que perderían su lugar';
  end if;
end $$;
-- +goose StatementEnd

alter table ingredients drop column count_place;

-- +goose Down
-- Regresa el texto con el nombre del lugar. Se pierden el orden, los lugares sin artículos y los
-- archivados, y la forma original de lo que la subida juntó: es el mismo dato con menos detalle.
alter table ingredients add column count_place text check (char_length(btrim(count_place)) between 1 and 30);
update ingredients i set count_place = p.name from count_places p where p.id = i.count_place_id;
drop index ingredients_by_count_place;
alter table ingredients drop constraint ingredients_count_place_fkey, drop column count_place_id;
drop table count_places;
