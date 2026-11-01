# Data model: 050
- `expense_payments.drawer_extra numeric(10,2) not null default 0`, `check (drawer_extra >= 0)`.
- Down: se niega si hay algún `drawer_extra > 0`.
