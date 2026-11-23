# Quickstart: validar la merma esperada (096)

Prerrequisitos: `make start` del worktree arriba, base restaurada (`make db-restaurar`), un usuario
con `menu.configure`.

## Subida 1

1. **Nada cambia al subir**: antes de migrar, exportar `id, current_cost` de productos, extras y
   preparados; migrar; recalcular; comparar → 0 diferencias (SC-001). Lo cubre la prueba de la 0121.
2. **General**: Ajustes › Costos › «Merma al preparar» 5 % → un producto con receta sin otros % sube
   5 % al centavo (SC-002). Vaciarlo lo regresa.
3. **Producto**: su ficha › 10 % → usa 10 %, no 15 %. 0 % → no suma.
4. **Artículo**: ficha › 15 % en la fresa → dentro del producto la fresa suma 15 %. 0 % en la lata →
   no suma. Borrar → «Sin capturar».
5. **Desglose**: ficha del producto › «Ver de dónde sale» → renglones con %, origen y pesos; la suma
   es el total (SC-003).
6. **Merma real**: registrar merma de un artículo con ventas → aparece «Real: N %» en la ficha del
   artículo y en el desglose; el costo no cambia.
7. **Pasado**: vender, cambiar el general, abrir la venta → mismo costo (SC-004).
8. **Permisos**: un cajero recibe 403 en los tres `PUT`.

Pruebas: `cd server && go test ./internal/domain/ -run 'Cost|Waste'`, `make test-integration`,
`cd web && bun run vitest run`. Pantalla con Playwright a 1024×600 y 1280×854.

## Subida 2

Ver tasks.md › Subida 2; requiere el #65 completo en producción.
