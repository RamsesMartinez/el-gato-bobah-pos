# Contratos: 047

## POST /expenses, POST /expenses/{id}/pay
Cada pago acepta `outsideCount?: boolean`.
- Efectivo + caja con arqueo + (sin turno abierto, o `paidOn` ≠ día del turno) y sin `outsideCount`
  → **409** `{ code: "EXPENSE_OUTSIDE_COUNT" }`.
- Con `outsideCount: true` → se guarda con `registerId` y sin corte.
- Efectivo + caja sin arqueo → se guarda sin corte, sin confirmación.

## GET /cash-registers, GET /cash-registers/all
Cada caja trae `countsCash: boolean`.

## PATCH /cash-registers/{id}
Acepta `countsCash?: boolean` (exige el permiso de configurar caja; sin él → **403**). Con turno
abierto → **409** `REGISTER_HAS_OPEN_SHIFT`; a la principal → **409** `PRIMARY_REGISTER_COUNTS`.

## Abrir turno (POST bajo /cash-sessions)
Caja con `countsCash = false` → **409** `{ code: "REGISTER_NOT_COUNTED" }`.
