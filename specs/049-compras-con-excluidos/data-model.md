# Data model: 049
- `expense_items.excluded boolean not null default false`.
- `expenses.document_total numeric(10,2) null` (`check (document_total is null or document_total >= 0)`).
- Down: se niega si hay renglones excluidos (volverían a contar en silencio) o algún `document_total` (no se reconstruye).
- `set local lock_timeout = '3s'`; el check de `document_total` `not valid` + `validate`; mismo tipo que `expenses.amount`.
