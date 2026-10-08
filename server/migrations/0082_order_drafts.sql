-- +goose Up

-- LA CUENTA EN CAPTURA VIVE EN EL SERVIDOR (spec 030, D-1).
--
-- El número es de trabajo: toma el siguiente libre de develop al fusionar (0080–0081 son de otra
-- rama; goose corre sin AllowMissing).
--
-- POR QUÉ TABLAS PROPIAS Y NO UN ESTADO NUEVO DE `orders`. Las ~30 consultas de dinero filtran los
-- pedidos por EXCLUSIÓN de estado (`status not in ('cancelada','reembolsada')`). Un estado
-- `capturando` en el enum entraría en silencio a ventas, corte, reportes, recetas y Top, y habría que
-- acordarse de excluirlo en cada una — y en la número 31. Con tabla propia, una cuenta que se está
-- capturando no es un pedido por construcción: ninguna consulta de dinero la puede contar.
--
-- Solo crea tablas: no toca `orders` ni ninguna tabla viva, así que no toma candados sobre lo que
-- está vendiendo. Todos los unique que piden las FKs compuestas ya existen: products_tenant_key
-- (0071), delivery_platforms_id_company_key (0037), orders_id_company_key (0065), users_tenant_key
-- (0073).
--
-- Las FKs llevan la empresa en la llave porque los chequeos de integridad referencial de Postgres
-- saltan RLS: sin eso, una cuenta de una empresa podría colgarse del producto o del pedido de otra.
-- Van `no action` y no `restrict`: `restrict` no se difiere y aborta el borrado en cascada de una
-- empresa (mismo motivo que 0079).
--
-- CRECIMIENTO (estimado, sin `delete` a propósito: la descartada es rastro): ~30 mil cuentas, ~150
-- mil renglones y ~200 mil agregados por año a 40–80 pedidos/día; decenas de MB/año. Techo y camino:
-- cuando pese, purgar `order_draft_adds` de cuentas terminales con más de 30 días (los agregados solo
-- sirven para que un reintento no duplique, y una cuenta enviada o descartada ya no recibe
-- reintentos). No se construye hoy.

