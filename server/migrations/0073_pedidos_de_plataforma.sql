-- +goose Up

-- RECIBIR LOS PEDIDOS DE LAS PLATAFORMAS (spec 021).
--
-- Cuatro tablas nuevas, dos índices que faltaban en tablas viejas, y un `check` de la 0007 que hay
-- que relajar. Lo que NO hay aquí: ninguna columna nueva en `orders`. El pedido que llega vive
-- aparte hasta que alguien lo acepta, y ahí se convierte en un `orders` normal.
--
-- POR QUÉ VIVE APARTE. `orders` es la tabla del dinero. Un pedido que aún no se acepta aparecería
-- en el tablero de cocina, en la deuda por cobrar y en el corte de caja antes de ser una venta —
-- principio III: lo que no es ingreso no entra al total. Y hay una razón mecánica: `orders.opened_by`
-- es not null, y un pedido que llega solo no tiene quién lo abrió. Creando la fila al ACEPTAR, quien
-- acepta ES `opened_by` y la columna sigue diciendo la verdad, sin inventar un usuario de sistema.

set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 0. DOS ÍNDICES QUE FALTAN EN TABLAS QUE YA EXISTEN
--
-- Sin esto, ninguna FK compuesta de abajo se puede ni declarar: Postgres exige un único que case
-- exactamente con las columnas referenciadas, y el `alter table` truena en seco.
--
-- Verificado contra Postgres real antes de escribir esto: `users` solo tenía `users_pkey`,
-- `users_company_username_key` y `users_pin_lookup_unico`; `platform_connections` solo su `pkey` y
-- `platform_connections_tienda`. Mismo patrón y misma razón que `products_tenant_key` (0071) y
-- `register_sessions_tenant_key` (0061): los chequeos de integridad referencial de Postgres SALTAN
-- RLS por diseño, así que una FK simple no impide apuntar a la fila de otra empresa.
--
-- Nota que NO es de esta feature: `platform_menu_reads` y `platform_item_links` de la 0071 tienen
-- el mismo hueco con `connection_id`. Allá es lectura de menú; aquí hay dinero, porque aceptar crea
-- una fila en `orders`. Se deja escrito, no se arregla aquí.
-- ---------------------------------------------------------------------------------------------
create unique index users_tenant_key on users (company_id, id);
create unique index platform_connections_tenant_key on platform_connections (company_id, id);

-- ---------------------------------------------------------------------------------------------
-- 1. CON QUÉ SE VERIFICA LA FIRMA
--
-- POR QUÉ EN LA BASE Y NO EN EL ENTORNO, que es la decisión más discutida de esta migración: la URL
-- del webhook es UNA SOLA para todas las empresas. Con la llave en `UBER_EATS_*`, el sistema solo
-- puede verificar los avisos de una empresa, y los de la segunda llegan a la misma puerta sin forma
-- de validarlos ni de saber de quién son. No es una columna que se agregue después: es la feature
-- que no funciona para el segundo cliente. Principio VIII — esto no se puede agregar al mismo costo.
--
-- POR QUÉ POR EMPRESA Y NO POR TIENDA: la llave la da el tablero de la plataforma por APLICACIÓN, y
-- una aplicación es de una empresa, que puede tener varias sucursales bajo la misma app. Ponerla en
-- `platform_connections` la duplicaría por sucursal y las dejaría divergir.
--
-- SIN CIFRAR, a propósito. Es el único secreto RECUPERABLE de la base —no se puede hashear: hay que
-- recalcular el HMAC con él—. Cifrarlo con una llave del entorno MUEVE el secreto, no lo protege:
-- quien lee la base desde la aplicación tiene también el entorno. Añadiría un camino de rotación y
-- un modo de fallo nuevos —«la llave de cifrado cambió y ningún pedido entra»— sin cerrar ningún
-- ataque real. El día que exista un gestor de secretos de verdad, se mueve ahí.
-- ---------------------------------------------------------------------------------------------
create table platform_webhook_keys (
  id                   bigint generated always as identity primary key,
  -- FK COMPUESTA, abajo. No `references delivery_platforms(id)` a secas, por lo del §0.
  delivery_platform_id smallint not null,
  key_primary          text not null,
  -- La secundaria existe porque el tablero de la plataforma la ofrece, y sirve para cambiar la
  -- llave sin dejar de recibir pedidos: durante el cambio las dos validan.
  key_secondary        text,
  -- Delata una rotación que nadie terminó. Sin esto, una llave secundaria olvidada se queda válida
  -- para siempre y nadie se entera.
  rotated_at           timestamptz,
  created_at           timestamptz not null default now(),
  company_id           bigint not null default current_setting('app.company_id', true)::bigint
                       references companies(id) on delete cascade,

  constraint platform_webhook_keys_una_por_app unique (company_id, delivery_platform_id),
  -- Una llave corta es un dedazo de captura, y un dedazo AQUÍ significa «ningún pedido entra» sin
  -- que nada falle: los avisos llegan, no validan, y se rechazan en silencio.
  constraint platform_webhook_keys_primaria_seria check (char_length(key_primary) between 16 and 512),
  constraint platform_webhook_keys_secundaria_seria
    check (key_secondary is null or char_length(key_secondary) between 16 and 512),
  -- Dos llaves iguales no son una rotación: son un descuido que hace creer que hay respaldo.
  constraint platform_webhook_keys_distintas check (key_secondary is null or key_secondary <> key_primary),

  -- Sin `on delete`: borrar una plataforma del catálogo no debe llevarse en silencio una llave que
  -- alguien capturó a mano del tablero de la plataforma. Mismo criterio que la 0071 y la 0037.
  constraint platform_webhook_keys_plataforma_de_la_empresa
    foreign key (delivery_platform_id, company_id) references delivery_platforms (id, company_id)
);

