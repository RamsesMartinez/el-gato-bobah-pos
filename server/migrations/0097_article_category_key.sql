-- +goose Up
-- Revisión de la 042: la llave de ingredients.category_id es de 0003 y no lleva la empresa. Una llave
-- foránea salta RLS, así que un artículo podía quedar ligado a la categoría de otra empresa. El servidor
-- ya lo valida (categoryOfCompany); esta llave lo cierra también en la base. Y el índice de los renglones
-- de conteo por artículo, que el conteo, el aviso y el cambio de unidad leen sin el conteo en la mano.
--
-- Tolera a la API anterior: no quita columnas ni cambia lo que ella escribe.

set local lock_timeout = '3s';

-- Antes de cambiar la llave: ningún artículo ligado a la categoría de otra empresa. Si lo hay, se aborta
-- en vez de adivinar a cuál pertenece.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from ingredients i join ingredient_categories c on c.id = i.category_id
              where c.company_id <> i.company_id) then
    raise exception '0097: hay artículos ligados a la categoría de otra empresa';
  end if;
end $$;
-- +goose StatementEnd

alter table ingredient_categories add constraint ingredient_categories_id_company_key unique (id, company_id);
alter table ingredients drop constraint ingredients_category_id_fkey;
alter table ingredients add constraint ingredients_category_fkey
  foreign key (category_id, company_id) references ingredient_categories (id, company_id);

create index stock_count_lines_by_ingredient on stock_count_lines (company_id, ingredient_id);

-- +goose Down
set local lock_timeout = '3s';
drop index stock_count_lines_by_ingredient;
alter table ingredients drop constraint ingredients_category_fkey;
alter table ingredients add constraint ingredients_category_id_fkey
  foreign key (category_id) references ingredient_categories (id);
alter table ingredient_categories drop constraint ingredient_categories_id_company_key;
