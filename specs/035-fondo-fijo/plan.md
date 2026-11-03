# Implementation Plan: Fondo de caja fijo

**Branch**: `035-fondo-fijo` | **Date**: 2026-10-11 | **Spec**: [spec.md](spec.md)

## Summary

Cada caja guarda un fondo fijo (`cash_registers.fixed_float`). El cierre lo trae escrito, quien
cierra puede cambiarlo solo para ese cierre, y la diferencia contra lo contado se registra como un
movimiento `retiro` del turno con quién se lo llevó, en la misma transacción que el cierre. El
retiro queda fuera del esperado del cajón. Cierres previos intactos. Decisiones en [research.md](research.md).

## Technical Context

**Language/Version**: Go 1.27.2 (server), TypeScript / React 19 (web)
**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query
**Storage**: PostgreSQL con RLS (`gatobobah_app`)
**Testing**: `go test` (domain table-driven + integración bajo `appRoleStore`), vitest
**Target Platform**: API en VM Linux; tabletas 1024×600
**Project Type**: web-service + SPA
**Constraints**: subida con cajas abiertas (migración aditiva, API vieja y nueva conviven)
**Scale/Scope**: 2 pantallas tocadas (hoja de cierre, Ajustes › Caja), 1 migración

## Migración: número

**La migración toma el número libre siguiente al momento de fusionar.** Hoy develop llega a
`0089_attendance_and_audit.sql` y la rama 034 trae otra `0089_catalog_versions.sql`; ese choque lo
resuelve la sesión del POS al fusionar la 034. Esta rama no toca la 034 ni su migración. Aquí se
nombra provisionalmente `0090_fixed_cash_float.sql` y **se renumera al fusionar** si ese número ya
está tomado (también su test de integración y cualquier mención en docs).

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | migración + `queries/cash.sql` (sqlc) → `domain.Withdrawal` / `ResolveFloatLeft` (puras) → `app.BackofficeService.CloseSession` y `SetFixedFloat` (tx) → handlers finos |
| II Errores | `domain.ErrValidation` envuelto con `%w`; mapeo solo en `httpapi.Error` |
| III Dinero | `Round2` + `ValidMoney` en fondo fijo, fondo del día y retiro; el retiro se clasifica una vez y NO entra al esperado: test que falla nombrando «el retiro se restó del esperado» |
| IV Test-first | tests de dominio y de integración antes del código; `inTheThreeCases` sobre lectura/escritura del fondo fijo; test de migración con dos empresas y cierres previos |
| Apertura a ciegas | `fixedFloat` no viaja en `GET /cash-registers` del cajero (research R7) |
| V Seguridad | `RequirePermission(PermCashConfigure)`; `withdrawalUserId` validado contra la empresa (FK compuesta); parámetro malformado → 400 |
| VI YAGNI | sin reverso de retiro, sin historial del fondo fijo (el snapshot por cierre basta) |
| VII Comentarios / idioma | identificadores en inglés (`fixed_float`, `withdrawal`); texto de UI en español |
| VIII Puertas | por caja (no cierra «más de una caja»); permiso, no rol; snapshot `fixed_float_at_close` |
| Subida operando | solo columnas nulables, checks ampliados, índice parcial; servidor acepta cuerpo viejo (R4) |

Sin violaciones.

## Pantallas y maquetas

1. **Hoja de cierre** (`web/src/features/backoffice/CashPage.tsx`, `FondoQueSeDeja.tsx`) — opción **A** con ajuste del dueño:
   - Pregunta «¿Se retira dinero del cajón?» con dos botones de 48 px: «Sí, dejar el fondo y retirar el resto» (**marcado al abrir**) y «No, todo se queda en el cajón».
   - Con «Sí»: el campo «Se queda de fondo» llega con el fijo (pill «Solo hoy» si se cambia), y debajo el retiro calculado y «Se lo lleva» como botones de persona (quien cierra ya marcado).
   - Con «No»: el fondo es lo contado, sin campo, sin retiro ni persona. El cliente manda `floatLeft = contado`; el contrato no cambia.
   - Vacío bloquea «Cerrar caja». Todo va al final de la hoja, debajo de terminales, como hoy.
   - **Ajustes de la revisión de tableta**:
     - Una sola caja de ~150 px: la pregunta va como dos segmentos en el renglón del título, y «Se queda [$1,000] → Retiro $4,300» en un renglón, así el teclado del sistema no tapa el retiro.
     - Las personas van en una sola fila con desplazamiento horizontal, con quien cierra primero. Con más de ~6 personas se usa `Picker`.
     - El `ConfirmSheet` de «Cerrar caja» que ya existe dice «Se retiran $X · se los lleva N», o «Todo se queda en el cajón: $X». Así la persona marcada por omisión se revisa sin agregar toques.
     - Si falta la persona, el aviso «Falta capturar…» lo nombra.
     - Con «No», se agrega en gris «(el fijo es $X)».
2. **Ajustes › Caja** (`web/src/features/admin/AjustesDeCaja.tsx`) — opción **B**: bloque «Fondo fijo» con un renglón de 56 px por caja (nombre y monto, o «Sin configurar ›»). Al tocarlo se abre una hoja inferior con un teclado grande, «Quitar fondo fijo», «Cancelar» y «Guardar». Quitar manda `fixedFloat: null`.
   - **Ajustes de la revisión de tableta**:
     - El campo de la hoja es de solo lectura (`inputMode="none"`) y se teclea con el teclado de la hoja, reutilizando el de la hoja de conteo o `PinPad`.
     - «Quitar fondo fijo» va a la izquierda, `ghost` y en rojo.
     - Sin `PermCashConfigure`, el renglón se ve sin flecha y no abre nada.
3. **Corte cerrado**: renglón «Retiro al cierre: $X, se lo llevó N» y el fondo dejado. Solo agrega un renglón a una lista existente, así que no lleva maqueta.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Hoja de cierre | https://claude.ai/artifact/REve1nLB2PT1v2FDqpsFnP | A + «¿Se retira dinero?» (abre en Sí) | 0 toques en el cierre normal; no retirar queda visible a un toque (pedido del dueño) |
| Ajustes › Caja: fondo fijo | https://claude.ai/artifact/8yzCc5ixHmHaSVsEerEMFk | B | Lista compacta y teclado grande; no se cambia por accidente |

## Project Structure

```text
specs/035-fondo-fijo/  plan.md research.md data-model.md quickstart.md contracts/api.md

server/migrations/0090_fixed_cash_float.sql        # número provisional, ver arriba
server/queries/cash.sql                             # NetCashMovements, SetFloatLeft, InsertWithdrawal, SetRegisterFixedFloat
server/internal/domain/cash_float.go (+_test)      # Withdrawal, ResolveFloatLeft
server/internal/domain/permission.go               # PermCashConfigure
server/internal/app/backoffice.go                  # CloseSession, SetFixedFloat, SessionView
server/internal/httpapi/handlers_backoffice.go, router.go
server/internal/integration/fixed_float_test.go    # SC-004, doble cierre, tres casos, migración
web/src/api/backoffice.ts
web/src/features/backoffice/CashPage.tsx, FondoQueSeDeja.tsx (+test)
web/src/features/admin/AjustesDeCaja.tsx (+test)
```

## Riesgos conocidos

- El rollback de la API es solo hacia adelante: con retiros ya guardados, la API vieja infla las ventas de esos cortes (research R7).
- Una tableta con el front viejo mostrará el retiro como «Salida» hasta recargar.

## Complexity Tracking

Sin violaciones que justificar.
