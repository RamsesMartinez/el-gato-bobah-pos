begin;
update products p set name=b.name,price=b.price,category_id=b.category_id,is_active=b.is_active from _bak_p_pre13 b where p.id=b.id;
update categories c set name=b.name, is_active=b.is_active from _bak_c_pre13 b where c.id=b.id;
delete from product_modifier_groups where group_id in (select id from modifier_groups where name in ('Sabor chamoyada','Tamaño chamoyada','Sabor frappé de agua','Tamaño frappé de agua'));
delete from modifier_groups where name in ('Sabor chamoyada','Tamaño chamoyada','Sabor frappé de agua','Tamaño frappé de agua');
update categories set is_active=false where name='Frappé de Agua' and parent_id=4;
commit;
