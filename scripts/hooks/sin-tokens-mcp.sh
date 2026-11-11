#!/usr/bin/env bash
# Un token del servidor MCP (spec 072) no se commitea.
#
# El repositorio es público y el token opera el negocio con los permisos de su dueño hasta que
# alguien lo revoque. El camino para que acabe aquí es corto: un `.mcp.json` del proyecto con el
# valor pegado en vez de `${POS_MCP_TOKEN}`, o un fixture de prueba copiado de la pantalla.
set -euo pipefail

encontrados="$(git diff --cached -U0 --no-color | grep -E '^\+' | grep -oE 'egbpat_[0-9]+_[0-9]+_[0-9a-f]{64}' || true)"
[ -z "$encontrados" ] && exit 0

echo "Hay un token del servidor MCP en lo que vas a commitear:"
git diff --cached --name-only -G 'egbpat_[0-9]+_[0-9]+_[0-9a-f]{64}' | sed 's/^/  /'
echo
echo "Quítalo y usa la variable de entorno (\${POS_MCP_TOKEN}). Si ya salió de tu máquina,"
echo "revócalo en Integraciones y crea otro."
exit 1
