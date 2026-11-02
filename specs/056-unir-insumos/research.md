# Research: Un insumo con varias marcas (entrega 1)

Cada decisión con lo que se evaluó. Lo que ya existe y se reusa se nombra con su símbolo.

## R1. Dónde se registra una unión

**Decisión**: en `ingredient_replacements` (0096), con una columna nueva `kind`
(`'unit_change'` por omisión, `'merge'`). Cada origen es un renglón: `old_ingredient_id` = origen,
`new_ingredient_id` = destino, `factor` = cuánto del destino vale una unidad base del origen (1 si se
miden igual). El traspaso de existencia se liga con la columna que ya existe,
`stock_movements.replacement_id`.

**Por qué**: la 042 ya modela «este artículo quedó archivado y su sucesor es aquel, con esta
equivalencia», con RLS, llaves compuestas por empresa, `unique (company_id, old_ingredient_id)` (un
insumo se archiva por sucesión una sola vez) y la liga de movimientos. Una unión es exactamente eso,
con N orígenes. Una tabla nueva duplicaría las llaves, las políticas y la prueba de aislamiento.

**Alternativas**: tabla `ingredient_merges` propia (más explícita, pero duplica todo lo anterior y
obliga a dos caminos para «¿de quién es sucesor este insumo?»); columna `merged_into_id` en
`ingredients` (no guarda factor, ni quién ni cuándo, y no liga el traspaso).

**Tolerancia a la versión anterior**: `kind` con default; la API vieja inserta reemplazos sin
mandarla y quedan como `unit_change`, que es lo que son.

## R2. Concurrencia: una venta o compra en curso sobre el origen

El riesgo concreto: una venta lee la receta (origen), la unión confirma, y la venta inserta su
movimiento en el origen ya archivado. El trigger de existencias suma sin releer, así que el origen
queda en −100 y el destino nunca lo resta. La 042 tiene el mismo hueco en el reemplazo y no lo cierra.

**Decisión**, en tres capas:

1. **Candados en la unión**, en este orden: `LockCompositionEdits` (el candado de captura de la
   empresa, antes que cualquier fila, como `ChangeUnit`); los insumos (destino y orígenes) con
   `for update nowait` **ordenados por id** (dos uniones a la vez no se cruzan); sus existencias con
   `LockArticleStockNowait`. `for update` choca con el `for key share` que toma la llave foránea de
   cualquier movimiento en curso y con el `for no key update` de `lockIngredients` de las compras:
   una venta o compra a medias hace que la unión responda 409 «vuelve a intentar», sin esperar.
2. **Un movimiento que llega a un origen unido se redirige al destino** (trigger `before insert` en
   `stock_movements`, función `redirect_merged_ingredient_movement`): sigue la cadena de uniones
   (`kind = 'merge'`), multiplica la cantidad por el producto de los factores, divide el costo
   unitario y cambia `ingredient_id`. Cubre la cancelación de un pedido vendido antes de unir (repone
   en el destino), la versión anterior de la API durante una subida, y cualquier camino que no sepa
   de uniones. El traspaso de la propia unión lleva `replacement_id` y no se redirige.
3. **Respaldo diferido para la carrera** (y el mismo respaldo en las tablas de R3: una fila que
   espera la llave foránea a que la unión confirme tiene la misma foto vieja): un `constraint trigger ... deferrable initially deferred`
   en `stock_movements` revisa al confirmar la transacción que el movimiento no quedó en un origen
   unido (sin `replacement_id`). El caso que atrapa: la venta empezó su `insert` antes de que la unión
   confirmara (el `before` no la vio), esperó la llave foránea y entró al origen. Al confirmar la
   venta, el trigger diferido ve la unión ya confirmada y truena con `EGB02`; la app responde 409 y
   el reintento entra por la capa 2. Como nace en `tx.Commit`, donde `store.translating` no mira, los
   commits de `store.go` pasan por `DomainError` (revisión de arquitectura, A1).

**Por qué no solo la capa 1**: el `nowait` cubre la venta que tomó el insumo ANTES que la unión; la
que lo pide DESPUÉS espera a que la unión confirme y entra al origen. **Por qué no solo la capa 2**:
el `before` corre con la foto del inicio de la sentencia y no ve una unión que confirma mientras la
sentencia espera. **Por qué diferido y no un `after` inmediato**: el orden de los triggers `after`
es alfabético y compite con el de la llave foránea; uno diferido corre al confirmar, siempre después.

