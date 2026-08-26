# El Gato Bobah POS — Domain Model & PostgreSQL Schema Plan

> **HISTÓRICO — plan de la construcción inicial, ejecutado. No es el estado actual.**
> Se conserva para entender *por qué* el diseño es como es. Para saber cómo está el sistema hoy:
> el código, [`AGENTS.md`](../../AGENTS.md) y
> [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md).
> Índice de `docs/`: [../README.md](../README.md).

Target: PostgreSQL 16 + Redis, Go backend, single location, CQRS-light (ledger + trigger-maintained caches + Redis read models — **no event sourcing**).

## 0. Conventions

- PKs: `bigint generated always as identity` (`smallint` for tiny lookup tables). Orders also carry a client-generated `client_uuid` for tablet idempotency/offline retry.
- Money: `numeric(10,2)`. Unit costs: `numeric(12,6)` (per-gram costs are tiny). Quantities: `numeric(14,4)`.
- `timestamptz` everywhere; `created_at not null default now()`, `updated_at` via trigger.
- Names: `citext` + unique — kills FUDO weakness #2 (name joins) at the root: all joins by ID, names merely unique-per-scope.
- Soft delete = `is_active boolean not null default true` (FUDO's `Activo`). Nothing transactional is ever hard-deleted.
- All Spanish-facing vocabularies (states, movement types) as Postgres enums; UI labels live in the frontend.

```sql
create extension if not exists citext;
```

## 1. Units & Suppliers (fixes FUDO #3 and #7)

```sql
create type unit_kind as enum ('masa','volumen','pieza');

create table units (
  id        smallint generated always as identity primary key,
  code      text not null unique,          -- 'g','kg','ml','l','floz','cda','pieza'
  name      text not null,
  kind      unit_kind not null,
  to_base   numeric(16,6) not null check (to_base > 0)  -- factor to kind base: g / ml / pieza
);
-- seed: g=1, kg=1000, ml=1, l=1000, floz=29.5735, cda=15 (volumen), cdta=5, pieza=1
```

Every ingredient has a **base unit** (one kind). Recipe lines may use any unit *of the same kind* (validated by trigger); the engine normalizes via `to_base`. This replaces FUDO's `Mayonesa 870g = 75.269 × Mayonesa Cda` hack with a real `cda` unit. Cross-kind cases (slice = 1/20 loaf, jar→grams) are handled by **purchase formats** (§2), not fake ingredients.

```sql
create table suppliers (
  id bigint generated always as identity primary key,
  name citext not null unique,
  phone text, notes text,
  is_active boolean not null default true,
  created_at timestamptz not null default now()
);
```
First-class supplier entity, referenced by ingredients, products, and expenses (payee). Import dedupes FUDO's free strings (Aurrera, TD Empaques, Sam's Club, …).

## 2. Recipes, Ingredients, Preps (subingredientes done right)

**One `recipes` entity shared by products, modifier options, and prep ingredients** — one costing engine, one table:

```sql
create table recipes (
  id bigint generated always as identity primary key,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table recipe_items (
  id          bigint generated always as identity primary key,
  recipe_id   bigint not null references recipes(id) on delete cascade,
  ingredient_id bigint not null references ingredients(id),
  quantity    numeric(14,4) not null check (quantity > 0),
  unit_id     smallint not null references units(id),  -- same kind as ingredient.base_unit (trigger)
  position    int not null default 0,
  unique (recipe_id, ingredient_id)
);
create index on recipe_items (ingredient_id);  -- reverse lookup for cost/availability cascade
```

Packaging **stays a recipe line** (FUDO got this right): bolsa/vaso/tapa are ingredients flagged `is_packaging` (for reporting), included in cost roll-up.

