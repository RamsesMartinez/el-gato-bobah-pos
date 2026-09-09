-- +goose Up
-- El folio con el que llegó el pedido, y lo que la plataforma se quedó (spec 014).
--
-- Las dos piezas van en UNA migración porque comparten la restricción que las define: la
-- liquidación de un pedido no se puede conciliar sin su folio, y las dos cuelgan de la misma fila
-- de `orders`. Partirla dejaría una ventana en la que la segunda mitad no tiene con qué probarse.
--
-- LO IRRECUPERABLE, que es lo que justifica hacerlo hoy: el reporte de pagos de Rappi expone 3
-- meses de historia y el de Uber 31 días. Un folio que no se capture ahora no existe después, y sin
-- él un depósito solo se concilia por monto y fecha — que empata mal en cuanto hay dos pedidos del
-- mismo importe.

-- lock_timeout como en 0040, 0041 y 0042, y por el mismo motivo: `alter table` y `create index`
-- toman ACCESS EXCLUSIVE sobre `orders`, la tabla que toca cada venta. El riesgo no es el tamaño
-- —son ~123 filas— sino la COLA: si al desplegar hay una transacción larga abierta, esto se encola
-- detrás y arrastra toda lectura nueva. Falla limpio en 3 s antes que dejar el POS mudo.
--
-- Y el lock es de TODA la migración, no de cada instrucción: goose corre el archivo en una sola
-- transacción, así que se sostiene hasta el commit final. Con las filas de hoy son milisegundos; se
-- dice aquí para que no se re-derive el día que `orders` sea grande y alguien copie este patrón.
set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 1. El folio de la plataforma, en `orders`
-- ---------------------------------------------------------------------------------------------
-- TEXTO y no un entero: Uber nombra sus pedidos con un UUID de 36 caracteres, DiDi con un entero de
-- 19 dígitos y Rappi con uno de 10. Ningún tipo numérico los cubre a los tres, y guardar el código
-- corto de 5 caracteres que ve el personal sería irrecuperable: del corto no se reconstruye el UUID.
--
-- Nullable de forma PERMANENTE, no "nullable hoy, not null mañana": un pedido de mostrador —la
-- enorme mayoría— nunca lo va a tener, y los 123 que ya están tampoco.
--
-- El par `_set_by`/`_set_at` es el mismo trato que 0037 le dio a `product_platform_prices.updated_by`
-- ("es el rastro que justifica dejar que un cajero escriba precios"). Aquí el cajero escribe el
-- valor con el que después se concilia un depósito, y un folio mal tecleado se descubre semanas más
-- tarde, cuando ya nadie recuerda quién capturó. Sin `on delete`, como las otras 15 FK *_by del
-- esquema: los usuarios se desactivan, no se borran.
alter table orders
  add column platform_order_ref  text,
  add column platform_ref_set_by bigint references users(id),
  add column platform_ref_set_at timestamptz;

-- La cadena vacía NO es "sin folio". Guardarla haría que dos pedidos sin folio chocaran contra la
-- unicidad, o peor, que pasaran los dos y nadie notara que uno está vacío. El recorte de extremos
-- lo hace la frontera (domain.NormalizePlatformRef); esto es la red de abajo, para el camino que
-- no pasa por ahí — una migración de datos, un script suelto.
--
-- 64 es cota de cordura, no regla de negocio: el más largo conocido son los 36 del UUID de Uber, y
-- el tope existe contra un pegado accidental de media pantalla.
alter table orders add constraint orders_platform_ref_forma check (
  platform_order_ref is null
  or (platform_order_ref = btrim(platform_order_ref, E' \t\n\r')
      and length(platform_order_ref) between 1 and 64)
);

-- "Pedido de mostrador con folio de plataforma" queda IMPOSIBLE POR CONSTRUCCIÓN, y no como una
-- validación de pantalla que el siguiente camino se salta. Es el modo de falla que la constitución
-- llama "el camino nuevo que se salta el control viejo".
alter table orders add constraint orders_platform_ref_solo_plataforma check (
  platform_order_ref is null or delivery_platform_id is not null
);

-- Todo o nada, como el trío de cancelación que ya vive en esta tabla. Un camino futuro que escriba
-- el folio sin su rastro —o que borre uno de los tres al corregir— deja el "quién lo capturó" roto
-- sin que nada avise, y ese rastro existe justamente para poder confiar en él meses después.
alter table orders add constraint orders_platform_ref_rastro check (
  (platform_order_ref is null) = (platform_ref_set_by is null)
  and (platform_ref_set_by is null) = (platform_ref_set_at is null)
);

-- Unicidad PARCIAL, copiando la forma de 0057. El `where` es lo que impide que el índice cargue con
-- todo el histórico de mostrador sin servir para nada (dos NULL nunca chocan en un índice único).
--
-- Lleva la PLATAFORMA porque el mismo número puede existir en dos plataformas distintas, y
-- rechazarlo tiraría una captura legítima — que es lo caro de quitar después. Y lleva `company_id`
-- porque RLS agrega ese predicado a toda consulta del rol de app: un índice que no arranca por ahí
-- se queda descartando filas de otras empresas dentro del scan.
create unique index orders_platform_ref
  on orders (company_id, delivery_platform_id, platform_order_ref)
  where platform_order_ref is not null;

