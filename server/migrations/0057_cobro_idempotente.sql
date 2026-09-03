-- +goose Up
-- Una llave de idempotencia por PAGO, para que dividir la cuenta no se coma el dinero de un comensal.
--
-- Con un solo pago, el doble tap lo atrapa la validación: el segundo intento no cabe en lo que falta
-- y rebota. Al DIVIDIR desaparece esa red — las dos mitades de $250 son indistinguibles entre sí, así
-- que reenviar la primera pasa todas las validaciones y deja el pedido saldado. Medido contra
-- Postgres real: pedido de $500, dos llamadas idénticas → pagado $500, propina duplicada, pedido
-- cerrado, y la tarjeta del segundo comensal nunca se cobra. El arqueo cuadra contra los pagos que sí
-- se registraron; lo que se pierde es dinero que jamás entró, sin un renglón que lo nombre.
--
-- Es el mismo mecanismo que ya protege a la creación del pedido (orders.client_uuid), aplicado al
-- momento en que ahora se cobra.
alter table order_payments add column if not exists client_uuid uuid;

-- Por (company_id, client_uuid), espejo exacto de `orders_company_client_uuid_key` (0023): una llave
-- vale una vez por empresa, no una vez por pedido. Acotarla al pedido dejaría pasar la misma llave
-- sobre OTRO pedido, y ahí un reintento mal dirigido —la pantalla equivocada, un pedido que cambió
-- bajo los pies del operador— se registraría como un cobro nuevo sin que nada lo notara.
--
-- Nullable y parcial: los pagos que ya están en producción no tienen llave y no se les inventa una.
-- Sin el `where`, el índice cargaría con todo el histórico sin servir para nada (dos NULL nunca
-- chocan en un índice único).
create unique index if not exists order_payments_idem
  on order_payments (company_id, client_uuid)
  where client_uuid is not null;

-- +goose Down
drop index if exists order_payments_idem;
alter table order_payments drop column if exists client_uuid;
