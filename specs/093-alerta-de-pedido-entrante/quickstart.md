# Quickstart: probar la alerta en local

## Preparación

1. `make start` en el worktree, con `PLATFORM_ORDERS_SIMULATED=true` en `deploy/.env` (solo
   desarrollo; la API se niega a arrancar con él en producción).
2. Base restaurada (`make db-restaurar`) o la de pruebas; empresa con al menos una conexión de
   plataforma.
3. Sembrar pendientes: `make entrantes-de-prueba n=2` (inserta pedidos de plataforma pendientes en
   la base local, con `decide_before` a 11 minutos, y publica nada: la tableta se entera por el
   sondeo o al recargar; para el evento en vivo usar la e2e).
4. Abrir `http://localhost:3093` a 1024×600 (y 1280×854), con `hasTouch`.

## Escenarios

| # | Pasos | Esperado |
|---|---|---|
| 1 | Estar en Almacén, sembrar 1 | En ≤ 15 s (sondeo) o < 1 s (e2e con evento) suena y aparece el indicador «1» |
| 2 | Tocar el indicador | Abre Pedidos con «Por aceptar» arriba |
| 3 | Aceptar | Deja de sonar, sale la comanda, el pedido aparece en «En preparación» |
| 4 | Sembrar 1, Rechazar → «Se acabó un producto» | Deja de sonar; la fila queda `rechazado` con `deny_reason = out_of_stock` |
| 5 | Recargar con 1 pendiente sin tocar nada | Indicador visible + «Toca para activar el sonido»; al tocar, suena |
| 6 | DevTools offline, sembrar, volver online | Suena en < 5 s tras reconectar |
| 7 | Con 1 sonando, offline 30 s | Sigue sonando |
| 8 | Dos navegadores, aceptar en uno | El otro se calla en < 5 s |
| 9 | Ocultar la pestaña (simula suspensión), sembrar, volver | Suena en < 5 s |
| 10 | Quitar el permiso a un rol | Esa sesión no suena ni ve nada; la API responde 403 |

## Al terminar

Ningún pedido de prueba queda abierto: los aceptados se entregan y se cobran (`/deliver`, ya vienen
pagados por la plataforma) y los sembrados que sobren se rechazan.

## Pruebas automáticas

- `cd server && go test ./internal/domain/ -run Incoming` — regla de pendiente, orden y motivos.
- `make test-integration` — `IncomingOrders*` bajo `appRoleStore` en los tres casos de RLS, la
  carrera de dos aceptaciones, la cancelación por webhook y la migración 0117.
- `cd web && bun run vitest run src/features/entrantes` — alarma, hook y componentes.
- `bun run e2e -- entrantes` — escenarios 1–4, 6 y 8 contra la API local.
