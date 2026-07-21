-- +goose Up
-- Cada transacción lleva su moneda (ISO-4217). El sistema es currency-aware desde ya;
-- por ahora todo opera en MXN. NO hay motor FX ni precios por moneda todavía (feature aparte):
-- esto solo hace correcto y extensible el modelo de datos.
alter table orders            add column currency char(3) not null default 'MXN';
alter table register_sessions add column currency char(3) not null default 'MXN';
alter table expenses          add column currency char(3) not null default 'MXN';

-- +goose Down
alter table orders            drop column currency;
alter table register_sessions drop column currency;
alter table expenses          drop column currency;