-- ---------------------------------------------------------------------------------------------
-- 2. QUÉ AVISO LLEGÓ
--
-- Uno por entrega de la plataforma. Es lo que permite deduplicar y lo que permite reconstruir qué
-- pasó cuando un pedido salga mal.
--
-- UN AVISO DE UNA TIENDA QUE NO RECONOCEMOS NO LLEGA AQUÍ. `company_id` es not null y su default
-- sale de `app.company_id`, que en ese caso no existe: la empresa ES el resultado de reconocer la
-- tienda. Ese aviso se rechaza ANTES de tocar Postgres y deja solo su evento de seguridad. No es
-- visible desde la pantalla de administración, y está bien: esa pantalla es de los pedidos del
-- negocio, y ese aviso no es de ningún negocio nuestro.
-- ---------------------------------------------------------------------------------------------
create table platform_webhook_events (
  id            bigint generated always as identity primary key,
  event_id      text not null,
  event_type    text not null,
  connection_id bigint not null,
  received_at   timestamptz not null default now(),
  processed_at  timestamptz,
  outcome       text,
  -- LA CLASE DEL FALLO, JAMÁS EL MENSAJE CRUDO. El error de una API ajena puede traer la dirección
  -- del cliente adentro, y DiDi transmite su `app_secret` EN EL QUERY STRING: guardar `err.Error()`
  -- aquí escribiría un secreto en la base por un camino que nadie está mirando. Mismo criterio que
  -- `platform_menu_reads.failure_kind` de la 0071.
  failure_kind  text,
  -- COMO `text` Y NO COMO `jsonb`, y esto importa: la firma se verifica sobre el cuerpo CRUDO.
  -- `jsonb` normaliza y reordena llaves, así que un cuerpo guardado como jsonb YA NO SE PUEDE
  -- REVERIFICAR. A decenas de pedidos al día, poder consultarlo con operadores de JSON no compensa
  -- perder la única prueba de autenticidad que tenemos. Si hace falta consultarlo, `raw_body::jsonb`
  -- funciona en la consulta.
  --
  -- TRAE DATOS DEL CLIENTE. Se poda a los 60 días: se borra el cuerpo y la fila se queda. El número
  -- no es arbitrario — el documento de pago de la plataforma llega mensual y una discrepancia se
  -- descubre al conciliarlo; pasado ese ciclo el cuerpo ya no sirve para nada y solo carga riesgo.
  raw_body      text,
  company_id    bigint not null default current_setting('app.company_id', true)::bigint
                references companies(id) on delete cascade,

  -- POR EMPRESA, Y LA VERSIÓN GLOBAL ERA UN DEFECTO.
  --
  -- Nació global con el argumento de que el identificador lo genera la plataforma y es único en su
  -- universo. Bajo RLS ese argumento se derrumba: la fila de OTRA empresa es INVISIBLE, así que el
  -- `on conflict do nothing` no devuelve ninguna fila, el servicio lee ese `ErrNoRows` como «ya lo
  -- procesamos» y contesta 200.
  --
  -- La plataforma entonces NO reintenta y el pedido se pierde SIN UNA SOLA LÍNEA DE ERROR. Es el
  -- peor modo de fallo que tiene este sistema: el cliente espera comida que nadie prepara y el log
  -- está limpio. Lo reproduce `TestUnAvisoRepetidoDeOtraEmpresaNoSeTraga`.
  --
  -- Lo que el único global pretendía cerrar —procesar dos veces el mismo aviso si el resolutor se
  -- equivoca de empresa— lo cierra ahora el rechazo por ambigüedad del servicio, que es donde
  -- corresponde: un único no puede arbitrar entre filas que no ve.
  constraint platform_webhook_events_una_vez unique (company_id, event_id),
  constraint platform_webhook_events_id_acotado check (char_length(event_id) between 1 and 200),
  constraint platform_webhook_events_tipo_acotado check (char_length(event_type) between 1 and 100),
  -- Las listas van como `check` y no solo en Go porque de estas columnas depende una garantía de
  -- seguridad, y una garantía que solo vive en el código se rompe al agregar una rama nueva.
  constraint platform_webhook_events_desenlace
    check (outcome is null or outcome in ('procesado','repetido','ignorado','fallido')),
  constraint platform_webhook_events_clase_de_fallo
    -- MISMO VOCABULARIO que el de `platform_menu_reads` (0071), más las dos propias de un pedido.
    -- El espejo lo vigila `domain.ClasesDeFalloDePedido()`: una lista que solo vive en Go se
    -- desincroniza del `check` y el primer valor nuevo revienta un insert en producción.
    check (failure_kind is null or failure_kind in
           ('sin_credenciales','auth_rechazada','tiempo_agotado','respuesta_invalida',
            'detalle_ilegible','mapeo_imposible')),
  constraint platform_webhook_events_coherente check ((outcome is null) = (processed_at is null)),

  constraint platform_webhook_events_conexion_de_la_empresa
    foreign key (connection_id, company_id) references platform_connections (id, company_id)
);

