#!/usr/bin/env bash
# Subida sin corte (spec 041): arranca la versión nueva en el lado libre de la API (azul o verde),
# espera que esté lista, cambia el upstream de Caddy con un reload y apaga la vieja dejando
# terminar lo que traía. Si la nueva no queda lista, el tráfico nunca se mueve.
#
# Lo llama `make deploy-image` con API_IMAGE. Contrato: specs/041-subidas-sin-corte/contracts/despliegue.md.
# Prueba: scripts/tests/deploy-azul-verde-test.sh.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

COMPOSE=(docker compose -f deploy/docker-compose.yml)
# Solo para la vuelta local (quickstart §2): un override con APP_ENV de desarrollo y puertos altos.
[ -z "${DEPLOY_COMPOSE_OVERRIDE:-}" ] || COMPOSE+=(-f "$DEPLOY_COMPOSE_OVERRIDE")
PROJECT=${COMPOSE_PROJECT_NAME:-deploy}
# El contenedor de antes de la spec 041, con un solo lado. Su nombre sale del proyecto: fijo,
# una vuelta local en otro proyecto no probaría el mismo camino que producción.
LEGACY="${PROJECT}-api-1"
UPSTREAM=deploy/upstream
ACTIVO=$UPSTREAM/activo.caddy
READY_TIMEOUT=${READY_TIMEOUT:-60}
READY_INTERVAL=${READY_INTERVAL:-2}
SETTLE_SECONDS=${SETTLE_SECONDS:-3}
MIN_MEM_MB=${MIN_MEM_MB:-100}

paso() { echo "deploy: $*"; }
muere() { echo "deploy: ERROR: $*" >&2; exit 1; }

# Escribe el upstream sin que Caddy pueda leer un archivo a medias: el tmp no empata con el
# patrón activo*.caddy que importa el Caddyfile.
escribir_activo() {
  printf 'to %s:8080\n' "$1" > "$ACTIVO.tmp.$$"
  mv -f "$ACTIVO.tmp.$$" "$ACTIVO"
}

recargar_caddy() {
  "${COMPOSE[@]}" exec -T caddy caddy reload --config /etc/caddy/Caddyfile
}

lista() {
  "${COMPOSE[@]}" exec -T "api-$1" /api -readycheck >/dev/null 2>&1
}

esperar_lista() {
  local fin=$((SECONDS + READY_TIMEOUT))
  until lista "$1"; do
    [ "$SECONDS" -lt "$fin" ] || return 1
    sleep "$READY_INTERVAL"
  done
}

# 0. Sin archivo, Caddy arranca pero no tiene a quién mandar. La instalación de antes servía en api:8080.
mkdir -p "$UPSTREAM"
[ -f "$ACTIVO" ] || escribir_activo api

# 1. Memoria para la segunda copia, antes de tocar nada.
libre_mb=$(awk '/^MemAvailable:/ { print int($2 / 1024) }' /proc/meminfo)
[ "$libre_mb" -ge "$MIN_MEM_MB" ] || muere "solo hay ${libre_mb} MB libres y la segunda copia pide ${MIN_MEM_MB} MB; no se tocó nada"

# 2. Qué lado atiende hoy. Cualquier otro contenido (api, vacío) es la instalación de antes.
case "$(cat "$ACTIVO")" in
  *api-blue:8080*) viejo=blue nuevo=green ;;
  *api-green:8080*) viejo=green nuevo=blue ;;
  *) viejo=legacy nuevo=blue ;;
esac
paso "atiende $viejo; la versión nueva (${API_IMAGE:-sin API_IMAGE}) va a api-$nuevo"

# 3. La nueva junto a la vieja. --force-recreate: si un despliegue interrumpido dejó viva esa copia
# con otra imagen, no se reusa. --no-deps: postgres y redis no se tocan.
# SKIP_PULL: `make deploy` compila la imagen en la máquina y no hay registro del cual bajarla.
[ "${SKIP_PULL:-0}" = 1 ] || "${COMPOSE[@]}" pull "api-$nuevo"
if ! "${COMPOSE[@]}" up -d --no-deps --force-recreate "api-$nuevo"; then
  "${COMPOSE[@]}" stop "api-$nuevo" || true
  muere "api-$nuevo no arrancó; el tráfico sigue en $viejo"
fi

# 4. Lista de verdad (incluye la base), no solo viva.
if ! esperar_lista "$nuevo"; then
  "${COMPOSE[@]}" logs --tail 50 "api-$nuevo" >&2 || true
  "${COMPOSE[@]}" stop "api-$nuevo" || true
  muere "api-$nuevo no quedó lista en ${READY_TIMEOUT} s; el tráfico sigue en $viejo"
fi

# 5. El cambio. Caddy nacido antes de la spec 041 no tiene montado el directorio de upstream ni ve
# el Caddyfile nuevo (git checkout reemplaza el archivo y el bind mount se queda con el viejo):
# hay que recrearlo, y eso es el único corte, una sola vez.
anterior=$(cat "$ACTIVO")
escribir_activo "api-$nuevo"
if docker inspect -f '{{range .Mounts}}{{.Destination}} {{end}}' "${PROJECT}-caddy-1" 2>/dev/null | grep -q /etc/caddy/upstream; then
  cambio=(recargar_caddy)
else
  # --timeout 1: Caddy se apaga con «eternal grace period» y espera a los avisos en vivo, que no
  # cierran solos; con el default de Docker eran 10 s sin escuchar (medido en la vuelta local).
  # Esos streams se cortan igual al recrearlo, y las tabletas reconectan solas.
  paso "Caddy sin el directorio de upstream: se recrea (corte breve, solo esta vez)"
  cambio=("${COMPOSE[@]}" up -d --no-deps --force-recreate --timeout 1 caddy)
fi
regresar() {
  printf '%s\n' "$anterior" > "$ACTIVO.tmp.$$" && mv -f "$ACTIVO.tmp.$$" "$ACTIVO"
  recargar_caddy || true
  "${COMPOSE[@]}" stop "api-$nuevo" || true
}
if ! "${cambio[@]}"; then
  regresar
  muere "Caddy no tomó el cambio a api-$nuevo; el tráfico sigue en $viejo"
fi

# 6. Sigue lista ya con tráfico. Si cayó, el tráfico regresa a la vieja, que sigue viva.
sleep "$SETTLE_SECONDS"
if ! lista "$nuevo"; then
  regresar
  muere "api-$nuevo dejó de estar lista después del cambio; el tráfico regresó a $viejo"
fi
paso "el tráfico ya va a api-$nuevo"

# 7. La vieja termina lo que traía: la API se da 25 s (shutdownTimeout) y Docker 30.
if [ "$viejo" = legacy ]; then
  if docker inspect "$LEGACY" >/dev/null 2>&1; then
    docker stop -t 30 "$LEGACY" || paso "aviso: $LEGACY no se detuvo limpio"
    docker rm "$LEGACY" >/dev/null || true
  fi
else
  "${COMPOSE[@]}" stop -t 30 "api-$viejo" || paso "aviso: api-$viejo no se detuvo limpio (el tráfico ya está en api-$nuevo)"
fi

# 8. Imágenes viejas: la VM tiene poco disco.
docker image prune -f >/dev/null 2>&1 || true
paso "listo: api-$nuevo atiende"
