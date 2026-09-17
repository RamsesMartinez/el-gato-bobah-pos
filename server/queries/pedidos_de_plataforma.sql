-- Recibir los pedidos de las plataformas (spec 021).
--
-- Casi todo va bajo RLS y ninguna consulta filtra por company_id: la política tenant_isolation ya
-- lo hace. LA EXCEPCIÓN es ResolverTiendaDeAviso, que corre SIN empresa fijada a propósito — está
-- marcada y explicada abajo, y es la única de todo el repositorio que puede hacerlo.

-- ---------------------------------------------------------------------------------------------
-- Las llaves con las que se verifica la firma
-- ---------------------------------------------------------------------------------------------

-- name: UpsertWebhookKey :exec
-- Capturar o reemplazar la llave primaria. `rotated_at` se limpia: poner una primaria nueva cierra
-- cualquier rotación a medias que hubiera quedado abierta.
insert into platform_webhook_keys (delivery_platform_id, key_primary)
values ($1, $2)
on conflict (company_id, delivery_platform_id) do update
   set key_primary = excluded.key_primary, key_secondary = null, rotated_at = null;

-- name: RotateWebhookKey :execrows
-- Empieza una rotación: la llave que estaba pasa a secundaria y entra la nueva como primaria.
-- Durante la rotación las DOS validan, que es el punto: cambiarla sin dejar de recibir pedidos.
update platform_webhook_keys
   set key_secondary = key_primary, key_primary = $2, rotated_at = now()
 where delivery_platform_id = $1;

-- name: FinishWebhookKeyRotation :execrows
-- Cierra la rotación tirando la llave vieja. Una secundaria que nadie retira se queda válida para
-- siempre y nadie se entera: `rotated_at` existe para poder delatarla.
update platform_webhook_keys
   set key_secondary = null, rotated_at = null
 where delivery_platform_id = $1;

-- name: GetWebhookKeyState :one
-- Para la pantalla. Devuelve SI HAY llave, no la llave: un secreto que se puede volver a leer desde
-- la interfaz es un secreto que se filtra por una captura de pantalla.
select (key_primary is not null) as configurada,
       (key_secondary is not null) as rotando,
       rotated_at
  from platform_webhook_keys
 where delivery_platform_id = $1;

-- ---------------------------------------------------------------------------------------------
-- Resolver de qué empresa es un aviso
-- ---------------------------------------------------------------------------------------------

-- name: ResolverTiendaDeAviso :many
--
-- LA ÚNICA CONSULTA DEL REPOSITORIO QUE CORRE SIN EMPRESA FIJADA, y tiene que ser así: quien llama
-- al webhook es la plataforma, no una persona. No hay sesión, no hay JWT y por lo tanto no hay
-- `app.company_id` — la empresa ES EL RESULTADO de esta consulta, no su entrada.
--
-- Por eso devuelve SOLO ids y las llaves de firma: nada del negocio sale por aquí. A partir de lo
-- que devuelve, todo lo demás corre con el tenant ya fijado.
--
-- DEVUELVE VARIAS FILAS A PROPÓSITO. Dos empresas pueden registrar el mismo id de tienda —pasa en
-- el ambiente de pruebas, donde las plataformas reparten tiendas de demostración compartidas— y
-- quien resuelve la ambigüedad es LA FIRMA: la empresa cuya llave valide el cuerpo es la dueña. Un
-- único global de (plataforma, tienda) parecía más limpio y se descartó: dejaría que la empresa la
-- determine un dato que cualquiera escribe en el cuerpo, en vez de quién pudo firmarlo.
select c.id as connection_id, c.company_id, c.is_active,
       k.key_primary, k.key_secondary
  from platform_connections c
  join delivery_platforms p
    on p.id = c.delivery_platform_id and p.company_id = c.company_id
  left join platform_webhook_keys k
    on k.delivery_platform_id = c.delivery_platform_id and k.company_id = c.company_id
 where c.external_store_id = $1 and lower(p.name) = lower(sqlc.arg(platform_name)::text);

