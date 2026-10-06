#!/usr/bin/env bash
# Apunta api-dev.elgatobobah.com a la IP que Google le dio a pos-vps-dev en este arranque.
#
#   scripts/dns-ambiente-dev.sh        # lo corre la skill ambiente-dev después de `on`
#
# Por qué existe: la VM de pruebas ya no tiene IP fija. Pasa casi todo el tiempo apagada, y una IP
# reservada sin usar se cobra más cara que en uso; sin IP fija, cada arranque trae una IP distinta.
# El registro es DNS-only (sin proxy de Cloudflare) y con TTL de 60 s, para que el cambio se vea
# en un minuto y Caddy pueda renovar su certificado por HTTP contra la IP nueva.
#
# Solo toca el registro de api-dev, y se niega a correr si la instancia no es pos-vps-dev: la de
# producción tiene IP fija y su DNS no lo mueve nada automático.
#
# El token sale de CLOUDFLARE_API_TOKEN (permiso Zone·DNS·Edit sobre elgatobobah.com) y, si no
# está en el entorno, de ~/.claude/settings.json. Nunca va en el repositorio, que es público.
set -euo pipefail

INSTANCE=pos-vps-dev
ZONE_GCP=us-central1-a
PROJECT=el-gato-bobah-pos
DOMAIN=elgatobobah.com
RECORD=api-dev.$DOMAIN

TOKEN="${CLOUDFLARE_API_TOKEN:-}"
if [[ -z "$TOKEN" && -f "$HOME/.claude/settings.json" ]]; then
  TOKEN=$(jq -r '.env.CLOUDFLARE_API_TOKEN // empty' "$HOME/.claude/settings.json")
fi
[[ -n "$TOKEN" ]] || { echo "FALLO: falta CLOUDFLARE_API_TOKEN" >&2; exit 1; }

IP=$(gcloud compute instances describe "$INSTANCE" --zone "$ZONE_GCP" --project "$PROJECT" \
  --format='value(networkInterfaces[0].accessConfigs[0].natIP)')
[[ -n "$IP" ]] || { echo "FALLO: $INSTANCE no tiene IP pública (¿está apagada?)" >&2; exit 1; }

cf() { curl -sf -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" "$@"; }
API=https://api.cloudflare.com/client/v4

ZONE_ID=$(cf "$API/zones?name=$DOMAIN" | jq -r '.result[0].id // empty')
[[ -n "$ZONE_ID" ]] || { echo "FALLO: el token no ve la zona $DOMAIN (¿venció?)" >&2; exit 1; }

read -r RECORD_ID ACTUAL < <(cf "$API/zones/$ZONE_ID/dns_records?type=A&name=$RECORD" \
  | jq -r '.result[0] | "\(.id // "") \(.content // "")"')
[[ -n "$RECORD_ID" ]] || { echo "FALLO: no existe el registro A de $RECORD" >&2; exit 1; }

if [[ "$ACTUAL" == "$IP" ]]; then
  echo "OK $RECORD ya apunta a $IP"
  exit 0
fi

cf -X PATCH "$API/zones/$ZONE_ID/dns_records/$RECORD_ID" \
  -d "{\"content\":\"$IP\",\"ttl\":60,\"proxied\":false}" | jq -e '.success' >/dev/null
echo "OK $RECORD: $ACTUAL -> $IP"
