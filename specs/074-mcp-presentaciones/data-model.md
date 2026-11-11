# Data Model: 074

Ningún cambio de esquema. Las entidades ya existen:

| Entidad | Dónde | Lo que usa esta feature |
| --- | --- | --- |
| Empaque de compra (`ingredient_presentations`) | spec 038, 056, 058 | crear y editar: nombre, contenido en unidad base, unidad de compra si es aproximado, marca (≤ 40, única por insumo junto con el nombre), activo |
| Renglón de gasto (`expense_items`) | spec 038, 053 | quitar (anula el renglón y revierte lo que sumó) y agregar (con empaque y marca de ese momento) |
| Gasto (`expenses`) | spec 053 | encabezado, importe y estado que recalcula la ruta; pagos intactos |
| Token (`api_tokens.scopes`) | spec 072, 073 | sin cambio: las herramientas nuevas caben en `write:inventory` y `write:expenses` |
