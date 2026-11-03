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
