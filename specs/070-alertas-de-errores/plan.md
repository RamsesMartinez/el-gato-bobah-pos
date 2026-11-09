# Implementation Plan: Aviso por correo de cada error interno de la API

**Branch**: `070-alertas-de-errores` | **Date**: 2026-11-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/070-alertas-de-errores/spec.md`

## Summary

Un middleware nuevo en el router reemplaza al `middleware.Recoverer` de chi: recupera los panics
(respondiendo el mismo 500 `INTERNAL` de siempre) y, al terminar cada petición con estado 500, arma
un `domain.ServerAlert` y lo **encola** sin esperar en un `alerts.Notifier`. Un solo goroutine del
notifier decide con una ventana de 15 min por (método, patrón de ruta, tipo de error) —en Redis
compartido, con memoria del proceso como respaldo— si el correo sale ya, se cuenta como repetición
o sale como resumen al cerrar la ventana, y lo manda con el `mailer` SMTP que ya existe. El texto
del correo lo arma `domain` (puro), que también enmascara secretos.

## Technical Context

**Language/Version**: Go 1.27.3

**Primary Dependencies**: chi (patrón de ruta), go-redis v9 (ya en `go.mod`), `net/smtp` vía
`internal/mailer`. Ninguna dependencia nueva.

**Storage**: Redis (llaves con vencimiento, nada en Postgres). Sin migración.

**Testing**: `go test` table-driven en `domain`; `alerts` con un enviador falso (func) y relojes
cortos; Redis real del compose para el agrupamiento compartido (salta si no hay `REDIS_URL` de
prueba, como `ratelimit_redis_test.go`); `httpapi` con `httptest` sobre el `Router` real.

**Target Platform**: la API en la VM (contenedor Linux), azul/verde.

**Project Type**: web-service (solo `server/`).

**Performance Goals**: cero espera añadida al request; encolar es un `select` no bloqueante.

**Constraints**: correo ≤ 1 por ventana por llave (más el resumen); apagado ≤ 5 s extra.

**Scale/Scope**: un local; decenas de 500 al mes en el peor caso.

## Capas (principio I)

| Pieza | Capa | Archivo |
| --- | --- | --- |
| `ServerAlert`, `AlertKey`, `SanitizeAlertText`, `AlertEmail` (asunto y cuerpo), filtro de marcos | `domain` (puro) | `server/internal/domain/server_alert.go` |
| `ErrorKind(err)` (tipo de fondo y código de Postgres) | `alerts` (importa pgconn) | `server/internal/alerts/kind.go` |
| `Notifier`: cola acotada, goroutine único, ventanas en Redis/memoria, resumen al cerrar, `Close` | `alerts` (infraestructura, como `cache`) | `server/internal/alerts/notifier.go`, `window.go` |
| Nombre de la empresa para el correo (bajo RLS, con la empresa del aviso) | `app` | `CompanyService.Name` en `server/internal/app/company.go` |
| Middleware `reportServerErrors`, registro del error desde `Error()` | `httpapi` | `server/internal/httpapi/server_errors.go`, `respond.go`, `logging.go` |
| `ALERT_EMAIL_TO` | `config` | `server/internal/config/config.go` |
| Cableado y apagado | `cmd/api` | `server/cmd/api/main.go` |

`alerts` no es `app`: no orquesta dominio contra la base; es transporte, igual que `cache` y
`mailer`. La única lectura de base (el nombre) entra como función inyectada desde `app`.

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I. Layering | Tabla de arriba. El texto y el enmascarado son puros en `domain`. |
| II. Errores y contexto | El error llega envuelto a `Error()`; el middleware no crea `context.Background()` dentro del request: el goroutine del notifier tiene su propio ctx raíz, terminado por `Close`. Todo goroutine termina (ctx de cierre); los timers se detienen en `Close`. |
| III. Dinero | No aplica. |
| IV. Test-first | Bordes enumerados en el spec → tests antes del código (tasks). Sin migración: no aplica la regla de migración ni de RLS para tablas; la lectura del nombre usa `WithTenant` con la empresa del aviso (RLS «solo mi fila»). |
| V. Seguridad | Sin cuerpos, encabezados ni query en el correo; patrón de ruta, no URL; enmascarado de JWT, Bearer, clave-valor sensibles y dígitos de tarjeta; test sobre el correo completo. Envío fallido = renglón de bitácora sin contenido. Un atacante que fuerza 500 en bucle no llena el buzón (agrupamiento) ni la memoria (cola acotada, ventanas que vencen). |
| VI. YAGNI | Sin reintentos ni cola persistente; sin interfaz para el enviador (una `func`); sin config para la ventana (constante). La única variable nueva es el destinatario, que sí cambia por despliegue. |
| VII. Comentarios | El porqué de cada borde (abort, ventana renovada, TTL > timer) en el código. |
| VIII. Puertas | No toca esquema: «¿se puede agregar después al mismo costo?» — sí, todo. |
| Migración tolera versión anterior | Sin migración. Azul/verde: la versión vieja no avisa; la nueva sí; no comparten llaves que la vieja entienda. |

Sin violaciones.

## Pantallas y maquetas

Sin pantallas nuevas.

## Project Structure

### Documentation (this feature)

```text
specs/070-alertas-de-errores/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/alert-email.md
└── tasks.md
```

### Source Code

```text
server/
├── cmd/api/main.go                       # cablea Notifier, lo cierra en el apagado
├── internal/domain/server_alert.go(+_test)
├── internal/alerts/{notifier,window,kind}.go(+_test)
├── internal/app/company.go               # Name(ctx, id)
├── internal/config/config.go             # ALERT_EMAIL_TO
└── internal/httpapi/
    ├── server_errors.go(+_test)          # middleware: recover + aviso
    ├── respond.go                        # Error() registra el error del 500
    ├── logging.go                        # traceInfo con empresa y nombre; reusa el del middleware
    ├── middleware.go                     # RequireAuth llena empresa y nombre
    └── router.go                         # reemplaza middleware.Recoverer
docs/email-zoho.md                        # el correo ya no es solo recuperación; ALERT_EMAIL_TO
deploy/.env.example (si existe)           # ALERT_EMAIL_TO
```

**Structure Decision**: solo backend.

## Riesgos y cómo se ven

- **Si `Error()` deja de registrar el error** (un wrapper nuevo del `ResponseWriter` sin `Unwrap`), el
  correo sale con «error sin detalle» en vez de no salir: el estado 500 lo ve el middleware igual.
  Lo cubre un test sobre el `Router` real.
- **Destinatario sin configurar en producción**: avisos apagados en silencio. El arranque escribe
  `server_alerts_disabled` y el despliegue de esta feature agrega la variable en las dos VM.

## Complexity Tracking

Sin violaciones.
