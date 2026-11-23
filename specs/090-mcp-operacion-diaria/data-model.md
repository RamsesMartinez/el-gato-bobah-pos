# Data Model: MCP para la operación diaria

## Cambio de esquema (0116)

| Tabla | Columna | Tipo | Regla | Por qué |
| --- | --- | --- | --- | --- |
| `ingredient_cost_corrections` | `reason` | `text null` | `null`, o 1 a 200 caracteres y igual a sí mismo recortado (el servicio recorta) | Motivo de la corrección; obligatorio por MCP, opcional por pantalla |

- RLS y GRANT: la tabla ya los tiene (0096); una columna nueva los hereda.
- Respaldo anónimo: el paso 5 pone `'motivo anonimizado'` donde no es null; `FUGAS` lo cuenta.
- Down: `drop column reason`.

## Lecturas nuevas (sin esquema)

- **Opción por id**: `id, groupId, groupName, groupActive, name, priceDelta, maxPerLine, favorite,
  active, compositionStatus`. Bajo RLS: otra empresa → 404.
- **Producto en la lista**: `inactiveGroupCount` = grupos ligados con `modifier_groups.is_active =
  false`.
- **Cotización de cuenta** (`DraftQuote`): la forma de `DraftView` sin id ni versión, más `markupPct`
  y, en cada renglón y extra, `priceSource` ∈ {`counter`, `platform`, `markup`} y `basePrice`.
- **Historial de costo**: cada corrección agrega `reason` (null si vino de la pantalla sin motivo).
