#!/usr/bin/env bash
# Baja un respaldo de la base de PRODUCCIÓN, le borra los datos personales y lo restaura en una
# base de pruebas local.
#
# POR QUÉ EXISTE. La constitución (principio IV) exige que una migración se pruebe "contra una base
# restaurada de un respaldo real y con al menos dos empresas", y hasta hoy no había con qué: el
# harness de integración hace `drop schema public cascade` y migra desde cero. Con una base sembrada
# limpia, todo camino "por cada otra empresa" es un no-op y las formas que de verdad muerden —un
# pedido viejo sin nombre de folio, uno sin sesión de caja, los 158 que quedaron con la fecha
# equivocada— no existen. Una migración así pasa verde en local y en CI para romper en el VPS.
#
# ANONIMIZADO Y NO CRUDO. El respaldo trae nombres de cliente, notas de pedido y credenciales del
# personal. Una base que se usa en pruebas —y que algún día correrá en CI— no es lugar para eso, y
# el principio V ya trata el nombre del cliente y las notas como datos que no salen ni a los logs.
# Se borran AL RESTAURAR, en la misma transacción, no en un paso posterior que se puede olvidar.
#
# LO QUE NO SE TOCA, A PROPÓSITO: importes, fechas, estados, sesiones de caja, plataformas, folios y
# llaves foráneas. Ese es justo el material que estos tests vienen a probar; anonimizarlo sería
# volver a la base sembrada por otro camino.
set -euo pipefail

# El dump que se baja trae los datos CRUDOS de producción: nombres de cliente, notas, motivos de
# cancelación, hashes de contraseña y el pin_lookup. Dos guardas, porque una sola falla:
#
#   - umask 077: el archivo nace 600 y no 644. Lo único que lo separaba de cualquier usuario de la
#     máquina era el permiso por defecto.
#   - trap ... EXIT: se borra AL SALIR, pase lo que pase — también si el restore falla a la mitad.
#     Antes se quedaba en la raíz del repo para siempre, y lo único que lo mantenía fuera de un repo
#     PÚBLICO era una línea de .gitignore.
umask 077

VPS_INSTANCE=${VPS_INSTANCE:-pos-vps}
VPS_USER=${VPS_USER:-ramses_mtz96}
VPS_ZONE=${VPS_ZONE:-us-central1-a}
PROD_CONTAINER=${PROD_CONTAINER:-deploy-postgres-1}
PROD_DB=${PROD_DB:-gatobobah}
PROD_DB_USER=${PROD_DB_USER:-gatobobah}

# El rol de app necesita password para que los tests de RLS puedan conectarse como él. Es el mismo
# valor que fija el harness de integración; si cambia allá, cambia aquí.
APP_ROLE_PASSWORD=${APP_ROLE_PASSWORD:-test_app_pw}

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DESTINO_DUMP=${DESTINO_DUMP:-"$RAIZ/.respaldo-produccion.sql.gz"}
trap 'rm -f "$DESTINO_DUMP"' EXIT

# psql no está instalado en la caja de desarrollo (el Postgres de dev vive en un contenedor), así
# que se usa el cliente de la MISMA imagen que corre en producción. `--network host` es lo que hace
# que la URL con localhost:5490 signifique lo mismo dentro y fuera del contenedor; sin eso habría
# que reescribir el host y el puerto, y una URL reescrita a mano es justo donde se restaura sobre
# la base equivocada.
IMAGEN_PG=${IMAGEN_PG:-postgres:16-alpine}
psql_() { docker run --rm -i --network host "$IMAGEN_PG" psql "$@"; }

if [[ -z "${TEST_RESTORED_DATABASE_URL:-}" ]]; then
  cat >&2 <<'AYUDA'
Falta TEST_RESTORED_DATABASE_URL: la base donde se restaura el respaldo.

  export TEST_RESTORED_DATABASE_URL="postgres://gatobobah:gatobobah@localhost:5490/gatobobah_restaurado?sslmode=disable"

Va SEPARADA de TEST_DATABASE_URL a propósito: el harness de integración borra el esquema entero al
empezar cada test, así que restaurar el respaldo ahí lo destruiría en el primer `go test`.
AYUDA
  exit 1
fi

# Guarda contra el dedazo que cuesta caro: restaurar encima de la base de desarrollo. El nombre
# tiene que decir para qué es; sin esta línea, un TEST_RESTORED_DATABASE_URL copiado del de dev
# borra el trabajo del día sin preguntar.
NOMBRE_BASE="${TEST_RESTORED_DATABASE_URL##*/}"
NOMBRE_BASE="${NOMBRE_BASE%%\?*}"
case "$NOMBRE_BASE" in
  *restaurad*|*test*) ;;
  *)
    echo "El destino se llama '$NOMBRE_BASE'. Esta herramienta BORRA la base destino, así que solo" >&2
    echo "acepta un nombre que diga que es de pruebas (contiene 'restaurad' o 'test')." >&2
    exit 1
    ;;
