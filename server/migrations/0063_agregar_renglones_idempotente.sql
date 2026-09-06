-- +goose Up
-- AGREGAR RENGLONES A UN PEDIDO DEJA DE PODER OCURRIR DOS VECES.
--
-- Crear el pedido ya es idempotente (`orders.client_uuid`, 0023) y cobrarlo también (0057). Agregar
-- quedó fuera, y es el único de los tres que mueve DOS cosas a la vez: el total del pedido y el
-- inventario. Un doble tap sobre una tableta que no alcanzó a pintar la respuesta, o un corte de red
-- al confirmar, mete el renglón dos veces: se le cobra de más al cliente y se descuenta dos veces la
-- materia prima, que después no cuadra contra el conteo físico.
--
-- No lo atrapa ninguna validación existente: dos renglones idénticos en el mismo pedido son
-- perfectamente legítimos —el cliente pidió otro café— así que nada distingue el reintento de la
-- segunda orden salvo la llave.
--
-- El LOTE es la unidad, no el renglón. Quien opera manda "lo que agregué" de una vez, y un reintento
-- reenvía el mismo lote completo; partir la llave por renglón dejaría medio agregado aplicado si un
-- reintento llegara recortado. La llave que usa el front es el id de la CUENTA, la misma con la que
-- ya crea el pedido: es estable entre reintentos, se persiste, y muere cuando la cuenta se cierra.
create table order_line_batches (
  client_uuid uuid not null,
  order_id    bigint not null references orders(id) on delete cascade,
  created_at  timestamptz not null default now(),
  company_id  bigint not null default current_setting('app.company_id', true)::bigint
              references companies(id) on delete cascade,
  -- Por (empresa, llave) y no por (pedido, llave): una llave vale UNA vez por empresa. Acotarla al
  -- pedido dejaría pasar la misma llave sobre otro pedido, y ahí un reintento mal dirigido —la
  -- pantalla equivocada, un pedido que cambió bajo los pies del operador— agregaría comida a una
  -- cuenta ajena sin que nada lo notara. Es el mismo criterio que `order_payments_idem` (0057).
  --
  -- Arranca por company_id como todo índice de este esquema: RLS le pega ese predicado a cada
  -- consulta del rol de la app.
  primary key (company_id, client_uuid)
);

-- El pedido se guarda para poder distinguir el reenvío legítimo —misma llave, mismo pedido— del
-- reintento mal dirigido, que tiene que rebotar en vez de aplicarse.
create index order_line_batches_order on order_line_batches (order_id);

alter table order_line_batches enable row level security;
create policy tenant_isolation on order_line_batches
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- El grant de 0024 fue `on all tables in schema public`, que es PUNTUAL: no hay default privileges,
-- así que cada tabla creada después necesita el suyo. Sin esto todo pasa en local —dev conecta como
-- owner— y en producción el primer agregado devuelve 42501.
--
-- Sin `update` ni `delete`: una fila de esta tabla se inserta y se queda. Un grant que nadie usa es
-- superficie que alguien acaba usando.
grant select, insert on order_line_batches to gatobobah_app;

-- +goose Down
drop table if exists order_line_batches;
