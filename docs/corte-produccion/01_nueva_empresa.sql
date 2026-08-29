-- CORTE A PRODUCCIÓN — abre la empresa que queda como principal y le copia el catálogo.
--
-- POR QUÉ ASÍ Y NO BORRANDO LAS VENTAS DE PRUEBA: el sistema ya es multi-tenant de verdad (RLS por
-- company_id en toda tabla de negocio, login usuario@slug, caché de menú y SSE por empresa). Abrir
-- un tenant nuevo cuesta lo mismo que un DELETE bien hecho y no destruye nada: el histórico de
-- pruebas queda íntegro y consultable en su propia empresa.
--
-- EL SLUG SE INTERCAMBIA A PROPÓSITO: el slug es la mitad derecha del identificador de login, así
-- que la empresa NUEVA se queda con `gatobobah` y los operadores siguen entrando con
-- `admin@gatobobah` sin enterarse del corte. La vieja pasa a `bobah-pruebas`.
--
-- LAS CONTRASEÑAS NO SE TOCAN: se copia el hash bcrypt tal cual y `username` es único POR EMPRESA
-- (users_company_username_key), así que los mismos usuarios existen en las dos con la misma clave.
--
-- EL OFFSET DE ID: los `id` son identity globales, no por empresa, así que copiar una fila obliga a
-- reapuntar cada FK que la referencia. En vez de mapear id viejo→nuevo tabla por tabla, cada fila
-- copiada nace con `id + offset`: la misma suma reapunta todas las FK sin tabla de mapeo.
--
-- El offset es POR TABLA y vale `max(id)` de esa tabla, no una constante global. Un 1,000,000 fijo
-- parece más simple y NO sirve: `channels`, `delivery_platforms` y `payment_methods` tienen el id en
-- `smallint` (tope 32767) y revientan con "smallint out of range". Con `max(id)` los ids copiados
-- caen en (max, 2*max] — nunca chocan con los existentes y siempre caben en el tipo de la columna.
-- Cada FK usa el offset de la tabla A LA QUE APUNTA, no el de la suya.
\set ON_ERROR_STOP on
BEGIN;

do $corte$
declare
  v_old bigint;
  v_new bigint;
  v_t text;
  v_cols text;
  v_vals text;
  v_ident boolean;
  v_n bigint;
  -- Orden topológico: una tabla solo aparece después de todas a las que apunta. NO están las
  -- transaccionales (orders, order_lines, order_payments, stock_movements, register_*, expenses,
  -- refresh_tokens, order_counters): la empresa nueva nace sin historia, que es el punto.
  -- Tampoco stock_levels — hoy son 66 renglones en negativo (hasta -6250) porque nunca se cargó
  -- inventario y cada movimiento fue una resta de venta; el nivel correcto al arrancar es que la
  -- fila no exista, y el app la crea sola en el primer movimiento.
  -- fudo_import_map queda fuera porque está vacío (0 filas).
  -- Tampoco `payment_methods` ni `units`: no tienen company_id, son GLOBALES y las dos empresas ya
  -- comparten las mismas filas. Copiarlas duplicaría los métodos de pago para todos.
  v_tablas constant text[] := array[
    'channels','delivery_platforms','expense_categories',
    'ingredient_categories','suppliers','cash_registers','recipes','modifier_groups','users',
    'categories','products','ingredients','modifier_options',
    'product_channels','product_modifier_groups','category_channels','combo_slots',
    'recipe_items','supplier_items','ingredient_purchase_formats',
    'combo_slot_products','business_settings','user_preferences'
  ];
