# Implementation Plan: Cambiar la terminal de un pago ya cobrado

**Branch**: `036-cambiar-terminal-de-pago` | **Date**: 2026-10-11 | **Spec**: [spec.md](spec.md)

## Summary

Un endpoint nuevo cambia la terminal de un pago con tarjeta bajo el candado del pedido y escribe el evento `payment.terminal_changed` en la bitácora en la misma transacción. El detalle de venta lista los pagos con su terminal y, con permiso, ofrece «Cambiar terminal». El cobro deja de preseleccionar la terminal recordada del usuario, en el servidor y en la hoja. El corte de un turno cerrado avisa cuando lo esperado por terminal cambió después del arqueo. **Sin migración**: las columnas del pago (0088) y la bitácora (0089) ya existen en `develop`.

## Technical Context

**Language/Version**: Go 1.27.2 (server), TypeScript + React 19 (web)
**Primary Dependencies**: chi, pgx + sqlc, Chakra UI v3, TanStack Query
**Storage**: PostgreSQL con RLS; tablas `order_payments`, `card_terminals`, `audit_events`, `session_terminal_counts`
**Testing**: `go test` (domain unitario, integración contra Postgres bajo `appRoleStore`), vitest
**Target Platform**: API en VM; POS en tabletas de 7–10" (1024×600)
**Project Type**: web (server + web)
**Performance Goals**: el cambio es una transacción corta sobre un pedido; nada que medir aparte
**Constraints**: se sube con el negocio operando; sin migración; la API vieja y la nueva conviven sin problema porque no cambia el esquema
**Scale/Scope**: 1 endpoint, 1 permiso, 1 acción de bitácora, 2 pantallas tocadas, 1 aviso en el corte

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I. Layering | Reglas del cambio en `domain.ValidateTerminalChange`; orquestación y transacción en `app.OrdersService.ChangePaymentTerminal`; handler fino; SQL nuevo vía sqlc |
| II. Errores | Sentinels `domain.ErrValidation` / `ErrNotFound` / `ErrConflict` existentes; mapeo solo en `httpapi.Error` |
| III. Dinero | No mueve montos; los totales por terminal salen de la misma consulta que el corte (`TerminalCollectedForSession`) |
| IV. Test-first | Tabla de bordes en `domain` primero; integración bajo `appRoleStore` con `inTheThreeCases` para la lectura nueva; regresión del caso real (pago de $250 Terminal A → Terminal B) |
| V. Seguridad | `RequirePermission(PermPaymentsChangeTerminal)` en el router; terminal destino validada contra la sucursal de la caja del pago bajo RLS (la FK salta RLS); motivo acotado |
| VI. YAGNI | Sin tabla de historial propia: la bitácora es el historial. Sin cambiar método ni monto |
| VIII. Puertas | Permiso nuevo, no nombre de rol. El pago guarda snapshot del nombre de la terminal; el evento guarda antes y después |
| Producto: operar sin decidir | Contradicción aparente: quitar la terminal por omisión hace decidir al cobrar. Se acepta porque la terminal es un hecho físico (dónde se pasó la tarjeta) que la configuración no puede saber; con una sola terminal activa sigue llegando elegida |
| Producto: subir operando | Sin migración; un cliente viejo en caché sigue mandando `terminalId` y el servidor lo acepta |

**Resultado**: pasa, sin excepciones.

## Pantallas y maquetas

| Pantalla | Cambio | Componentes |
|---|---|---|
| Detalle de venta (Ventas) | Sección «Pagos»: un renglón por pago con método, monto y terminal; botón «Cambiar terminal» (44 px) solo con permiso y solo en tarjeta | `DialogRoot`; abre la hoja |
| Hoja «Cambiar terminal» | Nueva hoja inferior: terminal actual, botones de 52 px con las activas de la sucursal (la actual deshabilitada), motivo, Confirmar | Patrón de `DevolucionSheet` (motivo obligatorio) |
| Hoja de cobro | El selector de terminal llega vacío con 2+ terminales; Cobrar deshabilitado hasta elegir | `TerminalDelCobro` sin `porOmision` |
| Corte cerrado | Aviso de una línea por terminal cuyo esperado cambió tras el arqueo, con la diferencia recalculada | `ConteoDeTerminales` |

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Detalle de venta + hoja «Cambiar terminal» | https://claude.ai/artifact/MwyB4MmVyWvBBGZsrya7TK | A · Hoja inferior | Mismo patrón que devolver un pago y no empuja la lista de pagos |

## Project Structure

### Documentation (this feature)

```text
specs/036-cambiar-terminal-de-pago/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/change-payment-terminal.md
└── tasks.md            # /speckit-tasks
```

### Source Code

