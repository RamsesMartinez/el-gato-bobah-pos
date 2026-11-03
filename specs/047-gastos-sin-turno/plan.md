# Implementation Plan: Gastos en efectivo sin turno abierto

**Branch**: `047-gastos-sin-turno` | **Date**: 2026-10-31 | **Spec**: [spec.md](spec.md)

## Summary

Tres cambios chicos y un ajuste de caja:

1. **Fecha del negocio en el gasto**: el formulario propone fechas con `hora.diaDelNegocio(new Date())`
   en vez de `toISOString()`; el servidor, cuando una fecha no viene, usa `domain.BusinessDate` en la
   zona del negocio en vez de `now()` en UTC.
2. **«Lleva arqueo» por caja** (`cash_registers.counts_cash`, default `true`): una columna, un
   interruptor en la tabla de cajas que ya existe, y dos guardas (no abrir turno sin arqueo; no quitar
   el arqueo con turno abierto ni a la principal).
3. **El pago en efectivo ya no exige turno**: `expense_payments.register_id` guarda la caja aunque
   no haya corte. El pago entra al corte solo si la caja lleva arqueo, tiene turno abierto y la fecha
   del pago es el día de negocio de ese turno. Si no, y la caja lleva arqueo, el servidor exige la
   confirmación explícita `outsideCount` (409 `EXPENSE_OUTSIDE_COUNT` sin ella), que la pantalla pide
   con una hoja de confirmación ya existente (`ConfirmSheet`).

## Technical Context

**Language/Version**: Go 1.27 (server), TypeScript + React 19 (web)
**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query
**Storage**: PostgreSQL con RLS por empresa
**Testing**: `go test` (dominio + integración bajo `appRoleStore`), vitest
**Target Platform**: tabletas 1024×600
**Constraints**: se sube con el negocio operando; migración aditiva; sin pantallas nuevas

## Constitution Check

| Principio | Cumple | Cómo |
|---|---|---|
| I. Capas | Sí | La regla «¿entra al corte?» vive en `domain.ExpensePaymentTarget` (puro); `app.validatePayments` la aplica; el handler solo decodifica `outsideCount`. |
| II. Errores | Sí | Sentinels nuevos `ErrExpenseOutsideCount` (409 con código) y `ErrRegisterNotCounted` (409); mapeo solo en `httpapi.Error`. |
| III. Dinero | Sí | No cambia el cálculo; un pago fuera de corte nunca entra a un esperado (lo prueba un test que nombra el concepto). |
| IV. Test-first | Sí | Tabla de `ExpensePaymentTarget` primero; integración bajo `appRoleStore` con `inTheThreeCases` para la columna nueva; migración con su test. |
| V. Seguridad | Sí | La confirmación la exige el servidor, no la pantalla. Fechas futuras siguen rechazadas. Mismos roles que hoy. |
| VI. YAGNI | Sí | Sin casilla por pago, sin cortes por hora, sin red de seguridad: quedan en #111. |
| VII. Comentarios / idioma | Sí | Identificadores en inglés (`counts_cash`, `outsideCount`), textos en español. |
| VIII. Puertas | Sí | ¿Se puede agregar después al mismo costo? La casilla «incluir en el arqueo» y los cortes por hora sí: `register_id` + `paid_on` ya guardan de qué caja y qué día salió el dinero, que es el hecho irrecuperable. No se cierra ninguna puerta. |
| Subidas sin ventana | Sí | Migración aditiva (dos columnas con default/nulas); la API vieja ignora `register_id` y `counts_cash`. |

## Capas

| Pieza | Capa |
|---|---|
| Migración `0099_expense_payment_register.sql` | `server/migrations` |
| `ExpensePaymentTarget`, sentinels | `server/internal/domain` |
| `validatePayments`, `parseDayOrToday`, `OpenSession`, `UpdateCashRegister` | `server/internal/app` |
| `outsideCount`, `countsCash` en el JSON | `server/internal/httpapi` |
| Consultas de cajas y pagos | `server/queries` (+ `make sqlc`) |
| Fechas, selector de caja, hoja de aviso | `web/src/features/backoffice/ExpenseDialog.tsx` |
| Interruptor «Lleva arqueo» | `web/src/features/backoffice/CashPage.tsx` (tabla de cajas existente) |

## Pantallas (sin pantallas nuevas)

1. **Nuevo gasto → pago en efectivo**: el selector «Caja» lista todas las cajas activas (antes solo
   las abiertas). Desaparece el texto «Abre una caja para pagar en efectivo». Nada más cambia.
2. **Al guardar**, si el servidor responde `EXPENSE_OUTSIDE_COUNT`, se abre la `ConfirmSheet`
   existente con el texto de R13. Confirmar reenvía con `outsideCount: true`.
3. **Caja → Cajas** (tabla existente): una columna «Lleva arqueo» con el mismo `Switch` de «Activa»,
   deshabilitado en la principal.

