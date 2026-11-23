# Se carga con `source` desde los scripts de dev: si este worktree tiene su infraestructura propia
# (.env.worktree.local, ver worktree-env.sh), sus puertos y su proyecto de compose mandan. En la
# carpeta principal no hay archivo y no cambia nada.
_wt_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -f "$_wt_root/.env.worktree.local" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$_wt_root/.env.worktree.local"
  set +a
fi
unset _wt_root
# Un worktree creado antes de la spec 091 no trae MEDIA_PORT: sale de su número (9300+NN), como lo
# escribiría worktree-env.sh, para que su almacenamiento de fotos no choque con el de otro.
if [ -z "${MEDIA_PORT:-}" ] && [[ "${COMPOSE_PROJECT_NAME:-}" =~ ^egb([0-9]+)$ ]]; then
  export MEDIA_PORT=$((9300 + 10#${BASH_REMATCH[1]}))
fi
