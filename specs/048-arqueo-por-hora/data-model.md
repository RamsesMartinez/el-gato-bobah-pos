# Data model: 048

## expense_payments
- `include_in_count boolean not null default false` + trigger: con corte fuerza `true`;
  `check (register_session_id is null or include_in_count)`.
- `paid_at timestamptz not null default clock_timestamp()` (Go la manda; el default es para la API vieja; relleno R6).
- `unique (company_id, id)` para la FK del movimiento.
- Índice parcial `(company_id, register_id, paid_at) where register_session_id is null and include_in_count`.

## register_cash_movements
- `expense_payment_id bigint null`, FK `(company_id, expense_payment_id)` → `expense_payments(company_id, id)` `on delete restrict`.
- `unique (expense_payment_id) where kind = 'salida'`.

Down: se niega si hay pendientes; quita en orden inverso.
