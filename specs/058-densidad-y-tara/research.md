# Research: Densidad y tara

## R1. Dónde vive la regla de conversión con densidad

- **Decision**: una sola función de `domain`, `BaseQty`, gana el caso masa↔volumen: `PurchaseUnit`
  lleva `Density *decimal.Decimal` (g/ml). Masa → volumen: `qty × ToBase ÷ d`; volumen → masa:
  `qty × ToBase × d`; Round4. Sin densidad, `ErrUnitKindMismatch` como hoy. Longitud con cualquier
  otra clase sigue rechazada. La densidad se valida con `domain.ValidDensity` (0.30–3.00, 4 decimales).
- **Rationale**: `PlanStockLines` ya pasa por `BaseQty` en capturar, recibir, corregir, editar y
  anular (spec 038/053); meter la densidad ahí cubre los cinco caminos sin tocar ninguno (el borde
  «el camino nuevo que se salta el control viejo»).
- **Alternatives**: convertir en `app` antes de planear (duplica la regla en cada camino); guardar
  el renglón ya en ml (pierde lo que dice el ticket).

## R2. Lo ya convertido no se recalcula: densidad fija por renglón

- **Decision**: `expense_items.density_g_per_ml numeric(8,4) null`. `ListStockLines` devuelve
  `coalesce(ei.density_g_per_ml, i.density_g_per_ml)`; `applyStockLinesOn`, al aplicar el primer
  movimiento de un renglón masa↔volumen cuyo `density_g_per_ml` es nulo, lo fija con la densidad
  usada (`SetExpenseItemDensity`, `where density_g_per_ml is null`). Desde ahí toda corrección de
  ese renglón usa la fija.
- **Rationale**: `PlanStockLines` recalcula lo que el renglón *debería* haber sumado en cada
  escritura y aplica la diferencia. Si leyera la densidad vigente, rehacer la pesada y luego tocar
  el gasto (corregir lo recibido, editarlo) insertaría una diferencia por el cambio de densidad:
  exactamente el «recalcular lo ya convertido» que el dueño prohibió. La copia es el snapshot del
  principio VIII.
- **Por qué al primer movimiento y no al capturar**: el dueño pidió que un renglón todavía no
  recibido use la vigente (US2 escenario 5). Un renglón capturado y recibido en la misma operación
  la fija en esa misma transacción.
- **Bajo candado**: ver la revisión de arquitectura del plan (M1): `lockIngredients` antes de
  `ListStockLines`, y la densidad fija se escribe con el valor exacto usado.
- **Alternatives**: guardar la densidad al capturar (contradice US2-5); recalcular todo y ajustar
  con un movimiento compensatorio (más movimientos, misma prohibición).

## R3. El cambio de unidad masa↔volumen: el factor lo pone el servidor

- **Decision**: `ValidUnitChange` acepta masa↔volumen solo con densidad. `planUnitChange` deriva el
  factor (g→ml `1/d`, ml→g `d`, a 6 decimales). El cuerpo puede omitir `factor`; si lo manda y no
  coincide con el derivado, 409 `DENSITY_CHANGED` (la densidad cambió entre la vista previa y la
  confirmación). La vista previa devuelve `density` y `factor`. La bitácora del cambio guarda la
  densidad en `after.density`.
- **Rationale**: aceptar un factor libre en masa↔volumen reabre «1 g = 1 ml» por la puerta de atrás;
  y confirmar con un factor viejo después de rehacer la pesada convertiría con una densidad que ya
  no es la del artículo.
- **Densidad tras el cambio**: el mismo artículo (camino directo) la conserva si el destino es g o
  ml y la pierde (se anota en bitácora) si es pieza o cm. En el reemplazo, el nuevo la recibe si es
  g o ml. Las taras se conservan (`SetPresentationRaw` y `CreatePresentation` pasan `tare_g`).
- **Costo**: `ConvertCost(cost, factor)` existente: $0.10/g con d = 1.30 → factor 0.769231 →
  $0.13/ml.

## R4. Tara y conteo por peso bruto

