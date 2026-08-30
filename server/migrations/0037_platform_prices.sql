-- +goose Up
-- Venta por plataformas digitales (spec 002). Tres cosas en una migración porque no se pueden
-- separar: los precios por plataforma no sirven sin poder ligar el método de pago a su plataforma,
-- y esa liga es imposible mientras payment_methods sea global y delivery_platforms sea per-tenant.
--
-- La parte de payment_methods es la ÚNICA de esta migración con dinero real apuntándole. El orden
-- de sus pasos no es negociable y está comentado abajo.

-- ---------------------------------------------------------------------------------------------
-- 1. Margen por plataforma
-- ---------------------------------------------------------------------------------------------
-- Default 0 a propósito: una plataforma creada después no debe empezar cobrando 35% más sin que
-- nadie lo pidiera. El tope de 500% es cota de cordura — sin él, un dedo de más convierte un
-- producto de $100 en uno de $600 y el error se ve hasta el ticket.
alter table delivery_platforms
  add column price_markup_pct numeric(5,2) not null default 0
  constraint delivery_platforms_markup_range check (price_markup_pct >= 0 and price_markup_pct <= 500);

-- Corre como el owner, así que RLS NO aplica: alcanza a las empresas que existen hoy, que es lo
-- buscado. Una empresa creada después toma el default 0, y eso es lo correcto: el margen se define
-- cuando ese negocio hace su propia vinculación con la plataforma, no antes.
-- 'Propio' queda en 0: es reparto del propio negocio, sin comisión que absorber.
update delivery_platforms set price_markup_pct = 35.00 where name in ('Didi', 'Uber Eats', 'Rappi');

-- ---------------------------------------------------------------------------------------------
-- 2. Excepciones de precio (productos y opciones de modificador)
-- ---------------------------------------------------------------------------------------------
-- Solo existen las EXCEPCIONES: sin fila, el precio es base × (1 + margen). Materializar los 502
-- productos × 3 plataformas serían 1,506 filas que se vuelven obsoletas en el primer cambio de
-- precio base, y nadie se entera.
--
-- La PK no lleva company_id y es correcto: las dos columnas son FKs a tablas per-tenant con
-- identity global, así que dos empresas nunca comparten un product_id ni un platform_id. NO es el
-- caso de categories_name_scope (arreglado en 0036), donde el índice incluía el literal 0 del
-- coalesce(parent_id,0), que sí era un valor compartido. Mismo patrón que product_channels_pkey.
create table product_platform_prices (
  product_id  bigint not null references products(id) on delete cascade,
  -- Sin on delete: borrar una plataforma no debe llevarse en silencio una lista de precios curada
  -- a mano. Ninguna FK a una tabla de lookup cascadea en este esquema (product_channels → channels
  -- es no action), y el único caso donde dispararía es justo ese.
  platform_id smallint not null references delivery_platforms(id),
  price       numeric(10,2) not null check (price > 0),
  -- not null y sin on delete, igual que las otras 15 FK *_by del esquema: es el rastro que
  -- justifica dejar que un cajero escriba precios. Los usuarios se desactivan, no se borran, así
  -- que un `set null` nunca dispararía y lo único que lograría es hacer la columna nullable.
  updated_by  bigint not null references users(id),
  updated_at  timestamptz not null default now(),
  company_id  bigint not null default current_setting('app.company_id', true)::bigint
              references companies(id) on delete cascade,
  primary key (product_id, platform_id)
);
-- El primero cubre listar la lista completa de una plataforma sin tocar la tabla; el segundo es el
-- patrón de las 33 tablas per-tenant desde 0023, y el que usa RLS al pegarle company_id a todo.
create index product_platform_prices_platform on product_platform_prices (platform_id, product_id, price);
create index product_platform_prices_company  on product_platform_prices (company_id);

create table modifier_option_platform_prices (
  option_id   bigint not null references modifier_options(id) on delete cascade,
  platform_id smallint not null references delivery_platforms(id),
  -- >= 0 y no > 0 como en productos: un extra sin costo ("sin cebolla") es normal y su delta es 0;
  -- un producto en $0 siempre es un error de captura.
  price_delta numeric(10,2) not null check (price_delta >= 0),
  updated_by  bigint not null references users(id),
  updated_at  timestamptz not null default now(),
  company_id  bigint not null default current_setting('app.company_id', true)::bigint
              references companies(id) on delete cascade,
  primary key (option_id, platform_id)
);
create index modifier_option_platform_prices_platform on modifier_option_platform_prices (platform_id, option_id, price_delta);
create index modifier_option_platform_prices_company  on modifier_option_platform_prices (company_id);