```sql
create table ingredient_categories (
  id bigint generated always as identity primary key,
  name citext not null unique,
  is_active boolean not null default true
);

create type cost_source as enum ('manual','compra','receta');

create table ingredients (
  id            bigint generated always as identity primary key,
  name          citext not null unique,
  category_id   bigint references ingredient_categories(id),
  base_unit_id  smallint not null references units(id),
  is_prep       boolean not null default false,          -- subingrediente preparado
  recipe_id     bigint unique references recipes(id),    -- only when is_prep
  yield_qty     numeric(14,4),                           -- batch output in base units
  waste_pct     numeric(5,2) not null default 0 check (waste_pct >= 0 and waste_pct < 100), -- merma %
  current_cost  numeric(12,6) not null default 0,        -- per base unit (cache, engine-maintained)
  cost_source   cost_source not null default 'manual',
  supplier_id   bigint references suppliers(id),
  is_packaging  boolean not null default false,
  track_stock   boolean not null default true,
  min_stock     numeric(14,4),                           -- reorder alert threshold, base units
  is_active     boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (not is_prep or (recipe_id is not null and yield_qty > 0))
);

create table ingredient_purchase_formats (   -- pack→base conversion, replaces conversion-hack subingredientes
  id bigint generated always as identity primary key,
  ingredient_id bigint not null references ingredients(id) on delete cascade,
  name text not null,                        -- 'Frasco 870 g', 'Barra 20 rebanadas', 'Lata'
  qty_in_base numeric(14,4) not null check (qty_in_base > 0),
  last_cost numeric(10,2),                   -- cost of one format unit at last purchase
  supplier_id bigint references suppliers(id),
  is_default boolean not null default false
);
```

**Preps** (Salsa Verde, Mezcla Crepas): `is_prep=true` + recipe + yield. Two operating modes, both supported:
- *Produced ahead* (`track_stock=true`): a **producción** stock movement adds prep on-hand and consumes components; sales deplete the prep itself.
- *Made to order* (`track_stock=false`): cost and availability recurse into components at sale time.

Recursion is unrestricted (FUDO observed 1 level); cost engine carries a cycle guard.

## 3. Catalog: categories, channels, products, combos

```sql
create table channels (            -- generalizes FUDO's Tienda Online / Menú QR (weakness #8, kept & generalized)
  id smallint generated always as identity primary key,
  code text not null unique,       -- 'pos','qr','online'
  name text not null
);

create table categories (
  id        bigint generated always as identity primary key,
  name      citext not null,
  parent_id bigint references categories(id),   -- 2 levels enforced by trigger (parent must be root)
  sort_key  numeric(18,9) not null default 1000, -- fractional ranking: insert-between = midpoint (fixes #9)
  color     text,                                 -- stable tile color (fixes random-hue UX bug)
  image_url text,
  is_active boolean not null default true
);
create unique index categories_name_scope on categories (coalesce(parent_id,0), name);

create table category_channels (   -- category-level visibility default; absence = visible
  category_id bigint not null references categories(id) on delete cascade,
  channel_id  smallint not null references channels(id),
  visible     boolean not null,
  primary key (category_id, channel_id)
);
```

Verdict on the tree question: **2-level, enforced** — FUDO's data never needed more, and the POS drill-down UI is built for exactly two hops. The `parent_id` shape allows deepening later without migration.

```sql
create type product_type as enum ('simple','combo');

create table products (
  id            bigint generated always as identity primary key,
  sku           text unique,                     -- Código / barcode (sparse)
  name          citext not null unique,
  description   text,
  type          product_type not null default 'simple',
  category_id   bigint not null references categories(id),
  price         numeric(10,2) not null check (price >= 0),
  cost_source   cost_source not null default 'manual',
  manual_cost   numeric(12,4),
  current_cost  numeric(12,4) not null default 0,          -- cache, costing engine
  margin_amount numeric(12,4) generated always as (price - current_cost) stored,
  recipe_id     bigint unique references recipes(id),
  track_stock   boolean not null default false,  -- direct-stock resale items (sodas, ramune)
  allow_oversell boolean not null default true,  -- 'Vender sin stock'
  min_stock     numeric(14,4),
  is_favorite   boolean not null default false,
  sort_key      numeric(18,9) not null default 1000,
  image_url     text,
  is_active     boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (type <> 'combo' or recipe_id is null),
  check (not track_stock or recipe_id is null)   -- FUDO: only 1/752 violated this; enforce
);
create index on products (category_id) where is_active;
create index on products (is_favorite) where is_favorite;

create type channel_visibility as enum ('visible','oculto');
create table product_channels (    -- row absent = inherit category (FUDO's 'Según categoría' tri-state)
  product_id bigint not null references products(id) on delete cascade,
  channel_id smallint not null references channels(id),
  visibility channel_visibility not null,
  primary key (product_id, channel_id)
);
```

