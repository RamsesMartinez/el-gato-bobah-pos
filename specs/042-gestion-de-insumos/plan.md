# Implementation Plan: Gestión de artículos del almacén

**Branch**: `042-gestion-de-insumos` | **Date**: 2026-10-19 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/042-gestion-de-insumos/spec.md`

## Summary

Almacén › Artículos se vuelve el catálogo de artículos. Tendrá buscador, filtros y «Nuevo
artículo», y cada artículo tendrá una ficha editable con:

- el historial de costos y su gráfica de tendencia;
- archivar y reactivar;
- el cambio de unidad, directo o por reemplazo;
- los empaques con su equivalencia.

**Lo que se agrega:**
- **Dos tablas nuevas** (migración 0096):
  - `ingredient_cost_corrections`: las correcciones de costo a mano.
  - `ingredient_replacements`: un artículo que se reemplaza por otro en otra unidad, con sus dos
    movimientos ligados.
- **Un permiso**: `inventory.view`.
- **Diez acciones de bitácora.**

**Lo que se reutiliza:** `ingredients.is_active`, `ingredient_presentations`, `count_places` y los
movimientos de compra, que ya guardan su costo.

**Artículos sin querer.** Los artículos que hoy nacen al tocar la unidad dejan de crearse aparte. El
gasto y la receta mandan el artículo nuevo dentro de su propio guardado, en la misma transacción.

**Aviso del conteo.** Lo decide el servidor al capturar. Responde «confirma» sin decir contra qué
comparó, para que el conteo siga a ciegas.

## Technical Context

**Language/Version**: Go 1.27.2 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query. La gráfica es un SVG
propio de unas 60 líneas, sin librería nueva (principio VI).

**Storage**: PostgreSQL 16 con RLS por empresa. Migración nueva `0096_ingredient_management.sql`.

**Testing**:
- Dominio table-driven: referencia del aviso del conteo, conversión de unidad, clasificación de
  movimientos y validación de la ficha.
- Integración bajo `appRoleStore` con `inTheThreeCases`.
- Migración probada sobre el respaldo restaurado con dos empresas.
- vitest de las tres pantallas.
- Playwright a 1024×600.

**Target Platform**: tabletas de 7 a 10 pulgadas (1024×600)

**Project Type**: web (server + web)

**Performance Goals**:
- La lista de ~400 artículos sale en una consulta y filtra en el servidor.
- El historial de costos se pagina por llave (20 por página).

**Constraints**:
- Sin selector nativo y controles de 44 px o más.
- Ningún artículo creado por una captura cancelada.
- El pasado de compras, ventas y mermas no se reescribe.

**Scale/Scope**: cientos de artículos por empresa y decenas de compras por artículo al mes.

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | Reglas puras en `domain/ingredients.go` (validar ficha, `UnitChangePath`, `ConvertQty`) y en `domain/count_sanity.go`. Servicio en `app/ingredients_admin.go` con `store.WithTx`. Handlers finos y SQL en `queries/ingredients_admin.sql` |
| II Errores | `ErrValidation` (nombre, unidad, equivalencia), `ErrConflict` (nombre repetido con id y si está archivado; ficha que cambió; receta activa; conteo abierto; venta en curso con `nowait`) y `ErrNotFound` (otra empresa) |
| III Dinero | Costo inicial y corregido con `ValidMoney` y `Round4` por unidad base. La corrección no recalcula ventas pasadas. El historial declara qué incluye: compras recibidas y correcciones, no el costo calculado de un preparado |
| IV Tests | Dominio primero. Integración en los tres casos para todas las lecturas nuevas. `TestEveryCompanyTableIsIsolated` recoge las dos tablas. La migración se prueba con dos empresas. `TestEveryAuditActionIsProduced` cubre las diez acciones. Un test por cada camino de FR-003 (lista, gasto, receta al agregar insumo y «Crear una nueva») que falla si queda un artículo creado |
| V Seguridad | `RequirePermission(inventory.view)` para leer e `inventory.configure` para escribir. Ningún `RequireRole` nuevo. El aviso del conteo no expone la referencia. Llaves compuestas con `company_id` en las tablas nuevas |
| VI YAGNI | Sin librería de gráficas ni tabla de historial: el historial sale de `stock_movements` (compras) más las correcciones |
| VII Idioma | Tablas, columnas, rutas y acciones en inglés. Texto de pantalla en español |
| VIII Puertas | Ver abajo |
| Tableta | Ficha en hoja a pantalla completa. `Picker` para unidad, categoría y lugar. Teclado del conteo con selector de unidad de 48 px |
| Migración tolerante | Solo agrega tablas y valores: la API anterior no las lee y sigue funcionando durante el traslape |

### Pregunta VIII: ¿se puede agregar después al mismo costo?

- **Varios almacenes (#65).** No se cierra. Dos operaciones mueven existencias y las dos lo hacen
  sobre las filas de `stock_levels` de cada sucursal, sin suponer una sola:
  - La existencia inicial entra a la sucursal que resuelve `current_branch_id()`.
  - El reemplazo traspasa sucursal por sucursal.

  Cuando exista `warehouse_id`, las dos lo toman igual que el conteo.
- **Equivalencias de compra (#112).** No se cierra. Los empaques siguen siendo del artículo. Una
  marca o un código de barras se ligará al empaque o al artículo en otra tabla, sin tocar estas.
- **Snapshot del nombre.** Los movimientos y los renglones de compra muestran el nombre actual del
  artículo, así que renombrar cambia cómo se ven en pantalla, pero no el dato. Las ventas no cambian
  porque `order_lines` guarda su propio nombre. Se acepta.
- **Reversibilidad del cambio directo de unidad** (decisión del dueño, 2026-10-19). Reescribe los
  ajustes, los renglones de conteo, las recetas, los empaques y la existencia del artículo. El
  evento `ingredient.unit_changed` guarda cada valor y unidad originales (`before`), lo bastante
  para revertir a mano o con una herramienta futura. No se construye el «deshacer».

## Pantallas y maquetas

| Pantalla | Dónde vive | Cómo se reparte el alto de 1024×600 |
|---|---|---|
| **Lista de artículos** | Almacén › Artículos (pestaña que ya existe). Arriba, en una fila, el buscador (artículo o empaque) y «+ Nuevo artículo»; debajo, una fila de chips: «Lugar: Todos ›» (hoja con los lugares y «Sin lugar»), el par «Activos / Archivados», «Sin costo» y «Existencia negativa». La tabla tiene artículo (con «Se mide en…»), existencia (las negativas marcadas), lugar, costo («Sin costo» marcado) y empaques. «Nuevo artículo» abre una hoja con nombre * y en qué se mide * obligatorios; existencia inicial, costo, lugar, mínimo y empaques son opcionales. Si el nombre ya existe, avisa ahí mismo, con «Reactivar» cuando el artículo está archivado | Fila de búsqueda de 44 px y fila de chips de 44 px (~100 px con separaciones); caben 8 renglones de 50 px |
| **Ficha del artículo** | Hoja a pantalla completa al tocar un renglón. Encabezado: «‹ Artículos», nombre, «Archivar» (rojo, separado de Guardar) y «Guardar». **Columna izquierda**, desplazable: nombre, categoría ›, lugar ›, mínimo, «Se mide en … · Cambiar ›» y la sección Empaques con su explicación («Cómo lo compras…»), cada uno como «Bolsa · 5 kg = 5,000 g» con Editar, y «+ Agregar empaque» (hoja con sugerencias solo de la misma clase de unidad). **Columna derecha**: costo actual grande con «Corregir costo», un indicador de tendencia («▲ Sube 3 %» o «▼ Baja», contra la compra más vieja de las últimas 20), la gráfica de las últimas 20 (SVG de ~140 px de alto, etiquetas de 13 px y la última cifra en 14 px, las correcciones como punto hueco) y debajo el historial desplazable (fecha, proveedor o «Corrección a mano · quién», «$30 por Bolsa · 5 kg» y costo por kg). Muestra 20 y «Ver más» trae las 20 anteriores (FR-006); con menos de 2 compras no hay indicador de tendencia. En el encabezado, «Bitácora» abre los eventos del artículo (FR-016: quién, cuándo y qué cambió, de 20 en 20). «Se mide en … · Cambiar ›» lleva a dos pasos: (1) la clase de unidad nueva y la equivalencia, y si el artículo tiene compras o ventas avisa que se crea uno nuevo y este se archiva; (2) una tabla de existencia por sucursal, recetas y empaques con «hoy → en el nuevo», editable, y «Crear el nuevo y archivar este». Sin compras ni ventas, el paso 2 dice «Cambiar» y convierte el mismo artículo Archivar abre la revisión: existencia, recetas activas y, si las hay, «Quítalo de esas recetas antes de archivar» | Encabezado de 44 px; dos columnas de ~430 px que llenan el resto; unas 6 compras visibles sin desplazar |
| **Nuevo artículo** | Hoja corta desde «Nuevo artículo» (la de la maqueta de la lista, sin empaques ni categoría): nombre *, en qué se mide *, existencia inicial, costo, lugar (`Picker`) y mínimo; encabezado fijo con Cancelar y Guardar. Al guardar abre la ficha con «Agregar empaque» a la vista | Hoja de ~360 px; el encabezado no se desplaza |
| **Teclado del conteo** | Almacén › Conteo, al tocar un artículo. **Izquierda**: un renglón por empaque cerrado con «−», la cantidad y «+» (52 px); «Abierto o suelto» con su casilla y el selector de unidad (g o kg, ml o L, cm o m), que abre en la unidad base. Un artículo en piezas con empaque aproximado por peso ofrece pz, g y kg: lo pesado se ve «≈ N pz» junto a la casilla y el total sale con «≈»; se guarda en piezas; el total en 40 px con la unidad del mismo tamaño. **Derecha**: teclado de 60 px y «Guardar». Con más de 3 empaques, el total va fijo arriba y los contadores se desplazan debajo. Si el servidor responde `needsConfirm`, una ventana muestra la cifra en grande con «Corregir» y «Sí, guardar» | Encabezado de 44 px; contadores de 56 px; teclado de 4 × 60 px más Guardar de 56 px |

### Maquetas

El código de cada maqueta está en [maquetas/](maquetas/).

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Lista de artículos | https://claude.ai/artifact/XaiVPWAGVKzpU5eCtHvtRh | A: filtros en chips | Cada filtro es un toque y siempre se ve qué está puesto. Ajustes del dueño: empaques opcionales al crear, «Reactivar» si el nombre está archivado, obligatorios marcados, «Sin lugar» en el filtro de lugar y «Activos / Archivados» |
| Ficha del artículo | https://claude.ai/artifact/HnMD8yqMYLQTMpDhsYdAGe | A: dos columnas | El costo y su gráfica se ven mientras se edita. Ajustes del dueño: indicador de tendencia, etiquetas legibles, historial de 20 con «Ver más» de 20 en 20, tendencia solo con 2 compras o más, bitácora del artículo y reemplazo guiado en dos pasos |
| Teclado del conteo | https://claude.ai/artifact/QxfyGDwfrby4md58ACjE5c | A: empaques con + y − | Los empaques cerrados se cuentan a toques y la unidad de lo suelto está junto al número. Ajustes del dueño: ejemplos con último conteo y en piezas (tope 5,000); con más de 3 empaques, el total queda fijo arriba |

## Capas

| Pieza | Capa | Dónde |
|---|---|---|
| Validar ficha (nombre 1–60, mínimo ≥ 0, costo con `ValidMoney`); `UnitChangePath(movimientos) → direct \| replace`, donde solo `compra`, `venta`, `merma`, `produccion` y `cancelacion` bloquean y `ajuste` no; `ConvertQty(qty, factor)` | domain | `server/internal/domain/ingredients.go` |
| `CountSanity(captured, ref)`: referencia en orden (esperado > 0, último conteo cerrado > 0, 10 × el empaque más grande, tope por unidad de 50 kg, 50 L, 50 m o 5,000 piezas). Contar 0 nunca avisa por «menor». Avisa también por debajo de 1/100 del empaque más chico | domain | `server/internal/domain/count_sanity.go` |
| Permiso `inventory.view` en `AllPermissions` y en gerente | domain | `permission.go`, `web/src/app/permissions.ts` |
| Diez acciones nuevas en la lista blanca, con sus etiquetas | domain + web | `domain/audit.go`, `web/src/shared/audit/labels.ts` |
| `ingredient_cost_corrections`, `ingredient_replacements`, RLS, grants y dos índices | migración | `server/migrations/0096_ingredient_management.sql` |
| Lista con filtros, ficha, actualizar con `updated_at` esperado, archivar o reactivar, revisión de archivado, historial por llave, sugerencias de empaque por tipo de unidad y las conversiones del cambio de unidad | sqlc | `server/queries/ingredients_admin.sql` |
| Servicio con crear, editar, corregir costo, archivar, reactivar, cambiar unidad (vista previa y confirmar) e historial | app | `server/internal/app/ingredients_admin.go` |
| El gasto y la receta aceptan `newArticle` dentro de su guardado. Se quita la llamada aparte a `POST /ingredients` | app + web | `app/expenses.go`, `app/composition.go`, `ExpenseDialog.tsx`, `CompositionSheet.tsx` |
| Capturar en el conteo devuelve `needsConfirm` sin la referencia, y el cliente reenvía con `confirmed: true` | app + web | `app/stock_counts.go`, `StockCountTab.tsx`, `countEntry.ts` |
| Bitácora del artículo: `audit_events` por `entity_type = 'ingredient'` y su `entity_id`, más los de sus empaques; paginada por llave | sqlc + app | `queries/ingredients_admin.sql`, `app/ingredients_admin.go` |
| Rutas `/stock/articles/*` | httpapi | `handlers_ingredients_admin.go`, `router.go` |
| Lista, ficha, gráfica y hoja de unidad | web | `features/backoffice/articles/ArticlesTab.tsx`, `ArticleSheet.tsx`, `CostTrend.tsx`, `UnitChangeSheet.tsx` |

## Bordes (se piensan antes del código)

| # | Borde | Cómo se cierra | Test |
|---|---|---|---|
| E1 | Cancelar o fallar el gasto o la receta con un artículo nuevo | El artículo viaja dentro del guardado y se crea en su transacción | Integración por camino (4) y vitest: cancelar no llama a la API |
| E2 | Nombre repetido, también de un archivado | Candado 28037 e `IngredientNameTaken` con `is_active`. Responde 409 con el id y `archived`; la pantalla ofrece «Reactivar» | Integración |
| E3 | Dos personas editan la ficha | `update … where updated_at = $expected`: 0 filas da 409 «cambió mientras la veías» | Integración con dos escrituras |
| E4 | Archivar con receta activa o conteo abierto que lo capturó | Se revisa bajo `for update` del artículo y responde 409 con la lista | Integración |
| E5 | Archivar con compra por recibir | Se avisa y se permite. Recibir después suma existencia sin reactivar | Integración |
| E6 | Cambio directo con una venta en curso | `for update nowait` del artículo y de sus `stock_levels`, y se vuelve a revisar `UnitChangePath` dentro del candado: 409 «vuelve a intentar» | Integración con dos transacciones |
| E7 | Cambio directo: existencia y ajustes | Existencia × factor; cada ajuste y cada renglón de conteo × factor (`unit_cost` ÷ factor). Recetas y empaques convertidos, o la cifra que el dueño corrigió. Originales en `before` | Integración: la suma de movimientos sigue igual a la existencia |
| E8 | Reemplazo | Artículo nuevo, `ajuste` de salida en el viejo y de entrada en el nuevo por sucursal, ligados en `ingredient_replacements`; `recipe_items` pasan al nuevo con la cantidad convertida; el viejo se archiva. Una transacción | Integración |
| E9 | Cambio con conteo abierto que tiene el artículo | 409 hasta cerrar o descartar | Integración |
| E10 | Entre kg y g (mismo tipo) | No es cambio de unidad base: la base es g. No se ofrece | Dominio |
| E11 | Corregir el costo de un preparado | 409: lo calcula la receta | Integración |
| E12 | Existencia inicial con dos sucursales sin selector | `BRANCH_AMBIGUOUS` y no se crea nada | Integración |
| E13 | Referencia del aviso con esperado ≤ 0 | Cae al último conteo, al empaque y al tope, en ese orden | Dominio, tabla con los cuatro casos más 0 y 1/100 |
| E14 | Sugerencias de empaque | Filtradas por `unit_kind` del artículo, con su cantidad | Integración: el artículo en L no ve «Bolsa · 15 kg» |
| E15 | Bajar la migración | Down se niega si hay correcciones o reemplazos (como 0089, 0092 y 0094); la salida es el respaldo. Quita el índice de la bitácora y la columna de los movimientos | Migración de ida y vuelta; las pruebas que bajen la crean con empresas antes |
| E16 | Gráfica con 1 compra o con 0 | Con 0 no se dibuja. Con 1 sale un punto con su cifra | vitest |

## Revisión after_plan (2026-10-19)

`/revision-de-arquitectura` con `db-architect` y `tablet-ui-reviewer`: cambios requeridos. Lo
aplicado (decisiones del dueño marcadas):

**Bloqueantes**
- **B1 — versión anterior corriendo.** El aviso del conteo solo se evalúa si la petición trae
  `confirmable: true`; sin él, el servidor guarda como hoy. Con él, la respuesta es 409
  `NEEDS_CONFIRM`, que un cliente viejo trata como error. `POST /ingredients` se conserva una subida
  más y se borra en la siguiente. Test: petición sin el campo y con cifra absurda, guarda.
- **B2 — existencia contra movimientos.** Tras convertir, `stock_levels.on_hand` de cada sucursal se
  fija a la suma de sus movimientos convertidos, dentro del mismo candado. Test con un factor que no
  divide exacto (1/150 y 30 ajustes).
- **B3 — qué convierte y qué bloquea.** Se convierten existencia, ajustes, renglones de conteo,
  `recipe_items`, empaques, `min_stock`, `current_cost` (÷ factor) y `yield_qty`. Bloquean con 409
  y lista de pendientes: renglones de compra por recibir y mermas `pending` (el aviso dice cuáles).
  `supplier_items` no guarda cantidades en la base: no se toca. **Orden**: candado del artículo →
  `ingredients.base_unit_id` → recetas (con su `unit_id` nuevo) → empaques (se recalcula
  `is_approximate`; uno que deja de cuadrar se archiva —`active = false`, nunca se borra— con su nombre y cantidad originales en la bitácora, y se lista en el paso 2) → movimientos y conteo → existencia.
  Test que recorre las FK hacia `ingredients` y truena si aparece una tabla no clasificada, y uno por
  cambio de clase (masa↔pieza, volumen↔pieza). La conversión entre masa y volumen no se ofrece.
  Excepción declarada en Complexity Tracking: reescribe movimientos, contra la regla de la 038.
  `before` de la bitácora: un evento por tabla con `batch_id` común, para no crecer sin tope.
- **B4 — Down.** Ver E15.

**Alto y medio (base de datos)**
- Índice `audit_events (company_id, entity_type, entity_id, occurred_at desc, id desc)` en la 0096.
  La bitácora del artículo junta los eventos del artículo y de sus empaques por
  `ingredient_presentations_by_ingredient`; se revisa con `explain`.
- Historial de costos: solo `compra` con `quantity > 0`, `unit_cost` no nulo, `expense_item_id` no
  nulo y sin regalos; manda la fecha del movimiento en las dos ramas. Índice parcial
  `stock_movements (company_id, ingredient_id, created_at desc, id desc) where movement_type = 'compra'`.
- `stock_movements.replacement_id` nullable con FK compuesta a `ingredient_replacements` e índice
  parcial; los dos ajustes del reemplazo llevan `branch_id` explícito. `ingredient_replacements` con
  `check (old <> new)`, `unique (id, company_id)` e índice por `new_ingredient_id`.
- El artículo del reemplazo nace con `current_cost / factor` y deja una fila en
  `ingredient_cost_corrections`.
- FK de las tablas nuevas `no action`, no `restrict` (borrar una empresa en cascada no truena).
  `user_id` con FK `(company_id, user_id)` a `users`. La corrección pone `cost_source = 'manual'` en
  la misma transacción; una compra posterior lo sobrescribe.
- Equivalencia con tope `ValidQty`; una que desborde la existencia convertida da 400.
- `inTheThreeCases` por consulta nueva: lista, ficha, historial, bitácora, `archive-check` y
  sugerencias.

**Alto y medio (pantalla)**
- La lista muestra ~6 renglones con el encabezado de Almacén: «Lugar» y «Activos / Archivados» suben
  a la fila del buscador y en chips quedan «Sin costo» y «Existencia negativa» (~7 renglones). Se
  mide a 1024×600 contra la pantalla real.
- Conteo: tocar la cantidad de un empaque lleva el `NumericKeypad` a ese renglón; «+» y «−» ajustan
  de uno en uno (decisión del dueño). «Guardar» sin captura no guarda y «No hay» registra el cero
  (decisión del dueño). Tras guardar, aviso no bloqueante y regreso a la lista del conteo.
- Cambio de unidad, paso 2: pie fijo con «‹ Atrás» y el botón de confirmar; renglones agrupados
  (Existencia, Recetas con «N recetas convertidas» desplegable, Empaques), los que no convierten
  exacto primero; «Revisar» deshabilitado sin clase y equivalencia > 0.
- Archivar con recetas activas: cada receta abre su `CompositionSheet` y regresa a la revisión; sin
  bloqueos, el mismo diálogo confirma.
- «Archivar» pasa al pie de la columna izquierda de la ficha; el encabezado queda «‹ Artículos»,
  «Bitácora» y «Guardar». Salir con cambios sin guardar pregunta «¿Descartar cambios?».
- Ficha de pie (< 800 px de ancho): una columna con la gráfica arriba; el SVG con `viewBox` y
  `width="100%"`; columnas con `Page fill` + `flex="1" minH={0}`.
- Cantidades con número y selector de unidad (no texto «5 kg»), con `NumericKeypad` o
  `inputMode="decimal"`. Lugar, categoría y filtro de lugar con `Picker`.
- El servidor entrega costo por unidad, lo pagado por empaque, el porcentaje de tendencia (contra
  la más vieja de las últimas 20) y las piezas de un peso; la pantalla no calcula.
- «Corregir costo» no aparece en un preparado. La fila de costo envuelve en dos renglones si no cabe.
- Se quita «También se ve en Negocio › Bitácora»: esa pantalla no filtra por artículo.
- El empaque aproximado («Kilo ≈ 400 pz») se captura en la ficha, en Empaques, con «Se pesa
  también» y su equivalencia.

## Project Structure

```text
specs/042-gestion-de-insumos/
├── plan.md, research.md, data-model.md, quickstart.md, contracts/api.md
└── tasks.md            # /speckit-tasks

server/
├── migrations/0096_ingredient_management.sql
├── queries/ingredients_admin.sql, stock_counts.sql, expenses.sql, composition.sql
├── internal/domain/ingredients.go, count_sanity.go, permission.go, audit.go (+ _test)
├── internal/app/ingredients_admin.go, expenses.go, composition.go, stock_counts.go
├── internal/httpapi/handlers_ingredients_admin.go, router.go
└── internal/integration/ingredients_admin_test.go, migration_0096_test.go

web/src/
├── features/backoffice/articles/ (ArticlesTab, ArticleSheet, CostTrend, UnitChangeSheet)
├── features/backoffice/StockPage.tsx, StockCountTab.tsx, countEntry.ts, ExpenseDialog.tsx
├── shared/CompositionSheet.tsx, shared/audit/labels.ts
└── api/backoffice.ts
```

**Structure Decision**: monorepo existente, sin paquetes nuevos.

## Complexity Tracking

| Excepción | Por qué | Alternativa descartada |
|---|---|---|
| El cambio directo de unidad reescribe ajustes y renglones de conteo pasados: excepción a la regla de la 038 («el almacén se mueve por diferencia, nunca editando un movimiento») | Decisión del dueño (2026-10-19). Los originales quedan en la bitácora con `batch_id` | Solo el reemplazo: deja casi todos los artículos cargados en octubre sin cambio directo |
