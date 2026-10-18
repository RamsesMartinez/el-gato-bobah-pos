# Quickstart: validar subidas sin corte

## 1. Local — apagado ordenado (API del worktree en :8041)

1. Abrir un stream: `curl -N "http://localhost:8041/api/v1/events?token=$TOKEN" &`.
2. Mandar SIGTERM a la API (`make stop` o `kill -TERM` al proceso).
3. Esperado: el log muestra `shutting down` y `shutdown complete` con < 1000 ms; ningún renglón `level=ERROR`; el `curl` termina.

## 2. Local — azul/verde con el compose de producción

1. `COMPOSE_PROJECT_NAME=egb41prod` con `deploy/docker-compose.yml` y `API_DOMAIN=localhost`; Caddy publicado en puertos altos (p. ej. 8441) para no chocar con lo que ya escucha en 80/443.
2. En otra terminal: `scripts/medir-corte.sh https://localhost/healthz 120`.
3. Correr `scripts/deploy-azul-verde.sh` dos veces (azul→verde→azul) y una con una imagen que no arranca (`API_IMAGE=…` con `APP_DB_PASSWORD` roto).
4. Esperado: `medir-corte` reporta 0 fallidas y latencia máx < 2 s; la vuelta rota termina ≠0 y el tráfico sigue en el color anterior.

## 3. Tests

- `cd server && go test ./... && go test -tags=integration ./internal/integration/...` (vía `make test-integration`).
- `cd web && bun run vitest run`.

## 4. Ambiente de pruebas — una vez, con la versión final

1. Primera subida (transición `api` → `api-blue`) con `medir-corte` corriendo contra `https://api-dev.elgatobobah.com/healthz`.
2. Una subida normal más y una con versión rota.
3. Esperado: SC-001, SC-002 y SC-004 en las subidas normal y rota, y SC-006 (un solo corte, 2 s o menos) en la primera; se anotan en este archivo.

## 5. Producción

Con OK explícito del dueño. Tras subir, solo salud: `/healthz`, `/readyz`, job verde y journal sin errores.

## Resultados

| Medición | Antes | Después | Contra qué |
| --- | --- | --- | --- |
| Apagado con 2 avisos en vivo abiertos (§1), 2026-10-18 | 10 033 ms, `level=ERROR "shutdown" context deadline exceeded` | 55 ms, `shutdown complete duration_ms=1`, sin ERROR | API local del worktree (:8041), binario anterior vs. rama 041 |
| Primera subida (`api` → `api-blue`, recrea Caddy), §2, con Caddy en su `stop` por defecto | — | 50 de 347 fallidas (~10 s): Caddy espera a los avisos en vivo y Docker lo mata a los 10 s | proyecto local `egb41prod` (compose de develop → compose de la rama) |
| Primera subida, con `--timeout 1` al recrear Caddy | — | 7 de 350 fallidas (~1.5 s), deploy en 7 s | igual; corte único por máquina |
| Subida normal azul → verde, con un aviso en vivo abierto | — | 0 de 349 fallidas, latencia máx 3 ms, deploy en 5 s; el aviso en vivo se cerró y reconecta | igual |
| Subida normal verde → azul | — | 0 de 351 fallidas, latencia máx 10 ms, deploy en 5 s | igual |
| Versión que no arranca (falta su configuración) | — | 0 de 350 fallidas; deploy termina en error «api-blue no quedó lista en 60 s; el tráfico sigue en legacy» | igual |

Las cifras de la tabla son de la vuelta local. SC-001, SC-004 y SC-006 se cierran con la medición del ambiente de pruebas (T031).
