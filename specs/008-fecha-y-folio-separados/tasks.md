---

description: "Task list — 008 · La fecha la da el reloj, el folio lo da el turno"
---

# Tasks: La fecha la da el reloj, el folio lo da el turno

**Input**: Design documents from `specs/008-fecha-y-folio-separados/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md), [quickstart.md](./quickstart.md)

**Tests**: Obligatorios. El principio IV de la constitución es no negociable: primero el test que
falla, luego el código. Cada tarea de prueba dice **qué defecto atrapa**; una que no atrape ninguno
sobra.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: puede correr en paralelo (archivo distinto, sin dependencias pendientes)
- **[Story]**: a qué historia pertenece

---

## Orden de fases: por qué US2 va antes que US1

US1 (fecha) es P1 y US2 (folio) es P2, pero **US2 se implementa primero**. Si la fecha se suelta
mientras el contador sigue colgado de `business_date`, el folio empieza a reiniciarse a medianoche
y se reintroduce el defecto de los dos tickets #1 en la misma noche. La prioridad dice qué importa
más; la dependencia dice qué va antes.

---

## Phase 1: Setup

- [X] T001 Confirmar la línea base en verde antes de tocar nada: `cd server && go build ./... && go test ./...` y `cd web && bun run lint && bun run vitest run`
- [X] T002 Leer [server/migrations/0058_bolsa_de_folios.sql](../../server/migrations/0058_bolsa_de_folios.sql) y `FolioNamesConsumidos` / `MarcarFolioConsumido` en [server/queries/orders.sql](../../server/queries/orders.sql): la bolsa de nombres tiene su propio estado y hay que saber cuál de sus dos consultas cuelga de la fecha antes de moverla

---

## Phase 2: US2 — El folio lo da el turno (Priority: P2, va primero por dependencia)

**Goal**: El número y el nombre de un pedido se reparten dentro del turno, sin consultar la fecha.

**Independent Test**: Con un turno que cruza la medianoche, la numeración sigue corrida y ningún
nombre se repite entre pedidos vivos.

### Pruebas primero

- [X] T003 [US2] Test de integración en `server/internal/integration/` — **atrapa**: el folio que se parte a medianoche. Turno abierto a las 23:00, venta a las 23:50 y otra a las 00:10; la segunda recibe el número siguiente, no el 1. Debe fallar antes de existir `folio_counters`
- [X] T004 [P] [US2] Test de integración de concurrencia — **atrapa**: perder el candado de fila al cambiar de tabla contadora. Dos transacciones creando venta en el mismo turno reciben números distintos y consecutivos
- [X] T005 [P] [US2] Test de integración bajo el rol `gatobobah_app` (no owner) — **atrapa**: el grant que falta, invisible en local y `42501` en el primer request de producción
- [X] T006 [P] [US2] Test de integración de aislamiento RLS entre dos empresas sobre `folio_counters` — **atrapa**: una fuga entre empresas que no se ve hasta que hay un segundo cliente
- [X] T007 [US2] Test de integración de la semilla — **atrapa**: el turno abierto con 158 pedidos repartiendo el número 1 después de migrar. Base con turno abierto numerado hasta N; tras la migración, la siguiente venta recibe N+1
- [X] T008 [P] [US2] Test de integración de reversibilidad de 0061: `Up → Down → Up` deja la base igual
- [X] T008a [US2] Test de integración de cerrar y reabrir el mismo día — **atrapa**: que alguien afloje la regla de "no se cierra con pedidos vivos" y convierta el reinicio del folio en una colisión real. Es la prueba de la **premisa** sobre la que descansa toda la decisión de numerar por turno, no del código nuevo: (1) turno con un pedido `abierta` → cerrar falla; (2) terminar el pedido → cerrar funciona; (3) turno nuevo el mismo día → primera venta recibe folio 1 y no hay ningún pedido vivo con folio 1
- [X] T008b [P] [US2] Test de integración del alcance del **nombre** (FR-004) — **atrapa**: que el número se mueva al turno y el nombre se quede colgado de la fecha, dejando los dos caminos a medio separar. Dos pedidos vivos del mismo turno nunca comparten nombre; dos turnos distintos del mismo día sí pueden repetirlo, y eso es correcto

### Implementación

- [X] T009 [US2] Escribir `server/migrations/0061_folio_por_turno.sql` con la tabla `folio_counters` según [data-model.md](./data-model.md): PK `(company_id, register_session_id)`, FK a `register_sessions` sin cascade, política `tenant_isolation`, `grant select, insert, update` explícito al rol de la aplicación, y la semilla desde `orders`. Comentar en la propia migración por qué `order_counters` se deja en pie
- [X] T010 [US2] Correr el subagente `db-architect` sobre 0061 **antes** de aplicarla — regla del repo para toda migración nueva
- [X] T011 [US2] Reemplazar `NextDailyNumber` por `NextFolioNumber` en [server/queries/orders.sql](../../server/queries/orders.sql), con el upsert sobre `folio_counters` arbitrando por el nombre de la PK, y su comentario explicando que el candado de fila es lo que serializa
- [X] T012 [US2] Reemplazar `FolioNamesUsedToday` por su versión por turno en [server/queries/orders.sql](../../server/queries/orders.sql), y ajustar la bolsa de nombres de T002 si también cuelga de la fecha
- [X] T013 [US2] `make sqlc` y verificar que compila
- [X] T014 [US2] En [server/internal/app/orders.go](../../server/internal/app/orders.go), numerar con el turno: `NextFolioNumber(sess.ID)` y `resolverFolio` recibiendo el turno en vez de la fecha
- [X] T015 [US2] Reescribir el comentario de `bizDate` en orders.go: hoy explica la herencia de fecha y va a dejar de ser cierto. Tiene que decir por qué ahora son dos caminos independientes

**Checkpoint**: T003–T008 en verde. El folio ya no lee la fecha.

---

## Phase 3: US1 — La fecha la da el reloj (Priority: P1)

**Goal**: Una venta se archiva con el día en que ocurrió, en la zona del negocio.

**Independent Test**: Con un turno abierto de hace cuatro días, una venta de hoy aparece en Ventas
de hoy.

### Pruebas primero

- [X] T016 [US1] Test de integración — **atrapa**: el defecto reportado. Turno con `business_date` de hace cuatro días, venta creada hoy: su `business_date` es hoy y su `register_session_id` sigue siendo el del turno
- [X] T017 [P] [US1] Test de integración — **atrapa**: la fecha resuelta en UTC. Venta a las 19:00 locales cae en ese día y no en el siguiente
- [X] T018 [P] [US1] Test de integración — **atrapa**: caer a la zona del navegador o a UTC cuando no hay ajustes. Empresa sin fila de configuración usa el default del producto
- [X] T018a [P] [US1] Test de integración con una zona guardada que dejó de ser válida — **atrapa**: la venta que se archiva en UTC en silencio, seis horas corrida y perfectamente plausible. Cae al default del producto y la venta se cobra igual
- [X] T018b [P] [US1] Test unitario en `domain` con `America/Tijuana` el día del cambio de horario — **atrapa**: un cálculo que reste 24 horas en vez de preguntarle a la zona. Ese día la distancia entre dos medianoches es de 23 o 25 horas
- [X] T018c [P] [US1] Test de regresión de FR-015 y SC-007 — **atrapa**: el hermano que no se movió. Al cambiar de dónde sale la fecha, la rama que exige turno abierto hereda cero protecciones: sin turno, crear la venta sigue fallando con `ErrNoOpenRegister`, y con turno sigue funcionando
- [X] T019 [P] [US1] Test unitario en `domain` del helper de "turno de otro día", con el turno abierto ayer a las 23:00 que lleva una hora abierto. Va en `domain` y no en el servicio: es lógica pura sin I/O, y el principio I no lo deja a elección

### Implementación

- [X] T020 [US1] Agregar a `OrdersService` la resolución de zona con el mismo patrón que `SalesService.Location` en [server/internal/app/sales.go](../../server/internal/app/sales.go)
- [X] T021 [US1] En [server/internal/app/orders.go](../../server/internal/app/orders.go), sustituir `bizDate := sess.BusinessDate` por `domain.BusinessDate(s.now(), loc)`
- [X] T022 [US1] Corregir el comentario del bloque de modos de corte en [server/internal/domain/businessdate.go](../../server/internal/domain/businessdate.go), que afirma "el día al que pertenece una venta lo decide el turno" y deja de ser cierto

**Checkpoint**: US1 entregable por sí sola. Es el MVP.

---

## Phase 4: Corrección histórica (FR-007, FR-008)

**Goal**: La fecha de una venta significa lo mismo en todo el histórico.

- [X] T023 Test de integración contra una base restaurada de un respaldo real y **con al menos dos empresas** — **atrapa**: una corrección que reescribe más de lo que dice. Guarda las cifras de cada arqueo cerrado, migra, y compara fila por fila; `daily_number`, `folio_name` y `register_session_id` sin un solo cambio
- [X] T024 [P] Test de reversibilidad de 0062: tras el `Down`, las fechas vuelven a ser las de antes
- [X] T025 Escribir `server/migrations/0062_fecha_de_venta_del_reloj.sql`: tabla `orders_business_date_fix` con los valores previos, `update` de las filas que difieren, y `Down` que restaura desde el respaldo y borra la tabla
- [X] T026 Correr `db-architect` sobre 0062 antes de aplicarla
- [X] T027 Ensayar las dos migraciones contra una copia restaurada de los datos reales de producción y anotar cuántas filas cambian por empresa, para compararlo contra lo medido (0 de 31 en el negocio en operación)

---

## Phase 5: US3 — Las ventas de un corte (Priority: P3)

**Goal**: El detalle de un corte muestra las ventas que ese corte cobró.

**Independent Test**: Abrir un corte cerrado y ver sus ventas, con la cuenta y el total, sin que
aparezca ninguna de otro corte.

### Pruebas primero

- [X] T028 [US3] Test de integración — **atrapa**: una lista y un resumen de la misma pantalla derivados de predicados distintos. Corte con entregadas, una cancelada y una reembolsada: `salesCount` las cuenta todas, `salesTotal` excluye las que no dejaron ingreso y las propinas
- [X] T029 [P] [US3] Test de integración — **atrapa**: ventas de otro corte coladas en la lista
- [X] T030 [P] [US3] Test de front en `web/src/features/backoffice/CashPage.test.tsx` — **atrapa**: el recorte silencioso que se lee como "esto es todo". Con `salesCount` mayor que `salesShown`, la pantalla dice cuántas hay en total
- [X] T031 [P] [US3] Test de front del corte sin ventas: una frase, no una tabla vacía

### Implementación

- [X] T032 [US3] Agregar `SessionSales` y `CountSessionSales` a [server/queries/cash.sql](../../server/queries/cash.sql), con el **mismo `where`** y filtrando por `register_session_id`, más el comentario de por qué no es por ventana de tiempo
- [X] T033 [US3] `make sqlc`
- [X] T034 [US3] Extender `SessionDetail` en [server/internal/app/backoffice.go](../../server/internal/app/backoffice.go) con `sales`, `salesCount`, `salesShown` y `salesTotal`, respetando el tope `MaxListLimit`
- [X] T035 [US3] Tipos del contrato en [web/src/api/pos.ts](../../web/src/api/pos.ts)
- [X] T036 [US3] Sección "Ventas del corte" en `CorteDetail` de [web/src/features/backoffice/CashPage.tsx](../../web/src/features/backoffice/CashPage.tsx), con su encabezado declarando qué incluye y qué excluye el total
- [X] T037 [US3] Correr `tablet-ui-reviewer` sobre el detalle del corte a 1024×600: contar cuántos renglones le quita la sección nueva al resumen, los gastos y lo declarado por método

---

## Phase 6: US4 — El aviso de turno viejo (Priority: P2)

**Goal**: Quien opera ve que su turno abierto ya no es de hoy, sin que eso le impida cobrar.

**Independent Test**: Con un turno abierto de ayer, el aviso aparece y cobrar sigue funcionando.

### Pruebas primero

- [X] T038 [US4] Test de integración de `cash-status` — **atrapa**: comparar horas transcurridas en vez de días. Turno abierto ayer a las 23:00 devuelve `deOtroDia: true` aunque lleve una hora
- [X] T039 [P] [US4] Test de integración: turno abierto hoy devuelve `deOtroDia: false`; sin turno abierto, `open: false` y `deOtroDia: false`
- [X] T040 [P] [US4] Test de front — **atrapa**: un aviso que bloquea el cobro. Con el aviso visible, el camino de cobrar sigue disponible

### Implementación

- [X] T041 [US4] Extender `SellingRegisterOpen` en [server/internal/app/backoffice.go](../../server/internal/app/backoffice.go) para devolver también cuándo abrió el turno y si su día ya no es hoy, resuelto **en el servidor** con la zona del negocio
- [X] T042 [US4] Devolver los campos nuevos en `CashStatus` de [server/internal/httpapi/handlers_backoffice.go](../../server/internal/httpapi/handlers_backoffice.go), según [contracts/api.md](./contracts/api.md)
- [X] T043 [US4] Tipos en [web/src/api/pos.ts](../../web/src/api/pos.ts), tratando los campos ausentes como "sin aviso" para que el front que deploya antes que el backend no se rompa
- [X] T044 [US4] Pintar el aviso con la acción de ir a cerrar el turno, sin comerle ancho a la barra del POS, que ya no tiene ancho libre
- [X] T045 [US4] Correr `tablet-ui-reviewer` sobre el aviso

---

## Phase 7: Cierre

- [X] T046 Actualizar [docs/matriz-de-pantallas.md](../../docs/matriz-de-pantallas.md) y [docs/matriz-de-cobro.md](../../docs/matriz-de-cobro.md) con los casos nuevos y con lo que queda **sin** cubrir
- [ ] T047 Correr `go-backend-reviewer` sobre los cambios de `server/`
- [X] T048 Gates completos: `go build ./... && go test ./...`, `bun run lint`, `bun run vitest run`, `bun run build`
- [X] T049 Suite e2e a 1024×600 contra el ambiente desplegado, cerrando y cobrando todo pedido que cree
- [X] T050 Verificar en dev, con datos reales: el turno viejo del 31-ago se cierra, se abre uno nuevo, y una venta de hoy aparece en Ventas de hoy

---

## Dependencies

```text
Phase 1 (Setup)
   └─> Phase 2 (US2 · folio por turno)      ← bloquea a US1
          └─> Phase 3 (US1 · fecha del reloj)
                 └─> Phase 4 (corrección histórica)

Phase 5 (US3 · ventas del corte)   ─┐
Phase 6 (US4 · aviso de turno)     ─┴─> independientes entre sí y de 2–4

Phase 7 (cierre) tras todo lo anterior
```

- Las fases 5 y 6 pueden correr en paralelo con la cadena 2→3→4: no comparten archivo con ella
  salvo `pos.ts`, que se toca en tareas distintas.
- Dentro de cada fase, las tareas marcadas `[P]` tocan archivos distintos.

## MVP

**Phase 2 + Phase 3.** Con eso, una venta de hoy aparece en Ventas de hoy y el folio no se parte a
medianoche — que es el defecto reportado y su consecuencia. Las fases 4, 5 y 6 son mejoras
entregables por separado.