- **Decision**: `ingredient_presentations.tare_g numeric(10,2) null check (tare_g > 0)`. En el conteo,
  `CountCapture.Gross *GrossCapture{Weight, WeightUnitID, PresentationID, Containers}`. Regla pura
  `domain.NetFromGross(grossG, tareG, containers, baseKind, density)`: neto = bruto − tara ×
  empaques; negativo → `ErrGrossBelowTare`; base g → neto; base ml → neto ÷ d (sin densidad →
  `ErrUnitKindMismatch`). El resultado se suma a `Qty` (empaques cerrados + suelto) y pasa por el
  máximo y el aviso de siempre (`captureTx`).
- **Rationale**: la cuenta la hace el servidor con los valores vigentes (FR-017); la pantalla la
  muestra con los mismos valores que trae el `CountItem` (densidad y tara por empaque), así que lo
  que se ve y lo que se guarda salen de los mismos números.
- **Excluyente con `Weight`** (el conteo por peso de artículos en piezas, spec 042): mandar los dos
  es 400. La tara solo se ofrece en artículos g/ml; el conteo por peso de piezas no cambia.
- **Envases**: entero de 1 a 999. Bruto en g o kg (unidades de masa), validado con `ValidCountQty`.
- **Lo pesado se guarda** en `stock_count_lines` (revisión de arquitectura, puerta VIII): ver plan.

## R5. Captura de la densidad y su historia

- **Decision**: en `ingredients`: `density_g_per_ml numeric(8,4)`, `density_method text check in
  ('weighing','label','manual')`, `density_set_by bigint`, `density_set_at timestamptz`, con un check «los
  cuatro o ninguno». `PUT /stock/articles/{id}/density` con `{method:"weighing", fullG, emptyG,
  contentMl}`, `{method:"label", netG, contentMl}` (sin báscula: peso neto y contenido de la
  etiqueta; pregunta del dueño 2026-11-02) o `{method:"manual", density}`; `DELETE` la quita. `domain.DensityFromWeighing`
  valida (vacío < lleno, contenido > 0) y redondea a 4 decimales; `ValidDensity` aplica el rango a los
  dos métodos. Evento `ingredient.density_set` / `ingredient.density_cleared` con antes, después,
  método y pesada. Último en guardar gana (spec, borde «dos personas»).
- **Rationale**: la historia completa vive en la bitácora (patrón de la 042); la fila guarda solo lo
  vigente, que es lo que convierte.
- **Solo g/ml y activo**: el servicio rechaza con 400 un artículo en pieza o cm, y con 409 uno
  archivado (un insumo unido, spec 056, está archivado).

## R6. El front convierte con la misma regla

- **Decision**: `factorOf` de `ExpenseDialog` (y el equivalente de `AddLineSheet`/`ArrivedItemSheet`)
  usa una función nueva `densityFactor(buyKind, baseKind, toBase, density)` en `expenseDraft.ts`
  (vitest con los mismos casos que `BaseQty`). `SearchArticles` devuelve `densityGPerMl`. Un renglón
  masa↔volumen sin densidad pinta en el renglón «Falta la densidad · Capturar la pesada»; el botón
  abre `DensitySheet` del artículo y al guardar invalida `articles` para que el renglón se reevalúe.
- **Rationale**: borde «el hermano que no se movió»: hoy el front toma un renglón de otra clase como
  si necesitara el contenido de una pieza.

## R7. Migración tolerante a la API anterior

Solo agrega columnas nulas y checks sobre ellas. La API anterior no las lee ni las escribe: sus
inserts siguen válidos. Durante el traslape, un renglón masa↔volumen capturado en la API nueva y
tocado por la vieja falla con su 400 de hoy (no daña nada). Down: suelta las columnas; se niega si
hay renglones con `density_g_per_ml` (perderlo cambiaría lo que el renglón sumaría al editarlo) —
con su limpieza de prueba en `guardedDownCleanup`.

## R8. Respaldo anónimo

Columnas nuevas: todas numéricas, fecha o id, salvo `density_method` (enum de texto con check: sin
dato personal) → `noPersonalData` con su porqué.

## Fuera de alcance (confirmado)

- Presentaciones capturadas en la otra clase («Galón 4.3 kg» de un artículo en ml): hoy se
  capturan en la unidad del artículo; convertirlas con densidad queda para otro issue.
- Recetas y mermas en la otra clase; tabla de densidades genéricas.
