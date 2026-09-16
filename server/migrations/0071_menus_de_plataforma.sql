-- +goose Up

-- LEER EL MENÚ DE LAS PLATAFORMAS Y COMPARARLO CON EL NUESTRO (spec 020).
--
-- Cuatro tablas: la tienda conectada, la foto de su menú, lo que esa foto traía, y el
-- emparejamiento manual contra el catálogo. Ninguna toca una tabla existente.
--
-- LA DIRECCIÓN DE LA VERDAD, que decide varias columnas de aquí: **manda lo publicado arriba**.
-- El precio por plataforma del POS (`product_platform_prices`) no es el precio al que se vende;
-- es una copia que alguien captura DESPUÉS, a mano, para que el ticket cuadre con lo que la
-- plataforma ya cobró. Por eso esta feature guarda la foto tal como viene y no la interpreta.

set local lock_timeout = '3s';

create type platform_read_status as enum ('en_curso', 'ok', 'fallida');

-- Tres niveles porque el menú tiene tres, y los dos lados los modelan igual: el POS con
-- products / modifier_groups / modifier_options, y Uber con item / modifier_group_ids /
-- modifier_options (donde una opción, a su vez, ES un item).
--
-- `grupo` no se usa en esta feature y existe desde el día uno a propósito: agregarlo después
-- obligaría a decidir de qué clase eran las filas ya escritas, y eso solo se puede adivinar.
create type platform_item_kind as enum ('platillo', 'grupo', 'opcion');

-- POR QUÉ DOS ENUM Y DOS `text` CON `check`. Agregar un valor a un enum no se puede usar en la misma
-- transacción que lo agrega, así que un valor nuevo cuesta dos migraciones. `status` y `kind` son la
-- forma del dominio y no van a crecer; `failure_kind` y `local_kind` sí —una plataforma nueva trae
-- clases de fallo nuevas, y el inventario de insumos traerá `receta`— y un `check` se altera en una
-- sola migración. No es inconsistencia de estilo: es cuál de los dos se espera que cambie.

-- LLAVE DE TENANT PARA PODER REFERENCIAR PRODUCTOS SIN CRUZAR EMPRESAS.
--
-- Una FK simple a `products(id)` NO impide que la empresa A empareje un producto de la empresa B:
-- los chequeos de integridad referencial de Postgres **saltan RLS** por diseño, así que la política
-- de aislamiento no los ve. El emparejamiento quedaría apuntando a un catálogo ajeno y la
-- comparación reportaría diferencias contra productos que ese negocio no vende.
--
-- Lo atrapó `TestUnEmparejamientoNoCruzaEmpresas` antes de que existiera una sola fila. El patrón
-- de la FK compuesta ya estaba en el esquema: 0061 (register_sessions) y 0066 (conteos).
create unique index products_tenant_key on products (company_id, id);

-- ---------------------------------------------------------------------------------------------
-- 1. La tienda conectada
-- ---------------------------------------------------------------------------------------------
create table platform_connections (
  id                   bigint generated always as identity primary key,
  -- FK COMPUESTA con company_id, abajo. No `references delivery_platforms(id)` a secas: es el mismo
  -- hueco que este archivo cierra para `products` treinta líneas más arriba, y dejarlo aquí sería
  -- arreglarlo en una tabla y abrirlo en la de al lado.
  delivery_platform_id smallint not null,
  -- El id de la tienda DEL LADO DE ALLÁ, completo y sin normalizar. Uber entrega un UUID, DiDi un
  -- entero de 19 dígitos: texto, nunca un número.
  external_store_id    text not null,
  -- Cómo la llama quien administra. Nadie distingue dos sucursales por UUID.
  label                text not null,
  is_active            boolean not null default true,
  created_at           timestamptz not null default now(),
  company_id           bigint not null default current_setting('app.company_id', true)::bigint
                       references companies(id) on delete cascade,

  -- `external_store_id` DENTRO de la llave, y ahí está la puerta del principio VIII: una empresa
  -- va a tener varias sucursales y cada una es una tienda distinta arriba, con su propio menú.
  -- `unique (company_id, delivery_platform_id)` sería el «único por company_id que en realidad
  -- debería ser por sucursal» que la constitución nombra, y la segunda sucursal no entraría.
  --
  -- `branches` no existe todavía; cuando exista se agrega un `branch_id` nullable y ninguna fila
  -- hay que repartir, porque cada conexión ya sabe de qué tienda es.
  constraint platform_connections_tienda unique (company_id, delivery_platform_id, external_store_id),
  constraint platform_connections_store_acotado check (char_length(external_store_id) between 1 and 200),
  constraint platform_connections_label_acotado check (char_length(label) between 1 and 60),

  -- Sin `on delete`: borrar una plataforma del catálogo no debe llevarse en silencio la conexión ni
  -- el emparejamiento, que cuesta una sesión de trabajo manual. Mismo criterio que 0037.
  --
  -- COMPUESTA con company_id porque los chequeos de integridad saltan RLS: con una FK simple, una
  -- empresa puede apuntar al id de plataforma de OTRA —son smallint correlativos desde 1, se
  -- enumeran probando— y la fila entra. Después el `join` de la consulta sí filtra por RLS y no
  -- encuentra nada: la conexión existe, ocupa la llave única, y no aparece en la lista ni se puede
  -- borrar desde la pantalla. Peor: `catalogoLocal` usaría el markup de la otra empresa para
  -- calcular las diferencias de precio. El patrón ya estaba en 0037 y 0041.
  constraint platform_connections_plataforma_de_la_empresa
    foreign key (delivery_platform_id, company_id) references delivery_platforms (id, company_id)
);

