-- +goose Up
-- Barreras de lo dado por perdido (decisión del dueño, 2026-10-09; revisión de la 0085).
--
-- 1. Lo perdido nunca supera el total del pedido. El tope vivía solo en el servidor; una escritura
--    que lo pasara dejaría un «debe» negativo en el pendiente de Ventas y en el cierre.
-- 2. Quien dio algo por perdido no se borra (`restrict`, explícito): el rastro de ese dinero no
--    desaparece por arrastre. Ya era el efecto del `no action` por omisión; se escribe para que se
--    vea que se eligió.
-- 3. Índice del día de lo perdido para el tile de Ventas. Empieza por company_id porque RLS agrega
--    ese predicado, y es parcial: casi ningún pedido tiene algo perdido.
alter table orders
  add constraint orders_write_off_within_total check (written_off_amount <= total);

alter table orders drop constraint orders_written_off_by_fkey;
alter table orders
  add constraint orders_written_off_by_fkey foreign key (written_off_by) references users(id) on delete restrict;

create index orders_company_written_off_day on orders (company_id, written_off_business_date)
  where written_off_at is not null;

-- +goose Down
drop index if exists orders_company_written_off_day;
alter table orders drop constraint orders_written_off_by_fkey;
alter table orders
  add constraint orders_written_off_by_fkey foreign key (written_off_by) references users(id);
alter table orders drop constraint orders_write_off_within_total;
