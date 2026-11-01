# Implementation Plan: Editar un gasto ya guardado (entrega 1)

**Branch**: `053-editar-gasto` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/053-editar-gasto/spec.md`

## Summary

Un gasto guardado se corrige desde su detalle: encabezado editable, renglones que se agregan y se
borran, y entregas parciales con su fecha. Todo pasa por una vista previa que calcula el servidor
corriendo **la misma transacción** que confirma y revirtiéndola (R1); una huella garantiza que lo
confirmado es lo que se vio (R2). La fecha de cada entrega vive en una columna nueva de
`stock_movements` (`effective_on`, migración 0104, R3); el contra-movimiento de un borrado lleva la
fecha de cada entrega original (R7), y ante un conteo cerrado posterior quien edita decide «Ajustar
el inventario» o «No mover», que se registra como un movimiento de compensación ligado al conteo
(R5, R6). El importe sigue la regla de R4 y el estado de pago se recalcula; los pagos nunca se tocan.

## Technical Context

**Language/Version**: Go 1.27.3 (server) · TypeScript / React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose, shopspring/decimal · Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS (`gatobobah_app`); migración 0104

**Testing**: `go test` (unitarios en `domain`, integración bajo `appRoleStore` con
`inTheThreeCases`), vitest, Playwright a 1024×600 contra el `make start` del worktree

**Target Platform**: API Linux en contenedor; tabletas de 7-10" a 1024×600

**Project Type**: web (monorepo `server/` + `web/`)

**Performance Goals**: vista previa en menos de 1 s con un gasto de hasta 60 renglones
(`maxExpenseItems`); no es ruta caliente

**Constraints**: candados sin esperar a las ventas (`lockIngredients`); tolerancia a la API anterior
durante la subida; nada de `<select>` ni calendario nativo

**Scale/Scope**: un negocio, decenas de gastos al día, ~60 renglones por gasto como máximo

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple | Estado |
|---|---|---|
| I. Layering | Reglas puras en `domain/expense_edit.go`; orquestación y transacción en `app/expense_edit.go`; handlers finos en `httpapi/handlers_expense_edit.go`; SQL en `queries/purchases.sql` + `make sqlc` | OK |
| II. Errores | Centinelas de `domain` (`ErrValidation`, `ErrConflict`, nuevos `ErrDecisionRequired`, `ErrExpenseEditStale`) mapeados solo en `httpapi.Error`; el centinela interno de la vista previa no sale del servicio | OK |
| III. Dinero | `Round2` en importes, `ValidMoney` antes de escribir; el importe lo calcula el servidor (R4); cada cifra de la vista previa declara qué es (importe, pagado, saldo, pagado de más) | OK |
| IV. Test-first | Unitarios table-driven de R4, R5 (lógica de la ventana), FR-017, FR-018, fecha y huella; integración de la migración (dos empresas), de cada criterio de aceptación, de la reversión de la vista previa y de aislamiento con `inTheThreeCases` | OK |
| IV. Migración tolerante | Solo agrega una columna nullable; la API vieja inserta sin ella | OK |
| V. Seguridad | `RequirePermission(expenses.edit)` y `RequirePermission(inventory.receive)` en el router; `arrivedOn` revalida `inventory.receive` en el handler (`domain.EditNeedsReceive`); parámetros inválidos se rechazan (decisión, `shortfall`, fecha); `preview` solo acepta `1`/ausente | OK |
| VI. YAGNI | Sin tabla de entregas, sin tabla de decisiones (van en movimientos y bitácora), sin pantalla de permisos | OK |
| VII. Comentarios / idioma | Identificadores en inglés; textos y comentarios en español | OK |
| VIII. Puertas | Ver abajo | OK |
| Restricciones del producto | Tableta 1024×600, controles ≥ 44 px, `Picker`/`DatePickerButton`, un solo estilo; textos para quien opera | OK (lo revisa `tablet-ui-reviewer`) |
| Roles híbridos | Permiso nuevo `expenses.edit` (capacidad, nombre estable en inglés), en `AllPermissions` → solo admin por omisión; ningún `RequireRole` nuevo | OK |

**Principio VIII — ¿se puede agregar después al mismo costo?**

- *Fecha de entrega*: **no**. Un día de llegada que no se registra hoy no se recupera; por eso entra
  ahora como columna (barato) aunque los reportes por periodo todavía no la lean.
- *Más de un almacén por sucursal* (#65): los movimientos nuevos son por sucursal como todos; la
  compensación es un movimiento más, no asume un solo lugar. Puerta abierta.
- *Más de una caja vendiendo*: el pago adicional usa el flujo existente; nada asume «la» caja.
- *Saber de quién es un cambio*: toda acción guarda `user_id` en el movimiento y actor en la
  bitácora.

## Pantallas y maquetas

Tres piezas tocan `web/`; la primera cambia de disposición y las otras dos son nuevas. Lo que sigue
describe la opción que eligió el dueño (tabla «Maquetas»).

1. **Detalle del gasto** (`ExpenseDetailDialog`, opción A «Modo edición»). Dos estados:
   - *Ver* (como hoy): chips de estado, una línea de encabezado (categoría · proveedor · día ·
     comentario), Importe y Pagado (y «Pagado de más» / «Falta pagar» cuando aplica), renglones con
     pedido / recibido / falta y un botón «Entregas (N)» que despliega su historial. Pie: «Anular»
     (solo pendientes, como hoy) · «Corregir lo que llegó» · «Recibir» · **«Editar gasto»** (con
     `expenses.edit`). Aparece también en un gasto sin renglones (el caso del 1-nov).
   - *Editar*: el encabezado se vuelve campos en línea: proveedor, categoría (`Picker`) y fecha
     (`DatePickerButton`) en una fila de tres columnas, y el comentario (`Input`) debajo (~120 px en
     total, para que queden ≥ 4 renglones visibles), cada renglón no excluido muestra «✕ Quitar» (y
     «Deshacer» una vez marcado), los nuevos se ven en verde con «Deshacer», y «+ Agregar renglón»
     abre `AddLineSheet`. Pie: «N cambio(s) sin confirmar» · «Cancelar» · «Revisar cambios» → vista
     previa.
   Alto de 1024×600: título 56 px + pie 72 px; el resto es la lista con scroll interno.
2. **Hoja de entrega** (nueva, `DeliverySheet`, opción B «Todo precargado» + «No llegó»): encabezado
   con «Llegó el [Hoy 📅]» (`DatePickerButton`) y «Fechas por renglón» (avanzado: un
   `DatePickerButton` por renglón). Cada renglón pendiente trae **lo que falta ya puesto** en un
   −/+ cuyo número es un **campo editable que, al tocarlo, abre el teclado numérico** (pedido del
   dueño el 1-nov). Por la revisión de tableta (U1) ese teclado es `NumericKeypad`, el teclado en
   pantalla del repo (teclas de 56 px, no se mueve; la casilla va con `inputMode="none"`): al tocar
   la casilla, el teclado sustituye al pie de la hoja con «Listo», y el del sistema no se abre encima
   de «Ver efectos». Un botón «No llegó» deja el renglón en 0 (con 0, el botón cambia a «Llegó
   todo»). Si queda menos de lo que falta: «⚠ Faltarán N» en la misma línea que el nombre (14 px,
   600), para que todo renglón mida 60 px. El cuerpo es `flex="1" minH={0}` con scroll (patrón de
   `Page fill`). Pie: «N de M renglones» (o «Nada llegó todavía» con todo en 0) · «Cancelar» · «Ver
   efectos» → vista previa. Se abre desde el detalle, desde el atajo
   «Recibir» de la lista de gastos y desde «Por recibir».
3. **Vista previa** (nueva, `EditPreviewSheet`, opción B «Tabla antes/después», componente único):
   hoja inferior con una tabla Qué / Antes / Después / Cambio; los grupos Inventario, Dinero y Costos
   son filas de encabezado; el cambio lleva signo y color (verde sube, rojo baja, ámbar aviso). Una
   decisión ocupa una fila extra justo debajo de la fila afectada, con fondo ámbar y sus opciones como
   botones de 44 px en una sola línea (pregunta a la izquierda, opciones a la derecha): «Ajustar el
   inventario» / «No mover»; «Registrar pago» / «Dejar pendiente»; «Sigue pendiente de entrega» /
   «Ajustar el importe a lo recibido». Dentro de cada grupo, las filas con decisión van primero; el
   encabezado de columnas queda fijo (`position: sticky`). Versión compacta sin avisos: la misma tabla
   con solo las filas que cambian. Pie: «⚠ Falta decidir N» (al tocarlo, desplaza a la siguiente
   decisión pendiente) o «Listo para confirmar» · «Volver» · «Confirmar» (deshabilitado hasta
   decidir). Todo sale de los componentes del repo (`DrawerRoot` con `maxH="80dvh"`, `Button`,
   `Badge`) y de los tokens del tema; el ámbar nunca es la única señal (⚠ y texto). Texto para quien
   opera: «No se movió el inventario», nunca «compensación».

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Detalle del gasto | [maqueta](https://claude.ai/artifact/M62VS1Mk4LDfJ5YpJQMXgA) | A · Modo edición | Ver y cambiar separados: nada se marca para quitar por accidente al revisar |
| Hoja de entrega | [maqueta](https://claude.ai/artifact/C9d679P9wUAAnELefon723) | B + «No llegó» | El pedido entero sale sin toques; «No llegó» deja un renglón en 0 de un toque; la cantidad abre el teclado |
| Vista previa | [maqueta](https://claude.ai/artifact/E31v28Wwqxx9o2NY4heeKF) | B · Tabla antes/después | Comparar cifras alineadas por columna |

## Project Structure

### Documentation (this feature)

```text
specs/053-editar-gasto/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/edit-expense.md
└── tasks.md            # /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── migrations/0104_stock_movement_effective_on.sql
├── queries/purchases.sql                  # effective_on en InsertLineMovement, historial por
│                                          # renglón, conteo posterior, compensación, encabezado,
│                                          # importe y estado
├── internal/domain/
│   ├── expense_edit.go (+_test.go)        # R4, FR-017/018, fechas, decisiones, huella
│   ├── permission.go (+_test.go)          # expenses.edit
│   └── audit.go                           # expense.edited
├── internal/app/
│   ├── expense_edit.go                    # editExpense / deliverExpense: motor + vista previa
│   ├── purchases.go                       # applyStockLines con fecha por renglón; reverso por entrega
│   ├── purchases_receive.go               # effective_on = hoy en las rutas viejas
│   └── expenses.go                        # detalle: historial de entregas por renglón
├── internal/httpapi/
│   ├── handlers_expense_edit.go           # POST /expenses/{id}/edit, /deliveries
│   └── router.go
└── internal/integration/expense_edit_test.go (+ migración 0104)

