-- +goose Up

-- CADUCIDAD POR LOTE (spec 098, issue #146; research R1–R12).
--
-- Un lote es un artículo en un almacén con una fecha de caducidad y lo que le queda. Lo sin fecha no
-- se guarda: es la existencia (stock_levels.on_hand) menos los lotes. Así la existencia no cambia de
-- significado, la versión anterior de la API sigue igual, y un movimiento que la regla no asigna a un
-- lote queda solo en «sin fecha».
--
-- La regla de consumo vive aquí, en un trigger de stock_movements, a propósito (R1): quince consultas
-- insertan movimientos (venta, cancelación, compra y su corrección, merma y su aprobación y deshacer,
-- conteo, «se acabó», traspaso, unión, reemplazo, existencia inicial, compensación) y la versión
-- anterior de la API inserta los suyos durante la subida. En la base, todos pasan por lo mismo:
--   - sin fecha primero mientras sea positivo, luego los lotes buenos del que caduca antes;
--   - nunca de un lote caducado: lo que no alcanza deja lo sin fecha en negativo, sin bloquear;
--   - una reposición vuelve a los lotes de los que salió; un traspaso lleva las fechas al destino;
--   - si lo bueno queda en negativo con caducados en el almacén, la campana avisa (`expired_left`).
--
-- Nada de lo de hoy se reparte en lotes inventados: al subir, toda la existencia queda sin fecha.

set local lock_timeout = '3s';

-- Destino de la llave compuesta de las asignaciones (R12 M1): sin ella, una asignación podría apuntar
-- al movimiento de otra empresa aunque RLS fallara (las llaves foráneas saltan RLS).
alter table stock_movements add constraint stock_movements_id_company_key unique (id, company_id);

alter table stock_movements add column expires_on date;
alter table expense_items add column expires_on date;
alter table waste_records add column lot_expires_on date;

create table stock_lots (
  id            bigint generated always as identity primary key,
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                references companies (id) on delete cascade,
  branch_id     bigint not null,
  warehouse_id  bigint not null,
  ingredient_id bigint not null,
  expires_on    date not null constraint stock_lots_expires_on_range check (expires_on between date '2000-01-01' and date '2100-12-31'),
  qty           numeric(14,4) not null default 0 constraint stock_lots_qty_not_negative check (qty >= 0),
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now(),
  -- El almacén es de la sucursal del lote, aunque RLS fallara.
  constraint stock_lots_warehouse_of_branch foreign key (company_id, branch_id, warehouse_id)
    references warehouses (company_id, branch_id, id),
  constraint stock_lots_ingredient_fkey foreign key (ingredient_id, company_id) references ingredients (id, company_id),
  -- Un lote es almacén + artículo + fecha: dos compras con la misma fecha suman al mismo.
  constraint stock_lots_key unique (company_id, warehouse_id, ingredient_id, expires_on),
  constraint stock_lots_tenant_key unique (company_id, id)
);
-- Unir y cambiar de unidad buscan todos los lotes de un artículo.
create index stock_lots_by_ingredient on stock_lots (company_id, ingredient_id);

create table stock_lot_allocations (
  id          bigint generated always as identity primary key,
  company_id  bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
              references companies (id) on delete cascade,
  movement_id bigint not null,
  lot_id      bigint not null,
  -- Negativo: el movimiento tomó del lote. Positivo: le devolvió o le sumó.
  qty         numeric(14,4) not null constraint stock_lot_allocations_qty_not_zero check (qty <> 0),
  -- on delete no action, como las llaves de almacén de la 0120: un movimiento no se borra, y una
  -- empresa que se borra cascadea por companies.
  constraint stock_lot_allocations_movement_fkey foreign key (movement_id, company_id) references stock_movements (id, company_id),
  constraint stock_lot_allocations_lot_fkey foreign key (company_id, lot_id) references stock_lots (company_id, id)
);
create index stock_lot_allocations_by_movement on stock_lot_allocations (company_id, movement_id);
create index stock_lot_allocations_by_lot on stock_lot_allocations (company_id, lot_id);

create table stock_count_line_lots (
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                references companies (id) on delete cascade,
  count_id      bigint not null,
  ingredient_id bigint not null,
  expires_on    date not null constraint stock_count_line_lots_expires_on_range check (expires_on between date '2000-01-01' and date '2100-12-31'),
  qty           numeric(14,4) not null constraint stock_count_line_lots_qty_positive check (qty > 0),
  primary key (count_id, ingredient_id, expires_on),
  constraint stock_count_line_lots_count_fkey foreign key (count_id, company_id) references stock_counts (id, company_id),
  constraint stock_count_line_lots_ingredient_fkey foreign key (ingredient_id, company_id) references ingredients (id, company_id)
);

alter table stock_lots enable row level security;
create policy tenant_isolation on stock_lots
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
alter table stock_lot_allocations enable row level security;
create policy tenant_isolation on stock_lot_allocations
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
alter table stock_count_line_lots enable row level security;
create policy tenant_isolation on stock_count_line_lots
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);

