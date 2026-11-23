# Implementation Plan: Varios almacenes por sucursal

**Branch**: `094-varios-almacenes` | **Date**: 2026-11-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/094-varios-almacenes/spec.md` (issue #65)

## Summary

Cada sucursal pasa de un montón de existencias a N almacenes, con uno de venta. Todo registro que hoy
guarda existencias de la sucursal gana `warehouse_id`, que entra nullable y lo llena un trigger con
el almacén de venta (patrón de la 0076), y pasa a `not null` en la subida siguiente. Las llaves
únicas por sucursal se expanden a almacén en la primera subida y se contraen en la segunda, para que
la API anterior nunca pierda su `on conflict`. Se sube en cinco subidas siguiendo el orden del
requerimiento (research R3).

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript / React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose (embebidas); Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS (`gatobobah_app`); migraciones 0117 (subida 1), 0118 (subida 2),
0119 (subida 3)

**Testing**: `go test` (dominio, table-driven), integración contra Postgres de pruebas bajo
`appRoleStore` con `inTheThreeCases`, migración sobre respaldo restaurado con dos empresas; vitest;
Playwright a 1280×854 y 1024×600

**Target Platform**: equipo de mostrador táctil (1280×854 mínimo; el dueño pidió también 1024×600)

**Project Type**: web-service + web-app (monorepo `server/` + `web/`)

**Performance Goals**: la venta no suma más de un `lookup` por movimiento (el trigger de llenado
resuelve el almacén de venta por índice único)

**Constraints**: subidas con el negocio operando; API anterior y nueva sobre el mismo esquema;
tablas medidas: `stock_movements` 7,219 filas, `stock_levels` 407 (2026-11-23)

**Scale/Scope**: 2 empresas, 1 sucursal cada una, 1–3 almacenes por sucursal

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I Layering | Reglas puras en `domain/warehouse.go` y `domain/transfer.go` (validar nombre, archivar, plan de traspaso, «hay existencia»); servicios en `app/warehouses.go`, `app/transfers.go`; handlers finos; SQL solo en `queries/warehouses.sql`, `queries/transfers.sql` |
| II Errores | Sentinels nuevos `ErrWarehouseNotFound`, `ErrWarehouseIsSales`, `ErrWarehouseHasStock`, `ErrWarehouseCountOpen`, `ErrWarehouseNameTaken`, `ErrTransferKeyReused`, `ErrWarehouseNotWritableYet`; `EGB03` → `ErrWarehouseMissing` en `store/errors.go`; mapeo solo en `httpapi.Error` |
| III Dinero | Traspaso sin dinero propio: valor = cantidad × costo vigente (`Round2`); cantidades con `Round4` y `ValidQty`. El reporte de valor declara que excluye negativos (no suma valor negativo) — test que lo nombra |
| IV TDD / RLS | Test primero por cada borde de la lista de abajo; `warehouses`, `stock_transfers`, `stock_transfer_lines` bajo `TestEveryCompanyTableIsIsolated`; servicios nuevos con `inTheThreeCases`; migración probada sobre respaldo con dos empresas; las tres migraciones toleran a la versión anterior (research R1, R2) |
| V Seguridad | Permisos nuevos con `RequirePermission`; `warehouseId` ajeno = 404 (no enumera); `warehouseId` malformado = 400; `clientKey` reusado con otro cuerpo = 409; llaves compuestas impiden cruzar sucursal o empresa aunque RLS fallara |
| VI YAGNI | Sin permisos por almacén, sin traspasos entre sucursales, sin almacén de venta por lugar de venta (research R8, spec Assumptions) |
| VII Idioma | Identificadores en inglés (`warehouses`, `is_sales`); textos en español |
| VIII Puertas | Cruza «Más de un almacén por sucursal». ¿Se puede agregar después al mismo costo? Lo que no: el almacén de cada movimiento — por eso entra en la subida 1 aunque solo exista uno. Se actualiza la tabla de puertas de la constitución con la subida 1 (R12) |

**Bordes enumerados antes de escribir código** (constitución IV):
- Valor vacío: `warehouseId` ausente vs. `""` vs. `0` → ausente = de venta; `""`/`0`/texto = 400.
- Estado que no sobrevive: el último destino de compra se recuerda en `localStorage` con try/catch;
  si falta, el de venta. El traspaso guarda su `clientKey` en el borrador para que un F5 a media
  confirmación no lo duplique.
- Camino nuevo que se salta el control viejo: el traspaso inserta movimientos → pasa por
  `redirect_merged_ingredient_movement` y por el aviso de stock bajo como cualquier movimiento; toma
  `lockIngredients` (nowait) antes de escribir, como la compra.
- Hermano que no se movió: los seis upserts por sucursal (research R2), las 11 consultas con
  `current_branch_id()` que leen existencias, `RecomputeArticleStock` (suma por sucursal → por
  almacén), `InsertMergeMove` / `InsertReplacementMove` (uno por sucursal → uno por almacén),
  `LockArticleStockNowait`. Lista completa en tasks.
- Concurrencia: archivar vs. traspaso al mismo almacén — archivar toma el almacén `for update` y el
  traspaso `for share`.

## Pantallas y maquetas

| Pantalla | Dónde vive | Qué cambia | Componentes |
| --- | --- | --- | --- |
| Almacenes | Negocio › Inventario, junto a «Lugares para contar», visible con `inventory.warehouse_manage` (R12) | Nueva (maqueta A3): «Agregar almacén» arriba; grupos «Donde se vende» y «Donde se guarda»; cada almacén en un renglón de 72 px con ícono (tienda / bodega), nombre, etiqueta «De venta» y cuántos artículos tiene; tocarlo abre la hoja de renombrar y archivar (con `ConfirmSheet`). Los encabezados de grupo con el estilo de sección del POS, sin mayúsculas | `ListRow` ampliado con ícono opcional, hoja inferior (`DrawerRoot`), `ConfirmSheet` |
| Traspaso | Almacén › pestaña «Traspaso», inmediatamente después de «Existencias», con `inventory.transfer` | Nueva (maqueta B): «De [Picker] → A [Picker]» con botón para invertir y buscador arriba; cantidades con `NumericKeypad` fijo al pie; tabla de lo que hay en el origen con su existencia en origen y destino y un −/+ por renglón; nota y «Traspasar N artículos» al pie | `Table`, stepper −/+ de 44 px, `Input` de búsqueda |
| Existencias | Almacén › Existencias | Segmentado «Local · de venta / Bodega / Todos» arriba de la tabla, solo con 2+ almacenes (con 5+ pasa a botón con hoja); un negativo en el de venta con existencia en otro ofrece «Traer de Bodega», que abre una hoja con la cantidad propuesta y el teclado numérico | Segmentado de botones de 44 px, `Table` actual, `NumericKeypad` en hoja inferior |
| Compra (captura completa) | `ExpenseDialog`, paso de captura | Rediseño (maqueta R3): encabezado proveedor · fecha · total + «Más datos»; lista de renglones de una línea (nombre, cantidad × empaque, estado, importe) a la izquierda y el detalle del elegido a la derecha (cantidad, empaque, ¿llegó?, ¿dónde se guarda?, regalo / no es del negocio, quitar). «¿Dónde se guarda?» arriba de la lista (solo con 2+ almacenes) | `Picker`, segmentados de 44 px, `InfoButton` |

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Almacenes | [maquetas](https://claude.ai/artifact/L8X2ZE7XG5fEZh22PLAmqh) | A3 · Íconos y dos grupos | La primera vuelta se veía plana; ícono por almacén y los grupos «Donde se vende» / «Donde se guarda» dicen el papel de cada uno de un vistazo |
| Traspaso | [maquetas](https://claude.ai/artifact/V9629bGHmWbAaL1CarFKU1) | B · Lista con cantidades | Origen y destino lado a lado y varios artículos con nota en un traspaso (2 toques + teclear) |
| Existencias | [maquetas](https://claude.ai/artifact/YDruwPNs9Zsks9QFMY2Xei) | A · Segmentado | Los almacenes a la vista, 1 toque. Con 5 o más almacenes el mismo control pasa a botón con hoja (opción B), sin cambiar nada más de la pantalla |
| Compra (captura completa) | [maquetas](https://claude.ai/artifact/S1AecknYDfWrjyUNaCtgMU) | R3 · Lista y detalle al lado | El dueño pidió rediseñar toda la captura (se veía amontonada): encabezado en una línea, renglones de una línea, detalle al lado sin tapar la lista. «¿Dónde se guarda?» para todo el ticket y etiqueta en el renglón que va a otro lado |
| Color de la captura | [maquetas](https://claude.ai/artifact/XdX8cGkCLBvZ5orGH584py) | P1 · Ícono al inicio con la paleta del negocio | Se veía cargado de colores. Lo elegido y lo activo toman el `colorPalette` heredado de Apariencia (nada de `colorPalette="brand"` fijo); el estado es un ícono redondo al inicio del renglón (palomita, reloj, persona) y solo lo excepcional se escribe; el resto neutro |
| Regalo / no es del negocio | [maquetas](https://claude.ai/artifact/GaMa1CmFozHRUqP7itjjys) | S1 · Interruptores con explicación | Quien lo vio no supo qué era; cada opción dice en una línea qué pasa y el (i) da el ejemplo. Como es la más alta, el panel de detalle desplaza por dentro (`flex=1 minH=0 overflowY=auto`) y la lista no se mueve |

Ajustes de las dos revisiones de diseño: [research.md › R11 y R12](research.md). Componente común nuevo: `web/src/components/Segmented.tsx`.

## Project Structure

### Documentation (this feature)

```text
specs/094-varios-almacenes/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
└── tasks.md
```

### Source Code

```text
server/
├── migrations/NNNN_warehouses.sql            # subida 1 (número al llegar a develop, R11 M7)
├── migrations/0118_warehouses_contract.sql   # subida 2
├── migrations/0119_stock_transfers.sql       # subida 3
├── queries/warehouses.sql, transfers.sql     # nuevas; y las existentes por almacén
├── internal/domain/warehouse.go, transfer.go (+ _test.go)
├── internal/app/warehouses.go, transfers.go  # y los servicios de existencias que cambian
├── internal/httpapi/handlers_warehouses.go, mcp_tools_094.go
└── internal/integration/warehouses_test.go, transfers_test.go, migracion_0117_test.go
web/src/
├── features/backoffice/warehouses/ (WarehousesTab, TransferTab, useWarehouses, WarehouseSelector)
├── features/backoffice/StockPage.tsx, ExpenseDialog.tsx, StockCountTab.tsx
└── api/backoffice.ts, app/permissions.ts
docs/mecanica/almacen.md                      # notas de lo que no se ve
```

**Structure Decision**: monorepo existente; ninguna carpeta nueva fuera de `features/backoffice/warehouses/`.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Dos llaves únicas vivas a la vez en la subida 1 | La API anterior hace `on conflict` sobre la de sucursal | Cambiarla de golpe da 500 en conteo y mínimos durante el traslape (constitución IV) |
