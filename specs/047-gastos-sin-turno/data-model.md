# Data model: 047

## cash_registers
- `counts_cash boolean not null default true` — «Lleva arqueo». Check `not is_primary or counts_cash`.
- `unique (id, company_id)` para la FK compuesta.

## expense_payments
- `register_id bigint null`, `foreign key (register_id, company_id) references cash_registers(id,
  company_id) on delete restrict` (creada `not valid` + `validate`).
- Trigger `before insert or update of register_session_id`: con sesión, `register_id` se toma de
  ella (R4). Los existentes se rellenan desde su sesión en la migración.
- Nulo en filas viejas sin sesión = «no aplica / no se sabe». Todo pago nuevo con caja lo lleva.
- El Down de 0099 se niega si existe un pago con `register_id` y sin sesión (R5).
