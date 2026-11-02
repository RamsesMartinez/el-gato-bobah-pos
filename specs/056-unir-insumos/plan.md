# Implementation Plan: Un insumo con varias marcas (entrega 1)

**Branch**: `056-unir-insumos` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/056-unir-insumos/spec.md`

## Summary

Unir insumos duplicados en uno (destino), con vista previa calculada por el servidor y confirmación
explícita; marca opcional en la presentación; aviso de posible duplicado al guardar un insumo nuevo; y una
lista de grupos de posibles duplicados desde donde se llega a la misma unión (decisión del dueño al
elegir las maquetas: A + C).

La unión reusa el reemplazo de artículos de la 042: cada origen queda en `ingredient_replacements`
con `kind = 'merge'` y su factor, el traspaso de existencia son movimientos ligados con
`replacement_id`, y el origen se archiva. Recetas, presentaciones (con marca), formatos de compra y
artículos de proveedor pasan al destino; la historia del origen no se reescribe y se lee desde el
destino. La base garantiza que nada nuevo cuelgue de un origen unido: los movimientos tardíos se
redirigen al destino y el resto se rechaza, con un respaldo diferido para la carrera con una venta en
curso (research R2, R3). La vista previa es la misma transacción revertida, con huella (patrón 053).

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS por empresa; migración **0104**

**Testing**: `go test` (domain table-driven), integración contra Postgres real (`make test-integration`,
respaldo restaurado, dos empresas, `appRoleStore`/`inTheThreeCases`), vitest, Playwright 1024×600

**Target Platform**: API Linux en VM; POS web en tabletas de 7–10" (1024×600)

**Project Type**: web (server/ + web/)

**Performance Goals**: unir 2–5 insumos con decenas de recetas en < 1 s; el aviso de duplicado
responde < 300 ms con ~1,000 insumos

**Constraints**: ninguna pérdida ni duplicado de existencia con ventas simultáneas; tolerancia a la
API anterior durante la subida azul/verde; no tocar `purchases.go` ni `purchaseqty.go` (la 055)

**Scale/Scope**: una empresa con ~300 insumos y >15 grupos duplicados; hasta 20 orígenes por unión

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple | Estado |
|---|---|---|
| I. Layering | Reglas puras en `domain/ingredient_merge.go` (validar la solicitud, plan de recetas y presentaciones, costo resultante) y `domain/similar_names.go`; orquestación y transacción en `app/ingredient_merge.go`; handlers finos en `httpapi/handlers_ingredients_admin.go`; SQL en `queries/ingredient_merge.sql` + `make sqlc` | OK |
| II. Errores | Centinelas nuevos en `domain` (`ErrMergeBlocked`, `ErrMergeStale`, `ErrMergePresentationConflict`, `ErrIngredientMerged`), mapeados solo en `httpapi.Error`; `EGB02` lo traduce `store.DomainError`; la vista previa usa el `errPreviewRollback` existente | OK |
| III. Dinero / cantidades | Costos con `ConvertCost`, cantidades con `ConvertQty`/`ConvertRecipeQty` (Round4) y `ValidConvertedQty` antes de escribir; el servidor calcula todo, el cliente solo manda ids y factor; la vista previa declara qué es cada cifra (existencia por sucursal, costo por unidad base) | OK |
| IV. Test-first | Unitarios de `domain` contra los bordes (factor, choque de presentaciones, costo sin compras, nombres parecidos); integración de cada criterio de aceptación, de la carrera con una venta (R2), del movimiento tardío redirigido, de los rechazos `EGB02`, de RLS en los tres casos y de la 0104 contra respaldo real con dos empresas | OK |
| IV. Migración tolerante | Solo agrega: `kind` con default, `brand` nullable, índice más permisivo, triggers que solo actúan sobre orígenes unidos (no existen hasta que la API nueva una). La API anterior lee y escribe igual | OK |
| IV. Base local = producción | Todo bajo `gatobobah_app`; las funciones de trigger son `security definer` con empresa explícita y se prueban también bajo el rol de la app | OK |
| V. Seguridad | `RequirePermission(inventory.configure)` en `merge` y `similar`; `preview` solo `1` o ausente; ids de otra empresa → 404 por RLS; tope de 20 orígenes; `name` de `similar` validado (1–80), nunca un default | OK |
| VI. YAGNI | La unión no crea tabla (reusa `ingredient_replacements`); la única tabla nueva guarda «No son lo mismo», que el dueño eligió (maqueta C) y que sin persistir volvería a proponerse siempre; sin deshacer, sin regla de costo configurable (entrega 2), sin extensión nueva de Postgres | OK |
| VII. Comentarios / idioma | Identificadores en inglés; texto de interfaz y comentarios en español | OK |
| VIII. Puertas | Ver abajo | OK |
| Restricciones del producto | Hojas inferiores, `Picker` con buscador para elegir orígenes, controles ≥ 44 px, un solo estilo (piezas de vista previa compartidas con la 053) | OK (lo revisa `tablet-ui-reviewer`) |
| Roles híbridos | Permiso existente `inventory.configure`; ningún `RequireRole` nuevo | OK |

**Principio VIII — ¿se puede agregar después al mismo costo?**

- *Comparar marcas en el historial (entrega 2)*: **sí**, porque la presentación se mueve conservando
  su id y `expense_items.presentation_id` ya guarda con cuál se compró. Si la unión copiara la
  presentación en vez de moverla, las compras pasadas perderían su marca: por eso se mueve.
- *Deshacer una unión*: **sí**, al mismo costo: el traspaso queda ligado por `replacement_id`, la
  unión dice qué origen, qué factor y cuándo, y nada de la historia del origen se borra.
- *Más de un almacén por sucursal* (#65): el traspaso es por sucursal con su sucursal explícita, como
  el reemplazo; con almacenes se agregaría la columna al movimiento sin partir nada.
- *Regla de costo por empresa (entrega 2)*: el costo de la unión sale de una función de `domain`
  (`MergedCost`) que la entrega 2 sustituye por la regla configurada.

## Pantallas y maquetas

| Pantalla | Dónde vive | Reparto del alto (1024×600) | Componentes |
|---|---|---|---|
| **Unir insumos** (nueva, opción A) | Ficha del insumo → «Unir otros aquí» (con `inventory.configure`, activo, no preparado). Hoja inferior, paso 1: buscador; «Parecidos» (máx. 3, con «Ver todos») y «Todos» debajo; renglones de 52 px donde todo el renglón es el área de toque; pie fijo «Cancelar» / «Siguiente». Paso 2 (solo si algún elegido se mide distinto): «¿Cuánto equivale?», un renglón por elegido «1 pieza = ___ ml de Leche entera» con el teclado numérico de la app (como el del conteo), no el del sistema. Paso 3: la vista previa | Hoja a 92 dvh; chrome medido ≈ 56 encabezado + 52 buscador + 64 pie + 2 × 28 títulos = 228 px; quedan ≈ 320 px, 6 renglones. La equivalencia ya no quita renglones de la lista | `DrawerRoot`, buscador del `Picker`, renglones de 52 px, teclado numérico propio |
| **Vista previa de unir** (nueva) | Es el paso 3 de la misma hoja, no una hoja encima: tabla Qué/Antes/Después/Cambio agrupada (Existencia por sucursal · Recetas que pasan · Presentaciones · Costo por unidad base), pie fijo «Volver» (secundario, izquierda) / «Unir» (primario, derecha). «Volver» conserva lo elegido y las equivalencias. Un 409 «vuelve a intentar» se dice en la hoja sin perder nada. Si algo cambió: «Algo cambió mientras revisabas» y la vista previa nueva | Tabla con encabezado fijo, scroll en `maxH` dvh y `overflowX="auto"` propio | Piezas de `shared/preview/` extraídas de `EditPreviewSheet` (que debe pintar idéntico; sus pruebas son la regresión) |
| **Posibles duplicados** (nueva, opción C) | Almacén › Artículos → botón «Posibles duplicados» con badge N en la **segunda** fila de la barra (`ArticlesTab.tsx:76-81`, junto a los chips), `variant="outline"`; no se pinta con N = 0 ni sin `inventory.configure`. Hoja con la lista de grupos; tocar un grupo lo expande en su lugar: un renglón de 52 px por insumo con radio «Se queda», y debajo un renglón por par con «No son lo mismo» (ghost, 44 px, a ≥ 16 px de todo lo demás) y al pie «Revisar unión» → abre la hoja de unir directo en el paso 2 o 3 con lo elegido. «No son lo mismo» muestra «Par descartado · Deshacer» 5 s y solo entonces se guarda (el par es un hecho sin `delete`; deshacer es no mandarlo). Dos hojas como máximo, nunca tres; la de unir monta cerrada y abre en el render siguiente (gotcha de `CobrarSheet`) | Lista de grupos en scroll `flex=1 minH=0` | `DrawerRoot`, mismas filas |
| **Ficha de un insumo unido** (cambio) | Archivados → ficha | En lugar de «Reactivar», la franja «Unido a Leche entera el 3 nov · Abrir» (44 px). Sin «traspaso», «fusionado» ni nombres internos | `ArticleSheet` |
| **Presentaciones** (cambio) | Ficha → empaques | Un campo «Marca (opcional)» en el formulario; la marca junto al nombre en la lista, con una sola función `presentationLabel` («Caja 12 × 1 L · Marca A») que envuelve en vez de truncar, usada también en compra, entrega, conteo y merma. El conteo deja `key={p.name}` por la llave de id (con marca los nombres se repiten) | `PresentationsSheet`, `StockCountTab` y los selectores de compra (todos `Picker`/lista, ninguno `<select>`) |
| **Nuevo artículo** (cambio, opción B) | Almacén › Artículos › Nuevo → «Guardar» | Si hay parecidos, la hoja cambia a «¿Es alguno de estos?»: hasta 5 con «Abrir» (44 px) por renglón y scroll interno, «Volver» (regresa al formulario con lo escrito intacto) y «Crear de todos modos» (secundario, 44 px). Nada se agrega mientras se escribe (el teclado ocupa ~250 px) | `NewArticleSheet` |

Textos: «Se queda» / «Se une», «1 pieza = ___ ml», «Vuelve a intentar»; nunca «huella», «factor»,
«origen/destino» sin explicar ni nombres de columnas.

## Maquetas

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Unir insumos | [maqueta](https://claude.ai/artifact/1GjjjPnBWsex8uksiWtMCc) | A + C · Desde la ficha y además grupos sugeridos | Desde la ficha une con los parecidos a la vista; la lista de grupos hace que los >15 grupos se encuentren sin buscarlos uno por uno |
| Aviso de insumo parecido | [maqueta](https://claude.ai/artifact/JWgptn6r6py7z8otnfnJ6N) | B · Al guardar | Con el teclado abierto no hay alto para un aviso; cuesta un toque solo cuando hay parecido |

## Project Structure

### Documentation (this feature)

```text
specs/056-unir-insumos/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
├── checklists/requirements.md
└── tasks.md             # /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── migrations/0104_ingredient_merge.sql          # kind, brand, índice, funciones y triggers
├── queries/ingredient_merge.sql                  # nuevas: candados ordenados, mover presentación, recetas, costo
├── queries/ingredients_admin.sql                 # ArticleCostHistory/ArticleAudit leen los orígenes con un CTE recursivo; GetArticle: mergedInto
├── queries/ingredients.sql                       # ListPresentations/Create/Update con brand
├── internal/domain/ingredient_merge.go (+_test)  # ValidMergeRequest, PlanMergedRecipe, PlanMergedPresentations, MergedCost
├── internal/domain/similar_names.go (+_test)     # SimilarNames, DuplicateGroups
├── internal/domain/errors / audit.go             # centinelas y acciones nuevas
├── internal/store/errors.go, store.go            # EGB02 → ErrIngredientMerged; DomainError también en tx.Commit
├── internal/app/ingredient_merge.go              # MergeIngredients (preview|confirm), MergeTables
├── internal/app/unit_change.go                   # se niega con orígenes unidos
├── internal/app/articles.go / ingredients_admin.go / presentations.go
├── internal/httpapi/handlers_ingredients_admin.go, router.go, respond.go
└── internal/integration/ingredient_merge_test.go, migration_0104_test.go, unit_change_test.go

