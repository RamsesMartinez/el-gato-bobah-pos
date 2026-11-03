# Tasks: Lugares para contar

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md), [quickstart.md](quickstart.md)

TDD estricto (constitución IV): cada test se escribe y se ve fallar por la razón correcta antes de su
implementación. Las tareas de pantalla citan la fila de «Maquetas» del plan que implementan.

## Phase 1: Setup

- [X] T001 Confirmar que `0095` es la siguiente migración libre en `server/migrations/` (y que ninguna rama abierta la usa) y que el worktree tiene la base `gatobobah_restaurado` con dos empresas y artículos con `count_place` repetido en mayúsculas o espacios

## Phase 2: Foundational (bloquea a todas las historias)

- [X] T002 [P] Test de dominio: validar nombre de 1 a 30 letras tras `btrim`, normalización (reusa `NormalizeReasonName`), y orden nuevo al mover a `toPosition` (al inicio, al final, en medio, fuera de rango → `ErrValidation`), en `server/internal/domain/count_places_test.go`
- [X] T003 Test de la migración 0095 sobre el respaldo restaurado con dos empresas, en `server/internal/integration/migration_0095_test.go`:
  - «Refri», «refri » y «REFRI» quedan en un solo lugar, con el nombre más usado y sin espacios de más (E1).
  - El mismo texto en dos empresas da un lugar por empresa (E2), y ningún artículo que tenía lugar queda sin él (SC-002).
  - El índice rechaza un duplicado normalizado; RLS con `nullif`, grants sin `delete`, liga compuesta y su índice.
  - Ida y vuelta: la Down recrea `count_place` con el nombre y pierde el orden (E9).
  - Paridad del índice con `domain.NormalizeReasonName` usando «Ñandú», «Café  » y espacio duro (E10).
- [X] T004 Migración `server/migrations/0095_count_places.sql` según `data-model.md`, hasta que T003 pase
- [X] T005 `domain/count_places.go`: validación, normalización y cálculo del orden, hasta que T002 pase
- [X] T006 Consultas en `server/queries/count_places.sql` (listar con el conteo de artículos, incluidos los inactivos; crear al final; renombrar; tomar el lugar `for update` y `for share`; renumerar activos `order by position, id`; mover los artículos al archivar; asignar) y cambio de `CountItems` en `stock_counts.sql` y de `ingredients.sql` al `left join count_places`, sin nombrar `i.company_id`; `make sqlc`

## Phase 3: User Story 1 — Elegir el lugar de una lista (P1) 🎯 MVP

**Independent test**: con «Refri» y «Barra», asignar «Refri» a un artículo; crear « refri » elige el «Refri» existente y no crea otro.

- [X] T007 [P] [US1] Tests de integración bajo `appRoleStore` con `inTheThreeCases`, en `server/internal/integration/count_places_test.go`:
  - Listar y crear (repetido → 409 con `existingId`).
  - Asignar y quitar.
  - Un lugar de otra empresa → 404; uno archivado → 409 (E4).
  - Asignar mientras otra transacción archiva ese lugar: el artículo nunca queda en uno archivado (E5).
- [X] T008 [P] [US1] Tests del handler por el router real, en `server/internal/integration/count_places_http_test.go`:
  - Permisos: asignar y crear con `inventory.count`; listar con `count` **o** `configure`.
  - Nombre vacío o de 31 letras → 400.
  - `GET /stock/ingredients` y la hoja del conteo traen `countPlaceId` y `countPlace`.
  - Arreglos `[]` en el JSON crudo.
