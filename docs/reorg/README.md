# reorg/ — reorganización del menú migrado de FUDO

**Ya aplicado en producción.** Se conserva como historia de qué se le hizo al catálogo y como
plantilla del patrón que se sigue usando para cambios de datos del menú.

## El patrón

Cada cambio es un par: `NN_descripcion.sql` y su `NN_rollback.sql` gemelo. El rollback se escribe
**antes** de aplicar el cambio, no después — sin él la migración no se corre. `00_dry_run.sql` es
la verificación previa: se corre primero para ver qué filas tocaría.

Esto **no** es el mecanismo de migraciones del esquema: eso es goose, en `server/migrations/`,
embebido en el binario (`make migrate-new name=xxx`). Aquí solo se mueven **datos** del catálogo —
precios, categorías, nombres, modificadores.

## Qué se hizo

| # | Cambio |
|---|---|
| 01 | Migración de precios |
| 02 | Café y chilaquiles |
| 03 | Categorías |
| 05 | Salsas como modificador |
| 06 | Bebidas |
| 07 | Nombres |
| 08 | Títulos |
| 09 | "Otro" |
| 10 | Subcategorías |
| 11 | Precios de bebidas |
| 12 | Tamaño en boneless |
| 13 | Chamoyada FDA |
| 14 | Crepas |
| 15 | Nombres de crepas |
| 16 | Dedupe de modificadores |
| 17 | Subcategorías de ramen |
| 18 | Desactivar "otros/envío" |

No hay `04`: se descartó antes de aplicarse.

Los `.csv` y `menu2026_extraido.txt` son el material de análisis con el que se armó la propuesta
(duplicados detectados, familias de productos, el menú extraído del PDF del local).

## Pendiente

[`16_NOTAS_porciones_recetas.md`](16_NOTAS_porciones_recetas.md) documenta una decisión
**aplazada**: hoy una opción de modificador consume una receta fija, sin importar el platillo o la
variante. No se implementó; sigue abierto.
