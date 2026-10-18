# Tasks: Subidas a producción sin corte

**Input**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [contracts/](contracts), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV): cada test se escribe y se ve en rojo antes de su implementación.

**Orden**: la historia 3 (apagado ordenado) va primero aunque sea P2: el azul/verde (US1, US2) apaga la copia vieja con ella, y sin ella cada subida espera el plazo completo.

## Phase 1: Setup

- [X] T001 Crear `deploy/upstream/.gitkeep` y agregar `deploy/upstream/activo.caddy` a `.gitignore`

## Phase 2: Foundational

Sin tareas: no hay esquema ni dependencia compartida que bloquee.

## Phase 3: User Story 3 — Apagar la API es rápido y limpio (P2, prerrequisito)

**Goal**: el apagado cierra los avisos en vivo, espera solo lo que está en curso y no deja ERROR si termina en plazo.
**Independent Test**: [quickstart.md §1](quickstart.md): con un stream `/events` abierto, SIGTERM a la API local (:8041) termina en < 1 s sin `level=ERROR`.

- [X] T002 [P] [US3] Test en `server/internal/realtime/broker_test.go`: `Close()` cierra el canal de cada suscriptor de todas las empresas; `Subscribe` después de `Close` devuelve un canal ya cerrado; `Publish` después de `Close` no entra en pánico
- [X] T003 [P] [US3] Test en `server/internal/httpapi/handlers_sse_test.go`: con el broker cerrado, `Events` regresa (con `httptest` y un plazo de 1 s), aunque el contexto de la petición siga vivo
- [X] T004 [P] [US3] Test en `server/internal/httpapi/router_test.go`: `/readyz` responde 503 `{"status":"shutting_down"}` cuando se marca el apagado, y `/healthz` sigue en 200
- [X] T005 [P] [US3] Test de integración bajo `appRoleStore` en `server/internal/integration/lecturas_apagado_test.go`: `Esperar(ctx)` regresa cuando termina la lectura en curso; si vence `ctx`, la lectura queda `fallida` y no `en_curso`, con lecturas vivas de dos empresas (una marca por id con `WithTenant` de su empresa; sin empresa, RLS no ve nada y el update no haría nada)
- [X] T006 [US3] Implementar `Broker.Close` en `server/internal/realtime/broker.go` (bandera `closed` bajo el mismo `mu`)
- [X] T007 [US3] En `server/internal/httpapi/handlers_sse.go`, leer `ev, ok := <-ch` y regresar si `!ok`
- [X] T008 [US3] En `server/internal/httpapi/router.go`, que `/readyz` lea una bandera atómica de apagado, y exponerla para que la marque `main`
- [X] T009 [US3] En `server/internal/app/menus_de_plataforma.go`, un `sync.WaitGroup` alrededor de `go s.correrLectura(...)` y el método `Esperar(ctx)`, que al vencer marca fallidas las lecturas vivas por id, cada una con `WithTenant` de su empresa (el servicio guarda empresa e id de cada lectura lanzada)
- [X] T010 [US3] En `server/cmd/api/main.go`, el apagado según [contracts/apagado.md](contracts/apagado.md): marcar `/readyz`; detener los ciclos de fondo; `srv.RegisterOnShutdown(broker.Close)`; `Shutdown` con 15 s; lecturas y ciclos de fondo hasta los 20 s (mínimo 3 s); `slog.Info("shutdown complete", "duration_ms", …)` si terminó y `slog.Error` solo si algo venció
- [X] T011 [US3] Agregar el flag `-readycheck` (gemelo de `runHealthcheck`, contra `/readyz`) en `server/cmd/api/main.go`
- [X] T012 [US3] Medir en local el escenario de [quickstart.md §1](quickstart.md) y anotar la duración en `quickstart.md`

## Phase 4: User Story 1 — Quien opera no nota la subida (P1)

**Goal**: dos copias detrás de Caddy, el despliegue cambia de una a otra sin peticiones fallidas.
**Independent Test**: [quickstart.md §2](quickstart.md): `medir-corte` reporta 0 fallidas durante dos subidas seguidas en local.

