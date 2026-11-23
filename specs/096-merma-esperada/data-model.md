# Data Model: Merma esperada (096)

## Subida 1 — migración 0121 (`0121_expected_waste.sql`)

| Tabla | Columna | Tipo | Regla |
| --- | --- | --- | --- |
| `business_settings` | `expected_waste_pct` | `numeric(5,2)` nullable | `check (expected_waste_pct >= 0 and expected_waste_pct < 100)`; nulo = sin merma general |
| `products` | `expected_waste_pct` | `numeric(5,2)` nullable | mismo check; nulo = usa el general |
| `ingredients` | `expected_waste_pct` | `numeric(5,2)` nullable | mismo check; nulo = sin captura; 0 = capturado en 0 |

- Up: agrega las tres columnas; `update ingredients set expected_waste_pct = waste_pct where waste_pct > 0`.
  `set local lock_timeout = '3s'`. Sin índices (se leen enteras en el recálculo).
- Down: quita las tres columnas. No hay hecho que se pierda que el Up no vuelva a aceptar salvo los
  0 % capturados a propósito después de subir → renglón en `guardedDownCleanup` no aplica (el Down no
  se niega); se documenta en la prueba de migración que el Down pierde la distinción 0 / sin captura.
- `waste_pct` **se queda** (la API anterior la lee). Nadie la escribe salvo `fudo-import`, que pasa a
  la nueva.
- Ninguna columna nueva es de texto: no entra a la clasificación del respaldo anónimo.
- Las tablas ya tienen RLS; no hay tabla nueva.

## Dominio (sin I/O)

- `CostIngredient.ExpectedWastePct *float64` (reemplaza `WastePct`).
- `CostProduct.ExpectedWastePct *float64`.
- `NewCostGraph(..., general *float64)`.
- `ExpectedWastePct(article, fallback *float64) (pct float64, source WasteSource)` —
  `WasteSource` ∈ `article | product | general | none`.
- `ValidExpectedWastePct(*decimal) error` — nulo válido; 0 ≤ x < 100, dos decimales; NaN/Inf → 400.
- `CostBreakdown{Source, Base, Waste, Total, Lines []BreakdownLine, Components []BreakdownComponent}`.
- `RealWaste{WarehouseID, Wasted, Consumed, Ratio *float64, State}` con `State` ∈ `ratio | no_sales |
  no_data`; `RealWasteRatio(wasted, consumed)` puro.

## Subida 2 — migración NNNN (número al traer develop entonces)

| Cambio | Detalle |
| --- | --- |
| `business_settings.expected_waste_deducts` | `boolean not null default false` |
| `stock_movement_type` | `add value 'merma_esperada'` (en migración propia: `alter type … add value` no corre dentro de la transacción de goose con uso en la misma) |
| `ingredients.waste_pct` | `drop column` (S1 dejó de leerla) |
| Índices | los que la medición de SC-007 justifique, empezando por `company_id` |

Reporte conteo vs ventas y merma real: calculados, sin tabla.
