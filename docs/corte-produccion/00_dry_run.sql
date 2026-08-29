-- DRY RUN — no modifica nada. Enseña de dónde parte el corte a producción.
--
-- Contexto: el POS convivió con FUDO mientras se probaba, y la empresa que hay arriba acumuló
-- ventas de prueba. En vez de borrarlas, se abre una empresa NUEVA que nace limpia y se queda con
-- el slug `gatobobah` (el login `usuario@slug` de los operadores no cambia); la actual se renombra
-- a "Bobah Pruebas" y conserva su histórico.
\echo '=== A) empresas ==='
select id, slug, name, is_active, created_at::date from companies order by id;

\echo '=== B) lo transaccional que se queda en la empresa de pruebas ==='
select 'orders' t, count(*) n from orders
union all select 'order_lines', count(*) from order_lines
union all select 'order_payments', count(*) from order_payments
union all select 'stock_movements', count(*) from stock_movements
union all select 'register_sessions', count(*) from register_sessions
union all select 'expenses', count(*) from expenses
order by 1;

\echo '=== C) el catálogo que se copia a la empresa nueva ==='
select 'categories' t, count(*) n from categories
union all select 'products', count(*) from products
union all select 'modifier_groups', count(*) from modifier_groups
union all select 'modifier_options', count(*) from modifier_options
union all select 'product_modifier_groups', count(*) from product_modifier_groups
union all select 'recipes', count(*) from recipes
union all select 'recipe_items', count(*) from recipe_items
union all select 'ingredients', count(*) from ingredients
union all select 'suppliers', count(*) from suppliers
union all select 'cash_registers', count(*) from cash_registers
union all select 'payment_methods', count(*) from payment_methods
union all select 'users', count(*) from users
union all select 'business_settings', count(*) from business_settings
order by 1;

\echo '=== D) usuarios que van a poder entrar en la empresa nueva con su MISMA contraseña ==='
select username, role, is_active, must_change_password from users order by id;

-- El offset de cada tabla vale su propio max(id), así que los ids copiados caen en (max, 2*max].
-- Este número sirve para confirmar de un vistazo que el doble sigue cabiendo en el tipo de la
-- columna — importa en `channels`, `delivery_platforms` y `payment_methods`, que usan smallint.
\echo '=== E) el id más alto del catálogo ==='
select max(m) as max_id_catalogo from (
  select max(id) m from products union all select max(id) from categories
  union all select max(id) from modifier_options union all select max(id) from ingredients
  union all select max(id) from recipes union all select max(id) from recipe_items
  union all select max(id) from modifier_groups union all select max(id) from suppliers
  union all select max(id) from users) x;

\echo '=== F) niveles de inventario: NO se copian (todos en negativo, son restas de ventas de prueba) ==='
select count(*) n, min(on_hand) mn, max(on_hand) mx from stock_levels;
