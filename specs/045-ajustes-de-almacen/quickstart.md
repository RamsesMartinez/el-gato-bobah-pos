# Quickstart: 045

Con `make start` del worktree y base restaurada (`make db-restaurar`), a 1024×600:

1. `cd web && bun run vitest run units StockCountTab ArticleSheets StockPage AppearancePage` → verde.
2. Almacén › Existencias y Artículos: un artículo en ml negativo muestra el mismo texto en ambas y en su ficha.
3. Ficha con existencia negativa › «Revisar con un conteo» → pestaña Conteo con el artículo buscado.
4. Conteo › buscar «apio»: no aparece «Tapioca».
5. Playwright con `colorScheme: 'dark'` y `localStorage` vacío: la app abre en oscuro; capturas de POS, hoja de cobro y corte sin texto ni control perdido.
6. Página larga desplazada: «Pedidos» completo en la barra lateral (medir su caja ≥ 0 px arriba); tocar «Asistencia» la deja visible.
7. Ficha: escribir el mínimo y tocar de inmediato «Revisar con un conteo»: el mínimo queda guardado.