web/src/
├── shared/preview/ (PreviewGroup, ChangeRow, signed/tone)  # extraído de EditPreviewSheet
├── shared/presentationLabel.ts
├── features/backoffice/EditPreviewSheet.tsx       # usa shared/preview sin cambiar lo que pinta
├── features/backoffice/articles/MergeSheet.tsx, MergePreviewSheet.tsx, DuplicatesSheet.tsx (+tests)
├── features/backoffice/articles/ArticleSheet.tsx, NewArticleSheet.tsx
├── features/backoffice/PresentationsSheet.tsx, ExpenseDialog.tsx, AddLineSheet.tsx, ArrivedItemSheet.tsx, StockCountTab.tsx
├── features/waste/WastePage.tsx
├── api/backoffice.ts
├── shared/audit/labels.ts
└── e2e/unir-insumos.spec.ts
```

**Structure Decision**: web application existente (`server/` + `web/`). No se tocan
`server/internal/app/purchases.go` ni `purchaseqty.go`.

## Revisión de arquitectura (2026-11-01)

`db-architect` y `tablet-ui-reviewer` sobre este plan. Sin bloqueantes; todo lo ALTO y MEDIO quedó
incorporado en este plan, en [data-model.md](data-model.md) y en [research.md](research.md):

- A1: `EGB02` del trigger diferido nace en `tx.Commit`; los commits de `store.go` pasan por `DomainError`.
- A2: la cadena de uniones tiene tope y `check_ingredient_merge` impide ciclos y destinos inválidos.
- A3 (puerta VIII): `expense_items.presentation_brand` copia la marca al comprar, por trigger.
- A4: la guarda de rechazo también es diferida en las ocho tablas.
- M1: solo las funciones de trigger son `security definer`; el historial usa CTE bajo RLS.
- M2–M4: clasificación del respaldo anónimo, limpieza ordenada del Down, no reactivar un origen.
- Tableta: equivalencia en paso propio con teclado de la app, vista previa como paso de la misma
  hoja, grupo expandido en su lugar, «No son lo mismo» por par con «Deshacer», botón en la segunda
  fila, `presentationLabel` única, `key` por id en el conteo.
- Prueba pedida: la unión toma primero y la venta llega después (orden inverso), dos uniones a la vez,
  y un movimiento histórico del origen no se redirige al restaurar un respaldo.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Guardas en la base (redirigir, respaldo diferido, rechazar, validar la unión, no reactivar, copiar la marca) | Cerrar la carrera venta/unión y cubrir la API anterior y los caminos que no saben de uniones (R2, R3) | Validar en Go deja la carrera abierta (la venta ya leyó la receta) y obliga a tocar el camino de compras que está cambiando la 055 |