esac

echo "==> 1/5 Bajando el respaldo de producción ($VPS_INSTANCE:$PROD_CONTAINER/$PROD_DB)"
# --no-owner: el dueño en producción y en la caja local se llaman igual hoy, pero depender de eso
# hace que el restore falle el día que alguien cambie uno. Los GRANT sí se conservan (NO se usa
# --no-privileges): el test de que `gatobobah_app` tiene permiso sobre cada tabla es justamente uno
# de los que esta base viene a hacer posibles, y quitarlos lo volvería un test que se prueba a sí
# mismo.
# gzip: el enlace a la VM es lento y el dump es texto; comprimir en el origen ahorra minutos.
gcloud compute ssh "$VPS_USER@$VPS_INSTANCE" --zone="$VPS_ZONE" --quiet \
  --command="docker exec $PROD_CONTAINER pg_dump -U $PROD_DB_USER -d $PROD_DB --no-owner | gzip -6" \
  > "$DESTINO_DUMP"

if [[ ! -s "$DESTINO_DUMP" ]]; then
  echo "El respaldo salió vacío. No se restaura nada." >&2
  exit 1
fi
echo "    $(du -h "$DESTINO_DUMP" | cut -f1) en $DESTINO_DUMP"

# La URL del destino, partida para poder hablarle al servidor sin la base (para el DROP/CREATE).
SIN_BASE="${TEST_RESTORED_DATABASE_URL%/*}"
QUERY=""
[[ "$TEST_RESTORED_DATABASE_URL" == *\?* ]] && QUERY="?${TEST_RESTORED_DATABASE_URL#*\?}"
URL_ADMIN="$SIN_BASE/postgres$QUERY"

echo "==> 2/5 Recreando la base destino ($NOMBRE_BASE)"
psql_ "$URL_ADMIN" -v ON_ERROR_STOP=1 -q -c "drop database if exists \"$NOMBRE_BASE\" with (force)"
psql_ "$URL_ADMIN" -v ON_ERROR_STOP=1 -q -c "create database \"$NOMBRE_BASE\""
# El rol tiene que existir ANTES del restore: el dump trae los GRANT que lo nombran, y sin el rol
# psql los rechaza uno por uno y la base queda a medias sin que el exit code lo diga.
psql_ "$URL_ADMIN" -v ON_ERROR_STOP=1 -q -c \
  "do \$\$ begin
     if not exists (select 1 from pg_roles where rolname = 'gatobobah_app') then
       create role gatobobah_app;
     end if;
   end \$\$;"

echo "==> 3/5 Restaurando"
gunzip -c "$DESTINO_DUMP" | psql_ "$TEST_RESTORED_DATABASE_URL" -v ON_ERROR_STOP=1 -q

echo "==> 4/5 Borrando datos personales y credenciales"
# Todo en UNA transacción: si algo falla a media anonimización, lo que NO se puede quedar es una
# base con la mitad de los datos personales todavía dentro y el script diciendo que falló.
psql_ "$TEST_RESTORED_DATABASE_URL" -v ON_ERROR_STOP=1 -q <<'SQL'
begin;

-- Cliente: lo único que identifica a una persona que compró.
update orders set customer_name = null, notes = null
 where customer_name is not null or notes is not null;
update order_lines set notes = null where notes is not null;

-- Texto libre que el personal escribe y donde ya se ha visto el nombre de quien reclamó.
update orders          set cancel_reason = 'motivo anonimizado' where cancel_reason is not null;
update orders          set refund_reason = 'motivo anonimizado' where refund_reason is not null;
update order_refunds   set reason        = 'motivo anonimizado' where reason        is not null;
update order_lines     set cancel_reason = 'motivo anonimizado' where cancel_reason is not null;
update register_sessions        set notes   = null where notes   is not null;
update cash_transfers           set note    = null where note    is not null;
update register_cash_movements  set concept = 'concepto anonimizado';
update stock_movements set reason = null, note = null where reason is not null or note is not null;

-- Personal: identidad y credenciales. El id se conserva para que todo join siga cuadrando, que es
-- de lo que dependen los reportes que estos tests miden.
update users set
  name           = 'Usuario ' || id,
  username       = 'usuario' || id,
  recovery_email = 'usuario' || id || '@ejemplo.invalid',
  password_hash  = '$2a$10$anonimizadoanonimizadoanonimizadoanonimizadoanonimizadoanoni',
  pin_hash       = null,
  pin_lookup     = null;

