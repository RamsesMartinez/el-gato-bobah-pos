# Tasks: Ubicación, autoguardado de la ficha y empaques claros

**Input**: [spec.md](spec.md), [plan.md](plan.md)

Reglas que aplican a todas las tareas:

- **TDD estricto (IV).** Cada prueba se escribe primero y se ve fallar por la razón correcta.
- **Pantallas.** Cada tarea de pantalla cita la fila de «Maquetas» del plan que implementa.
- **Bordes.** «E1…E11» se refieren a la tabla de bordes de [plan.md](plan.md).

## Phase 1: Setup

- [X] T001 Confirmar que el ambiente local tiene la base restaurada con dos empresas y artículos en
  peso, volumen y piezas con mínimo capturado (para reproducir #120).

## Phase 2: Foundational (bloquea el autoguardado)

- [X] T002 Reproducir #120: guardar el mismo mínimo dos veces en un artículo de cada clase (g, ml,
  cm, piezas), con y sin unidad grande, desde la ficha y desde «Nuevo artículo». Anotar en el plan,
  en «Al construir», la causa y en qué capa vive.
- [X] T003 Prueba de regresión de #120 que falla: vitest de `ArticleSheet` si la causa es de pantalla,
  integración en `server/internal/integration/ingredients_admin_test.go` si es del servidor. Nombre que
  diga qué se rompía («guardar el mismo mínimo dos veces deja el mismo valor»).
- [X] T004 Arreglo de #120 hasta que T003 pase. Cerrar #120 con la liga al commit.
- [X] T005 [P] Prueba de integración: `PATCH /stock/articles/{id}` con un solo campo cambia ese campo y
  deja los demás como estaban, y su evento `ingredient.updated` trae solo lo que cambió (FR-020). Bajo
  `appRoleStore` con `inTheThreeCases` (otra empresa, conexión reciclada y sin empresa).
- [X] T006 `app/ingredients_admin.go` y su handler aceptan un solo campo, también con `null` explícito,
  y responden el `updatedAt` nuevo (Revisión R3), hasta que T005 pase.

**Checkpoint**: #120 cerrado; el servidor acepta un guardado por campo.

## Phase 3: User Story 2 — La ficha guarda sola (P1)

- [X] T007 [P] [US2] vitest de la cola en `web/src/features/backoffice/articles/useAutosave.test.ts`:
  - dos cambios seguidos salen uno después del otro, en orden, con el `updatedAt` del anterior (E1);
  - al fallar por la red, el campo conserva lo escrito marcado y «Reintentar» manda lo que se ve; con
    un rechazo del servidor vuelve a lo guardado (E2);
  - un segundo toque en una lista o un interruptor que guarda se ignora;
  - 409 `STALE` avisa y recarga la ficha (E3);
  - salir de un campo sin cambiarlo no encola nada (E5);
  - un dato inválido se marca y no se encola (E6);
  - esperar a vaciar la cola resuelve cuando termina el último guardado (E4);
  - escribir en un campo mientras otro guarda no pierde lo escrito ni cierra el teclado (R1);
  - con reloj simulado: cada tecla reinicia la pausa de 2 s; «1», pausa, «5» manda 1 y después 15, y
    si el 1 todavía no sale, se reemplaza por 15 en vez de mandar dos (E12);
  - «1.», «-» y vacío no se mandan por la pausa; al salir del campo se validan (FR-022, E12).
