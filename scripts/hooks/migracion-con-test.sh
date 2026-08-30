#!/usr/bin/env bash
# Una migración nueva no se commitea sin su test de integración (constitución, principio IV).
#
# POR QUÉ ESTO EXISTE, y no un recordatorio más: la regla ya estaba escrita y cargada en contexto
# cuando se escribió la migración 0037 antes que sus tests. Los tests, al escribirse después,
# encontraron un defecto real (auto_declare sin fijar, que en una instalación nueva pediría contar
# físicamente el dinero de las plataformas). Salió bien por suerte. Una regla que solo se lee se
# olvida; una que falla el commit, no.
#
# El efecto de una migración es un cambio de esquema y de datos: no hay función que llamar, así que
# solo un test de integración contra Postgres real puede verificarla. Y los fallos que trae —un
# `grant` faltante, una política de RLS, una FK que cruza empresas— son INVISIBLES en local, porque
# la API de desarrollo se conecta como owner y sin APP_DATABASE_URL.
set -euo pipefail

migraciones="$(git diff --cached --name-only --diff-filter=A -- 'server/migrations/*.sql' || true)"
[ -z "$migraciones" ] && exit 0

tests="$(git diff --cached --name-only -- 'server/internal/integration/*_test.go' || true)"
if [ -n "$tests" ]; then
  exit 0
fi

echo "Migración nueva sin test de integración en el mismo commit:"
echo "$migraciones" | sed 's/^/  /'
echo
echo "Agrega su test en server/internal/integration/ y vuelve a commitear."
echo "Lo que ese test tiene que cubrir, y que ninguna otra cosa ve:"
echo "  - los GRANT de las tablas nuevas, probados bajo appRoleStore (rol gatobobah_app, no owner)"
echo "  - la política de RLS: que una empresa no vea ni escriba lo de otra"
echo "  - si la migración mueve datos, que el conteo y las sumas de dinero no cambien"
echo
echo "Si de verdad no aplica (una migración que solo agrega un índice, por ejemplo), commitea el"
echo "test junto de todos modos o explica el porqué en el mensaje y usa --no-verify SOLO con el"
echo "visto bueno del dueño: la constitución prohíbe saltarse los hooks por comodidad."
exit 1