-- El trigger corre como quien inserta el movimiento (gatobobah_app): necesita escribir lotes y
-- asignaciones. Update en asignaciones y en el desglose solo lo usa el cambio de unidad.
grant select, insert, update on stock_lots, stock_lot_allocations to gatobobah_app;
grant select, insert, update, delete on stock_count_line_lots to gatobobah_app;

-- Un artículo unido no recibe lotes nuevos ni desglose (las dos guardas de la 0104). La unión mueve
-- los lotes del origen al destino antes de archivarlo.
create trigger stock_lots_merged_guard before insert or update of ingredient_id on stock_lots
  for each row execute function reject_merged_ingredient();
create constraint trigger stock_lots_merged_check after insert or update of ingredient_id on stock_lots
  deferrable initially deferred for each row execute function reject_merged_ingredient();
create trigger stock_count_line_lots_merged_guard before insert or update of ingredient_id on stock_count_line_lots
  for each row execute function reject_merged_ingredient();
create constraint trigger stock_count_line_lots_merged_check after insert or update of ingredient_id on stock_count_line_lots
  deferrable initially deferred for each row execute function reject_merged_ingredient();

-- El aviso nuevo de la campana. Sin correo: el dueño lo pidió en la campana (R5).
alter table notifications drop constraint notifications_kind;
alter table notifications add constraint notifications_kind check (kind in ('stock_low', 'stock_out', 'not_found', 'expired_left'));
alter table notifications drop constraint notifications_resolved_reason;
alter table notifications add constraint notifications_resolved_reason check (resolved_reason in (
  'above_min', 'restocked', 'counted', 'arrived', 'undone', 'alert_off', 'archived',
  'merged', 'unit_changed', 'dismissed', 'created', 'alias', 'expired_cleared'));

-- +goose StatementBegin
-- business_today es el día del negocio de la empresa (su zona, México por omisión): un lote que
-- caduca el día D es bueno todo el día D en la hora del local, no hasta las 18:00 de México.
create or replace function business_today(p_company bigint) returns date
language sql stable as $$
  select (now() at time zone coalesce(nullif(
            (select bs.timezone from business_settings bs where bs.company_id = p_company), ''),
            'America/Mexico_City'))::date
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- expired_left_state dice si lo bueno de un artículo en un almacén está en negativo con caducados.
create or replace function expired_left_state(p_company bigint, p_warehouse bigint, p_ingredient bigint,
  out should_open boolean, out is_open boolean, out expired numeric)
language sql stable as $$
  with t as (select business_today(p_company) as d),
  e as (select coalesce(sum(l.qty), 0) as q from stock_lots l, t
         where l.company_id = p_company and l.warehouse_id = p_warehouse and l.ingredient_id = p_ingredient and l.expires_on < t.d),
  o as (select coalesce((select on_hand from stock_levels where warehouse_id = p_warehouse and ingredient_id = p_ingredient), 0) as q)
  select (o.q - e.q) < 0 and e.q > 0,
         exists (select 1 from notifications n where n.company_id = p_company and n.warehouse_id = p_warehouse
                   and n.item_key = p_ingredient and n.kind = 'expired_left' and n.status = 'open'),
         e.q
    from e, o
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- evaluate_expired_left abre o cierra el aviso «lo que queda está caducado». Uno abierto por almacén y
-- artículo (índice de la 0120). Lo llaman el trigger y el cierre de un conteo con fechas; la unión y
-- el cambio de fecha de un renglón no lo evalúan: el siguiente movimiento del artículo lo corrige.
create or replace function evaluate_expired_left(p_company bigint, p_warehouse bigint, p_ingredient bigint)
returns void language plpgsql as $$
declare
  s record;
