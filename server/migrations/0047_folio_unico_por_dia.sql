-- +goose Up
-- Dos pedidos del mismo día no pueden llamarse igual: el nombre existe para distinguirlos al
-- cantarlos en cocina, y dos "Tigre" a la vez lo vuelven inútil justo cuando más se usa.
--
-- Desde que la pantalla propone el nombre al abrir la cuenta —para que el operador lo vea desde el
-- primer producto y no solo al cobrar—, la unicidad ya no sale sola de derivarla del folio
-- numérico: dos cuentas abiertas podrían proponer el mismo animal. El servicio resuelve el choque
-- agregando la vuelta ("Tigre 2"); este índice es la red que garantiza que nunca se escape.
--
-- Parcial: los pedidos anteriores a 0046 tienen folio_name nulo y no deben chocar entre sí.
create unique index orders_folio_dia
  on orders (company_id, business_date, folio_name)
  where folio_name is not null;

-- +goose Down
drop index if exists orders_folio_dia;
