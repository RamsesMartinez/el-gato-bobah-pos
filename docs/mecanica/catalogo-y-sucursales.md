# Catálogo y sucursales

Notas de mecánica de esta área: lo que no se ve en el código ni en la pantalla y ya costó un defecto. Se leen **antes** de tocar sucursales, versión del menú y cambios de catálogo por SQL. Vienen de `AGENTS.md` (se movieron el 2026-11-07 porque las instrucciones que carga cada sesión pasaron de 150 mil caracteres); los principios siguen en la constitución.

- **Sucursales** (spec 025, 0076): cajas, pedidos, tiendas de plataforma y existencias guardan
  `branch_id`; catálogo, empleados y lo fiscal siguen por empresa. Tres cosas que muerden:
  - **«La sucursal» la resuelve SOLO la base**: `branch_for_company(empresa)` y su envoltura
    `current_branch_id()`. Con una sucursal activa la devuelven; con dos y sin selector truenan con
    `EGB01`, que `store.DomainError` traduce a `domain.ErrBranchAmbiguous` (409 `BRANCH_AMBIGUOUS`).
    No escribas en Go un «toma la matriz»: es justo el adivinar que mezcla las ventas de dos locales.
  - **Un insert sin `branch_id` lo llena un trigger con la sucursal de la EMPRESA DE LA FILA**, no
    la de la sesión. Por eso un insert como owner para otra empresa funciona sin pasarla. Un pedido
    toma la de la caja de su turno; uno de plataforma, la de su tienda (explícita en `CreateOrder`).
    Un movimiento de almacén con `order_id` toma la de su pedido (0078): antes tomaba la de la
    empresa, y con dos sucursales toda venta tronaba con `BRANCH_AMBIGUOUS`. Una tabla nueva que
    cuelgue de un pedido necesita el mismo trigger, no el genérico.
  - **La matriz nace en `CreateCompany`, no en un trigger sobre `companies`**: `pg_restore` carga con
    COPY, que dispara triggers, y duplicaría la matriz que el respaldo ya trae. Un script que cree
    empresas a mano crea también su matriz (ver `docs/corte-produccion/01_nueva_empresa.sql`).
- **Un cambio de catálogo por SQL directo ya NO necesita borrar el menú guardado** (spec 034).
  Diez tablas —las que lee el menú del POS: `products`, `categories`, `modifier_groups`,
  `modifier_options`, `product_modifier_groups`, `product_channels`, `channels`,
  `delivery_platforms` y los dos precios por plataforma— tienen triggers de sentencia que suben
  `catalog_versions` de cada empresa tocada, sin importar quién escribe ni si puso
  `app.company_id`. La llave del menú en Redis y su `ETag` llevan esa versión, y la tableta revisa
  cada minuto y al volver al frente (304 si no cambió). No hagas `DEL pos:menu:*` ni
  `POST /admin/reload` para el menú: ya no hace falta.

  Origen: el 2026-10-10 un script desligó un grupo de extras de un producto; el menú guardado solo
  se borraba desde las pantallas de administración, la tableta siguió ofreciendo el grupo y el
  servidor rechazó la venta una y otra vez hasta que alguien borró el caché a mano. Si el menú
  llega a leer **otra tabla**, agrégala a la lista de la migración 0093 (en una migración nueva) y
  a `TestEveryMenuTableBumpsTheVersion`: una tabla sin trigger es exactamente ese camino. Y si
  aun así una tableta manda algo que ya no vale, el servidor responde `MENU_CHANGED` y la tableta
  recarga y pide elegir de nuevo.
- **Dónde se vende cada producto** (spec 092, 0118). Lugares de venta por empresa en `channels`:
  `pos` = Mostrador, `online` = Web (`qr` existe y no se usa). La decisión por producto vive en
  `product_channels` con **«sin fila = se ofrece»**: un producto nuevo o uno de antes de la 092 se
  vende en los dos sin que nadie lo marque. `product_channels.price` es el precio en ese lugar
  (null = el de mostrador; nunca cero). Cuatro cosas que muerden:
  - **Toda empresa necesita su catálogo de lugares de venta**: `CreateCompany` lo siembra y la 0118
    se lo dio a las que no lo tenían. El servicio igual lo asegura (`EnsureChannel`) antes de
    escribir: sin él, «apagar Web» insertaría cero filas y respondería bien.
  - **Leer el lugar de venta con subconsultas por código, no con un join**: cuando se agregue la
    sucursal a `product_channels`, un join multiplicaría filas. El upsert apunta a la restricción por
    nombre (`product_channels_pkey`) para que ese cambio no toque el SQL.
  - **La regla de la web vive en una sola función** (`WebSalesService.Offer`): qué se vende, a qué
    precio y si está agotado. El resumen de Ajustes y el menú público (#316) la usan; no la repitan.
  - **Oculto en mostrador lo rechaza también el servidor** (`MENU_CHANGED`) en pedidos sin
    plataforma; un pedido de plataforma no mira el lugar de venta.
- **«Vender sin existencia» y «Agotado»** (spec 092). `products.allow_oversell` (default true; la
  0118 normalizó a true los valores heredados de FUDO, que nunca se aplicaron). Con la opción
  apagada, un producto sale agotado si lo que descuenta una pieza sin extras (`domain.ExpandSale`:
  existencia propia, receta base o piezas del paquete) no alcanza en la sucursal; sin fila de
  existencia = 0. Lo que no descuenta nada nunca se agota.
  - **La existencia con la que se vende se lee SOLO por `app/sales_stock.go`** (`salesStock`,
    `lockSalesStock`). Con varios almacenes por sucursal (#65) cambia ahí por dentro y en ningún otro
    lado.
  - **El chequeo de la venta va dentro de la transacción del pedido y antes del primer descuento**,
    con un solo `select … for update` ordenado: dos tabletas que venden la última pieza no pasan las
    dos (`TestTwoCountersCannotSellTheLastPiece`). Rechaza con 409 `OUT_OF_STOCK`. Un pedido de
    plataforma nunca se revisa.
  - **La lista de agotados va aparte del menú** (`GET /pos/menu/sold-out`, sin caché): el menú se
    guarda por versión de catálogo y la existencia cambia con cada venta sin subir esa versión. No
    metas el agotado en el documento del menú.
- **Fotos del catálogo** (spec 091, runbook [medios-r2.md](../medios-r2.md)):
  - **`image_url` es la variante de 1200; la de 400 se deriva** cambiando `-1200.webp` por
    `-400.webp`. Escribir otra cosa a mano en esa columna la rechaza un check desde la 0117 (las
    filas viejas no se revisan: el check es `not valid`).
  - **La API solo borra lo que armó**: una URL que no empieza con `MEDIA_PUBLIC_BASE_URL` +
    empresa + clase + id del mismo registro se deja en paz. Por eso **duplicar un producto ya no
    copia su foto**: la llave lleva el id del original, y quitar la del duplicado habría borrado la
    del original.
  - **Una categoría sin familia se dibuja como «platillo»**. La familia es del producto (un enum),
    no de la empresa; una nueva lleva `alter type … add value`, su constante en `domain`, su
    renglón en `web/src/media/foodFamilies.ts` y su `<symbol>` en el sprite, que **cambia de
    nombre** (`food-families.v2.svg`) porque `public/` no lleva hash.
  - **El navegador prepara la foto**: Safari no codifica WebP en canvas y la pantalla lo dice al
    abrir el editor. En local, `start.sh` crea el bucket de versitygw con lectura pública; el
    almacenamiento no manda CORS, así que una prueba que lea pixeles baja la foto desde Node, no
    con `fetch` desde la página.

