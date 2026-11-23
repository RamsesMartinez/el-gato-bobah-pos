---
description: "Tareas de la 092: dónde se vende cada producto, vender sin existencia, precio web y Ajustes por secciones"
---

# Tasks: Dónde se vende cada producto (mostrador y web), vender sin existencia, precio web y Configuración con menú lateral

**Input**: `specs/092-venta-en-mostrador-y-web/` (plan.md, spec.md, research.md, data-model.md,
contracts/api.md, quickstart.md)

**Tests**: obligatorios (constitución IV, TDD). Cada tarea de prueba va **antes** de su código y se
ve fallar por la razón correcta.

**Organización**: por historia de usuario. Rutas reales del monorepo (`server/`, `web/`).

## Phase 1: Setup

- [X] T001 Traer `origin/develop` a la rama con merge firmado, revisar que la migración 0117 siga libre en `server/migrations/` (si otra rama la tomó, renumerar en todo `specs/092-venta-en-mostrador-y-web/` y en las tareas de abajo) y correr `codegraph sync` si hay índice

---

## Phase 2: Foundational (bloquea todas las historias)

**Propósito**: la migración, el catálogo de lugares de venta por empresa y la puerta única a la existencia con la que se vende.

- [X] T002 Prueba de integración de la migración en `server/internal/integration/migracion_0117_test.go`, sobre base restaurada con dos empresas: (a) `product_channels.price` existe, es nullable y rechaza `0` y `-1` por `product_channels_price_positive`; (b) toda empresa tiene `pos`/`qr`/`online` y los nombres son «Mostrador» y «Web»; (c) 0 productos con `allow_oversell=false` y `updated_at` sin mover en los que cambiaron; (d) `channels.id`, `product_channels.channel_id` y `category_channels.channel_id` son `bigint`; (e) la FK compuesta `(company_id, channel_id)` rechaza un canal de otra empresa insertado como owner; (f) Down y Up otra vez sin error, y el Down quita `price`
- [X] T003 Migración `server/migrations/0117_product_channel_price.sql` según data-model.md §Up/§Down: `set local lock_timeout = '3s'`, ancho a `bigint` (con la secuencia de identidad), `unique (company_id, id)` + FK compuesta `not valid`/`validate`, columna `price` con su check, siembra de canales con `on conflict … do update set name`, `allow_oversell = true` con `trg_products_updated` desactivado durante la sentencia, comentarios del porqué (R-3) y del Down (se pierden los precios web)
- [X] T004 Siembra de los tres canales en `CreateCompany` en `server/queries/companies.sql` (CTE `channels` con `pos`/Mostrador, `qr`/Menú QR, `online`/Web); prueba en `server/internal/integration/` que una empresa nueva los tiene
- [X] T005 Revisar `server/internal/integration/catalog_versions_test.go` (el insert de `channels … 'pos'` cerca de la línea 102) para que no choque con la siembra por `unique (company_id, code)`
- [X] T006 `make sqlc` y ajustar el Go que use `channel_id` como `int16` (ahora `int64`); `cd server && go build ./...`
- [X] T007 [P] Pruebas de tabla en `server/internal/domain/stock_availability_test.go` para `SoldOut` y `CheckStock`: existencia propia, receta de un insumo por pieza, receta de varios insumos, paquete con piezas, producto que no descuenta nada (nunca agotado), sin fila de existencia = 0, varios renglones del mismo producto que suman, dos productos que comparten insumo (solo cuentan los de opción apagada), cantidad exacta a la existencia (pasa) y una de más (falla con nombre y lo que queda)
- [X] T008 [P] Pruebas de tabla en `server/internal/domain/sales_channels_test.go` para `ChannelPrice` (null hereda, capturado gana), `ValidChannelPrice` (null válido; 0, negativo, NaN, ±Inf, 3 decimales y más del tope → `ErrValidation`) y `MeasuresStock` (own_stock, recipe y package sí; none, options y undecided no)
- [X] T009 Implementar `server/internal/domain/stock_availability.go` (`StockLevels`, `SoldOut`, `CheckStock`, `ErrOutOfStock` + tipo `OutOfStock{Name, Left}` con `Unwrap`) y `server/internal/domain/sales_channels.go` (`ChannelPrice`, `ValidChannelPrice`, `MeasuresStock`) hasta que T007 y T008 pasen
- [X] T010 Puerta única de existencia para vender en `server/internal/app/sales_stock.go`: `salesStock(ctx, q)` (lectura de niveles de la sucursal) y `lockSalesStock(ctx, q, branchID, items)` (un solo `select … for update order by item_type, product_id, ingredient_id`, devuelve `on_hand` de ese select), con su consulta en `server/queries/sales_stock.sql`; comentario que nombra al #65 como quien cambia su interior
- [X] T011 Mapear `domain.ErrOutOfStock` a 409 `OUT_OF_STOCK` en `server/internal/httpapi/respond.go`, con su caso en `server/internal/httpapi/respond_test.go`

