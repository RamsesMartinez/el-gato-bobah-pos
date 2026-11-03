#!/usr/bin/env bash
# Se niega a bajar los contenedores compartidos (proyecto `deploy`) desde un worktree.
#
# POR QUÉ. Sin .env.worktree.local, docker compose toma el proyecto de la carpeta del compose
# (`deploy/`), el mismo de la carpeta principal: un `make stop` o `make deps-down` en un worktree
# baja los contenedores de la carpeta principal, y con -v borra su base. Pasó el 2026-10-12: se
# perdió el volumen deploy_pgdata_dev y hubo que restaurarlo de un respaldo.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=worktree-env-load.sh
source "$ROOT/scripts/worktree-env-load.sh"

git_dir="$(git -C "$ROOT" rev-parse --git-dir 2>/dev/null || echo x)"
common="$(git -C "$ROOT" rev-parse --git-common-dir 2>/dev/null || echo x)"
project="${COMPOSE_PROJECT_NAME:-deploy}"

if [ "$git_dir" != "$common" ] && [ "$project" = "deploy" ]; then
  echo "Este es un worktree sin infraestructura propia: bajar el proyecto «deploy» tiraría los" >&2
  echo "contenedores de la carpeta principal. Corre antes make worktree-env n=NN." >&2
  exit 1
fi
