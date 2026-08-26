# El Gato Bobah — POS

Punto de venta propio para el café El Gato Bobah (boba, crepas, boneless/alitas, ramen coreano).
Reemplaza a FUDO con un sistema 100% nuestro. **En producción** desde julio de 2026.

## Estructura (monorepo)

```text
server/       Backend — Go 1.27 + chi + pgx/sqlc + PostgreSQL + Redis
web/          Frontend — React 19 + TypeScript + Chakra UI v3 + Vite (bun)
deploy/       docker-compose + Caddy (TLS)
specs/        Una carpeta por feature nueva (spec-kit)
docs/         Documentación — ver docs/README.md, dice qué está vigente
references/   Exports reales de FUDO (fuente del importador de catálogo)
```

## Requisitos

- **Node 24** (via nvm: `nvm use` lee `.nvmrc`)
- **bun** (obligatorio — `npm`/`yarn` están bloqueados por el guard `only-allow bun`)
- **Go 1.27+** y Docker

## Desarrollo

```bash
make install    # setup completo (valida entorno, instala deps y herramientas)
make start      # levanta Postgres, Redis, mailpit, API y web
make api-test   # go test ./...
make web-test   # vitest
make sec        # lint + gosec + govulncheck + audit del front
make help       # todos los targets
```

Ningún puerto es fijo: `make start` reusa el que ya publica un contenedor vivo o toma el primero
libre. Fíjalos con `BACKEND_PORT=… FRONTEND_PORT=… make start`.

## Cómo se trabaja aquí

- **Principios de ingeniería** (layering, dinero, seguridad, testing, comentarios):
  [`.specify/memory/constitution.md`](.specify/memory/constitution.md). Es la única copia.
- **Mecánica del repo** (comandos, puertos, supply chain, hooks): [`AGENTS.md`](AGENTS.md).
- **Features nuevas** pasan por spec-kit: `/speckit-specify` → `plan` → `tasks` → `analyze` →
  `implement`. El paso `analyze` no es opcional en este repo.
- **Documentación**: el índice de [`docs/`](docs/README.md) marca qué es referencia viva y qué es
  histórico. Los planes de diseño de la construcción inicial están en `docs/design/` y **no**
  describen el estado actual.
