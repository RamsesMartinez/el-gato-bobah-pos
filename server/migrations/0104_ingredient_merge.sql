-- Unir insumos duplicados y marca en la presentación (spec 056, issue #185).
--
-- Todo es agregar: la API anterior sigue funcionando durante la subida azul/verde (constitución IV).
-- Las guardas solo actúan sobre insumos que son ORIGEN de una unión, y no existe ninguno hasta que la
-- API nueva una.
--
-- La unión reusa los reemplazos de la 0096: un renglón por origen con kind = 'merge'. El traspaso de
-- existencia son movimientos ligados por stock_movements.replacement_id, como el del reemplazo.

-- +goose Up
set local lock_timeout = '3s';

alter table ingredient_replacements add column kind text not null default 'unit_change'
  constraint ingredient_replacements_kind_check check (kind in ('unit_change', 'merge'));

-- La marca o el proveedor de una presentación («Caja 12 × 1 L · Marca A»).
alter table ingredient_presentations add column brand text
  constraint ingredient_presentations_brand_check check (brand is null or char_length(btrim(brand)) between 1 and 40);

-- El nombre deja de ser único por insumo: lo es con su marca. El nuevo se crea antes de soltar el
-- viejo; es más permisivo, así que la API anterior (que inserta sin marca) choca igual que hoy.
create unique index ingredient_presentations_name_brand_key on ingredient_presentations (
  company_id, ingredient_id,
  regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g'),
  regexp_replace(translate(lower(btrim(coalesce(brand, ''))), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g'))
  where active;
drop index ingredient_presentations_name_key;

-- La marca con la que se compró, copiada al renglón (principio VIII, snapshot). La marca de una
-- presentación se puede editar; sin la copia, editarla reescribe de qué marca fue cada compra pasada.
alter table expense_items add column presentation_brand text;

-- «No son lo mismo»: dos insumos que alguien revisó y que la lista de posibles duplicados ya no
-- propone. El par se guarda ordenado para que (a, b) y (b, a) sean el mismo.
create table ingredient_distinct_pairs (
  id                 bigint generated always as identity primary key,
  company_id         bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                     references companies(id) on delete cascade,
  low_ingredient_id  bigint not null,
  high_ingredient_id bigint not null,
  user_id            bigint not null,
  created_at         timestamptz not null default now(),
  constraint ingredient_distinct_pairs_order check (low_ingredient_id < high_ingredient_id),
  constraint ingredient_distinct_pairs_key unique (company_id, low_ingredient_id, high_ingredient_id),
  constraint ingredient_distinct_pairs_low_fkey
    foreign key (low_ingredient_id, company_id) references ingredients (id, company_id),
  constraint ingredient_distinct_pairs_high_fkey
    foreign key (high_ingredient_id, company_id) references ingredients (id, company_id),
  constraint ingredient_distinct_pairs_user_fkey
    foreign key (company_id, user_id) references users (company_id, id)
);
create index ingredient_distinct_pairs_by_high on ingredient_distinct_pairs (company_id, high_ingredient_id);
alter table ingredient_distinct_pairs enable row level security;
create policy tenant_isolation on ingredient_distinct_pairs
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on ingredient_distinct_pairs to gatobobah_app;

-- Las funciones de trigger son security definer y filtran por la empresa de la FILA, no por
-- app.company_id: una conexión reciclada trae el ajuste vacío (0074) y, bajo RLS, no vería ninguna
-- unión y dejaría pasar justo lo que vienen a impedir. Funcionan igual para gatobobah_app, el webhook
-- y el dueño. Nadie las llama directo: Postgres no revisa EXECUTE al disparar un trigger, y se quita a
-- public al final.

-- Una unión válida: el destino no puede ser origen de otra unión ni haber sido reemplazado (así no
-- hay ciclos aunque un script se equivoque), y ni origen ni destino pueden estar archivados o ser
-- preparados. La API lo valida antes con un mensaje para quien opera; esto es la barrera.
-- +goose StatementBegin
create function check_ingredient_merge() returns trigger
language plpgsql security definer set search_path = pg_catalog, public, pg_temp as $$
begin
  if exists (select 1 from public.ingredient_replacements r
              where r.company_id = new.company_id and r.old_ingredient_id = new.new_ingredient_id) then
    raise exception 'ingredient_merge: el destino ya se unió o se reemplazó' using errcode = 'check_violation';
  end if;
  if exists (select 1 from public.ingredients i
              where i.company_id = new.company_id and i.id in (new.old_ingredient_id, new.new_ingredient_id)
                and (not i.is_active or i.is_prep)) then
    raise exception 'ingredient_merge: origen o destino archivado o preparado' using errcode = 'check_violation';
  end if;
  return new;
end;
$$;
-- +goose StatementEnd
create trigger ingredient_replacements_check_merge before insert on ingredient_replacements
  for each row when (new.kind = 'merge') execute function check_ingredient_merge();

-- Un movimiento que llega a un origen unido entra al destino: una pantalla abierta desde antes, la
-- cancelación de un pedido vendido antes de unir (repone donde ahora está la mercancía) o la versión
-- anterior de la API durante la subida. Sigue la cadena si el destino también se unió después, con
-- el producto de las equivalencias. El traspaso de la propia unión lleva replacement_id y no se toca.
-- Una cantidad diminuta que redondea a cero queda en ±0.0001: la columna no admite cero.
-- +goose StatementBegin
create function redirect_merged_ingredient_movement() returns trigger
language plpgsql security definer set search_path = pg_catalog, public, pg_temp as $$
declare
  v_target bigint := new.ingredient_id;
  v_factor numeric := 1;
  v_next   bigint;
  v_step   numeric;
  v_depth  int := 0;
  v_qty    numeric;
begin
  if new.ingredient_id is null or new.replacement_id is not null then
    return new;
  end if;
  loop
    select r.new_ingredient_id, r.factor into v_next, v_step
      from public.ingredient_replacements r
     where r.company_id = new.company_id and r.old_ingredient_id = v_target and r.kind = 'merge';
    exit when not found;
    v_target := v_next;
    v_factor := v_factor * v_step;
    v_depth := v_depth + 1;
    if v_depth > 16 then
      raise exception 'ingredient_merged: cadena de uniones demasiado larga' using errcode = 'EGB02';
    end if;
  end loop;
  if v_depth = 0 then
    return new;
  end if;
  v_qty := round(new.quantity * v_factor, 4);
  if v_qty = 0 then
    v_qty := sign(new.quantity) * 0.0001;
  end if;
  new.ingredient_id := v_target;
  new.quantity := v_qty;
  if new.unit_cost is not null then
    new.unit_cost := round(new.unit_cost / v_factor, 6);
  end if;
  return new;
end;
$$;
-- +goose StatementEnd
create trigger stock_movements_redirect_merged before insert on stock_movements
  for each row execute function redirect_merged_ingredient_movement();

-- El respaldo de la carrera: un insert que empezó antes de que la unión confirmara no la vio en el
-- trigger de arriba (la foto es la del inicio de la sentencia), esperó la llave foránea y aterrizó en
-- el origen. Diferido, corre al confirmar la transacción con una foto nueva, ve la unión y la
-- transacción entera se revierte: la app responde «vuelve a intentar» y el reintento entra al destino.
-- La misma función sirve inmediata (before) y diferida (constraint trigger) en las tablas que no son
-- movimientos: ahí no se redirige, se rechaza nombrando el destino.
-- +goose StatementBegin
create function reject_merged_ingredient() returns trigger
language plpgsql security definer set search_path = pg_catalog, public, pg_temp as $$
declare
  v_target text;
begin
  if new.ingredient_id is null then
    return new;
  end if;
  -- Anidado y no con «and»: PL/pgSQL no garantiza el corto circuito y las demás tablas no tienen esa
  -- columna.
  if tg_table_name = 'stock_movements' then
    if new.replacement_id is not null then
      return new;
    end if;
  end if;
  if tg_op = 'UPDATE' and new.ingredient_id is not distinct from old.ingredient_id then
    return new;
  end if;
  select t.name into v_target
    from public.ingredient_replacements r join public.ingredients t on t.id = r.new_ingredient_id
   where r.company_id = new.company_id and r.old_ingredient_id = new.ingredient_id and r.kind = 'merge';
  if found then
    raise exception 'ingredient_merged|%', v_target using errcode = 'EGB02';
  end if;
  return new;
end;
$$;
-- +goose StatementEnd
create constraint trigger stock_movements_merged_check after insert on stock_movements
  deferrable initially deferred for each row execute function reject_merged_ingredient();

-- +goose StatementBegin
do $$
declare
  tbl text;
begin
  foreach tbl in array array['recipe_items', 'expense_items', 'ingredient_presentations', 'supplier_items',
                             'ingredient_purchase_formats', 'waste_records', 'stock_count_lines',
                             'ingredient_cost_corrections'] loop
    execute format('create trigger %I before insert or update of ingredient_id on %I
                      for each row execute function reject_merged_ingredient()', tbl || '_merged_guard', tbl);
    execute format('create constraint trigger %I after insert or update of ingredient_id on %I
                      deferrable initially deferred for each row execute function reject_merged_ingredient()',
                   tbl || '_merged_check', tbl);
  end loop;
end $$;
-- +goose StatementEnd

-- Un origen unido no vuelve a activarse (la entrega 1 no deshace): la API anterior todavía ofrece
-- «Reactivar» durante la subida.
-- +goose StatementBegin
create function reject_reactivating_merged_ingredient() returns trigger
language plpgsql security definer set search_path = pg_catalog, public, pg_temp as $$
declare
  v_target text;
begin
  select t.name into v_target
    from public.ingredient_replacements r join public.ingredients t on t.id = r.new_ingredient_id
   where r.company_id = new.company_id and r.old_ingredient_id = new.id and r.kind = 'merge';
  if found then
    raise exception 'ingredient_merged|%', v_target using errcode = 'EGB02';
  end if;
  return new;
end;
$$;
-- +goose StatementEnd
create trigger ingredients_no_reactivate_merged before update of is_active on ingredients
  for each row when (new.is_active and not old.is_active) execute function reject_reactivating_merged_ingredient();

-- La copia de la marca al renglón de compra. En la base y no en Go: la API anterior no la manda y la
-- compra queda con su marca igual (y no se toca el camino de compras).
-- +goose StatementBegin
create function snapshot_presentation_brand() returns trigger
language plpgsql security definer set search_path = pg_catalog, public, pg_temp as $$
begin
  if tg_op = 'UPDATE' and new.presentation_id is not distinct from old.presentation_id then
    return new;
  end if;
  if new.presentation_id is null then
    new.presentation_brand := null;
  else
    select p.brand into new.presentation_brand
      from public.ingredient_presentations p
     where p.id = new.presentation_id and p.company_id = new.company_id;
  end if;
  return new;
end;
$$;
-- +goose StatementEnd
create trigger expense_items_snapshot_brand before insert or update of presentation_id on expense_items
  for each row execute function snapshot_presentation_brand();

revoke execute on function check_ingredient_merge(), redirect_merged_ingredient_movement(), reject_merged_ingredient(),
  reject_reactivating_merged_ingredient(), snapshot_presentation_brand() from public;

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from ingredient_replacements where kind = 'merge')
     or exists (select 1 from ingredient_presentations where brand is not null)
     or exists (select 1 from expense_items where presentation_brand is not null)
     or exists (select 1 from ingredient_distinct_pairs) then
    raise exception '0104: hay uniones, marcas o pares descartados registrados; restaura el respaldo en vez de bajar';
  end if;
end $$;
-- +goose StatementEnd
drop trigger expense_items_snapshot_brand on expense_items;
drop trigger ingredients_no_reactivate_merged on ingredients;
-- +goose StatementBegin
do $$
declare
  tbl text;
begin
  foreach tbl in array array['recipe_items', 'expense_items', 'ingredient_presentations', 'supplier_items',
                             'ingredient_purchase_formats', 'waste_records', 'stock_count_lines',
                             'ingredient_cost_corrections'] loop
    execute format('drop trigger %I on %I', tbl || '_merged_guard', tbl);
    execute format('drop trigger %I on %I', tbl || '_merged_check', tbl);
  end loop;
end $$;
-- +goose StatementEnd
drop trigger stock_movements_merged_check on stock_movements;
drop trigger stock_movements_redirect_merged on stock_movements;
drop trigger ingredient_replacements_check_merge on ingredient_replacements;
drop function snapshot_presentation_brand();
drop function reject_reactivating_merged_ingredient();
drop function reject_merged_ingredient();
drop function redirect_merged_ingredient_movement();
drop function check_ingredient_merge();
drop table ingredient_distinct_pairs;
alter table expense_items drop column presentation_brand;
create unique index ingredient_presentations_name_key on ingredient_presentations
  (company_id, ingredient_id, regexp_replace(translate(lower(btrim(name)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g'))
  where active;
drop index ingredient_presentations_name_brand_key;
alter table ingredient_presentations drop column brand;
alter table ingredient_replacements drop column kind;
