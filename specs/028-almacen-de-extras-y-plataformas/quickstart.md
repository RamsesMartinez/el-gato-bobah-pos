# Quickstart: validar el almacén

1. Gates: `go build`, `go test`, integración con `TEST_DATABASE_URL` y `TEST_RESTORED_DATABASE_URL`,
   `make lint`, `bun run lint`, vitest y build.
2. Carga de FUDO en local sobre un respaldo de producción restaurado:
   `go run ./cmd/fudo-import -compositions -company gatobobah`. Revisar el reporte de lo que no
   empató y la lista «Estimada» en el catálogo.
3. En el ambiente de pruebas con el respaldo de producción:
   - vender en mostrador un producto con un extra que lleve composición, y ver los dos movimientos
     en Almacén → Movimientos;
   - cancelar el pedido y ver que repone una sola vez;
   - capturar una composición desde la tableta a 1024×600 y confirmarla.
   Todo pedido de prueba se cobra antes de terminar.
