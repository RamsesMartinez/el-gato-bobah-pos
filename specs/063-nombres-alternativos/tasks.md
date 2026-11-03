---
description: "Tareas de la feature 063: nombres alternativos en productos y artículos"
---

# Tasks: Nombres alternativos en productos y artículos

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[data-model.md](data-model.md), [contracts/alt-names.md](contracts/alt-names.md),
[quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). Cada tarea de prueba va antes de su implementación y
se ve fallar por la razón correcta. Las de integración llevan `//go:build integration` y corren con
`make test-integration`.

**Rutas**: `server/` (Go) y `web/src/` (React).

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

- [ ] T001 Confirmar que el worktree tiene `make start` arriba (:8063/:3063) y el Postgres de pruebas (`make test-integration` con `TEST_DATABASE_URL` de `.env.worktree.local`); anotar en el reporte final si `TEST_RESTORED_DATABASE_URL` no está y qué pruebas de respaldo se saltan

---

## Phase 2: Foundational (bloquea todas las historias)

**Purpose**: esquema, llave de comparación y reglas de dominio comunes.

### Pruebas primero

- [ ] T002 [P] Prueba table-driven de `CatalogNameKey`, `CleanAltNames` (recorta y junta espacios con `strings.Fields`, descarta vacíos, 0–10, 1–60 letras, repetido en la lista, igual al nombre propio, nunca nil) y del mensaje de `AltNameTaken` (nombre / otro nombre / inactivo / archivado) en `server/internal/domain/alt_names_test.go`
- [ ] T003 [P] Prueba de integración de la 0107 sobre base restaurada con dos empresas: columnas nacen `{}`, ninguna fila cambia, la API anterior inserta y actualiza productos e ingredientes sin nombrar la columna, el check rechaza 11 elementos, un NULL y un vacío, y bajar quita columnas y función sin tocar filas, en `server/internal/integration/migration_0107_test.go`
- [ ] T004 [P] Prueba de paridad `catalog_name_key` (SQL) contra `domain.CatalogNameKey` con acentos, mayúsculas, tabulador, salto de línea y espacio duro en las orillas y en medio, en `server/internal/integration/catalog_name_key_test.go`

### Implementación

- [ ] T005 Migración `server/migrations/0107_alt_names.sql` según [data-model.md](data-model.md) (lock_timeout 3 s, una sentencia por tabla con su check, función con `StatementBegin/End`, Down)
- [ ] T006 Pasar el db-architect sobre `server/migrations/0107_alt_names.sql` ANTES de aplicarla a la base local; corregir lo que encuentre
- [ ] T007 [P] `server/internal/domain/alt_names.go`: `MaxAltNames`, `MaxAltName`, `CatalogNameKey`, `CleanAltNames`, `ErrAltNameTaken`, `AltNameTaken` (hasta que T002 pase)
- [ ] T008 Mapear `ErrAltNameTaken` → 409 `ALT_NAME_TAKEN` con `details {name, existingId, archived}` en `server/internal/httpapi/respond.go`
- [ ] T009 Clasificar `products.alt_names` e `ingredients.alt_names` en `noPersonalData` («catálogo: otros nombres del producto/artículo») en `server/internal/integration/respaldo_anonimo_test.go`
- [ ] T010 Aplicar la 0107 a la base local (`make db-migrate` o arranque de la API) y correr `make sqlc`; T003 y T004 en verde

**Checkpoint**: esquema y dominio listos.

---

## Phase 3: User Story 2 — Capturar los alternativos de un producto (P1)

Va antes que la US1 porque la US1 necesita algo que buscar; las dos son P1.

**Independent Test**: agregar dos alternativos a un producto y verlos tras recargar; agregar el nombre de otro producto y ver el aviso que lo nombra.

### Pruebas primero

- [ ] T011 [P] [US2] Integración `SetProductAltNames` en `server/internal/integration/alt_names_products_test.go`: guarda y conserva orden; quitar todos guarda `{}` y el JSON crudo trae `[]`; choca con el nombre de otro producto, con un alternativo de otro (otro espaciado y sin acento), con un inactivo (mensaje lo dice); el genérico de plataforma da 404; dos guardados simultáneos del mismo alternativo en dos productos → uno guarda y otro 409
- [ ] T012 [P] [US2] Integración: `CreateProduct`, `UpdateProduct` (renombrar) y `DuplicateProduct` con un nombre que es alternativo de otro → 409 `ALT_NAME_TAKEN`; duplicar no copia alternativos; renombrar sin cambiar el nombre no toma el candado de nombres; el `PATCH` de siempre (sin `altNames`, como lo manda una tableta vieja) deja los alternativos intactos (FR-009), en `server/internal/integration/alt_names_products_test.go`
- [ ] T013 [P] [US2] Integración de aislamiento con `inTheThreeCases`: un alternativo de otra empresa no choca, no se nombra en el aviso y no aparece en los parecidos; sin empresa y con conexión reciclada no se lee nada, en `server/internal/integration/alt_names_isolation_test.go`
- [ ] T014 [P] [US2] Prueba HTTP: `PUT /admin/products/{id}/alt-names` y `GET …/alt-names/similar` exigen `menu.configure` (cajero 403), 400 con 11 nombres o 61 letras, en `server/internal/httpapi/alt_names_test.go`
- [ ] T015 [P] [US2] Prueba de dominio del ranking de parecidos con alternativos: deduplica por id, excluye el propio y los idénticos, máximo 5, en `server/internal/domain/alt_names_test.go`

