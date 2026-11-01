#!/usr/bin/env bash
# Vigila qué pasa solo y qué pregunta el hook de producción.
set -uo pipefail
hook="$(dirname "$0")/../hooks/proteger-produccion.sh"
fallas=0
caso() {
  local espera=$1 cmd=$2 sale
  sale=$(jq -n --arg c "$cmd" '{tool_input:{command:$c}}' | bash "$hook" | jq -r '.hookSpecificOutput.permissionDecision // "none"')
  [ -z "$sale" ] && sale=none
  if [ "$sale" != "$espera" ]; then echo "✗ esperaba $espera y salió $sale: $cmd"; fallas=$((fallas+1)); fi
}
ssh='gcloud compute ssh pos-vps --zone=us-central1-a --quiet --command='
caso allow "${ssh}'sudo journalctl SYSLOG_IDENTIFIER=api --since \"-30 min\" -o cat | grep -c \" 5[0-9][0-9] \"'"
caso allow "${ssh}'docker ps --format {{.Names}}'"
caso allow "${ssh}'curl -s localhost:8080/readyz'"
caso allow "${ssh}'sudo -u postgres pg_dump gatobobah'"
caso ask   "${ssh}'docker compose -f deploy/docker-compose.yml up -d'"
caso ask   "${ssh}'docker restart deploy-api-1'"
caso ask   "${ssh}'journalctl -n 5; rm -rf /srv'"
caso ask   "${ssh}'docker exec deploy-postgres-1 psql -c \"delete from orders\"'"
caso ask   "${ssh}'cat /opt/pos/deploy/.env'"
caso ask   "${ssh}'journalctl -n 5 > /tmp/x'"
caso allow "gcloud compute ssh ramses_mtz96@pos-vps --zone=us-central1-a --quiet --command='sudo journalctl SYSLOG_IDENTIFIER=api -o cat | grep -E \"\\\"status\\\":5[0-9][0-9]\" | tail -15; echo ---' 2>&1 | tail -18"
caso ask   "${ssh}'journalctl -n 5 2>/tmp/x'"
caso ask   "${ssh}'sudo systemctl restart docker'"
caso ask   "gcloud compute ssh pos-vps --zone=us-central1-a -- -N -L 5434:10.0.0.2:5432"
caso ask   "${ssh}'ls /srv'"
caso none  "gcloud compute ssh pos-vps-dev --command='docker restart api'"
[ $fallas -eq 0 ] && echo "✓ proteger-produccion: todos los casos" || exit 1
