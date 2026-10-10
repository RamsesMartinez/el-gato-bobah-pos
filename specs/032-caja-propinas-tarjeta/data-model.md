# Data Model: Caja, propinas y cobro con tarjeta

Toda tabla nueva: `company_id` con default `nullif(current_setting('app.company_id', true), '')::bigint`, RLS con `nullif`, grants a `gatobobah_app`, índices que empiezan por `company_id`. La vigila `TestEveryCompanyTableIsIsolated`.

## Cambios a tablas existentes

### register_cash_movements
| Columna | Tipo | Regla |
| --- | --- | --- |
| kind | text | check amplía a `entrada, salida, propina, reverso` |
| concept_id | bigint → cash_concepts | obligatorio para `salida` nueva (check con fecha de corte; las históricas sin concepto salen en «sin concepto») |
| recipient_user_id | bigint → users | obligatorio si `propina` |
| recipient_name | text | snapshot, obligatorio si `propina` |
| tip_source_kind | payment_kind | obligatorio si `propina` |
| reverses_id | bigint → register_cash_movements, `on delete restrict` | obligatorio si `reverso`; índice único parcial: a lo más un reverso por movimiento |
| document_date | date | opcional, solo contabilidad |

### register_sessions
| Columna | Regla |
| --- | --- |
| tips_carried_over numeric(10,2) ≥ 0 | propina que se quedó en caja al cerrar |
| opening_reason text / opening_reason_note text | obligatorio si la apertura difiere del cierre anterior de la misma caja |
| card_count_mode text | copia del modo de la sucursal al abrir (EB-33) |

### order_payments
`card_terminal_id bigint → card_terminals on delete restrict`, `card_terminal_name text` (snapshot), `card_kind text check in ('debito','credito')`.

### order_refunds
`card_refund_folio text`, `card_refund_captured_by bigint → users`; trigger: si el pago original es tarjeta, folio no vacío tras `btrim`.

### branches
`card_count_mode text not null default 'automatico' check in ('automatico','por_terminal')`.

### user_preferences
`default_card_terminal_id bigint → card_terminals on delete set null`.

## Tablas nuevas

- **cash_concepts**: id, name, name_key (lower+btrim, único por empresa entre activos), expense_category_id?, supplier_id?, archived_at?, merged_into_id? → cash_concepts.
- **tip_payout_sources**: movement_id → register_cash_movements, order_payment_id → order_payments, amount. PK (movement_id, order_payment_id).
- **card_terminals**: id, branch_id → branches, name (único por sucursal entre activas), archived_at?.
- **session_terminal_counts**: session_id, card_terminal_id, declared, created_by. PK (session_id, card_terminal_id).
- **daily_summary_sends**: business_date, status (`pendiente/enviado/fallido`), attempts, last_error, sent_at. Único (company_id, business_date).

## Reglas de cálculo (domain)

- `pendiente(método) = cobradas − devueltas − entregadas (+ heredado si efectivo)`; nunca < 0.
- `efectivo_esperado = fondo + ventas_efectivo + propinas_efectivo − propinas_entregadas(todas) + entradas − salidas ± reversos − devoluciones_cajón`.
- `efectivo_negocio = contado − propinas_pendientes_en_cajón`.
- Gastos del periodo excluyen `propina` y traspasos; se muestran aparte.

## Siembra
- Cada sucursal existente recibe una terminal por omisión; la sucursal de la empresa `gatobobah` (por slug) queda en `por_terminal`.
- Conceptos por omisión por empresa: basura, hielo, vigilancia, insumos.