-- Sesiones vivas: no se anonimizan, se tiran. Un refresh token restaurado es una credencial
-- funcional de producción viviendo en una base de pruebas.
truncate refresh_tokens, password_reset_tokens;

-- Texto libre de gastos: ahí caben nombres de personas y de proveedores.
update expense_items set description = 'concepto anonimizado' where description is not null;
update expenses      set description = 'gasto anonimizado'    where description is not null;
-- Referencias de cobro: números de tarjeta parciales y folios de transferencia reales.
update order_payments   set reference = null where reference is not null;
update expense_payments set reference = null where reference is not null;

-- Proveedores y datos de contacto del negocio.
update suppliers set phone = null, notes = null where phone is not null or notes is not null;
update business_settings set
  phone      = null,
  address    = null,
  -- El logo son hasta 256 KB de binario que ningún test mira y que engordan el respaldo.
  logo_bytes = null, logo_mime = null, logo_updated_at = null;

commit;
SQL

echo "==> 5/5 Preparando el rol de app y aplicando migraciones pendientes"
psql_ "$URL_ADMIN" -v ON_ERROR_STOP=1 -q -c \
  "alter role gatobobah_app with login password '$APP_ROLE_PASSWORD'"
psql_ "$URL_ADMIN" -v ON_ERROR_STOP=1 -q -c \
  "grant connect on database \"$NOMBRE_BASE\" to gatobobah_app"
# Mismo GUC por defecto que fija el harness: las conexiones del OWNER (que salta RLS) auto-sellan
# company_id sin que cada test lo ponga. Se usa la empresa de id más bajo, que es la dueña del
# histórico — la misma regla que usó la migración 0037 para decidir de quién eran los pagos.
EMPRESA_BASE=$(psql_ "$TEST_RESTORED_DATABASE_URL" -tAc "select min(id) from companies")
psql_ "$URL_ADMIN" -v ON_ERROR_STOP=1 -q -c \
  "alter database \"$NOMBRE_BASE\" set app.company_id = '$EMPRESA_BASE'"

( cd "$RAIZ/server" && DATABASE_URL="$TEST_RESTORED_DATABASE_URL" go run ./cmd/migrate )

# Verificación final. Un respaldo restaurado que se quedó con UNA empresa no sirve para lo que se
# bajó: con una sola, todo camino "por cada otra empresa" es un no-op y la migración pasa verde
# para romper en producción. Falla ruidoso antes que dejar creer que la base está lista.
EMPRESAS=$(psql_ "$TEST_RESTORED_DATABASE_URL" -tAc "select count(*) from companies")
PEDIDOS=$(psql_ "$TEST_RESTORED_DATABASE_URL" -tAc "select count(*) from orders")
# La verificación cubre TODAS las sentencias de arriba, no tres de trece: antes pasaba en verde
# aunque diez de ellas no hubieran corrido.
FUGAS=$(psql_ "$TEST_RESTORED_DATABASE_URL" -tAc "
  select (select count(*) from orders where customer_name is not null or notes is not null)
       + (select count(*) from order_lines where notes is not null)
       + (select count(*) from users where name !~ '^Usuario [0-9]+\$' or pin_hash is not null or pin_lookup is not null)
       + (select count(*) from refresh_tokens)
       + (select count(*) from password_reset_tokens)
       + (select count(*) from suppliers where phone is not null or notes is not null)
       + (select count(*) from business_settings where phone is not null or address is not null or logo_bytes is not null)
       + (select count(*) from register_sessions where notes is not null)
       + (select count(*) from cash_transfers where note is not null)
       + (select count(*) from stock_movements where reason is not null or note is not null)
       + (select count(*) from order_payments where reference is not null)
       + (select count(*) from expense_payments where reference is not null)")

echo
echo "Base restaurada: $NOMBRE_BASE"
echo "  empresas: $EMPRESAS   pedidos: $PEDIDOS   datos personales que quedaron: $FUGAS"

if [[ "$EMPRESAS" -lt 2 ]]; then
  echo "FALLA: la base quedó con $EMPRESAS empresa(s) y la constitución exige al menos dos." >&2
  exit 1
fi
if [[ "$FUGAS" -ne 0 ]]; then
  echo "FALLA: quedaron $FUGAS filas con datos personales. No uses esta base." >&2
  exit 1
fi
echo "Lista. Los tests que la usan corren con:"
echo "  TEST_RESTORED_DATABASE_URL=\"$TEST_RESTORED_DATABASE_URL\" go test -tags=integration ./internal/integration/..."
