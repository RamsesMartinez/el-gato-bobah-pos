# Tasks: Ajustes de Almacén tras la prueba de la 044

**Input**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [quickstart.md](quickstart.md)
**Tests**: obligatorios (constitución IV): cada test se escribe y se ve fallar antes de su arreglo.
Todo en `web/`; sin servidor ni esquema. B# = bordes del plan.

## Phase 1: Setup

- [X] T001 Levantar `make start` del worktree y restaurar datos (`make db-restaurar`); anotar `FRONTEND_PORT` de `.env.worktree.local` para las pruebas locales

## Phase 2: Foundational

Ninguna: las cinco historias son independientes.

## Phase 3: US1 — La misma existencia en todas las pantallas (P1) 🎯 MVP

**Prueba independiente**: un artículo en ml negativo y uno en piezas muestran el mismo texto en Existencias, Artículos y la ficha.

- [X] T002 [P] [US1] Ampliar la tabla de `formatBaseQty` con 0, -1, 1, 1.5, -3000 ml, 999 g, 0.3333 L y unidad desconocida (B1–B3) en web/src/shared/units.test.ts
- [X] T003 [P] [US1] Test de Existencias: un renglón con `on_hand: "-3000"`, `unit_code: "ml"` muestra «-3 L», y `"1"`/`pieza` muestra «1 pieza» (hoy falla) en web/src/features/backoffice/StockPage.test.tsx
- [X] T004 [US1] Pintar la existencia con `formatBaseQty(s.on_hand, s.unit_code)` en web/src/features/backoffice/StockPage.tsx:92
- [X] T005 [US1] Barrer `web/src` buscando cantidades del almacén pintadas a mano (`on_hand}`, `unit_code}`, `{qty} {unit`) y pasarlas por `formatBaseQty`; anotar en plan.md › «Al construir» cuáles se tocaron

## Phase 4: US2 — «Revisar con un conteo» lleva al conteo (P2)

**Prueba independiente**: desde la ficha, un toque abre el conteo con el artículo buscado. Maqueta: plan.md › «Maquetas», fila «Ficha del artículo» (opción A).

- [X] T006 [P] [US2] Tests en web/src/features/backoffice/articles/ArticleSheets.test.tsx: con `inventory.count` el aviso tiene el botón «Revisar con un conteo ›» de 44 px en el mismo renglón; sin el permiso queda solo texto (B7); con el mínimo recién escrito, tocar el botón guarda primero y luego navega, y si el guardado falla no navega y se ve el error (B12)
- [X] T007 [P] [US2] Tests en web/src/features/backoffice/StockCountTab.test.tsx: `?q=leche` llena la búsqueda de la lista; un `q` que no está en el conteo deja la lista vacía con la búsqueda visible; sin conteo abierto se ve «Empezar» y la búsqueda se aplica al empezar (B6)
- [X] T008 [US2] Convertir el `<Text>` de web/src/features/backoffice/articles/ArticleSheet.tsx:275 en `Button` compacto (`minH="44px"`, `colorPalette="brand"` variante outline (no es destructivo: el rojo queda solo para lo que borra o no tiene vuelta, decisión del 2026-10-21; el aviso conserva su fondo rojo), sin `py` extra en el renglón, sin partir la línea) visible solo con `can('inventory.count', user)`; al tocar, esperar al autoguardado (`useAutosave`) y navegar a `/almacen?tab=conteo&q=<nombre>`
- [X] T009 [US2] Leer `q` de la URL en web/src/features/backoffice/StockPage.tsx y pasarlo como búsqueda inicial a `ListView` en web/src/features/backoffice/StockCountTab.tsx

## Phase 5: US3 — El buscador del conteo encuentra lo que se buscó (P2)

**Prueba independiente**: buscar «apio» con «Apio» y «Tapioca» trae solo «Apio».

