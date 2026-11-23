# Quickstart: Caducidad por lote (098)

Contra el `make start` del worktree (API :8098, web :3098) con `make db-restaurar`.

1. `cd server && go test ./internal/domain/ -run 'Lot|Expir'` — orden de consumo, proyección,
   validación del desglose.
2. `make test-integration` — `stock_lots_test.go`: cada borde del spec (lote sin fecha, misma fecha,
   venta que agota lo bueno, traspaso parcial, conteo que corrige, cancelación que repone), RLS en los
   tres casos de las tres tablas, y la migración 0128 sobre respaldo restaurado (ninguna existencia
   cambia, ningún lote nace).
3. En la web a 1024×600: recibir una compra con fecha → Almacén › Caducidades la muestra → vender
   hasta agotar lo bueno con un lote caducado → la campana avisa → mandar el caducado a merma → el
   aviso se cierra.
4. Contar un artículo con desglose de fechas y cerrar: Caducidades muestra los lotes contados.
5. Traspasar de Bodega a Local: los lotes llegan con su fecha.
