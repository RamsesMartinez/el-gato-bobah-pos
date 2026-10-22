#!/usr/bin/env bash
# Prueba de scripts/respaldo-anonimo.sh: lo que llega crudo de una plataforma no sobrevive al
# anonimizado, y la verificación final lo cuenta.
#
# POR QUÉ. El anonimizador nació antes que los pedidos de plataforma y nadie lo movió cuando
# llegaron: el aviso y el detalle se guardan crudos —nombre, teléfono y dirección del cliente
# adentro de un texto— y pasaban intactos a la base de pruebas, con la verificación diciendo «0
# datos personales». Una verificación que no sabe de una tabla no puede fallar por ella. Esta prueba
# es estática a propósito: el script real necesita producción, y lo que se vigila aquí es que el
# hermano que no se movió no se vuelva a quedar atrás.
#
#   bash scripts/tests/respaldo-anonimo-test.sh
set -euo pipefail

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$SRC/scripts/respaldo-anonimo.sh"
fallas=0
falla() { echo "FALLA: $*" >&2; fallas=$((fallas + 1)); }

# Las tablas que guardan un cuerpo crudo de plataforma, sacadas de las migraciones y no de una lista
# escrita aquí: la siguiente que agregue una columna así entra sola a la prueba.
crudas=$(awk '
  tolower($0) ~ /create table/ { t = tolower($0); sub(/.*create table (if not exists )?/, "", t); sub(/[ (].*/, "", t) }
  $1 ~ /^raw_(body|detail|payload)$/ && t != "" { print t }
' "$SRC"/server/migrations/*.sql | sort -u)
[[ -n "$crudas" ]] || falla "no encontré ninguna tabla con cuerpo crudo: la prueba dejó de ver las migraciones"

truncate_line=$(grep -E '^truncate platform_' "$SCRIPT" || true)
fugas=$(sed -n '/^FUGAS=/,/")$/p' "$SCRIPT")
for t in $crudas platform_incoming_order_lines; do
  [[ " $truncate_line " == *"$t"[,\ \;]* ]] || falla "$t guarda datos crudos del cliente y el anonimizador no la trunca"
  grep -q "from $t)" <<<"$fugas" || falla "$t no está en la verificación final: quedaría con datos y el script diría 0 fugas"
done
# Sin CASCADE: arrastraría en silencio cualquier tabla que un día las referencie.
grep -qiE '^truncate platform_.*cascade' "$SCRIPT" && falla "el truncate de plataformas usa CASCADE"

if [[ "$fallas" -gt 0 ]]; then exit 1; fi
echo "ok: respaldo-anonimo.sh trunca y verifica $(wc -w <<<"$crudas") tablas con datos crudos de plataforma"