- [X] T013 [P] [US1] Escribir `scripts/medir-corte.sh URL SEGUNDOS`: pide la URL cada 200 ms con `curl --max-time 2`, imprime totales, fallidas y latencia máxima, y sale ≠0 si hubo alguna fallida ([research.md R8](research.md))
- [X] T014 [P] [US1] Vitest en `web/src/hooks/useOrderEvents.test.ts` (nuevo), `web/src/hooks/useMenuEvents.test.ts` y `web/src/features/pos/usePedidosDePlataforma.test.ts`: un `onopen` después de un `onerror` invalida las mismas consultas que un evento; el primer `onopen` no invalida
- [X] T015 [US1] Implementar la invalidación al reconectar en `web/src/hooks/useOrderEvents.ts`, `web/src/hooks/useMenuEvents.ts` y `web/src/features/pos/usePedidosDePlataforma.ts`
- [X] T016 [US1] En `deploy/docker-compose.yml`: reemplazar el servicio `api` por `api-blue` y `api-green` (anchor YAML común), con `tag: api` en el logging de journald; `caddy` monta `./upstream:/etc/caddy/upstream:ro` y deja de depender de `api`
- [X] T017 [US1] En `deploy/Caddyfile`: `reverse_proxy { import /etc/caddy/upstream/activo*.caddy; flush_interval -1 }`. El patrón glob es a propósito: con un nombre literal, si el archivo falta Caddy no arranca (VM reiniciada entre el `git checkout` y la primera corrida del script)
- [X] T018a [US1] Test en `scripts/tests/deploy-azul-verde-test.sh` (patrón de `restaurar-respaldo-test.sh`), con `docker` y `curl` simulados en el `PATH`: primera subida desde `<proyecto>-api-1`, azul→verde, verde→azul, nueva que no queda lista (tráfico y archivo intactos, salida ≠0), `activo.caddy` ausente (se escribe `to api:8080` antes de recargar) y `activo.caddy` a medio escribir (el tmp no se lee nunca); engancharlo en `lefthook.yml` pre-commit cuando cambie `scripts/deploy-azul-verde.sh`
- [X] T018 [US1] Escribir `scripts/deploy-azul-verde.sh` según [contracts/despliegue.md](contracts/despliegue.md), pasos 0 a 8, incluida la transición desde `${COMPOSE_PROJECT_NAME:-deploy}-api-1` (nombre derivado del proyecto, nunca fijo)
- [X] T019 [US1] En `Makefile`, que `deploy-image` llame a `scripts/deploy-azul-verde.sh` en vez de `pull` + `up -d`; `deploy` (fallback) igual
- [X] T020 [US1] Correr [quickstart.md §2](quickstart.md) en local (proyecto de compose aparte, `API_DOMAIN=localhost`, Caddy en puertos altos): transición `api`→azul, azul→verde y verde→azul, con `medir-corte` y un stream SSE abierto; anotar los números

## Phase 5: User Story 2 — Una versión rota no tumba al local (P1)

**Goal**: si la nueva no queda lista, el tráfico se queda en la vieja y el despliegue falla.
**Independent Test**: [quickstart.md §2](quickstart.md) paso 3: con una versión rota, `medir-corte` 0 fallidas y el script termina ≠0.

- [X] T021 [US2] En `scripts/deploy-azul-verde.sh`, verificar los caminos de falla de los pasos 3 a 6 (detener el destino, restaurar `activo.caddy`, recargar) y que un segundo despliegue tras la falla suba normal
- [X] T022 [US2] Correr en local la versión rota (`APP_DB_PASSWORD` equivocado en el destino) y la interrupción a media subida (matar el script después del paso 3), y anotar el resultado en `quickstart.md`

## Phase 6: User Story 4 — Nada sale dos veces durante el traslape (P2)

**Goal**: probar que los procesos de fondo son seguros con dos copias.
**Independent Test**: `make test-integration` con el test de abajo.

- [X] T023 [P] [US4] Test de integración en `server/internal/integration/daily_summary_concurrente_test.go`: ocho `SendPending` concurrentes desde dos pools del rol de app, mismo día, dan exactamente 1 envío (el aislamiento de `daily_summary_sends` ya lo vigila `TestEveryCompanyTableIsIsolated`; aquí no se lee nada nuevo)
- [X] T024 [P] [US4] Test de integración en `server/internal/integration/lecturas_traslape_test.go`: `CerrarLecturasColgadas` no toca una lectura `en_curso` de menos de `TiempoMaximoDeLectura`
- [X] T025 [US4] Documentar en `research.md` R3–R5 el resultado de las pruebas y confirmar que no quedan otros temporizadores (`grep NewTicker` en `server/`)

