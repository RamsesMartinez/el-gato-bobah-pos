# Implementation Plan: Densidad y tara de los artículos del almacén

**Branch**: `058-densidad-y-tara` | **Date**: 2026-11-02 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/058-densidad-y-tara/spec.md`

## Summary

Densidad por artículo (g/ml), capturada con una pesada o escrita; con ella, `domain.BaseQty`
convierte compras masa↔volumen y el cambio de unidad acepta masa↔volumen con el factor derivado de
la densidad. Cada renglón de compra fija la densidad con que convirtió la primera vez (snapshot), así
rehacer la pesada no recalcula lo ya recibido. Cada empaque guarda su tara y el conteo acepta «pesar con empaque»:
el servidor resta la tara y convierte con la densidad. Migración **0105** (solo agrega).

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose, shopspring/decimal; Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS por empresa; migración **0105** (libre: la última en develop es la
0104 y ninguna rama ni worktree abierto trae otra)

**Testing**: `go test` table-driven en `domain`; integración contra Postgres real
(`make test-integration`, respaldo restaurado con dos empresas, `appRoleStore` / `inTheThreeCases`);
vitest; Playwright 1024×600

**Target Platform**: API Linux en VM; POS web en tabletas 7–10" (1024×600)

**Project Type**: web (`server/` + `web/`)

**Performance Goals**: sin cambio medible: una lectura más de la fila del artículo que ya se lee

**Constraints**: lo ya convertido no cambia al rehacer la pesada; tolerancia a la API anterior en la
subida azul/verde; conteo a ciegas

**Scale/Scope**: ~300 artículos por empresa; decenas con densidad

## Constitution Check

| Principio | Cómo se cumple | Estado |
|---|---|---|
| I. Layering | Regla pura en `domain/density.go` (`ValidDensity`, `DensityFromWeighing`, `DensityFactor`, `NetFromGross`) y `purchaseqty.go` (`BaseQty` con densidad); orquestación en `app/density.go`, `app/unit_change.go`, `app/stock_counts.go`, `app/purchases.go`; handlers finos; SQL en `queries/*.sql` + `make sqlc` | OK |
| II. Errores | `ErrDensityChanged`, `ErrGrossBelowTare` en `domain`; `ErrUnitKindMismatch` con detalle `UnitKindMismatch{IngredientID, Name}`; mapeo solo en `httpapi.Error` | OK |
| III. Dinero / cantidades | Cantidades Round4, densidad 4 decimales validada en frontera; costo con `ConvertCost` existente; el servidor convierte, el cliente solo muestra con los mismos números que el servidor le entrega | OK |
| IV. Test-first | Unitarios de bordes (densidad fuera de rango, vacío ≥ lleno, cero, bruto < tara, ml sin densidad, longitud), integración de cada escenario y de «rehacer la pesada no mueve lo recibido» al corregir y al editar el gasto, RLS en los tres casos para las vistas que exponen densidad y tara, migración 0105 contra respaldo real | OK |
| IV. Migración tolerante | Solo columnas nulas y checks sobre ellas (R7) | OK |
| IV. Base local = producción | Todo bajo `gatobobah_app` | OK |
| V. Seguridad | `RequirePermission(inventory.configure)` para densidad y tara; `inventory.count` para el conteo; cuerpos estrictos (método desconocido o campo de más → 400); topes de rango y de envases | OK |
| VI. YAGNI | Sin tabla de historia (la bitácora basta), sin densidades genéricas, sin presentaciones en la otra clase | OK |
| VII. Comentarios / idioma | Identificadores en inglés; texto y comentarios en español | OK |
| VIII. Puertas | Ver abajo | OK |
| Restricciones del producto | Hojas inferiores, teclado numérico de la app, controles ≥ 44 px, un solo estilo; ningún `<select>` | OK (lo revisa `tablet-ui-reviewer`) |
| Roles híbridos | Solo permisos existentes; ningún `RequireRole` nuevo | OK |

**Principio VIII — ¿se puede agregar después al mismo costo?**

- *Densidad usada por renglón de compra*: **no**. Si no se copia al convertir, después de rehacer la
  pesada no hay forma de saber con qué densidad entró cada compra, y corregir ese renglón movería la
  existencia. Por eso se guarda hoy (`expense_items.density_g_per_ml`).
- *Historia de densidades*: **sí** al mismo costo: la bitácora guarda cada cambio con su pesada.
- *Bruto capturado en el conteo*: **no** (corregido en la revisión de arquitectura): se guarda en `stock_count_lines`.
- *Más de un almacén por sucursal* (#65): nada de esta spec asume un solo lugar.

## Pantallas y maquetas

| Pantalla | Dónde vive | Reparto del alto (1024×600) | Componentes |
|---|---|---|---|
| **Densidad** (nueva, maqueta A) | Ficha del artículo (g o ml) → renglón «Densidad · Opcional» con «Sin capturar» o «1.10 g/ml · por pesada · Ana, 2 nov», un ícono (i) de 44 px que despliega en su lugar para qué sirve con el ejemplo del galón (4.3 kg → 3,909 ml), y el botón «Capturar» / «Cambiar» (secundario: es opcional). Hoja inferior «Densidad · Opcional» con su propio (i), dos modos («Con una pesada» por omisión / «Escribir el valor», que ofrece g/ml o «Con la etiqueta»: peso neto y contenido, para quien no puede pesar), tres campos con el teclado numérico de la app, resultado en grande («1.10 gramos por mililitro») antes de «Guardar»; «Quitar densidad» como acción secundaria separada ≥ 16 px | Encabezado 56 + modo 52 + 3 campos × 64 + resultado 56 + pie 64 ≈ 420 px | `DrawerRoot`, segmentado del tema, teclado de la app |
| **Renglón de compra** (cambio) | Captura de gasto, agregar renglón, edición, por recibir | Una línea más bajo el renglón: «4,300 g ÷ 1.10 = 3,909 ml», o «Falta la densidad · Capturar la pesada» (botón 44 px) | Texto secundario del renglón; abre la hoja de densidad encima (máximo dos hojas) |
| **Cambiar en qué se mide** (cambio) | Ficha → `UnitChangeSheet` | Con densidad, g ↔ ml aparece como destino y la equivalencia se muestra fija («1 g = 0.7692 ml · sale de la densidad 1.30 g/ml»), sin campo. Sin densidad: el destino aparece deshabilitado con «Primero captura la densidad» y su botón | Misma hoja |
| **Empaques** (cambio, paso 1 de la maqueta de conteo) | `PresentationsSheet` | Campo «Pesa vacío (g)» con etiqueta «Opcional» y un (i) («solo si cuentan pesando el envase con el producto adentro»), solo en artículos g/ml; en la lista, «pesa vacío 120 g» junto al contenido | Formulario existente |
| **Conteo** (cambio, maqueta A) | `StockCountTab`, hoja de captura del artículo | Igual que hoy; con `grossWeighable`, al final un renglón plegado «▸ Pesar con empaque · Opcional · (i)». Al abrirlo (en su lugar, misma hoja): peso de la báscula (g/kg), empaque (si hay varios con tara, `Picker`), empaques (−/+ con número editable) y la cuenta «850 g − 2 × 120 g = 610 g ÷ 1.10 = 554.5 ml»; el total suma cerradas + suelto + pesado. Sin ningún empaque con tara, el renglón no existe | Captura existente |

Textos: «Densidad», «Pesa lleno», «Pesa vacío», «Contiene (ml)», «Pesar con empaque», «empaques»;
nunca «g/ml» sin decir «gramos por mililitro» la primera vez en la hoja, ni «tara» sin «pesa vacío».

## Maquetas

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Densidad | [maqueta](https://claude.ai/artifact/QNL9K3cWBb22tTen5QKmhY) | A · Hoja con dos modos, marcada «Opcional», con ícono (i) | Todo a la vista en una hoja, y la misma hoja sirve para el atajo desde la compra. El dueño pidió que se vea opcional y que el (i) explique para qué sirve con un ejemplo, sin texto fijo |
| Conteo por peso bruto | [maqueta](https://claude.ai/artifact/JDvpjYW7zyqdySg4jaK5bd) | A · Sección en la captura, plegada y marcada «Opcional», con ícono (i) | Es una forma avanzada: solo aparece si un empaque tiene «Pesa vacío», y plegada no le estorba a quien no pesa. El camino empieza en Empaques (campo opcional) |

## Project Structure

### Documentation (this feature)

```text
specs/058-densidad-y-tara/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
server/
├── migrations/0105_density_and_tare.sql
├── queries/ingredients_admin.sql      # SetArticleDensity, ClearArticleDensity, GetArticle con densidad
├── queries/ingredients.sql            # SearchArticles, ListPresentations/Create/Update/SetPresentationRaw con tare_g
├── queries/purchases.sql              # ListStockLines con densidad, SetExpenseItemDensity
├── queries/stock_counts.sql           # CountItems con densidad y tara
├── internal/domain/density.go (+_test)
├── internal/domain/purchaseqty.go (+_test)      # BaseQty con Density
├── internal/domain/ingredients.go (+_test)      # ValidUnitChange masa↔volumen
├── internal/domain/audit.go                     # acciones nuevas
├── internal/app/density.go                      # SetDensity / ClearDensity
├── internal/app/purchases.go                    # stockStates con densidad; fijarla al primer movimiento
├── internal/app/unit_change.go                  # factor derivado, DENSITY_CHANGED, conservar densidad y tara
├── internal/app/stock_counts.go                 # Gross
├── internal/app/presentations.go                # tareG
├── internal/httpapi/handlers_ingredients_admin.go, handlers_stock*.go, router.go, respond.go
└── internal/integration/density_test.go, migration_0105_test.go, + casos en unit_change y conteo

web/src/
├── features/backoffice/articles/DensitySheet.tsx (+test)
├── features/backoffice/articles/ArticleSheet.tsx, UnitChangeSheet.tsx (+test)
├── features/backoffice/expenseDraft.ts (+test)   # densityFactor / conversionWords
├── features/backoffice/ExpenseDialog.tsx, AddLineSheet.tsx, ArrivedItemSheet.tsx, PendingReceiptsTab.tsx, ExpenseDetailDialog.tsx, EditPreviewSheet.tsx
├── features/backoffice/PresentationsSheet.tsx
├── features/backoffice/StockCountTab.tsx, countEntry.ts (+test)
├── api/backoffice.ts
├── shared/audit/labels.ts
└── e2e/densidad-y-tara.spec.ts
```

**Structure Decision**: web application existente.

**Nota de sqlc**: `ingredients.company_id` y `ingredient_presentations.company_id` existen para sqlc
(la primera se agregó con `EXECUTE format()` en 0023, que sqlc no lee). La FK compuesta de
`density_set_by` se declara dentro de un bloque `do $$ … execute …$$` si `sqlc generate` truena con
«column does not exist»; ninguna consulta nombra `company_id`.

## Revisión de arquitectura (2026-11-02)

`db-architect` y `tablet-ui-reviewer` sobre este plan. Sin bloqueantes de principio VIII; dos ALTOS de
datos y tres de pantalla. Todo queda resuelto aquí (manda sobre research y data-model donde choquen):

**Datos**

- **A1 — quitar la densidad con una compra pendiente**: `PlanStockLines` convierte también lo pedido, así
  que un renglón masa↔volumen sin densidad fija dejaría el gasto entero sin poder tocarse. `ClearDensity`
  responde 409 `DENSITY_IN_USE` si hay un renglón vivo del artículo, sin presentación, de unidad de otra
  clase y con `density_g_per_ml` nulo. Mismo efecto en el traslape azul/verde (R7): el 400 de la API
  vieja es de todo el gasto, dura segundos y anular sí funciona.
- **A2 — la regla «solo g/ml» la impone la base**: trigger `before update of base_unit_id on ingredients`
  que, si la clase nueva no es masa ni volumen, pone en nulo las cuatro columnas de densidad y
  `tare_g` de sus presentaciones; `ingredient_presentations_before_write` (0092) rechaza `tare_g` en un
  artículo pieza/cm. Cubre la API vieja, scripts y el cambio de unidad nuevo.
- **M1 — candado real**: todo camino que aplica renglones ya toma `lockIngredients` (`for no key update
  nowait` sobre `ingredients`) antes de escribir; se exige que sea ANTES de `ListStockLines` (se verifica
  en cada llamador) y `SetDensity`/`ClearDensity` toman el mismo candado `nowait`. `SetExpenseItemDensity`
  recibe el decimal exacto que usó `BaseQty`, nunca una subconsulta.
- **M2 — `UpdateStockLine`**: si cambia el artículo o la unidad deja de ser de otra clase, la densidad
  fija vuelve a nulo (el renglón ya se revirtió antes de cambiar de artículo). Predicado de fijado:
  masa↔volumen, sin presentación, delta ≠ 0. Una corrección a recibido 0 no la borra.
- **M3 — vista previa de edición (053)**: la huella incluye la densidad efectiva de cada renglón
  masa↔volumen sin densidad fija; rehacer la pesada entre vista previa y confirmación → 409
  `EXPENSE_EDIT_STALE`.
- **M4 — FK**: `(company_id, density_set_by) references users (company_id, id)` con `on delete no action`
  (un `set null` anularía `company_id`), declarada siempre dentro de un `do $$ execute $$` porque sqlc no
  ve `ingredients.company_id`. Método: `weighing | label | manual` en todos los documentos. Todo escritor
  llena las cuatro columnas.
- **M5 — Down**: se niega si hay densidad fija en renglones, renglones vivos masa↔volumen, densidades,
  taras o datos de pesada en conteos; `guardedDownCleanup` pone todo en nulo en la copia de prueba.
  `density_method` va a `noPersonalData`.
- **Tara**: `check (tare_g > 0 and tare_g <= 100000)` y el mismo tope en `domain`. Se escribe con
  `SetPresentationTare` aparte, así un `PUT` sin `tareG` no la toca y `null` la quita.
- **Unión de insumos (056)**: si el destino no tiene densidad y los orígenes con densidad coinciden, el
  destino la recibe (con evento); si difieren, la vista previa de la unión lo dice y no se copia. Una
  presentación del destino sin tara recibe la del duplicado que se archiva.
- **Puerta VIII — lo pesado en el conteo**: se guarda. `stock_count_lines` agrega `gross_g`,
  `gross_containers`, `tare_g_used` y `density_used` (nulas). Sin ellas, un faltante que alimenta la
  pérdida (spec 039) no se puede distinguir después de una pesada con tara mal capturada. Esto corrige
  el renglón «Bruto capturado en el conteo» de la tabla del principio VIII de arriba: la respuesta es
  **no**, y por eso se guarda.
- **El conteo no adivina**: la captura manda `seenTareG` y `seenDensity`, lo que la pantalla usó para la
  cuenta; si ya no son los vigentes, 409 `COUNT_INPUTS_CHANGED` y la hoja recarga el artículo. La
  respuesta trae el neto calculado y es lo que se muestra como contado.

**Pantallas**

- **Hoja de densidad**: patrón de `AddLineSheet` — `NumericKeypad` en el pie solo con un campo activo
  («Listo»), cuerpo con scroll y `useKeypadField`. Alto con teclado ≤ 90 dvh; cerrado ≈ 430 px.
- **Conteo**: `EntryFields` con `overflowY="auto" minH={0}` y «Total contado» fijo fuera del scroll. Al
  abrir «Pesar con empaque», «Abierto o suelto» se oculta y se limpia (excluyentes: lo abierto se pesa
  o se teclea, no las dos). En «Uno por uno» el renglón plegado abre la hoja completa de captura del
  artículo en vez de crecer la tarjeta. Prueba medida a 1024×600 con 1 y 4 empaques, abierta y cerrada.
- **Palabra única**: «empaque» en todo (el término que ya usa el producto); el renglón se llama «Pesar
  con empaque» y el campo «Pesa vacío (g)». El texto del (i) dice «no lo sumes también como suelto».
- **Capas**: dos como máximo. Desde un gasto, el atajo vive en el renglón de `ExpenseDialog` (diálogo +
  hoja de densidad); dentro de `AddLineSheet` solo se pinta el aviso. Desde el cambio de unidad, «Primero
  captura la densidad» cierra `UnitChangeSheet` y abre la hoja de densidad desde la ficha. La hoja de
  densidad monta cerrada y abre en el render siguiente (gotcha de `CobrarSheet`) y al guardar solo
  invalida el artículo: la capa de abajo no se desmonta ni pierde lo capturado.
- **Renglón de compra**: la conversión o el aviso solo en renglones masa↔volumen, en la misma fila, con
  un botón de 44 px por artículo aunque tenga varios renglones; el aviso dice «para pasar kg a ml».
- **Ficha**: el renglón de densidad va pegado a «Se mide en», solo en g/ml; el (i) despliega debajo sin
  mover el botón. Se lee «1.10 gramos por ml»; nada de `weighing`/`label`/`manual` en pantalla ni en la
  bitácora. «Quitar densidad» con confirmación en línea («¿Quitar? Sí / No»), como `NoHay`.