begin
  select * into s from expired_left_state(p_company, p_warehouse, p_ingredient);
  if s.should_open and not s.is_open then
    insert into notifications (company_id, branch_id, warehouse_id, kind, item_type, ingredient_id, item_name, unit_code,
                               on_hand_seen, email_state)
    select p_company, w.branch_id, w.id, 'expired_left', 'ingrediente', i.id, left(i.name, 200), left(u.code, 20),
           (select on_hand from stock_levels where warehouse_id = w.id and ingredient_id = i.id), 'skipped'
      from warehouses w, ingredients i join units u on u.id = i.base_unit_id
     where w.id = p_warehouse and i.id = p_ingredient
    on conflict do nothing;
  elsif s.is_open and not s.should_open then
    update notifications set status = 'resolved', resolved_at = now(),
           resolved_reason = case when s.expired = 0 then 'expired_cleared' else 'restocked' end
     where company_id = p_company and warehouse_id = p_warehouse and item_key = p_ingredient
       and kind = 'expired_left' and status = 'open';
  end if;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- lot_put suma a un lote (lo crea si no existe) y deja la asignación.
create or replace function lot_put(p_movement bigint, p_company bigint, p_branch bigint, p_warehouse bigint,
  p_ingredient bigint, p_expires date, p_qty numeric) returns void language plpgsql as $$
declare
  v_lot bigint;
begin
  if p_qty <= 0 then
    return;
  end if;
  insert into stock_lots as l (company_id, branch_id, warehouse_id, ingredient_id, expires_on, qty)
  values (p_company, p_branch, p_warehouse, p_ingredient, p_expires, p_qty)
  on conflict (company_id, warehouse_id, ingredient_id, expires_on)
    do update set qty = l.qty + excluded.qty, updated_at = now()
  returning id into v_lot;
  insert into stock_lot_allocations (company_id, movement_id, lot_id, qty) values (p_company, p_movement, v_lot, p_qty);
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- relocate_lot pasa lo que un renglón de compra ya sumó de una fecha a otra (nula = sin fecha), sin
-- movimiento: la existencia no cambia, solo su desglose (R8). Toma primero la fila de stock_levels,
-- como todo escritor de lotes. Un lote de origen con menos de lo pedido da lo que tiene.
create or replace function relocate_lot(p_company bigint, p_warehouse bigint, p_ingredient bigint,
  p_from date, p_to date, p_qty numeric) returns void language plpgsql as $$
declare
  v_qty  numeric := p_qty;
  v_free numeric;
  r      record;
begin
  if p_qty <= 0 or p_from is not distinct from p_to then
    return;
  end if;
  perform 1 from stock_levels where warehouse_id = p_warehouse and ingredient_id = p_ingredient for update;
  if p_from is null then
    -- De lo sin fecha solo pasa lo que queda: lo vendido de ahí ya no existe y no se le pone fecha.
    select greatest(0, coalesce((select on_hand from stock_levels where warehouse_id = p_warehouse and ingredient_id = p_ingredient), 0)
                       - coalesce((select sum(l.qty) from stock_lots l where l.company_id = p_company
                                     and l.warehouse_id = p_warehouse and l.ingredient_id = p_ingredient), 0))
      into v_free;
    v_qty := least(p_qty, v_free);
  else
    select l.id, l.qty into r from stock_lots l where l.company_id = p_company and l.warehouse_id = p_warehouse
       and l.ingredient_id = p_ingredient and l.expires_on = p_from for update;
    if not found or r.qty <= 0 then
      v_qty := 0;
    else
      v_qty := least(r.qty, p_qty);
      update stock_lots set qty = qty - v_qty, updated_at = now() where id = r.id;
    end if;
  end if;
  if p_to is not null and v_qty > 0 then
    insert into stock_lots as l (company_id, branch_id, warehouse_id, ingredient_id, expires_on, qty)
    select p_company, w.branch_id, w.id, p_ingredient, p_to, v_qty from warehouses w where w.id = p_warehouse
    on conflict (company_id, warehouse_id, ingredient_id, expires_on)
      do update set qty = l.qty + excluded.qty, updated_at = now();
  end if;
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- apply_count_lots fija los lotes de un artículo contado con fechas (R6): cada lote del almacén queda en
-- lo contado de su fecha (0 si no se mencionó) más lo que movimientos posteriores a la captura le
-- tomaron o devolvieron (neto). Es la misma regla con la que el conteo ajusta la existencia: contra
-- lo que había al capturar. Quien llama ya tomó la fila de stock_levels.
create or replace function apply_count_lots(p_company bigint, p_count bigint, p_ingredient bigint)
returns void language plpgsql as $$
declare
  v_wh     bigint;
  v_branch bigint;
  v_last   bigint;
