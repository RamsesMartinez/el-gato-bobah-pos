-- +goose Up
-- El libro de devoluciones, y de qué renglón salió cada descuento de inventario.
--
-- Hasta aquí `orders.refund_amount` era UN escalar, y con un escalar no se puede responder "¿qué se
-- devolvió y por qué método?". Devolver un platillo de tres —que en este negocio ya pasa— exigía
-- devolver la cuenta entera o arreglarlo fuera del sistema, donde ningún reporte lo ve.
--
-- lock_timeout como en 0040-0042 y por el mismo motivo: el ALTER de stock_movements toma un lock
-- sobre una tabla que crece con cada venta. Con la cola de una transacción larga abierta, el ALTER
-- se encola detrás y arrastra toda lectura nueva. Es preferible que la migración falle limpio en 3
-- segundos a que el POS se quede mudo esperando.
set local lock_timeout = '3s';

-- Una fila por devolución PARCIAL: un pedido puede tener varias, de renglones distintos y por
-- métodos distintos.
create table order_refunds (
  id                bigserial primary key,
  order_id          bigint not null references orders(id) on delete cascade,
  -- NULL = se devolvió contra la cuenta entera, no contra un renglón. Es el caso de siempre y el
  -- que usan las devoluciones de un pedido completo.
  --
  -- Sin `on delete cascade`: un renglón no se borra nunca (cancelarlo lo marca, no lo quita), y si
  -- alguna vez se borrara, perder el rastro de a qué se devolvió el dinero es peor que la fila
  -- huérfana. `restrict` obliga a mirarlo.
  order_line_id     bigint references order_lines(id) on delete restrict,
  -- El método POR EL QUE ENTRÓ el dinero, que es por donde tiene que salir. Sin `on delete`: los
  -- métodos no se borran, se desactivan.
  payment_method_id smallint not null references payment_methods(id),
  amount            numeric(10,2) not null check (amount > 0),
  reason            text not null check (length(btrim(reason)) > 0),
  refunded_by       bigint not null references users(id),
  -- El movimiento de caja que sacó este dinero del cajón. NULL = no salió del cajón (tarjeta,
  -- plataformas): ese dinero nunca estuvo en la caja y descontarlo inventaría un faltante.
  cash_movement_id  bigint references register_cash_movements(id) on delete set null,
  created_at        timestamptz not null default now(),
  company_id        bigint not null default current_setting('app.company_id', true)::bigint
                    references companies(id) on delete cascade
);

-- Arranca por company_id a propósito: RLS le pega ese predicado a toda consulta del rol de la app, y
-- un índice que empiece por order_id se queda descartando filas de otras empresas dentro del scan.
-- Cubre las dos consultas que existen: lo devuelto de un pedido y lo devuelto de un renglón.
create index order_refunds_company_order on order_refunds (company_id, order_id, order_line_id);

alter table order_refunds enable row level security;
create policy tenant_isolation on order_refunds
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- El grant de 0024 fue `on all tables in schema public`, que es PUNTUAL: no hay default privileges,
-- así que cada tabla creada después necesita el suyo. Sin esto la migración pasa, los tests pasan y
-- `make start` pasa —dev sirve como owner— y en producción la primera devolución devuelve 42501.
--
-- Sin `delete`: una devolución registrada no se borra. Si se hizo mal, se corrige con otra fila que
-- lo diga, porque el rastro de a dónde fue el dinero es justo lo que este libro existe para guardar.
grant select, insert, update on order_refunds to gatobobah_app;
grant usage, select on sequence order_refunds_id_seq to gatobobah_app;

-- De qué RENGLÓN salió cada descuento de inventario.
--
-- Sin esto, reponer un renglón cancelado obliga a recalcular su consumo con la receta de HOY, y una
-- receta que cambió entre la venta y la cancelación repondría una cantidad distinta de la que
-- salió. Con la referencia se revierte lo que de verdad se descontó.
--
-- Lo histórico queda en NULL, y es la decisión: de un movimiento viejo no consta a qué renglón
-- pertenecía, y repartirlo por producto sería afirmar algo que nadie sabe. Un renglón de un pedido
-- anterior a esta migración no se repone con precisión — se dice, no se adivina.
alter table stock_movements add column order_line_id bigint references order_lines(id) on delete set null;

-- Solo las filas que tienen renglón: el índice existe para "revierte lo de ESTE renglón", y los
-- millones de movimientos históricos con NULL no tienen por qué ocupar lugar en él.
create index stock_movements_line on stock_movements (order_line_id) where order_line_id is not null;

-- +goose Down
drop index if exists stock_movements_line;
alter table stock_movements drop column if exists order_line_id;
drop table if exists order_refunds;