-- ---------------------------------------------------------------------------------------------
-- La cuenta
-- ---------------------------------------------------------------------------------------------
create table order_drafts (
  -- Lo pone la tableta: es la llave de idempotencia de crear y, al enviarla, el `client_uuid` del
  -- pedido o del lote de renglones.
  id                   uuid primary key,
  company_id           bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                         references companies(id) on delete cascade,
  -- Nulo = cuenta nueva. No nulo = lo «Nuevo» de un pedido ya enviado (D-3), o el pedido en que se
  -- convirtió al enviarse.
  order_id             bigint,
  -- Texto y no enum: un enum nuevo no se quita en el Down sin recrear la tabla.
  status               text not null default 'capturando'
                         check (status in ('capturando', 'enviada', 'descartada')),
  -- El nombre se amarra al nacer (D-2) y se conserva al enviarse, como rastro. Nulo solo en lo
  -- «Nuevo» de un pedido: su nombre es el del pedido.
  folio_name           text,
  -- De qué bolsa salió, para devolverlo a ESA bolsa al descartar aunque el negocio cambie de esquema.
  folio_scheme         folio_scheme,
  service_type         service_type not null default 'mostrador',
  customer_name        text check (char_length(customer_name) <= 60),
  delivery_platform_id smallint,
  platform_order_ref   text check (
    platform_order_ref is null
    or (platform_order_ref = btrim(platform_order_ref, E' \t\n\r')
        and length(platform_order_ref) between 1 and 64)),
  delivery_fee         numeric(10,2) not null default 0 check (delivery_fee >= 0),
  discount_amount      numeric(10,2) check (discount_amount is null or discount_amount >= 0),
  discount_percent     numeric(5,2) check (discount_percent is null or discount_percent between 0 and 100),
  -- Quién puso el descuento y quién tecleó el folio EN LA CUENTA. Al enviarla pasan al pedido: sin
  -- esto el pedido diría que los puso quien abrió la cuenta, y la puerta «Descuentos» del principio
  -- VIII pide saber quién descontó.
  discount_set_by      bigint,
  platform_ref_set_by  bigint,
  -- Quién la abrió (FR-020). Pasa al pedido como `opened_by`.
  opened_by            bigint not null,
  header_version       int not null default 1,
  created_at           timestamptz not null default now(),
  -- Cualquier cambio. Base de las 12 horas (D-8).
  updated_at           timestamptz not null default now(),
  sent_at              timestamptz,
  discarded_at         timestamptz,
  -- Nulo = la descartó el barrido (12 horas o pedido cancelado), no una persona.
  discarded_by         bigint,
  discard_reason       text check (discard_reason in ('manual', 'expired', 'order_closed', 'empty')),

  constraint order_drafts_id_company_key unique (id, company_id),
  constraint order_drafts_order foreign key (order_id, company_id)
    references orders (id, company_id) on delete no action,
  constraint order_drafts_platform foreign key (delivery_platform_id, company_id)
    references delivery_platforms (id, company_id) on delete no action,
  constraint order_drafts_opened_by foreign key (company_id, opened_by)
    references users (company_id, id) on delete no action,
  constraint order_drafts_discount_set_by foreign key (company_id, discount_set_by)
    references users (company_id, id) on delete no action,
  constraint order_drafts_platform_ref_set_by foreign key (company_id, platform_ref_set_by)
    references users (company_id, id) on delete no action,
  constraint order_drafts_discarded_by foreign key (company_id, discarded_by)
    references users (company_id, id) on delete no action,

  constraint order_drafts_named check (order_id is not null or folio_name is not null),
  constraint order_drafts_scheme_pair check ((folio_name is null) = (folio_scheme is null)),
  constraint order_drafts_sent_pair check ((status = 'enviada') = (sent_at is not null)),
  -- Enviada ⇒ apunta a su pedido: por eso lo «Nuevo» se distingue por `folio_name is null` y no por
  -- `order_id`.
  constraint order_drafts_sent_has_order check (status <> 'enviada' or order_id is not null),
  constraint order_drafts_discarded_pair check ((status = 'descartada') = (discarded_at is not null)),
  constraint order_drafts_reason_pair check ((status = 'descartada') = (discard_reason is not null)),
  constraint order_drafts_one_discount check (discount_amount is null or discount_percent is null),
  constraint order_drafts_discount_author check (
    (discount_amount is null and discount_percent is null) = (discount_set_by is null)),
  constraint order_drafts_platform_ref_author check ((platform_order_ref is null) = (platform_ref_set_by is null))
);

-- Dos cuentas vivas no comparten nombre (FR-001): es lo que decide cuál de dos tabletas que abren
-- cuenta a la vez se queda con el nombre; la otra reintenta con otro.
create unique index order_drafts_live_name on order_drafts (company_id, folio_name)
  where status = 'capturando' and folio_name is not null;
-- Una sola «Nuevo» viva por pedido: la segunda tableta recibe la que ya existe.
create unique index order_drafts_live_per_order on order_drafts (company_id, order_id)
  where status = 'capturando' and order_id is not null;
-- La lista de cuentas vivas y el barrido de las 12 horas.
create index order_drafts_live on order_drafts (company_id, updated_at) where status = 'capturando';
-- Pegar lo «Nuevo» a su pedido en la lista, y el barrido de lo «Nuevo» de pedidos cancelados.
create index order_drafts_by_order on order_drafts (company_id, order_id) where order_id is not null;