-- Soporte del filtro "pendientes de folio" de la pantalla de Ventas. Parcial sobre un subconjunto
-- chico, y SOLO se usa si la consulta lleva el predicado literal: medido contra 150k pedidos, con
-- el patrón `narg is null or (…)` y plan genérico —al que pgx cae solo, porque usa statements con
-- nombre— Postgres no puede probar que el predicado del índice se cumple y cae a un bitmap scan
-- sobre la fecha. Por eso sales.sql lleva dos variantes de cada consulta y no una parametrizada.
create index orders_plataforma_sin_folio
  on orders (company_id, business_date)
  where delivery_platform_id is not null and platform_order_ref is null;

-- Soporte de la BÚSQUEDA por folio. El índice único de arriba no sirve para esto: arranca por
-- (company_id, delivery_platform_id, …) y deja la plataforma en medio, pero quien pega un folio
-- sacado del documento de pago no la conoce. Este lleva el folio inmediatamente después de
-- company_id, que es lo que RLS ya fija.
create index orders_platform_ref_busqueda
  on orders (company_id, platform_order_ref)
  where platform_order_ref is not null;

-- Destino de la FK COMPUESTA de la liquidación. Va aquí porque el destino tiene que existir antes
-- que la llave; mismo patrón con el que 0037 preparó `delivery_platforms_id_company_key`.
alter table orders add constraint orders_id_company_key unique (id, company_id);