-- updated_at por trigger y no por la query: un upsert que olvide setearlo dejaría la auditoría
-- congelada en la fecha de la primera captura (patrón de 0009).
create trigger trg_product_platform_prices_updated before update on product_platform_prices
  for each row execute function set_updated_at();
create trigger trg_modifier_option_platform_prices_updated before update on modifier_option_platform_prices
  for each row execute function set_updated_at();

alter table product_platform_prices enable row level security;
create policy tenant_isolation on product_platform_prices
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);
alter table modifier_option_platform_prices enable row level security;
create policy tenant_isolation on modifier_option_platform_prices
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- El grant de 0024 fue `on all tables in schema public`, que es PUNTUAL: no hay default
-- privileges, así que cada tabla creada después necesita el suyo (0025, 0026, 0028, 0029, 0030
-- hacen lo mismo). Sin esto la migración pasa, los tests pasan y `make start` pasa —dev sirve como
-- owner, sin RLS ni grants— y en producción el primer request devuelve 42501 permission denied.
grant select, insert, update, delete on product_platform_prices         to gatobobah_app;
grant select, insert, update, delete on modifier_option_platform_prices to gatobobah_app;

-- ---------------------------------------------------------------------------------------------
-- 3. payment_methods pasa a ser per-tenant y se liga a su plataforma
-- ---------------------------------------------------------------------------------------------
-- POR QUÉ: payment_methods era global y delivery_platforms es per-tenant, así que no había forma
-- de contestar "¿qué métodos son de Uber Eats?" salvo comparando nombres — frágil y silencioso:
-- renombrar una plataforma rompía la validación del cobro sin que nada avisara.
--
-- De paso cierra un bug de aislamiento vivo: UpdatePaymentMethodAutoDeclare hace
-- `update ... where id = $1` sin filtro de empresa sobre una tabla global, así que hoy el admin de
-- una empresa puede cambiarle la configuración de cobro a otra. Con RLS eso se cierra solo.
--
-- Es la parte con DINERO: order_payments, expense_payments y register_session_totals la
-- referencian. El orden de abajo está elegido para que ningún paso deje la base en un estado
-- inconsistente, y el paso de verificación aborta la transacción entera si algo no cuadra.

alter table payment_methods add column company_id bigint references companies(id) on delete cascade;
alter table payment_methods add column src_id smallint;  -- temporal: mapea copia → original

-- +goose StatementBegin
do $mig$
declare
  v_base       bigint;
  v_antes_op   bigint; v_antes_ep bigint; v_antes_rst bigint;
  v_suma_op    numeric; v_suma_ep numeric;
  v_despues_op bigint; v_despues_ep bigint; v_despues_rst bigint;
  v_cruzadas   bigint;
  v_por_empresa bigint;
