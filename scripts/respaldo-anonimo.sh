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
# Se borran en el mismo script que restaura, no en un paso posterior que se puede olvidar, y si algo
# falla antes de que la verificación final diga 0, la base se tira entera (ver limpiar).
#
# LO QUE NO SE TOCA, A PROPÓSITO: importes, fechas, estados, sesiones de caja, plataformas, folios y
# llaves foráneas. La excepción son los pedidos que llegan de las plataformas, que se tiran enteros
# (ver el paso 5). Ese es justo el material que estos tests vienen a probar; anonimizarlo sería
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
# Y la base destino: en cuanto se restaura trae todo lo crudo, y si algo falla antes de que la
# verificación final diga 0 —el paso que borra, una migración, la propia verificación— se tira
# entera. Antes el rollback evitaba una base «a medias», pero dejaba la base completa sin anonimizar
# y el script terminaba diciendo solo «no la uses».
BASE_ANONIMA=0
limpiar() {
  rm -f "$DESTINO_DUMP"
  if [[ "$BASE_ANONIMA" != 1 && -n "${URL_ADMIN:-}" ]]; then
    echo "No se terminó de anonimizar: se borra $NOMBRE_BASE para que no quede con datos crudos." >&2
    psql_ "$URL_ADMIN" -q -c "drop database if exists \"$NOMBRE_BASE\" with (force)" \
      || echo "NO SE PUDO BORRAR $NOMBRE_BASE, que tiene datos crudos de producción: bórrala a mano." >&2
  fi
}
trap limpiar EXIT

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
# Sin --no-owner ni --no-privileges: los dueños y los GRANT llegan como están en producción. Los
# GRANT porque el test de que `gatobobah_app` tiene permiso sobre cada tabla es justamente uno de
# los que esta base viene a hacer posibles. Los dueños porque `candidatas_del_aviso` (0073) es de
# gatobobah_webhook a propósito: con --no-owner pasa a ser del owner, salta RLS, y los tests del
# webhook corren contra un bypass que allá no existe. Que los roles se llamen igual aquí y allá es
# requisito, no casualidad (constitución IV, «La base local niega lo mismo que producción»).
# gzip: el enlace a la VM es lento y el dump es texto; comprimir en el origen ahorra minutos.
gcloud compute ssh "$VPS_USER@$VPS_INSTANCE" --zone="$VPS_ZONE" --quiet \
  --command="docker exec $PROD_CONTAINER pg_dump -U $PROD_DB_USER -d $PROD_DB | gzip -6" \
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
# Los roles tienen que existir ANTES del restore: el dump trae los GRANT y los OWNER TO que los
# nombran, y sin el rol psql los rechaza uno por uno y la base queda a medias sin que el exit code
# lo diga.
psql_ "$URL_ADMIN" -v ON_ERROR_STOP=1 -q -c \
  "do \$\$ begin
     if not exists (select 1 from pg_roles where rolname = 'gatobobah_app') then
       create role gatobobah_app;
     end if;
     if not exists (select 1 from pg_roles where rolname = 'gatobobah_platform') then
       create role gatobobah_platform nologin;
     end if;
     if not exists (select 1 from pg_roles where rolname = 'gatobobah_webhook') then
       create role gatobobah_webhook nologin nobypassrls;
     end if;
   end \$\$;"

echo "==> 3/5 Restaurando"
gunzip -c "$DESTINO_DUMP" | psql_ "$TEST_RESTORED_DATABASE_URL" -v ON_ERROR_STOP=1 -q

echo "==> 4/5 Preparando el rol de app y aplicando migraciones pendientes"
# ANTES de borrar: así el paso que borra corre contra el esquema de esta rama, que es el que valida
# TestEveryColumnIsCoveredByTheAnonymousBackup. Al revés, una columna que esta rama agrega y el
# paso 5 borra todavía no existe en el respaldo, y el paso truena.
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

echo "==> 5/5 Borrando datos personales y credenciales"
# Todo en UNA transacción: si algo falla a media anonimización no queda una base con la mitad de los
# datos personales; el rollback la deja cruda y limpiar la borra al salir.
psql_ "$TEST_RESTORED_DATABASE_URL" -v ON_ERROR_STOP=1 -q <<'SQL'
begin;

