# Data model: Fondo de caja fijo

Migración aditiva. **Número: el libre siguiente al fusionar** (ver research R4).

## cash_registers (existe)

| Columna | Tipo | Regla |
|---|---|---|
| `fixed_float` | `numeric(10,2)` null | `check (fixed_float is null or fixed_float >= 0)`; nulo = sin configurar |

## register_sessions (existe)

| Columna | Tipo | Regla |
|---|---|---|
| `float_left` | ya existe (0088) | lo que se dejó; sin cambio |
| `fixed_float_at_close` | `numeric(10,2)` null | snapshot del fijo al cerrar; nulo en cierres previos y en cajas sin fijo |

Derivado al leer: `floatOverridden = fixed_float_at_close is not null and float_left <> fixed_float_at_close`.

## register_cash_movements (existe)

- `kind` admite además `'retiro'`.
- `register_cash_movements_tip_shape` se reescribe con sus TRES cláusulas de 0087: `(kind in ('propina','retiro')) = (recipient_user_id is not null)`, `(recipient_user_id is null) = (recipient_name is null)` y el largo de `recipient_name` (1–120). `recipient_name` es el snapshot del nombre (edge «dado de baja»).
- `kind_check` y `tip_shape` se agregan `NOT VALID` y luego `VALIDATE` en la misma migración: tabla chica, pero escrita por la caja viva.
- `create unique index register_cash_movements_one_withdrawal on register_cash_movements (session_id) where kind = 'retiro';`
- FK compuesta a `users` ya existe (0087).
- El índice único va sin `company_id` a propósito: `session_id` ya es de un solo tenant (mismo caso que `register_cash_movements_one_reverse`, 0088).
- `amount > 0` (el check vigente de monto aplica).

## Reglas (domain, puras)

- `domain.Withdrawal(counted, floatLeft) (decimal, error)`: `Round2(counted − floatLeft)`; error si `floatLeft < 0`, `> counted` o inválido.
- `domain.ResolveFloatLeft(sent *decimal, fixed *decimal, counted decimal) (decimal, error)`: enviado → enviado (400 si > contado); si no, `min(fijo, contado)`; si no, lo contado.

## Down

Aborta si hay filas `retiro`, `fixed_float` o `fixed_float_at_close` no nulos (como 0087/0088). Si no: restaura `kind_check` a `('entrada','salida','propina','reverso')` y `tip_shape` a su forma de 0087, borra el índice y las dos columnas.

## Consultas y código afectados

- `NetCashMovements`: resta solo `salida` y `propina`; suma `entrada` y `reverso`; `retiro` no cuenta.
- Nuevas: `SetRegisterFixedFloat`, `InsertWithdrawal`; `SetFloatLeft` escribe también `fixed_float_at_close`.
- `corteBreakdown` (Go): `case CashRetiro` fuera de `net` y de salidas; renglón propio.
- `ListCashMovements`: agrega `recipient_name`. `ListAllCashRegisters` y `GetCashRegister`: agregan `fixed_float`. `ListCashRegisters` (la del cajero) NO.
- `traduceConflictoDeCaja`: mapea `register_cash_movements_one_withdrawal` a `ErrConflict`.
- Lectura del corte/turno: devuelve `floatLeft`, `fixedFloatAtClose`, retiro (monto, quién); el turno abierto devuelve `fixedFloat`.
