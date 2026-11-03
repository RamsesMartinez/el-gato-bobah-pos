-- +goose Up
-- Densidad y tara (spec 058). Solo agrega columnas nulas y reglas sobre ellas: la API anterior no las
-- lee ni las escribe, y sus inserts siguen valiendo durante la subida azul/verde.

-- La densidad vigente del insumo, con quién y cómo se puso. Su historia vive en la bitácora.
alter table ingredients
  add column density_g_per_ml numeric(8,4)
    constraint ingredients_density_range check (density_g_per_ml between 0.3 and 3),
  add column density_method text
    constraint ingredients_density_method check (density_method in ('weighing', 'label', 'manual')),
  add column density_set_by bigint,
  add column density_set_at timestamptz,
  add constraint ingredients_density_all_or_none check (
    (density_g_per_ml is null) = (density_method is null)
    and (density_g_per_ml is null) = (density_set_by is null)
    and (density_g_per_ml is null) = (density_set_at is null));

-- La FK compuesta va con EXECUTE: `ingredients.company_id` se agregó con DDL dinámico (0023) y sqlc,
-- que lee estas migraciones, no la conoce. `no action`: un `set null` anularía también company_id.
-- +goose StatementBegin
do $$
begin
  execute 'alter table ingredients add constraint ingredients_density_set_by_fkey
             foreign key (company_id, density_set_by) references users (company_id, id)';
end $$;
-- +goose StatementEnd

-- Lo que pesa vacío un empaque, para contar pesando con el producto adentro.
alter table ingredient_presentations
  add column tare_g numeric(10,2)
    constraint ingredient_presentations_tare_range check (tare_g > 0 and tare_g <= 100000);

-- La densidad con que un renglón de compra se convirtió la primera vez. Rehacer la pesada no puede
-- mover lo que ya entró al almacén (decisión del dueño, 2026-11-02): sin esta copia, corregir el
-- renglón después recalcularía con la densidad nueva.
alter table expense_items
  add column density_g_per_ml numeric(8,4)
    constraint expense_items_density_range check (density_g_per_ml between 0.3 and 3);

-- Lo pesado en un conteo: explica después de dónde salió la cifra contada (puerta VIII: un faltante
-- por una tara mal capturada no se distingue de una pérdida si no queda registrado).
alter table stock_count_lines
  add column gross_g numeric(14,4) constraint stock_count_lines_gross_range check (gross_g >= 0),
  add column gross_containers smallint constraint stock_count_lines_containers_range check (gross_containers between 1 and 999),
  add column tare_g_used numeric(10,2) constraint stock_count_lines_tare_range check (tare_g_used > 0),
  add column density_used numeric(8,4) constraint stock_count_lines_density_range check (density_used between 0.3 and 3),
  add constraint stock_count_lines_gross_together check (
    (gross_g is null) = (gross_containers is null) and (gross_g is null) = (tare_g_used is null)
    and (density_used is null or gross_g is not null));

-- Solo un insumo en g o ml tiene densidad y taras. Lo impone la base, no el servicio: la API anterior
-- (durante la subida), un script o el cambio de unidad pueden pasar un insumo a piezas o a cm.
-- +goose StatementBegin
create function clear_density_on_unit_kind_change() returns trigger
language plpgsql as $$
declare
  new_kind unit_kind;
begin
  if new.base_unit_id is not distinct from old.base_unit_id then
    return new;
  end if;
  select kind into new_kind from units where id = new.base_unit_id;
  if new_kind not in ('masa', 'volumen') then
    new.density_g_per_ml := null;
    new.density_method := null;
    new.density_set_by := null;
    new.density_set_at := null;
    update ingredient_presentations set tare_g = null
     where ingredient_id = new.id and company_id = new.company_id and tare_g is not null;
  end if;
  return new;
end;
$$;
-- +goose StatementEnd
create trigger ingredients_clear_density_on_unit_change
  before update of base_unit_id on ingredients
  for each row execute function clear_density_on_unit_kind_change();

-- La regla del empaque gana la tara: solo en insumos de masa o volumen.
-- +goose StatementBegin
create or replace function ingredient_presentations_before_write() returns trigger
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

  -- Spec 058: lo que pesa vacío un empaque solo sirve para contar pesando un insumo en g o ml. Se
  -- limpia en vez de rechazar: un empaque que llega a un insumo en piezas (una unión de insumos que se
  -- miden distinto, un cambio de unidad) no debe tumbar la operación por un dato que ahí no aplica. El
  -- servicio rechaza con 400 a quien la captura a propósito.
  if new.tare_g is not null and base_kind not in ('masa', 'volumen') then
    new.tare_g := null;
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

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from ingredients where density_g_per_ml is not null)
     or exists (select 1 from ingredient_presentations where tare_g is not null)
     or exists (select 1 from expense_items where density_g_per_ml is not null)
     or exists (select 1 from stock_count_lines where gross_g is not null)
     or exists (select 1 from expense_items ei
                  join units bu on bu.id = ei.unit_id
                  join ingredients i on i.id = ei.ingredient_id
                  join units iu on iu.id = i.base_unit_id
                 where ei.voided_at is null and ei.presentation_id is null
                   and bu.kind in ('masa', 'volumen') and iu.kind in ('masa', 'volumen') and bu.kind <> iu.kind) then
    raise exception '0105: hay densidades, taras, pesadas o compras entre peso y líquido; restaura el respaldo en vez de bajar';
  end if;
end $$;
-- +goose StatementEnd
drop trigger ingredients_clear_density_on_unit_change on ingredients;
drop function clear_density_on_unit_kind_change();
-- +goose StatementBegin
create or replace function ingredient_presentations_before_write() returns trigger
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

  if new.is_approximate is distinct from (purchase_kind is not null and purchase_kind <> base_kind) then
    raise exception 'un empaque es aproximado solo si se compra en otro tipo de unidad que el insumo'
      using errcode = 'check_violation';
  end if;

  if base_kind = 'pieza' and new.qty_in_base <> trunc(new.qty_in_base) then
    raise exception 'un empaque de un insumo por pieza trae piezas enteras'
      using errcode = 'check_violation';
  end if;

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
alter table stock_count_lines
  drop constraint stock_count_lines_gross_together,
  drop column gross_g, drop column gross_containers, drop column tare_g_used, drop column density_used;
alter table expense_items drop column density_g_per_ml;
alter table ingredient_presentations drop column tare_g;
alter table ingredients
  drop constraint ingredients_density_set_by_fkey,
  drop constraint ingredients_density_all_or_none,
  drop column density_g_per_ml, drop column density_method, drop column density_set_by, drop column density_set_at;