-- Las restricciones NOT VALID se apartan mientras se borra. Producción tiene filas viejas que no
-- las cumplen (por eso se agregaron sin validar: un renglón de compra en $0 antes de la 0092), y
-- Postgres las vuelve a revisar en CUALQUIER update de la fila, aunque la columna no tenga que ver:
-- el paso tronaba con el respaldo real. Vuelven igual, sin validar, antes del commit.
create temp table unvalidated_constraints on commit drop as
  select conrelid::regclass::text as table_name, conname as constraint_name,
         pg_get_constraintdef(oid) as definition
    from pg_constraint
   -- conrelid = 0 es una restricción de dominio, y una heredada no se suelta en la hija.
   where contype = 'c' and not convalidated and connamespace = 'public'::regnamespace
     and conrelid <> 0 and conislocal;
do $$ declare r record; begin
  for r in select * from unvalidated_constraints loop
    execute format('alter table %s drop constraint %I', r.table_name, r.constraint_name);
  end loop;
end $$;

-- Cliente: lo único que identifica a una persona que compró.
update orders set customer_name = null, notes = null
 where customer_name is not null or notes is not null;
update order_lines set notes = null where notes is not null;
-- Las cuentas abiertas que todavía no se cobran (0082) llevan el mismo nombre y las mismas notas.
update order_drafts      set customer_name = null where customer_name is not null;
update order_draft_lines set notes         = null where notes         is not null;

-- Texto libre que el personal escribe y donde ya se ha visto el nombre de quien reclamó. Donde un
-- check exige texto, se reemplaza en vez de anularlo.
update orders          set cancel_reason = 'motivo anonimizado' where cancel_reason is not null;
update orders          set refund_reason = 'motivo anonimizado' where refund_reason is not null;
update orders          set written_off_reason = 'motivo anonimizado' where written_off_reason is not null;
update order_refunds   set reason        = 'motivo anonimizado' where reason        is not null;
update order_lines     set cancel_reason = 'motivo anonimizado' where cancel_reason is not null;
update order_payment_voids set reason = 'motivo anonimizado', reference = null;
update register_sessions        set notes   = null where notes   is not null;
update register_sessions        set opening_reason_note = 'nota anonimizada' where opening_reason_note is not null;
update session_cash_counts      set manual_reason = 'motivo anonimizado' where manual_reason is not null;
update cash_transfers           set note    = null where note    is not null;
update register_cash_movements  set concept = 'concepto anonimizado';
-- La propina entregada guarda copia del nombre de quien la recibió.
update register_cash_movements  set recipient_name = 'Usuario ' || recipient_user_id
 where recipient_name is not null;
update stock_movements set reason = null, note = null where reason is not null or note is not null;
update waste_records   set note = null where note is not null;
update attendance_corrections set reason = 'motivo anonimizado';
-- IP y descripción del aparato de quien checó.
update attendance_entries set in_ip = null, out_ip = null, in_device = null, out_device = null
 where in_ip is not null or out_ip is not null or in_device is not null or out_device is not null;

-- Bitácora: el antes y el después copian los motivos de arriba (y lo que se agregue mañana), y
-- guarda la IP y el aparato. Es de solo agregar ni siquiera para el owner (0089); el trigger se
-- apaga solo dentro de esta transacción y vuelve antes del commit.
alter table audit_events disable trigger audit_events_append_only;
update audit_events set before = null, after = null, ip = null, device = null
 where before is not null or after is not null or ip is not null or device is not null;
alter table audit_events enable trigger audit_events_append_only;

-- Personal: identidad y credenciales. El id se conserva para que todo join siga cuadrando, que es
-- de lo que dependen los reportes que estos tests miden.
update users set
  name           = 'Usuario ' || id,
  username       = 'usuario' || id,
  recovery_email = 'usuario' || id || '@ejemplo.invalid',
  password_hash  = '$2a$10$anonimizadoanonimizadoanonimizadoanonimizadoanonimizadoanoni',
  pin_hash       = null,
  pin_lookup     = null;
-- Operadores de la consola de plataforma: lo mismo.
update platform_operators set
  name          = 'Operador ' || id,
  username      = 'operador' || id,
  password_hash = '$2a$10$anonimizadoanonimizadoanonimizadoanonimizadoanonimizadoanoni';