- [X] T010 [P] [US3] Test de tabla de `matchesWords(text, query)` en web/src/utils/format.test.ts: «apio» vs «Tapioca» (no), «almen» vs «Leche de almendra» (sí), «AZÚ»/«azucar» vs «Azúcar» (sí), «leche alm» (sí), espacios dobles, una letra, vacío (B4)
- [X] T011 [US3] Implementar `matchesWords` en web/src/utils/format.ts sobre `normalize`: cada palabra buscada es inicio de alguna palabra del texto
- [X] T012 [US3] Test y uso en web/src/features/backoffice/StockCountTab.tsx:434 para nombre y empaques (B5: «bols» encuentra «Bolsa de hielo») en StockCountTab.test.tsx; barrer los demás buscadores del almacén (`grep -rn "normalize(" web/src/features/backoffice`) y aplicar `matchesWords` a los que filtren artículos, con su test; anotar en plan.md › «Al construir» cuáles se tocaron y cuáles no

## Phase 6: US4 — La barra lateral no corta el primer renglón (P3)

**Prueba independiente**: a 1024×600, con la barra desplazada a media opción, la de arriba se ve completa. Maqueta: plan.md › «Maquetas», fila «Barra lateral» (opción A).

- [X] T013 [US4] Reproducir con Playwright a 1024×600 contra el `make start` local: página larga desplazada y barra a media opción; medir la caja de «Pedidos»; anotar la causa en plan.md › «Al construir»
- [X] T014 [US4] Test e2e en web/e2e/cabe-en-la-tableta.spec.ts: tras desplazar la barra y soltarla, la primera opción visible queda completa (`getBoundingClientRect().top` ≥ el borde de la lista) mide al menos 44 px de alto, y al tocar «Asistencia» sigue visible (B10, SC-004)
- [X] T015 [US4] Ajuste por renglón en web/src/app/AppShell.tsx:83: `scrollSnapType: 'y mandatory'` en la lista y `scrollSnapAlign: 'start'` en cada opción; sin regresar el scroll a 0 al navegar; comprobar que la flecha de «hay más» sigue funcionando

## Phase 7: US5 — El tema sigue al del sistema (P3)

**Prueba independiente**: con el sistema en oscuro y sin elección guardada, la app abre en oscuro.

- [X] T016 [P] [US5] Tests en web/src/features/admin/AppearancePage.test.tsx: «Como el sistema» va primero y está activa sin elección guardada; una elección guardada `light`/`dark` se conserva (B8)
- [X] T017 [US5] `defaultTheme="system"` en web/src/main.tsx:30; renombrar «Sistema» a «Como el sistema» y ponerla primero en `THEMES` de web/src/features/admin/AppearancePage.tsx
- [X] T018 [US5] Test e2e con `colorScheme: 'dark'` y `localStorage` vacío: la app abre en oscuro y cambia en vivo con `emulateMedia` (B9) en web/e2e/almacen-claro-oscuro.spec.ts
- [X] T019 [US5] Repaso en oscuro de POS, hoja de cobro y corte de caja (B11): capturas a 1024×600, contraste 4.5:1 texto y 3:1 controles; empezar por la pantalla de venta del POS, web/src/shared/CobrarSheet.tsx y el corte de caja en web/src/features/backoffice/CashPage.tsx; pasar a tokens los colores fijos que se pierdan, en esos archivos o donde aparezcan; anotarlos en plan.md › «Al construir»

## Phase 8: Polish

- [X] T020 `cd web && bun run lint && bun run vitest run && bun run build`
- [X] T021 Vuelta de quickstart.md completa contra el `make start` y contra `bun run build && bun run preview`, a 1024×600 con `hasTouch`
- [X] T022 Comparación completa (SC-001): script de Playwright contra el `make start` local con la base restaurada que lee el texto de existencia de cada artículo en Existencias, en Artículos y en su ficha y falla listando los que difieran; resultado esperado 0 diferencias, anotado en plan.md › «Al construir» con el número de artículos revisados
- [X] T023 Prueba con usuario sin contexto (subagente) sobre Almacén y la barra lateral; lo que encuentre va a plan.md › «Al construir»
- [ ] T024 Marcar las casillas del issue #127 en la descripción del PR (`Closes #127`)

## Dependencies

- US1–US5 independientes entre sí; US2 T009 toca StockCountTab.tsx igual que US3 T012: hacerlas en serie.
- Dentro de cada historia: tests → implementación.
- Polish al final.

## Parallel

- T002, T003, T006, T007, T010, T016 (tests en archivos distintos) a la vez.
- US4 y US5 en paralelo con US1.

## Implementation Strategy

MVP = US1 (cifras contradictorias). Luego US2+US3 (conteo), luego US4 y US5. Cada historia se puede subir sola.
