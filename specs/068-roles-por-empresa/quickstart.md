# Quickstart — validar la entrega 1

Contra el `make start` del worktree (API :8068, web :3068), con base restaurada (`make db-restaurar`).

1. `cd server && go test ./internal/domain/ ./internal/httpapi/` — el mapa trae el permiso para
   admin, gerente y cajero, no para mesero; su mensaje no nombra roles.
2. `make test-integration` (o el test `TestEntregadas…` con `-run`) — por ruta y rol: cajero 200 en
   entregadas y 403 en devolver; mesero 403 en entregadas; gerente 200 en las tres.
3. `cd web && bun run vitest run src/features/orders` — con cajero aparece la sección y «Ticket»,
   no «Devolver»; con mesero no aparece la sección.
4. A 1024×600: entrar como cajero (crear uno en Empleados si la base no tiene), entregar un pedido
   capturado por otro usuario, abrir Pedidos: el pedido está en entregados, «Ticket» abre la
   reimpresión, no hay «Devolver». Cerrar/cobrar lo que se haya creado.
