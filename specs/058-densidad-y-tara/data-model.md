# Data Model: Densidad y tara (migración 0105)

## `ingredients` (columnas nuevas)

| Columna | Tipo | Regla |
|---|---|---|
| `density_g_per_ml` | `numeric(8,4) null` | `check (density_g_per_ml between 0.3 and 3)` |
| `density_method` | `text null` | `check (density_method in ('weighing','label','manual'))` |
| `density_set_by` | `bigint null` | usuario que la puso; FK compuesta `(company_id, density_set_by) references users (company_id, id)`, como `ingredient_replacements.user_id` (0104) |
| `density_set_at` | `timestamptz null` | hora de la base |

Check `ingredients_density_all_or_none`: las cuatro nulas o las cuatro con valor.
Solo artículos con base g o ml la tienen: lo impone el servicio (`SetDensity`) y el cambio de unidad
la limpia al pasar a pieza/cm (R3).

## `ingredient_presentations` (columna nueva)

| Columna | Tipo | Regla |
|---|---|---|
| `tare_g` | `numeric(10,2) null` | `check (tare_g > 0 and tare_g <= 100000)`; peso del empaque vacío en g. Un trigger la limpia y la rechaza en artículos pieza/cm |

Se ofrece solo en artículos g/ml. Pasa con la presentación en unión (056, se mueve la fila) y en el
cambio de unidad (directo: misma fila; reemplazo: `CreatePresentation` la copia).

## `expense_items` (columna nueva)

| Columna | Tipo | Regla |
|---|---|---|
| `density_g_per_ml` | `numeric(8,4) null` | `check (between 0.3 and 3)`; la densidad con que el renglón se convirtió la primera vez (R2). Nula = todavía no convierte o no lo necesita |

Se fija una sola vez (`update … where density_g_per_ml is null`). No se reescribe nunca.

## `stock_count_lines` (columnas nuevas, puerta VIII)

| Columna | Tipo | Regla |
|---|---|---|
| `gross_g` | `numeric(14,4) null` | lo que marcó la báscula, en g |
| `gross_containers` | `smallint null` | `check between 1 and 999` |
| `tare_g_used` | `numeric(10,2) null` | la tara con que se restó |
| `density_used` | `numeric(8,4) null` | la densidad con que se convirtió (nula si la base es g) |

Las cuatro nulas si la cifra no se capturó pesando con empaque; recapturar sin pesar las vuelve a
nulo. Check: `gross_g`, `gross_containers` y `tare_g_used` van juntas.

## Trigger nuevo en `ingredients`

`before update of base_unit_id`: si la clase nueva no es masa ni volumen, pone en nulo las cuatro
columnas de densidad y `tare_g` de sus presentaciones (revisión A2).

## Tipos de `domain`

- `PurchaseUnit.Density *decimal.Decimal` — g/ml para el salto masa↔volumen.
- `ValidDensity(d) error`, `DensityFromWeighing(fullG, emptyG, contentMl) (decimal.Decimal, error)`.
- `DensityFactor(fromKind, toKind, d) (decimal.Decimal, error)` — factor del cambio de unidad.
- `NetFromGross(in GrossInput) (decimal.Decimal, error)` con `GrossInput{GrossG, TareG, Containers,
  BaseKind, Density}`.
- Centinelas: `ErrDensityChanged` (409 `DENSITY_CHANGED`), `ErrGrossBelowTare` (400), reuso de
  `ErrUnitKindMismatch` con código `UNIT_KIND_MISMATCH` que nombra el artículo (para el atajo).
- Auditoría: `ingredient.density_set`, `ingredient.density_cleared`, `presentation.tare_set`
  (antes/después; las dos primeras con método y pesada).

## Vistas que cambian

- `ArticleSheet`: `density {value, method, setByName, setAt} | null`.
- `SearchArticles` (captura de gasto): `densityGPerMl`.
- `ListPresentations` / `PresentationView`: `tareG`.
- `CountItem`: `densityGPerMl`, y `CountPresentation.tareG`; `grossWeighable` (al menos un empaque
  activo con tara, y densidad si la base es ml).
- `ListStockLines` / renglón de gasto en detalle, por recibir y vista previa de edición:
  `densityGPerMl` (la fija o la vigente) para pintar la conversión.
- `UnitChangePreview`: `density`, `factor`.

## Transiciones

- Densidad: ninguna → puesta (pesada|escrita) → rehecha → quitada. Cada paso, un evento.
- Renglón de compra masa↔volumen: sin densidad fija → (primer movimiento) fija. Irreversible.