-- ---------------------------------------------------------------------------------------------
-- 2. La foto del menú, con su resultado
-- ---------------------------------------------------------------------------------------------
create table platform_menu_reads (
  id            bigint generated always as identity primary key,
  connection_id bigint not null references platform_connections(id) on delete cascade,
  started_at    timestamptz not null default now(),
  finished_at   timestamptz,
  status        platform_read_status not null default 'en_curso',
  item_count    integer,
  -- LA CLASE DEL FALLO, JAMÁS EL MENSAJE CRUDO. El error de una API ajena puede traer la dirección
  -- completa adentro, y DiDi transmite su `app_secret` EN EL QUERY STRING: guardar `err.Error()`
  -- aquí escribiría un secreto en la base por un camino que nadie está mirando.
  --
  -- La lista va como `check` y no solo en el código de Go porque de esta columna depende una
  -- garantía de seguridad, y una garantía que vive en un comentario se rompe la primera vez que
  -- alguien agrega una rama nueva al servicio.
  failure_kind  text,
  company_id    bigint not null default current_setting('app.company_id', true)::bigint
                references companies(id) on delete cascade,

  constraint platform_menu_reads_coherente
    check ((status = 'en_curso') = (finished_at is null)),
  -- Una lectura que vuelve SIN PRODUCTOS no es un menú vacío: es una lectura que no sirve.
  -- Tratarla como dato válido haría que la comparación reportara que sobra el catálogo entero —
  -- el peor reporte posible, y el que más invita a una acción destructiva.
  constraint platform_menu_reads_ok_trae_items
    check (status <> 'ok' or coalesce(item_count, 0) > 0),
  constraint platform_menu_reads_clase_de_fallo
    check (failure_kind is null or failure_kind in (
      'sin_credenciales', 'auth_rechazada', 'tiempo_agotado',
      'respuesta_invalida', 'menu_vacio', 'menu_truncado'))
);

-- Empieza por company_id porque RLS agrega ese predicado a toda consulta del rol de app, y un
-- índice que arranca por otra columna se queda descartando filas ajenas dentro del scan.
create index platform_menu_reads_recientes
  on platform_menu_reads (company_id, connection_id, started_at desc);

-- UNA SOLA LECTURA EN CURSO POR TIENDA, impuesto por el esquema y no por un `if`.
--
-- El servicio consulta si ya hay una antes de abrir la suya, pero eso son dos statements: dos
-- `POST .../read` a la vez pasan los dos el chequeo y arrancan dos goroutines leyendo la misma
-- tienda. Gastan dos tokens para escribir la misma foto, y Uber invalida el más viejo a partir del
-- 101 en una hora.
--
-- Con este índice, el segundo `insert` falla con 23505 y el servicio lo traduce a «ya hay una
-- lectura en curso». El chequeo previo se queda: da el mensaje correcto sin pagar un error.
create unique index platform_menu_reads_una_en_curso
  on platform_menu_reads (connection_id) where status = 'en_curso';

-- ---------------------------------------------------------------------------------------------
-- 3. Lo que la plataforma publicaba en ese momento
-- ---------------------------------------------------------------------------------------------
create table platform_menu_items (
  read_id     bigint not null references platform_menu_reads(id) on delete cascade,
  -- Tal como lo entrega la plataforma. Los ids de Uber vienen truncados a 20 caracteres y traen
  -- acentos y emoji; recortarlos o normalizarlos rompe el emparejamiento en la siguiente lectura.
  external_id text not null,
  kind        platform_item_kind not null,
  name        text not null,
  -- CENTAVOS ENTEROS, como los entrega la plataforma. Convertir a pesos aquí obligaría a elegir un
  -- redondeo en la frontera de la base, que es el lugar donde menos se ve. La conversión ocurre una
  -- sola vez, en domain, con su prueba.
  price_cents bigint not null,
  available   boolean not null,
  company_id  bigint not null default current_setting('app.company_id', true)::bigint
              references companies(id) on delete cascade,

  -- La PK no lleva company_id y es correcto: `read_id` es identity global sobre una tabla
  -- per-tenant, así que dos empresas nunca comparten uno. Mismo razonamiento que
  -- product_platform_prices en 0037; prefijarlo costaría 8 bytes por fila sin ganar selectividad.
  primary key (read_id, external_id),
  constraint platform_menu_items_precio_no_negativo check (price_cents >= 0),
  constraint platform_menu_items_id_acotado check (char_length(external_id) between 1 and 200)
);

