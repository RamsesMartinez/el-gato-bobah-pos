# Contrato: cambios de la 077 en herramientas MCP

Todas las escrituras: `dry_run` (por omisión `true`) e `idempotency_key` (obligatoria al aplicar).

## `ingredient_update` — `write:inventory` · `inventory.configure`

Argumentos nuevos:

| Argumento | Tipo | Notas |
| --- | --- | --- |
| `active` | bool | `false` desactiva (archiva), `true` reactiva. Va solo: con cualquier otro cambio, rechazo |
| `confirm_on_hand` | bool | Solo con `active: false`. Archiva aunque tenga existencia, sin ajustarla; solo si la persona lo decide |

Vista previa de `active: false`: `attention` primero, en este orden: existencia por sucursal (con la
indicación de merma o ajuste antes), cada receta activa («Lo usa la receta activa X (producto, id N)»),
conteo abierto. `resolved`: `archive_check` tal como lo devuelve la ruta. Notas: compras por
recibir; ya inactivo.

Vista previa de `active: true`: `resolved.on_hand` (de la ficha); nota «vuelve tal como estaba»; ya
activo.

Aplicar: `result: {"ok": true}`; `before`/`after` con `isActive`.

## `articles_search` — `read` · `expenses.capture`

| Argumento | Tipo | Notas |
| --- | --- | --- |
| `query` | texto | como hoy |
| `include_inactive` | bool | agrega insumos archivados que coinciden (pide ver el almacén) |

Cada renglón trae `active`. Sin el parámetro, los mismos renglones que hoy.

## `stock_levels` — `read` · `inventory.view`

Cada renglón trae `is_active`; los insumos archivados no salen.

## Pistas de error

| Código | Pista |
| --- | --- |
| `ARCHIVE_HAS_STOCK` | causa + «Registra la merma (waste_record) o el ajuste (stock_adjust) y repite; si la persona decide archivarlo con esa existencia, manda confirm_on_hand: true.» |
| `IN_USE` | causa + «Quítalo de esas recetas, o cierra o descarta el conteo, en la pantalla; desactivarlo no cambia ninguna receta.» |
| `NAME_TAKEN` de un archivado | «(id N, archivado: se reactiva con ingredient_update y active: true)» |
