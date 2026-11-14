---
description: "Tareas del menú lateral en dos niveles"
---

# Tasks: Menú lateral en dos niveles

**Input**: `specs/085-menu-en-dos-niveles/` (plan.md, spec.md, research.md, data-model.md, contracts/menu.md, quickstart.md)

**Tests**: TDD obligatorio (constitución IV): cada test se escribe y se ve fallar antes de su código.

**Maqueta**: las tareas de pantalla implementan la fila «Riel lateral» de plan.md › Maquetas (opción C ajustada).

## Phase 1: Setup

- [X] T001 Traer develop y confirmar si la ruta de «Se acabó» (#291) ya existe en `web/src/App.tsx`; anotar el resultado en `specs/085-menu-en-dos-niveles/plan.md` › «Se acabó»

## Phase 2: Foundational (bloquea todas las historias)

- [X] T002 Test que falla: lo que ve cada rol sugerido (admin, gerente, cajero, mesero, con y sin asistencia) y un rol propio con solo `sales.view`, según la tabla de spec.md; Menú e Inventario contiguos; Vender y Pedidos primero; `moduleOf` para `/catalogo/productos`, `/plataformas/3/emparejar`, `/caja/administrar`, `/apariencia` en `web/src/app/nav.test.ts`
- [X] T003 Reescribir `web/src/app/nav.ts`: `MENU_MODULES`, `visibleModules(user, { attendanceEnabled })` sobre `canOpen`/`can`, `moduleOf(pathname)`; sin nombres de rol (pasa T002 y `sin-nombre-de-rol.test.ts`)

## Phase 3: User Story 1 — El cajero encuentra lo de todos los días (P1)

**Goal**: Vender, Pedidos y Caja a un toque; riel sin desplazar a 1280×854.
**Independent Test**: como cajero a 1280×854, seis botones, Vender/Pedidos/Caja a un toque.

- [X] T004 [US1] Tests que fallan en `web/src/app/MenuRail.test.tsx`: Vender y Pedidos son enlaces directos con divisoria después de Pedidos; un módulo con un solo destino visible es enlace directo (Caja del cajero); botones de primer nivel con `minHeight` ≥ 72 px; el número de mermas va sobre Inventario con `aria-label`; existe el hueco `[data-bell-slot]` de 44 px junto al tema
- [X] T005 [US1] Implementar `web/src/app/MenuRail.tsx` (primer nivel, divisoria, destino único directo, mermas, hueco de campana) con tokens de `web/src/theme/ui.ts`
- [X] T006 [US1] Montar `MenuRail` en `web/src/app/AppShell.tsx` en lugar de la lista plana, con el pie intacto y el logo/aviso de versión; conservar el desplazamiento, el chevron y `railScrollFor` como red debajo de 854 px; actualizar `web/src/app/AppShell.test.tsx` a la estructura nueva sin perder sus casos (permisos, Asistencia activable, Salir 44 px, aviso de versión, mermas)
- [X] T007 [US1] e2e `web/e2e/menu-por-rol.spec.ts`: a 1280×854 el riel no desplaza con admin y cajero (personas creadas con `web/e2e/personas.ts`), Salir dentro de la ventana; a 1280×768 Salir y el módulo activo visibles

## Phase 4: User Story 2 — Quien administra llega a cada pantalla (P1)

**Goal**: panel por módulo, accesible, y vista expandida en pantalla ancha.
**Independent Test**: como gerente, abrir cada módulo y llegar a Gastos, Asistencia y Ajustes; a 2K todo a la vista.

- [X] T008 [US2] Tests que fallan en `web/src/app/MenuRail.test.tsx`: tocar un módulo con varios destinos abre `<nav>` con sus enlaces (`aria-expanded`, `aria-controls`), foco al primer enlace; elegir navega y cierra; Escape y el velo cierran sin navegar y devuelven el foco; cambiar de ruta cierra; tocar otro módulo cambia de panel; el módulo activo marcado con `/plataformas/3/emparejar`; con `matchMedia` ancho (≥1920×1080) se pintan todos los destinos sin panel
- [X] T009 [US2] Implementar el panel y la vista expandida en `web/src/app/MenuRail.tsx` (posición junto al botón o anclado abajo, `maxH` en dvh con lista desplazable, `matchMedia` con escucha de cambios)
- [X] T010 [US2] e2e en `web/e2e/menu-por-rol.spec.ts`: gerente abre Caja › Gastos, Personal › Asistencia, Ajustes › Interfaz a 1280×854; a 2560×1440 el riel expandido muestra todos los destinos del admin sin desplazar

## Phase 5: User Story 3 — Nada que ya funcionaba se rompe (P1)

**Goal**: cada ruta y redirección de hoy abre igual.
**Independent Test**: URL directa de cada ruta con cada rol que la abre.

- [X] T011 [P] [US3] e2e en `web/e2e/menu-por-rol.spec.ts`: por URL directa, admin abre todas las rutas de la tabla de spec.md y `/productos`, `/opciones`, `/catalogo` redirigen como hoy; `/almacen?tab=conteo` y `/negocio#lugares` abren su pestaña/sección; el cajero abre `/pos`, `/pedidos`, `/caja`, `/merma`, `/apariencia`, `/cuenta`
- [X] T012 [US3] Actualizar los e2e que dependían del riel plano: `web/e2e/aviso-de-version.spec.ts` (navegar por el riel nuevo, a 1280×854), `web/e2e/cabe-en-la-tableta.spec.ts` (enlace Asistencia dentro de Personal), `web/e2e/almacen-claro-oscuro.spec.ts` (Almacén dentro de Inventario), `web/e2e/asistencia-y-auditoria.spec.ts`, `web/e2e/medir-no-estorba.spec.ts`

## Phase 6: User Story 4 — La medición (P2)

**Goal**: Interfaz e Integraciones medidas.
**Independent Test**: abrir ambas y ver el conteo aceptado.

- [X] T013 [P] [US4] Test que falla en `server/internal/domain/uso_test.go`: `EventoDeUsoValido("apariencia","")` e `("integraciones","")` válidos; `ScreenAllowed("integraciones")` exige `integrations.manage`, `apariencia` no exige permiso
- [X] T014 [US4] Agregar `apariencia` e `integraciones` a `pantallasMedibles` y `permissionByScreen` en `server/internal/domain/uso.go`
- [X] T015 [P] [US4] Test que falla y cambio: `pantallaDe('/apariencia')` y `pantallaDe('/integraciones')` en `web/src/app/rutas-medidas.ts` (test junto al archivo); etiquetas «Interfaz» e «Integraciones» en `web/src/consola/etiquetas-de-uso.ts`

## Phase 7: Polish

- [X] T016 Anotar en `docs/mecanica/medicion-de-uso.md` que Interfaz e Integraciones ya se miden
- [X] T017 `bun run lint`, `bun run vitest run`, `bun run build` en `web/`; `go build ./... && go test ./internal/domain/` en `server/`
- [X] T018 `/revision-de-codigo` y un test por cada hallazgo
- [ ] T019 Usuario nuevo a 1280×854 y 2K, como cajero y como gerente de otra fonda: encontrar Pedidos, Gastos, Asistencia y Ajustes sin ayuda; capturas solo en `web/test-results/`

## Dependencies

- T001 → T002 → T003 → (US1, US2, US3, US4)
- US1 (T004–T007) antes de US2 (T008–T010): el panel vive en el mismo componente
- US3 y US4 son independientes de US1/US2 salvo T012, que necesita T006 y T009
- Polish al final

## Parallel

- T011, T013, T015 en paralelo tras T003.

## Implementation Strategy

MVP = Phase 2 + US1 + US3 (riel nuevo sin romper rutas); después US2 (panel y 2K), US4 y Polish.