Note the **big cleanup vs FUDO**: no `sell_alone`, no price-0 pseudo-products, no "Otro" dumping category — modifier options are no longer products (§4). Margin % is derived in views (`margin_amount / nullif(price,0)`); the stored absolute margin matches FUDO semantics (#5) but both are exposed.

**Combos first-class** (fixes #6): a combo is a product (`type='combo'`) with slots. Fixed component = slot with one option; choice ("DUO FRAPPÉ: pick 2") = slot with several options and optional surcharges.

```sql
create table combo_slots (
  id bigint generated always as identity primary key,
  combo_id  bigint not null references products(id) on delete cascade,
  name      text not null,                      -- 'Elige tu frappé'
  min_select smallint not null default 1,
  max_select smallint not null default 1 check (max_select >= min_select),
  position  int not null default 0
);
create table combo_slot_products (
  slot_id    bigint not null references combo_slots(id) on delete cascade,
  product_id bigint not null references products(id),
  price_delta numeric(10,2) not null default 0,
  is_default boolean not null default false,    -- used for combo cost roll-up
  primary key (slot_id, product_id)
);
```

## 4. Modifiers as first-class entities (fixes #1, preserves the stock-depletion power)

```sql
create table modifier_groups (
  id bigint generated always as identity primary key,
  name citext not null unique,           -- internal name: 'Perlas explosivas'
  is_active boolean not null default true
);

create table modifier_options (
  id          bigint generated always as identity primary key,
  group_id    bigint not null references modifier_groups(id) on delete cascade,
  name        text not null,
  price_delta numeric(10,2) not null default 0,   -- can be negative (descuentos)
  recipe_id   bigint unique references recipes(id),        -- ← option depletes ingredient stock
  linked_product_id bigint references products(id),        -- option = a stocked product (e.g. SOJU in combo)
  max_per_line smallint not null default 1,                -- FUDO per-option Máxima cantidad
  current_cost numeric(12,4) not null default 0,           -- cache from recipe/linked product
  sort_key    numeric(18,9) not null default 1000,
  is_active   boolean not null default true,
  unique (group_id, name),
  check (recipe_id is null or linked_product_id is null)
);

create table product_modifier_groups (   -- shared groups, per-attachment title/min/max (FUDO pattern, kept)
  id bigint generated always as identity primary key,
  product_id bigint not null references products(id) on delete cascade,
  group_id   bigint not null references modifier_groups(id),
  title      text,                       -- 'Elije el ingrediente' — display title per attachment
  min_select smallint not null default 0,
  max_select smallint not null default 1 check (max_select >= min_select),
  position   int not null default 0,
  unique (product_id, group_id)
);
```

What this preserves from FUDO's hack: option → recipe → ingredient depletion (the 137 recipe-bearing options), price deltas, group sharing (group 80 attached to every ramen), per-attachment title/min/max, per-option qty cap. What it kills: 335 catalog-polluting pseudo-products, duplicated prices (override matched product price in only 101/485 rows), "SIN X" sentinel products (now just `min_select=0` or a plain $0 no-recipe option), penny-pricing.

`linked_product_id` covers the "option is really a sellable stocked product" case: sale depletes that product's direct stock and its cost feeds the snapshot.

## 5. Stock: movement ledger as source of truth (fixes #4 formally)

```sql
create type stock_item_type as enum ('ingrediente','producto');
create type stock_movement_type as enum
  ('venta','compra','ajuste','merma','produccion','cancelacion');
  -- cancelacion = restock on order cancel; devolución later if needed

create table stock_movements (
  id            bigint generated always as identity primary key,
  item_type     stock_item_type not null,
  ingredient_id bigint references ingredients(id),
  product_id    bigint references products(id),
  movement_type stock_movement_type not null,
  quantity      numeric(14,4) not null check (quantity <> 0),  -- SIGNED delta, base units
  unit_cost     numeric(12,6),               -- cost per base unit at movement time
  order_id      bigint references orders(id),
  expense_id    bigint references expenses(id),   -- links a 'compra' to the money spent
  user_id       bigint references users(id),
  reason        text,                        -- Razón (ajuste/merma vocabularies)
  note          text,
  created_at    timestamptz not null default now(),
  check ((item_type='ingrediente') = (ingredient_id is not null)),
  check ((item_type='producto')    = (product_id is not null))
);
create index on stock_movements (ingredient_id, created_at desc);
create index on stock_movements (product_id, created_at desc);
create index on stock_movements (order_id);
create index on stock_movements (movement_type, created_at desc);

create table stock_levels (       -- derived cache, rebuildable: sum(quantity) from ledger
  item_type     stock_item_type not null,
  ingredient_id bigint unique references ingredients(id),
  product_id    bigint unique references products(id),
  on_hand       numeric(14,4) not null default 0,
  updated_at    timestamptz not null default now(),
  check ((item_type='ingrediente') = (ingredient_id is not null)),
  check ((item_type='producto')    = (product_id is not null))
);
-- AFTER INSERT trigger on stock_movements upserts stock_levels (single-row UPDATE, fast).
```

Policies (matching real FUDO usage — ingredient stock hits −1808 today):
- **Ledger never blocks**: `on_hand` may go negative (truthful accounting beats fake zeros).
- **Sale-time gate**: `products.allow_oversell` — when false, the POS write path rejects a line whose derived availability (below) is < qty. Enforced in the Go transaction, not the DB.
- **Disponibilidad (sellable availability)** is a derived read model, clamped ≥ 0:
  - `track_stock` product → `max(on_hand, 0)`
  - recipe product/option → `min over recipe_items i of floor( avail(ingredient_i) / (qty_i_base × (1 + waste_pct_i/100)) )`
  - prep ingredient: `track_stock` → its own on_hand; else recurse into its recipe (÷ yield).
- **Producción** (prep batch): one tx = `+yield_qty` prep movement, `−component` movements at component cost; prep `current_cost` refreshed.
- **Stock mínimo alerts**: query `stock_levels join ... where on_hand <= min_stock`; surfaced on the dashboard read model.

Sale depletion (inside the order tx): each non-cancelled line explodes product recipe × qty + each selected option's recipe × option qty (+ linked product direct stock) into `venta` movements referencing `order_id`. Cancellation inserts mirror `cancelacion` movements (ledger stays append-only — no deletes).

## 6. Sales: order → lines → modifier selections → payments

**State machine — 4 states** (vs FUDO's 7). Payment is orthogonal (derived from payments sum), which is what made FUDO need PAYMENT-PROCESS/IN-COURSE/DELIVERY-SENT etc.

```
abierta ──→ lista ──→ entregada        (terminal)
   │           │
   └───────────┴────→ cancelada        (terminal, requires reason + user)
```
Mostrador quick sale may jump `abierta → entregada` directly. The Active Sales board buckets: Pendientes = abierta & unpaid, En curso = abierta & paid, Listos = lista.

```sql
create type order_status  as enum ('abierta','lista','entregada','cancelada');
create type service_type  as enum ('mostrador','para_llevar','domicilio');

create table delivery_platforms (
  id smallint generated always as identity primary key,
  name citext not null unique,          -- Uber Eats, Didi, Rappi, Propio
  is_active boolean not null default true
);

create table order_counters (business_date date primary key, last_number int not null);
-- daily number: UPDATE ... SET last_number = last_number+1 RETURNING (upsert row at first order of day)

create table orders (
  id            bigint generated always as identity primary key,
  client_uuid   uuid not null unique,           -- tablet idempotency key
  business_date date not null,                  -- from register session / local day
  daily_number  int not null,
  status        order_status not null default 'abierta',
  service_type  service_type not null,
  delivery_platform_id smallint references delivery_platforms(id),
  customer_name text,
  notes         text,
  register_session_id bigint references register_sessions(id),
  opened_by     bigint not null references users(id),
  subtotal       numeric(10,2) not null default 0,
  discount_total numeric(10,2) not null default 0,
  total          numeric(10,2) not null default 0,
  opened_at     timestamptz not null default now(),
  ready_at      timestamptz,
  completed_at  timestamptz,
  cancelled_at  timestamptz,
  cancelled_by  bigint references users(id),
  cancel_reason text,
  updated_at    timestamptz not null default now(),
  unique (business_date, daily_number),
  check (service_type = 'domicilio' or delivery_platform_id is null),
  check ((status='cancelada') = (cancelled_at is not null and cancelled_by is not null and cancel_reason is not null))
);
create index orders_board on orders (service_type, status) where status in ('abierta','lista');
create index on orders (business_date, status);

create table order_lines (
  id             bigint generated always as identity primary key,
  order_id       bigint not null references orders(id) on delete cascade,
  product_id     bigint not null references products(id),
  parent_line_id bigint references order_lines(id),      -- combo component lines
  product_name   text not null,                          -- SNAPSHOT
  quantity       numeric(8,2) not null check (quantity > 0),
  unit_price     numeric(10,2) not null,                 -- SNAPSHOT (base price)
  modifiers_total numeric(10,2) not null default 0,      -- per-unit sum of selected deltas
  unit_cost      numeric(12,4) not null default 0,       -- SNAPSHOT: product cost + options cost (historical profit)
  line_total     numeric(10,2) not null,                 -- (unit_price + modifiers_total) * quantity
  notes          text,
  cancelled_at   timestamptz,                            -- per-item cancellation (FUDO's Histórico de cancelaciones)
  cancelled_by   bigint references users(id),
  cancel_reason  text,
  created_at     timestamptz not null default now()
);
create index on order_lines (order_id);
create index on order_lines (product_id, created_at desc);   -- product ranking reports

create table order_line_modifiers (
  id                 bigint generated always as identity primary key,
  order_line_id      bigint not null references order_lines(id) on delete cascade,
  modifier_option_id bigint not null references modifier_options(id),
  group_title        text not null,        -- SNAPSHOT
  option_name        text not null,        -- SNAPSHOT
  quantity           smallint not null default 1 check (quantity > 0),
  price_delta        numeric(10,2) not null,   -- SNAPSHOT per unit
  unit_cost          numeric(12,4) not null default 0  -- SNAPSHOT
);
create index on order_line_modifiers (order_line_id);
```

**Payments — split payments are just multiple rows** (fixes #7 for methods):

```sql
create type payment_kind as enum ('efectivo','tarjeta','transferencia','plataforma','otro');

create table payment_methods (
  id smallint generated always as identity primary key,
  name citext not null unique,             -- Efectivo, Tarjeta, SPEI, Didi, Uber Eats, Rappi
  kind payment_kind not null,
  affects_cash_drawer boolean not null default false,  -- only Efectivo counts in corte expected-cash
  is_active boolean not null default true,
  sort_key numeric(18,9) not null default 1000
);

create table order_payments (
  id bigint generated always as identity primary key,
  order_id   bigint not null references orders(id),
  payment_method_id smallint not null references payment_methods(id),
  amount     numeric(10,2) not null check (amount > 0),
  tip_amount numeric(10,2) not null default 0 check (tip_amount >= 0),
  register_session_id bigint references register_sessions(id),
  received_by bigint references users(id),
  reference  text,                          -- card auth / platform order id
  created_at timestamptz not null default now()
);
create index on order_payments (order_id);
create index on order_payments (register_session_id, payment_method_id);
```
`paid` is derived: `sum(amount) >= orders.total`. Refund/void = compensating negative order + note (post-MVP); MVP forbids editing payments after corte close.

## 7. Cash: cortes de caja, expenses, tips

```sql
create type session_status as enum ('abierta','cerrada');

create table register_sessions (
  id bigint generated always as identity primary key,
  business_date date not null,
  status session_status not null default 'abierta',
  opening_cash numeric(10,2) not null default 0,
  opened_by bigint not null references users(id),
  opened_at timestamptz not null default now(),
  closed_by bigint references users(id),
  closed_at timestamptz,
  notes text
);
create unique index one_open_session on register_sessions (status) where status = 'abierta';

create table register_session_totals (        -- written at close: declared vs expected per method
  session_id bigint not null references register_sessions(id) on delete cascade,
  payment_method_id smallint not null references payment_methods(id),
  expected  numeric(10,2) not null,           -- Σ payments (+opening cash & movements for Efectivo)
  declared  numeric(10,2) not null,           -- what the cashier counted
  difference numeric(10,2) generated always as (declared - expected) stored,
  primary key (session_id, payment_method_id)
);

create table register_cash_movements (        -- paid-ins/paid-outs affecting the drawer mid-session
  id bigint generated always as identity primary key,
  session_id bigint not null references register_sessions(id),
  kind text not null check (kind in ('entrada','salida')),
  amount numeric(10,2) not null check (amount > 0),
  concept text not null,                      -- 'PROPINAS pagadas', 'cambio', 'compra hielo'
  expense_id bigint references expenses(id),
  user_id bigint not null references users(id),
  created_at timestamptz not null default now()
);
```

Expected cash at close = `opening_cash + Σ order_payments(Efectivo, session) + Σ entradas − Σ salidas`. Non-cash methods: expected = Σ payments of that method in session (reconciles card terminal / platform reports). Tips: card tips accumulate in `order_payments.tip_amount`; payout to staff is a `salida` linked to an expense in category "Propinas" — exactly how the owner already uses FUDO (PROPINAS payee), but structured.

**Expenses — 2-level taxonomy kept** (financial group → category), payee as entity:

```sql
create type financial_group as enum ('operacional','administrativo','otro');

create table expense_categories (
  id bigint generated always as identity primary key,
  name citext not null,
  financial_group financial_group not null,
  is_active boolean not null default true,
  unique (financial_group, name)
);

create table expenses (
  id bigint generated always as identity primary key,
  expense_date date not null,                 -- accounting date, backdatable (FUDO behavior kept)
  category_id bigint not null references expense_categories(id),
  supplier_id bigint references suppliers(id),   -- payee, first-class
  amount numeric(10,2) not null check (amount > 0),
  payment_method_id smallint references payment_methods(id),
  register_session_id bigint references register_sessions(id),  -- set when paid from drawer
  description text,
  created_by bigint not null references users(id),
  created_at timestamptz not null default now()
);
create index on expenses (expense_date);
```
A stock purchase = one `expenses` row + `compra` stock movements linking back via `expense_id` (no separate purchase-order module in MVP).

## 8. Users / employees

```sql
create type user_role as enum ('admin','cajero','mesero');

create table users (
  id bigint generated always as identity primary key,
  name text not null,
  username citext unique,             -- backoffice login (admin)
  role user_role not null,
  pin_hash text,                      -- bcrypt of 4–6 digit PIN; POS quick-switch
  password_hash text,                 -- backoffice password (admins only)
  is_active boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
```
Auth model: the tablet holds a long-lived **device token**; each order/action is attributed by PIN entry (server verifies PIN → user_id, short in-memory session per terminal). Every transactional table already carries `user_id` audit columns (`opened_by`, `cancelled_by`, `created_by`, movements' `user_id`) — no generic audit table for MVP. Seed a shared "Mostrador" user? **No** — that's the FUDO anti-pattern that made attribution useless (602 cancelled items on "Mostrador"); PIN-switch makes personal attribution cheap.

## 9. Costing engine (recursive roll-up + snapshots)

**Formulas** (all in base units):

```
ingredient_cost(i) =
  raw:  current_cost (set manually or from last compra: expense/format math)
  prep: recipe_cost(i.recipe_id) / i.yield_qty

recipe_cost(r) = Σ over items:
  qty_base(item) × ingredient_cost(item.ingredient) × (1 + waste_pct/100)
  where qty_base = quantity × unit.to_base            -- merma inflates cost, FUDO-compatible

product_cost(p)  = manual_cost                      if cost_source='manual'
                 = recipe_cost(p.recipe_id)          if cost_source='receta'
combo_cost(c)    = Σ slots: Σ default options' product_cost × 1 (min_select copies)
option_cost(o)   = recipe_cost(o.recipe_id) | product_cost(linked_product) | 0
```

**Implementation**: a Go service (`internal/costing`) — not PL/pgSQL — because it needs cycle detection, memoization, and Redis invalidation anyway. Cascade trigger points (all synchronous, catalog is ~750 products so full recompute is <50 ms):
1. Ingredient `current_cost`/`waste_pct` change or `compra` movement → recompute dependent preps (via `recipe_items.ingredient_id` reverse index), then dependent products/options, update `current_cost` caches in one tx.
2. `recipe_items` change → recompute that recipe's owner + upward closure.
3. Price change → margin is a generated column, nothing to do.
4. Nightly full recompute job as a safety net (idempotent).

**Snapshots** (historical profit survives cost changes — hard requirement): at order-line insert, copy `products.current_cost` (+ Σ selected `option.current_cost × qty`) into `order_lines.unit_cost` and per-option `order_line_modifiers.unit_cost`, plus name/price snapshots. Profit reports read **only** snapshot columns; the live `current_cost` is never joined into sales history. This makes a `cost_history` table unnecessary for MVP.

## 10. CQRS-light read side (Redis + Postgres)

Write path stays one Postgres tx (order + lines + modifiers + movements + counters); read models are caches, all rebuildable:

| Read model | Store | Content | Invalidation |
|---|---|---|---|
| `menu:v{N}` | Redis string (JSON) | Full POS catalog: categories→products→attached groups→options, prices, sort keys, favorite flags, channel visibility resolved | Version key `menu:version` INCR by any write to categories/products/modifier*/combo*/channels; tablets poll version (or SSE) and refetch — one GET serves the whole POS |
| `avail:{product_id}` | Redis hash | Derived Disponibilidad + `allow_oversell` | On any stock movement: resolve affected ingredients → `SMEMBERS rev:ing:{id}` (ingredient→product reverse index, rebuilt on recipe change) → recompute those products only |
| `board:{service_type}` | Redis sorted set (score=opened_at) + pub/sub `orders_events` | Active orders (abierta/lista) JSON for the Ventas Activas screen | On order insert/state change; SSE/WebSocket push to tablets (fixes the no-polling staleness) |
| `dash:{business_date}` | Redis hash | Tickets count, ventas $, per-method totals, cancellations | INCR on payment/order events; authoritative numbers always re-queried from Postgres for the corte |

No Postgres materialized views for MVP — at ~2,600 tickets/11 months, plain indexed queries (indexes above: `orders(business_date,status)`, `order_payments(register_session_id, payment_method_id)`, `order_lines(product_id, created_at)`) answer every report instantly. Revisit matviews only if reports ever slow down.

## 11. FUDO-weakness fix map (section 8 of the fudo report)

| # | FUDO weakness | Fix in this schema |
|---|---|---|
| 1 | Modifier options as pseudo-products | `modifier_options` entity with own `recipe_id`/`linked_product_id` (§4) |
| 2 | Name-based joins | ID FKs everywhere; citext-unique names only as human keys |
| 3 | Trivial unit system | `units` with kind+`to_base`; purchase formats; preps with yield (§1–2) |
| 4 | Negative stock chaos | Ledger allows negatives (truth), Disponibilidad derived & clamped, `allow_oversell` gate at sale time (§5) |
| 5 | Margin as absolute $ | Stored generated `margin_amount` + derived %, auto from cached cost |
| 6 | Combos underpowered | `combo_slots`/`combo_slot_products` first-class (§3) |
| 7 | Free-string providers/methods/payees | `suppliers`, `payment_methods`, `expense_categories` entities |
| 8 | Tri-state channel visibility | Kept & generalized: `channels` + override rows, absence = inherit |
| 9 | Spaced-integer Posición | `numeric(18,9) sort_key` fractional ranking, no renumbering |
| 10 | No order/line/state granularity | Full order→line→modifier→payment model, 4-state machine, per-line cancellation with reason+user |

## 12. Data import from `references/` FUDO exports

Go CLI `cmd/fudo-import` (read xls via `excelize` after converting .xls→.xlsx, or pre-convert to CSV). Idempotent, staged, with an unmatched-rows report. Traceability table:

```sql
create table fudo_import_map (
  entity text not null, fudo_id bigint, fudo_name text,
  new_table text not null, new_id bigint not null,
  primary key (entity, coalesce(fudo_id,0), coalesce(fudo_name,''))
);
```

Order of phases (name normalization: `upper(trim(regexp_replace(name,'\s+',' ')))`):
1. **Seeds**: units, channels (pos/qr/online), payment methods (from Medios de pago sheet: Efectivo, Tarjeta, Transferencia SPEI, Didi, Uber Eats, Rappi — drop the "Efectivo Uber Eats" $2 noise row), delivery platforms, expense categories (from Reporte-Gastos taxonomy), users (Admin, Carlos, Kate, Brenda — PINs set manually).
2. **Suppliers**: distinct `Proveedor` strings from ingredientes + productos + gastos, deduped.
3. **Ingredient categories + ingredients** (`ingredientes.xls`): map Unidad (kg→base g? — keep base = kg/L/pieza as-is for operator familiarity; `to_base` handles display), Merma→`waste_pct`, Costo→`current_cost`, Control de Stock→`track_stock`, Stock Mínimo from `stock.xls`.
4. **Subingredientes (25 rows) — triage per row**: parents whose lines look like real recipes (Salsa Verde/Roja, Mezcla Crepas, Mezcla Mini Donas) → prep ingredients (`is_prep`, recipe, `yield_qty` = parent pack size); conversion hacks (Mayonesa 870g=75.269 Cda, Rebanada=0.05 barra, Champiñones=2.632 lata) → collapse into one ingredient with proper base unit + `ingredient_purchase_formats` row; delete the counterpart fake ingredient and rewrite recipe references.
5. **Categories** (2-level from Categoría/Subcategoría columns) — **excluding "Otro"'s modifier subcategories** (Modificadores genéricos/Adicionales/EXTRAS/Decoraciones), which don't migrate as categories.
6. **Products**: the 752 rows minus pure modifier-option pseudo-products. Rule: a FUDO product migrates as a **product** if it is NOT referenced in `Modificadores - Productos`, OR it is referenced but also standalone-sellable (`Permitir vender solo=Si` and category ≠ "Otro"). Map Activo→is_active, Favorito, Vender sin stock→allow_oversell, Posición→sort_key (divide by 1e6), Tienda Online/Menú QR tri-state→`product_channels` override rows (only Si/No rows; 'Según categoría' = no row), Control de Stock+direct Stock→track_stock.
7. **Recipes** (`Recetas`, 620 rows): create `recipes` + items joined by normalized name → ingredient map; set `cost_source='receta'`; report the ~4 recipe products without cost.
8. **Modifier groups & options**: groups keyed by FUDO's numeric `ID Grupo modificador` (the one real ID we get); group name = most common Título else `Grupo {id}`. Options from `Modificadores - Productos`: name = option product name, `price_delta` = sheet Precio (**the sheet override wins**, matched product price only 101/485 times; flag the $0.01 penny rows and the $1510 outlier for manual review), `max_per_line` = Máxima. If the option's pseudo-product has a recipe → create the recipe on the **option** (`recipe_id`); if the pseudo-product also migrated as a real product (dual-role like SOJU) → share the same `recipe_id` is impossible (unique FK) → duplicate the recipe rows, or set `linked_product_id` instead when it's a direct-stock item. Attachments from `Modificadores - Grupos` → `product_modifier_groups` (Título, min, max). Skip the 9 orphan group IDs unless attached to a migrating product.
9. **Combos**: only 4 + 3 Subproductos rows — migrate **manually** with the owner (the BANDERILLA 25×/15× rows are batch-yield misuse, discard; COMBO SOJU → combo slot with SOJU).
10. **Opening stock**: from `stock.xls` raw `Stock` (products with direct stock + all ingredients) → one `ajuste` movement each (`reason='importación FUDO'`, `unit_cost=current_cost`). Import negatives as-is, then schedule a physical count (conteo → ajuste) at go-live so the ledger starts truthful.
11. **History**: sales/expense reports are pre-aggregated — do **not** import into transactional tables. Optionally load `Evolución de ventas`/`Medios de pago` into a read-only `legacy_daily_sales` table for year-over-year charts (post-MVP).
12. **Validation pass**: recompute all costs, diff against FUDO's `Costo` column (331 products), report deltas > $0.50; diff derived Disponibilidad against `stock.xls` Disponibilidad.

## 13. Repo layout for this deliverable

```
backend/
  migrations/            -- golang-migrate, numbered:
    0001_extensions.sql        0006_stock.sql
    0002_units_suppliers.sql   0007_orders.sql
    0003_ingredients_recipes.sql 0008_payments_cash.sql
    0004_catalog.sql           0009_expenses_users.sql
    0005_modifiers.sql         0010_triggers.sql   -- updated_at, stock_levels upsert, unit-kind & category-depth guards
    0011_seeds.sql             -- units, channels, enums' lookup rows
  internal/costing/      -- roll-up engine + invalidation
  internal/readmodel/    -- Redis builders (menu, avail, board, dash)
  cmd/fudo-import/       -- §12 importer
```

Key triggers (0010): `set_updated_at` on mutable tables; `stock_levels` upsert after movement insert; `recipe_items` unit-kind check; `categories` depth ≤ 2 guard; order status-transition guard (allowed pairs only, cancelled-fields consistency already CHECKed).

Open items intentionally deferred (post-MVP, schema already accommodates): discounts/promotions entity (column placeholders exist on orders), customer entity (name string for now), tables/mesas, refunds, multi-register, purchase orders.
