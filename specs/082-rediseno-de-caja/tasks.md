---
description: "Tareas del rediseño de Caja (082, issue #290)"
---

# Tasks: Rediseño del módulo Caja

**Input**: `specs/082-rediseno-de-caja/` — [plan.md](plan.md), [spec.md](spec.md),
[research.md](research.md), [data-model.md](data-model.md), [contracts/](contracts/),
[quickstart.md](quickstart.md).

**TDD obligatorio** (constitución IV): en cada bloque el test va antes del código y se ve fallar por
la razón correcta. Las tareas de pantalla citan su fila de «Maquetas» del plan.

**Orden de entrega**: las cuatro fases del issue, cada una con su PR a `develop` mergeado en verde y
al día antes de empezar la siguiente. Dentro de cada fase, las historias que entrega.

## Formato: `[ID] [P?] [Story] Descripción`

- **[P]**: se puede hacer en paralelo (otro archivo, sin depender de una tarea abierta).
- **[Story]**: US1…US7 del spec.

---

## Phase 1: Setup

- [X] T001 Traer `origin/develop` a la rama (merge firmado, ya trae el release #288) y correr `codegraph sync` en /home/ramy/git/egb-082-rediseno-de-caja
- [X] T002 Línea base de la red de caja (FR-020): guardar en el scratchpad el número de casos que pasan de `make test-integration` (archivos de `server/internal/integration/` nombrados en research.md R5), `cd web && bun run vitest run src/features/backoffice` y `bun run e2e` de `caja-*`, `arqueo-ciego`, `contar-el-cajon` y `presupuesto-del-cierre` contra el `make start` del worktree; no se versiona

---

## Phase 2: Foundational (bloquea las fases 1 a 4)

- [X] T003 Crear un admin local con contraseña y PIN nuevos (`make reset-admin`) o restaurar datos con `make db-restaurar` (API detenida), para e2e y prueba con usuario nuevo; anotar las credenciales solo en el scratchpad
- [X] T004 [P] Test table-driven `TestCutDifference` en `server/internal/domain/cash_difference_test.go`: corte posterior a 0067 (cajón −120, métodos de cajón en 0 → Drawer −120, Otros 0, Total −120, el efectivo «cubierto»), entre 0066 y 0067 (conteo sin esperado → Drawer nil, efectivo cuenta en Otros), anterior a 0066 (sin conteo), sobrante, redondeo a 2 decimales, y esperado oculto (Total nil). Incluye `TestElEfectivoNoSeCuentaDosVecesEnLaDiferencia`, que falla nombrando el concepto duplicado
- [X] T005 Implementar `MethodDiff`, `DrawerCount`, `CutDiff` y `CutDifference` en `server/internal/domain/cash_difference.go` hasta que T004 pase

**Checkpoint**: la regla única existe y está probada en `domain`.

---

## Phase 3: Entrega 1 — Lectura y claridad (US1, US2, US6) · PR «082 fase 1»

### US1 — Una sola diferencia, rotulada (P1)

- [X] T006 [US1] Test de integración `TestTheThreeScreensShowTheSameDifference` en `server/internal/integration/cut_difference_test.go`: cierra un corte con el cajón corto y la tarjeta cuadrada; la fila de `GET /cash-sessions/`, `GET /cash-sessions/{id}` y la respuesta de `POST /cash-sessions/close` traen el mismo `difference {drawer, otherMethods, total}`; con arqueo ciego y sin `cash.configure`, las tres piezas van nulas donde hoy va nulo `totalDifference`
- [X] T007 [US1] Cambiar `ListSessions` en `server/queries/cash.sql` para devolver las piezas (`methods_uncovered_difference`, `drawer_expected`, `drawer_counted`, `has_closing_count`) en lugar de sumar `total_difference`, con el mismo `where` que `CountSessions`; `make sqlc`
- [X] T008 [US1] En `server/internal/app/backoffice.go`: `SessionHistoryFor`, `SessionDetailFor` y la vista que devuelve `CloseSession` calculan `difference` con `domain.CutDifference`; `totalDifference` se conserva igual a `difference.total` (compatibilidad); el ocultamiento por arqueo ciego corre antes y deja las piezas nulas (R3). T006 pasa
- [X] T009 [P] [US1] Test vitest `web/src/features/caja/diferencia.test.tsx`: `<Diferencia>` escribe «Faltante −$120.00» con U+2212, «Sobrante +$35.50», «Cuadra», «—» con null; `moneyFixed(1200)` = «$1,200.00»; el contraste de los tokens `fg.shortage`/`fg.overage` contra `bg` es ≥ 7:1 en claro y oscuro
- [X] T010 [US1] Crear `web/src/features/caja/diferencia.tsx` (`Diferencia`, `moneyFixed`, `montoOSinDato`, mover `diffColor` desde `CashPage.tsx`) y los tokens `fg.shortage`/`fg.overage` en `web/src/components/ui/provider.tsx` (Maquetas M1)
- [X] T011 [US1] Tipos `difference` en `web/src/api/backoffice.ts` (campos opcionales para forzar la guarda) y test en `CashPage.test.tsx`: lista, detalle y diálogo «Caja cerrada» muestran la misma diferencia; el efectivo de la conciliación dice «ver arqueo» cuando hay arqueo
- [X] T012 [US1] En `web/src/features/backoffice/CashPage.tsx`: tablas en Esperado · Contado · Diferencia (TotalsTable, terminales, ArqueoDelCorte, TablaDelCierre), tres cifras arriba del detalle (Cajón · Otros métodos · Total) según Maquetas M1, diálogo «Caja cerrada» con las tres partes, montos con `moneyFixed` alineados a la derecha y `tabular-nums`, sobrante fuera del recuadro rojo de faltantes
- [X] T013 [P] [US1] Estado de venta con rótulo traducido (`CashPage.tsx` ~1825) y test; aviso de salidas sin concepto sin «corrígelas en Caja» cuando ya se está en Caja (`AvisosDeCaja.tsx`, prop de contexto) con su test

### US2 — El detalle cuenta la historia (P1)

- [X] T014 [US2] Test de integración `TestTheOpeningReasonExplainsThePreviousCut` en `server/internal/integration/cut_difference_test.go`: dos cajas; abrir la B con conteo distinto y motivo+nota; el histórico trae `openingReason`/`openingReasonNote` en el turno nuevo y `explainedByNext` en el corte anterior **de la B** (no en el de la A); la última fila de una página de 20 encuentra su siguiente en la página 2; con arqueo ciego, `explainedByNext` se oculta junto con el arqueo del último corte; incluye `inTheThreeCases`
- [X] T015 [US2] `left join lateral` del turno siguiente y del anterior en `ListSessions` y en una consulta `GetSessionNeighbors` para el detalle (`server/queries/cash.sql`, con `// ponytail:` sobre la falta de índice `(register_id, id)`), `make sqlc`, y campos `openingReason`, `openingReasonNote`, `explainedByNext`, `previousSessionId`, `openedByName`/`closedByName` en `SessionHistoryRow`/`SessionDetailView` (`server/internal/app/backoffice.go`). T014 pasa
- [X] T016 [US2] Test MCP en `server/internal/integration/mcp_tools_test.go` (o el archivo de pruebas MCP existente): `cash_sessions_history` devuelve `openingReason` y `openingReasonNote` (no los borra `withoutCustomerText`)
- [X] T017 [US2] Detalle del corte en `CashPage.tsx`: «El turno» (abrió, cerró, fondo que se dejó, fondo fijo, retiro y quién, motivo al abrir) en lista a 1024 y rejilla desde 1366, aviso «Diferencia explicada al abrir el turno #N» con enlace, y marcas en la lista; test en `CashPage.test.tsx` (Maquetas M1)

### US6 — Tema de un toque (P3)

- [X] T018 [P] [US6] Test `web/src/app/TemaRapido.test.tsx`: alterna claro↔oscuro sobre `resolvedTheme`, escribe la llave `theme` de `localStorage`, y el botón mide ≥ 48 px
- [X] T019 [P] [US6] `web/src/app/TemaRapido.tsx` en la franja fija de `web/src/app/AppShell.tsx` (arriba del usuario y `SystemInfo`) (Maquetas M5)
- [X] T020 [P] [US6] Aviso «Falta entregar» (`CashPage.tsx` ~1921) con tokens semánticos que contrasten ≥ 4.5:1 en los dos temas, y test de contraste

### Cierre de la entrega 1

- [X] T021 e2e `web/e2e/caja-diferencia.spec.ts` a 1024×600: cerrar con el cajón corto y verificar la misma cifra en diálogo, lista y detalle; abrir el siguiente con motivo y verla en el corte anterior; botón de tema
- [X] T022 Anotar en `docs/mecanica/caja-y-cobro.md` la regla única de diferencia y el enlace del motivo de apertura
- [X] T023 `/revision-de-codigo` sobre el diff de la entrega; cada hallazgo con su test antes del arreglo
- [ ] T024 `make ci-local` verde; PR «082 fase 1» a `develop`, CI verde, al día, merge

---

## Phase 4: Entrega 2 — Partir sin cambiar comportamiento (US7) · PR «082 fase 2»

- [X] T025 [US7] Correr la red completa (T002) en `develop` actualizado y guardar los conteos «antes»
- [X] T026 [US7] Partir `server/queries/cash.sql` en `cash_sessions.sql`, `cash_counts.sql`, `cash_movements.sql` y `cash_history.sql` sin cambiar ninguna consulta; `make sqlc` y `sqlc diff` vacío salvo el nombre de los archivos generados
- [X] T027 [US7] Crear `CashShiftService` (`server/internal/app/cash_shift.go`), `CashMovementService` (`cash_movements.go`) y `CashReportService` (`cash_report.go`) moviendo los métodos de `backoffice.go` listados en research.md R5, con sus dependencias como campos y sin interfaces; cablearlos en `cmd/api` y en `Handlers`
- [X] T028 [US7] Mover los handlers de caja a `server/internal/httpapi/handlers_cash.go` llamando a los servicios nuevos; rutas y respuestas iguales; `go build ./... && go test ./...`
- [X] T029 [P] [US7] Test vitest de rutas `web/src/features/caja/rutas.test.tsx`: `/caja/historico/:corteId` abre ese corte, recargar en Histórico sigue en Histórico, `/caja/administrar` sin `cash.configure` no muestra los bloques de caja, la dirección vieja de ajustes lleva a `/caja/administrar`
- [X] T030 [US7] Partir `CashPage.tsx` en `web/src/features/caja/` (`CajaPage`, `turno/`, `arqueo/`, `conciliacion/`, `movimientos/`, `reporte/`, `administrar/`) con rutas de [contracts/rutas-web.md](contracts/rutas-web.md) en `App.tsx` y `app/routes.ts`; mover los tests con su componente cambiando solo imports y router
- [X] T031 [US7] Mover `AjustesDeCaja` a `web/src/features/caja/administrar/` junto a las cajas, cada bloque con su permiso; en `/negocio` dejar un enlace (Maquetas M5)
- [X] T032 [US7] Correr la red completa «después» y comparar con T025: mismos casos, mismos resultados; anotar los conteos en el PR
- [X] T033 `/revision-de-codigo`; hallazgos con su test
- [ ] T034 `make ci-local` verde; PR «082 fase 2» a `develop`, CI verde, al día, merge

---

## Phase 5: Entrega 3 — Correcciones con rastro (US3) · PR «082 fase 3»

- [X] T035 [US3] Reconfirmar la siguiente migración libre contra `origin/develop`, todas las ramas remotas y los worktrees `../egb-*`; renombrar 0113 si cambió
- [X] T036 [P] [US3] Test table-driven en `server/internal/domain/cash_adjustment_test.go`: kinds y reasons válidos, `CanAdjustSession` (solo cerrada), `CanReverseAdjustment` (no reverso de reverso, no dos veces), vigente por tipo con reversos, `after` igual al efectivo rechazado, nota vacía/>200 rechazada, conteo sin arqueo firmado → `NO_SIGNED_COUNT`
- [X] T037 [US3] `server/internal/domain/cash_adjustment.go` hasta que T036 pase; sentinels `ErrSessionNotClosed`, `ErrAdjustmentReversed`, `ErrNoSignedCount` mapeados en `httpapi/respond.go`
- [X] T038 [US3] Test de la migración `server/internal/integration/migracion_0113_test.go` sobre el respaldo restaurado con dos empresas: sube, la API anterior sigue cerrando y leyendo cortes, el Down se niega con filas y baja sin ellas; renglón en `guardedDownCleanup` (`harness_restaurado_test.go`)
- [X] T039 [US3] Correr el subagente `db-architect` sobre la migración escrita **antes** de aplicarla
- [X] T040 [US3] `server/migrations/0113_cash_session_adjustments.sql` según [data-model.md](data-model.md) (checks, FK compuestas `no action`, FK del reverso por `(company_id, session_id, id)`, índice único parcial de `reverses_id`, RLS con `nullif`, `grant select, insert`); T038 pasa
- [X] T041 [US3] Respaldo anónimo: `note` anonimizada en el paso 5 y contada en `FUGAS` de `scripts/respaldo-anonimo.sh`; `kind` y `reason` en `noPersonalData` (`server/internal/integration/respaldo_anonimo_test.go`); `TestEveryColumnIsCoveredByTheAnonymousBackup` y `TestEveryCompanyTableIsIsolated` pasan
- [X] T042 [US3] Test de integración `server/internal/integration/cash_adjustments_test.go` bajo `appRoleStore` con `inTheThreeCases`: corregir conteo y fondo, nota, revertir, reverso doble concurrente → uno gana y otro `ALREADY_REVERSED`, reverso con id de otro corte → 404, corte abierto → 409, cajero → 403, la lista y el detalle usan la diferencia corregida y el conteo firmado no cambia, corregir el corte anterior de una caja con turno abierto no cambia lo que ese turno espera, bitácora `cash.session_adjusted`/`cash.session_adjustment_reversed`
- [X] T043 [US3] Consultas `server/queries/cash_adjustments.sql`, `CashAdjustmentService` (`server/internal/app/cash_adjustments.go`, corte `FOR UPDATE`, `before_amount` del servidor, auditoría en la misma `q`), acciones nuevas en `domain/audit.go`, endpoints y rutas con `RequirePermission(cash.adjust_count)`; `CutDifference` recibe el conteo efectivo; T042 pasa
- [X] T043b [US3] Test de integración `TestThePeriodDifferenceAddsEveryCutOfTheFilter` y suma del periodo en el servidor: `GET /cash-sessions/` devuelve `periodDifference` calculada en Go con `CutDifference` y la corrección vigente sobre **todos** los cortes del filtro (mismo `where` que `ListSessions`/`CountSessions`, no solo la página); el test declara que excluye ventas, propinas y fondo, cuenta cortes de dos páginas y oculta la suma con arqueo ciego sin `cash.configure` (`server/queries/cash_history.sql`, `server/internal/app/cash_report.go`)
- [X] T044 [US3] Pantalla: hoja «Corregir» y corrección vigente con «Revertir» en el detalle, marca «Corregido» en la lista, suma del periodo corregida (P4) en `web/src/features/caja/reporte/`, con tests vitest (Maquetas M4, M1)
- [X] T045 [US3] e2e `web/e2e/caja-correcciones.spec.ts`: corregir, ver original y corregido, revertir, segundo reverso rechazado, cajero sin la acción
- [X] T046 Anotar en `docs/mecanica/caja-y-cobro.md` las correcciones (qué no reescriben, quién, rollback hacia adelante tras la 0113)
- [ ] T047 `/revision-de-codigo` (incluye `security-auditor` por los permisos); hallazgos con su test
- [ ] T048 `make ci-local` verde; PR «082 fase 3» a `develop`, CI verde, al día, merge

---

## Phase 6: Entrega 4 — Asistente de cierre, X/Z y anchos (US4, US5) · PR «082 fase 4»

### US4 — Cerrar por pasos (P2)

- [ ] T049 [US4] Test `web/src/features/backoffice/cierreDeCaja.test.ts`: `pendientesDelCierre` devuelve una entrada con clave, texto y acción por cada una de las 9 condiciones del botón (R4) más las 2 de asistencia; el botón se habilita si y solo si la lista está vacía; el estado de cada paso (gris, naranja con cuenta, verde) sale de la misma lista
- [ ] T050 [US4] `pendientesDelCierre` en `web/src/features/backoffice/cierreDeCaja.ts` (o su lugar tras la fase 2) hasta que T049 pase
- [ ] T051 [US4] Test vitest del asistente: borrador en el aparato sobrevive a recargar; si la caja ya cerró en otra tableta, el borrador se descarta avisando; el aviso de asistencia no esconde el conteo; con arqueo ciego el último paso no muestra diferencia ni ofrece explicación
- [ ] T052 [US4] Asistente de cierre (Maquetas M3, opción B) en `web/src/features/caja/turno/asistente/`: pasos al costado con su estado, conteo en 3 columnas desde 840 px con filas de 60 px y −/+ de 52 px (tocar el número abre `NumericKeypad`, FR-037), otros métodos y terminales, último paso con fondo, retiro y explicación; ruta `/caja/:cajaId/cerrar`
- [ ] T053 [US4] Turno abierto (Maquetas M2, opción B): Operar a la izquierda con las 5 acciones en una fila y «Cómo va el turno» fijo a la derecha con «Cerrar caja» al pie; sin título «Caja»; test de que «Cómo va el turno» no tiene efectos
- [ ] T054 [US4] Test de integración `TestHowTheShiftIsGoingHidesTheDrawerWhenBlind` y apertura con conteo distinto mostrando su paso (conteo → motivo → confirmación) en `ContadorDeEfectivo`/`MotivoDeApertura` con su test vitest
- [ ] T055 [US4] Test de la migración `server/internal/integration/migracion_0114_test.go` y test de integración de tolerancia: `GET/PUT /settings/cash-close` (vacío, negativo y no válido → 400; cajero → 403 en PUT), `needsExplanation` solo para `cash.configure`, la lectura de la tolerancia bajo `appRoleStore` con `inTheThreeCases`, explicación al cerrar guardada como nota en la misma transacción, 403 si se manda con arqueo ciego sin permiso
- [ ] T056 [US4] `db-architect` sobre 0114 antes de aplicarla; `server/migrations/0114_cash_difference_tolerance.sql`; `domain.CutNeedsExplanation` con su test table-driven; servicio y rutas de tolerancia; explicación en `CloseSession`; T055 pasa
- [ ] T057 [US4] Tolerancia en Caja → Administrar y marca «Por explicar» en el Histórico (Maquetas M5, M1)

### US5 — Pantallas grandes (P2)

- [ ] T058 [US5] Breakpoints `3xl: 1440px` y `4xl: 2560px` en el tema (`web/src/components/ui/provider.tsx`) sin mover los existentes; test que verifica que los anteriores no cambiaron
- [ ] T059 [US5] Caja sin tope de 920 px: Histórico en 2/3/4 columnas (lista 340 px a 1024), turno con cajas abiertas lado a lado en 2560, texto corrido con `maxW="70ch"` (Maquetas M1, M2)
- [ ] T060 [US5] e2e `web/e2e/caja-anchos.spec.ts` a 768×1024, 1024×600, 1366×768, 1920×1080 y 2560×1440: sin scroll horizontal, controles ≥ 48 px, contenido ≥ 85 % del ancho útil en 1920 y 2560, lista del Histórico con nombre, Abrió, Cerró y «Ver» completos en 1366; capturas en `test-results/`
- [ ] T061 Prueba con usuario nuevo (subagente sin contexto) a 1024×600 y en 1920 y 2560 contra el `make start` del worktree: cerrar una caja con un pendiente y una diferencia, corregir un corte, encontrar por qué un corte tiene diferencia; capturas en `test-results/`; lo que encuentre se corrige con su test
- [ ] T062 Anotar en `docs/mecanica/caja-y-cobro.md` el asistente, la tolerancia y «Por explicar»
- [ ] T063 `/revision-de-codigo` (con `tablet-ui-reviewer`); hallazgos con su test
- [ ] T064 `make ci-local` verde; PR «082 fase 4» a `develop`, CI verde, al día, merge

---

## Phase 7: Release y cierre

- [ ] T065 PR de release `develop` → `main` (comprobar antes que no haya otro release abierto), CI verde, merge, seguir el deploy hasta `GET https://api.elgatobobah.com/readyz` → ready; solo salud (healthz, readyz, job de deploy, journal sin 5xx)
- [ ] T066 `make cerrar-worktree datos=1` desde el worktree; borrar la rama remota con `gh api -X DELETE repos/{owner}/{repo}/git/refs/heads/082-rediseno-de-caja`
- [ ] T067 Comentar y cerrar #290 con lo que entró por fase y lo que quedó fuera (conciliación contra el banco, editar arqueos firmados, reabrir, cuenta contable)

---

## Dependencies & Execution Order

- Setup (T001–T002) → Foundational (T003–T005) → Entrega 1 → Entrega 2 → Entrega 3 → Entrega 4 → Release.
- Las entregas van en serie: cada PR se mergea antes de empezar la siguiente (orden del issue). La 2 mueve los archivos que tocan la 3 y la 4.
- Dentro de una entrega: test → código → pantalla → e2e → revisión → PR.
- US1 antes que US2 (comparten `ListSessions`); US6 es independiente.

## Parallel Opportunities

- Entrega 1: T009/T010 (front de diferencia), T013, y T018–T020 (tema) en paralelo con T006–T008 (servidor).
- Entrega 3: T036 (dominio) en paralelo con T038 (test de migración).
- Entrega 4: T049–T050 en paralelo con T055–T056 (tolerancia en servidor) y T058.

## Implementation Strategy

MVP = Entrega 1 (US1 + US2): resuelve las cifras que se contradicen y la nota que nadie veía, sin
esquema. Cada entrega siguiente se sube sola y deja el sistema coherente.
