# Implementation Plan: Caducidad por lote

**Branch**: `098-caducidad-por-lote` | **Date**: 2026-11-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/098-caducidad-por-lote/spec.md` (issue #146)

## Summary

Lotes por almacén, artículo y fecha en una tabla nueva; lo sin fecha se deriva de la existencia que ya
existe. Un trigger de `stock_movements` aplica a todos los caminos (incluida la API anterior) la
regla: sin fecha primero, luego por fecha, nunca caducado; repone al mismo lote; lleva fechas en el
traspaso; y abre en la campana «lo que queda está caducado». Se captura la fecha al recibir y al
contar; una vista «Caducidades» proyecta consumo contra caducidad y manda lotes a merma. Detalle y
porqué en [research.md](research.md).

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript / React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose (embebidas); Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS; migración 0128 (aditiva)

**Testing**: `go test` table-driven en `domain` (orden de consumo, proyección, validación); integración
bajo `appRoleStore` con `inTheThreeCases` para las tres tablas y la vista; prueba de la 0128 sobre
respaldo restaurado con dos empresas; vitest; Playwright a 1024×600 (pedido del dueño) y 1280×854

**Target Platform**: equipo de mostrador táctil, 1280×854 mínimo; esta feature se diseña y prueba
también a 1024×600 por pedido del dueño

**Project Type**: web-service + web-app (monorepo)

**Performance Goals**: SC-005 — un pedido de 90 movimientos sin lotes paga solo un `exists` por
movimiento; con lotes, el costo se mide contra el mismo pedido sin lotes (referencia: 30.9 ms con el
trigger de avisos, `docs/mecanica/almacen.md`)

**Constraints**: subida con el negocio operando; la API anterior inserta movimientos sin fecha y la
regla los cubre; ninguna existencia cambia al migrar

**Scale/Scope**: 2 empresas, ~763 artículos activos; lotes solo de lo que se fecha

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I Layering | Regla de consumo en la base (R1, la única forma de cubrir a la API anterior y a los 15 caminos); validación del desglose y de fechas, y la proyección, puros en `domain/lots.go`; orquestación en `app/expiries.go`, `stock_counts.go`, `purchases*.go`, `waste.go`, `ingredient_merge`; handlers finos en `httpapi/handlers_expiries.go`; SQL en `queries/lots.sql` |
| II Errores | `domain.ErrValidation` para fecha fuera de rango, desglose inválido, `lotExpiresOn` en producto; `ErrLotsExceedCounted` sentinel → 400 en `httpapi.Error` |
| III Dinero | La feature no mueve dinero: cantidades con `Round4`. La merma desde un lote usa el costo de siempre. La vista no suma valor |
| IV TDD / RLS | Bordes del spec, cada uno con su test de integración antes del código; RLS de `stock_lots`, `stock_lot_allocations` y `stock_count_line_lots` con `inTheThreeCases` y bajo `TestEveryCompanyTableIsIsolated`; prueba de la 0128 con dos empresas: ninguna existencia cambia; tolera a la API anterior (aditiva, trigger cubre sus movimientos) |
| V Seguridad | `inventory.view` para Caducidades, `inventory.write_off` para mandar a merma, los de siempre para compra y conteo; almacén ajeno 404; `warehouseId` vacío 400; fechas malformadas 400, nunca default |
| VI YAGNI | Sin número de lote del fabricante, sin elegir lote a mano, sin lotes de productos, sin ventana configurable, sin tabla para la proyección |
| VII Idioma | `stock_lots`, `expires_on`, `expired_left`, `ProjectLots`; textos en español |
| VIII Puertas | ¿Se puede agregar después al mismo costo? Lotes de productos con existencia propia: sí (filas nuevas desde la siguiente compra). Número de lote del fabricante: sí (columna nueva en `stock_lots`, la llave pasaría a incluirlo; los lotes de hoy quedarían con número nulo). Lo que **no** se puede reconstruir después es la fecha de lo recibido: por eso entra ya en el renglón de compra y en el movimiento |

**Bordes enumerados antes de escribir código** (constitución IV):
- *Valor vacío que significa algo*: fecha vacía = sin fecha (`null`), nunca hoy; `lots` ausente en la
  captura del conteo = no cambia, `[]` = borra.
- *Estado que no sobrevive*: el desglose del conteo se guarda con la captura del artículo, como hoy la
  cantidad; recargar no lo pierde.
- *Camino nuevo que se salta el control viejo*: la merma desde Caducidades usa el mismo `POST` de merma
  (permiso, aprobación, bitácora).
- *Hermano que no se movió*: los 15 inserts de movimientos los cubre el trigger; los que necesitan
  fecha la pasan (compra y su relocalización, compensación, merma, aprobación y deshacer de merma).
  Unión, reemplazo y cambio de unidad mueven lotes explícitamente. `RecomputeArticleStock` no toca
  lotes (los escala el cambio de unidad).
- *Concurrencia*: venta y cierre de conteo del mismo artículo se serializan en la fila de
  `stock_levels` (R1); dos ventas igual.
- *Lote que caduca a media venta*: «hoy» se lee una vez por movimiento.

## Pantallas y maquetas

| Pantalla | Dónde vive | Qué cambia | Componentes |
| --- | --- | --- | --- |
| Caducidades (nueva) | Almacén › pestaña «Caducidades» después de «Existencias», solo con `inventory.view`, con `Segmented` de almacén (Local, Bodega, Todos) | Lista por plazo (maqueta A): grupos «Ya caducó», «Esta semana», «8 a 30 días», «31 a 60 días» con su cuenta y «Más adelante» cerrado; los vacíos no se muestran; estado vacío «Aún no hay fechas de caducidad. Regístralas al recibir una compra o al contar», carga y error con «Reintentar»; renglón de 56 px con artículo · almacén, una frase de proyección («Se termina en ~N días», «No se termina antes de caducar: quedan ~X», «No se ha vendido en 30 días», «Caducó: mándalo a merma»; en color de aviso las dos que piden decisión), cantidad, fecha y plazo, y «A merma» (sólido en lo caducado); con merma por aprobar, «Merma pendiente de aprobación» y el botón apagado | `Segmented`, `Button` 44 px, tokens del tema |
| Mandar a merma un lote | Hoja inferior desde «A merma» | Título con artículo, fecha y almacén; cantidad (todo el lote por omisión, «Todo») con `NumericKeypad` fijo a la derecha (decimales según la unidad); motivo con `Picker`, «Caducado» por omisión en lo caducado y sin motivo (botón apagado) en un lote vigente; «Mandar a merma». 2 toques en el caso común; cabe en 540 px de alto | Drawer, `NumericKeypad`, `Picker` |
| Fecha al recibir | Cada renglón de la hoja de entrega (`DeliverySheet`) | «Caduca…» compacto en la misma línea, solo en renglones con cantidad > 0, que abre la hoja mes y luego día (maqueta C): 12 meses por página con ‹ año ›, luego los días; «No trae fecha» abajo; celdas de 44 px; cabe en 90dvh a 1024×600. 3 toques. Vacío = sin fecha. Una fecha pasada pide confirmar con la hoja de confirmación de siempre. El botón del encabezado pasa a «Día de llegada por renglón» | Componente común nuevo `ExpiryPickerButton` en `web/src/components/` |
| Fechas en el conteo | Dentro del renglón elegido en la lista del conteo (maqueta A, sobre la vista sencilla de la 096) | Debajo del artículo elegido: sus fechas como chips en una línea (fecha · cantidad; tocar reabre el selector con ese valor; «quitar» de 44×44 aparte); las fechas que ya tenía el artículo se ofrecen sin cantidad; «+ Agregar fecha» con el mismo `ExpiryPickerButton` (aquí deja ir atrás); el teclado fijo escribe en el campo marcado con borde; «Sin fecha: X» calculado y, si las fechas suman más que lo contado, error en ese artículo sin bloquear el cierre. La lista tiene su propio scroll. Hoy se pinta dentro de la hoja de captura del conteo actual, y la vista sencilla de la 096 la hereda; su vista avanzada no lleva desglose | `ExpiryPickerButton`, `NumericKeypad` |
| Aviso en Existencias | Arriba del contenido de Existencias (maqueta A), a la derecha del selector de almacén cuando cabe | Una línea de 44 px «1 caducado · 2 caducan en 7 días · Ver caducidades ›» solo si hay algo en el almacén elegido | tokens del tema, `Button` |
| Campana | `Campana.tsx` | Aviso «Se está usando [artículo] y lo que queda está caducado» con la acción «Ver caducidades»; el nombre interno del aviso no sale a pantalla | existentes |

Ajustes de la revisión de diseño: [research.md › R12](research.md).

Maquetas a 1024×600 (pedido del dueño) y 1280×854, con la cifra de cada opción contra la pantalla
actual (toques y renglones visibles). Conteo y Existencias se alinean con las maquetas B de la 096.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Caducidades (y su hoja de merma) | [maquetas](https://claude.ai/artifact/DG42Q9HiKmgoVB2S5zjG2o) | A · Lista por plazo | Cada lote dice en una frase qué pasa y conserva el orden por fecha; 5 de 12 lotes a 1024×600, 8 de 12 a 1280×854; merma en 2 toques |
| Fecha al recibir | [maquetas](https://claude.ai/artifact/HooHrHzUjn2xZ7xmbbQ6i2) | C · Mes y luego día | 3 toques para cualquier fecha a 2 años (el calendario de siempre: 6 a 4 meses, 14 a un año); la misma hoja sirve al contar |
| Fechas en el conteo | [maquetas](https://claude.ai/artifact/MZSR4xHwwSaJeGb4M7zrkR) | A · Dentro del renglón | El teclado fijo ya está a la vista y «Sin fecha» se calcula solo; 4 toques por fecha |
| Aviso en Existencias | [maquetas](https://claude.ai/artifact/CLsWDPNFX73cXqJk1i7k3F) | A · Una línea arriba | Dice qué es (caducado o por caducar); a 1024×600 la pestaña de Caducidades queda fuera de vista; cuesta 1 de 14 renglones |

## Project Structure

### Documentation (this feature)

```text
specs/098-caducidad-por-lote/
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
├── migrations/0128_stock_lots.sql
├── queries/lots.sql (+ cambios en purchases.sql, expenses.sql, waste.sql, stock_counts.sql, notifications)
├── internal/domain/lots.go (+ _test.go)                # orden, proyección, validación
├── internal/app/expiries.go, stock_counts.go, purchases*.go, waste.go, ingredient merge/unit change
├── internal/httpapi/handlers_expiries.go, router.go
└── internal/integration/stock_lots_test.go, migration_0128_test.go
web/src/
├── features/backoffice/expiries/ExpiriesTab.tsx, WasteLotSheet.tsx (+ tests)
├── components/ExpiryPickerButton.tsx (+ test)
├── features/backoffice/DeliverySheet.tsx, StockCountTab.tsx, StockPage.tsx
├── features/notificaciones/Campana.tsx
└── api/backoffice.ts
docs/mecanica/almacen.md                                 # lotes: lo que no se ve
```

**Structure Decision**: monorepo existente; una carpeta nueva `features/backoffice/expiries/`.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Lógica de negocio (orden de consumo) en un trigger, no en `domain` | Única forma de que la API anterior y los 15 caminos de movimiento apliquen la misma regla (FR-024) | En Go: 15 lugares que recordar y la API anterior fuera durante la subida |
