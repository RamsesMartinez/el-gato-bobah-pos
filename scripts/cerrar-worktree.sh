#!/usr/bin/env bash
# cerrar-worktree.sh — apaga y quita el worktree de una feature ya mergeada.
#
#   bash scripts/cerrar-worktree.sh ../egb-041-subidas-sin-corte          # deja sus volúmenes
#   bash scripts/cerrar-worktree.sh ../egb-041-subidas-sin-corte --datos  # borra también su base
#
# Se niega si el worktree tiene cambios sin commit o commits que no están en origin: cerrar no
# puede tirar trabajo. Baja SOLO la infraestructura de ese worktree (su proyecto de compose), por
# los mismos guardias de `make stop`/`make deps-down` (nunca el proyecto `deploy` de la carpeta
# principal).
set -euo pipefail

DEST="${1:-}"; DATOS="${2:-}"
[[ -d "$DEST" && -f "$DEST/.env.worktree.local" ]] || { echo "Uso: bash scripts/cerrar-worktree.sh <carpeta del worktree> [--datos]" >&2; exit 2; }
DEST="$(cd "$DEST" && pwd)"
BRANCH="$(git -C "$DEST" rev-parse --abbrev-ref HEAD)"

if [[ -n "$(git -C "$DEST" status --porcelain)" ]]; then
  echo "✗ $DEST tiene cambios sin commit; no lo cierro." >&2; exit 1
fi
git -C "$DEST" fetch -q origin
if [[ -n "$(git -C "$DEST" log --oneline "$BRANCH" --not --remotes 2>/dev/null)" ]]; then
  echo "✗ $BRANCH tiene commits que no están en origin; súbelos o descártalos antes." >&2; exit 1
fi

echo "▶ Deteniendo API, web y contenedores de $DEST"
(cd "$DEST" && make -s stop)
if [[ "$DATOS" == "--datos" ]]; then
  # `make deps-down` no borra volúmenes; se borran solo los del proyecto de ESTE worktree, por
  # etiqueta. El de la carpeta principal (`deploy`) nunca: ahí vive la base de desarrollo del dueño.
  PROJECT="$(grep '^COMPOSE_PROJECT_NAME=' "$DEST/.env.worktree.local" | cut -d= -f2)"
  [[ -n "$PROJECT" && "$PROJECT" != "deploy" ]] || { echo "✗ proyecto de compose inválido: '$PROJECT'" >&2; exit 1; }
  (cd "$DEST" && make -s deps-down) || true
  docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" | xargs -r docker volume rm >/dev/null
  echo "  volúmenes de $PROJECT borrados"
fi

MAIN="$(git -C "$DEST" worktree list --porcelain | awk '/^worktree /{print $2; exit}')"
git -C "$MAIN" worktree remove "$DEST"
# La rama local sobra si ya está mergeada en develop; si no, se queda (no se pierde nada en origin).
git -C "$MAIN" fetch -q origin develop
if git -C "$MAIN" merge-base --is-ancestor "$BRANCH" origin/develop 2>/dev/null; then
  git -C "$MAIN" branch -D "$BRANCH" >/dev/null && echo "  rama $BRANCH borrada (ya está en develop)"
fi
echo "✅ Cerrado: $DEST"