### Implementación

- [ ] T016 [US2] Consultas `LockProductNames`, `FirstProductNameClash`, `SetProductAltNames`, `ProductNameClashesAlt`, `ProductAltNameCandidates` en `server/queries/alt_names.sql`; `make sqlc`
- [ ] T017 [US2] `SetProductAltNames` y `SimilarProductNames` en `server/internal/app/alt_names.go` (candado 28063 primero, `CleanAltNames`, choque, escritura)
- [ ] T018 [US2] Candado y choque de FR-005 en `CreateProduct`, `UpdateProduct` (solo nombre cambiado) y `DuplicateProduct` en `server/internal/app/admin.go`
- [ ] T019 [US2] Handlers `PutProductAltNames`, `SimilarProductAltNames` en `server/internal/httpapi/handlers_alt_names.go` y rutas con `RequirePermission(PermMenuConfigure)` en `server/internal/httpapi/router.go`
- [ ] T020 [US2] `AdminListProducts` y su tipo devuelven `alt_names` (siempre arreglo) en `server/queries/admin.sql` y `server/internal/app/admin.go`
- [ ] T021 [P] [US2] Vitest de `AltNamesField`: agrega, quita (botón de 44 px con `aria-label`), con 10 oculta el campo, espera parecidos antes de agregar y pasa a «Agregar de todos modos», muestra el choque bajo el campo, en `web/src/components/AltNamesField.test.tsx`
- [ ] T022 [US2] Componentes `web/src/components/AltNameChip.tsx` y `web/src/components/AltNamesField.tsx` (Maquetas: «Editor «Otros nombres»», opción A con aviso de parecidos; [R14](research.md))
- [ ] T023 [US2] Cliente `adminApi.setProductAltNames` y `similarProductAltNames`, `altNames` en `AdminProduct` en `web/src/api/admin.ts`
- [ ] T024 [P] [US2] Vitest de `ProductEditDialog`: «Guardar» manda primero el `PUT alt-names` si cambió y luego el `PATCH`; texto escrito sin «Agregar» se agrega al guardar; un choque deja la ficha abierta con el aviso bajo el campo, en `web/src/shared/ProductEditDialog.test.tsx`
- [ ] T025 [US2] Editor en `web/src/shared/ProductEditDialog.tsx` después de Categoría (Maquetas: «Editor «Otros nombres»», A)

**Checkpoint**: la US2 funciona sola.

---

## Phase 4: User Story 1 — Encontrar un producto en Vender por otro nombre (P1) MVP

**Independent Test**: «Ice Coffee» con «Café frío»; buscar «cafe fri» en Vender, ver la etiqueta, venderlo; ticket y comanda dicen «Ice Coffee».

### Pruebas primero

- [ ] T026 [P] [US1] Integración: el menú del POS trae `altNames` de un producto que los tiene y los omite si no; guardar alternativos sube la versión de catálogo (ETag cambia); un producto vendido tras buscarse por alternativo guarda `order_lines.name` con el nombre principal, y la comanda/ticket (lo que lee la impresión) no contiene el alternativo, en `server/internal/integration/alt_names_menu_test.go`
- [ ] T027 [P] [US1] Vitest de `buscarProductos`: nombre primero, alternativo después, código al final; devuelve el alternativo que coincidió (el primero en orden); sin acentos ni mayúsculas; error de dedo no coincide; coincidir por nombre y alternativo cuenta como nombre, en `web/src/features/pos/buscarProducto.test.ts`
- [ ] T028 [P] [US1] Vitest de `matchedAlias` en `web/src/utils/format.test.ts`

### Implementación