-- ---------------------------------------------------------------------------------------------
-- 3. EL PEDIDO QUE ESPERA DECISIÓN
-- ---------------------------------------------------------------------------------------------
create type platform_order_state as enum ('pendiente','aceptado','rechazado','cancelado','expirado');

create table platform_incoming_orders (
  id                bigint generated always as identity primary key,
  -- Sin `on delete cascade`: desconectar una tienda no debe llevarse el rastro de sus pedidos, que
  -- es lo único que queda para responder «¿sí llegó ese pedido?» meses después.
  connection_id     bigint not null,
  external_order_id text not null,
  -- El folio corto que ve el cliente, si la plataforma lo da. Es lo que el cliente dice por
  -- teléfono, y sin él nadie encuentra su pedido.
  display_id        text,

  -- LAS TRES NULLABLE, y no es descuido. Los avisos llegan SIN ORDEN GARANTIZADO: una cancelación
  -- puede ser el PRIMER aviso que vemos de un pedido, y entonces nunca hubo un GET al detalle — no
  -- hay con qué llenarlas. El `check` de abajo lo acota a ese único caso.
  placed_at         timestamptz,
  decide_before     timestamptz,
  total             numeric(10,2),

  state             platform_order_state not null default 'pendiente',
  -- CUÁNDO SALIÓ DE PENDIENTE. Aplica a los cuatro estados terminales.
  settled_at        timestamptz,
  -- QUIÉN DECIDIÓ. Solo en `aceptado` y `rechazado`: `cancelado` lo hace la plataforma y `expirado`
  -- lo nota el sistema, y no hay humano detrás de ninguno de los dos. Un solo check que atara las
  -- tres cosas obligaría a inventar un usuario de sistema para esos estados — el mismo parche que
  -- ya se rechazó con `opened_by`.
  decided_by        bigint,
  deny_reason       text,
  order_id          bigint,
  service_type      service_type not null,
  -- Solo lo que hace falta para preparar y entregar.
  customer_name     text,
  -- EL DETALLE CRUDO, byte por byte, como lo devolvió la plataforma. Es lo ÚNICO desde lo cual se
  -- puede corregir un mapeo equivocado días después, cuando la plataforma ya no entregue ese
  -- pedido. El aviso (`raw_body` de arriba) son cuatro campos y una liga: no sirve para esto.
  -- Misma poda de 60 días y por la misma razón.
  raw_detail        text,
  created_at        timestamptz not null default now(),
  company_id        bigint not null default current_setting('app.company_id', true)::bigint
                    references companies(id) on delete cascade,

  -- El mismo folio en dos plataformas es legítimo; repetido en la misma tienda, no.
  constraint platform_incoming_orders_folio unique (company_id, connection_id, external_order_id),
  constraint platform_incoming_orders_folio_acotado
    check (char_length(external_order_id) between 1 and 200),

  -- LA MATRIZ ESTADO → COLUMNAS, que es la razón de ser de los cuatro checks siguientes:
  --
  --   estado      settled_at   decided_by   deny_reason   order_id
  --   pendiente   NULL         NULL         NULL          NULL
  --   aceptado    con valor    con valor    NULL          con valor
  --   rechazado   con valor    con valor    con valor     NULL
  --   cancelado   con valor    NULL         NULL          puede tener
  --   expirado    con valor    NULL         NULL          NULL
  constraint platform_incoming_orders_pendiente_limpio
    check (state <> 'pendiente' or (settled_at is null and decided_by is null
                                    and deny_reason is null and order_id is null)),
  constraint platform_incoming_orders_terminal_con_hora
    check (state = 'pendiente' or settled_at is not null),
  constraint platform_incoming_orders_quien_decidio
    check ((decided_by is not null) = (state in ('aceptado','rechazado'))),
  constraint platform_incoming_orders_motivo_solo_al_rechazar
    check ((deny_reason is not null) = (state = 'rechazado')),
  constraint platform_incoming_orders_pedido_solo_si_acepto
    check (order_id is null or state in ('aceptado','cancelado')),

  -- Un pedido que NACIÓ cancelado no tiene nada que decidir ni nada que cobrar: lo único que
  -- importa es que quede registrado que existió y que la plataforma lo canceló.
  constraint platform_incoming_orders_datos_del_pedido
    check (state = 'cancelado'
           or (placed_at is not null and decide_before is not null and total is not null)),

  constraint platform_incoming_orders_conexion_de_la_empresa
    foreign key (connection_id, company_id) references platform_connections (id, company_id),
  constraint platform_incoming_orders_quien_decide_es_de_la_empresa
    foreign key (decided_by, company_id) references users (id, company_id),
  constraint platform_incoming_orders_pedido_de_la_empresa
    foreign key (order_id, company_id) references orders (id, company_id)
);

