#!/usr/bin/env bash
# Prueba de scripts/deploy-azul-verde.sh (spec 041): qué lado arranca, cuándo cambia Caddy y qué
# deja si algo sale mal. Usa un `docker` falso que anota cada llamada; nunca toca un contenedor.
#
# POR QUÉ. El script decide a qué lado va el tráfico de producción. Sus caminos de falla —la nueva
# no queda lista, se cae justo después del cambio, el despliegue se interrumpe— no se ven en una
# subida que sale bien, y son los que importan: el que se equivoca deja la API sin atender.
#
#   bash scripts/tests/deploy-azul-verde-test.sh
set -euo pipefail

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fallas=0
falla() { echo "FALLA [$caso]: $*" >&2; fallas=$((fallas + 1)); }

mkdir -p "$TMP/bin"
cat > "$TMP/bin/docker" <<'EOF'
#!/usr/bin/env bash
# Respuestas controladas por FAKE_*; cada llamada queda en $DOCKER_LOG.
echo "$*" >> "$DOCKER_LOG"
args="$*"
case "$args" in
  *"exec -T api-blue /api -readycheck"*|*"exec -T api-green /api -readycheck"*)
    lado=green; [[ "$args" == *api-blue* ]] && lado=blue
    var="FAKE_READY_$lado"; seq="${!var:-ok}"
    n=$(cat "$FAKE_DIR/ready_$lado" 2>/dev/null || echo 0); echo $((n + 1)) > "$FAKE_DIR/ready_$lado"
    IFS=, read -ra pasos <<< "$seq"; i=$n; [ "$i" -ge "${#pasos[@]}" ] && i=$((${#pasos[@]} - 1))
    [ "${pasos[$i]}" = ok ]; exit ;;
  *"exec -T caddy caddy reload"*)
    echo "RELOAD $(cat "$FAKE_REPO/deploy/upstream/activo.caddy")" >> "$DOCKER_LOG"; exit 0 ;;
  "inspect -f"*caddy-1)
    echo "${FAKE_CADDY_MOUNTS:-/data /etc/caddy/Caddyfile /etc/caddy/upstream}"; exit 0 ;;
  "inspect "*-api-1)
    [ "${FAKE_LEGACY:-0}" = 1 ]; exit ;;
esac
exit 0
EOF
chmod +x "$TMP/bin/docker"

# Repositorio mínimo: el script y el directorio de upstream, nada más.
nuevo_repo() {
  rm -rf "$TMP/repo" "$TMP/fake"; mkdir -p "$TMP/repo/scripts" "$TMP/repo/deploy/upstream" "$TMP/fake"
  cp "$SRC/scripts/deploy-azul-verde.sh" "$TMP/repo/scripts/"
  : > "$TMP/docker.log"
}
activo() { cat "$TMP/repo/deploy/upstream/activo.caddy" 2>/dev/null || echo "(sin archivo)"; }
correr() { # variables FAKE_* y COMPOSE_PROJECT_NAME por entorno; deja el código en $codigo
  set +e
  (cd "$TMP/repo" && PATH="$TMP/bin:$PATH" DOCKER_LOG="$TMP/docker.log" FAKE_DIR="$TMP/fake" FAKE_REPO="$TMP/repo" \
    API_IMAGE=img:sha-1 READY_TIMEOUT=2 READY_INTERVAL=0 SETTLE_SECONDS=0 "$@" bash scripts/deploy-azul-verde.sh) > "$TMP/out" 2>&1
  codigo=$?
  set -e
}
log_tiene() { grep -qF -- "$1" "$TMP/docker.log"; }
nunca_up_sin_servicio() {
  if grep -E 'compose .* up -d *$' "$TMP/docker.log" >/dev/null; then falla "corrió «up -d» sin nombrar servicio: levantaría los dos lados"; fi
}

caso="primera subida"
nuevo_repo
correr env FAKE_LEGACY=1 FAKE_CADDY_MOUNTS="/data /etc/caddy/Caddyfile"
[ "$codigo" = 0 ] || falla "salió $codigo: $(cat "$TMP/out")"
[ "$(activo)" = "to api-blue:8080" ] || falla "activo = $(activo); quería api-blue"
log_tiene "up -d --no-deps --force-recreate api-blue" || falla "no arrancó api-blue"
log_tiene "up -d --no-deps --force-recreate --timeout 1 caddy" || falla "no recreó Caddy (con espera corta), que nació sin el directorio de upstream montado"
log_tiene "stop -t 30 deploy-api-1" || falla "no apagó el contenedor anterior deploy-api-1"
nunca_up_sin_servicio