begin
  select c.warehouse_id, c.branch_id into v_wh, v_branch from stock_counts c where c.id = p_count;
  select coalesce(cl.last_movement_id, 0) into v_last from stock_count_lines cl
   where cl.count_id = p_count and cl.ingredient_id = p_ingredient;
  insert into stock_lots (company_id, branch_id, warehouse_id, ingredient_id, expires_on, qty)
  select p_company, v_branch, v_wh, p_ingredient, b.expires_on, 0
    from stock_count_line_lots b where b.count_id = p_count and b.ingredient_id = p_ingredient
  on conflict (company_id, warehouse_id, ingredient_id, expires_on) do nothing;
  update stock_lots l set updated_at = now(), qty = greatest(0,
         coalesce((select b.qty from stock_count_line_lots b
                    where b.count_id = p_count and b.ingredient_id = p_ingredient and b.expires_on = l.expires_on), 0)
       + coalesce((select sum(a.qty) from stock_lot_allocations a join stock_movements m on m.id = a.movement_id
                    where a.company_id = p_company and a.lot_id = l.id and m.id > coalesce(v_last, 0)
                      and m.stock_count_id is distinct from p_count), 0))
   where l.company_id = p_company and l.warehouse_id = v_wh and l.ingredient_id = p_ingredient;
  perform evaluate_expired_left(p_company, v_wh, p_ingredient);
end;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- apply_stock_lots aplica la regla de R4 a cada movimiento de artículo. Corre después de
-- trg_stock_movement (que ya tomó y movió la fila de stock_levels de este almacén y artículo, así que
-- todo escritor de estos lotes queda en fila detrás de ella) y antes del aviso de existencias.
create or replace function apply_stock_lots() returns trigger language plpgsql as $$
declare
  v_exp     date := new.expires_on;
  v_need    numeric;
  v_take    numeric;
  v_undated numeric;
  v_today   date;
  v_touched boolean := false;
  r         record;
  s         record;
