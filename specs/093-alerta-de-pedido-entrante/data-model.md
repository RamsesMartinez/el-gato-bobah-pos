# Data Model: Alerta continua de pedido entrante

## Esquema: una migración, solo filas (0119)

Sin tablas ni columnas nuevas. Todo lo que el modelo necesita ya existe en
`platform_incoming_orders` (`state`, `decided_by`, `settled_at`, `deny_reason`, `decide_before`).

| Cambio | Detalle |
|---|---|
| `permissions` | `(39, 'orders.accept_incoming', 'orders')` |
| `system_role_permissions` | `(1..4, 39)`: herencia a los cuatro sugeridos |
| `role_permission_overrides` | `(company_id, role_id, 39, true)` para cada rol con `system_role_id is null` existente; `company_id` explícito (el default sale de la sesión), `on conflict do nothing` |
| Down | Se niega si existe un override de 39 con `granted = false` (una empresa se lo quitó a un rol); si no, borra overrides, `system_role_permissions` y `permissions` de 39, en ese orden. Renglón `119` en `guardedDownCleanup` |

¿Se puede agregar después al mismo costo? Sí: son filas de catálogo. No cierra ninguna puerta del
principio VIII.

## Tipos de dominio (`server/internal/domain/incoming.go`, puro)

### IncomingOrder

| Campo | Tipo | Nota |
|---|---|---|
| `Source` | `IncomingSource` (`"platform"`) | Lista cerrada; `"web"` llega con el #328 |
| `ID` | int64 | Id dentro de su origen; la llave pública es `(source, id)` |
| `SourceName` | string | Lo que se pinta: «Uber Eats», después «Web» |
| `DisplayID` | string | Folio que ve el cliente |
| `CustomerName` | string | Vacío si el origen no lo da |
| `CustomerPhone` | string | Vacío hoy (la plataforma no lo da); lo llena el #328 |
| `Address` | string | Ídem |
| `ServiceType` | string | `para_llevar` / `domicilio` |
| `Total` | decimal | |
| `PlacedAt` | *time | |
| `DecideBefore` | *time | Nulo si el origen no tiene plazo |
| `Lines` | `[]IncomingLine` | Nunca nil (sale `[]`) |

`IncomingLine`: `Name`, `Quantity`, `UnitPrice`, `Matched`, `Options []IncomingLine` (nunca nil).

### Regla de pendiente

`IsPending(state, decideBefore, now)`: `state == pendiente` y (`decideBefore` nulo o
`decideBefore > now`). Se aplica en la consulta de lista y se verifica otra vez antes de llamar a la
plataforma al aceptar o rechazar.

### Orden

`SortByUrgency`: con plazo primero, el más cercano arriba; sin plazo después, el más viejo arriba. En SQL: `order by decide_before nulls last, placed_at, id`.

### IncomingRejectReason

| Código propio | Texto en pantalla | Código de la plataforma |
|---|---|---|
| `closed` | Ya cerramos | `STORE_CLOSED` |
| `out_of_stock` | Se acabó un producto | `ITEM_AVAILABILITY` |
| `too_busy` | Estamos saturados | `CAPACITY` |
| `address` | No llegamos a esa dirección | `ADDRESS` |
| `other` | Otro motivo | `OTHER` |

Un código desconocido es `ErrValidation`, nunca cae a `other` (principio V).

## Transiciones de un pedido de plataforma

```text
pendiente ──aceptar (plataforma confirma)──▶ aceptado   (crea pedido del POS, cobro, comanda)
pendiente ──rechazar (plataforma confirma)─▶ rechazado  (deny_reason = código propio)
pendiente ──webhook de cancelación─────────▶ cancelado
pendiente ──pasa decide_before─────────────▶ (sigue en la fila como pendiente, ya no se lista ni se decide)
```

Toda transición publica `incoming.changed` a la empresa.
