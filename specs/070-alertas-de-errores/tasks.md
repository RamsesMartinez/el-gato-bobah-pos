---
description: "Tareas de la spec 070: aviso por correo de cada error interno de la API"
---

# Tasks: Aviso por correo de cada error interno de la API

**Input**: `specs/070-alertas-de-errores/` (plan, spec, research, data-model, contracts, quickstart)

**Tests**: obligatorios (constitución IV): cada tarea de código va precedida de su test, que se ve
fallar por la razón correcta.

## Phase 1: Setup

- [X] T001 Agregar `AlertEmailTo string \`env:"ALERT_EMAIL_TO"\`` y `ServerAlertsEnabled()` (destinatario y SMTP presentes) con su test en `server/internal/config/config.go` y `server/internal/config/config_test.go`

## Phase 2: Foundational (dominio puro)

- [X] T002 [P] Tests table-driven de `SanitizeAlertText` en `server/internal/domain/server_alert_test.go`: JWT, `Bearer x`, `password=…`, `"token":"…"`, `pin: 1234`, `api_key=…`, tarjeta de 16 dígitos con y sin espacios/guiones, texto sin secretos intacto, recorte a 1000 caracteres
- [X] T003 [P] Tests de `ProjectFrames` (solo marcos del módulo, sin `runtime`/`net/http`/chi, ≤ 12, sin el marco de `httpapi.Error`) y de `AlertKey` en `server/internal/domain/server_alert_test.go`
- [X] T004 Tests de `AlertEmail(alert, summary)` en `server/internal/domain/server_alert_test.go`: asunto inmediato, de panic y de resumen según [contrato](contracts/alert-email.md), con el host de `APP_BASE_URL` como ambiente; hora en CDMX; «sin sesión»; empresa sin nombre muestra solo el id; repeticiones y descartados solo cuando > 0; HTML escapado (`<script>` en el mensaje no sale crudo)
- [X] T005 Implementar `ServerAlert`, `AlertSummary`, `SanitizeAlertText`, `ProjectFrames`, `AlertKey`, `AlertEmail` en `server/internal/domain/server_alert.go` hasta que T002–T004 pasen
- [X] T006 [P] Tests de `ErrorKind` (root `%T`, `fmt.Errorf` con `%w` en cadena, `errors.Join`, `*pgconn.PgError` → `pg:23505`, nil) en `server/internal/alerts/kind_test.go`; implementar en `server/internal/alerts/kind.go`

## Phase 3: User Story 1 — me entero de un 500 (P1) MVP

**Independent test**: un 500 y un panic por el `Router` real producen un aviso con todos los campos.