**Checkpoint**: migración probada, canales en toda empresa, reglas puras probadas.

---

## Phase 3: User Story 1 - Decidir dónde se vende cada producto (P1)

**Goal**: casillas Mostrador/Web por producto, encendidas por omisión; oculto en mostrador no se vende en mostrador.

**Independent Test**: crear un producto sin tocar nada → sale en Vender y queda marcado para la web; apagar Mostrador → desaparece de Vender y `POST /orders` sin plataforma responde 422 `MENU_CHANGED`.

### Tests

- [X] T012 [P] [US1] Integración en `server/internal/integration/sales_channels_test.go`, bajo `appRoleStore` con `inTheThreeCases`: guardar `sales` (counter/web) y leerlo en `AdminListProducts`; `sales` ausente en el PATCH no toca nada; `sales` a medias → 400; empresa creada con `insert into companies` crudo (sin canales) puede apagar Web (el servicio asegura el canal) y nunca responde 200 sin escribir; duplicar copia lugares y precio; un producto recién creado nace con `allow_oversell=true` (FR-009)
- [X] T013 [P] [US1] Integración en `server/internal/integration/sales_channels_test.go`: pedido sin plataforma con un producto oculto en mostrador → `MENU_CHANGED`; el mismo producto en un pedido con plataforma entra; el menú del POS lo excluye
- [X] T014 [P] [US1] Prueba del handler en `server/internal/httpapi/handlers_admin_test.go`: el JSON del PATCH con y sin `sales`, y la lista trae `soldAtCounter`, `soldOnWeb`, `webPrice`, `allowOversell`, `measuresStock`, `description`

### Implementation