```text
server/
├── internal/domain/   permission.go (PermPaymentsChangeTerminal), audit.go (payment.terminal_changed),
│                      terminal_change.go (+_test)
├── queries/           terminals.sql: GetPaymentForTerminalChange, ActiveTerminalsOfSessionBranch
├── internal/app/      orders.go (ChangePaymentTerminal; PaymentView con terminal),
│                      terminals.go (resolveTerminal sin preferencia del usuario), backoffice.go (corte cerrado)
├── internal/httpapi/  router.go, handlers_orders.go
└── internal/integration/ cambiar_terminal_test.go
web/src/
├── features/sales/    SaleDetailDialog.tsx (+test), CambiarTerminalSheet.tsx (+test)
├── shared/            CobrarSheet.tsx, TerminalDelCobro.tsx, terminalEfectiva.ts (+tests)
├── features/backoffice/ ConteoDeTerminales.tsx (+test)
├── api/pos.ts, app/permissions.ts
```

## Complexity Tracking

Sin violaciones que justificar.

## Estimado

| Bloque | Estimado |
|---|---|
| Dominio, permiso, acción de bitácora y tests | 1.5 h |
| Servicio, consultas, endpoint e integración (incluye regresión del caso real y los tres casos de RLS) | 3 h |
| Quitar terminal por omisión (servidor y hoja) con tests | 1.5 h |
| Detalle de venta + hoja «Cambiar terminal» con tests | 3 h |
| Aviso en el corte cerrado | 1 h |
| Revisores, `make ci-local`, capturas a 1024×600 | 2 h |
| **Total** | **~12 h (1.5 días)**, estimado |

## Ajustes tras la revisión de tableta

| # | Decisión |
|---|---|
| T1 | En el cobro, con 4 terminales activas o menos, botones de 52 px en línea, ninguno marcado al abrir: 1 toque extra por cobro con tarjeta en vez de 2 (`Picker` solo con más de 4). Con una sola, llega elegida |
| T2 | El motivo llega prellenado «Se cobró en otra terminal», editable; sin `autoFocus`. La hoja tiene `maxH` en dvh con scroll interno para que Confirmar siga alcanzable con el teclado abierto. Caso común: 3 toques, sin teclado |
| T3 | La sección «Pagos» reemplaza el renglón «Medio de pago» del detalle: un renglón de 44 px por pago («Débito · Terminal B · $250» y el botón al final). La hoja se monta dentro de `DialogContent` y abre en el render siguiente, como `DevolucionSheet` |
| T4 | Sin marca «cambiada» en el renglón: el rastro es la pestaña Historial, con la etiqueta en español de `payment.terminal_changed` |
| T5 | El aviso de turno cerrado vive en la vista del corte cerrado (no en `ConteoDeTerminales` de captura): «Terminal B: el esperado cambió $250.00 después del arqueo. Diferencia ahora: −$250.00» |
| T6 | Pago sin terminal: la hoja dice «Sin terminal» y ofrece todas las activas |
| T7 | Cuenta dividida: la terminal elegida se limpia tras cada cobro, así el segundo pago con tarjeta también se elige |
| T8 | Se borran la consulta `card-terminal-default`, `porOmision` y las rutas de la terminal por omisión del usuario si no tienen otro consumidor |

## Ajustes tras la revisión de base de datos

| # | Decisión |
|---|---|
| D1 | El request lleva `fromTerminalId` (nulo para «Sin terminal»). Si no coincide con la terminal leída bajo el candado del pedido: 409 «El pago ya cambió». Test con dos transacciones |
| D2 | El aviso del corte cerrado se calcula sobre la UNIÓN de lo cobrado hoy por terminal (`TerminalCollectedForSession`) y las filas guardadas del arqueo (ausente = $0). Cubre la terminal que no cobró nada en el turno (el caso real). Corregido al implementar: con arqueo automático no hay nada firmado por terminal, así que no hay aviso; el corte de método ya refleja lo de hoy |
| D3 | Las devoluciones con tarjeta se atribuyen a la terminal del último pago con tarjeta del pedido, así que cambiar ese pago mueve también la devolución, que puede estar en otro turno. Como D2 compara al pintar cada corte cerrado, ese turno también avisa. Test: cambiar el pago de un pedido con devolución en otro turno. Límite que ya existía y queda declarado: si la terminal nueva no cobró nada en el turno de la devolución, la devolución no aparece en la lista por terminal |
| D4 | `GetPaymentForTerminalChange` filtra `op.id = $1 and op.order_id = $2` y se lee después de `GetOrderForUpdate`. Test con un pago de otro pedido → 404 |
| D5 | No se crea `ActiveTerminalsOfSessionBranch`: se resuelve la caja de la sesión del pago y se reutiliza `ActiveTerminalsOfRegisterBranch`, que es la misma regla del cobro. La lectura nueva lleva `inTheThreeCases` |
| D6 | El evento guarda en `before` y `after` la terminal (id y nombre), el monto, el turno y la sucursal. Test: si la bitácora falla, el cambio se revierte |
| D7 | El nombre de la terminal guardado en el pago sale de la fila leída bajo el candado, nunca del cliente |

## Estimado revisado

~14 h (≈2 días de trabajo), estimado: +2 h sobre el anterior por D1–D3 y por los botones del cobro (T1).
