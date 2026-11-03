# Data model: 050
- `expense_payments.drawer_extra numeric(10,2) not null default 0`, `check (drawer_extra >= 0)`.
- Trigger `before insert` en `register_cash_movements` (en la 0102): con `expense_payment_id` y `kind = 'salida'`, `amount := ep.amount + ep.drawer_extra`.
- Down: se niega si hay algún `drawer_extra > 0`.
