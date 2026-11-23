---

description: "Tareas de la feature 089"
---

# Tasks: Vender — canales compactos, más cuentas a la vista y qué le falta a cada una

**Input**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [contracts/ui.md](contracts/ui.md)

**Tests**: TDD (constitución IV): cada test va antes de su código y se ve fallar.

**Idioma** (constitución VII): todo identificador nuevo en inglés (`missingFor`, `useDeliverAccount`,
`AccountsColumn`, `chargeAccount`, `onDeliver`, `onCharge`, `liveAccount`); los archivos que ya
existen en español conservan su nombre; el texto de la pantalla y de las pruebas, en español.

Todas las tareas de pantalla implementan la fila «Vender (parte de arriba y cuentas) · B · Columna de
cuentas» de la sección **Maquetas** de [plan.md](plan.md).

## Phase 1: Setup

- [X] T001 Verificar el `make start` del worktree con la base restaurada (`curl localhost:8089/readyz`) y `bun run vitest run src/features/pos` en verde antes de tocar nada, desde `web/`

## Phase 2: Foundational

**Purpose**: la traducción estado → «lo que falta», que leen las tres historias.

- [X] T002 Test table-driven de `missingFor(c)` para los seis estados (texto largo, corto, color, `cobrar`, `entregar`; monto = `outstanding`; un pedido de $0 o saldado nunca dice «cobrar $0.00»; una cuenta en captura nunca ofrece acciones) en `web/src/domain/cuentas.test.ts`
- [X] T003 Implementar `missingFor` (research R-1) junto a `ESTADO` en `web/src/domain/cuentas.ts`; `estadosDeCuenta.ts` deja de exportar su `loQueFalta` viejo y reexporta el nuevo

**Checkpoint**: T002 en verde.

## Phase 3: User Story 1 — Cada cuenta dice qué le falta (P1)

**Independent test**: con cuentas en los cinco estados vivos, cada renglón y ficha dice lo de R-1.

- [X] T004 [P] [US1] Test: cada renglón de la hoja dice «Falta …» según el estado y no repite el estado interno, en `web/src/features/pos/TodasLasCuentasSheet.test.tsx` (ajusta «cada renglón dice el estado y lo que falta» y «una cerrada con algo nuevo…»)
- [X] T005 [P] [US1] Test: la ficha de la fila angosta dice la forma corta («Cobrar $180 · entregar», «Falta entregar») y su `aria-label` es «{nombre} · {texto largo}», en `web/src/features/pos/FilaDeCuentas.test.tsx`
- [X] T006 [P] [US1] Test: el cierre de caja nombra cada cuenta pendiente con el mismo texto, en `web/src/features/caja/` (test existente de `turno.tsx` que cubra «Cuentas pendientes», o uno nuevo junto a él)
- [X] T007 [US1] Usar `missingFor` en `web/src/features/pos/TodasLasCuentasSheet.tsx`, `web/src/features/pos/FilaDeCuentas.tsx` y `web/src/features/caja/turno.tsx` (FR-001, FR-002)

## Phase 4: User Story 2 — Resolver desde la cuenta (P1)

**Independent test**: entregar una pagada desde Vender la quita de la fila; cobrar una entregada que debe abre la hoja de cobro.

- [X] T008 [P] [US2] Test del hook `useDeliverAccount`: llama `posApi.deliverOrder(id)`, invalida `['pos','accounts']` y `['orders']`, no manda dos veces con doble toque (pendiente), y ante error muestra `mensajeDeError` sin internals, en `web/src/features/pos/useDeliverAccount.test.tsx`
- [X] T009 [US2] Implementar `useDeliverAccount` en `web/src/features/pos/useDeliverAccount.ts` (research R-2)
- [X] T010 [P] [US2] Test: la hoja «Todas las cuentas» muestra «Entregar» y «Cobrar $X» solo donde aplican (nunca en captura), y tocarlos llama `onDeliver`/`onCharge` sin elegir la cuenta, en `web/src/features/pos/TodasLasCuentasSheet.test.tsx`
- [X] T011 [US2] Botones de entregar/cobrar en `web/src/features/pos/TodasLasCuentasSheet.tsx` (props `onDeliver`, `onCharge`, `delivering`)
- [X] T012 [P] [US2] Test: el ticket de un pedido muestra la franja «Falta …» con «Entregar» cuando le falta entregar, y no la muestra en una cuenta en captura, en `web/src/features/pos/Ticket.test.tsx`
- [X] T013 [US2] Franja «Falta …» bajo el encabezado en `web/src/features/pos/Ticket.tsx` (props `liveAccount?: AccountItem`, `onDeliver`)
- [X] T014 [US2] Test de `POSPage`: cobrar desde la cuenta abre la hoja de cobro de ESE pedido aunque la cuenta abierta sea otra, en `web/src/features/pos/POSPage.test.tsx`
- [X] T015 [US2] Cobrar desde la cuenta en `web/src/features/pos/POSPage.tsx`: `chargeAccount(c)` lee `posApi.order(c.orderId)` y abre la hoja de cobro con `aPedidoParaCobrar` (research R-3); cablear `useDeliverAccount` a la columna, la hoja y el ticket

