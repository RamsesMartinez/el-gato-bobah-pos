-- +goose Up
-- Devolución de una orden YA entregada. `entregada` sigue siendo terminal para el flujo
-- normal (no se reutiliza cancel); una devolución es un evento aparte que la marca como
-- 'reembolsada' y la trata como pérdida (sin restock: la mercancía ya se consumió).
-- ADD VALUE es seguro dentro de la tx de goose (PG12+): no se USA el valor en esta misma
-- migración; el ALTER TABLE solo agrega columnas.
alter type order_status add value 'reembolsada';

alter table orders
  add column refunded_at    timestamptz,
  add column refunded_by    bigint references users(id),
  add column refund_reason  text,
  -- NOT NULL DEFAULT 0 (no nullable) para que sqlc lo mapee a decimal.Decimal exacto en vez
  -- de pgtype.Numeric; el estado 'reembolsada' —no el monto— distingue si hubo devolución.
  add column refund_amount  numeric(10,2) not null default 0;

-- +goose Down
alter table orders
  drop column if exists refund_amount,
  drop column if exists refund_reason,
  drop column if exists refunded_by,
  drop column if exists refunded_at;
-- Nota: Postgres no soporta quitar un valor de enum, así que 'reembolsada' persiste en
-- order_status tras el Down (inofensivo: ninguna fila lo usará).
