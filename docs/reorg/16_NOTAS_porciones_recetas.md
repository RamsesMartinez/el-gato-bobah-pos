# Nota de planeación — porciones por opción según platillo/variante

> Contexto de la migración #16. **No se implementa ahora**; queda documentado para la
> siguiente iteración de recetas/costeo.

## El problema

Hoy una opción de modificador (ej. "Salsa BBQ") consume **una** receta fija
(`modifier_options.recipe_id`) sin importar a qué producto se agregue. Pero la cantidad
real cambia por platillo: unas alitas de **370 g** llevan menos salsa que unas de **1 kg**.

En FUDO esto se resolvía **duplicando la opción** (p. ej. "Salsa Brava CH" vs "Salsa Brava G")
para que el sistema descontara distinta cantidad de gramos/ml en cada tamaño. Eso generó
los grupos gemelos que la #16 acaba de unificar (misma salsa, distinto grupo solo por la
porción). En #16 los colapsamos porque **hoy no modelamos porciones a nivel opción**
(verificado: 0 opciones con `recipe_id`/`linked_product_id` en toda la BD).

## Estado actual del esquema

- `modifier_options.recipe_id` → una receta por opción (cantidad fija). Sin poblar.
- `modifier_options.linked_product_id` → o la opción es un producto con stock directo. Sin poblar.
- `product_modifier_groups` → enlace producto↔grupo con override de `min/max/title/position`.
  **Aquí vive la relación por-producto**, así que es el lugar natural para colgar la porción.

## Dirección propuesta (siguiente iteración)

Modelar la porción **en el enlace por-producto**, no duplicando opciones. Una opción de
las dos formas:

1. **Factor de escala por producto** (simple): columna `portion_factor numeric default 1`
   en un nuevo `product_modifier_option_portions(product_id, modifier_option_id, factor)`,
   o un factor por tamaño/variante. La receta base de la opción × factor = consumo real.

2. **Cantidad explícita por (opción, variante)**: cuando el escalado lineal no basta, una
   tabla de override `(modifier_option_id, product_id) → cantidad/receta`.

Requisito transversal ya conocido: falta el concepto de **"variante"** de producto (CH/M/G/J
como variantes de un mismo platillo en vez de productos separados). Cuando exista, la porción
se ata a `(opción, variante)` y esto se vuelve natural. Por ahora estamos a nivel
**producto** suelto.

## Qué NO hacer

- No volver a duplicar opciones/grupos por tamaño (es justo lo que #16 limpió).
- No poblar `recipe_id` con recetas por-tamaño distintas para la "misma" opción.