- [X] T009 [US1] Servicio en `server/internal/app/count_places.go` (listar, crear con el 23505 convertido en 409 con el id existente) y `SetCountPlace` con id en `stock_counts.go`; `articles.go` y la hoja del conteo con `countPlaceId`; hasta que T007 pase
- [X] T010 [US1] Rutas en `server/internal/httpapi/handlers_count_places.go` y `router.go` con `RequirePermission`, y `PUT /stock/ingredients/{id}/place` con `{ placeId }` en `handlers_stock_counts.go`, hasta que T008 pase
- [X] T011 [P] [US1] Tests de vitest del lugar del artículo (Maquetas: «Lugar del artículo», opción A), en `web/src/features/backoffice/CountPlaceField.test.tsx`:
  - El renglón dice «Lugar: Refri ›» o «Lugar: Sin lugar ›».
  - Abre el `Picker` con «Sin lugar» y los lugares en su orden; el buscador no toma el foco.
  - Escribir un nombre nuevo ofrece «Crear «x»»; con 409 elige el existente y dice «Ya tenías «Refri»; quedó elegido».
  - No hay campo de texto libre ni `<select>`.
- [X] T012 [US1] `CountPlaceField.tsx` con `Picker` (`clearable`, `clearLabel="Sin lugar"`, `onCreate`) y las llamadas en `web/src/api/backoffice.ts`; ajustar `PresentationsSheet.test.tsx` y el conteo que muestra el lugar; hasta que T011 pase

- [X] T025 [US1] Lo que encontró T022 (persona nueva): no halló dónde se asigna el lugar (solo lo vio dentro de un conteo). Almacén › Artículos muestra «Lugar: …» o «Sin lugar» junto a cómo llega, y el botón dice «Cambiar o poner lugar ›»; la agarradera usa el icono ≡ que nombra la instrucción. Con su test en `StockPage.test.tsx`

- [X] T026 [US2] Lo que encontró la segunda vuelta de T022: no halló cómo ordenar los lugares y el botón del lugar mostraba el valor viejo tras elegir. El conteo, antes de empezar, dice «Se recorre en este orden: …» y con `inventory.configure` lleva a Negocio › Lugares (`/negocio#lugares`); `PlaceChooser` muestra lo elegido de inmediato. Tests en `StockCountTab.test.tsx`, `CountPlaceField.test.tsx` y `web/e2e/lugares-para-contar.spec.ts`

- [X] T027 [US1] Lo que encontró la tercera vuelta de T022: «+ Empaque o lugar» juntaba dos cosas; el lugar tiene su propio botón en el renglón de Artículos y el de la hoja dice solo «Empaques ›»; «Todavía no dices cómo llega» pasa a «Sin empaques»; los destinos al archivar llevan borde; desde el conteo, Negocio › Lugares ofrece «‹ Volver al conteo». Tests en `StockPage.test.tsx`, `CountPlacesSettings.test.tsx` y `web/e2e/lugares-para-contar.spec.ts`

## Phase 4: User Story 3 — Los lugares escritos a mano pasan a la lista (P1)

**Independent test**: cubierto por T003. Esta historia es la migración de la Phase 2; aquí solo se corre contra la copia restaurada.

- [X] T013 [US3] Correr la 0095 sobre `gatobobah_restaurado` y comparar cuántos textos distintos normalizados había por empresa contra cuántos lugares hay después, sin artículos que perdieran su lugar (quickstart paso 1)

## Phase 5: User Story 2 — Administrar los lugares y su orden (P2)

**Independent test**: renombrar «Refri» cambia todos sus artículos; arrastrar «Bodega» al primer lugar hace que el conteo empiece por ella.

- [X] T014 [P] [US2] Tests de integración en los tres casos, en `server/internal/integration/count_places_admin_test.go`:
  - Renombrar cambia todos los artículos y un repetido → 409.
  - Mover a `toPosition` renumera 0..n-1, y fuera de rango → 400 (E13).
  - Archivar:
    - Con artículos y sin `moveTo` → 409 con cuántos.
    - Con `moveTo`, mueve también los inactivos (E6).
    - `moveTo` igual al lugar → 400; `moveTo` archivado → 409.
  - Crear y mover a la vez no repiten posición (E7).
  - Dos renombres simultáneos: gana uno. Renombrar con el nombre de un lugar que se está creando en ese momento → 409; nunca quedan dos nombres iguales.
  - El conteo recorre por `position` (FR-008), y uno abierto toma el orden nuevo al recargar sin perder lo capturado (E8).