begin
  -- Fotografía ANTES. Lo que importa no es que la migración "corra", sino que no se mueva ni una
  -- fila ni un peso: si al final no cuadra, se aborta y la transacción entera se va para atrás.
  select count(*), coalesce(sum(amount),0) into v_antes_op, v_suma_op from order_payments;
  select count(*), coalesce(sum(amount),0) into v_antes_ep, v_suma_ep from expense_payments;
  select count(*) into v_antes_rst from register_session_totals;

  -- La empresa dueña de las filas actuales es la de MENOR id, no la del slug 'gatobobah'. El
  -- backfill de 0023 sí usaba el slug, pero el corte a producción se lo pasó a la empresa NUEVA:
  -- hoy 'gatobobah' es la id 2 (estrenada, con 1 venta) y los 55 pagos históricos son de la id 1
  -- ('bobah-pruebas'). El id más bajo es la única forma estable de nombrar a la original.
  select min(id) into v_base from companies;
  if v_base is null then
    raise exception 'No hay ninguna empresa: no se puede decidir de quién son los métodos actuales';
  end if;

  update payment_methods set company_id = v_base where company_id is null;

  -- El unique(name) GLOBAL se cambia AQUÍ, antes de copiar. Al revés, el primer insert de la
  -- segunda empresa truena con 23505 — y no se ve en una base con una sola empresa, así que la
  -- migración pasaría verde en local y en CI para morir en el VPS.
  alter table payment_methods drop constraint payment_methods_name_key;
  alter table payment_methods add constraint payment_methods_company_name_key unique (company_id, name);

  -- Copia por empresa. NO se lista `id`: es `generated always as identity` y listarlo daría 428C9
  -- sin `overriding system value`. El mapeo copia→original viaja en src_id.
  insert into payment_methods (company_id, name, kind, affects_cash_drawer, is_active, sort_key, auto_declare, src_id)
  select c.id, pm.name, pm.kind, pm.affects_cash_drawer, pm.is_active, pm.sort_key, pm.auto_declare, pm.id
  from companies c
  cross join payment_methods pm
  where pm.company_id = v_base and c.id <> v_base;

  -- Remapeo por (company_id, src_id) y NUNCA por nombre: citext ignora mayúsculas pero NO acentos
  -- ni espacios ('Tarjeta débito' <> 'Tarjeta debito'), y el match por nombre solo funcionaría
  -- porque hoy las copias son idénticas byte a byte — una propiedad de la copia, no del diseño.
  update order_payments op set payment_method_id = nuevo.id
  from payment_methods nuevo
  where nuevo.company_id = op.company_id and nuevo.src_id = op.payment_method_id;

  update expense_payments ep set payment_method_id = nuevo.id
  from payment_methods nuevo
  where nuevo.company_id = ep.company_id and nuevo.src_id = ep.payment_method_id;

  update register_session_totals rst set payment_method_id = nuevo.id
  from payment_methods nuevo
  where nuevo.company_id = rst.company_id and nuevo.src_id = rst.payment_method_id;

  -- Verificación: ni una fila ni un peso de diferencia, y cero filas apuntando al método de otra
  -- empresa. La lista de tablas dependientes se enumera desde pg_constraint y no a mano: el Down
  -- de 0029 re-agrega expenses.payment_method_id, así que "son tres" es cierto hoy y no es una
  -- invariante.
  select count(*), coalesce(sum(amount),0) into v_despues_op, v_suma_op from order_payments;
  if v_despues_op <> v_antes_op then
    raise exception 'order_payments cambió de % a % filas', v_antes_op, v_despues_op;
  end if;
  select count(*) into v_despues_ep from expense_payments;
  if v_despues_ep <> v_antes_ep then
    raise exception 'expense_payments cambió de % a % filas', v_antes_ep, v_despues_ep;
  end if;
  select count(*) into v_despues_rst from register_session_totals;
  if v_despues_rst <> v_antes_rst then
    raise exception 'register_session_totals cambió de % a % filas', v_antes_rst, v_despues_rst;
  end if;

  select count(*) into v_cruzadas from (
    select 1 from order_payments x join payment_methods m on m.id = x.payment_method_id
      where m.company_id <> x.company_id
    union all
    select 1 from expense_payments x join payment_methods m on m.id = x.payment_method_id
      where m.company_id <> x.company_id
    union all
    select 1 from register_session_totals x join payment_methods m on m.id = x.payment_method_id
      where m.company_id <> x.company_id
  ) z;
  if v_cruzadas > 0 then
    raise exception '% filas siguen apuntando al método de pago de otra empresa', v_cruzadas;
  end if;

  -- Todas las empresas deben terminar con el mismo catálogo de métodos.
  select count(distinct n) into v_por_empresa from (
    select company_id, count(*) n from payment_methods group by company_id
  ) z;
  if v_por_empresa <> 1 then
    raise exception 'las empresas quedaron con distinto número de métodos de pago';
  end if;

  raise notice 'payment_methods per-tenant: % pagos de venta y % de gasto remapeados sin cambio de monto',
    v_antes_op, v_antes_ep;
end
$mig$;
-- +goose StatementEnd

alter table payment_methods alter column company_id set not null;
alter table payment_methods alter column company_id set default current_setting('app.company_id', true)::bigint;
alter table payment_methods drop column src_id;
create index payment_methods_company on payment_methods (company_id);

-- delivery_platforms gana un unique (id, company_id) para poder colgar la FK COMPUESTA de abajo.
-- Aquí sí vale la pena, a diferencia del riesgo residual que se acepta en las tablas de precios:
-- esta columna AGRUPA DINERO REAL en el corte, y un método apuntando a la plataforma de otra
-- empresa rompe el subtotal bajo RLS sin dar error.
alter table delivery_platforms add constraint delivery_platforms_id_company_key unique (id, company_id);
alter table payment_methods add column delivery_platform_id smallint;
alter table payment_methods add constraint payment_methods_platform_fkey
  foreign key (delivery_platform_id, company_id) references delivery_platforms (id, company_id);

alter table payment_methods enable row level security;
create policy tenant_isolation on payment_methods
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);
-- El grant de 0024 ya cubría payment_methods (existía desde 0002); no hace falta reemitirlo.

-- ---------------------------------------------------------------------------------------------
-- 4. Los seis métodos de plataforma
-- ---------------------------------------------------------------------------------------------
-- Va DESPUÉS del remapeo y para TODAS las empresas: hacerlo antes rompería el match del paso
-- anterior, y hacerlo solo para una dejaría catálogos divergentes que nadie nota hasta el primer
-- corte con plataformas.
--
-- Los repartidores de las tres a veces pagan EN EFECTIVO en el mostrador, y ese dinero entra al
-- cajón físico. Cobrarlo con un método que no toca el cajón deja un sobrante que nadie sabe
-- explicar al cerrar el turno. Por eso cada plataforma tiene dos.
--
-- sort_key 400/450, 500/550, 600/650: ExpectedByMethodSince ordena por sort_key, y con el mismo
-- valor el orden dentro del par quedaría no determinista en el corte.