-- ---------------------------------------------------------------------------------------------
-- 4. El emparejamiento — el trabajo humano que no se reconstruye
-- ---------------------------------------------------------------------------------------------
create table platform_item_links (
  connection_id bigint not null references platform_connections(id) on delete cascade,
  -- NO HAY FK HACIA platform_menu_items, Y ES DELIBERADO. Si la hubiera, podar lecturas viejas se
  -- llevaría por delante el emparejamiento —una sesión completa de trabajo manual— en silencio y
  -- semanas después, con la pantalla reportando de pronto que el 100% del menú difiere.
  --
  -- El precio de no tenerla: nada en la base impide guardar una pareja contra un id que no existe
  -- arriba. Eso lo valida el servicio antes de aceptar el link.
  external_id   text not null,
  kind          platform_item_kind not null,
  -- FK COMPUESTA con company_id, no simple: ver `products_tenant_key` arriba. Una FK a products(id)
  -- a secas deja emparejar el catálogo de otra empresa, porque los chequeos de integridad saltan RLS.
  product_id    bigint not null,
  -- Qué es el lado de ACÁ. Nace con un solo valor y es el hueco que deja abierta la puerta de
  -- inventario de insumos: el día que existan recetas, un platillo de la plataforma podrá
  -- corresponder a una receta o a un insumo. Agregar la columna después obliga a decidir qué eran
  -- las filas viejas; ponerla hoy cuesta una palabra.
  local_kind    text not null default 'producto',
  -- Nulo = PROPUESTA, no hecho. Una pareja automática aceptada en silencio produce comparaciones
  -- falsas que nadie puede auditar, y medido: el nombre exacto acierta 6 de 65.
  confirmed_at  timestamptz,
  confirmed_by  bigint references users(id) on delete set null,
  created_at    timestamptz not null default now(),
  company_id    bigint not null default current_setting('app.company_id', true)::bigint
                references companies(id) on delete cascade,

  -- Un item de la plataforma apunta a lo sumo a UN producto (FR-012). Al revés sí: varias filas
  -- comparten product_id, que es el caso real — una `Chamoyada` abajo son doce arriba.
  primary key (connection_id, external_id),
  constraint platform_item_links_confirmador check (confirmed_by is null or confirmed_at is not null),
  constraint platform_item_links_local_kind check (local_kind in ('producto', 'opcion_de_modificador')),
  constraint platform_item_links_id_acotado check (char_length(external_id) between 1 and 200),

  -- `on delete restrict`, NO cascade: en este repo los productos SÍ se borran (el reorg de datos de
  -- AGENTS.md §6 corre `delete from products`), y con cascade un reorg se llevaría las parejas
  -- confirmadas sin avisar. El precedente es order_lines.product_id, que referencia sin `on delete`.
  constraint platform_item_links_producto_de_la_empresa
    foreign key (company_id, product_id) references products (company_id, id) on delete restrict
);

-- UN SOLO ÍNDICE, no dos. La primera versión agregó también uno liso por `product_id` creyendo que
-- el chequeo de `restrict` no podía usar el compuesto «porque los chequeos de integridad saltan
-- RLS». Eso es cierto y no venía al caso: saltar RLS cambia qué filas se ven, no qué columnas entran
-- al predicado — el chequeo busca por los valores literales `(company_id, id)` de la fila que se
-- borra, y el compuesto los cubre. Medido con `pg_stat_user_indexes` sobre Postgres real.
create index platform_item_links_por_producto on platform_item_links (company_id, product_id);

-- ---------------------------------------------------------------------------------------------
-- RLS y grants. El grant NO se hereda: el de 0024 fue puntual y sin default privileges, así que
-- una tabla nueva sin su grant responde 42501 en el primer request de producción y nunca falla en
-- desarrollo, donde la API se conecta como owner.
--
-- `gatobobah_platform` NO recibe nada: esto administra el catálogo del restaurante, no la consola
-- de quien vende el sistema.
-- ---------------------------------------------------------------------------------------------
-- +goose StatementBegin
do $$
declare t text;
begin
  foreach t in array array['platform_connections','platform_menu_reads','platform_menu_items','platform_item_links']
  loop
    execute format('alter table %I enable row level security', t);
    execute format($f$create policy tenant_isolation on %I
      using      (company_id = current_setting('app.company_id', true)::bigint)
      with check (company_id = current_setting('app.company_id', true)::bigint)$f$, t);
    execute format('grant select, insert, update, delete on %I to gatobobah_app', t);
  end loop;
end $$;
-- +goose StatementEnd

-- +goose Down

-- Revertir pierde el emparejamiento, que es trabajo manual y no se reconstruye. Nada del negocio
-- se pierde: esta feature no toca un pedido ni un peso.
drop table if exists platform_item_links;
drop table if exists platform_menu_items;
drop table if exists platform_menu_reads;
drop table if exists platform_connections;
drop type if exists platform_item_kind;
drop type if exists platform_read_status;
drop index if exists products_tenant_key;
