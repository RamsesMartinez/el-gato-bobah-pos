# El Gato Bobah POS — monorepo (web/ = frontend Vite, server/ = backend Go)
.PHONY: help install start stop check check-env deps-up deps-down \
        web-dev web-build web-test api-dev api-run api-build api-test \
        sqlc sqlc-diff sqlc-vet db-migrate migrate-new fudo-import reset-admin reset-password build deploy \
        prod-db-tunnel prod-reset-password deploy-image
.DEFAULT_GOAL := help

# Puertos de la infra dev. Son env con default (y no un número fijo) porque el 5433/6380 de
# antes chocaba con el postgres/redis de otros compose locales. Overridea con
# `PG_PORT=… make db-migrate` si start.sh tuvo que mover el puerto.
PG_PORT    ?= 5490
REDIS_PORT ?= 6390
export PG_PORT
export REDIS_PORT
DEV_DATABASE_URL ?= postgres://gatobobah:gatobobah@localhost:$(PG_PORT)/gatobobah?sslmode=disable
DEV_REDIS_URL    ?= redis://localhost:$(REDIS_PORT)
DEV_JWT_SECRET   ?= dev-secret-no-usar-en-prod
# sqlc pinado (no @latest): el código generado y `sqlc vet` deben ser reproducibles entre local
# y CI. Súbelo a mano aquí y en .github/workflows/ci.yml a la vez.
SQLC_VERSION     ?= v1.31.1
# En Windows `go env GOPATH` regresa la ruta con backslashes y el sh de make se los come
# (C:\Users\… queda C:Users…), rompiendo lint/vuln/sqlc/goose. Normalizar a "/" es no-op fuera de ahí.
GOBIN := $(subst \,/,$(shell go env GOPATH))/bin
export DEV_DATABASE_URL DEV_REDIS_URL DEV_JWT_SECRET

help: ## Lista los targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

check: ## Verifica prerequisitos (docker, bun, go, node) y dice qué falta
	@bash scripts/check.sh

check-env: ## Valida que deploy/.env tenga las variables requeridas configuradas
	@bash scripts/check-env.sh

install: ## Prepara TODO: valida entorno + variables, instala deps y herramientas, baja imágenes
	@bash scripts/check.sh || (echo "Instala lo que falta arriba y vuelve a correr 'make install'"; exit 1)
	@bash scripts/check-env.sh || (echo "Configura deploy/.env y vuelve a correr 'make install'"; exit 1)
	@echo "▶ Instalando dependencias del frontend (bun)…"
	cd web && bun install
	@echo "▶ Descargando dependencias del backend (go)…"
	cd server && go mod download
	@echo "▶ Instalando herramientas Go (sqlc, goose, air, linters de seguridad)…"
	@GOBIN="$(GOBIN)" go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	@GOBIN="$(GOBIN)" go install github.com/pressly/goose/v3/cmd/goose@latest
	@GOBIN="$(GOBIN)" go install github.com/air-verse/air@latest
	@GOBIN="$(GOBIN)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@GOBIN="$(GOBIN)" go install golang.org/x/vuln/cmd/govulncheck@latest
	@GOBIN="$(GOBIN)" go install github.com/evilmartians/lefthook/v2@latest
	@echo "▶ Instalando git hooks (lefthook)…"
	@$(GOBIN)/lefthook install
	@echo "▶ Bajando imágenes docker (postgres, redis)…"
	docker compose -f deploy/docker-compose.dev.yml pull
	@echo "\n✅ Listo. Corre 'make start' para levantar todo."

start: ## Levanta todo (postgres+redis+API+web); avisa si falta algo
	@bash scripts/start.sh

stop: ## Detiene todo lo que 'make start' dejó vivo (docker + API/web huérfanos)
	@bash scripts/stop.sh

# --- Infra dev (postgres + redis) ---
deps-up: ## Levanta postgres + redis (dev)
	docker compose -f deploy/docker-compose.dev.yml up -d
deps-down: ## Detiene postgres + redis (dev)
	docker compose -f deploy/docker-compose.dev.yml down

# --- Frontend ---
web-dev: ## Frontend en dev (Vite, http://localhost:3000)
	cd web && bun run dev
web-build: ## Build de producción del frontend
	cd web && bun run build
web-test: ## Tests del frontend (vitest)
	cd web && bun run test

# --- Backend ---
api-dev: deps-up ## API con hot reload (air); secretos desde deploy/.env
	@bash scripts/dev-api.sh air
api-run: deps-up ## API sin air (go run); secretos desde deploy/.env
	@bash scripts/dev-api.sh
api-build: ## Compila el backend
	cd server && go build ./...
api-test: ## Tests del backend
	cd server && go test ./...

# --- Seguridad / calidad ---
lint: ## Lint backend (golangci-lint + gosec)
	cd server && $(GOBIN)/golangci-lint run ./...
vuln: ## Vulnerabilidades backend (govulncheck)
	cd server && $(GOBIN)/govulncheck ./...