-- Pedidos que llegan de las plataformas: se tiran enteros. El aviso y el detalle se guardan
-- CRUDOS, tal como los mandó la plataforma —nombre, teléfono y dirección del cliente adentro de un
-- texto— y no hay columna que anular: habría que reescribir JSON de otro. Nada fuera de estas tres
-- tablas las referencia, así que van sin CASCADE: si un día alguien las referencia, el truncate
-- falla aquí en vez de arrastrar en silencio una tabla que no se pidió. El pedido que ya entró al
-- POS se queda en `orders`, con el cliente anulado arriba.
truncate platform_webhook_events, platform_incoming_order_lines, platform_incoming_orders;

-- Sesiones vivas: no se anonimizan, se tiran. Un refresh token restaurado es una credencial
-- funcional de producción viviendo en una base de pruebas.
truncate refresh_tokens, password_reset_tokens;

-- Texto libre de gastos: ahí caben nombres de personas y de proveedores.
update expense_items set description = 'concepto anonimizado' where description is not null;
update expenses      set description = 'gasto anonimizado'    where description is not null;
update expenses      set cancel_reason = 'motivo anonimizado'  where cancel_reason is not null;
-- La lectura del ticket: el documento entero del proveedor, con sus datos fiscales.
update expenses      set doc_raw = null where doc_raw is not null;
-- Referencias de cobro: números de tarjeta parciales y folios de transferencia reales.
update order_payments   set reference = null where reference is not null;
update expense_payments set reference = null where reference is not null;
-- El folio de la devolución en la terminal; un check lo exige junto con quien lo capturó.
update order_refunds set card_refund_folio = 'folio anonimizado' where card_refund_folio is not null;

-- Proveedores y datos de contacto del negocio.
update suppliers set phone = null, notes = null where phone is not null or notes is not null;
-- El nombre también: un proveedor puede ser una persona física, y con los gastos queda cuánto se le
-- pagó y cuándo.
update suppliers set name = 'Proveedor ' || id;
-- La marca de una presentación (0104) suele ser el proveedor, y su copia en la compra también. Una por
-- presentación y no en nulo: dos «Caja» del mismo insumo con marca distinta chocarían en el índice
-- único al quedar sin marca.
update ingredient_presentations set brand = 'Marca ' || id where brand is not null;
update expense_items set presentation_brand = 'Marca ' || coalesce(presentation_id, id) where presentation_brand is not null;
-- Preferencias de pantalla: la API acepta cualquier clave y cualquier JSON de quien tenga sesión, así
-- que no se puede prometer qué hay adentro. Ninguna prueba las necesita.
truncate user_preferences;
update business_settings set
  phone      = null,
  address    = null,
  -- El encabezado y el pie del ticket repiten dirección y teléfono.
  header_note = null,
  footer_note = null,
  -- Correos de las personas que reciben el resumen del día.
  daily_summary_emails = '{}',
  -- El logo son hasta 256 KB de binario que ningún test mira y que engordan el respaldo.
  logo_bytes = null, logo_mime = null, logo_updated_at = null;
update branches set address = null, phone = null, postal_code = null
 where address is not null or phone is not null or postal_code is not null;
-- El error del envío del resumen trae el correo al que no se pudo entregar.
update daily_summary_sends set last_error = null where last_error is not null;

-- pg_get_constraintdef termina en «NOT VALID»: vuelven sin validar, como estaban.
do $$ declare r record; begin
  for r in select * from unvalidated_constraints loop
    execute format('alter table %s add constraint %I %s', r.table_name, r.constraint_name, r.definition);
  end loop;
end $$;

commit;
SQL


