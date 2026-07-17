-- Migración #09 — vaciar "Otro (revisar)": inactivar opciones/variantes disfrazadas,
-- mover productos reales a su categoría. Reversible (_bak_otro_pre09).
begin;
create table if not exists _bak_otro_pre09 as select id, category_id, is_active from products where category_id=30 or id in (387,388,423);

-- opciones/variantes que NO deben verse como producto -> inactivar
update products set is_active=false where id in (
  406,407,408,409,                                  -- tamaños (14 oz/16 oz/8-14 piezas)
  410,411,412,413,414,415,417,418,419,422,427,430,432,433,434, -- sabores $0 (ya son modificadores)
  401,                                              -- Granillo Halloween (topping)
  383,384,391,392,                                  -- aderezos (opción)
  390,                                              -- Lunetas (topping crepa)
  404                                               -- Chuleta Ahumada (topping ramen)
);
-- productos reales mal ubicados -> a su categoría correcta
update products set category_id=37 where id in (387,388);  -- Dedos de Queso -> Snacks
update products set category_id=1  where id=423;           -- Mokaccino -> Bebidas Calientes
-- renombrar la categoría (quedan solo casos operativos ambiguos)
update categories set name='Otros' where id=30;
commit;
\echo '--- Otros (30): activos restantes (deben quedar 2: Envio, Juego Gonggi) ---'
select id,name,price from products where is_active and category_id=30;
\echo '--- Dedos/Mokaccino reubicados ---'
select id,name,category_id from products where id in (387,388,423);
