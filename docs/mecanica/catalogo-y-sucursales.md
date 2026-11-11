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
