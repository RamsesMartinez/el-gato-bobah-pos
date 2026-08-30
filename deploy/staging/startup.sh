#!/bin/bash
# Startup script de la VM de pruebas (api-dev). Corre en CADA arranque, así que es idempotente:
# la instancia es spot y Google puede reclamarla en cualquier momento, así que tiene que volver
# sola sin que nadie entre por SSH.
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y -qq ca-certificates curl gnupg
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
  chmod a+r /etc/apt/keyrings/docker.asc
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
fi
systemctl enable --now docker

# Los contenedores se levantan solos tras una interrupción de spot: `restart: unless-stopped` en el
# compose lo cubre mientras el docker daemon arranque, que es lo que asegura la línea de arriba.