begin
  -- Productos y los traspasos de una unión o de un cambio de unidad (los lotes los mueve el servicio).
  if new.item_type <> 'ingrediente' or new.replacement_id is not null then
    return null;
  end if;

  -- La fecha sale de su origen si el movimiento no la trae (R12 A2): la versión anterior de la API no
  -- conoce la columna y aprueba mermas o corrige renglones fechados igual. Solo si el movimiento es del
  -- artículo de la fila: la merma de un preparado resta a sus insumos, que no son de ese lote.
  -- Solo una salida: deshacer la merma (positiva) devuelve por sus asignaciones lo que cada lote y lo sin
  -- fecha prestaron; con la fecha del registro, todo iría al lote.
  if v_exp is null and new.waste_record_id is not null and new.quantity < 0 then
    select w.lot_expires_on into v_exp from waste_records w
     where w.id = new.waste_record_id and w.ingredient_id = new.ingredient_id;
  end if;
  if v_exp is null and new.expense_item_id is not null then
    select e.expires_on into v_exp from expense_items e
     where e.id = new.expense_item_id and e.ingredient_id = new.ingredient_id;
  end if;

  if v_exp is not null then
    -- Directo a ese lote. Un negativo baja hasta 0; lo que no alcanza sale de lo sin fecha.
    if new.quantity > 0 then
      perform lot_put(new.id, new.company_id, new.branch_id, new.warehouse_id, new.ingredient_id, v_exp, new.quantity);
    else
      select l.id, l.qty into r from stock_lots l
       where l.company_id = new.company_id and l.warehouse_id = new.warehouse_id
         and l.ingredient_id = new.ingredient_id and l.expires_on = v_exp
       for update;
      if found and r.qty > 0 and new.quantity < 0 then
        v_take := least(r.qty, -new.quantity);
        update stock_lots set qty = qty - v_take, updated_at = now() where id = r.id;
        insert into stock_lot_allocations (company_id, movement_id, lot_id, qty) values (new.company_id, new.id, r.id, -v_take);
      end if;
    end if;
    v_touched := true;

  elsif new.quantity > 0 then
    -- Una reposición vuelve a los lotes de los que salió, lo último primero (FR-023).
    -- `::text`: 'merma_esperada' (spec 096, subida 2) entra al enum en una migración posterior; como
    -- texto, la comparación no truena mientras el valor no exista.
    if new.movement_type::text in ('cancelacion', 'merma_esperada') and new.order_id is not null then
      v_need := new.quantity;
      for r in
        select a.lot_id, -sum(a.qty) as owed
          from stock_movements m join stock_lot_allocations a on a.movement_id = m.id and a.company_id = m.company_id
         where m.company_id = new.company_id and m.order_id = new.order_id and m.id <> new.id
           and m.ingredient_id = new.ingredient_id and m.warehouse_id = new.warehouse_id
           and m.movement_type::text in ('venta', 'cancelacion', 'merma_esperada')
           and m.order_line_id is not distinct from new.order_line_id
           and m.modifier_option_id is not distinct from new.modifier_option_id
           and m.component_of_product_id is not distinct from new.component_of_product_id
         group by a.lot_id
        having sum(a.qty) < 0
         order by max(a.id) desc
      loop
        exit when v_need <= 0;
        v_take := least(r.owed, v_need);
        update stock_lots set qty = qty + v_take, updated_at = now() where id = r.lot_id;
        insert into stock_lot_allocations (company_id, movement_id, lot_id, qty) values (new.company_id, new.id, r.lot_id, v_take);
        v_need := v_need - v_take;
        v_touched := true;
      end loop;
    elsif new.movement_type = 'merma' and new.waste_record_id is not null then
      v_need := new.quantity;
      for r in
        select a.lot_id, -sum(a.qty) as owed
          from stock_movements m join stock_lot_allocations a on a.movement_id = m.id and a.company_id = m.company_id
         where m.company_id = new.company_id and m.waste_record_id = new.waste_record_id and m.id <> new.id
           and m.ingredient_id = new.ingredient_id and m.warehouse_id = new.warehouse_id
         group by a.lot_id
        having sum(a.qty) < 0
         order by max(a.id) desc
      loop
        exit when v_need <= 0;
        v_take := least(r.owed, v_need);
        update stock_lots set qty = qty + v_take, updated_at = now() where id = r.lot_id;
        insert into stock_lot_allocations (company_id, movement_id, lot_id, qty) values (new.company_id, new.id, r.lot_id, v_take);
        v_need := v_need - v_take;
        v_touched := true;
      end loop;
    elsif new.movement_type = 'traspaso' and new.transfer_id is not null then
      -- La pata de entrada crea en el destino lo que la de salida tomó de cada lote, con su fecha.
      for r in
        select l.expires_on, -sum(a.qty) as moved
          from stock_movements m
          join stock_lot_allocations a on a.movement_id = m.id and a.company_id = m.company_id
          join stock_lots l on l.id = a.lot_id
         where m.company_id = new.company_id and m.transfer_id = new.transfer_id and m.quantity < 0
           and m.ingredient_id = new.ingredient_id
         group by l.expires_on
        having sum(a.qty) < 0
      loop
        perform lot_put(new.id, new.company_id, new.branch_id, new.warehouse_id, new.ingredient_id, r.expires_on, r.moved);
        v_touched := true;
      end loop;
    end if;
    -- Cualquier otro positivo suma a lo sin fecha; si había un aviso abierto, puede cerrarlo
    -- (llegó existencia buena que cubre el negativo).
    v_touched := v_touched or exists (select 1 from notifications n where n.company_id = new.company_id
      and n.warehouse_id = new.warehouse_id and n.item_key = new.ingredient_id and n.kind = 'expired_left' and n.status = 'open');

  else
    -- Una salida sin lote indicado: sin fecha primero, luego por fecha, nunca caducado (R3).
    if not exists (select 1 from stock_lots l where l.company_id = new.company_id
                     and l.warehouse_id = new.warehouse_id and l.ingredient_id = new.ingredient_id) then
      return null;
    end if;
    -- Lo sin fecha ANTES de este movimiento: el trigger de existencias ya lo sumó (R12 M2).
    select coalesce((select on_hand from stock_levels where warehouse_id = new.warehouse_id and ingredient_id = new.ingredient_id), 0)
           - new.quantity
           - coalesce((select sum(qty) from stock_lots l where l.company_id = new.company_id
                         and l.warehouse_id = new.warehouse_id and l.ingredient_id = new.ingredient_id), 0)
      into v_undated;
    v_need := -new.quantity - greatest(v_undated, 0);
    if v_need <= 0 then
      -- Lo sin fecha alcanzó: lo bueno sigue positivo y no hay aviso que abrir ni cerrar.
      return null;
    end if;
    v_today := business_today(new.company_id);
    for r in
      select l.id, l.qty from stock_lots l
       where l.company_id = new.company_id and l.warehouse_id = new.warehouse_id
         and l.ingredient_id = new.ingredient_id and l.qty > 0 and l.expires_on >= v_today
       order by l.expires_on, l.id
       for update
    loop
      exit when v_need <= 0;
      v_take := least(r.qty, v_need);
      update stock_lots set qty = qty - v_take, updated_at = now() where id = r.id;
      insert into stock_lot_allocations (company_id, movement_id, lot_id, qty) values (new.company_id, new.id, r.id, -v_take);
      v_need := v_need - v_take;
    end loop;
    -- Solo si lo bueno no alcanzó puede tocar abrir el aviso; si alcanzó, solo hay que mirar si había
    -- uno abierto (una salida no lo cierra, pero así queda una sola regla).
    v_touched := v_need > 0 or exists (select 1 from notifications n where n.company_id = new.company_id
      and n.warehouse_id = new.warehouse_id and n.item_key = new.ingredient_id and n.kind = 'expired_left' and n.status = 'open');
  end if;

  if v_touched then
    -- Solo si el estado difiere: cada bloque con excepción es una subtransacción (ver la 0127).
    select * into s from expired_left_state(new.company_id, new.warehouse_id, new.ingredient_id);
    if s.should_open <> s.is_open then
      begin
        perform evaluate_expired_left(new.company_id, new.warehouse_id, new.ingredient_id);
      exception when others then
        -- La venta nunca falla por el aviso; el siguiente movimiento lo vuelve a intentar.
        raise warning 'notification_failed: % (%)', sqlerrm, sqlstate;
      end;
    end if;
  end if;
  return null;
