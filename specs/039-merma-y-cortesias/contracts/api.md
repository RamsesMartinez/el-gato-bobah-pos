# Contrato HTTP: Merma

Todo bajo `/api/v1`, con `RequirePermission`, fuera de grupos con rol. Arreglos siempre `[]`, nunca
`null`. Errores: 400 validación, 404 de otra empresa o inexistente, 409 transición inválida o
insumo ocupado («vuelve a intentar»), 409 `BRANCH_AMBIGUOUS`.

## Motivos

| Método | Ruta | Permiso | Cuerpo / respuesta |
|---|---|---|---|
| GET | `/waste/reasons` | `inventory.write_off` | `[{ id, name, description, isStaffMeal, uses }]` activos, más usados primero |
| POST | `/waste/reasons` | `inventory.write_off` | `{ name, description }` → el creado, o 409 `{ existingId }` si el nombre ya existe |
| PUT | `/waste/reasons/{id}` | `inventory.configure` | `{ name, description, isStaffMeal, archived }` |
| POST | `/waste/reasons/{id}/merge` | `inventory.configure` | `{ intoId }`: los registros pasan a `intoId`; el motivo queda archivado y `mergedIntoId` |

## Registros

| Método | Ruta | Permiso | Cuerpo / respuesta |
|---|---|---|---|
| POST | `/waste` | `inventory.write_off` | `{ reasonId, ingredientId? , productId?, qty, unitId?, presentationId?, note? }` → `{ id, status: "applied" \| "pending", totalCost }` |
| GET | `/waste/today` | `inventory.write_off` | Las del día de la sucursal, con `canUndo` |
| POST | `/waste/{id}/undo` | `inventory.write_off` (el que registró) o `inventory.configure` | 204 |
| GET | `/waste/pending` | `inventory.write_off_approve` | `[{ id, item, qty, totalCost, reason, recordedBy, recordedAt }]`; la franja lo pide al cargar y con el aviso en vivo |
| GET | `/waste?from=&to=&reasonId=` | `inventory.health_view` | Registros del periodo para la pestaña Merma, con estado; `reasonId` opcional |
| POST | `/waste/{id}/approve` | `inventory.write_off_approve` | 204; 409 si ya se resolvió o la cubrió un conteo |
| POST | `/waste/{id}/reject` | `inventory.write_off_approve` | 204 |

## Reportes

| Método | Ruta | Permiso | Respuesta |
|---|---|---|---|
| GET | `/stock/counts/{id}/loss` | `inventory.health_view` | `{ firstCount: bool, costAsOf, items: [{ ingredientId, name, unit, registered, unexplained, lost, surplus, unitCost? }], totals }` |
| GET | `/waste/by-reason?from=&to=` | `inventory.health_view` | `{ reasons: [{ id, name, isStaffMeal, totalCost, records }] }` (las fichas); fechas mal formadas → 400 |

## Configuración

`PUT /settings/inventory` (038) acepta además `wasteApprovalThreshold: string | null`.