-- ---------------------------------------------------------------------------------------------
-- 2. La liquidación: lo que el documento de pago de la plataforma dice
-- ---------------------------------------------------------------------------------------------
-- TABLA APARTE Y NO COLUMNAS EN `orders`, por tres razones que no se pueden separar:
--
--   1. Se crea DESPUÉS de la venta y por otro camino (llega el documento días más tarde).
--   2. Su PRESENCIA es el estado del dinero. Con columnas nullables, "todavía no llega el
--      documento" y "el documento dice cero" se verían igual, que es justo lo que el spec prohíbe.
--   3. El pedido de mostrador —la enorme mayoría de las filas— cargaría ocho columnas nulas en la
--      tabla que toca cada venta.
--
-- Y TODO ES SNAPSHOT del documento, nunca calculado con un porcentaje configurado: las tasas
-- cambian y las promociones las alteran, así que recalcular el pasado con la tasa de hoy reescribe
-- la historia.
create table platform_settlements (
  -- La PK en `order_id` es lo que garantiza "a lo más una liquidación por pedido": lo hace el
  -- motor, y "recapturar reemplaza" se vuelve un `on conflict do update` — una sola instrucción,
  -- sin ventana de inconsistencia.
  order_id            bigint primary key,

  -- Lo que la plataforma REPORTA como venta. No se compara con orders.total aquí: cuadrarlos es la
  -- feature de conciliación, y rechazar la discrepancia impediría registrar justo lo que se quiere ver.
  reported_gross      numeric(10,2) not null,

  commission_amount   numeric(10,2) not null,
  -- NULLABLE a propósito: hay documentos que dan el monto sin declarar la tasa, y un 0 ahí
  -- afirmaría "la plataforma cobró 0%", que es medible y falso. Es el mismo principio de "ausencia
  -- se representa como ausencia" aplicado a una columna.
  commission_pct      numeric(5,2),

  -- El descuento se guarda como TOTAL + la parte que puso la PLATAFORMA. La parte del restaurante
  -- es la resta y se calcula en el dominio: guardar los tres serían dos verdades sobre el mismo
  -- hecho, y un documento corregido que mueva una y no la otra deja la fila contradiciéndose.
  -- Rappi parte el descuento en amount_by_rappi / amount_by_partner y CAMBIA POR CAMPAÑA; sin esta
  -- columna, cada promoción se registra como pérdida propia del restaurante.
  discount_total      numeric(10,2) not null default 0,
  discount_platform   numeric(10,2) not null default 0,

  withholdings        numeric(10,2) not null default 0,

  -- El neto PUEDE SER NEGATIVO y es la única columna de dinero del esquema sin check de signo. No
  -- es un descuido: con una promoción que financió el restaurante por completo, el neto negativo es
  -- lo que de verdad pasó, y rechazarlo obligaría a capturar una mentira.
  net_amount          numeric(10,2) not null,

  -- La referencia del depósito (payment_id de Rappi, Payout reference ID de Uber). Texto y no una
  -- entidad propia porque EXPIRA —Rappi conserva 3 meses, Uber 31 días—: modelar el depósito se
  -- agrega después al mismo costo, pero el dato hay que capturarlo hoy o no existe.
  payout_reference    text,
  document_ref        text,

  captured_by         bigint not null references users(id),
  captured_at         timestamptz not null default now(),
  updated_at          timestamptz not null default now(),
  company_id          bigint not null default current_setting('app.company_id', true)::bigint
                      references companies(id) on delete cascade,

  -- FK COMPUESTA y no `references orders(id)` a secas: los chequeos de integridad referencial de
  -- Postgres SALTAN RLS por diseño, así que una FK simple aceptaría sin protestar una liquidación
  -- cuyo company_id es de una empresa y cuyo order_id es de otra. La fila quedaría invisible en
  -- todo join bajo RLS y el error saldría en el resumen de dinero, no en el insert. Es el hueco que
  -- 0041 cerró para las tablas que agrupan dinero.
  --
  -- La excepción que 0037 aceptó en product_platform_prices ("ids globales, dos empresas nunca
  -- comparten uno") NO aplica: eso es catálogo y esto es dinero.
  --
  -- Sin `on delete`: no existe ningún `delete from orders` en el repo —un pedido se cancela, no se
  -- borra—, así que la cláusula nunca dispararía; lo único que lograría es sugerir que borrar un
  -- pedido es un camino soportado.
  constraint platform_settlements_order_fkey
    foreign key (order_id, company_id) references orders (id, company_id),

  constraint ps_montos_no_negativos check (
    reported_gross >= 0 and commission_amount >= 0
    and discount_total >= 0 and discount_platform >= 0 and withholdings >= 0
  ),
  constraint ps_tasa_en_rango check (
    commission_pct is null or (commission_pct >= 0 and commission_pct <= 100)
  ),
  -- La parte no puede ser mayor que el todo. Con esta forma es una comparación REAL; guardando las
  -- dos partes en vez del total sería imposible por construcción y el rechazo que pide el spec no
  -- existiría.
  constraint ps_descuento_de_la_plataforma_cabe check (discount_platform <= discount_total),
  -- Mismo motivo que el tope del folio: cota de cordura contra un pegado accidental. Y la cadena
  -- vacía se rechaza aquí también — una referencia de depósito "" no es una referencia.
  constraint ps_referencias_acotadas check (
    (payout_reference is null
      or (payout_reference = btrim(payout_reference) and length(payout_reference) between 1 and 128))
    and (document_ref is null
      or (document_ref = btrim(document_ref) and length(document_ref) between 1 and 256))
  )
);

-- El patrón de las tablas per-tenant desde 0023, y el que usa RLS al pegarle company_id a todo. El
-- orden por captured_at sirve al resumen del periodo, que es la única consulta que barre la tabla.
create index platform_settlements_company on platform_settlements (company_id, captured_at desc);

-- updated_at por trigger y no por la query: un upsert que olvide setearlo dejaría la auditoría
-- congelada en la fecha de la primera captura (patrón de 0009).
create trigger trg_platform_settlements_updated before update on platform_settlements
  for each row execute function set_updated_at();

alter table platform_settlements enable row level security;
create policy tenant_isolation on platform_settlements
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

-- EL GRANT NO ES OPCIONAL. El de 0024 fue `on all tables in schema public`, que es PUNTUAL: no hay
-- default privileges, así que cada tabla creada después necesita el suyo (0025, 0026, 0028, 0029,
-- 0030 y 0037 hacen lo mismo). Sin esto la migración pasa, los tests pasan y `make start` pasa
-- —dev sirve como owner, sin RLS ni grants— y en producción el primer request devuelve 42501.
grant select, insert, update, delete on platform_settlements to gatobobah_app;

-- ---------------------------------------------------------------------------------------------
-- LA PUERTA QUE QUEDA ABIERTA, DICHA POR SU NOMBRE
-- ---------------------------------------------------------------------------------------------
-- Cambiar la PLATAFORMA de un pedido que ya tiene folio dejaría basura silenciosa: un folio de Uber
-- colgando de Rappi. Hoy es imposible porque NINGÚN endpoint muta `delivery_platform_id` después de
-- crear el pedido, y por eso no hay check que lo impida (no se puede escribir uno que distinga un
-- cambio legítimo de uno equivocado). Quien agregue ese camino decide ahí qué pasa con el folio:
-- borrarlo con su rastro, o rechazar el cambio. No lo descubra después de haberlo escrito.

-- +goose Down
-- ATENCIÓN: después de la primera captura real esto deja de ser un rollback y pasa a ser PÉRDIDA DE
-- DATOS. El folio es irrecuperable —Rappi expone 3 meses de historia, Uber 31 días— y las
-- liquidaciones capturadas se van con la tabla. Vale para la ventana entre aplicar la migración y
-- capturar el primer folio, y no más.
drop table if exists platform_settlements;
-- Va DESPUÉS del drop de la tabla: mientras exista, su FK compuesta depende de este unique.
alter table orders drop constraint if exists orders_id_company_key;
drop index if exists orders_platform_ref_busqueda;
drop index if exists orders_plataforma_sin_folio;
drop index if exists orders_platform_ref;
alter table orders drop constraint if exists orders_platform_ref_rastro;
alter table orders drop constraint if exists orders_platform_ref_solo_plataforma;
alter table orders drop constraint if exists orders_platform_ref_forma;
alter table orders drop column if exists platform_ref_set_at,
                   drop column if exists platform_ref_set_by,
                   drop column if exists platform_order_ref;
