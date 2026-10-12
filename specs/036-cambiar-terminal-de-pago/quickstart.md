# Quickstart

1. `make db-restaurar` con un respaldo reciente y `make start`.
2. Como cajero, cobrar un pedido con tarjeta: la terminal llega vacía (con 2+ activas) y Cobrar no procede sin elegir. Elegir Terminal A.
3. Cobrar otro pedido: la terminal vuelve a llegar vacía.
4. Como cajero, abrir Ventas → la venta: se ve «Terminal A» en el pago y no hay «Cambiar terminal».
5. Como gerente, «Cambiar terminal» → Terminal B, sin motivo: no deja. Con motivo: el pago dice Terminal B; Caja → arqueo esperado de Terminal A baja y Terminal B sube por el monto.
6. Bitácora: evento «payment.terminal_changed» con autor, antes, después y motivo.
7. Cerrar el turno con arqueo, cambiar otro pago y abrir el corte cerrado: aviso de cambio posterior con la diferencia recalculada.
8. Gates: `cd server && go build ./... && go test ./...`; `cd web && bun run lint && bun run vitest run`; `make ci-local`.
