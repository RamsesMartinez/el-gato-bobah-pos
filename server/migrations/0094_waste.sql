-- +goose Up
-- Lo que se echó a perder (spec 039): motivos por empresa, registros de merma que restan del almacén
-- (o esperan aprobación si pasan el tope de la empresa) y lo que el conteo necesita para partir el
-- faltante en registrado y sin explicar.
--
-- Como la 0092: tablas de empresa con RLS y toda referencia entre ellas COMPUESTA con company_id,
-- porque una llave foránea salta RLS. Nada se borra: un registro cambia de estado, nunca desaparece.

set local lock_timeout = '3s';

-- 1. MOTIVOS. De cada empresa; los sugeridos se siembran abajo y la empresa los edita.
create table waste_reasons (
  id             bigint generated always as identity primary key,
  company_id     bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                 references companies(id) on delete cascade,
  name           text not null check (char_length(btrim(name)) between 1 and 40),
  description    text not null check (char_length(btrim(description)) between 1 and 200),
  -- «No cuenta como pérdida del negocio» (la comida del personal): el reporte lo pone aparte. El
  -- registro copia la marca al registrarse, así que cambiarla no reclasifica el pasado.
  is_staff_meal  boolean not null default false,
  archived_at    timestamptz,
  -- Juntar no reescribe los registros (revisión de arquitectura): el reporte sigue esta liga, de un
  -- solo nivel, y cada registro conserva el motivo con que se capturó.
  merged_into_id bigint,
  -- Nulo en los sembrados por la migración, que no tienen autor.
  created_by     bigint,
  created_at     timestamptz not null default now(),
  constraint waste_reasons_id_company_key unique (id, company_id),
  constraint waste_reasons_merged_into_fkey foreign key (merged_into_id, company_id) references waste_reasons (id, company_id),
  constraint waste_reasons_created_by_fkey foreign key (company_id, created_by) references users (company_id, id),
  constraint waste_reasons_not_into_itself check (merged_into_id is null or merged_into_id <> id)
);
-- Un nombre por empresa sin distinguir mayúsculas, acentos ni espacios: la misma normalización que
-- `IngredientNameTaken` y `domain.NormalizeReasonName`. Solo entre los que se ofrecen.
create unique index waste_reasons_name_key on waste_reasons
  (company_id, regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g'))
  where archived_at is null and merged_into_id is null;

-- 2. REGISTROS. Uno por cosa que se echó a perder, en la sucursal de quien la registra.
create table waste_records (
  id                  bigint generated always as identity primary key,
  company_id          bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                      references companies(id) on delete cascade,
  branch_id           bigint not null,
  reason_id           bigint not null,
  item_type           stock_item_type not null,
  ingredient_id       bigint,
  product_id          bigint,
  -- En la unidad base del insumo, o piezas de producto.
  quantity            numeric(14,4) not null check (quantity > 0),
  -- Lo que se tecleó y en qué empaque, para mostrarlo como se capturó.
  entered_qty         numeric(14,4) not null check (entered_qty > 0),
  presentation_id     bigint,
  -- Costo de ese momento (snapshot): el pasado no cambia cuando cambia el costo.
  unit_cost           numeric(12,6),
  total_cost          numeric(12,2),
  is_staff_meal       boolean not null,
  note                text check (char_length(note) <= 200),
  status              text not null check (status in ('applied', 'pending', 'rejected', 'covered', 'undone')),
  recorded_by         bigint not null,
  recorded_at         timestamptz not null default clock_timestamp(),
  resolved_by         bigint,
  resolved_at         timestamptz,
  covered_by_count_id bigint,
  constraint waste_records_id_company_key unique (id, company_id),
  constraint waste_records_id_company_branch_key unique (id, company_id, branch_id),
  constraint waste_records_branch_fkey foreign key (company_id, branch_id) references branches (company_id, id),
  constraint waste_records_reason_fkey foreign key (reason_id, company_id) references waste_reasons (id, company_id),
  constraint waste_records_ingredient_fkey foreign key (ingredient_id, company_id) references ingredients (id, company_id),
  constraint waste_records_product_fkey foreign key (product_id, company_id) references products (id, company_id),
  constraint waste_records_presentation_fkey foreign key (presentation_id, company_id)
    references ingredient_presentations (id, company_id) on delete set null (presentation_id),
  constraint waste_records_recorded_by_fkey foreign key (company_id, recorded_by) references users (company_id, id),
  constraint waste_records_resolved_by_fkey foreign key (company_id, resolved_by) references users (company_id, id),
  constraint waste_records_covered_by_fkey foreign key (covered_by_count_id, company_id) references stock_counts (id, company_id),
  constraint waste_records_item check ((item_type = 'ingrediente') = (ingredient_id is not null)
                                       and (item_type = 'producto') = (product_id is not null)),
  constraint waste_records_cost_whole check ((unit_cost is null) = (total_cost is null)),
  constraint waste_records_covered_has_count check ((status = 'covered') = (covered_by_count_id is not null)),
  -- Rechazada, cubierta o deshecha siempre dicen quién y cuándo; por aprobar, nunca.
  constraint waste_records_resolved_has_who check (
    case status
      when 'pending' then resolved_at is null and resolved_by is null
      when 'applied' then (resolved_at is null) = (resolved_by is null)
      when 'covered' then resolved_at is not null
      else resolved_at is not null and resolved_by is not null
    end)
);
create index waste_records_by_day on waste_records (company_id, branch_id, recorded_at desc);
create index waste_records_pending on waste_records (company_id, branch_id) where status = 'pending';
create index waste_records_by_reason on waste_records (company_id, reason_id);

-- 3. EL MOVIMIENTO DICE DE QUÉ MERMA VIENE, en la misma sucursal que su registro. `no action` y no
--    `restrict`: borrar una empresa cae por cascada y `no action` se revisa al final de la sentencia.
alter table stock_movements
  add column waste_record_id bigint,
  add constraint stock_movements_waste_record_fkey
    foreign key (waste_record_id, company_id, branch_id) references waste_records (id, company_id, branch_id);
create index stock_movements_by_waste on stock_movements (company_id, branch_id, ingredient_id, id)
  where waste_record_id is not null;
create index stock_movements_by_waste_record on stock_movements (waste_record_id, company_id)
  where waste_record_id is not null;

-- 4. EL CONTEO GUARDA LO QUE VIO. El periodo del faltante se ancla por id de movimiento y no por
--    hora: `now()` es la hora de inicio de la transacción, y una merma que empezó antes de la
--    captura y confirmó después caería en el periodo equivocado (revisión de arquitectura). Y el
--    costo al cerrar, para que un reporte cerrado no cambie cuando cambia el costo.
alter table stock_count_lines
  add column last_movement_id bigint,
  add column unit_cost        numeric(12,6);

-- 5. EL TOPE. Nulo: ninguna merma espera aprobación.
alter table business_settings
  add column waste_approval_threshold numeric(12,2) check (waste_approval_threshold > 0);

-- 6. RLS Y GRANTS. Sin delete: nada se borra.
alter table waste_reasons enable row level security;
create policy tenant_isolation on waste_reasons
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
alter table waste_records enable row level security;
create policy tenant_isolation on waste_records
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert, update on waste_reasons to gatobobah_app;
grant select, insert, update on waste_records to gatobobah_app;

-- 7. MOTIVOS SUGERIDOS para cada empresa que ya existe. Son de cualquier negocio de comida, no de
--    uno; la empresa los renombra o archiva. Las empresas nuevas los reciben en `CreateCompany`.
insert into waste_reasons (company_id, name, description, is_staff_meal)
select c.id, s.name, s.description, s.is_staff_meal
  from companies c
 cross join (values
   ('Caducó',               'Pasó su fecha o se echó a perder guardado', false),
   ('Se cayó',              'Un accidente al servir o al moverlo',         false),
   ('Mal preparado',        'Salió mal y no se pudo vender',               false),
   ('Prueba de receta',     'Lo que se usó para probar una receta',        false),
   ('Consumo del personal', 'Lo que se toma o se come el equipo',          true)
 ) as s(name, description, is_staff_meal)
 where not exists (
   select 1 from waste_reasons r
    where r.company_id = c.id
      and regexp_replace(translate(lower(btrim(r.name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')
        = regexp_replace(translate(lower(btrim(s.name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g'));

-- +goose Down
-- Se niega con una sola merma registrada: es la única copia de qué se tiró, por qué y quién.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from waste_records) or exists (select 1 from stock_movements where waste_record_id is not null) then
    raise exception 'hay mermas registradas: son la única copia de qué se tiró y por qué';
  end if;
end $$;
-- +goose StatementEnd
alter table business_settings drop column waste_approval_threshold;
alter table stock_count_lines drop column unit_cost, drop column last_movement_id;
drop index stock_movements_by_waste_record;
drop index stock_movements_by_waste;
alter table stock_movements drop constraint stock_movements_waste_record_fkey, drop column waste_record_id;
drop table waste_records;
drop table waste_reasons;