## Phase 7: User Story 5 — Las migraciones toleran a la versión anterior (P3)

- [X] T026 [US5] Enmendar `.specify/memory/constitution.md` (MINOR → 1.24.0, en commit propio): toda migración funciona con la versión anterior de la API corriendo; se agrega en una subida y se quita en otra; un plan que quite o renombre lo que la versión en producción lee, en la misma subida, es hallazgo de `/speckit-analyze`
- [X] T027 [US5] Revisar que `.claude/agents/db-architect.md` mida la regla nueva y apuntarlo al principio

## Phase 7b: Hallazgos de la revisión de código (2026-10-18)

- [X] T033 [US3] `FailMenuRead` solo cierra lecturas `en_curso` en `server/queries/menus_plataforma.sql` (+ `make sqlc`; revisado por `db-architect`); test `TestFailingAReadDoesNotOverwriteOneThatFinished` en `server/internal/integration/lecturas_apagado_test.go`
- [X] T034 [US4] La pasada del resumen termina aunque el apagado cancele el ciclo (`context.WithoutCancel` en `server/internal/app/daily_summary.go`) y `main` espera los ciclos de fondo; test `TestShutdownDuringASummaryPassLetsItFinish` en `server/internal/integration/daily_summary_concurrente_test.go`
- [X] T035 [US3] Presupuesto por etapa del apagado en `server/cmd/api/main.go` y cierre en paralelo con 5 s de tope en `Esperar` (`server/internal/app/menus_de_plataforma.go`)
- [X] T036 [US3] Test del cierre de avisos en vivo sincronizado con el primer `Flush` en vez de dormir, en `server/internal/httpapi/apagado_test.go`
- [X] T037 [US3] `/events` responde 503 `SHUTTING_DOWN` con el broker cerrado (`domain.ErrShuttingDown`, mapeado en `server/internal/httpapi/respond.go`); test `TestEventsDuringShutdownAnswers503`

## Phase 8: Polish

- [X] T028 [P] Escribir `docs/despliegue-sin-corte.md`: cómo sube, cómo saber qué color está activo, cómo regresar a la versión anterior, y el `journalctl SYSLOG_IDENTIFIER=api`; agregar su renglón en `docs/README.md`
- [X] T029 [P] Actualizar `AGENTS.md` §2/§3 (`make deploy-image`, bitácora por `tag`) y el comentario de cabecera de `deploy/docker-compose.yml` que cita `deploy-api-1`
- [X] T030 `cd server && go build ./... && go test ./...`, `make test-integration`, `cd web && bun run lint && bun run vitest run`, `make ci-local`
- [X] T031 Una vez, con la versión final en el ambiente de pruebas: transición, una subida normal y una rota con `scripts/medir-corte.sh https://api-dev.elgatobobah.com/healthz`; anotar SC-001, SC-002, SC-004 y SC-006 en `quickstart.md`
- [ ] T032 Producción, solo con OK explícito del dueño: respaldo, subir, y revisar solo la salud (`/healthz`, `/readyz`, job verde, journal sin errores)

## Dependencies

- US3 (T002–T012) → US1 (T016–T020; T018a antes de T018) → US2 (T021–T022)
- US4 y US5 son independientes: se pueden hacer en cualquier momento
- T031 requiere todo lo anterior; T032 requiere T031 y el OK del dueño

## Parallel

- T002, T003, T004 y T005 (tests de US3, archivos distintos)
- T013 y T014 junto con cualquier tarea de US3
- T023, T024, T026, T028 y T029 junto con US1 y US2

## Implementation Strategy

1. **Primero US3**: por sí sola quita 10 de los 15 s de corte y el ERROR falso. Se puede subir aislada.
2. Después US1 + US2 juntas: el script nace con sus caminos de falla.
3. US4 y US5 en paralelo, antes del ambiente de pruebas.
