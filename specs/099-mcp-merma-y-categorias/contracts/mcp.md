# Contratos del MCP — 099

Las tres son escrituras con el contrato común (`dry_run` true o ausente = vista previa;
`dry_run: false` + `idempotency_key` = aplica; misma clave y mismos argumentos = misma respuesta;
misma clave con otros argumentos = error). Refs (#335) declarados para cada id.

## ingredient_expected_waste_set — `ScopeWriteCatalog`, `menu.configure`

| Arg | Tipo | Regla |
|---|---|---|
| `ingredient_id` | int | > 0 (ref `ingredient`) |
| `pct` | texto | `^[0-9]{1,2}(\.[0-9]{1,2})?$`; o `clear` |
| `clear` | bool | `true` = quitar la captura (hereda producto o general); o `pct` |
| `reason` | texto | 1–200, obligatorio |

- Exactamente uno de `pct` o `clear: true`; si no → error de argumentos que lo dice.
- Vista previa: la respuesta de `GET …/expected-waste/preview` (antes, después, fallback, usos),
  más notas: «sin captura» ≠ 0 %, la fórmula, que aplicar recalcula costos y no mueve existencias.
  Si `unchanged` → nota «ya tiene ese valor; aplicar no cambia nada».
- Aplica: `PUT /stock/articles/{id}/expected-waste` con `{"pct": "8" | null, "reason": …}`.
- Estado antes/después: `GET /stock/articles/{id}` (la ficha).

## category_create — `ScopeWriteCatalog`, `menu.configure`

| Arg | Tipo | Regla |
|---|---|---|
| `name` | texto | obligatorio |
| `parent_id` | int opcional | raíz activa de la empresa (ref `category`) |

- Vista previa: primero `similar` (categorías activas e inactivas cuyo nombre contiene al buscado o al
  revés), el padre resuelto (nombre, activo, raíz) y notas si el padre no sirve.
- Aplica: `POST /admin/categories`.

## category_update — `ScopeWriteCatalog`, `menu.configure`

| Arg | Tipo | Regla |
|---|---|---|
| `category_id` | int | > 0 (ref `category`) |
| `name` | texto opcional | nombre nuevo |
| `active` | bool opcional | false la apaga, true la reactiva |
| `parent_id` | int opcional | mover bajo esa raíz (ref `category`) |
| `make_root` | bool opcional | true = quitarle el padre; excluye `parent_id` |

- Al menos uno; si no → error de argumentos.
- Vista previa: estado actual (nombre, padre, activa), el nuevo, productos activos e inactivos,
  subcategorías (activas e inactivas); avisos que van primero si la ruta lo va a rechazar (productos
  activos, subcategorías activas, padre inactivo o no raíz).
- Aplica: `PATCH /admin/categories/{id}` con `name`/`isActive`/`parentId`.

## categories_list (ampliada)

Arg nuevo `include_inactive` (bool): `GET /admin/categories?status=all`. Cada renglón trae
`isActive` e `inactiveProductCount`.

## Pistas por código (`toolFixByCode`)

| Código | Pista |
|---|---|
| `CATEGORY_NAME_TAKEN` | Usa esa categoría (si está inactiva, reactívala con category_update y active: true) o elige otro nombre. |
| `CATEGORY_HAS_ACTIVE_PRODUCTS` | Mueve esos productos a otra categoría o desactívalos con product_update, y repite. |
| `CATEGORY_HAS_ACTIVE_CHILDREN` | Desactiva o mueve primero esas subcategorías con category_update. |
| `CATEGORY_PARENT_INVALID` | El padre debe ser una categoría raíz activa y la categoría no debe tener subcategorías; búscalo con categories_list. |