- [X] T015 [US2] Servicio de renombrar, mover y archivar con `pg_advisory_xact_lock` por empresa y renumerado en otra sentencia; rutas con `inventory.configure`; hasta que T014 pase
- [X] T016 [P] [US2] Tests de vitest de la sección (Maquetas: «Lugares», opción C), en `web/src/features/admin/CountPlacesSettings.test.tsx`:
  - El título lleva «Agregar lugar» en la misma línea; cada renglón va en una sola línea con su agarradera ≡ de 52 px y `aria-label="Mover {nombre}"`.
  - Editar: renombrar, y «Archivar» rojo apartado de Guardar.
  - Archivar sin artículos confirma en una línea; con artículos pregunta a dónde pasan, con «Sin lugar» como opción.
  - Mover con el teclado (`KeyboardSensor`) llama a `move` con `toPosition`.
- [X] T017 [US2] `web/src/features/admin/CountPlacesSettings.tsx` con dnd-kit (`PointerSensor` `delay: 500, tolerance: 8`, `touch-action: none` y `-webkit-touch-callout: none` solo en la agarradera, `autoScroll`), montada en `InventorySettingsSection.tsx`, hasta que T016 pase
- [X] T018 [US2] Prueba Playwright táctil a 1024×600 contra el ambiente de pruebas, en `web/e2e/lugares-para-contar.spec.ts`:
  - Deslizar sobre un renglón desplaza la lista y no mueve nada (E11).
  - 500 ms sobre ≡ levanta el renglón; arrastrarlo de la fila 10 a la 1 desplaza la lista sola (E12).
  - El lugar del artículo abre la hoja sin teclado del sistema.
  - Contar los toques para elegir un lugar que ya está en la lista: 2 (abrir la hoja y tocarlo) (SC-001).
  - Limpieza: los lugares que crea la prueba se archivan.

## Phase 6: Polish

- [X] T019 [P] Bloque de la 040 en `AGENTS.md` (el lugar no es un almacén, el candado por empresa para el orden, la Down pierde el orden) y filas en `docs/matriz-de-pantallas.md`
- [X] T020 [P] Test de palabras de pantalla: «Lugar», «Sin lugar» y «Mantén presionada ≡ para mover un lugar», y nunca «catálogo» ni «almacén» en las dos pantallas (FR-011), agregando los archivos a `web/src/features/backoffice/operatorWords.test.ts`
- [X] T021 Correr `quickstart.md` contra la API local
- [X] T022 Prueba con usuario sin contexto (subagente) a 1024×600 en el ambiente de pruebas: asignar un lugar, crear uno y reordenar (SC-005)
- [X] T023 `make ci-local`
- [X] T024 `/revision-de-codigo` (hook `after_implement`)

## Dependencies

Phase 1 → Phase 2 → US1 → US3 → US2. US3 es la migración de la Phase 2 corrida contra datos reales; va después de US1 porque la pantalla que la usa es la de US1. US2 depende de los lugares y de las consultas de la Phase 2. T025, T026 y T027 salieron de la prueba con usuario sin contexto (T022, tres vueltas), así que van después de ella aunque vivan en US1 y US2.

Archivos compartidos que obligan orden:
- `count_places.go` del servicio: T009 → T015.
- `router.go`: T010 → T015.
- `backoffice.ts`: T012 → T017.

## Parallel

- Phase 2: T002 y T003 a la vez.
- US1: T007, T008 y T011 a la vez.
- US2: T014 y T016 a la vez.
- Polish: T019 y T020 a la vez.

## Implementation Strategy

MVP = Phase 1–2 + US1 + US3: se acaba el texto libre y nadie pierde su lugar. Después viene US2, administrar y arrastrar; mientras no esté, el recorrido sale en el orden alfabético inicial.
