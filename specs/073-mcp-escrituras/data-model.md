# Data Model: Escrituras del servidor MCP

## Cambios de esquema

| Tabla | Cambio | Migración |
| --- | --- | --- |
| `api_tokens` | El check `api_tokens_scopes` admite además `write:purchases` y `write:orders`; cardinalidad de 1 a 7 | 0112 |

- **Up**: `alter table api_tokens drop constraint api_tokens_scopes, add constraint api_tokens_scopes
  check (...) ` con la lista ampliada. Tabla de pocos renglones: el bloqueo es instantáneo.
- **Down**: se niega (`raise exception`) si algún token tiene un alcance nuevo; renglón en
  `guardedDownCleanup` que quita los alcances nuevos de esos tokens en la copia de prueba. Si no,
  restaura el check de la 0111.
- Sin columnas nuevas: `TestEveryColumnIsCoveredByTheAnonymousBackup` no cambia.

## Entidades que no se guardan

### Vista previa de costo de compra (respuesta de `POST /expenses/purchase-preview`)

| Campo | Tipo | Nota |
| --- | --- | --- |
| `lines[].position` | int | Orden del renglón en la compra |
| `lines[].ingredientId` | int? | Nulo en productos de reventa |
| `lines[].name` | text | Nombre del insumo |
| `lines[].baseQty` | decimal (4) | Lo que entra, en la unidad base |
| `lines[].baseUnitCode` | text | |
| `lines[].unitCost` | decimal (6) | Costo por unidad base de esta compra |
| `costs[].ingredientId`, `name` | | Uno por insumo, aunque venga en varios renglones |
| `costs[].costBefore` / `costAfter` | decimal (6) | Costo vigente hoy / el que deja la compra completa |
| `costs[].keptByNewerPurchase` | bool | «Último precio» y llegó una compra más reciente |
| `costingMethod` | text | `last` o `weighted` |

### Requisitos de cierre (dominio, puro)

`CloseRequirements{ DrawerNeedsCount bool; Methods []{ID int; Name string} }` y
`CloseDeclarations{ DrawerCounted bool; Declared map[int]bool }` →
`MissingCloseDeclarations` devuelve los nombres que faltan, en el orden del turno.

### Resumen de cancelación (dominio, puro)

`CancelSummary(status, payments)` → si exige devolución, el total por método y si hay tarjeta (pide
folio). Lo usa la vista previa de `order_cancel`; la regla de verdad sigue en el servicio.

## Alcances (dominio)

`ScopeWritePurchases = "write:purchases"`, `ScopeWriteOrders = "write:orders"` en `AllAPIScopes`, en
el orden: read, expenses, cash, inventory, catalog, purchases, orders.
