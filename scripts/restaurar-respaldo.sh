#!/usr/bin/env bash
# Restaura un respaldo de producción en la base de DESARROLLO, conservando dueños y GRANTs.
#
#   make db-restaurar                      # el .dump más reciente de backups/prod/
#   make db-restaurar dump=backups/prod/pre-019-20260912-2135.dump
#
# Por qué NO lleva --no-owner ni --no-privileges: local tiene que negar lo mismo que producción.
# Con --no-privileges el rol gatobobah_app se queda sin un solo GRANT y la API que sirve como él no
# arranca; con --no-owner la vista `candidatas_del_aviso` (0073) pasa a ser del owner en vez de
# gatobobah_webhook, y el webhook resuelve tiendas con un bypass que en producción no existe. Los
# dos flags "hacen que el restore no se queje" y los dos fabrican un ambiente que miente.
#
# Borra la base de desarrollo entera. No toca producción: solo habla con el contenedor local.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTENEDOR="${PG_CONTAINER:-deploy-postgres-1}"
BASE="${POSTGRES_DB:-gatobobah}"
DUENO="${POSTGRES_USER:-gatobobah}"

DUMP="${1:-}"
if [ -z "$DUMP" ]; then
  DUMP="$(ls -t "$ROOT"/backups/prod/*.dump 2>/dev/null | head -1 || true)"
  [ -n "$DUMP" ] || { echo "No hay ningún .dump en backups/prod/ (ver backups/README.md para tomar uno)." >&2; exit 1; }
fi
[ -f "$DUMP" ] || { echo "No existe: $DUMP" >&2; exit 1; }

psql_() { docker exec -i "$CONTENEDOR" psql -U "$DUENO" -v ON_ERROR_STOP=1 -qAt "$@"; }

echo "==> Respaldo: $DUMP"
sha256sum "$DUMP"

# Los roles son del CLÚSTER, no de la base: en un volumen de Postgres recién creado no existen, y
# sin ellos cada GRANT y cada OWNER TO del dump falla. Se crean sin password; el bootstrap de la
# API se lo fija desde APP_DB_PASSWORD / PLATFORM_DB_PASSWORD al arrancar, como en producción.
psql_ -d postgres <<'SQL'
do $$
begin
  if not exists (select 1 from pg_roles where rolname = 'gatobobah_app') then
    create role gatobobah_app login;
  end if;
  if not exists (select 1 from pg_roles where rolname = 'gatobobah_platform') then
    create role gatobobah_platform nologin;
  end if;
  if not exists (select 1 from pg_roles where rolname = 'gatobobah_webhook') then
    create role gatobobah_webhook nologin nobypassrls;
  end if;
end $$;
SQL

echo "==> Recreando la base $BASE (se cierran las conexiones vivas: detén la API antes)"
psql_ -d postgres -c "select pg_terminate_backend(pid) from pg_stat_activity where datname = '$BASE' and pid <> pg_backend_pid()" >/dev/null
psql_ -d postgres -c "drop database if exists $BASE"
psql_ -d postgres -c "create database $BASE owner $DUENO"

docker cp "$DUMP" "$CONTENEDOR:/tmp/restaurar.dump"
docker exec "$CONTENEDOR" pg_restore -U "$DUENO" -d "$BASE" --exit-on-error /tmp/restaurar.dump
docker exec "$CONTENEDOR" rm -f /tmp/restaurar.dump

# Un restore que "terminó" no prueba que los permisos llegaron. Se comprueba lo mismo que la API
# comprueba al arrancar: el rol de servicio tiene GRANT y no salta RLS.
PERMISOS="$(psql_ -d "$BASE" -c "select has_table_privilege('gatobobah_app', 'orders', 'select')::text || ' ' || (select rolbypassrls from pg_roles where rolname = 'gatobobah_app')::text")"
if [ "$PERMISOS" != "true false" ]; then
  echo "✗ gatobobah_app quedó sin permisos o saltando RLS (select sobre orders, bypassrls = $PERMISOS)." >&2
  exit 1
fi

echo "==> Listo. Empresas restauradas:"
psql_ -d "$BASE" -c "select id || ' ' || slug from companies order by id"
echo "Arranca la API (make start / make api-dev): aplica las migraciones que falten y sirve como gatobobah_app."