begin
  select id into v_old from companies where slug = 'gatobobah';
  if v_old is null then
    raise exception 'No existe la empresa con slug gatobobah — ¿ya corrió este script?';
  end if;
  if exists (select 1 from companies where slug = 'bobah-pruebas') then
    raise exception 'Ya existe bobah-pruebas: el corte ya se hizo. Corre 01_rollback.sql antes de repetir.';
  end if;

  -- Se libera el slug ANTES de crear la nueva: companies.slug es unique.
  update companies set slug = 'bobah-pruebas', name = 'Bobah Pruebas', updated_at = now()
  where id = v_old;
  -- Se queda ACTIVA a propósito: es la única forma de entrar a consultar las ventas de prueba desde
  -- el POS, y con otro slug nadie cae ahí por accidente.

  insert into companies (slug, name) values ('gatobobah', 'El Gato Bobah') returning id into v_new;
  raise notice 'empresa pruebas=% | empresa produccion=%', v_old, v_new;

  -- Pasada 1: el offset de cada tabla. Va ANTES de copiar nada porque una FK necesita el offset de
  -- su tabla destino, que puede copiarse después que ella misma.
  create temporary table _corte_off (tabla text primary key, off bigint) on commit drop;
  foreach v_t in array v_tablas loop
    -- Candado: una tabla global (sin company_id) en esta lista generaría un INSERT que filtra por
    -- una columna inexistente. Truena aquí, con el nombre de la tabla, en vez de a media copia.
    if not exists (select 1 from information_schema.columns
                   where table_schema = 'public' and table_name = v_t and column_name = 'company_id') then
      raise exception 'La tabla % no tiene company_id: es global y no se copia.', v_t;
    end if;
    if exists (select 1 from information_schema.columns
               where table_schema = 'public' and table_name = v_t
                 and column_name = 'id' and is_identity = 'YES') then
      execute format('select coalesce(max(id), 0) from %I', v_t) into v_n;
      insert into _corte_off values (v_t, v_n);
    end if;
  end loop;

  -- Pasada 2: la copia.
  foreach v_t in array v_tablas loop
    -- Las columnas se leen del esquema vivo en vez de escribirlas a mano: una lista escrita a mano
    -- se desincroniza de la primera migración que agregue una columna y copia el catálogo INCOMPLETO
    -- en silencio, que es el peor modo de fallo posible aquí.
    select string_agg(quote_ident(col), ', ' order by ord),
           string_agg(expr, ', ' order by ord),
           bool_or(es_id)
      into v_cols, v_vals, v_ident
    from (
      select c.column_name as col, c.ordinal_position as ord, (c.is_identity = 'YES') as es_id,
             case
               -- El tenant se sella explícito; el DEFAULT de la columna lee el GUC y aquí no hay GUC.
               when c.column_name = 'company_id' then v_new::text
               -- La llave propia lleva el offset de SU tabla.
               when c.is_identity = 'YES'
                 then quote_ident(c.column_name) || ' + ' ||
                      (select off from _corte_off where tabla = v_t)
               -- Una FK lleva el offset de la tabla A LA QUE APUNTA. Solo se desplaza si esa tabla
               -- también se copia; las que apuntan fuera del juego (units, que es global) van tal
               -- cual — desplazarlas apuntaría a unidades inexistentes.
               when fk.destino is not null
                 then quote_ident(c.column_name) || ' + ' ||
                      coalesce((select off from _corte_off where tabla = fk.destino), 0)
               else quote_ident(c.column_name)
             end as expr
      from information_schema.columns c
      left join lateral (
        select con.confrelid::regclass::text as destino
        from pg_constraint con
        where con.contype = 'f'
          and con.conrelid = v_t::regclass
          and con.conkey[1] = (select a.attnum from pg_attribute a
                               where a.attrelid = v_t::regclass and a.attname = c.column_name)
          and con.confrelid::regclass::text = any(v_tablas)
        limit 1
      ) fk on true
      where c.table_schema = 'public' and c.table_name = v_t
        -- Las columnas generadas (products.margin_amount) las calcula Postgres: incluirlas en el
        -- INSERT es un error, y omitirlas no pierde nada porque se recalculan solas.
        and c.is_generated <> 'ALWAYS'
    ) z;

    -- categories.parent_id apunta a categories: un INSERT único basta porque Postgres verifica las
    -- FK al terminar la sentencia, no fila por fila, así que padre e hijo entran juntos.
    execute format('insert into %I (%s) %s select %s from %I where company_id = %s',
                   v_t, v_cols,
                   case when v_ident then 'overriding system value' else '' end,
                   v_vals, v_t, v_old);
    get diagnostics v_n = row_count;
    raise notice '  % -> % filas', rpad(v_t, 28), v_n;

    -- La secuencia queda por debajo de los ids copiados: sin esto, el primer alta desde el POS
    -- elige un id ya usado y truena con violación de llave primaria. El `where m is not null`
    -- salta las tablas que quedaron vacías (combo_slots hoy no tiene filas).
    if v_ident then
      execute format(
        'select setval(pg_get_serial_sequence(%L, ''id''), m) from (select max(id) m from %I) s where m is not null',
        v_t, v_t);
    end if;
  end loop;
end
$corte$;

\echo '=== VERIFICACION: el catalogo debe cuadrar renglon por renglon ==='
-- Vista temporal para no repetir el subselect del id de cada empresa en 40 lugares.
create temporary view v_emp as
select (select id from companies where slug = 'bobah-pruebas') as pruebas,
       (select id from companies where slug = 'gatobobah')     as produccion;

