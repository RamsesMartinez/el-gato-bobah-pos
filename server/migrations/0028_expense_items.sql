-- +goose Up
-- Detalle de mercancía del gasto y la separación pedido/recibido.
--
-- Un gasto era solo cabecera (importe + categoría). Con líneas puede alimentar el almacén: al
-- RECIBIR, cada línea inventariable genera su movimiento 'compra'. El gancho ya existía
-- (stock_movements.expense_id y el enum 'compra'); lo que faltaba era el detalle.

-- expense_date es la fecha del DOCUMENTO (cuándo se pidió/facturó); received_at es cuándo entró
-- al almacén. Son distintas por naturaleza: un pedido se paga el lunes y llega el jueves.
-- received_at null = todavía no recibido, y es el guard de idempotencia del endpoint /receive.
alter table expenses add column received_at date;
create index expenses_pending_receipt on expenses (received_at) where received_at is null;

-- Rastro del documento que originó el gasto.
-- doc_kind es TEXTO LIBRE y no un enum: un tipo de comprobante nuevo (nota, remisión, recibo)
-- no debe requerir una migración.
-- doc_raw guarda la extracción cruda tal como la devolvió el modelo. Cuando un proveedor nuevo
-- traiga un dato que la estructura de hoy no contempla, ya está guardado: se re-interpreta sin
-- volver a pagar la llamada ni pedirle el papel otra vez al dueño.
alter table expenses
  add column doc_kind  text,
  add column doc_folio text,
  add column doc_raw   jsonb;

create table expense_items (
  id            bigint generated always as identity primary key,
  expense_id    bigint not null references expenses(id) on delete cascade,
  -- item_type null = línea NO inventariable (bolsa, envío, "varios"): suma al gasto pero no
  -- toca el almacén. Es la mayoría de los renglones de un ticket de tienda mixta.
  item_type     stock_item_type,
  ingredient_id bigint references ingredients(id),
  product_id    bigint references products(id),
  description   text not null,
  -- quantity en la unidad de COMPRA (0.280 kg, 4 piezas), no en unidad base: es lo que el
  -- operador ve en el papel. La conversión a base la hace domain.BaseQty al recibir.
  quantity      numeric(14,4) not null check (quantity > 0),
  unit_id       smallint references units(id),
  -- qty_received: null = aún sin recibir; 0 = no llegó (un pedido marca renglones "No
  -- disponible"); N = llegó N, que puede diferir de lo pedido (peso ajustado, entrega parcial).
  -- Una columna cubre los tres casos y el movimiento de stock usa ESTA, no quantity.
  qty_received  numeric(14,4) check (qty_received >= 0),
  unit_cost     numeric(12,6) not null default 0 check (unit_cost >= 0),
  amount        numeric(10,2) not null check (amount >= 0),
  -- pack_qty_in_base: cuánto trae una unidad de compra en unidad base (una pieza de harina de
  -- 432 g → 432). Solo hace falta para el salto pieza → masa/volumen; se copia del formato
  -- aprendido del proveedor o lo captura el operador.
  pack_qty_in_base numeric(14,4) check (pack_qty_in_base > 0),
  position      int not null default 0,
  created_at    timestamptz not null default now(),
  -- Mismo patrón tenant que 0023/0026: el DEFAULT auto-sella el company_id en cada INSERT del
  -- app desde el GUC de sesión; missing_ok=true → NULL sin GUC.
  company_id    bigint not null default current_setting('app.company_id', true)::bigint
                references companies(id) on delete cascade,
  check ((item_type = 'ingrediente') = (ingredient_id is not null)),
  check ((item_type = 'producto')    = (product_id is not null)),
  -- una línea inventariable exige unidad: sin ella no hay conversión posible
  check (item_type is null or unit_id is not null)
);
create index expense_items_expense on expense_items (expense_id, position);
create index expense_items_ingredient on expense_items (ingredient_id);
create index expense_items_product on expense_items (product_id);
create index expense_items_company on expense_items (company_id);

-- RLS: mismo aislamiento de tenant que el resto (0024). El app (gatobobah_app) NO es owner → aplica.
alter table expense_items enable row level security;
create policy tenant_isolation on expense_items
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);
grant select, insert, update, delete on expense_items to gatobobah_app;

-- +goose Down
drop table if exists expense_items;
drop index if exists expenses_pending_receipt;
alter table expenses
  drop column if exists received_at,
  drop column if exists doc_kind,
  drop column if exists doc_folio,
  drop column if exists doc_raw;
