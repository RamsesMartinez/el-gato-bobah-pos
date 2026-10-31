# Contrato: asistencia en la apertura y el cierre de caja

Todas bajo `/api/v1`, con sesión, dentro del grupo `/cash-sessions` que ya existe (mismo acceso que
abrir y cerrar hoy).

## GET /cash-sessions/attendance?registerId={id}

Estado de asistencia para la acción que toca en esa caja: `open` si está cerrada, `close` si está
abierta. Lo calcula `domain.CashOpenAttendance` / `CashCloseAttendance`: el mismo predicado que
aplican abrir y cerrar.

`registerId` ausente o no numérico → 400. Caja que no existe → 404.

```json
{
  "stage": "close",
  "applies": true,
  "ready": false,
  "person": { "id": 12, "name": "Ana", "active": true },
  "openSince": "2026-10-30T15:02:00Z",
  "stale": false,
  "isSelf": false,
  "canAuthorize": true
}
```

| Campo | Significado |
|---|---|
| `stage` | `open` o `close` |
| `applies` | false con la asistencia apagada o si la persona no lleva asistencia; entonces `ready` es true y lo demás sobra |
| `ready` | se puede confirmar sin hacer nada más |
| `person` | `open`: quien está en sesión. `close`: quien abrió el turno. Siempre presente (nunca `null`). `active` es false si la persona fue dada de baja: en `close` la hoja no ofrece marcar con PIN, que `POST /attendance/clock` rechaza para una persona inactiva |
| `openSince` | entrada abierta de esa persona; ausente si no tiene |
| `stale` | la entrada abierta tiene más de 16 h: no se puede marcar salida con PIN (en `close`) ni cuenta como vigente (en `open`) |
| `isSelf` | `person.id` es quien está en sesión |
| `canAuthorize` | solo en `close`: quien está en sesión puede usar el cierre autorizado ahora (permiso y regla del cierre propio ya aplicados) |

No incluye IP, aparato ni historial: solo lo que la hoja necesita para decidir qué ofrecer.

## POST /cash-sessions (abrir) — cambio

Cuerpo sin cambios. Respuesta nueva:

| Situación | Respuesta |
|---|---|
| La regla aplica y quien abre no tiene entrada vigente | 409 `CLOCK_IN_REQUIRED`, mensaje «Para abrir la caja, primero marca tu entrada» |

Se decide dentro de la transacción de la apertura, antes de escribir el turno.

## POST /cash-sessions/close (cerrar) — cambio

Campo nuevo opcional en el cuerpo:

```json
{ "authorizedClose": { "reason": "Se fue sin checar, avisó por teléfono" } }
```

| Situación | Respuesta |
|---|---|
| La regla aplica, quien abrió tiene entrada abierta, sin `authorizedClose` | 409 `OPENER_NOT_CLOCKED_OUT`, mensaje «Ana abrió esta caja y no ha marcado su salida» |
| La regla aplica y la persona fue dada de baja con entrada abierta | 409 `OPENER_NOT_CLOCKED_OUT` igual; solo se resuelve con `authorizedClose` |
| `authorizedClose` sin el permiso `cash.close_without_clock_out` | 403 `FORBIDDEN`, «Tu usuario no puede cerrar la caja sin la salida de quien la abrió» |
| `authorizedClose` de quien abrió, con entrada de 16 h o menos | 409 `CLOCK_OUT_WITH_PIN`, «Marca tu salida con tu PIN para cerrar la caja» |
| `authorizedClose.reason` vacío o de más de 200 caracteres | 400 `VALIDATION` |
| `authorizedClose` válido | 200 como hoy; la entrada de quien abrió queda «sin salida» y se escribe `cash.closed_without_opener_clock_out` |
| `authorizedClose` cuando la regla no aplica o ya no hay entrada abierta | 200, cierre normal; sin evento de cierre autorizado |

Se decide dentro de la transacción del cierre, después de `LockSessionForClose` y antes de revisar
pedidos pendientes.

## POST /attendance/clock — sin cambios

La hoja lo usa tal cual (`{ userId, pin, expected }`), con su límite por IP y su bloqueo por PIN.
