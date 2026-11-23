# Quickstart — 089

1. `make start` en el worktree; base restaurada con `make db-restaurar`.
2. Crear pedidos en los estados vivos (por API: `e2e/pos.ts` → `pedidoPorApi`, `pagarPorApi`,
   `entregarPorApi`): en cocina, pagado sin entregar, pago parcial, entregado que debe, y una cuenta
   en captura.
3. Vender a 1024×600: cada cuenta dice lo que le falta (research R-1).
4. Entregar la pagada desde Vender → sale de la fila (y de la de otra pestaña).
5. Cobrar la que debe desde la cuenta → hoja de cobro → saldada → sale de la fila.
6. `bun run vitest run src/domain src/features/pos` y la e2e `e2e/vender-cuentas.spec.ts` (medidas
   con y sin franjas a 1024×600 y 1280×854).
7. Cerrar (entregar y cobrar) todo pedido de prueba que siga abierto.
