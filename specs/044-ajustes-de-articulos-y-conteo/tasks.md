# Tasks: Ajustes de artículos y conteo tras la prueba en tableta

**Input**: [spec.md](spec.md), [plan.md](plan.md)

**Tests**: obligatorios (constitución IV): cada prueba se escribe antes y se ve fallar.

## Phase 1: Setup

- [X] T001 Que `make nuevo-worktree` no herede las variables del worktree que lo corre, con su prueba en scripts/nuevo-worktree.sh y scripts/tests/nuevo-worktree-test.sh

## Phase 2: Foundational (límites del conteo; bloquea US4 y US6)

- [ ] T002 Prueba de dominio de `CountLimits`: defaults (1,000,000 en unidad base de máximo; 50,000 g, 50,000 ml, 5,000 cm, 5,000 piezas de confirmar), `ForKind`, `Validate` (cero, negativo, NaN, máximo ≤ confirmar, sobre `MaxCountLimit`, y que subir sobre el default se acepta) y `MaxMessage` («Máximo 1,000 kg por conteo», L, m, piezas) en server/internal/domain/count_limits_test.go
- [ ] T003 Implementar `CountLimits`, `DefaultCountLimits`, `MaxCountLimit` = 1,000,000,000 y los métodos en server/internal/domain/count_limits.go
- [ ] T004 Prueba de integración de la migración 0098 con dos empresas: relleno de todas, valores iguales a `domain.DefaultCountLimits`, checks con nombre que rechazan cero y máximo ≤ confirmar, Down vuelve a 0097 limpio, y `CreateCompany` siembra la fila con los mismos valores, en server/internal/integration/count_limits_test.go
- [ ] T005 Migración server/migrations/0098_count_limits.sql: tabla `count_limits` (company_id pk con default `nullif(current_setting('app.company_id', true), '')::bigint`, FK on delete cascade; 8 columnas numeric(14,4) con defaults; `updated_at`), checks con nombre (> 0, max > confirm, <= 1e9), RLS `tenant_isolation` con `nullif` (using y with check), grant select/insert/update a `gatobobah_app`, relleno `on conflict do nothing`; Down `drop table` con comentario de lo que pierde (va en el mismo commit que T004 por el hook)
- [ ] T006 CTE `count_limits_seed` en `CreateCompany` de server/queries/companies.sql y consultas `GetCountLimits` / `UpsertCountLimits` (fija `updated_at = now()`) en server/queries/count_limits.sql; `make sqlc`
- [ ] T007 Prueba de integración bajo `appRoleStore` con `inTheThreeCases`: leer y guardar límites; empresa sin fila cae a defaults sin escribir; la otra empresa no ve ni cambia, en server/internal/integration/count_limits_test.go
- [ ] T008 Servicio `CountLimitsService` (`Get` con fallback a defaults, `Set` con `Validate`, upsert y `recordAudit` `settings.count_limits_changed` con antes y después en la misma tx) en server/internal/app/count_limits.go; acción en la lista blanca de server/internal/domain/audit.go y prueba para `TestEveryAuditActionIsProduced`

## Phase 3: US1 - Volver a Artículos (P1)

- [ ] T009 [P] [US1] Prueba vitest: botón «Volver a Artículos» con flecha `aria-hidden`, borde, 44 px, y que hace lo mismo que `close()` (guarda, espera, pregunta descartar) en web/src/features/backoffice/articles/ArticleSheet.test.tsx
- [ ] T010 [US1] Botón en web/src/features/backoffice/articles/ArticleSheet.tsx:217; medir a 800 px de ancho y, si no cabe, «← Artículos» con borde

## Phase 4: US2 - «Revisar» legible (P1)

- [ ] T011 [P] [US2] Prueba vitest: etiqueta «Revisar» del tamaño de la cifra, roja sobre fondo rojo suave, sin `title`, celda `nowrap`, en web/src/features/backoffice/articles/ArticlesTab.test.tsx
- [ ] T012 [US2] Etiqueta en web/src/features/backoffice/articles/ArticlesTab.tsx:124 con ancho fijo de columna

## Phase 5: US4 - Mensaje del máximo (P1)

- [ ] T013 [US4] Prueba de integración: un conteo de un artículo en gramos con 1,500 kg responde 400 «Máximo 1,000 kg por conteo»; tras subir el máximo a 2,000 kg se acepta; bajar el máximo no toca lo ya capturado (E4); el peso convertido a piezas usa el máximo de piezas (E7), en server/internal/integration/count_limits_test.go
- [ ] T014 [US4] Prueba de dominio: `CountSanity` usa la cifra de confirmar recibida y no revela cifras, en server/internal/domain/count_sanity_test.go
- [ ] T015 [US4] La captura deja de validar contra `MaxStockQty` y usa los límites de la empresa (máximo y confirmar) en server/internal/app/stock_counts.go, server/internal/domain/stock_count.go y server/internal/domain/count_sanity.go
- [ ] T016 [US4] Rutas `GET /settings/count-limits` (`inventory.count` o `inventory.configure`) y `PUT` (`RequirePermission(inventory.configure)`), con prueba de 403 para quien cuenta (E10), en server/internal/httpapi/handlers_settings.go y router.go
- [ ] T017 [US4] Prueba vitest: el conteo muestra «Máximo N <unidad> por conteo» antes de mandar con los límites del GET; si el GET falla usa los defaults; un 400 muestra el texto del servidor, en web/src/features/backoffice/StockCountTab.test.tsx
- [ ] T018 [US4] Cliente de límites en web/src/api y aviso en web/src/features/backoffice/StockCountTab.tsx (refresca al abrir el conteo)