- [X] T015 [US1] Consultas en `server/queries/admin.sql`: `EnsureChannel` (`insert … on conflict do nothing` + `select id`), `UpsertProductChannel` (`on conflict on constraint product_channels_pkey`), campos nuevos en `AdminListProducts` con subconsultas escalares por código (sin join), `CloneProductChannels` copia `price`; `make sqlc`
- [X] T016 [US1] `GetPricedProducts` en `server/queries/orders.sql` trae `sold_at_counter` (sin fila oculta para `pos`); en `server/internal/app/orders.go` (`prepareCreate` y `prepareAddLines`) un producto oculto en mostrador cuenta como no activo solo si el pedido no es de plataforma
- [X] T017 [US1] `AdminService` en `server/internal/app/admin.go`: `UpdateProductInput.Sales *SalesInput` y lo mismo en el alta; validación (`ValidChannelPrice`, descripción ≤ 500) y escritura en la misma transacción del PATCH; `ErrNotFound` si no hay canal tras asegurarlo
- [X] T018 [US1] Handlers en `server/internal/httpapi/handlers_admin.go`: decodificar `sales` opcional con todos sus campos obligatorios; respuesta de la lista con los campos nuevos (`measuresStock` con `domain.MeasuresStock`)
- [X] T019 [P] [US1] Tipos y API en `web/src/api/admin.ts`: campos opcionales nuevos de `AdminProduct` (el compilador obliga a la guarda) y `sales` en `updateProduct`/`createProduct`
- [X] T020 [US1] Prueba (vitest) y luego la ficha a pantalla completa con menú lateral de pestañas (maqueta «Ficha del producto», G2 en plan.md §Maquetas) en `web/src/shared/ProductEditDialog.tsx` + `web/src/shared/producto/` (un archivo por pestaña: General, Disponibilidad, Venta en web, Receta e inventario, Extras); pruebas en `web/src/shared/ProductEditDialog.test.tsx`: cada bloque de hoy está en una sola pestaña, cambiar de pestaña conserva lo capturado, Guardar siempre tocable y salta a la pestaña del error con punto rojo, Extras ya no dice «no dependen de Guardar»
- [X] T021 [US1] Pestaña Disponibilidad en `web/src/shared/producto/Disponibilidad.tsx`: casillas grandes Mostrador/Web (≥ 56 px) y temporada; prueba de que apagar Mostrador manda `sales.counter=false`
- [X] T022 [US1] Alta en `web/src/features/admin/ProductsAdminPage.tsx` (`NewProductDialog`): «Opciones avanzadas» cerrado (botón ≥ 44 px) con las dos casillas y el resumen «Se vende en: Mostrador · Web»; prueba en `web/src/features/admin/ProductosCostoAltaDuplicado.test.tsx` o archivo hermano
- [X] T023 [US1] Lista de productos en `web/src/features/admin/ProductsAdminPage.tsx`: etiqueta «No se vende en ningún lado» cuando las dos están apagadas, con su prueba
- [X] T024 [US1] Vender ante `MENU_CHANGED` por un producto oculto: ya recarga el menú (spec 034); prueba de regresión en `web/src/features/pos/` de que el producto desaparece tras la recarga, y de que la pestaña «Top» y los favoritos no muestran un id que ya no está en el menú (`PopularProducts` puede devolverlo)

**Checkpoint**: US1 sola es entregable.

---

## Phase 4: User Story 2 - Que un producto se agote solo (P1)

**Goal**: con «Vender sin existencia» apagado y sin existencia para uno, sale «Agotado» en Vender y el servidor rechaza la venta de mostrador.

**Independent Test**: refresco (receta de un insumo) con la opción apagada e insumo en 0 → tarjeta «Agotado» en ≤ 60 s; cobrar desde pantalla vieja → 409 `OUT_OF_STOCK`.

### Tests

- [X] T025 [P] [US2] Integración en `server/internal/integration/agotado_test.go` bajo `appRoleStore` con `inTheThreeCases`: `GET /pos/menu/sold-out` devuelve `[]` (JSON crudo, nunca `null`) sin productos con la opción apagada; con uno sin existencia lo lista; reponer lo saca; existencia de otra sucursal no cuenta
- [X] T026 [P] [US2] Integración `TestTwoCountersCannotSellTheLastPiece` en `server/internal/integration/agotado_test.go`: dos transacciones venden la última pieza a la vez → una pasa y la otra recibe `ErrOutOfStock`; con la opción encendida las dos pasan y la existencia queda negativa; pedido de plataforma pasa siempre; agregar renglones a un pedido abierto y enviar una cuenta en captura también chequean
- [X] T027 [P] [US2] Prueba del handler `GET /pos/menu/sold-out` en `server/internal/httpapi/handlers_menu_test.go`: `Cache-Control: no-store` y `productIds: []`

### Implementation

