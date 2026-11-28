# Data model — 099

Sin migración. Entidades existentes y las reglas nuevas que se les aplican.

## ingredients (existente)

| Campo | Regla |
|---|---|
| `expected_waste_pct numeric(5,2) null` | `null` = sin captura (hereda producto → general); `0 ≤ pct < 100`, dos decimales (`domain.ValidExpectedWastePct`). |

Transición: cualquier valor ↔ cualquier valor, incluido `null`. Mismo valor = sin cambio, sin bitácora.

## categories (existente)

| Campo | Regla nueva |
|---|---|
| `name citext` | trim, 1–60 caracteres; único por `(company_id, coalesce(parent_id,0))`. |
| `parent_id` | nulo (raíz) o una raíz de la misma empresa, distinta de sí misma. Quien tiene hijos no recibe padre. |
| `is_active` | `false` solo sin productos activos (`is_active and system_kind is null`) ni hijos activos. `true` solo si el padre (si hay) está activo. |

Estados: `activa` ⇄ `inactiva`. Nace activa.

## audit_events (existente) — acciones

| Acción | Entidad | before | after |
|---|---|---|---|
| `ingredient.expected_waste` (existente) | `ingredient` | `{"pct": x\|null}` | `{"pct": y\|null, "reason": "…"}` (reason solo si llegó) |
| `category.created` (nueva) | `category` | `null` | `{"name","parentId","isActive"}` |
| `category.updated` (nueva) | `category` | campos que cambiaron, valor anterior | mismos campos, valor nuevo |

Las dos acciones nuevas van en el grupo de catálogo de `domain/audit.go` (junto a `category.family_set`).

## Tipos de dominio nuevos

- `domain.CategoryNode{ID, ParentID *int64, Active bool}` y
  `domain.CategoryFacts{Target *CategoryNode, Parent *CategoryNode, HasChildren bool, ActiveChildren []string, ActiveProducts int}`;
  `domain.CheckCategoryChange(facts, next CategoryNext) error`.
- `domain.CostImpact{Kind ("product"|"option"|"prep"), ID, Before, After float64, Manual bool}`;
  `domain.ExpectedWasteImpact(before, after *CostGraph, ingredientID) []CostImpact`.