-- Lo único que la pantalla consulta cada pocos segundos. Empieza por `company_id` porque RLS agrega
-- ese predicado a toda consulta del rol de la aplicación, y un índice que arranque por la fecha se
-- queda descartando filas de otras empresas dentro del scan (ver 0042).
create index platform_incoming_orders_pendientes
  on platform_incoming_orders (company_id, decide_before) where state = 'pendiente';
-- Para la pantalla de historia. Hoy no urge por volumen; agregarlo ahora es gratis y después se
-- olvida.
create index platform_incoming_orders_historia
  on platform_incoming_orders (company_id, placed_at desc);

-- ---------------------------------------------------------------------------------------------
-- 4. QUÉ SE PIDIÓ
-- ---------------------------------------------------------------------------------------------
create table platform_incoming_order_lines (
  id                bigint generated always as identity primary key,
  incoming_order_id bigint not null references platform_incoming_orders(id) on delete cascade,
  -- Una OPCIÓN es un renglón que apunta a otro renglón. Así entra el modificador sin una segunda
  -- tabla, que es también como lo modela la plataforma: allá un modificador ES un item.
  parent_line_id    bigint references platform_incoming_order_lines(id) on delete cascade,
  external_item_id  text not null,
  -- SNAPSHOT del nombre y del precio que mandó la plataforma. Editar el catálogo no reescribe el
  -- pasado, y es la forma barata de dejar abierta la puerta de costear después.
  external_name     text not null,
  -- MISMA PRECISIÓN QUE SU DESTINO (`order_lines.quantity` es numeric(8,2)). Un staging más preciso
  -- que la tabla final solo esconde dónde se pierde el decimal.
  quantity          numeric(8,2) not null,
  unit_price        numeric(10,2) not null,
  -- NULL = SIN EMPAREJAR, y eso es la feature, no un hueco: un platillo sin pareja no impide
  -- aceptar. El cliente ya pagó, y rechazarlo por un hueco de nuestra contabilidad interna no es
  -- una opción defendible. Lo que sí hace es quedar contado y visible.
  product_id        bigint,
  company_id        bigint not null default current_setting('app.company_id', true)::bigint
                    references companies(id) on delete cascade,

  constraint platform_incoming_order_lines_nombre_acotado
    check (char_length(external_name) between 1 and 300),
  constraint platform_incoming_order_lines_item_acotado
    check (char_length(external_item_id) between 1 and 200),
  constraint platform_incoming_order_lines_cantidad_positiva check (quantity > 0),
  constraint platform_incoming_order_lines_precio_no_negativo check (unit_price >= 0),

  -- `restrict` como el de `platform_item_links`: el reorg de datos del §6 de AGENTS.md sí borra
  -- productos, y llevarse el renglón de un pedido real sería perder una venta.
  constraint platform_incoming_order_lines_producto_de_la_empresa
    foreign key (company_id, product_id) references products (company_id, id) on delete restrict
);

