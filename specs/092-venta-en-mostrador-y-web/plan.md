# Implementation Plan: Dónde se vende cada producto, vender sin existencia, precio web y Configuración con menú lateral

**Branch**: `092-venta-en-mostrador-y-web` | **Date**: 2026-11-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/092-venta-en-mostrador-y-web/spec.md`

## Summary

Reusar el catálogo de lugares de venta que existe desde la 0004 (`channels` + `product_channels`,
«sin fila = se ofrece») para Mostrador (`pos`) y Web (`online`), agregarle un precio opcional por
lugar (`product_channels.price`, null = el de mostrador) y aplicar por fin `products.allow_oversell`
después de normalizar a `true` los valores heredados de FUDO. «Agotado» se decide con
`domain.ExpandSale` (existencia propia, receta base o piezas del paquete), se lista aparte del menú
cacheado y se valida con candado dentro de la transacción del pedido de mostrador. La ficha del
producto gana «Opciones avanzadas» (casillas Mostrador/Web y Vender sin existencia) y la sección
«Venta en web»; la pantalla Negocio se parte en secciones con menú lateral.

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS por empresa; Redis solo caché del menú (no cambia)

**Testing**: `go test` (unitarios en `domain`, tabla), integración contra Postgres real bajo
`appRoleStore` con `inTheThreeCases`, vitest + Testing Library, Playwright a 1024×600 y 1280×854

**Target Platform**: API en el VPS (azul/verde); front PWA en el equipo de mostrador

**Project Type**: web (server + web)

**Performance Goals**: `GET /pos/menu/sold-out` en una consulta cuando no hay productos con la
opción apagada (el caso de toda empresa el día de la subida); con alguno, una carga del grafo de
existencias (la misma que ya hace cada venta) y una consulta de niveles de la sucursal

**Constraints**: migración tolerante a la versión anterior corriendo; ningún producto cambia de
comportamiento en la subida; controles de 44 px; sin `<select>` nativo

**Scale/Scope**: ~540 productos por empresa; 2 empresas; 1 sucursal cada una

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I. Layering | `domain`: `ChannelPrice`, `ValidChannelPrice`, `SoldOut`, `CheckStock`, `MeasuresStock`, `ErrOutOfStock`. `app`: `AdminService` (guardar `sales` en la tx del PATCH/alta), `MenuService.SoldOut`, `OrdersService` (chequeo en `createInTx`/`addLinesInTx`), `WebSalesService.Summary`. `httpapi`: handlers finos, mapeo `OUT_OF_STOCK` en `respond.go`. `store/db`: queries sqlc nuevas. |
| II. Errores | `ErrOutOfStock` sentinel con tipo `OutOfStock{Name, Left}` y `Unwrap`; mapeo solo en `httpapi.Error`. |
| III. Dinero | `webPrice` con `Round2` + `ValidMoney`, `> 0`; el precio de mostrador y de plataforma no se tocan; el pedido sigue recalculando en el servidor. |
| IV. Test-first | Unitarios de tabla de cada función de `domain` primero. Integración: migración 0117 sobre base restaurada con dos empresas; RLS de lo nuevo con `inTheThreeCases`; `TestTwoCountersCannotSellTheLastPiece`; JSON crudo de `sold-out` con `[]`. |
| IV. Migración tolerante | Solo agrega columna nullable, filas faltantes y normaliza un booleano que la versión anterior no lee (data-model, tabla de traslape). |
| V. Seguridad | Todo detrás de `RequirePermission` (`menu.configure`, `business.configure`); `sold-out` en el grupo de Vender; valores absurdos → 400; la barrera del agotado y del oculto en mostrador vive en el servidor. |
| VI. YAGNI | Sin tabla nueva, sin columna por lugar; sin menú público (#316); «Avisos» y «Apariencia» fuera hasta tener contenido. |
| VII. Comentarios | El porqué de normalizar `allow_oversell` y de la lista aparte del menú va en la migración y en `MenuService.SoldOut`. |
| VIII. Puertas | «Una lista de productos por plataforma»: se usa la llave correcta `(producto, lugar de venta)` y la tabla admite plataformas y sucursal con columnas nullable (data-model, «Puerta»). Ninguna puerta se cierra. |
| Restricciones UX | Casillas de 44 px, sin `<select>`, «Opciones avanzadas» colapsado para no estorbar el alta, componentes comunes (`Switch`/`Checkbox`, `Field`, `Page`). Texto para quien opera. |

Sin violaciones; *Complexity Tracking* vacío.

## Capas y piezas

| Pieza | Capa | Archivo |
| --- | --- | --- |
| Migración 0117 + su test | migración / integración | `server/migrations/0117_product_channel_price.sql`, `server/internal/integration/migration_0117_test.go` |
| Lugares de venta al crear empresa | query | `server/queries/companies.sql` (`CreateCompany`) |
| Reglas de precio, agotado y chequeo | domain | `server/internal/domain/sales_channels.go`, `stock_availability.go` (+ `_test.go`) |
| Leer/guardar lugares de venta y precio web | query + app | `server/queries/admin.sql`, `server/internal/app/admin.go` |
| Lista de agotados | query + app + handler | `server/queries/menu.sql`, `server/internal/app/menu.go`, `server/internal/httpapi/handlers_menu.go` |
| Chequeo en la venta | app | `server/internal/app/orders.go`, `server/queries/orders.sql` (`GetPricedProducts` + `LockStockLevels`) |
| Resumen de la web | query + app + handler | `server/queries/web_sales.sql`, `server/internal/app/web_sales.go`, `server/internal/httpapi/handlers_web_sales.go` |
| Mapeo `OUT_OF_STOCK` | httpapi | `server/internal/httpapi/respond.go` |
| Ficha del producto | web | `web/src/shared/ProductEditDialog.tsx`, `web/src/shared/VentaEnWebSection.tsx`, `web/src/shared/OpcionesAvanzadas.tsx`, alta en `web/src/features/admin/ProductsAdminPage.tsx` |
| Agotado en Vender | web | `web/src/hooks/useSoldOut.ts`, tarjeta de producto en `web/src/features/pos/` |
| Lista de productos | web | badge «No se vende en ningún lado» en `ProductsAdminPage.tsx` |
| Configuración | web | `web/src/features/admin/BusinessSettingsPage.tsx` → `web/src/features/admin/settings/*` |

## Pantallas y maquetas

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Ficha del producto (casillas Mostrador/Web y Vender sin existencia en «Opciones avanzadas», sección «Venta en web») | [maquetas](https://claude.ai/artifact/JgmuMgs77e5mpgxVFDTydv) | _esperando al dueño_ | — |
| Configuración (Negocio) con menú lateral | [maquetas](https://claude.ai/artifact/8mxJFnD73y2dxyoBxWbbtg) | _esperando al dueño_ | — |
| Tarjeta «Agotado» en Vender | Sin maqueta: usa el estado atenuado de la tarjeta existente más una etiqueta; no cambia disposición | — | — |

## Project Structure

### Documentation (this feature)

```text
specs/092-venta-en-mostrador-y-web/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code

```text
server/
├── migrations/0117_product_channel_price.sql
├── queries/{admin,menu,orders,companies,web_sales}.sql
└── internal/
    ├── domain/{sales_channels,stock_availability}.go
    ├── app/{admin,menu,orders,web_sales}.go
    ├── httpapi/{handlers_admin,handlers_menu,handlers_web_sales,respond,router}.go
    └── integration/…_test.go
web/src/
├── shared/{ProductEditDialog,VentaEnWebSection,OpcionesAvanzadas}.tsx
├── hooks/useSoldOut.ts
├── features/pos/…           (tarjeta Agotado)
└── features/admin/{ProductsAdminPage,BusinessSettingsPage}.tsx, settings/*.tsx
```

**Structure Decision**: monorepo existente `server/` + `web/`; ninguna carpeta nueva fuera de
`web/src/features/admin/settings/`.

## Coordinación con otras ramas

- **#325** (fotos) también toca la ficha del producto: se trae `develop` a esta rama en cada etapa
  (antes de maquetas aprobadas, antes de implementar el front y antes del PR), con merge firmado.
- **#325 y #327** pueden tomar la migración 0117: la primera en llegar a develop se queda con el
  número; esta rama renumera si llega después.

## Complexity Tracking

Vacío: sin violaciones.
