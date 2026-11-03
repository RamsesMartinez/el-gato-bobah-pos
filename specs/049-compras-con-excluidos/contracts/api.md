# Contratos: 049
- `POST /expenses`: cada renglón acepta `excluded?: boolean` (el viejo `personal` se trata igual); el gasto acepta `documentTotal?: string` (dinero: se valida con `ValidMoney`, 400 si no).
- `PATCH /expenses/{id}/items/{itemId}` `{ excluded: boolean }`: `PermInventoryReceive`; 400 si rompe la regla de la 038; 409 si otro cambio tiene tomados los insumos (candado ocupado).
- Detalle: cada renglón trae `excluded`; el gasto trae `documentTotal`.
- `POST /cash-registers/{id}/sells`: `PermCashConfigure`; 409 `REGISTERS_MUST_BE_CLOSED`; 400 otra sucursal o caja inactiva; 200 sin cambios si ya vende. Las cajas traen `isPrimary` como hoy.