web-lint: ## Lint frontend (eslint + tsc)
	cd web && bun run lint && bun run typecheck
web-audit: ## Auditoría de deps del frontend
	cd web && bun audit || true
sec: lint vuln web-lint web-audit ## Todos los chequeos de seguridad/calidad
	@echo "\n✅ Chequeos de seguridad completados"

reset-admin: deps-up ## Actualiza contraseña/PIN del admin desde deploy/.env (sin borrar datos)
	@bash scripts/dev-api.sh reset-admin
reset-password: deps-up ## Resetea password de un usuario (prompt oculto): make reset-password user=admin@gatobobah
	@bash scripts/dev-api.sh reset-password "$(user)"

parse-doc: ## Extrae un ticket/factura de compra y lo imprime: make parse-doc f=docs/tickets/ticket.pdf
	@cd server && ENV_FILE=../deploy/.env go run ./cmd/parse-doc "../$(f)"
parse-docs: ## Corre la extracción sobre todos los documentos de docs/tickets/ (verificación)
	@cd server && ENV_FILE=../deploy/.env go run ./cmd/parse-doc ../docs/tickets/*.pdf

sqlc: ## Regenera el código sqlc
	cd server && $(GOBIN)/sqlc generate
sqlc-diff: ## Falla si el código sqlc generado no está al día (olvidaste `make sqlc`)
	cd server && $(GOBIN)/sqlc diff
db-migrate: deps-up ## Aplica las migraciones embebidas a la DB de dev (:$(PG_PORT))
	cd server && DATABASE_URL="$(DEV_DATABASE_URL)" go run ./cmd/migrate
sqlc-vet: db-migrate ## Prepara TODA query contra el esquema real (db-prepare) — atrapa drift esquema↔query
	cd server && SQLC_DB_URI="$(DEV_DATABASE_URL)" $(GOBIN)/sqlc vet
migrate-new: ## Crea migración goose: make migrate-new name=xxx
	cd server && $(GOBIN)/goose -dir migrations create $(name) sql
fudo-import: deps-up ## Importa el catálogo FUDO desde references/ (y limpia la cache del menú)
	cd server && DATABASE_URL="$(DEV_DATABASE_URL)" go run ./cmd/fudo-import --dir ../references
	@docker compose -f deploy/docker-compose.dev.yml exec -T redis redis-cli DEL pos:menu >/dev/null 2>&1 || true
	@echo "cache del menú limpiada"

# --- Producción ---
build: ## Build de imágenes de producción (API + web, self-contained, sin bun en host)
	GIT_SHA="$$(git rev-parse --short HEAD)" BUILT_AT="$$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
		docker compose -f deploy/docker-compose.yml build
deploy: ## Deploy compilando en la máquina local (fallback; en el VPS usa deploy-image)
	@bash scripts/check-env.sh || (echo "Configura deploy/.env antes de desplegar"; exit 1)
	$(MAKE) build
	docker compose -f deploy/docker-compose.yml up -d
deploy-image: ## Deploy bajando la imagen ya compilada por CI (API_IMAGE=ghcr.io/...:sha-xxxx)
	@bash scripts/check-env.sh || (echo "Configura deploy/.env antes de desplegar"; exit 1)
	docker compose -f deploy/docker-compose.yml pull api
	docker compose -f deploy/docker-compose.yml up -d
	@docker image prune -f >/dev/null 2>&1 || true

# Túnel SSH a Postgres de la VPS, para inspeccionar con DataGrip/psql sin exponer el puerto a
# internet. Resuelve la IP interna del contenedor en cada corrida (puede cambiar si se recrea).
# Corre en primer plano a propósito — Ctrl+C lo cierra, nada queda vivo en segundo plano.
prod-db-tunnel: ## Túnel local :5434 -> Postgres de producción (Ctrl+C para cerrar)
	@echo "Túnel localhost:5434 -> Postgres (VPS). Ctrl+C para cerrar."
	@PG_IP=$$(gcloud compute ssh ramses_mtz96@pos-vps --zone=us-central1-a --quiet \
		--command="docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' deploy-postgres-1"); \
	gcloud compute ssh ramses_mtz96@pos-vps --zone=us-central1-a -- -N -L 5434:$$PG_IP:5432

# Corre el binario YA compilado dentro del contenedor api en producción (sin `go` en la VPS,
# solo Docker). --ssh-flag=-t + `exec -it` fuerzan una pty de punta a punta, si no el prompt
# oculto del password (term.ReadPassword) no tiene terminal real donde leer.
prod-reset-password: ## Resetea password en la VPS (prompt oculto): make prod-reset-password user=admin@gatobobah
	gcloud compute ssh ramses_mtz96@pos-vps --zone=us-central1-a --ssh-flag="-t" --command="cd el-gato-bobah-pos && \
		docker compose -f deploy/docker-compose.yml exec -it api /api -reset-password='$(user)'"
