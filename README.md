# El Gato Bobah — POS

Punto de venta propio para el café El Gato Bobah (boba, crepas, boneless/alitas, ramen coreano).
Reemplaza a FUDO con un sistema 100% nuestro.

## Estructura (monorepo)

```text
web/          Frontend — React 18 + TypeScript + Chakra UI v2 + Vite (bun)
server/       Backend — Go + PostgreSQL + Redis (CQRS-light)   [F1 en adelante]
deploy/       docker-compose + Caddy                            [F1 en adelante]
docs/design/  Planes de diseño detallados (dominio, backend, UX)
references/   Exports reales de FUDO (fuente del importador)
```

## Requisitos

- **Node 24** (via nvm: `nvm use` lee `.nvmrc`)
- **bun** (obligatorio — `npm`/`yarn` están bloqueados por el guard `only-allow bun`)
- Go 1.25+ y Docker (para el backend, F1+)

## Desarrollo

```bash
make dev        # frontend en http://localhost:3000
make web-build  # build de producción (tsc + vite)
make web-test   # tests (vitest)
make help       # lista de targets
```

El plan maestro y las fases están en [docs/design/00-master-plan.md](docs/design/00-master-plan.md).
