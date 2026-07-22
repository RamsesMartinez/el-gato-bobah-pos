#!/usr/bin/env bash
# Detiene TODO lo que 'make start' levanta: postgres+redis (docker) y los procesos de
# API (Go) y web (Vite). start.sh elige puertos DINÁMICOS, así que no podemos asumir
# 8080/3000; la regla de detección es "proceso que escucha un puerto Y cuya cwd cuelga
# de este repo" — pilla API+web caiga donde caiga el puerto y NO mata un :3000 de otro
# proyecto que casualmente esté corriendo.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "▶ Bajando postgres + redis (docker)…"
docker compose -f "$ROOT/deploy/docker-compose.dev.yml" down 2>/dev/null || true

if ! command -v lsof >/dev/null 2>&1; then
  echo "⚠ 'lsof' no disponible: no puedo detectar API/web por puerto — ciérralos a mano (Ctrl-C en 'make start')."
  exit 0
fi

# cwd de un pid; en macOS/Linux lsof -Fn imprime 'n<ruta>' para el fd cwd.
cwd_of() { lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1; }
in_repo() { case "$1" in "$ROOT"|"$ROOT"/*) return 0 ;; *) return 1 ;; esac; }

kill_repo_listeners() { # $1 = señal (TERM/KILL); devuelve nº de procesos señalados
  local sig="$1" n=0 pid cwd cmd
  for pid in $(lsof -iTCP -sTCP:LISTEN -t -n -P 2>/dev/null | sort -u); do
    cwd="$(cwd_of "$pid")"
    in_repo "$cwd" || continue
    cmd="$(ps -p "$pid" -o comm= 2>/dev/null)"
    [ "$sig" = "TERM" ] && echo "  → $pid ($cmd)  cwd=$cwd"
    kill "-$sig" "$pid" 2>/dev/null && n=$((n + 1))
  done
  echo "$n"
}

echo "▶ Deteniendo API + web de este repo…"
n="$(kill_repo_listeners TERM | tail -1)"
if [ "${n:-0}" -gt 0 ]; then
  # 'go run' deja un binario hijo que puede ignorar TERM: segunda pasada -9 a lo que siga.
  sleep 1
  kill_repo_listeners KILL >/dev/null
fi

echo "✅ Stack detenido."
# ponytail: cubre lo que 'make start' lanza (go run + vite). 'air' (make api-dev) no
# escucha él mismo el puerto; si lo usas, córtalo con Ctrl-C. Añadir su pkill si molesta.
