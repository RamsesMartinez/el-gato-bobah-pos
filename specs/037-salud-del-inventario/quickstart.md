# Quickstart: validar Salud del inventario

## Preparación

- Worktree con su infraestructura propia (`make worktree-env n=37`, ver AGENTS.md).
- Base local con el respaldo de producción del 13-oct (`auditoria-inventario-20261013-1405.dump`) restaurado con `make db-restaurar`, y la API y el front levantados con `make start`.

## Escenarios

1. **Reproduce la auditoría (SC-001).** Almacén › Salud, periodo del 5 al 13-oct:
   - «Se vende sin restar» lista los productos de la auditoría del 13-oct con las mismas unidades por canal;
   - «Lo guardado en negativo» dice 126 (abre resumido mientras no haya compras).
2. **No marca lo que sí descuenta.** Los productos cuyo extra obligatorio sí resta (una bebida con sabor a elegir que lleva receta) no aparecen. Lo confirmado «No gasta insumos» (un «Sin …» o «No aplica») tampoco.
3. **Corrige un revendido (SC-003).**
   - Desde un renglón de «Se vende sin restar», abrir la receta de un producto revendido, elegir «Se compra hecho» y guardar 1 pieza.
   - Vender uno: el insumo nuevo baja 1, y el producto deja de aparecer para ventas posteriores; las ventas anteriores no se recalculan.
4. **Traslado de existencia.**
   - A un producto con existencia propia y 12 piezas, convertirlo en receta de 1 pieza de un insumo nuevo.
   - El insumo queda en +12, el producto en 0, hay dos ajustes ligados y un evento en la bitácora.
5. **Exportación (SC-005).** «Exportar» con el mismo periodo. El archivo abre en una hoja de cálculo con acentos bien y los mismos renglones que la pantalla.
6. **Permisos.** Con un cajero, `GET /stock/health` responde 403 y la pestaña no aparece.
7. **Parámetro malo.** `GET /stock/health?from=2026-13-01` responde 400, no el periodo por omisión.

## Gates

`make api-build && make api-test`, `make test-integration`, `bun run lint`, `bun run vitest run` y `bun run build`. Capturas a 1024×600, prueba con un usuario nuevo y revisión de tableta.
