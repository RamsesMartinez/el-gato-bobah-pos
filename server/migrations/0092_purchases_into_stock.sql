-- +goose Up
-- Compras al almacén (spec 038): empaques por insumo, compras que suman y se corrigen por
-- diferencia, lo que llega después, y el conteo de una sucursal.
--
-- Todas las tablas nuevas son por empresa con RLS, y toda referencia entre tablas de empresa es
-- COMPUESTA con company_id: los chequeos de integridad referencial saltan RLS (la clase de defecto
-- que cerró la 0041). Nada de esto rellena datos existentes.

set local lock_timeout = '3s';

-- 1. UNIDADES DE LONGITUD. La base es el centímetro por el mismo criterio que g y ml: la unidad más
--    chica de uso común, para que un corte de 35 cm no viva en decimales.
insert into units (code, name, kind, to_base) values
  ('cm', 'Centímetro', 'longitud', 1),
  ('m',  'Metro',      'longitud', 100);

-- 2. LLAVES POR EMPRESA que las FK compuestas de abajo necesitan como destino.
alter table ingredients   add constraint ingredients_id_company_key   unique (id, company_id);
alter table expense_items add constraint expense_items_id_company_key unique (id, company_id);

-- 3. EMPAQUES DE COMPRA POR INSUMO («Caja = 12 L»). Su contenido se copia al renglón de compra
--    (`pack_qty_in_base`), así que cambiar o desactivar un empaque no reescribe compras pasadas.
create table ingredient_presentations (
  id               bigint generated always as identity primary key,
  company_id       bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                   references companies(id) on delete cascade,
  ingredient_id    bigint not null,
  name             text not null check (char_length(btrim(name)) between 1 and 40),
  -- Lo que trae, en la unidad base del insumo (g, ml, cm o pieza).
  qty_in_base      numeric(14,4) not null check (qty_in_base > 0),
  active           boolean not null default true,
  -- Nulo: el empaque es una pieza de compra. Con valor: empaque APROXIMADO de otro tipo («por kilo
  -- ≈ 4 piezas»), el único camino entre tipos distintos, y solo porque alguien lo declaró así.
  purchase_unit_id smallint references units(id),
  is_approximate   boolean not null default false,
  created_at       timestamptz not null default now(),
  constraint ingredient_presentations_id_company_key unique (id, company_id),
  constraint ingredient_presentations_ingredient_fkey
    foreign key (ingredient_id, company_id) references ingredients (id, company_id) on delete cascade
);
-- Nombre único por insumo con la normalización de `IngredientNameTaken`: sin mayúsculas, acentos ni
-- espacios de más. Solo entre los activos: uno desactivado no estorba a uno nuevo igual.
create unique index ingredient_presentations_name_key on ingredient_presentations
  (company_id, ingredient_id, regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g'))
  where active;
create index ingredient_presentations_by_ingredient on ingredient_presentations (company_id, ingredient_id);

-- Reglas que comparan con la unidad del insumo y con el catálogo de unidades: otras tablas, así
-- que van en un trigger y no en un check.
-- +goose StatementBegin
create function ingredient_presentations_before_write() returns trigger
language plpgsql as $$
declare
  base_kind     unit_kind;
  purchase_kind unit_kind;
  norm          text;
begin
  select u.kind into base_kind
    from ingredients i join units u on u.id = i.base_unit_id
   where i.id = new.ingredient_id and i.company_id = new.company_id;

  if new.purchase_unit_id is not null then
    select kind into purchase_kind from units where id = new.purchase_unit_id;
  end if;

  -- Aproximado si y solo si se compra en una unidad de otro tipo que la base del insumo.
  if new.is_approximate is distinct from (purchase_kind is not null and purchase_kind <> base_kind) then
    raise exception 'un empaque es aproximado solo si se compra en otro tipo de unidad que el insumo'
      using errcode = 'check_violation';
  end if;

  -- Regla 16: en un insumo por pieza, el empaque trae piezas enteras (las de la etiqueta).
  if base_kind = 'pieza' and new.qty_in_base <> trunc(new.qty_in_base) then
    raise exception 'un empaque de un insumo por pieza trae piezas enteras'
      using errcode = 'check_violation';
  end if;

  -- Regla 14: un empaque no se llama como una unidad («Litro», «Kilo»): eso es comprar suelto. El
  -- empaque aproximado sí se nombra por su unidad («Kilo ≈ 4 piezas»).
  if not new.is_approximate then
    norm := regexp_replace(translate(lower(btrim(new.name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g');
    if exists (
      select 1 from units u
       where norm in (lower(u.code), translate(lower(u.name), 'áéíóúüñ', 'aeiouun'),
                      translate(lower(u.name), 'áéíóúüñ', 'aeiouun') || 's')
    ) or norm in ('kilo', 'kilos', 'litros', 'gramos', 'metros', 'piezas', 'pza', 'pzas') then
      raise exception 'un empaque no se llama como una unidad: eso es comprar suelto'
        using errcode = 'check_violation';
    end if;
  end if;
  return new;
end;
$$;
-- +goose StatementEnd
create trigger ingredient_presentations_before_write
  before insert or update on ingredient_presentations
  for each row execute function ingredient_presentations_before_write();

-- 4. CONTEOS. Un conteo abierto por sucursal; las cifras se guardan al capturarlas (sobreviven a
--    recargar y a cambiar de tableta) junto con lo que el sistema tenía en ese momento, y al cerrar
--    se ajusta por la diferencia (lo vendido mientras tanto ya restó).
create table stock_counts (
  id          bigint generated always as identity primary key,
  company_id  bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
              references companies(id) on delete cascade,
  branch_id   bigint not null,
  status      text not null default 'open' check (status in ('open', 'closed', 'discarded')),
  opened_by   bigint not null,
  opened_at   timestamptz not null default now(),
  closed_by   bigint,
  closed_at   timestamptz,
  constraint stock_counts_id_company_key unique (id, company_id),
  constraint stock_counts_branch_fkey foreign key (company_id, branch_id) references branches (company_id, id),
  constraint stock_counts_opened_by_fkey foreign key (company_id, opened_by) references users (company_id, id),
  constraint stock_counts_closed_by_fkey foreign key (company_id, closed_by) references users (company_id, id),
  constraint stock_counts_closed_has_who check ((status = 'open') = (closed_by is null and closed_at is null))
);
create unique index stock_counts_one_open on stock_counts (company_id, branch_id) where status = 'open';

create table stock_count_lines (
  count_id      bigint not null,
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                references companies(id) on delete cascade,
  ingredient_id bigint not null,
  -- 0 = «no hay»; sin renglón = «no se contó» (no se confunden).
  counted_qty   numeric(14,4) not null check (counted_qty >= 0),
  system_qty    numeric(14,4) not null,
  captured_by   bigint not null,
  captured_at   timestamptz not null default now(),
  primary key (count_id, ingredient_id),
  constraint stock_count_lines_count_fkey
    foreign key (count_id, company_id) references stock_counts (id, company_id) on delete cascade,
  constraint stock_count_lines_ingredient_fkey
    foreign key (ingredient_id, company_id) references ingredients (id, company_id),
  constraint stock_count_lines_captured_by_fkey
    foreign key (company_id, captured_by) references users (company_id, id)
);

-- 5. EL MOVIMIENTO DICE DE DÓNDE VIENE. Con `expense_item_id` se corrige un renglón sin confundirlo
--    con otro del mismo insumo; con `stock_count_id` cada ajuste apunta a su conteo (SC-003).
alter table stock_movements
  add column expense_item_id bigint,
  add column stock_count_id  bigint,
  add constraint stock_movements_expense_item_fkey
    foreign key (expense_item_id, company_id) references expense_items (id, company_id) on delete restrict,
  add constraint stock_movements_stock_count_fkey
    foreign key (stock_count_id, company_id) references stock_counts (id, company_id) on delete restrict;
create index stock_movements_by_expense_item on stock_movements (company_id, expense_item_id) where expense_item_id is not null;
create index stock_movements_by_stock_count on stock_movements (company_id, stock_count_id) where stock_count_id is not null;

-- 6. EL RENGLÓN DE COMPRA.
alter table expense_items
  add column presentation_id  bigint,
  -- Lo ya sumado al almacén, en unidad base: se actualiza en la misma transacción que cada
  -- movimiento del renglón, y permite listar lo pendiente sin sumar otra tabla.
  add column applied_qty_base numeric(14,4) not null default 0,
  -- «Ya no va a llegar», en la unidad del renglón; puede ser una parte.
  add column not_coming_qty   numeric(14,4) not null default 0,
  add column voided_at        timestamptz,
  add column is_gift          boolean not null default false,
  -- Se borra el empaque (solo por cascada de su insumo) y el renglón conserva su contenido copiado.
  add constraint expense_items_presentation_fkey
    foreign key (presentation_id, company_id) references ingredient_presentations (id, company_id)
    on delete set null (presentation_id),
  add constraint expense_items_not_coming_range check (not_coming_qty >= 0 and not_coming_qty <= quantity),
  add constraint expense_items_gift_is_free check (not is_gift or amount = 0);
-- Importe mayor que cero salvo un regalo (FR-004a). `not valid`: hay renglones heredados con 0
-- (medido el 14-oct en la copia de producción) que no se reescriben; aplica a lo nuevo y a lo que
-- se edite.
alter table expense_items add constraint expense_items_amount_positive check (amount > 0 or is_gift) not valid;
create index expense_items_open_by_expense on expense_items (company_id, expense_id) where voided_at is null;

-- 7. EL «LUGAR» DE UN INSUMO solo ordena el recorrido del conteo; no es un almacén (puerta del
--    principio VIII, issue #65).
alter table ingredients add column count_place text check (char_length(btrim(count_place)) between 1 and 30);

-- 8. CONFIGURACIÓN POR EMPRESA.
alter table business_settings
  add column costing_method   text not null default 'last' check (costing_method in ('last', 'weighted')),
  add column stock_item_label text not null default 'artículo' check (char_length(btrim(stock_item_label)) between 1 and 20);

-- 9. RLS Y GRANTS.
alter table ingredient_presentations enable row level security;
create policy tenant_isolation on ingredient_presentations
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
alter table stock_counts enable row level security;
create policy tenant_isolation on stock_counts
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
alter table stock_count_lines enable row level security;
create policy tenant_isolation on stock_count_lines
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);

grant select, insert, update on ingredient_presentations to gatobobah_app;
grant select, insert, update on stock_counts to gatobobah_app;
-- Los renglones de un conteo abierto se quitan («dejar sin contar»); los de uno cerrado los protege
-- el servicio, que solo escribe con el conteo abierto y bloqueado.
grant select, insert, update, delete on stock_count_lines to gatobobah_app;

-- +goose Down
-- Se niega si perdería algo: conteos, compras ligadas a su renglón o empaques usados son la única
-- copia de esos hechos. Tras desplegar, el rollback es restaurar el respaldo.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from stock_counts) then
    raise exception 'hay conteos: son la única copia de lo que se contó';
  end if;
  if exists (select 1 from stock_movements where expense_item_id is not null or stock_count_id is not null) then
    raise exception 'hay movimientos ligados a un renglón de compra o a un conteo';
  end if;
  if exists (select 1 from expense_items where presentation_id is not null or applied_qty_base <> 0
             or not_coming_qty <> 0 or voided_at is not null or is_gift) then
    raise exception 'hay renglones de compra con datos de la 0092';
  end if;
  if exists (select 1 from ingredients i join units u on u.id = i.base_unit_id where u.kind = 'longitud')
     or exists (select 1 from expense_items e join units u on u.id = e.unit_id where u.kind = 'longitud') then
    raise exception 'hay insumos o compras en unidades de longitud';
  end if;
end $$;
-- +goose StatementEnd

alter table business_settings drop column stock_item_label, drop column costing_method;
alter table ingredients drop column count_place;
drop index expense_items_open_by_expense;
alter table expense_items
  drop constraint expense_items_amount_positive,
  drop constraint expense_items_gift_is_free,
  drop constraint expense_items_not_coming_range,
  drop constraint expense_items_presentation_fkey,
  drop column is_gift,
  drop column voided_at,
  drop column not_coming_qty,
  drop column applied_qty_base,
  drop column presentation_id;
drop index stock_movements_by_stock_count;
drop index stock_movements_by_expense_item;
alter table stock_movements
  drop constraint stock_movements_stock_count_fkey,
  drop constraint stock_movements_expense_item_fkey,
  drop column stock_count_id,
  drop column expense_item_id;
drop table stock_count_lines;
drop table stock_counts;
drop trigger ingredient_presentations_before_write on ingredient_presentations;
drop function ingredient_presentations_before_write();
drop table ingredient_presentations;
alter table expense_items drop constraint expense_items_id_company_key;
alter table ingredients drop constraint ingredients_id_company_key;
delete from units where code in ('cm', 'm');