create index platform_incoming_order_lines_por_pedido
  on platform_incoming_order_lines (incoming_order_id);

-- ---------------------------------------------------------------------------------------------
-- 5. UN PEDIDO DE PLATAFORMA PUEDE SER PARA RECOGER
--
-- La 0007 trae `check (service_type = 'domicilio' or delivery_platform_id is null)`. Mientras la
-- captura era manual nadie lo notaba: quien capturaba elegía «domicilio». Con los pedidos entrando
-- solos el tipo lo dice la plataforma, y el primer pedido para recoger revienta la inserción.
--
-- Y es peor: recoger en tienda es el camino más barato para probar la integración de punta a punta,
-- porque no entra repartidor.
--
-- NO se quita el check: que un pedido de plataforma sea DE MOSTRADOR seguiría sin tener sentido.
--
-- El `drop`/`add` toma ACCESS EXCLUSIVE sobre `orders`, la tabla más transaccionada del sistema —
-- de ahí el `lock_timeout` de arriba. La validación es barata porque el cambio es un ensanchamiento
-- monótono: toda fila que pasaba el check viejo pasa el nuevo.
-- ---------------------------------------------------------------------------------------------
-- SE VERIFICA ANTES DE BORRAR, y eso no es paranoia. `orders_check` es un nombre que Postgres
-- AUTOGENERÓ: la 0007 declaró dos checks sin nombre y quedaron como `orders_check` y
-- `orders_check1`. Si algún día alguien agrega un check anónimo a `orders` en una migración
-- anterior, la numeración se recorre y este `drop` se llevaría la guarda de CANCELACIÓN —la que
-- exige que una orden cancelada tenga hora, responsable y motivo— sin que nada falle aquí. El
-- defecto aparecería meses después, en una orden cancelada sin rastro de quién la canceló.
-- +goose StatementBegin
do $$
declare def text;
begin
  select pg_get_constraintdef(oid) into def
    from pg_constraint
   where conrelid = 'orders'::regclass and conname = 'orders_check';
  if def is null then
    raise exception 'no existe orders_check en orders: el esquema cambió y hay que revisar esta migración a mano';
  end if;
  if def not like '%service_type%' or def not like '%delivery_platform_id%' then
    raise exception 'orders_check ya no es el check de servicio/plataforma sino %. Borrarlo se llevaría otra garantía', def;
  end if;
end $$;
-- +goose StatementEnd

alter table orders drop constraint orders_check;
alter table orders add constraint orders_servicio_de_plataforma
  check (service_type in ('domicilio','para_llevar') or delivery_platform_id is null);

