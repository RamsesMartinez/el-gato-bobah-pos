-- +goose Up
-- `categories_name_scope` nació en 0004, antes de que el sistema fuera multi-tenant, como único
-- sobre (coalesce(parent_id,0), name). Las migraciones 0022–0024 agregaron company_id y RLS a todas
-- las tablas pero no revisaron los índices únicos anteriores, y este quedó cruzando empresas: para
-- una categoría RAÍZ el coalesce da 0 en todas, así que la segunda empresa que quisiera su propia
-- "Bebidas" chocaba contra la de la primera. La RLS aislaba las lecturas pero el índice hacía
-- imposible poblar un tenant nuevo — se descubrió al abrir la empresa de producción.
--
-- El índice sigue haciendo su trabajo original (no dos categorías con el mismo nombre bajo el mismo
-- padre); solo se le antepone la empresa. Las categorías hijas nunca colisionaban porque su
-- parent_id ya es distinto entre empresas, pero se incluyen igual: la regla es "único dentro de la
-- empresa" y partirla en dos casos invita a que el siguiente cambio se equivoque.
drop index categories_name_scope;
create unique index categories_name_scope
  on categories (company_id, coalesce(parent_id, 0::bigint), name);

-- +goose Down
drop index categories_name_scope;
create unique index categories_name_scope
  on categories (coalesce(parent_id, 0::bigint), name);