## Phase 5: User Story 3 — Más cuentas a la vista y canales compactos (P2)

**Independent test**: e2e de medidas a 1024×600 y 1280×854, con y sin franjas.

- [X] T016 [P] [US3] Test de `AccountsColumn`: un renglón por cuenta en orden de antigüedad (estable al elegir), la abierta marcada con `aria-pressed`, «Cuenta nueva» primero cuando la tableta está en una cuenta sin productos, botón «Entregar» solo en pagada sin entregar y «Cobrar» solo en entregada que debe, ningún botón con las dos o en captura, tocar el botón no elige la cuenta, «+ Nueva» pide cuenta nueva, controles ≥ 44 px, `aria-label="Cuentas"` y `data-ficha` en cada renglón, en `web/src/features/pos/AccountsColumn.test.tsx`
- [X] T017 [US3] Implementar `web/src/features/pos/AccountsColumn.tsx` (248 px, lista con `flex="1" minH={0} overflowY="auto"`, desplaza a la vista la abierta) con `Button` del tema
- [X] T018 [P] [US3] Test de `PlatformPicker`: grupo pegado que mide 44 px de alto total (borde incluido) sin renglones de texto debajo con plataforma; el folio en el mismo renglón; la ayuda de mantener presionado detrás de un botón de 44 px, en `web/src/features/pos/PlatformPicker.test.tsx`
- [X] T019 [US3] Canales compactos en `web/src/features/pos/PlatformPicker.tsx` (research R-4)
- [X] T020 [US3] Disposición B en `web/src/features/pos/POSPage.tsx`: renglón de canales + herramientas; en modo ancho la columna a la izquierda de categorías y productos; en modo angosto la fila de hoy; buscador plegado con ticket abierto o con folio. Ajustar `POSPage.test.tsx` a lo que cambie
- [X] T021 [P] [US3] Test de regresión del aviso de agotado: con la franja de entrantes montada, el aviso se coloca debajo de ella, en `web/src/features/notificaciones/FranjaDeAgotado.test.tsx`
- [X] T022 [US3] Corregir el selector a `[data-testid="franja-de-entrantes"]` en `web/src/features/notificaciones/FranjaDeAgotado.tsx`
- [X] T023 [US3] E2E `web/e2e/vender-cuentas.spec.ts`: a 1024×600 y 1280×854 con ticket abierto mide inicio de productos, cuentas completas visibles en la columna (≥ 7 y ≥ 12 con al menos 14 cuentas) y controles ≥ 44 px; lo mismo con la franja de entrantes (52 px) inyectada arriba del área de trabajo y el aviso de agotado encima: la cuenta abierta y una fila completa de productos siguen visibles y sin tapar (FR-009, SC-005); con una pagada creada por API, «Entregar» desde la columna la quita (servidor: `status=entregada`); con una entregada que debe, «Cobrar» abre la hoja de cobro; todo pedido creado se cierra al final
- [X] T024 [US3] Ajustar las e2e que dependen de la fila (`e2e/cabe-en-la-tableta.spec.ts`, `e2e/deuda-de-especificaciones.spec.ts`, `e2e/una-sola-puerta.spec.ts`, helpers de `e2e/pos.ts`) a la columna

## Phase 6: Polish

- [X] T025 Nota del área en `docs/mecanica/caja-y-cobro.md`: «lo que falta» se deriva del estado del servidor; entregar desde Vender; la columna en modo ancho y la fila en angosto
- [ ] T026 `bun run lint`, `bun run vitest run`, `bun run build` en verde, desde `web/`
- [ ] T027 Prueba con alguien nuevo de una fonda a 1024×600 contra el `make start` del worktree (quickstart), corregir lo que encuentre con su test, y cerrar los pedidos de prueba

## Dependencies

- T002→T003 bloquean todo.
- US1 (T004–T007) y US2 (T008–T013) son independientes entre sí; T014 (test) y T015 (código) dependen de T009, T011, T013.
- US3: T017 depende de T003 y T009; T020 de T017 y T019; T023–T024 de T020.
- T027 al final.

## Parallel

- T004, T005, T006 a la vez; T008, T010, T012 a la vez; T016, T018, T021 a la vez.

## Implementation Strategy

MVP = Foundational + US1 + US2 (la cuenta dice qué le falta y se resuelve desde la hoja y el ticket).
Después US3 (la columna elegida). Se sube completo: el dueño pidió las tres cosas.
