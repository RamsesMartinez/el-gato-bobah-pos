# Implementation Plan: Subidas a producción sin corte

**Branch**: `041-subidas-sin-corte` | **Date**: 2026-10-18 | **Spec**: [spec.md](spec.md)

## Summary

Dos piezas, en este orden:

1. **Apagado ordenado** (en el binario): al recibir la señal, la API deja de anunciarse lista, cierra las conexiones de avisos en vivo, espera las peticiones en curso y las lecturas de menú que corren en segundo plano, y solo registra error si de verdad vence el plazo. Presupuesto por etapa, peor caso 25 s: peticiones 15 s, lecturas y ciclos de fondo hasta los 20 s, cierre de lo que no terminó 5 s ([contracts/apagado.md](contracts/apagado.md)).
2. **Azul/verde en la misma VM** (en `deploy/` y `scripts/`): dos servicios `api-blue` y `api-green` en compose, un archivo de upstream que Caddy importa, y un script de despliegue que arranca la nueva, espera que esté lista, cambia el upstream con `caddy reload` y apaga la vieja. Si la nueva no queda lista, la vieja sigue atendiendo y el despliegue falla. La primera subida de cada máquina recrea Caddy una vez (corte medido de ~1.5 s en local y ~4.7 s en la e2-micro del ambiente de pruebas; aceptado por el dueño, SC-006).

Lo que reveló leer el código (ver [research.md](research.md)):

- **El resumen diario YA tiene candado en la base**: `ClaimSummarySend` es un `insert … on conflict do update … where` sobre `daily_summary_sends (company_id, business_date)`; la segunda copia recibe `ErrNoRows` y no manda. El spec suponía que no lo tenía. No hace falta migración: hace falta la **prueba** de dos pasadas simultáneas, que hoy no existe.
- **`CerrarLecturasColgadas` es seguro en el traslape** (solo toca lecturas de más de 5 min), pero **el apagado mata las lecturas de menú en curso** (`go s.correrLectura(...)`, fuera de cualquier petición), y quedan «en curso» hasta la siguiente pasada de mantenimiento. El apagado ordenado tiene que esperarlas o marcarlas fallidas.
- **La tableta no vuelve a pedir el estado al reconectarse**: `useOrderEvents`, `useMenuEvents` y `usePedidosDePlataforma` solo marcan `live`. Un pedido creado en la copia vieja durante el traslape no llega como aviso; se ve hasta el refresco de 30 s. Se invalida al reconectar (`onopen` tras un `onerror`).

## Technical Context

**Language/Version**: Go 1.27.2 (servidor), TypeScript/React 19 (web), bash (despliegue)
**Primary Dependencies**: chi, pgx/sqlc, Caddy 2 (ya en `deploy/`), docker compose
**Storage**: PostgreSQL 16 — sin migración nueva
**Testing**: `go test` (unitario e integración con `-tags=integration`), vitest, un script de medición en bash contra el ambiente de pruebas
**Target Platform**: VM e2-micro Linux (1 vCPU, 1 GB), imagen distroless
**Project Type**: web service + infraestructura de despliegue
**Performance Goals**: 0 peticiones fallidas durante la subida; apagado < 1 s con solo avisos en vivo abiertos
**Constraints**: dos copias de la API (~10 MB c/u) caben en ~468 MB libres; sin herramienta nueva (nada de Kamal/Swarm/K8s); la imagen distroless no tiene shell ni curl
**Scale/Scope**: un proceso de despliegue, ~4 archivos de despliegue, 3 del servidor, 3 hooks del web

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I. Layering | El apagado vive en `cmd/api` y `realtime`; esperar las lecturas en `app` (un `WaitGroup` en el servicio). Ninguna lógica de negocio cambia de capa |
| II. Errores y contexto | Toda goroutine nueva termina con ctx o cierre de canal. El ERROR del apagado queda solo para el plazo vencido |
| III. Dinero | No aplica |
| IV. Test-first | Tests antes del código: broker cerrado termina el handler SSE; apagado con SSE abierto < 1 s sin error; dos pasadas simultáneas del resumen mandan 1 correo (integración); lectura en curso al apagar queda fallida, no en curso; reconexión invalida (vitest). La medición del corte es un script runnable |
| V. Seguridad | Sin endpoint nuevo. El archivo de upstream lo escribe solo el script de despliegue; el Caddyfile conserva sus headers. `-readycheck` solo pega a localhost |
| VI. YAGNI | Dos servicios fijos y un archivo; sin orquestador, sin balanceo con health checks activos de Caddy |
| VII. Comentarios | El porqué del orden del apagado y del cambio de upstream va en el código |
| VIII. Puertas | ¿Se puede agregar después al mismo costo? Sí: todo es despliegue y proceso, no hay hecho que se deje de registrar. Abre (no cierra) **«Más de una caja vendiendo a la vez»** a nivel de infraestructura; el broker en memoria sigue siendo por copia y está documentado |
| Restricciones del producto | Producción con datos reales: si la nueva no queda lista, gana la vieja. En producción no se prueba: la medición va en el ambiente de pruebas |

Enmienda (FR-013): la constitución gana en **IV** la regla «toda migración tolera a la versión anterior corriendo» → **MINOR**, 1.24.0, en commit propio.

Sin violaciones; Complexity Tracking vacío.

## Pantallas y maquetas

No hay pantalla nueva ni cambio de disposición: el cambio en `web/` es invalidar consultas al reconectar los avisos en vivo. No aplica `/maquetas`.

## Capas