# Verificación final. Un respaldo restaurado que se quedó con UNA empresa no sirve para lo que se
# bajó: con una sola, todo camino "por cada otra empresa" es un no-op y la migración pasa verde
# para romper en producción. Falla ruidoso antes que dejar creer que la base está lista.
EMPRESAS=$(psql_ "$TEST_RESTORED_DATABASE_URL" -tAc "select count(*) from companies")
PEDIDOS=$(psql_ "$TEST_RESTORED_DATABASE_URL" -tAc "select count(*) from orders")
# La verificación cubre TODO lo que borra el paso 5, una subconsulta por renglón: antes pasaba en
# verde aunque diez sentencias no hubieran corrido, y después no supo de los borradores de pedido.
# La prueba TestEveryColumnIsCoveredByTheAnonymousBackup exige que cada columna del paso 5 esté
# nombrada aquí, en un renglón de su tabla.
FUGAS=$(psql_ "$TEST_RESTORED_DATABASE_URL" -tAc "
  select (select count(*) from orders where customer_name is not null or notes is not null)
       + (select count(*) from orders where cancel_reason <> 'motivo anonimizado' or refund_reason <> 'motivo anonimizado' or written_off_reason <> 'motivo anonimizado')
       + (select count(*) from order_lines where notes is not null or cancel_reason <> 'motivo anonimizado')
       + (select count(*) from order_drafts where customer_name is not null)
       + (select count(*) from order_draft_lines where notes is not null)
       + (select count(*) from order_refunds where reason <> 'motivo anonimizado' or card_refund_folio <> 'folio anonimizado')
       + (select count(*) from order_payment_voids where reason <> 'motivo anonimizado' or reference is not null)
       + (select count(*) from users where name !~ '^Usuario [0-9]+\$' or username !~ '^usuario[0-9]+\$' or recovery_email !~ '^usuario[0-9]+@ejemplo[.]invalid\$' or password_hash !~ 'anonimizado' or pin_hash is not null or pin_lookup is not null)
       + (select count(*) from platform_operators where name !~ '^Operador [0-9]+\$' or username !~ '^operador[0-9]+\$' or password_hash !~ 'anonimizado')
       + (select count(*) from refresh_tokens)
       + (select count(*) from password_reset_tokens)
       + (select count(*) from suppliers where phone is not null or notes is not null or name !~ '^Proveedor [0-9]+\$')
       + (select count(*) from ingredient_presentations where brand !~ '^Marca [0-9]+\$')
       + (select count(*) from expense_items where presentation_brand !~ '^Marca [0-9]+\$')
       + (select count(*) from user_preferences)
       + (select count(*) from business_settings where phone is not null or address is not null or header_note is not null or footer_note is not null or daily_summary_emails <> '{}' or logo_bytes is not null or logo_mime is not null)
       + (select count(*) from branches where address is not null or phone is not null or postal_code is not null)
       + (select count(*) from daily_summary_sends where last_error is not null)
       + (select count(*) from register_sessions where notes is not null or opening_reason_note <> 'nota anonimizada')
       + (select count(*) from session_cash_counts where manual_reason <> 'motivo anonimizado')
       + (select count(*) from cash_transfers where note is not null)
       + (select count(*) from register_cash_movements where concept <> 'concepto anonimizado' or recipient_name <> 'Usuario ' || recipient_user_id)
       + (select count(*) from stock_movements where reason is not null or note is not null)
       + (select count(*) from waste_records where note is not null)
       + (select count(*) from attendance_corrections where reason <> 'motivo anonimizado')
       + (select count(*) from attendance_entries where in_ip is not null or out_ip is not null or in_device is not null or out_device is not null)
       + (select count(*) from audit_events where before is not null or after is not null or ip is not null or device is not null)
       + (select count(*) from expense_items where description <> 'concepto anonimizado')
       + (select count(*) from expenses where description <> 'gasto anonimizado' or cancel_reason <> 'motivo anonimizado' or doc_raw is not null)
       + (select count(*) from order_payments where reference is not null)
       + (select count(*) from expense_payments where reference is not null)
       + (select count(*) from platform_webhook_events)
       + (select count(*) from platform_incoming_orders)
       + (select count(*) from platform_incoming_order_lines)")

echo
echo "Base restaurada: $NOMBRE_BASE"
echo "  empresas: $EMPRESAS   pedidos: $PEDIDOS   datos personales que quedaron: $FUGAS"

if [[ "$FUGAS" -ne 0 ]]; then
  echo "FALLA: quedaron $FUGAS filas con datos personales. Se borra la base." >&2
  exit 1
fi
BASE_ANONIMA=1
if [[ "$EMPRESAS" -lt 2 ]]; then
  echo "FALLA: la base quedó con $EMPRESAS empresa(s) y la constitución exige al menos dos." >&2
  exit 1
fi
echo "Lista. Los tests que la usan corren con:"
echo "  TEST_RESTORED_DATABASE_URL=\"$TEST_RESTORED_DATABASE_URL\" go test -tags=integration ./internal/integration/..."
