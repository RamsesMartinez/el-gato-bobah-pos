# Tasks: El aviso de versión nueva no tapa la pantalla

**Input**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). Cada test se escribe antes de su código y se ve
fallar por la razón correcta.

Todas las tareas de pantalla implementan la fila «Riel lateral con versión nueva — A · en lugar del
logo» de la sección «Maquetas» de plan.md.

## Phase 1: Setup

- [X] T001 Confirmar el entorno local del worktree: `make start` arriba (web :3064, API :8064), y
  credenciales del admin con `make reset-admin` para la e2e (no se versionan).

## Phase 2: Foundational

- [X] T002 Test en `web/src/shared/pwa/registerPwa.test.ts`: con `virtual:pwa-register` simulado,
  disparar `onNeedRefresh` y comprobar que `useAppUpdate.needRefresh` queda en `true`, que
  `apply()` llama `updateSW(false)` y que **no** se crea ningún toast con título «Nueva versión
  disponible» (FR-005). Verlo fallar por el toast.
- [X] T003 Quitar el `toaster.create` de versión en `web/src/shared/pwa/registerPwa.ts` (el de
  «Instalar» se queda) y actualizar el comentario de `injectRegister` en `web/vite.config.ts` y el
  de `web/src/stores/appUpdate.ts`, que hablan del toast. T002 en verde.
- [X] T004 Gancho solo de desarrollo en `web/src/shared/pwa/registerPwa.ts`: bajo
  `import.meta.env.DEV`, `window.__egbSimularVersionNueva = () => markNeedRefresh(...)` con un
  `apply` que solo recarga (research, Decisión 4). Su declaración de tipo en
  `web/src/vite-env.d.ts` o junto al gancho.

## Phase 3: User Story 1 + 2 — el aviso en el lugar del logo, visible y de un toque (P1)

Las dos historias son el mismo componente: US1 dice dónde (no tapa, no quita alto) y US2 qué hace
(se ve, un toque, persiste).

**Independent Test**: quickstart.md, «Disposición a 1024×600».

### Tests

- [X] T005 [P] [US2] Test en `web/src/app/UpdateRailButton.test.tsx`: sin `needRefresh` no pinta
  nada; con `needRefresh` pinta un botón con nombre «Actualizar a la versión nueva», texto visible
  «Actualizar», `min-height` ≥ 44 px, y un toque llama `apply` una vez (FR-004, US2-2).
- [X] T006 [P] [US1] Tests en `web/src/app/AppShell.test.tsx` (quitar el mock de `SystemInfo` solo
  si estorba; el botón se prueba montando el shell): sin versión nueva el logo «El Gato Bobah» está y
  no hay botón «Actualizar a la versión nueva» (FR-006); con versión nueva el botón está y el logo
  no (FR-001); al prender `needRefresh` se vuelve a llamar `railScrollFor` (FR-007, plan «Cómo
  queda»).
- [X] T007 [P] [US2] Test en `web/src/app/SystemInfo.test.tsx`: con `needRefresh` no hay punto
  verde junto a la versión, y el diálogo «Detalles del sistema» sigue ofreciendo «Actualizar ahora».
- [X] T008 [US1] E2E en `web/e2e/aviso-de-version.spec.ts`, viewport 1024×600 y `hasTouch`, rol
  admin, contra `make start`: en Vender (`/pos`), Pedidos (`/pedidos`), Negocio (`/negocio`, con
  «Agregar lugar» a la vista) y Caja (`/caja`) — medir la caja del área de trabajo, llamar
  `window.__egbSimularVersionNueva()`, y comprobar: (a) el botón «Actualizar a la versión nueva»
  visible y ≥ 44 px; (b) el área de trabajo mide lo mismo al píxel (SC-002); (c) ningún botón visible
  del área de trabajo se cruza con la caja del botón (SC-001); (d) la lista del riel mide lo mismo
  que sin aviso y la opción activa y «Salir» están completas dentro del viewport (FR-007);
  (e) cambiar de pantalla dos veces y el botón sigue (US2-1). Capturas solo en `test-results/`.

### Implementation

- [X] T009 [US2] Crear `web/src/app/UpdateRailButton.tsx`: lee `useAppUpdate`; sin `needRefresh`
  devuelve `null`; con él, `Button` del tema `colorPalette="green"`, `w="64px"`, `minH="44px"`,
  `px={0}`, apilado: icono `LuCircleArrowUp` (18 px) y «Actualizar» a 10 px;
  `aria-label="Actualizar a la versión nueva"`; `onClick={apply}`. T005 en verde.
- [X] T010 [US1] En `web/src/app/AppShell.tsx`: si `needRefresh`, pintar `UpdateRailButton` en el
  lugar del logo con el mismo `mb={2}` y `alignSelf="center"`; si no, el logo de hoy. Agregar
  `needRefresh` a las dependencias del efecto que lleva la opción activa a la vista y del que mide
  «hay más». T006 en verde.
- [X] T011 [US2] En `web/src/app/SystemInfo.tsx`: quitar el `Circle` verde y su import; actualizar
  el comentario del componente. T007 en verde.
- [X] T012 [US1] Correr T008 contra `make start` y dejarlo en verde.

## Phase 4: Polish

- [X] T013 [P] Actualizar el comentario de `web/src/app/PantallaQueNoSeCae.tsx` que dice que el aviso
  de versión lo pinta el toaster.
- [ ] T014 Vuelta con usuario sin contexto (subagente) a 1024×600 en Vender con versión nueva
  esperando: ¿encuentra y aplica la actualización a la primera?, ¿qué cree que hace el botón?
  (SC-003). Lo observado, en texto en el reporte.
- [X] T015 Build servido: `bun run build && bun run preview` en `web/`, comprobar que
  `window.__egbSimularVersionNueva` es `undefined` y que el riel sin aviso se ve igual (quickstart).
- [X] T016 Gates: `bun run lint`, `bun run vitest run`, `bun run build` en `web/`; `make ci-local`
  antes de empujar.

## Dependencies

- T002 → T003 → T004. T004 antes de T008/T012.
- T005, T006, T007 en paralelo; cada uno antes de su implementación (T009, T010, T011).
- T009 antes de T010. T012 después de T009–T011.
- Phase 4 después de Phase 3.

## Parallel example

T005, T006 y T007 juntos (archivos distintos); luego T009 y T011 juntos.

## Implementation Strategy

Una sola entrega: las dos historias P1 son el mismo botón. MVP = Phase 2 + Phase 3.