-- Los tres actuales no tienen ni un pago real (solo salen en $0 en dos cortes viejos), así que
-- renombrarlos conserva su id y no rompe histórico.
--
-- El auto_declare se fija EXPLÍCITO en vez de heredar lo que hubiera: en producción está en true
-- porque alguien lo activó desde la interfaz, pero en una base recién migrada los seeds lo dejan en
-- false. Sin esta línea, una instalación nueva pediría contar físicamente el dinero de Uber, que
-- nunca pasa por el cajón. El de efectivo se queda en false, que es lo correcto: esos sí son
-- billetes que se cuentan.
update payment_methods
   set name = name || ' en línea', auto_declare = true, affects_cash_drawer = false
 where name in ('Didi', 'Uber Eats', 'Rappi');

-- +goose StatementBegin
do $seed$
declare
  r record;
begin
  -- Se liga cada método a la plataforma DE SU MISMA EMPRESA. Sin el filtro por empresa el
  -- subselect tomaría la plataforma equivocada (o reventaría, con más de una empresa).
  for r in
    select pm.id as metodo_id, pm.company_id, dp.id as plataforma_id, pm.name, pm.sort_key
    from payment_methods pm
    join delivery_platforms dp
      on dp.company_id = pm.company_id
     and pm.name = dp.name || ' en línea'
  loop
    update payment_methods set delivery_platform_id = r.plataforma_id where id = r.metodo_id;

    insert into payment_methods
      (company_id, name, kind, affects_cash_drawer, is_active, sort_key, auto_declare, delivery_platform_id)
    values
      (r.company_id, replace(r.name, ' en línea', ' efectivo'), 'plataforma',
       -- true: son billetes que entran al cajón y se cuentan al cerrar.
       true, true, r.sort_key + 50,
       -- false obligatorio: SetPaymentMethodAutoDeclare ya rechaza autodeclarar un método que toca
       -- el cajón, así que sembrarlo en true contradiría una regla que el sistema ya defiende.
       false, r.plataforma_id);
  end loop;
end
$seed$;
-- +goose StatementEnd

-- +goose Down
-- ATENCIÓN: este Down solo es válido en la ventana entre aplicar la migración y el primer cobro
-- con un método nuevo (o el primer método propio que dé de alta una empresa). Después, las FK
-- `no action` de order_payments/expense_payments/register_session_totals lo hacen fallar con
-- 23503, y revertir pasa a ser un data-fix a mano con rollback gemelo, no `goose down`.

-- +goose StatementBegin
do $undo$
declare
  v_base bigint;
begin
  select min(id) into v_base from companies;

  -- Devolver los dependientes al método de la empresa base, emparejando por nombre (aquí sí es
  -- seguro: los nombres son copias byte a byte y ya no hay paso intermedio que los toque).
  update order_payments op set payment_method_id = base.id
  from payment_methods actual, payment_methods base
  where actual.id = op.payment_method_id and base.company_id = v_base and base.name = actual.name
    and actual.company_id <> v_base;

  update expense_payments ep set payment_method_id = base.id
  from payment_methods actual, payment_methods base
  where actual.id = ep.payment_method_id and base.company_id = v_base and base.name = actual.name
    and actual.company_id <> v_base;

  update register_session_totals rst set payment_method_id = base.id
  from payment_methods actual, payment_methods base
  where actual.id = rst.payment_method_id and base.company_id = v_base and base.name = actual.name
    and actual.company_id <> v_base;

  delete from payment_methods where name like '% efectivo' and kind = 'plataforma';
  delete from payment_methods where company_id <> v_base;
  update payment_methods set name = replace(name, ' en línea', '') where name like '% en línea';
end
$undo$;
-- +goose StatementEnd

drop policy tenant_isolation on payment_methods;
alter table payment_methods disable row level security;
alter table payment_methods drop constraint payment_methods_platform_fkey;
alter table payment_methods drop column delivery_platform_id;
alter table delivery_platforms drop constraint delivery_platforms_id_company_key;
drop index payment_methods_company;
alter table payment_methods drop constraint payment_methods_company_name_key;
alter table payment_methods add constraint payment_methods_name_key unique (name);
alter table payment_methods drop column company_id;

drop table if exists modifier_option_platform_prices;
drop table if exists product_platform_prices;
alter table delivery_platforms drop column price_markup_pct;