**Alternativas**: rechazar toda venta sobre un origen unido (rompe la cancelación de pedidos viejos:
`ReponeInventario` inserta en el origen); `for share` de las recetas al vender (toca el camino de
venta de punta a punta, y la 055 está tocando el de compras).

## R3. Lo que no es un movimiento: rechazar, no redirigir

**Decisión**: `recipe_items`, `expense_items`, `ingredient_presentations`, `supplier_items`,
`ingredient_purchase_formats`, `waste_records`, `stock_count_lines` e `ingredient_cost_corrections`
llevan un trigger `before insert or update of ingredient_id` que truena con `EGB02` si el insumo
nuevo es un origen unido (y en un update, si cambió). `store.DomainError` lo traduce a
`domain.ErrIngredientMerged` → 409 `INGREDIENT_MERGED` «Ese insumo se unió a otro; vuelve a
elegirlo».

**Por qué**: redirigir una receta o un renglón de compra convertiría cantidades sin que nadie lo
vea; un movimiento, en cambio, es un hecho físico que ya pasó y su lugar correcto es el destino.
Todo en la base para no tocar `purchases.go`/`purchaseqty.go` (los está cambiando la 055) y para que
la versión anterior de la API también quede cubierta.

**Clasificación verificable**: `app.MergeTables` (espejo de `app.UnitChangeTables`) dice qué hace la
unión con cada tabla que apunta a `ingredients`. `TestEveryTableThatPointsToAnArticleIsClassified`
pasa a revisar los dos mapas, y una prueba nueva revisa en el catálogo que toda tabla clasificada
«rechaza» tiene el trigger.

## R4. Recetas

**Decisión**: con factor 1 y sin choque, el renglón cambia solo de insumo y conserva cantidad y
unidad (lo mínimo). Si la receta ya lleva el destino (`unique (recipe_id, ingredient_id)`), se suman
en la unidad base del destino en el renglón del destino y se borra el del origen. Con factor ≠ 1, se
convierte a la unidad base del destino con `domain.ConvertRecipeQty`, como el reemplazo.

Ciclos: un preparado no puede ser origen ni destino (FR-008), así que el destino no tiene receta
propia y cambiar un origen por él en cualquier receta no puede cerrar un ciclo. No hace falta
`StockGraph.ValidatePrepIngredient` aquí; la prueba lo deja asentado.

## R5. Presentaciones y marca

**Decisión**: columna `ingredient_presentations.brand text null` (1–40 sin espacios de orilla). El
índice único `ingredient_presentations_name_key` se rehace con la marca normalizada
(`coalesce(brand, '')`) dentro de la llave. Al unir, una presentación activa del origen **se mueve**
(update de `ingredient_id`, conserva su id): así las compras pasadas que la nombran
(`expense_items.presentation_id`) siguen diciendo qué marca se compró, que es la base de la entrega 2.
Sin marca, toma el nombre del origen (recortado a 40). Choque nombre+marca con el mismo contenido: la
del origen se archiva y no se mueve. Con contenido distinto: bloquea (la vista previa lo señala). Un
empaque aproximado que deja de cuadrar con la unidad del destino se archiva en el origen
(`presentationFits` de la 042). Las inactivas se quedan en el origen.

**Tolerancia**: el índice nuevo es más permisivo que el viejo; la API anterior inserta sin marca y
choca exactamente igual que hoy.

**Dónde se ve**: `ListPresentations` devuelve `brand`; el front usa una sola función
`presentationLabel` («Caja · Marca A») en la hoja de presentaciones, la captura de compra
(`ExpenseDialog`, `AddLineSheet`, `ArrivedItemSheet`), el conteo y la merma. El conteo hoy usa
`key={p.name}`: con nombres repetidos se cambia a la llave por id.

## R6. Costo resultante

**Decisión**: regla de hoy («último precio»): el costo de la compra más reciente (`compra` con
`unit_cost`, sin regalos ni anulados) entre destino y orígenes, dividido entre el factor del origen.
Sin compras en ninguno: el del destino; si es cero, el primer origen con costo, convertido. Se guarda
con `cost_source = 'compra'` o el del que se tomó, y después `recomputeCosts` recostea productos y
extras. El antes y el después van en la vista previa y en el evento de bitácora.