select 'categories' t, count(*) filter (where company_id = (select pruebas from v_emp)) pruebas, count(*) filter (where company_id = (select produccion from v_emp)) produccion from categories
union all select 'products',                count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from products
union all select 'modifier_groups',         count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from modifier_groups
union all select 'modifier_options',        count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from modifier_options
union all select 'product_modifier_groups', count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from product_modifier_groups
union all select 'product_channels',        count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from product_channels
union all select 'category_channels',       count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from category_channels
union all select 'recipes',                 count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from recipes
union all select 'recipe_items',            count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from recipe_items
union all select 'ingredients',              count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from ingredients
union all select 'ingredient_categories',    count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from ingredient_categories
union all select 'suppliers',                count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from suppliers
union all select 'supplier_items',           count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from supplier_items
union all select 'cash_registers',           count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from cash_registers
union all select 'channels',                 count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from channels
union all select 'delivery_platforms',       count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from delivery_platforms
union all select 'expense_categories',       count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from expense_categories
union all select 'users',                    count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from users
union all select 'business_settings',        count(*) filter (where company_id=(select pruebas from v_emp)), count(*) filter (where company_id=(select produccion from v_emp)) from business_settings
order by 1;

\echo '=== VERIFICACION: la empresa nueva NO debe tener nada transaccional (todo en 0) ==='
select 'orders' t, count(*) n from orders where company_id = (select produccion from v_emp)
union all select 'order_lines',       count(*) from order_lines       where company_id = (select produccion from v_emp)
union all select 'order_payments',    count(*) from order_payments    where company_id = (select produccion from v_emp)
union all select 'stock_movements',   count(*) from stock_movements   where company_id = (select produccion from v_emp)
union all select 'stock_levels',      count(*) from stock_levels      where company_id = (select produccion from v_emp)
union all select 'register_sessions', count(*) from register_sessions where company_id = (select produccion from v_emp)
union all select 'expenses',          count(*) from expenses          where company_id = (select produccion from v_emp)
union all select 'order_counters',    count(*) from order_counters    where company_id = (select produccion from v_emp)
order by 1;

\echo '=== VERIFICACION: ninguna FK debe cruzar de una empresa a la otra (todo en 0) ==='
select 'products.category_id' q, count(*) fugas from products p join categories c on c.id = p.category_id where p.company_id <> c.company_id
union all select 'products.recipe_id',            count(*) from products p join recipes r on r.id = p.recipe_id where p.company_id <> r.company_id
union all select 'categories.parent_id',          count(*) from categories h join categories pa on pa.id = h.parent_id where h.company_id <> pa.company_id
union all select 'modifier_options.group_id',     count(*) from modifier_options o join modifier_groups g on g.id = o.group_id where o.company_id <> g.company_id
union all select 'modifier_options.linked_product', count(*) from modifier_options o join products p on p.id = o.linked_product_id where o.company_id <> p.company_id
union all select 'product_modifier_groups.product', count(*) from product_modifier_groups x join products p on p.id = x.product_id where x.company_id <> p.company_id
union all select 'product_modifier_groups.group',   count(*) from product_modifier_groups x join modifier_groups g on g.id = x.group_id where x.company_id <> g.company_id
union all select 'recipe_items.ingredient_id',    count(*) from recipe_items ri join ingredients i on i.id = ri.ingredient_id where ri.company_id <> i.company_id
union all select 'recipe_items.recipe_id',        count(*) from recipe_items ri join recipes r on r.id = ri.recipe_id where ri.company_id <> r.company_id
union all select 'ingredients.category_id',       count(*) from ingredients i join ingredient_categories ic on ic.id = i.category_id where i.company_id <> ic.company_id
union all select 'product_channels.product_id',   count(*) from product_channels pc join products p on p.id = pc.product_id where pc.company_id <> p.company_id
union all select 'supplier_items.supplier_id',    count(*) from supplier_items si join suppliers s on s.id = si.supplier_id where si.company_id <> s.company_id
union all select 'business_settings.updated_by',  count(*) from business_settings bs join users u on u.id = bs.updated_by where bs.company_id <> u.company_id
order by 1;

\echo '=== VERIFICACION: recipe_items.unit_id sigue apuntando a units (tabla global, NO se desplaza) ==='
select count(*) as items_sin_unidad_valida
from recipe_items ri
where ri.unit_id is not null and not exists (select 1 from units u where u.id = ri.unit_id);

\echo '=== VERIFICACION: los usuarios de produccion entran con la MISMA contrasena y el MISMO PIN ==='
-- El join va por `name` y no por `username`: dos de los cuatro usuarios entran solo con PIN y
-- tienen username NULL, y un join por username los dejaría fuera del reporte sin avisar.
-- Un PIN repetido entre empresas NO es ambiguo: /pin-switch corre bajo WithTenant, o sea que
-- resuelve el PIN dentro de la empresa en la que ya está el dispositivo.
select u.name, u.username, u.role, u.is_active,
       (u.password_hash is not distinct from v.password_hash) as mismo_password,
       (u.pin_hash       is not distinct from v.pin_hash)      as mismo_pin
from users u
join users v on v.name = u.name and v.company_id = (select pruebas from v_emp)
where u.company_id = (select produccion from v_emp)
order by u.name;

\echo '=== VERIFICACION: las dos empresas ==='
select id, slug, name, is_active from companies order by id;

COMMIT;