| Pieza | Capa / archivo |
| --- | --- |
| Cerrar avisos en vivo al apagar | `realtime.Broker.Close` + `httpapi.Events` termina al cerrarse su canal |
| Orden del apagado, `-readycheck` | `server/cmd/api/main.go` |
| `/readyz` responde 503 al empezar el apagado | `httpapi` (bandera atómica que pone `main`) |
| `/events` responde 503 durante el apagado | `domain.ErrShuttingDown`, mapeado a `SHUTTING_DOWN` solo en `httpapi.Error` (`respond.go`) |
| Esperar lecturas de menú en segundo plano | `app.MenusDePlataformaService` (`Esperar(ctx)`; al vencer, marca fallidas por id con `WithTenant` de cada empresa; test bajo `appRoleStore`) |
| Dos copias, upstream importado | `deploy/docker-compose.yml`, `deploy/Caddyfile`, `deploy/upstream/` |
| Despliegue azul/verde | `scripts/deploy-azul-verde.sh`, llamado por `make deploy-image`; su prueba `scripts/tests/deploy-azul-verde-test.sh` en pre-commit |
| Medición del corte | `scripts/medir-corte.sh` |
| Reconexión del tablero | `web/src/hooks/useOrderEvents.ts`, `useMenuEvents.ts`, `features/pos/usePedidosDePlataforma.ts` |
| Regla de migraciones | `.specify/memory/constitution.md` |
| Mecánica | `AGENTS.md` §2/§3 y `docs/despliegue-sin-corte.md` (+ renglón en `docs/README.md`) |

## Bordes (se piensan antes del código)

- **Caddy manda tráfico a la vieja mientras se apaga**: el orden es *reload primero, señal después*. Las conexiones ya abiertas hacia la vieja terminan solas; las nuevas ya van a la nueva.
- **Petición que llega a la vieja justo antes del reload**: la vieja todavía escucha; `Shutdown` la deja terminar.
- **SSE abierto a través de Caddy tras el reload**: Caddy conserva el stream viejo hasta que la API lo cierra; al cerrarlo la vieja, el navegador reconecta y Caddy lo manda a la nueva. Medido en la vuelta local.
- **La nueva migra al arrancar con la vieja atendiendo**: cubierto por la regla nueva; el script no lo puede detectar, lo detecta la revisión del plan de cada feature.
- **Despliegue interrumpido**: el estado vive en `deploy/upstream/activo.caddy` (fuente de verdad del lado activo). El script lo lee al empezar; un lado vivo que no es el activo se considera basura y se recrea. Nunca se escribe el upstream antes de que la nueva esté lista.
- **Primera subida** (servicio `api` → `api-blue`): el Caddyfile nuevo llega por `git checkout` antes de `make`; sin `activo.caddy` el script lo escribe con `to api:8080` antes de cualquier recarga, y el Caddyfile lo importa por glob (`activo*.caddy`) para que Caddy arranque aunque falte (VM reiniciada antes de la primera corrida). El contenedor viejo es `${COMPOSE_PROJECT_NAME:-deploy}-api-1`, nunca un nombre fijo, para que la vuelta local pruebe el mismo camino. Arranca `api-blue`, espera, escribe `api-blue:8080` y **recrea Caddy** en vez de recargarlo: el contenedor anterior no tiene montado `deploy/upstream/` y su bind mount del Caddyfile apunta al archivo que `git checkout` reemplazó. Con la espera por defecto de Docker eso cortó ~10 s (Caddy apaga con «eternal grace period» y espera a los avisos en vivo); con `--timeout 1`, ~1.5 s en local y ~4.7 s en la e2-micro del ambiente de pruebas, donde Docker tarda ~4 s en quitar y crear el contenedor (aceptado por el dueño el 2026-10-18, SC-006). Después `docker stop -t 30 deploy-api-1` y lo quita. El mismo motivo hace que un cambio futuro del Caddyfile pida recrear, no recargar ([docs/despliegue-sin-corte.md](../../docs/despliegue-sin-corte.md)). Esta transición se ensaya en el ambiente de pruebas antes que nada.
- **Nunca `compose up -d` sin nombrar servicio**: levantaría las dos copias. El script nombra siempre el servicio y usa `--no-deps`.
- **Bind mount de un archivo + reescritura atómica** cambia el inodo y el contenedor ve el viejo: se monta el **directorio** `deploy/upstream/`.
- **Bitácora**: `journalctl CONTAINER_NAME=deploy-api-1` deja de servir; se fija `tag: api` en el driver de journald para buscar por `SYSLOG_IDENTIFIER=api` sin importar el color.
- **Memoria**: el script revisa `MemAvailable` antes de arrancar la segunda copia (umbral 100 MB) y falla sin tocar nada.
- **El healthcheck de compose sigue en `/healthz`**; la espera del script usa `-readycheck` (incluye la base), con un plazo de 60 s.
- **Mismo SHA en las dos**: funciona; se recrea la inactiva.

## Project Structure

### Documentation (this feature)

```text
specs/041-subidas-sin-corte/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── despliegue.md
│   └── apagado.md
└── checklists/requirements.md
```

### Source Code

```text
server/cmd/api/main.go
server/internal/realtime/broker.go (+ _test)
server/internal/httpapi/handlers_sse.go, health (readyz)
server/internal/app/menus_de_plataforma.go (+ _test)
server/internal/integration/daily_summary_concurrente_test.go
deploy/docker-compose.yml, deploy/Caddyfile, deploy/upstream/.gitkeep
scripts/deploy-azul-verde.sh, scripts/medir-corte.sh
Makefile (deploy-image)
web/src/hooks/useOrderEvents.ts, useMenuEvents.ts, features/pos/usePedidosDePlataforma.ts (+ tests)
.specify/memory/constitution.md, AGENTS.md, docs/despliegue-sin-corte.md, docs/README.md
```

**Structure Decision**: monorepo existente; nada nuevo fuera de dos scripts y un directorio de upstream.

## Complexity Tracking

Vacío.
