# Quickstart — 069

Prerrequisitos: `make start` del worktree (API :8069, web :3069), base restaurada con
`make db-restaurar`, Postgres de pruebas para `make test-integration`.

## Pruebas automatizadas

```bash
cd server && go test ./internal/domain/ -run 'Count|ModeOf|Options'
make test-integration   # incluye las de conteo de productos y «Lo restan sus opciones»
cd ../web && bun run vitest run src/features/backoffice src/shared
```

Las dos del criterio de aceptación (nombres en `tasks.md`):

1. Producto en −2, conteo de 15 → existencia 15 y un `ajuste` de +17 con `stock_count_id` del conteo.
2. Producto con grupo obligatorio de sabores → `mode: "options"` sin insumos → vender uno con
   «Fresa» deja un solo movimiento `venta`, del artículo de Fresa.

## Recorrido a mano (1024×600, `hasTouch`)

1. Almacén › Conteo › Empezar conteo: aparecen los productos con existencia propia, marcados como
   producto; no aparece ninguno que resta por receta.
2. Capturar 15 en un producto; recargar: la cifra sigue.
3. Capturar 2,5: se rechaza con mensaje claro.
4. Vista previa: el producto con sistema, contado y diferencia; los no contados en «sin contar».
5. Cerrar: en el historial del producto, un ajuste ligado al conteo.
6. Menú › Recetas › un producto con sabores que cuenta existencia propia › «Lo restan sus
   opciones» › Guardar: sin pedir insumos; la lista lo dice así.
7. Vender ese producto con un sabor y cobrar; ver que baja solo el artículo del sabor. Cerrar el
   pedido de prueba (entregar y cobrar).
