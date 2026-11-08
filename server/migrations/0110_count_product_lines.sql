-- +goose Up
-- LOS PRODUCTOS CON EXISTENCIA PROPIA ENTRAN AL CONTEO (spec 069, issue #252).
--
-- Tabla aparte y no una columna `product_id` en `stock_count_lines`: ahí `ingredient_id` es parte
-- de la llave primaria y la API anterior escribe con `on conflict (count_id, ingredient_id)`;
-- volverlo nulable obligaba a cambiar la llave durante una subida con las dos versiones vivas
-- (constitución IV). Y las columnas de pesada, densidad y empaques hablan de artículos. La API
-- anterior no lee ni escribe esta tabla: es solo agregar.
--
-- Si la API anterior cierra un conteo durante el traslape, sus renglones de producto quedan sin
-- ajustar. Se reconocen porque `unit_cost` sigue nulo en un conteo cerrado: solo la API nueva lo
-- llena al cerrar.
create table stock_count_product_lines (
  count_id         bigint not null,
  company_id       bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                   references companies(id) on delete cascade,
  product_id       bigint not null,
  -- Piezas enteras: una lata no se parte. 0 = «no hay»; sin renglón = «no se contó».
  counted_qty      numeric(14,4) not null
                   constraint stock_count_product_lines_counted_whole check (counted_qty >= 0 and counted_qty = trunc(counted_qty)),
  -- Lo que el sistema tenía al capturar: el ajuste se calcula contra esto y no contra lo de al
  -- cerrar, así lo vendido mientras se cuenta ya restó. Puede ser negativo.
  system_qty       numeric(14,4) not null,
  captured_by      bigint not null,
  captured_at      timestamptz not null default now(),
  -- El último movimiento del producto en la sucursal al capturar: ancla el periodo de un faltante
  -- futuro por id y no por hora (spec 039, R2). Sin llave foránea a propósito, como en
  -- `stock_count_lines`: 0 significa «sin movimientos» y solo se compara.
  last_movement_id bigint,
  -- El costo del producto al cerrar: el faltante de este conteo no cambia cuando cambia el costo.
  unit_cost        numeric(12,4),
  primary key (count_id, product_id),
  constraint stock_count_product_lines_count_fkey
    foreign key (count_id, company_id) references stock_counts (id, company_id) on delete cascade,
  constraint stock_count_product_lines_product_fkey
    foreign key (product_id, company_id) references products (id, company_id),
  constraint stock_count_product_lines_captured_by_fkey
    foreign key (company_id, captured_by) references users (company_id, id)
);
create index stock_count_product_lines_by_product on stock_count_product_lines (company_id, product_id);

alter table stock_count_product_lines enable row level security;
create policy tenant_isolation on stock_count_product_lines
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Quitar una cifra («dejar sin contar») borra el renglón; los de un conteo cerrado los protege el
-- servicio, que solo escribe con el conteo abierto y bloqueado.
grant select, insert, update, delete on stock_count_product_lines to gatobobah_app;

-- +goose Down
-- Se niega con cifras: son la única copia de lo que se contó. Los ajustes que dejaron en
-- `stock_movements` sobreviven a este Down (y el Down de la 0092 ya se niega con ellos).
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from stock_count_product_lines) then
    raise exception 'hay cifras de productos en conteos: son la única copia de lo que se contó';
  end if;
end;
$$;
-- +goose StatementEnd
drop table stock_count_product_lines;