-- ---------------------------------------------------------------------------------------------
-- 6. UN RENGLÓN PUEDE NO TENER PRODUCTO DEL CATÁLOGO
--
-- `order_lines.product_id` era not null, y eso impide lo que FR-013 exige: que un platillo sin
-- pareja NO impida aceptar. El cliente ya pagó; rechazar su pedido por un hueco de nuestra
-- contabilidad interna no es una opción defendible, y tampoco lo es tirar el renglón, porque
-- entonces la cocina no prepara ese platillo.
--
-- Es un ensanchamiento: toda fila que existe hoy tiene producto y lo sigue teniendo. Lo que cambia
-- son DOS consultas que la leen, y las dos fallaban EN SILENCIO con un NULL:
--
--   - `OrderHasPrepPending` unía a `products` con un join interno, así que un pedido cuyo único
--     renglón no tuviera pareja se habría considerado «sin nada que preparar» y se cerraría solo.
--     La cocina nunca lo vería. Ahora es left join con `coalesce(needs_prep, true)`: sin producto
--     se asume que SÍ hay que prepararlo, que es lo cierto para un platillo de plataforma.
--   - `PopularProducts` agrupaba por `product_id` y habría creado un balde NULL en la pestaña
--     «Top» del POS. Ahora los excluye.
-- ---------------------------------------------------------------------------------------------
alter table order_lines alter column product_id drop not null;

-- ---------------------------------------------------------------------------------------------
-- RLS y grants. El grant NO se hereda: el de 0024 fue puntual y sin default privileges, así que una
-- tabla nueva sin su grant responde 42501 en el primer request de producción y nunca falla en
-- desarrollo, donde la API se conecta como owner.
--
-- `gatobobah_platform` NO recibe nada: esto es la operación del restaurante, no la consola de quien
-- vende el sistema.
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
do $$
declare t text;
begin
  foreach t in array array['platform_webhook_keys','platform_webhook_events',
                           'platform_incoming_orders','platform_incoming_order_lines']
  loop
    execute format('alter table %I enable row level security', t);
    execute format($f$create policy tenant_isolation on %I
      using      (company_id = current_setting('app.company_id', true)::bigint)
      with check (company_id = current_setting('app.company_id', true)::bigint)$f$, t);
    execute format('grant select, insert, update, delete on %I to gatobobah_app', t);
  end loop;
end $$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------------------------
-- 7. CÓMO SE RESUELVE UNA TIENDA SIN SABER DE QUÉ EMPRESA ES
--
-- El webhook no tiene empresa: quien llama es la plataforma. La empresa es el RESULTADO de
-- autenticar el cuerpo, no su entrada — no hay nada que fijarle a `app.company_id` antes.
--
-- Y bajo el rol de la aplicación, una consulta sin tenant devuelve CERO filas: RLS funciona. O sea
-- que el webhook, tal como estaba, no habría resuelto NINGUNA tienda en producción, y el log solo
-- habría dicho «firma no autenticada». Nadie sospecharía de RLS. Lo deja en rojo
-- `TestElWebhookResuelveLaTiendaBajoElRolDeLaAplicacion`.
--
-- LA SALIDA: una vista cuyo DUEÑO es un rol propio con una política que sí ve todas las filas. Las
-- vistas se ejecutan con los privilegios de su dueño, así que la excepción queda acotada a ESA
-- proyección y se lee en `\d`, no en veinte renglones de comentario defendiendo un `store.Q`.
--
-- POR QUÉ UNA VISTA Y NO UNA FUNCIÓN `security definer`:
--   - Una función que devuelve conjunto no la tipa sqlc, así que habría que bajar a pgx a mano y
--     eso rompe el principio I. Una vista se tipa como cualquier relación.
--   - Una `security definer` cuyo dueño sea el owner de las tablas corre como superusuario, y su
--     único muro es el `search_path`. Este rol no puede entrar ni saltar RLS.
--
-- POR QUÉ LA VISTA NO TRAE NINGUNA LLAVE. Es la diferencia que decide. Un `select` sin filtro sobre
-- ella revela, como mucho, qué id de tienda es de qué empresa. Con las llaves adentro revelaría las
-- llaves de firma de TODAS las empresas, y entonces cualquiera que alcanzara la vista podría firmar
-- avisos a nombre de quien quisiera. El servicio lee la llave de cada candidata DESPUÉS, ya con el
-- tenant fijado y bajo RLS.
--
-- El rol es de CLÚSTER, no de base: por eso se crea con el patrón idempotente de 0024, y el Down
-- tolera que otra base del mismo clúster ya no lo use.
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
do $$
begin
  if not exists (select 1 from pg_roles where rolname = 'gatobobah_webhook') then
    create role gatobobah_webhook nologin;
  end if;
