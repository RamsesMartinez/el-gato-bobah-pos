# Quickstart — validar las entregas 2 y 3

Contra el `make start` del worktree (API :8068, web :3068) con base restaurada (`make db-restaurar`).

1. `cd server && go test ./internal/domain/ ./internal/httpapi/` — reglas de roles, catálogo y
   middleware.
2. `make test-integration` — equivalencia ruta × rol sugerido (antes = después), herencia, ajustes,
   aislamiento (`inTheThreeCases`), convivencia del trigger, y la migración contra respaldo de dos
   empresas.
3. `cd web && bun run vitest run` — menú y controles por permiso; pantalla Roles; Empleados con `Picker`.
4. A 1024×600, como admin: crear «Encargado de turno» con cobrar + caja + ventas, asignarlo a un
   empleado nuevo, entrar con él y comprobar menú y rutas; quitarle «ventas» y ver que la siguiente
   petición se niega sin volver a entrar; intentar inactivar con el empleado activo (rechazo); darlo de
   baja, inactivar el rol, borrarlo (rechazo: tuvo usuario). Revisar la bitácora.
5. Entrega 3: aplicar la 0109 con la API de la entrega 2 corriendo y comprobar que sigue sirviendo.