No hay maqueta: no se agrega pantalla ni cambia la disposición.

## Bordes

- B1: el turno se cierra entre que la pantalla carga y se guarda → el servidor decide con su lectura
  bajo el candado compartido existente; sin `outsideCount` responde 409 y la pantalla pregunta.
- B2: pago con fecha distinta del día del turno abierto → fuera de corte (con confirmación).
- B3: caja sin arqueo → nunca corte, nunca confirmación.
- B4: quitar arqueo con turno abierto o a la principal → 409.
- B5: abrir turno en caja sin arqueo → 409 `REGISTER_NOT_COUNTED`.
- B6: agregar un pago a un gasto existente (`/pay`) pasa por el mismo `validatePayments`.
- B7: un pago fuera de corte no aparece en `ListExpensesBySession` ni en el esperado (filtran por
  `register_session_id`, que queda nulo).
- B8: `register_id` de otra empresa → la FK no cruza empresas: se valida leyendo la caja con
  `GetCashRegister` bajo RLS antes de insertar.

## Ajustes tras la revisión de arquitectura (2026-10-31)

**Base de datos**
- R1. `UpdateCashRegister` pasa a `counts_cash = coalesce(sqlc.narg('counts_cash'), counts_cash)`:
  renombrar una caja no le quita el arqueo. El JSON trae `countsCash` como puntero.
- R2. Las guardas B4 (quitar arqueo con turno abierto) y B5 (abrir turno sin arqueo) leen la caja con
  `select … for update` en la misma transacción que escribe: sin eso, un PATCH y una apertura
  simultáneos dejan un turno abierto en una caja sin arqueo. Test de esa carrera con dos conexiones.
- R3. FK compuesta: `unique (id, company_id)` en `cash_registers` y
  `foreign key (register_id, company_id) references cash_registers(id, company_id) on delete restrict`,
  creada `not valid` y luego `validate constraint`. Un pago es un hecho contable: nunca `cascade`.
- R4. Trigger `before insert or update of register_session_id` en `expense_payments`: si hay sesión,
  `register_id` se toma de ella (y la API vieja, que no manda `register_id`, queda coherente durante
  el doble despliegue). Relleno de los existentes en un solo `update`, y un `do $$` que aborta si
  queda un pago con sesión y sin caja.
- R5. El Down se niega si existe un pago con `register_id` y sin sesión (efectivo fuera de corte):
  tras el primer pago así, el rollback es restaurar el respaldo. Dicho en el encabezado.
- R6. `register_id` nulo en filas viejas sin sesión significa «no aplica / no se sabe». Puerta para
  #111: índice parcial `(company_id, register_id, paid_on) where register_session_id is null`, no se
  construye hoy.
- R7. Test que nombra el concepto: `TestUnGastoFueraDeCorteNoEntraANingunCorte` sobre
  `ListExpensesBySession` (nombre real; B7 decía `ExpensesForSession`) y el esperado de efectivo.
- R8. Test de FK cruzada bajo `appRoleStore` con `inTheThreeCases`.
- Grants: heredados de 0026 y 0029; no hace falta otro. Número de migración: confirmar contra
  `develop` al fusionar.

**Pantalla**
- R9. Las cuatro referencias a `openRegisters` en `ExpenseDialog.tsx` (renglón de pago, chip
  «Efectivo» del pie, autoselección y «¿De qué caja sale?») pasan a la lista de cajas activas.
- R10. Las dos mutaciones (gasto nuevo y `/pay`) reconocen el código `EXPENSE_OUTSIDE_COUNT` en su
  `onError`, abren la `ConfirmSheet` y reenvían con `outsideCount: true`.
- R11. Efectivo sigue exigiendo caja: «Caja (obligatoria)» se queda. «Sin cajas abiertas.» pasa a
  «Sin cajas.» (solo ocurre si no hay ninguna).
- R12. El selector agrega un sufijo corto al nombre: «· sin turno» (con arqueo y cerrada) o
  «· sin arqueo». Sin altura extra.
- R13. Texto de la hoja: título «{Caja} no tiene turno abierto» (o «El pago es de otro día»), cuerpo
  «Este gasto no entrará a ningún corte.», botones «Registrar sin incluir en el arqueo» / «Cancelar».
- R14. En Cajas, el 409 al quitar el arqueo dice «Cierra el turno de esta caja primero.» y el de la
  principal «La caja principal siempre lleva arqueo.». El diálogo «Editar caja» no cambia: el
  servidor conserva el valor si no viene.

## Ajustes tras /speckit-analyze (2026-11-01)

- R15. Cambiar `countsCash` exige `domain.PermCashConfigure` (lo tienen admin y gerente, igual que
  hoy): el handler de `PATCH /cash-registers/{id}` lo revisa con `h.permissions` solo cuando el campo
  viene. El resto del PATCH conserva su `RequireRole`: moverlo es otra feature (puerta de roles por
  empresa, principio VIII).
