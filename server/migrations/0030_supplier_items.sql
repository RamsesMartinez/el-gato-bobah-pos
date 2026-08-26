-- +goose Up
-- Catálogo de artículos POR PROVEEDOR: la tabla intermedia que aprende el mapeo entre lo que
-- dice el papel y lo que hay en el inventario.
--
-- Sin esto, cada compra obliga a reteclear los mismos veinte renglones: las descripciones de
-- los tickets están truncadas ("MM 2K FRESA", "ACEITEVEGETA") y no coinciden con el nombre del
-- ingrediente. Se mapea una vez y las visitas siguientes se autollenan.

-- pg_trgm: similitud por trigramas para SUGERIR coincidencias ("COCA COLA 600ML" contra
-- "Coca Cola 600 ml"). Es extensión estándar de Postgres y es exactamente para esto; el
-- precedente de create extension en este esquema es citext (0001). El matching vive en SQL y
-- no en el extractor a propósito: así es determinista, gratis, instantáneo y sirve también
-- cuando se captura a mano sin documento.
create extension if not exists pg_trgm;

create table supplier_items (
  id            bigint generated always as identity primary key,
  supplier_id   bigint not null references suppliers(id) on delete cascade,
  -- raw_code: el código del proveedor cuando IDENTIFICA al artículo (el SKU de Sam's). Queda
  -- null cuando el documento no trae código o cuando el que trae es de departamento y se repite
  -- entre artículos distintos (domain.DropAmbiguousCodes lo detecta y lo vacía).
  raw_code      text,
  raw_name      text not null,   -- texto del documento, tal cual
  -- norm_name: raw_name normalizado (minúsculas, sin acentos, espacios colapsados). Es la llave
  -- de respaldo cuando no hay código, y el campo contra el que corre la similitud.
  norm_name     text not null,
  -- status separa "todavía no lo decido" de "decidí que no es inventariable" (bolsa, IVA,
  -- envío). Sin él las dos situaciones se ven idénticas —todo en null— y la cola de revisión
  -- no se puede consultar.
  status        text not null default 'pendiente'
                check (status in ('pendiente', 'mapeado', 'ignorado')),
  item_type     stock_item_type,
  ingredient_id bigint references ingredients(id) on delete set null,
  product_id    bigint references products(id) on delete set null,
  -- pack_qty_in_base: cuánto trae una unidad de compra de ESTE proveedor en unidad base
  -- (432 g por pieza). Es el dato que permite descontar "4 piezas" de un ingrediente en gramos.
  pack_qty_in_base numeric(14,4) check (pack_qty_in_base > 0),
  unit_id       smallint references units(id),
  last_cost     numeric(12,6) check (last_cost >= 0),
  last_seen_at  timestamptz not null default now(),
  created_at    timestamptz not null default now(),
  company_id    bigint not null default current_setting('app.company_id', true)::bigint
                references companies(id) on delete cascade,
  check ((item_type = 'ingrediente') = (ingredient_id is not null)),
  check ((item_type = 'producto')    = (product_id is not null)),
  -- 'mapeado' exige a qué se mapeó; 'ignorado' y 'pendiente' no apuntan a nada.
  check (status <> 'mapeado' or item_type is not null)
);

-- Llave del aprendizaje: el código del proveedor si lo hay, y si no el nombre normalizado.
-- Va como índice de expresión porque un unique constraint no admite coalesce.
create unique index supplier_items_key
  on supplier_items (company_id, supplier_id, coalesce(raw_code, norm_name));
-- Índice de trigramas para las sugerencias por parecido.
create index supplier_items_norm_trgm on supplier_items using gin (norm_name gin_trgm_ops);
create index supplier_items_pendientes on supplier_items (company_id, status) where status = 'pendiente';
create index supplier_items_company on supplier_items (company_id);

alter table supplier_items enable row level security;
create policy tenant_isolation on supplier_items
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);
grant select, insert, update, delete on supplier_items to gatobobah_app;

-- Índices de trigramas sobre el catálogo propio, para sugerir contra ingredientes y productos
-- cuando el proveedor es nuevo y no hay nada aprendido todavía.
create index ingredients_name_trgm on ingredients using gin ((name::text) gin_trgm_ops);
create index products_name_trgm on products using gin ((name::text) gin_trgm_ops);

-- +goose Down
drop index if exists products_name_trgm;
drop index if exists ingredients_name_trgm;
drop table if exists supplier_items;
-- pg_trgm se queda: soltarla tiraría cualquier otro índice que la use y no cuesta nada mantenerla.