caso="azul a verde"
nuevo_repo; echo "to api-blue:8080" > "$TMP/repo/deploy/upstream/activo.caddy"
correr env
[ "$codigo" = 0 ] || falla "salió $codigo: $(cat "$TMP/out")"
[ "$(activo)" = "to api-green:8080" ] || falla "activo = $(activo)"
log_tiene "RELOAD to api-green:8080" || falla "no recargó Caddy con api-green"
log_tiene "--force-recreate caddy" && falla "recreó Caddy en una subida normal: eso corta"
log_tiene "stop -t 30 api-blue" || falla "no apagó api-blue"
log_tiene "deploy-api-1" && falla "tocó el contenedor anterior fuera de la primera subida"
# El orden importa: Caddy cambia ANTES de apagar la vieja, o hay un hueco sin destino.
[ "$(grep -n 'RELOAD' "$TMP/docker.log" | head -1 | cut -d: -f1)" -lt "$(grep -n 'stop -t 30 api-blue' "$TMP/docker.log" | cut -d: -f1)" ] \
  || falla "apagó la vieja antes de mover el tráfico"
nunca_up_sin_servicio

caso="verde a azul"
nuevo_repo; echo "to api-green:8080" > "$TMP/repo/deploy/upstream/activo.caddy"
correr env
[ "$codigo" = 0 ] && [ "$(activo)" = "to api-blue:8080" ] || falla "salió $codigo con activo = $(activo)"
log_tiene "stop -t 30 api-green" || falla "no apagó api-green"

caso="la nueva no queda lista"
nuevo_repo; echo "to api-blue:8080" > "$TMP/repo/deploy/upstream/activo.caddy"
correr env FAKE_READY_green=fail
[ "$codigo" != 0 ] || falla "salió 0 con la nueva sin quedar lista"
[ "$(activo)" = "to api-blue:8080" ] || falla "movió el tráfico a una copia que no quedó lista: activo = $(activo)"
log_tiene "RELOAD" && falla "recargó Caddy sin que la nueva estuviera lista"
log_tiene "stop api-green" || falla "dejó corriendo la copia que no quedó lista"
log_tiene "stop -t 30 api-blue" && falla "apagó la vieja, que es la única que atiende"
grep -q "lista" "$TMP/out" || falla "el error no dice por qué: $(cat "$TMP/out")"

caso="se cae justo después del cambio"
nuevo_repo; echo "to api-blue:8080" > "$TMP/repo/deploy/upstream/activo.caddy"
correr env FAKE_READY_green=ok,fail
[ "$codigo" != 0 ] || falla "salió 0 con la nueva caída tras el cambio"
[ "$(activo)" = "to api-blue:8080" ] || falla "no regresó el tráfico: activo = $(activo)"
[ "$(grep RELOAD "$TMP/docker.log" | tail -1)" = "RELOAD to api-blue:8080" ] || falla "la última recarga no regresó a api-blue: $(grep RELOAD "$TMP/docker.log")"
log_tiene "stop -t 30 api-blue" && falla "apagó la vieja con la nueva caída"

caso="archivo a medio escribir"
nuevo_repo; echo "to api-blue:8080" > "$TMP/repo/deploy/upstream/activo.caddy"
printf 'to api-gr' > "$TMP/repo/deploy/upstream/activo.caddy.tmp"
correr env
[ "$codigo" = 0 ] && [ "$(activo)" = "to api-green:8080" ] || falla "un tmp a medio escribir cambió la decisión: salió $codigo, activo = $(activo)"

caso="proyecto de compose de un worktree"
nuevo_repo
correr env COMPOSE_PROJECT_NAME=egb41 FAKE_LEGACY=1
log_tiene "stop -t 30 egb41-api-1" || falla "no derivó el contenedor anterior del proyecto: $(grep stop "$TMP/docker.log")"
log_tiene "deploy-api-1" && falla "habló con deploy-api-1 desde el proyecto egb41"

caso="sin memoria para la segunda copia"
nuevo_repo; echo "to api-blue:8080" > "$TMP/repo/deploy/upstream/activo.caddy"
correr env MIN_MEM_MB=999999999
[ "$codigo" != 0 ] || falla "arrancó sin memoria suficiente"
log_tiene "up -d" && falla "llamó a docker up sin memoria suficiente"
[ "$(activo)" = "to api-blue:8080" ] || falla "cambió el activo"

if [ "$fallas" -gt 0 ]; then echo "deploy-azul-verde: $fallas falla(s)" >&2; exit 1; fi
echo "deploy-azul-verde: ok"