## Phase 6: US3 - Gráfica (P2)

- [ ] T019 [P] [US3] Prueba vitest: 0 puntos no dibuja; 1 punto con su cifra; varios con línea, área, último costo en etiqueta y correcciones huecas; tocar elige el punto más cercano por x y muestra fecha y costo dentro del SVG; tocar fuera cierra, en web/src/features/backoffice/articles/CostTrend.test.tsx
- [ ] T020 [US3] Rediseño en web/src/features/backoffice/articles/CostTrend.tsx según la maqueta de la gráfica (plan › Maquetas, fila 1)

## Phase 7: US5 - Teclado físico (P2)

- [ ] T021 [P] [US5] Prueba vitest: números, punto, coma como punto, Retroceso, Enter = Guardar, Tab = siguiente casilla solo si la hay; letras ignoradas; Enter sin captura o con el aviso abierto no hace nada; con foco en input o botón, o con hoja/diálogo/Picker abierto, no se apropia de las teclas; cada tecla se escribe una vez, en web/src/features/backoffice/StockCountTab.test.tsx
- [ ] T022 [US5] Un solo listener en web/src/features/backoffice/StockCountTab.tsx que alimenta a web/src/components/NumericKeypad.tsx

## Phase 8: US6 - Configurar límites (P2)

- [ ] T023 [P] [US6] Prueba vitest: sección «Límites del conteo» con 4 clases en kg, L, m y piezas; error en línea si máximo ≤ confirmar; «Guardar límites» manda PUT y «Descartar» vuelve; se oculta sin `inventory.configure`; miles al salir del campo, en web/src/features/admin/CountLimitsSettings.test.tsx
- [ ] T024 [US6] Componente web/src/features/admin/CountLimitsSettings.tsx dentro de web/src/features/admin/InventorySettingsSection.tsx antes de «Se echó a perder», según la maqueta de límites (plan › Maquetas, fila 2): pie sticky, `scrollIntoView` al enfocar, inputs de 44 px, textos del plan
- [ ] T025 [US6] Etiqueta de `settings.count_limits_changed` en web/src/shared/audit/labels.ts

## Phase 9: US7 - Existencias en tema oscuro (P1)

- [ ] T026 [US7] Reproducir en local (:3044) con Playwright a 1024×600 en tema oscuro (Almacén › Existencias), con captura, y encontrar los colores fijos que lo causan, en web/e2e/almacen-claro-oscuro.spec.ts
- [ ] T027 [US7] Prueba que falla: contraste ≥ 4.5:1 de renglones, encabezados y cifras en tema oscuro (Playwright local, misma spec) y vitest si el color sale de un componente
- [ ] T028 [US7] Cambiar los colores fijos por tokens del tema en web/src/features/backoffice/StockPage.tsx (y el componente de la lista que corresponda); capturas en claro y oscuro

## Phase 10: US8 - Repaso visual del almacén (P1)

- [ ] T029 [US8] Recorrido en web/e2e/almacen-claro-oscuro.spec.ts contra local (:3044) a 1024×600, en claro y oscuro: entrando como admin de bobah-pruebas (ve todas las pestañas), en el orden de la pantalla: Existencias, Movimientos, Artículos y ficha, Por recibir, Conteo con teclado, Se echó a perder, Salud, y Configuración del negocio › Almacén; mide contraste (4.5:1 texto, 3:1 controles), desborde horizontal y alto ≥ 44 px de controles; guarda capturas en specs/044-ajustes-de-articulos-y-conteo/capturas/
- [ ] T030 [US8] Por cada defecto: su prueba que falla (vitest o el recorrido) y el arreglo con tokens del tema en el componente que lo causa; anotar la lista en plan › Al construir
- [ ] T031 [US8] Correr el recorrido también contra `bun run build && bun run preview` (local) para acercarse al build de producción

## Phase 11: Polish

- [ ] T032 Playwright en local (:3044) a 1024×600: la lista conserva 6 renglones; la sección de límites con controles de 44 px y «Guardar límites» visible con el teclado del sistema simulado, en web/e2e/gestion-de-articulos.spec.ts
- [ ] T033 `make api-build && make api-test`, `make test-integration`, `make web-test`, `make web-lint`
- [ ] T034 AGENTS.md: renglón de «Límites del conteo» (tabla, defaults en la columna, solo el conteo los usa)
- [ ] T035 Prueba con usuario sin contexto en local (:3044, 1024×600, claro y oscuro) y anotar en plan › Al construir
- [ ] T036 Dejar la 044 corriendo en local para el dueño; PR #125 en borrador

## Dependencies

- Phase 2 bloquea US4 y US6. US1, US2, US3 y US5 son independientes.
- T004 y T005 van en el mismo commit (hook de migración).
- T017/T021 tocan el mismo archivo de prueba: en serie.
- US8 corre después de US1 a US7 (repasa las pantallas ya cambiadas). Nada se valida en el ambiente de pruebas.

## Implementation Strategy

1. Phase 2 → US4 (cierra el «datos inválidos»).
2. US1 y US2 en paralelo con lo anterior.
3. US6, US3, US5.
4. Polish y entrega al dueño.