web/src/
├── api/backoffice.ts                      # editExpense, deliverExpense, tipos de la vista previa
├── app/permissions.ts                     # expenses.edit
├── shared/audit/labels.ts                 # expense.edited
└── features/backoffice/
    ├── ExpenseDetailDialog.tsx            # nueva disposición (maqueta elegida)
    ├── DeliverySheet.tsx (+test)          # hoja de entrega
    ├── EditPreviewSheet.tsx (+test)       # vista previa única
    ├── AddLineSheet.tsx                   # agregar renglón (reusa el formulario de ArrivedItemSheet)
    ├── ExpensesPage.tsx                   # atajo «Recibir»
    └── PendingReceiptsTab.tsx             # «Recibir» abre DeliverySheet
web/e2e/editar-gasto.spec.ts               # criterios 1, 2, 4 y 5 a 1024×600; vista previa con
                                           # 2 decisiones: «Confirmar» y «Falta decidir» visibles
```

**Structure Decision**: monorepo existente; nada fuera de `server/` y `web/`. Documentación viva:
un párrafo en `AGENTS.md` (§2, junto a las specs 047-050) con lo que no se ve: `effective_on`, la
compensación de «No mover» y la vista previa revertida.

## Revisión de arquitectura (2026-11-01)

`db-architect` y `tablet-ui-reviewer` sobre este plan: cambios requeridos, todos aplicados aquí, en
research.md y en data-model.md.

| Hallazgo | Corrección |
|---|---|
| A1 · Un gasto pagado que vuelve a pendiente se podría anular con sus pagos vivos | `CanCancelExpense` + `CancelExpense` exigen que no haya pagos (409); test |
| A2 · La compensación tomaría la sucursal por trigger (EGB01 con dos) | `branch_id` explícito, el del conteo (R6) |
| M1 · Conteo posterior sin sucursal | Solo conteos de `current_branch_id()` (R5); test con dos sucursales |
| M2 · Borrar tras «Corregir lo que llegó» emitía una compra positiva | `domain.PlanDeliveryReversal` netea los días; un borrado nunca suma (R7) |
| M3 · `effective_on` nula se reinterpretaría si cambia la zona | Trigger que la llena en cada `compra` (y en la compensación) con la zona vigente de la empresa + relleno de lo existente en la 0104 (R3) |
| M4 · El Down tiraba fechas de entrega | El Down se niega si alguna fecha difiere del día de `created_at` |
| M5 · Una `pagada` sin pagos se degradaría | El estado se recalcula solo si cambió el importe y nunca degrada una pagada sin pagos (R4). Medido sobre el respaldo de producción del 1-nov: 80 gastos, 0 descuadrados |
| U1 · El teclado del sistema tapa «Ver efectos» | `NumericKeypad` en lugar del pie |
| U2 · La vista previa con 2 decisiones no cabe | «Falta decidir N» que desplaza, encabezado fijo, decisión en una línea, decisiones primero; e2e |
| U3-U6 · Alto de la hoja, encabezado en línea, «Nada llegó todavía», color como única señal | Descritos arriba en «Pantallas y maquetas» |

## Complexity Tracking

| Decisión | Por qué hace falta | Alternativa más simple descartada |
|---|---|---|
| Vista previa por transacción revertida | FR-025/SC-002: lo confirmado = lo visto, incluido costo y triggers | Calcular aparte: dos reglas que divergen |
| Movimiento de compensación para «No mover» | Conserva el invariante de lo aplicado y deja el porqué en el libro | No insertar nada: el siguiente plan lo «corregiría» |
| Dos rutas con un motor | Dos permisos distintos (FR-031) | Una ruta que decide permisos por cuerpo: el router no podría gatear |