- [X] T007 [US1] Tests en `server/internal/httpapi/server_errors_test.go` sobre el `Router` real con un `Notifier` cuyo enviador falso captura correos: (a) handler que llama `Error(w, errors.New("x"))` → aviso con método, patrón `/api/v1/…/{id}`, request id, kind y marcos; (b) handler que hace panic con un valor que contiene un JWT → respuesta 500 con cuerpo `INTERNAL`, aviso con `Panic`, y ni el aviso ni la bitácora llevan el JWT; (c) 400/404/409/501/503 → sin aviso; (d) panic con `http.ErrAbortHandler` → se repropaga y sin aviso; (e) petición autenticada → usuario id/nombre y empresa id en el aviso
- [X] T008 [US1] `traceInfo` gana `companyID` y `userName`; `RequireAuth` los llena; `RequestLogger` reusa el `traceInfo` del contexto si ya existe; `captureWriter` gana `Unwrap()` en `server/internal/httpapi/logging.go` y `server/internal/httpapi/middleware.go`
- [X] T009 [US1] Middleware `reportServerErrors(n *alerts.Notifier)` en `server/internal/httpapi/server_errors.go`: crea `traceInfo`, envuelve el writer (estado, error registrado, traza; `Unwrap`, `Flush`), recupera panics (repropaga `http.ErrAbortHandler`, registra `slog.Error` con el valor ya enmascarado por `SanitizeAlertText` y la traza, escribe el 500 `INTERNAL` si no se escribió nada), y con estado 500 arma `domain.ServerAlert` y llama `n.Report`
- [X] T010 [US1] `Error()` registra el error y `runtime.Callers` en el writer que lo acepta (recorriendo `Unwrap`) cuando el estado es 500, en `server/internal/httpapi/respond.go`
- [X] T011 [US1] `Router` recibe el notifier (campo en `Handlers`/`Deps`) y reemplaza `middleware.Recoverer` por `reportServerErrors` en `server/internal/httpapi/router.go` y `server/internal/httpapi/handlers.go`
- [X] T012 [US1] `CompanyService.Name(ctx, id)` vía `store.WithTenant` + `GetCompany` en `server/internal/app/company.go`, con test de integración bajo `appRoleStore` con `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa): devuelve el nombre de la empresa pedida y nunca el de otra; sin empresa no truena con un 500 sino que regresa error y el aviso sale con el id en `server/internal/integration/server_alert_company_test.go`

## Phase 4: User Story 2 — una ráfaga no llena el buzón (P1)

**Independent test**: N errores iguales con ventana corta → 1 inmediato + 1 resumen con N−1.

- [X] T013 [US2] Tests en `server/internal/alerts/notifier_test.go` (memoria, ventana inyectada de ms): primer error → correo inmediato; 99 repeticiones → ningún correo más hasta el cierre, luego uno con «99»; ventana sin repeticiones → nada al cierre; ruta o kind distinto → correo propio; repetición después del resumen renueva la ventana
- [X] T014 [US2] Tests con Redis real en `server/internal/alerts/notifier_redis_test.go` (salta sin `TEST_REDIS_URL`): dos notifiers con el mismo Redis y el mismo error → un solo inmediato y el resumen del dueño suma las repeticiones de los dos; conteo huérfano (sin dueño) se suma como «anteriores sin reportar» al abrir la siguiente ventana
- [X] T015 [US2] Implementar `Notifier` (cola, goroutine único, timers que entregan el cierre al goroutine, nombre de empresa con plazo de 2 s) en `server/internal/alerts/notifier.go` y las ventanas Redis/memoria (SET NX PX, INCR+EXPIRE, GETDEL, SET XX, DEL condicionado por Lua) en `server/internal/alerts/window.go`

## Phase 5: User Story 3 — el aviso nunca empeora el error (P1)

- [X] T016 [US3] Tests en `server/internal/alerts/notifier_test.go`: enviador que tarda → `Report` regresa en < 50 ms; enviador que falla → `slog` con `server_alert_send_failed` y sin reintento; cola llena → descarta y el siguiente correo dice cuántos; Redis apuntando a un puerto cerrado → agrupa en memoria y sigue mandando; `Close(ctx)` manda los resúmenes pendientes y respeta el plazo; `Report` después de `Close` no entra en pánico ni bloquea
- [X] T017 [US3] Implementar lo que falte de T016 en `server/internal/alerts/notifier.go` y `server/internal/alerts/window.go`
- [X] T018 [US3] Cablear en `server/cmd/api/main.go`: crear el notifier si `cfg.ServerAlertsEnabled()` (si no, `slog` `server_alerts_disabled`), pasarlo a `Deps`, y `Close` con 5 s después de `srv.Shutdown`

## Phase 6: User Story 4 — sin datos sensibles (P1)

- [X] T019 [US4] `TestAlertEmailCarriesNoSecrets` en `server/internal/httpapi/server_errors_test.go`: petición con `password`/`pin` en el cuerpo, `Authorization: Bearer <jwt>`, `?token=<jwt>` en la URL, número de tarjeta en el cuerpo, y error cuyo texto trae un JWT y `password=hunter2`; el asunto y cuerpo del correo capturado no contienen ninguno de esos valores ni la query
- [X] T020 [US4] Ajustar el armado del aviso hasta que T019 pase (el mensaje pasa por `SanitizeAlertText`; nunca se copia la URL cruda) en `server/internal/httpapi/server_errors.go`

## Phase 7: Polish

- [X] T021 [P] Documentar en `docs/email-zoho.md` que el correo ya no es solo recuperación (avisos de error, `ALERT_EMAIL_TO`, cómo se agrupan) y agregar `ALERT_EMAIL_TO` a `deploy/.env.example` si existe y a `scripts/dev-api.sh`/`check-env` si listan variables de correo
- [X] T022 `cd server && go build ./... && go test ./...` y `make test-integration`
- [X] T023 Prueba local con mailpit según [quickstart](quickstart.md): 500 real (postgres detenido), ráfaga, Redis detenido
- [ ] T024 Ambiente de pruebas: configurar SMTP Zoho y `ALERT_EMAIL_TO` en `pos-vps-dev`, forzar un 500 y confirmar el correo real; producción: agregar `ALERT_EMAIL_TO` en `pos-vps` antes de subir

## Dependencies

- T001 → T018. T002–T006 → todo lo demás. US1 (T007–T012) necesita un `Notifier` mínimo: T015 se adelanta en su forma más simple (sin Redis) antes de T009, y US2/US3 la completan.
- US4 depende de US1.

## Parallel

- T002, T003, T006 en paralelo (archivos distintos). T012 en paralelo con T013–T015.

## Strategy

MVP = US1 con notifier en memoria. Después agrupamiento compartido (US2), robustez (US3), y la
prueba de secretos (US4) antes de cualquier push.