- [X] T028 [US2] `MenuService.SoldOut` en `server/internal/app/menu.go`: si ningún producto activo tiene `allow_oversell=false`, `[]` sin cargar el grafo; si hay, `loadStockGraph` + `salesStock` + `domain.SoldOut`; consulta de candidatos en `server/queries/menu.sql`
- [X] T029 [US2] Handler y ruta `GET /pos/menu/sold-out` en `server/internal/httpapi/handlers_menu.go` y `server/internal/httpapi/router.go` (mismo grupo y permisos que `GET /pos/menu`)
- [X] T030 [US2] Chequeo en la venta en `server/internal/app/orders.go`: en `createInTx` y `addLinesInTx`, antes del primer `descontarRenglon` y solo sin plataforma, `lockSalesStock` + `domain.CheckStock` con la sucursal del pedido
- [X] T031 [US2] Guardar `allowOversell` en `sales` (ya en T017) y mostrarlo en la pestaña Receta e inventario `web/src/shared/producto/RecetaEInventario.tsx`: interruptor solo si `measuresStock`; si no, la línea «Este producto no descuenta del almacén. Agrega su receta para controlarlo.»; prueba vitest
- [X] T032 [P] [US2] `web/src/api/pos.ts` + `web/src/hooks/useSoldOut.ts`: consulta cada 60 s y al volver al frente, como `useMenu`; prueba en `web/src/hooks/useSoldOut.test.ts`
- [X] T033 [US2] Tarjeta «Agotado» en `web/src/features/pos/ProductTile.tsx` (y `ProductGrid.tsx`): mismo tamaño, etiqueta con tokens del tema, tocarla avisa «Agotado: no hay existencia» y no agrega; prueba en `web/src/features/pos/ProductTile.test.tsx`
- [X] T034 [US2] Ante 409 `OUT_OF_STOCK` al enviar o cobrar: mensaje con el nombre que manda el servidor e invalidación de `['menu','sold-out']`; prueba en el test del flujo de cobro de `web/src/features/pos/`

**Checkpoint**: US1 + US2 cubren los dos P1.

---

## Phase 5: User Story 3 - Precio y datos para la web en el mismo producto (P2)

**Goal**: pestaña «Venta en web» con interruptor, «Usar el precio de mostrador ($N)», precio propio y descripción.

**Independent Test**: capturar precio web → Vender sigue cobrando el de mostrador y la lista trae `webPrice`; borrarlo → `null`; 0 → rechazado.

### Tests

- [X] T035 [P] [US3] Integración en `server/internal/integration/sales_channels_test.go`: precio web guardado no cambia `products.price` ni `product_platform_prices`; `null` lo borra; `0`, negativo y 3 decimales → 400 (no 500 del check); cambiar el precio de mostrador sin precio web hace que el efectivo lo siga
- [X] T036 [P] [US3] Prueba vitest en `web/src/shared/producto/VentaEnWeb.test.tsx`: casilla «Usar el precio de mostrador ($35.00)» encendida por omisión; al apagarla el precio es obligatorio; apagar «Se vende en la web» oculta precio y descripción pero los conserva; el interruptor y la casilla Web de Disponibilidad son el mismo dato

### Implementation

- [X] T037 [US3] Pestaña `web/src/shared/producto/VentaEnWeb.tsx` según la maqueta elegida, con `Field`, `Switch`, `Checkbox` del tema y descripción (≤ 500)
- [X] T038 [US3] Regla única de la web (FR-005, FR-012) y su resumen: `WebSalesService.Offer(ctx)` en `server/internal/app/web_sales.go` devuelve por producto activo que se vende en la web su id, precio efectivo (`domain.ChannelPrice`) y si está agotado (`salesStock` + `domain.SoldOut`), con su consulta en `server/queries/web_sales.sql`; el resumen cuenta sobre esa misma lista (no otra consulta), con prueba de que un producto apagado para web no sale, uno agotado sale marcado y el precio efectivo hereda; handler `GET /admin/web-sales/summary` en `server/internal/httpapi/handlers_web_sales.go` con `RequirePermission(business.configure)` en `router.go`; prueba de integración bajo `appRoleStore` con `inTheThreeCases` en `server/internal/integration/web_sales_test.go`

---

## Phase 6: User Story 4 - Configuración ordenada por secciones (P2)

