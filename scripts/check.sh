#!/usr/bin/env bash
# Preflight: verifica que todo lo necesario para 'make start' esté instalado y corriendo.
# Sale con código 1 y una lista clara de lo que falta si algo no está listo.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GREEN='\033[0;32m'; RED='\033[0;31m'; YEL='\033[0;33m'; NC='\033[0m'
missing=0

ok()   { printf "  ${GREEN}✓${NC} %s\n" "$1"; }
bad()  { printf "  ${RED}✗${NC} %s\n     ${YEL}→ %s${NC}\n" "$1" "$2"; missing=1; }
warn() { printf "  ${YEL}!${NC} %s\n" "$1"; }

echo "Revisando prerequisitos para El Gato Bobah POS…"

# --- bun ---
if command -v bun >/dev/null 2>&1; then ok "bun $(bun --version)"
else bad "bun no instalado" "instálalo: curl -fsSL https://bun.sh/install | bash"; fi

# --- Node 24 ---
if command -v node >/dev/null 2>&1; then
  major=$(node -v | sed 's/v\([0-9]*\).*/\1/')
  if [ "$major" -ge 24 ] 2>/dev/null; then ok "node $(node -v)"
  else warn "node $(node -v) — el proyecto usa v24 (.nvmrc). Corre: nvm use"; fi
else bad "node no instalado" "usa nvm e instala Node 24: nvm install"; fi

# --- Go 1.25+ ---
if command -v go >/dev/null 2>&1; then
  gv=$(go version | sed 's/.*go\([0-9]*\.[0-9]*\).*/\1/')
  ok "go $gv"
else bad "go no instalado" "instala Go 1.25+: https://go.dev/dl"; fi

# --- Docker + daemon ---
if command -v docker >/dev/null 2>&1; then
  if docker info >/dev/null 2>&1; then ok "docker (daemon corriendo)"
  else bad "docker instalado pero el daemon no corre" "abre Docker Desktop / inicia el servicio docker"; fi
else bad "docker no instalado" "instala Docker: https://docs.docker.com/get-docker"; fi

# --- docker compose v2 ---
if docker compose version >/dev/null 2>&1; then ok "docker compose"
else bad "docker compose (v2) no disponible" "actualiza Docker a una versión con 'compose' integrado"; fi

# --- herramientas Go (opcionales, las instala 'make install') ---
GOBIN="$(go env GOPATH 2>/dev/null)/bin"
for tool in sqlc goose air; do
  if [ -x "$GOBIN/$tool" ] || command -v "$tool" >/dev/null 2>&1; then ok "$tool"
  else warn "$tool no instalado (corre 'make install' para instalarlo)"; fi
done

# --- .env del backend en dev ---
# (en dev las credenciales las inyecta el Makefile; en prod se usa deploy/.env)
if [ ! -f "$ROOT/deploy/.env" ]; then
  warn "deploy/.env no existe (solo necesario para 'make deploy'; copia deploy/.env.example)"
fi

echo
if [ "$missing" -ne 0 ]; then
  printf "${RED}Faltan requisitos.${NC} Resuelve los ✗ de arriba y vuelve a intentar.\n"
  exit 1
fi
printf "${GREEN}Todo listo.${NC} Puedes correr 'make start'.\n"