-- ---------------------------------------------------------------------------------------------
-- Los avisos que llegaron
-- ---------------------------------------------------------------------------------------------

-- name: InsertWebhookEvent :one
-- Se escribe ANTES de llamar a la plataforma por el detalle, en su propia transacción: si el
-- proceso se muere a media llamada, el aviso ya está en disco y el reintento lo encuentra.
insert into platform_webhook_events (event_id, event_type, connection_id, raw_body)
values ($1, $2, $3, $4)
on conflict (event_id) do nothing
returning id;

-- name: MarkWebhookEventDone :exec
update platform_webhook_events
   set processed_at = now(), outcome = $2, failure_kind = $3
 where id = $1;

-- name: GetWebhookEventByExternalID :one
select id, event_id, event_type, connection_id, processed_at, outcome, failure_kind
  from platform_webhook_events
 where event_id = $1;

-- ---------------------------------------------------------------------------------------------
-- Los pedidos que esperan decisión
-- ---------------------------------------------------------------------------------------------

-- name: InsertIncomingOrder :one
insert into platform_incoming_orders (
  connection_id, external_order_id, display_id, placed_at, decide_before,
  service_type, customer_name, total, raw_detail
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
on conflict (company_id, connection_id, external_order_id) do nothing
returning id;

-- name: InsertIncomingOrderLine :one
insert into platform_incoming_order_lines (
  incoming_order_id, parent_line_id, external_item_id, external_name,
  quantity, unit_price, product_id
) values ($1, $2, $3, $4, $5, $6, $7)
returning id;

-- name: ListPendingIncomingOrders :many
-- Lo que pinta el aviso de la tableta. Ordenado por urgencia: el más cerca de expirar primero, que
-- es el único orden que le sirve a quien tiene que decidir.
select o.id, o.connection_id, p.name as platform_name, o.external_order_id, o.display_id,
       o.placed_at, o.decide_before, o.service_type, o.customer_name, o.total
  from platform_incoming_orders o
  join platform_connections c on c.id = o.connection_id
  join delivery_platforms p on p.id = c.delivery_platform_id
 where o.state = 'pendiente'
 order by o.decide_before;

-- name: ListLinesOfIncomingOrders :many
-- Los renglones de varios pedidos de un golpe. Uno por pedido sería N+1 sobre la consulta que la
-- tableta repite cada pocos segundos.
select id, incoming_order_id, parent_line_id, external_item_id, external_name,
       quantity, unit_price, product_id
  from platform_incoming_order_lines
 where incoming_order_id = any(sqlc.arg(ids)::bigint[])
 order by incoming_order_id, parent_line_id nulls first, id;

-- name: GetIncomingOrder :one
select o.id, o.connection_id, p.name as platform_name, o.external_order_id, o.display_id,
       o.placed_at, o.decide_before, o.state, o.service_type, o.customer_name, o.total,
       o.order_id, o.decided_by, o.settled_at, o.deny_reason
  from platform_incoming_orders o
  join platform_connections c on c.id = o.connection_id
  join delivery_platforms p on p.id = c.delivery_platform_id
 where o.id = $1;

-- name: GetIncomingOrderByExternalID :one
select id, connection_id, state, order_id
  from platform_incoming_orders
 where connection_id = $1 and external_order_id = $2;

-- name: AcceptIncomingOrder :execrows
-- `where state = 'pendiente'` DENTRO del update, no en un select previo: dos tabletas pueden tocar
-- Aceptar al mismo tiempo, y quien no gane tiene que enterarse. Con :execrows, cero filas significa
-- «ya estaba decidido» y el handler responde 409 en vez de fingir que hizo algo.
update platform_incoming_orders
   set state = 'aceptado', settled_at = now(), decided_by = $2, order_id = $3
 where id = $1 and state = 'pendiente';

-- name: DenyIncomingOrder :execrows
update platform_incoming_orders
   set state = 'rechazado', settled_at = now(), decided_by = $2, deny_reason = $3
 where id = $1 and state = 'pendiente';

-- name: ListOrphanPlatformOrders :many
-- Los pedidos de plataforma que se aceptaron SIN turno abierto y que le tocan al turno que se abre.
--
-- Solo los del MISMO día de operación: un pedido de hace tres días aparecería como ingreso de hoy.
--
-- SE LISTAN PARA RENUMERARLOS, no para un update en bloque, y ésa es la parte que no era obvia:
-- `orders_folio_turno_key` es único por (company_id, register_session_id, daily_number). Mientras
-- el pedido no tiene turno, su `daily_number` es 0 y no choca con nada porque Postgres trata los
-- NULL de `register_session_id` como distintos entre sí. En el momento en que se le asigna el
-- turno, ese 0 entra a competir con los folios reales de ese turno — y si hay dos pedidos
-- huérfanos, chocan entre ellos. Por eso cada uno toma su número del contador del turno.
select id
  from orders
 where register_session_id is null
   and delivery_platform_id is not null
   and business_date = $1
 order by opened_at;

-- name: ClaimPlatformOrder :exec
-- Le da turno y folio real a UN pedido huérfano. Va dentro de la transacción que abre el turno.
update orders
   set register_session_id = $2, daily_number = $3
 where id = $1 and register_session_id is null;

-- name: GetPlatformPaymentMethod :one
-- El método de pago de una plataforma, en línea o en efectivo.
--
-- Existe uno por plataforma y por forma de cobro («Uber Eats en línea», «Uber Eats efectivo»)
-- desde antes de esta feature, porque la captura manual ya los usaba. Aquí solo se elige el que
-- corresponde: un pedido que el cliente pagó en la app NO es lo mismo que uno contra entrega, y
-- meterlos en el mismo cajón hace que el corte pida efectivo que nadie recibió.
select id from payment_methods
 where delivery_platform_id = $1
   and is_active
   and (name ilike '%efectivo%') = sqlc.arg(en_efectivo)::boolean
 limit 1;

-- name: CreatePlatformOrderPayment :exec
-- El pago de un pedido de plataforma. `register_session_id` puede ir NULL: la cocina no espera a
-- que alguien abra caja, y el pedido se enlaza al turno que se abra después.
insert into order_payments (order_id, payment_method_id, amount, register_session_id, received_by,
                            reference, client_uuid)
values ($1, $2, $3, $4, $5, $6, $7);

-- name: SeedPlatformPaymentMethods :exec
-- Los dos métodos de cobro de una plataforma, al CONECTAR la tienda.
--
-- `SeedBasePaymentMethods` los deja fuera a propósito y con razón: «vender por Uber exige que ese
-- negocio haya hecho su propia vinculación con la plataforma, y darle formas de cobro que no tiene
-- contratadas es peor que no darle ninguna». Conectar la tienda ES esa vinculación — es el momento
-- exacto que ese comentario nombra.
--
-- Sin esto, aceptar el primer pedido falla con «falta el método de pago», y el operador no tiene
-- desde dónde arreglarlo: los métodos de plataforma no se crean desde ninguna pantalla.
--
-- Dos y no uno: un pedido pagado en la aplicación NO es lo mismo que uno contra entrega, y meterlos
-- en el mismo cajón hace que el corte pida efectivo que nadie recibió. `is_cash` y
-- `affects_cash_drawer` solo en el de efectivo, que es lo que el check de la 0067 exige.
insert into payment_methods (company_id, name, kind, delivery_platform_id, is_cash,
                             affects_cash_drawer, is_active, sort_key, auto_declare)
values
  ($1, sqlc.arg(nombre_en_linea)::text,  'plataforma', $2, false, false, true, 400, false),
  ($1, sqlc.arg(nombre_efectivo)::text, 'plataforma', $2, true,  true,  true, 410, false)
on conflict (company_id, name) do nothing;
