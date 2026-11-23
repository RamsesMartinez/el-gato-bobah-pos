#!/usr/bin/env bash
# Siembra pedidos de plataforma PENDIENTES en la base local de este worktree, para probar la alerta
# de pedido entrante (spec 093) sin una plataforma de verdad.
#
#   make entrantes-de-prueba            # uno
#   make entrantes-de-prueba n=3        # tres
#   make entrantes-de-prueba slug=otro  # en otra empresa (por omisión, gatobobah)
#
# Aceptarlos y rechazarlos desde la tableta pide la API con PLATFORM_ORDERS_SIMULATED=true, que solo
# arranca en desarrollo. La tableta los ve en el siguiente sondeo (15 s) o al recargar.
#
# Solo habla con el Postgres del worktree. Se niega a correr sin .env.worktree.local o si el
# contenedor resultante es el de la carpeta principal (deploy-postgres-1): una base compartida no se
# llena de pedidos de prueba.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=worktree-env-load.sh
source "$ROOT/scripts/worktree-env-load.sh"

PROYECTO="${COMPOSE_PROJECT_NAME:-deploy}"
CONTENEDOR="${PROYECTO}-postgres-1"
if [ "$PROYECTO" = "deploy" ]; then
  echo "Este checkout no tiene base propia: corre antes make worktree-env n=NN. No se siembran pedidos" >&2
  echo "de prueba en la base de la carpeta principal." >&2
  exit 1
fi
N="${1:-1}"
SLUG="${2:-gatobobah}"
case "$N" in ''|*[!0-9]*) echo "n debe ser un número entero: $N" >&2; exit 1 ;; esac
[ "$N" -ge 1 ] && [ "$N" -le 20 ] || { echo "n va de 1 a 20: $N" >&2; exit 1; }
case "$SLUG" in ''|*[!a-z0-9-]*) echo "slug inválido: $SLUG" >&2; exit 1 ;; esac

docker exec -i "$CONTENEDOR" psql -U "${POSTGRES_USER:-gatobobah}" -d "${POSTGRES_DB:-gatobobah}" \
  -v ON_ERROR_STOP=1 -q -v n="$N" -v slug="$SLUG" <<'SQL'
\set QUIET on
-- psql no sustituye variables dentro de $$: se pasan como ajustes de la sesión.
select set_config('seed.slug', :'slug', false), set_config('seed.n', :'n', false) \g /dev/null
do $$
declare
  empresa bigint;
  plataforma smallint;
  conexion bigint;
  pedido bigint;
  folio text;
  nombres text[] := array['Daniela','Jorge','Fernanda','Luis','Marisol','Toño'];
  prod record;
begin
  select id into empresa from companies where slug = current_setting('seed.slug');
  if empresa is null then raise exception 'no existe la empresa %', current_setting('seed.slug'); end if;
  select id into plataforma from delivery_platforms where name = 'Uber Eats';
  -- La conexión de prueba, aparte de cualquier tienda real que traiga un respaldo.
  select id into conexion from platform_connections
   where company_id = empresa and delivery_platform_id = plataforma and external_store_id = 'tienda-de-prueba-local';
  if conexion is null then
    insert into platform_connections (delivery_platform_id, external_store_id, label, company_id)
    values (plataforma, 'tienda-de-prueba-local', 'Tienda de prueba (local)', empresa) returning id into conexion;
  end if;
  -- Aceptar cobra con el método «en línea» de la plataforma: sin él, aceptar falla.
  if not exists (select 1 from payment_methods where company_id = empresa and delivery_platform_id = plataforma and not is_cash) then
    insert into payment_methods (company_id, name, kind, delivery_platform_id, is_cash, affects_cash_drawer, is_active, sort_key, auto_declare)
    values (empresa, 'Uber Eats en línea', 'plataforma', plataforma, false, false, true, 400, false);
  end if;
  for i in 1..current_setting('seed.n')::int loop
    folio := 'PRUEBA-' || to_char(clock_timestamp(), 'HH24MISSMS') || '-' || i;
    insert into platform_incoming_orders (connection_id, external_order_id, display_id, placed_at, decide_before,
           total, service_type, customer_name, company_id)
    values (conexion, folio, upper(substr(md5(folio), 1, 5)), now(), now() + interval '11 minutes',
            0, (case when i % 2 = 0 then 'para_llevar' else 'domicilio' end)::service_type,
            nombres[1 + (i - 1) % array_length(nombres, 1)], empresa)
    returning id into pedido;
    -- Dos productos activos del catálogo, emparejados, y uno que no está en el menú.
    for prod in select p.id, p.name, coalesce(p.price, 50) as price from products p
                 where p.company_id = empresa and p.is_active order by random() limit 2 loop
      insert into platform_incoming_order_lines (incoming_order_id, external_item_id, external_name, quantity, unit_price, product_id, company_id)
      values (pedido, 'item-' || prod.id, prod.name, 1 + (i % 2), prod.price, prod.id, empresa);
    end loop;
    insert into platform_incoming_order_lines (incoming_order_id, external_item_id, external_name, quantity, unit_price, company_id)
    values (pedido, 'item-sin-pareja', 'Producto de temporada', 1, 45, empresa);
    update platform_incoming_orders o set total = (select sum(quantity * unit_price) from platform_incoming_order_lines l where l.incoming_order_id = o.id)
     where o.id = pedido;
  end loop;
  raise notice 'Sembrados % pedidos pendientes en %', current_setting('seed.n'), current_setting('seed.slug');
end $$;
SQL