end $$;
-- +goose StatementEnd

-- El rol NO puede entrar ni saltar RLS: solo existe para ser dueño de la vista.
alter role gatobobah_webhook nologin nobypassrls;

-- EL GRANT Y LA POLÍTICA SON DOS COSAS, y hacen falta las dos. La política decide QUÉ FILAS ve un
-- rol; el grant decide si puede leer la tabla siquiera. Con la política sola, la vista falla con
-- `42501 permission denied` — que es lo que pasó al escribir esto, y lo atraparon las pruebas del
-- webhook. Es la misma lección que el grant puntual de la 0024: un privilegio que no se hereda.
--
-- Solo `select`, y solo sobre las dos tablas que la vista une.
grant select on platform_connections to gatobobah_webhook;
grant select on delivery_platforms to gatobobah_webhook;

-- Las dos políticas que lo dejan ver todo, acotadas A ESE ROL. Es la misma gramática que ya usan
-- 0068 para la consola y 0069/0070 para uso y toques: una política permisiva por rol, legible en
-- el catálogo y revocable en una línea.
create policy webhook_resuelve_tienda on platform_connections
  for select to gatobobah_webhook using (true);
create policy webhook_resuelve_plataforma on delivery_platforms
  for select to gatobobah_webhook using (true);

create view candidatas_del_aviso as
  select c.id            as connection_id,
         c.company_id    as company_id,
         c.external_store_id,
         c.is_active,
         p.name          as platform_name
    from platform_connections c
    join delivery_platforms p
      on p.id = c.delivery_platform_id and p.company_id = c.company_id;

alter view candidatas_del_aviso owner to gatobobah_webhook;
revoke all on candidatas_del_aviso from public;
grant select on candidatas_del_aviso to gatobobah_app;

-- +goose Down

-- EL DOWN SE DETIENE SIN HABER TOCADO NADA si ya existe un pedido de plataforma PARA RECOGER.
-- Regresar el check viejo con una fila así lo violaría, y a media reversión es el peor momento para
-- descubrirlo. Mismo criterio que el Down de la 0061.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from orders
              where delivery_platform_id is not null and service_type = 'para_llevar') then
    raise exception 'hay pedidos de plataforma para recoger: regresar el check de la 0007 los dejaría fuera. Revísalos antes de revertir';
  end if;
end $$;
-- +goose StatementEnd

set local lock_timeout = '3s';

alter table orders drop constraint orders_servicio_de_plataforma;
alter table orders add constraint orders_check
  check (service_type = 'domicilio' or delivery_platform_id is null);

-- El Down NO regresa el not null de order_lines.product_id: si ya entró un renglón sin pareja,
-- ponerlo de vuelta lo borraría o reventaría. Se avisa y se deja abierto.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from order_lines where product_id is null) then
    raise notice 'hay renglones sin producto del catálogo: order_lines.product_id se queda nullable';
  else
    alter table order_lines alter column product_id set not null;
  end if;
end $$;
-- +goose StatementEnd

-- La vista, sus políticas y su rol, en orden inverso. Una vista con dueño ajeno que sobreviva a un
-- rollback es un asa que salta RLS sin quien responda por ella.
drop view if exists candidatas_del_aviso;
drop policy if exists webhook_resuelve_plataforma on delivery_platforms;
drop policy if exists webhook_resuelve_tienda on platform_connections;
-- El rol NO se borra: es de clúster y otra base puede seguir usándolo, igual que gatobobah_app y
-- gatobobah_platform.

drop table if exists platform_incoming_order_lines;
drop table if exists platform_incoming_orders;
drop table if exists platform_webhook_events;
drop table if exists platform_webhook_keys;
drop type if exists platform_order_state;
drop index if exists platform_connections_tenant_key;
drop index if exists users_tenant_key;