- [ ] T029 [US1] `MenuProducts` trae `alt_names` en `server/queries/menu.sql`; `MenuProduct.AltNames []string \`json:"altNames,omitempty"\`` en `server/internal/app/menu.go`
- [ ] T030 [US1] `matchedAlias(alts, query, match)` en `web/src/utils/format.ts`
- [ ] T031 [US1] `altNames?: string[]` en `MenuProduct` (`web/src/types/pos.ts`); `buscarProductos` con tercer balde y alternativo coincidente en `web/src/features/pos/buscarProducto.ts`
- [ ] T032 [US1] Componente `web/src/components/AltMatchTag.tsx` («también: …», `Badge` outline gris, truncado) (Maquetas: «Coincidió con» en resultados, opción B)
- [ ] T033 [US1] `ProductTile` muestra `AltMatchTag` en el lugar de la descripción cuando hay alternativo coincidente (el tile no crece) y `POSPage`/`ProductGrid` le pasan el alternativo, en `web/src/features/pos/ProductTile.tsx`, `ProductGrid.tsx`, `POSPage.tsx` (Maquetas: B)

**Checkpoint**: MVP — capturar (US2) y encontrar en Vender (US1).

---

## Phase 5: User Story 3 — Productos y recetas por otro nombre (P2)

**Independent Test**: buscar «cafe frio» en Productos y en el selector de producto de una receta de combo.

### Pruebas primero

- [ ] T034 [P] [US3] Integración `AdminListProducts`: buscar por alternativo devuelve el producto con `matchedAlt`, el total lo cuenta; por nombre sin acento encuentra; `%` no es comodín; con `inTheThreeCases` no ve otra empresa, en `server/internal/integration/alt_names_products_test.go`
- [ ] T035 [P] [US3] Vitest de `SearchSheet` con `aliases`: encuentra por alias, muestra la etiqueta, por nombre no la muestra, en `web/src/shared/SearchSheet.test.tsx`

### Implementación

- [ ] T036 [US3] Búsqueda con `catalog_name_key` sobre nombre y alternativos y `matched_alt` en `AdminListProducts` (`server/queries/admin.sql`), campo `matchedAlt` en `server/internal/app/admin.go`
- [ ] T037 [US3] `ProductsAdminPage` pinta `AltMatchTag` junto al nombre (truncada, la fila no crece) en `web/src/features/admin/ProductsAdminPage.tsx` (Maquetas: B)
- [ ] T038 [US3] `SearchOption.aliases` y la etiqueta en `web/src/shared/SearchSheet.tsx`; `CompositionSheet` pasa `aliases` en los selectores de producto (`part`/`link`), de insumo («Agregar insumo») y «¿Qué se resta?» en `web/src/shared/CompositionSheet.tsx` (Maquetas: B)

---

## Phase 6: User Story 4 — Alternativos en los artículos del almacén (P2)

**Independent Test**: dos alternativos a un artículo; buscarlo por uno en Artículos, Conteo, captura de gasto y receta; crear un artículo con ese nombre desde un gasto usa el existente.

### Pruebas primero

- [ ] T039 [P] [US4] Integración `SetArticleAltNames` en `server/internal/integration/alt_names_articles_test.go`: guarda, orden, `{}`; choca con nombre o alternativo de otro (incluido archivado, mensaje lo dice); el origen de una unión no reserva (FR-021); concurrencia; deja evento `ingredient.updated` con `altNames`; no mueve `updated_at`; funciona sobre un archivado; un producto y un artículo con el mismo nombre no chocan
- [ ] T040 [P] [US4] Integración: crear o renombrar un artículo con un nombre que es alternativo de otro → `NAME_TAKEN` con `existingId`; `ensureArticleTx` (gasto/receta) usa el existente activo; el cambio de unidad por reemplazo pasa los alternativos al nuevo y el viejo queda sin ellos, en `server/internal/integration/alt_names_articles_test.go`
- [ ] T041 [P] [US4] Integración de búsqueda y lecturas: `ListArticles` por alternativo con `matchedAlt`; `GetArticle`, `ListIngredients`, `SearchArticles` (también productos con existencia) y el conteo actual traen `altNames` (siempre `[]`); todas con `inTheThreeCases`, en `server/internal/integration/alt_names_articles_test.go`
- [ ] T042 [P] [US4] Prueba HTTP: `PUT /stock/articles/{id}/alt-names` y `…/similar` exigen `inventory.configure`, en `server/internal/httpapi/alt_names_test.go`
- [ ] T043 [P] [US4] Vitest: `Picker` con `aliases` (encuentra, etiqueta en `ListRow` vía `match`); filtro del Conteo con alternativos por `matchesWords`, en `web/src/components/Picker.test.tsx` y `web/src/features/backoffice/StockCountTab.test.tsx`
- [ ] T044 [P] [US4] Vitest de `ArticleSheet`: agregar y quitar pasan por el autoguardado (`SaveIndicator`), si falla la lista vuelve; texto pendiente se agrega al volver, en `web/src/features/backoffice/articles/ArticleSheet.test.tsx`

### Implementación