-- ---------------------------------------------------------------------------------------------
-- Sus renglones
-- ---------------------------------------------------------------------------------------------
create table order_draft_lines (
  -- Lo pone la tableta: el `opId` del agregado que creó el renglón.
  id         uuid primary key,
  company_id bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
               references companies(id) on delete cascade,
  draft_id   uuid not null,
  product_id bigint not null,
  qty        numeric(8,2) not null check (qty > 0),
  -- [{optionId, qty, portion}], validado en domain.ValidateDraftLine. Sin FK a las opciones: una
  -- opción borrada con la cuenta viva se descubre al enviar, como un 422 que nombra el producto.
  modifiers  jsonb not null default '[]' check (jsonb_typeof(modifiers) = 'array'),
  notes      text check (char_length(notes) <= 200),
  position   int not null,
  version    int not null default 1,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint order_draft_lines_draft foreign key (draft_id, company_id)
    references order_drafts (id, company_id) on delete cascade,
  -- `no action` y no `restrict` (ver arriba). Igual bloquea el reorg de datos que borre un producto
  -- con una cuenta viva que lo tiene.
  constraint order_draft_lines_product foreign key (company_id, product_id)
    references products (company_id, id) on delete no action
);
create index order_draft_lines_by_draft on order_draft_lines (company_id, draft_id, position);

-- ---------------------------------------------------------------------------------------------
-- Un renglón por cada «agregar»: la llave que hace idempotente el toque
-- ---------------------------------------------------------------------------------------------
-- Agregar se SUMA (D-5: lo que agrega cada tableta se suma), así que no se puede versionar como un
-- cambio. La idempotencia sale de anotar cada toque: un reintento con el mismo `op_id` no suma otra
-- vez.
create table order_draft_adds (
  company_id bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
               references companies(id) on delete cascade,
  op_id      uuid not null,
  draft_id   uuid not null,
  -- SIN FK a propósito: si el renglón se quitó después, el reintento de aquel agregado tiene que
  -- seguir siendo un no-op, no volver a crear el renglón ni fallar.
  line_id    uuid not null,
  created_at timestamptz not null default now(),
  primary key (company_id, op_id),
  constraint order_draft_adds_draft foreign key (draft_id, company_id)
    references order_drafts (id, company_id) on delete cascade
);
create index order_draft_adds_by_draft on order_draft_adds (company_id, draft_id);

-- ---------------------------------------------------------------------------------------------
-- RLS y permisos
-- ---------------------------------------------------------------------------------------------
alter table order_drafts enable row level security;
create policy tenant_isolation on order_drafts
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);

alter table order_draft_lines enable row level security;
create policy tenant_isolation on order_draft_lines
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);

alter table order_draft_adds enable row level security;
create policy tenant_isolation on order_draft_adds
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);

-- El grant de 0024 fue puntual, sin default privileges: cada tabla nueva lleva el suyo, o el primer
-- request en producción responde 42501.
--
-- Sin `delete` en la cuenta: la descartada se conserva (rastro de lo abandonado, D-7). Los renglones
-- SÍ se borran: quitar un producto de una cuenta que no se ha mandado no es dinero ni venta. Los
-- agregados solo se insertan.
grant select, insert, update on order_drafts to gatobobah_app;
grant select, insert, update, delete on order_draft_lines to gatobobah_app;
grant select, insert on order_draft_adds to gatobobah_app;

-- +goose Down

-- Se niega si perdería algo: una cuenta capturándose es lo que alguien tiene enfrente sin mandar a
-- cocina, y una enviada guarda quién capturó el pedido. Tras desplegar, deshacer esta migración con
-- datos es restaurar el respaldo.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from order_drafts where status = 'capturando') then
    raise exception 'hay cuentas capturando en order_drafts: se perdería lo que alguien está capturando';
  end if;
  if exists (select 1 from order_drafts where status = 'enviada') then
    raise exception 'hay cuentas enviada en order_drafts: se perdería quién capturó esos pedidos';
  end if;
end $$;
-- +goose StatementEnd

drop table order_draft_adds;
drop table order_draft_lines;
drop table order_drafts;