- [X] T008 [US2] `useAutosave.ts` hasta que T007 pase.
- [ ] T009 [P] [US2] vitest de la ficha en `ArticleSheets.test.tsx` (Maquetas › Ficha del artículo):
  - no hay «Guardar»; «¿Descartar cambios?» solo aparece con algo sin guardar, nunca con todo
    guardado (cierra #119);
  - categoría y ubicación guardan al elegir; nombre y mínimo tras 2 s sin escribir y al salir del
    campo; Enter guarda pero nunca es necesario;
  - el punto junto al título aparece con cambios sin mandar y cambia a «Guardando…» y «Guardado ✓»;
  - el indicador muestra «Guardando…», «Guardado ✓» y «No se pudo guardar · Reintentar»;
  - ningún campo se deshabilita mientras guarda;
  - «‹ Artículos», el fondo y Escape guardan el campo enfocado, esperan la cola y, si
    algo quedó sin guardar, preguntan «¿Descartar cambios?» (R2);
  - g/kg del mínimo guarda al tocarlo y conserva la cantidad (R7);
  - sin aviso flotante «Guardado» (R1);
  - «Corregir costo», «Cambiar» y «Archivar» conservan su hoja, con espera en el botón;
  - «Nuevo artículo» conserva «Guardar».
- [ ] T010 [US2] `ArticleSheet.tsx` con autoguardado e indicador (Maquetas › Ficha del artículo).
  Cerrar #119.

- [ ] T026 [P] [US2] Integración de `ArticleAudit` en `ingredients_admin_test.go` (E13): tres guardados
  del mismo campo y usuario a menos de 60 s salen como una entrada con el antes del primero y el después
  del último; un hueco de 60 s, otro usuario, otro campo o un evento de dos campos corta la cadena; la
  paginación no parte una entrada; los tres casos de aislamiento (`inTheThreeCases`).
- [ ] T027 [US2] `db-architect` revisa el cambio de `ArticleAudit` antes de escribirlo. `ArticleAudit` agrupa con
  funciones de ventana antes de paginar, y `ArticleAuditSheet.tsx` muestra «N cambios» con el valor final, hasta que T026 pase.

## Phase 4: User Story 1 — Ubicación (P1)

- [ ] T011 [P] [US1] Integración en `server/internal/integration/count_places_test.go`: crear una
  ubicación sin `inventory.configure` da 403 y con él la crea; una repetida por acentos o mayúsculas
  devuelve la que existe (E8); los tres casos de aislamiento (`inTheThreeCases`) (E9).
- [ ] T012 [US1] `router.go`: `POST /stock/count-places` pasa a `RequirePermission(inventory.configure)`.
- [ ] T013 [US1] vitest del campo en `CountPlaceField.test.tsx` y `ArticleSheets.test.tsx`
  (Maquetas › Ficha del artículo):
  - se llama «Ubicación» y su vacío «Sin definir»;
  - sin ubicaciones muestra «Ej. Refri, Bodega, Barra»;
  - buscar sin acentos ni mayúsculas;
  - «Crear "X"» con `inventory.configure`; sin él, «Pídele a un administrador que la cree»;
  - en el conteo, quien solo cuenta ya no ve «Crear» y lee el aviso (R8);
  - «Crear "X"» va primero, debajo del buscador, cuando no hay coincidencia exacta (R4);
  - el filtro de la lista dice «Ubicación: Todas» y «Sin definir», y el conteo dice «ubicación»;
  - el nombre no pasa del tope de 30 caracteres;
  - un artículo con una ubicación archivada se ve «Sin definir».
- [ ] T014 [US1] `CountPlaceField.tsx`, `ArticleSheet.tsx`, `ArticlesTab.tsx`, `NewArticleSheet.tsx` y
  `StockCountTab.tsx`:
  «Ubicación», «Sin definir» y alta según permiso. La lista no lleva botón en el renglón.

## Phase 5: User Story 3 — Empaques sin adivinar (P2)

- [ ] T015 [P] [US3] vitest de `PresentationsSheet.test.tsx` (Maquetas › Hoja de empaque nuevo):
  - botones base por clase (peso, volumen, piezas) y los aprendidos sin repetir (E10);
  - ninguno elegido al abrir; «Otro» abre el campo con «Ej. Cubeta, Garrafa»;
  - «¿En qué empaque lo compras?» y la frase «1 bolsa = 5 kg» que cambia con lo capturado;
  - «Se cuenta en …» ya no aparece suelto;
  - la hoja ya no tiene la fila de ubicación (FR-021), y la frase va junto al pie (R5);
  - el empaque aproximado de un artículo en piezas sigue disponible (R6);
  - el selector de unidad de «Trae» es el mismo componente que el de «Mínimo» (FR-017).
- [ ] T016 [US3] Empaques base en `articleUnits.ts` y hoja en `PresentationsSheet.tsx`, con el selector
  de unidad de «Trae» igual al de «Mínimo», y sin la fila de ubicación (FR-021).

## Phase 6: User Story 4 — Existencia negativa (P2)

- [ ] T017 [P] [US4] vitest: existencia menor que cero en su propio renglón, en rojo y con «Revisar con
  un conteo»; cero o positiva, sin aviso.
- [ ] T018 [US4] `ArticleSheet.tsx`.

## Phase 7: User Story 5 — Pruebas de pantalla (P1)

- [X] T019 [US5] `web/e2e/gestion-de-articulos.spec.ts`: 6 renglones completos en la lista, controles
  de 44 px y ficha sin desplazamiento de lado, acostada y de pie, a 1024×600. Pasa contra lo desplegado
  hoy (cierra T045 de la 042 cuando se mergee).
- [X] T020 [US5] `web/e2e/lugares-para-contar.spec.ts`: la ubicación se elige en 3 toques desde la
  ficha, sin teclado y sin «Guardar». Falla hasta que T010 y T014 estén desplegados.
- [ ] T025 [US5] e2e de la hoja de empaque con el alto reducido por el teclado: la frase y «Guardar
  empaque» quedan a la vista (R5), y todo control de la hoja mide 44 px o más (FR-019).
- [ ] T021 [US5] Correr la suite e2e completa contra el ambiente de pruebas con la 043 desplegada.

## Phase 8: Polish

- [ ] T022 `make ci-local` en verde.
- [ ] T023 Prueba con usuario sin contexto en local a 1024×600: darle ubicación a un artículo y agregar
  un empaque sin ayuda (SC-006). Anotar lo encontrado en «Al construir».
- [ ] T024 Cerrar en #103 los dos puntos que cubre esta spec («Lugar» y «Se cuenta en g»).

## Dependencies

- T002 a T004 (#120) antes de T008 y T010: el autoguardado no se activa con #120 abierto.
- T005 y T006 antes de T010.
- US1 (ubicación) puede ir en paralelo con US2, pero T014 toca `ArticleSheet.tsx` igual que T010:
  van en orden.
- US3 y US4 son independientes.