- [ ] T045 [US4] Consultas `FirstArticleNameClash` (sin orígenes de unión), `SetArticleAltNames`, `ArticleAltNames`, `ArticleAltNameCandidates` en `server/queries/alt_names.sql`; `FindArticleByName`/`FindOtherArticleByName` con `via_alt` en `server/queries/ingredients_admin.sql`; `make sqlc`
- [ ] T046 [US4] `ArticleNameTaken.ViaAlt` y su mensaje en `server/internal/domain/ingredients.go`
- [ ] T047 [US4] `SetArticleAltNames` y `SimilarArticleAltNames` (candado 28037, evento `ingredient.updated`) en `server/internal/app/alt_names.go`
- [ ] T048 [US4] `replaceArticle` mueve los alternativos al nuevo en `server/internal/app/unit_change.go`
- [ ] T049 [US4] `ListArticles` (búsqueda + `matched_alt`), `GetArticle`, `ListIngredients`, `SearchArticles` y `CountItems` traen `alt_names` en `server/queries/ingredients_admin.sql`, `ingredients.sql`, `stock_counts.sql`, y sus tipos en `server/internal/app/` (siempre arreglo)
- [ ] T050 [US4] Handlers y rutas con `RequirePermission(PermInventoryConfigure)` en `server/internal/httpapi/handlers_alt_names.go` y `router.go`
- [ ] T051 [US4] Cliente y tipos (`ArticleRow`, `ArticleSheet`, `Ingredient`, `Article`, `CountItem` con `altNames`; `matchedAlt`) en `web/src/api/backoffice.ts`
- [ ] T052 [US4] Editor en `web/src/features/backoffice/articles/ArticleSheet.tsx` con el autoguardado (Maquetas: «Editor «Otros nombres»», A)
- [ ] T053 [US4] `ArticlesTab` pinta `AltMatchTag` en `web/src/features/backoffice/articles/ArticlesTab.tsx`; `StockCountTab` filtra por alternativos y pinta la etiqueta en la línea de empaques si no cabe, en `web/src/features/backoffice/StockCountTab.tsx` (Maquetas: B)
- [ ] T054 [US4] `PickerOption.aliases` + `ListRow` prop `match` en `web/src/components/Picker.tsx` y `web/src/components/ListRow.tsx`; `ExpenseDialog`, `AddLineSheet` y `ArrivedItemSheet` pasan `aliases` (Maquetas: B). Los insumos de `CompositionSheet` usan `SearchSheet` (T038) con `altNames` de `GET /ingredients` (T049)

---

## Phase 7: Polish & cross-cutting

- [ ] T055 Revisar los escritores de nombres de R13 contra el código (`grep` de `insert into products|ingredients` y `set name`) y dejar en el reporte que los exentos siguen exentos; confirmar con grep que reportes, exportaciones y lo que se manda a plataformas no leen `alt_names` (FR-013)
- [ ] T056 Agregar a AGENTS.md §2 el renglón de la feature (alternativos: candados 28063/28037, `catalog_name_key`, ruta propia para no borrar desde una tableta vieja, orígenes de unión fuera del choque)
- [ ] T057 `cd server && go build ./... && go test ./...`, `make test-integration`, `cd web && bun run lint && bun run vitest run && bun run build`
- [ ] T058 `/revision-de-codigo` sobre el diff; corregir lo que encuentre con su prueba
- [ ] T059 Prueba con usuario nuevo a 1024×600 (`hasTouch`) contra `http://localhost:3063` con los buscadores de Vender, Productos, Artículos, Conteo, gastos y recetas, siguiendo [quickstart.md](quickstart.md); verificar ticket y comanda con el nombre principal; cobrar y cerrar lo que se cree; capturas solo en `test-results/`
- [ ] T060 Vuelta final contra el build servido en local (`bun run build && bun run preview`)

---

## Dependencies & Execution Order

- Phase 2 bloquea todo. T005 → T006 → T010 (la migración no se aplica sin el db-architect).
- US2 (Phase 3) → US1 (Phase 4): la US1 usa los alternativos capturados; sus pruebas de integración pueden sembrarlos por SQL.
- US3 depende de T020 (lista con `alt_names`) y de `AltMatchTag` (T032).
- US4 es independiente de US1–US3 salvo `AltNamesField` (T022), `AltMatchTag` (T032) y la migración.
- Polish al final.

## Parallel Example

```text
Phase 2: T002, T003, T004 en paralelo; luego T005.
US2: T011–T015 en paralelo (archivos distintos o casos distintos); T021 y T024 en paralelo.
US4: T039–T044 en paralelo.
```

## Implementation Strategy

1. Phase 2 → US2 → US1: MVP (capturar y encontrar en Vender, ticket con nombre principal).
2. US3 y US4.
3. Polish, revisión de código, prueba con usuario nuevo, PR.
