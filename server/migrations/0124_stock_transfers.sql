-- +goose Up

-- TRASPASOS ENTRE ALMACENES DE UNA SUCURSAL (spec 094, tercera subida; research R7, R11 H4/M3/M6, R14).
--
-- Un traspaso es un documento: origen, destino, nota, quién y cuándo, y sus renglones. Cada renglón
-- produce dos movimientos `traspaso` ligados por `transfer_id`: una salida del origen y una entrada al
-- destino, los dos con el costo vigente del artículo (snapshot, principio VIII). No se edita ni se
-- borra: se corrige con otro traspaso. Es idempotente por `client_key` (la tableta lo genera): un
-- reintento con el mismo cuerpo devuelve el mismo traspaso.
--
-- Puerta para lotes (#146, R14): un renglón es artículo y cantidad; el lote de origen podrá agregarse
-- después como columna nullable sin reescribir traspasos pasados.

set local lock_timeout = '3s';

create table stock_transfers (
  id               bigint generated always as identity primary key,
  company_id       bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                   references companies (id) on delete cascade,
  branch_id        bigint not null,
  from_warehouse_id bigint not null,
  to_warehouse_id  bigint not null,
  note             text check (note is null or char_length(note) <= 200),
  -- La clave que manda la tableta y la huella del cuerpo: un reintento igual devuelve el mismo
  -- traspaso; la misma clave con otro cuerpo es un error de la tableta, no un traspaso nuevo.
  client_key       uuid not null,
  body_hash        text not null,
  created_by       bigint not null references users (id),
  created_at       timestamptz not null default now(),
  constraint stock_transfers_branch_of_company foreign key (company_id, branch_id) references branches (company_id, id),
  -- El almacén es de la sucursal del traspaso, aunque RLS fallara (las llaves foráneas saltan RLS).
  constraint stock_transfers_from_of_branch foreign key (company_id, branch_id, from_warehouse_id)
    references warehouses (company_id, branch_id, id),
  constraint stock_transfers_to_of_branch foreign key (company_id, branch_id, to_warehouse_id)
    references warehouses (company_id, branch_id, id),
  constraint stock_transfers_distinct_warehouses check (from_warehouse_id <> to_warehouse_id),
  constraint stock_transfers_client_key unique (company_id, client_key),
  constraint stock_transfers_tenant_key unique (id, company_id),
  -- Destino de la llave de los movimientos: un movimiento de traspaso es de la sucursal del traspaso.
  constraint stock_transfers_branch_key unique (id, company_id, branch_id)
);
create index stock_transfers_by_branch on stock_transfers (company_id, branch_id, created_at desc);

create table stock_transfer_lines (
  transfer_id   bigint not null,
  line_no       smallint not null check (line_no > 0),
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                references companies (id) on delete cascade,
  item_type     stock_item_type not null,
  ingredient_id bigint,
  product_id    bigint,
  qty_base      numeric(14,4) not null check (qty_base > 0),
  primary key (transfer_id, line_no),
  constraint stock_transfer_lines_of_transfer foreign key (transfer_id, company_id) references stock_transfers (id, company_id),
  constraint stock_transfer_lines_ingredient_fkey foreign key (ingredient_id, company_id) references ingredients (id, company_id),
  constraint stock_transfer_lines_product_fkey foreign key (product_id, company_id) references products (id, company_id),
  constraint stock_transfer_lines_one_item check ((item_type = 'ingrediente') = (ingredient_id is not null)
                                                   and (item_type = 'producto') = (product_id is not null))
);
create index stock_transfer_lines_by_ingredient on stock_transfer_lines (company_id, ingredient_id) where ingredient_id is not null;

alter table stock_transfers enable row level security;
create policy tenant_isolation on stock_transfers
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
alter table stock_transfer_lines enable row level security;
create policy tenant_isolation on stock_transfer_lines
  using      (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Sin update ni delete: un traspaso no se edita.
grant select, insert on stock_transfers, stock_transfer_lines to gatobobah_app;

-- Un renglón nuevo que apunta a un insumo unido se rechaza nombrando el destino (las dos guardas de la
-- 0104: inmediata y diferida). Los movimientos del traspaso sí se redirigen, como cualquier movimiento.
create trigger stock_transfer_lines_merged_guard before insert or update of ingredient_id on stock_transfer_lines
  for each row execute function reject_merged_ingredient();
create constraint trigger stock_transfer_lines_merged_check after insert or update of ingredient_id on stock_transfer_lines
  deferrable initially deferred for each row execute function reject_merged_ingredient();

-- Los movimientos del traspaso, ligados al documento y en su sucursal.
alter table stock_movements add column transfer_id bigint;
alter table stock_movements add constraint stock_movements_transfer_of_branch
  foreign key (transfer_id, company_id, branch_id) references stock_transfers (id, company_id, branch_id);
create index stock_movements_by_transfer on stock_movements (company_id, transfer_id) where transfer_id is not null;

-- +goose Down

set local lock_timeout = '3s';

-- +goose StatementBegin
do $$
begin
  if exists (select 1 from stock_transfers) then
    raise exception 'hay traspasos registrados: bajar la 0124 perdería de dónde vino lo que se movió entre almacenes';
  end if;
end $$;
-- +goose StatementEnd

drop index stock_movements_by_transfer;
alter table stock_movements drop constraint stock_movements_transfer_of_branch, drop column transfer_id;
drop table stock_transfer_lines;
drop table stock_transfers;