end;
$$;
-- +goose StatementEnd

-- El nombre ordena el trigger entre trg_stock_movement (existencias) y trg_stock_movement_notify.
create trigger trg_stock_movement_lots after insert on stock_movements
  for each row execute function apply_stock_lots();

-- +goose Down

set local lock_timeout = '3s';

drop trigger trg_stock_movement_lots on stock_movements;
drop function apply_stock_lots();
drop function lot_put(bigint, bigint, bigint, bigint, bigint, date, numeric);
drop function relocate_lot(bigint, bigint, bigint, date, date, numeric);
drop function apply_count_lots(bigint, bigint, bigint);
drop function evaluate_expired_left(bigint, bigint, bigint);
drop function expired_left_state(bigint, bigint, bigint);
drop function business_today(bigint);

-- El check viejo rechaza el aviso nuevo y su razón de cierre: se borran antes de devolverlo.
delete from notifications where kind = 'expired_left';
alter table notifications drop constraint notifications_kind;
alter table notifications add constraint notifications_kind check (kind in ('stock_low', 'stock_out', 'not_found'));
alter table notifications drop constraint notifications_resolved_reason;
alter table notifications add constraint notifications_resolved_reason check (resolved_reason in (
  'above_min', 'restocked', 'counted', 'arrived', 'undone', 'alert_off', 'archived',
  'merged', 'unit_changed', 'dismissed', 'created', 'alias'));

drop table stock_count_line_lots;
drop table stock_lot_allocations;
drop table stock_lots;
alter table waste_records drop column lot_expires_on;
alter table expense_items drop column expires_on;
alter table stock_movements drop column expires_on;
alter table stock_movements drop constraint stock_movements_id_company_key;