**Goal**: Ajustes del negocio por secciones con selector fijo de tres vistas (A por omisión).

**Independent Test**: a 1280×854 y 1024×600, cualquier ajuste de hoy en ≤ 2 toques; `?seccion=caja` abre Caja; sin `attendance.configure` no sale Personal; el selector no se mueve entre vistas.

### Tests

- [X] T039 [P] [US4] Prueba vitest en `web/src/features/admin/BusinessSettingsPage.test.tsx`: cada bloque de hoy aparece en exactamente una sección; secciones según permiso; `?seccion=` abre la sección; la vista elegida se recuerda (con `localStorage` que truena, abre en A); los tres botones del selector tienen texto y ≥ 44 px
- [X] T040 [P] [US4] Prueba vitest de la sección «Venta en web» en `web/src/features/admin/settings/VentaEnWebSection.test.tsx`: muestra los dos conteos y el acceso a Menú

### Implementation

- [X] T041 [US4] Partir `web/src/features/admin/BusinessSettingsPage.tsx` en secciones (`web/src/features/admin/settings/{Negocio,VentaEnMostrador,VentaEnWeb,Caja}Section.tsx`, reutilizando `InventorySettingsSection` y `AttendanceSettingsSection`) sin cambiar el comportamiento de ningún ajuste
- [X] T042 [US4] Barra de título fija con selector de vista (Lista, Íconos, Índice) y las tres vistas en `web/src/features/admin/settings/SettingsLayout.tsx` (maqueta «Ajustes del negocio» en plan.md §Maquetas); vista en `localStorage` con try/catch; sección en `?seccion=`
- [X] T043 [US4] `VentaEnWebSection` con `GET /admin/web-sales/summary` (T038) y botón a Menú

---

## Phase 7: Polish

- [X] T044 Documentar en `docs/mecanica/catalogo-y-sucursales.md`: lugares de venta (`pos`=Mostrador, `online`=Web), «sin fila = se ofrece», precio por lugar, la puerta única de existencia para vender (#65) y que un canal nuevo necesita el trigger de 0093
- [ ] T045 Herramientas MCP: si `mcp_tools_090_catalog.go` edita productos con un PATCH completo, verificar que no borre `sales` (ausente = no tocar) con una prueba en `server/internal/httpapi/`
- [ ] T046 `/revision-de-codigo` (hook `after_implement`) y corregir lo que encuentre con su prueba
- [ ] T047 Prueba con alguien nuevo de una fonda a 1024×600 y 1280×854 contra `make start` del worktree con base restaurada (`make db-restaurar`), siguiendo `quickstart.md`, y medir con Playwright que ninguna pestaña de la ficha desliza a 1024×600 con un producto de 4 grupos de extras (SC-007); corregir y volver a probar
- [ ] T048 `cd server && go build ./... && go test ./...`, `make test-integration`, `cd web && bun run lint && bun run vitest run && bun run build`, y `make ci-local` en verde

---

## Dependencies & Execution Order

- Setup (T001) → Foundational (T002–T011) → historias.
- US1 (T012–T024) primero: crea la ficha a pantalla completa en la que US2 y US3 agregan pestañas.
- US2 (T025–T034) depende de Foundational y de T017 (guardar `allowOversell`) y T020 (pestañas).
- US3 (T035–T038) depende de T017 y T020.
- US4 (T039–T043) solo depende de T038 para la sección Venta en web; el resto es independiente.
- Polish al final.

## Parallel Opportunities

- T007 y T008 (domain) en paralelo.
- T012, T013, T014 (pruebas US1) en paralelo; T019 en paralelo con el backend de US1.
- T025, T026, T027 en paralelo; T032 en paralelo con T028–T030.
- US4 (front) en paralelo con US2 (back).

## Implementation Strategy

MVP = Foundational + US1 + US2 (los dos P1). US3 y US4 completan la feature antes del PR. Todo
sale en una sola subida; la migración tolera a la versión anterior corriendo.
