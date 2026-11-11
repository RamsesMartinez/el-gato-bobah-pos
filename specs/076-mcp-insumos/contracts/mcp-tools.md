# Contrato: herramientas MCP de la 076

Todas: `dry_run` (por omisión `true`) e `idempotency_key` (obligatoria al aplicar), como las de la
072–074. Cantidades y costo como texto decimal en la unidad base.

## `ingredient_create` — `write:inventory` · `inventory.configure`

| Argumento | Tipo | Obligatorio | Va a la ruta como |
| --- | --- | --- | --- |
| `name` | texto | sí | `name` |
| `base_unit_id` | entero | sí | `baseUnitId` (g, ml, cm o pieza; de `units_list`) |
| `category_id` | entero | no | `categoryId` |
| `place_id` | entero | no | `placeId` |
| `min_stock` | texto decimal | no | `minStock` |
| `initial_stock` | texto decimal | no | `initialStock` |
| `initial_cost` | texto decimal | no | `initialCost` (por unidad base) |

Vista previa: `maybe_exists` (primero): `[{id, name, base_unit, on_hand}]` y una nota «Quizá ya
existe: X (id N)…»; después `before`/`would_send`. Aplicar: `result.id`.

## `ingredient_update` — `write:inventory` · `inventory.configure`

| Argumento | Tipo | Notas |
| --- | --- | --- |
| `ingredient_id` | entero | obligatorio |
| `name` | texto | nombre nuevo |
| `category_id` / `clear_category` | entero / bool | uno u otro |
| `place_id` / `clear_place` | entero / bool | uno u otro |
| `min_stock` / `clear_min_stock` | texto decimal / bool | uno u otro |
| `alt_names` | lista de texto | la lista completa; `[]` los quita; ausente los conserva |

Orden al aplicar: ficha (PATCH) y después otros nombres (PUT). Vista previa: `changes` (antes y
después por campo), `maybe_exists` si cambia el nombre, `alt_names_similar` por otro nombre nuevo.

## `presentations_list` — `read` · `inventory.receive`

| Argumento | Tipo | Notas |
| --- | --- | --- |
| `ingredient_id` | entero | opcional; presente debe ser mayor que cero |

## Pistas de error nuevas

| Código | Pista |
| --- | --- |
| `NAME_TAKEN` | causa + «(id N[, archivado])» + «Usa ese insumo, o elige otro nombre. Reactivar un archivado es en la pantalla.» |
| `ALT_NAME_TAKEN` | causa + «(id N)» + «Quita ese nombre de la lista o pregunta si son el mismo insumo.» |
| `STALE` | «La ficha cambió mientras se guardaba: vuelve a intentar.» |