## R7. Vista previa y huella

**Decisión**: el patrón de la 053 (`runExpenseEdit`): la vista previa corre la unión completa dentro
de la transacción y la revierte con `errPreviewRollback`; confirmar repite la huella (sha256 del JSON
de la vista previa, sin ids recién insertados) y responde 409 `MERGE_STALE` si cambió. Así lo que se
ve es exactamente lo que se aplica.

**Front**: se extraen de `EditPreviewSheet` las piezas genéricas (`PreviewGroup`, renglón
antes/después/cambio, `signed`, `tone`, aviso de «algo cambió») a `web/src/shared/preview/`, y
`EditPreviewSheet` las usa sin cambiar lo que pinta. La hoja de unión las usa con sus propios grupos.

## R8. Historial del destino

**Decisión**: un CTE recursivo dentro de la consulta de sqlc (`with recursive sources …` sobre
`kind = 'merge'`, tope de profundidad 16) da el destino y sus orígenes con el factor acumulado.
`ArticleCostHistory` lee las compras y correcciones de esos ids, divide el costo entre el factor y
devuelve el nombre del origen (`from_name`, nulo para el propio). `ArticleAudit` incluye los eventos
`ingredient.merged_into` de sus orígenes. No es una función `security definer` llamable: correría
como owner y dejaría leer las uniones de otra empresa (revisión de arquitectura, M1); dentro de la
consulta, RLS aplica.

## R9. Aviso de posible duplicado

**Decisión**: lógica pura en `domain.SimilarNames(name, candidates)`, con la normalización de
`NormalizeReasonName` (la misma de `IngredientNameTaken`) más signos fuera. Parecido = (a) uno contiene
al otro como palabras completas y el más corto tiene al menos 4 letras, o (b) distancia de
Damerau-Levenshtein ≤ 1 con nombres de hasta 8 letras y ≤ 2 con más largos. Hasta 5, los más
parecidos primero. Endpoint `GET /stock/articles/similar?name=` con los activos de la empresa
(cientos de nombres: compararlos en Go es trivial y queda probado table-driven).

**Alternativas**: `pg_trgm` `similarity()` (ya instalado por la 0030): con nombres cortos da falsos
negativos («Lehce entera» contra «Leche entera» queda debajo de 0.5) y el umbral no se puede probar
sin base. `fuzzystrmatch` (levenshtein) no está instalado; agregar una extensión en producción es una
puerta más que no compra nada aquí.

## R10. Cambio de unidad de un destino con unidos

**Decisión**: `planUnitChange` se niega con 409 si el artículo tiene orígenes unidos. Su historia
convertida dejaría el factor de la unión hablando de la unidad vieja. Se dice en la pantalla del
cambio de unidad.

## R11. Convivencia con la 055

La 055 cambia `purchases.go` y `purchaseqty.go`. Este plan no toca esos dos archivos: el rechazo y la
redirección viven en la base (R2, R3) y el selector de presentación lee `ListPresentations`
(`ingredients.sql`). Antes de `/speckit-implement` se trae `develop` (commit firmado); si la 055 no
ha llegado, se implementa sin tocar esos archivos y se vuelve a traer `develop` antes del PR.

## R12. Grupos de posibles duplicados (maqueta C)

**Decisión**: `domain.DuplicateGroups(items, distinctPairs)` compara todos los activos no preparados
con la regla de `SimilarNames` y junta por transitividad (unión-búsqueda), sin los pares guardados en
`ingredient_distinct_pairs`. Con ~300 insumos son ~45,000 comparaciones de cadenas cortas: < 50 ms en
Go. Se calcula al pedir la pantalla; el contador del botón sale de la misma llamada.

**Por qué una tabla para «No son lo mismo»**: sin guardarlo, «Vaso 12 oz · Vaso 16 oz» vuelve en cada
visita y la lista deja de servir. Por par y no por grupo: un grupo cambia en cuanto entra o sale un
insumo.

**Alternativas**: calcular en SQL con `pg_trgm` (mismo problema de umbral de R9); guardarlo en
`localStorage` (no lo vería otra tableta ni otra persona).
