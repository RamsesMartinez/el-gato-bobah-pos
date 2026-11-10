# Implementation Plan: Escrituras del servidor MCP

**Branch**: `073-mcp-escrituras` | **Date**: 2026-11-10 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/073-mcp-escrituras/spec.md`

## Summary

Dieciséis herramientas nuevas sobre la puerta para agentes de la 072 (nueve escrituras, siete
lecturas): capturar y recibir compras, abrir/capturar/cerrar/descartar conteos, cancelar pedidos,
abrir y cerrar caja. Todas despachan en proceso a la **misma ruta que la pantalla**, con la
maquinaria existente (alcance, permiso, vista previa, idempotencia, bitácora). Lo nuevo fuera de la
capa MCP es poco y está acotado:

1. **Migración 0112**: el check de `api_tokens.scopes` admite `write:purchases` y `write:orders`.
2. **Una ruta de solo lectura** para la vista previa de una compra:
   `POST /api/v1/expenses/purchase-preview` corre la captura completa de un gasto con mercancía en una
   transacción que **se revierte** y devuelve, por insumo, el costo vigente antes y el que dejaría la
   compra. Es la misma función que guarda la compra: el costo de la vista previa no es una segunda
   copia de la regla (R2).
3. **Dos piezas puras en `domain`**: qué falta declarar para cerrar una caja por MCP (D4-c del
   spec) y el resumen de cancelación de un pedido (qué sale del corte).
4. **La pantalla de Integraciones** ofrece los dos alcances nuevos y dice qué abarca cada uno.

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: las de la 072 (SDK de MCP v1.8.0); ninguna nueva.

**Storage**: PostgreSQL — solo cambia un check (`api_tokens_scopes`). Ninguna tabla nueva.

**Testing**: unitarios en `domain` (tabla), `httpapi` (despachador con rutas de dos parámetros,
lote de conteo), integración bajo `appRoleStore` contra Postgres real (cada herramienta: alcance,
permiso, vista previa, idempotencia, aislamiento en los tres casos), test de la migración 0112, vitest
de Integraciones, y la prueba con un cliente MCP real contra `make start`.

**Target Platform**: la API en la VM; la pantalla en tableta a 1024×600.

**Project Type**: web-service + web-app (monorepo existente).

**Performance Goals**: una llamada cuesta lo de la petición de la pantalla más lo de la 072. El lote
de conteo son hasta 50 peticiones internas en una llamada (lo mismo que 50 toques en la tableta).

**Constraints**: ninguna herramienta puede más que la pantalla; el arqueo ciego no se rompe por un
camino nuevo; la migración convive con la API anterior.

**Scale/Scope**: 16 herramientas nuevas (49 en total); 2 alcances nuevos; 1 ruta nueva de solo
lectura.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple | Estado |
| --- | --- | --- |
| I. Layering | `domain/apitoken.go` (alcances y catálogo), `domain/mcp_writes.go` (qué falta declarar al cerrar, resumen de cancelación: puro). `app/expenses.go` (`PreviewPurchase`: la captura en una transacción revertida). `httpapi/handlers_backoffice.go` (handler fino de la vista previa), `httpapi/mcp_tools_*.go` (herramientas). Ninguna herramienta toca la base | OK |
| II. Errores | Sentinels de `domain`; los códigos nuevos que el agente tiene que saber corregir (`OPENING_REASON_REQUIRED`, `TIPS_DECISION_REQUIRED`, `REFUND_FOLIO_REQUIRED`, `TERMINAL_COUNT_REQUIRED`, `CLOCK_IN_REQUIRED`, `OPENER_NOT_CLOCKED_OUT`, `NEEDS_CONFIRM`, `INGREDIENT_MERGED`, `OPEN_ORDERS`, `UNPAID_ORDERS`) entran a `toolFixByCode` con su corrección en palabras, probados en tabla | OK |
| III. Dinero | La capa MCP no hace aritmética de dinero: lo dictado viaja como texto al handler, que valida y redondea. El costo de la vista previa lo calcula la misma función que guarda la compra (R2) | OK |
| IV. Test-first | Cada herramienta con su test de integración antes del código; borde por borde (ver «Casos de borde» abajo). Migración 0112 con su test sobre base restaurada con dos empresas | OK |
| IV. Aislamiento | La ruta nueva lee tablas de empresa: test bajo `appRoleStore` con `inTheThreeCases`. Las herramientas: un token de otra empresa no ve ni toca compra, conteo, pedido ni caja ajenos (por cada una) | OK |
| IV. Migración tolera la anterior | Solo amplía un check. La API anterior lee los alcances nuevos como desconocidos y no los usa; su pantalla muestra una etiqueta vacía en un token que los tenga (cosmético, minutos). El Down se niega si algún token ya usa un alcance nuevo y lleva su renglón en `guardedDownCleanup` | OK |
| V. Seguridad | Ver «Seguridad». `security-auditor` en la revisión de código | OK, a revisar |
| VI. YAGNI | Sin pesar con empaque en el conteo, sin renglones de compra que no van al almacén, sin traspasos ni propinas. Sin tabla nueva: la vista previa del costo no se guarda | OK |
| VII. Comentarios / idioma | Identificadores, herramientas y alcances en inglés; descripciones y errores en español | OK |
| VIII. Puertas | «Más de una caja vendiendo a la vez»: las herramientas siempre reciben la caja explícita, nunca «la» abierta. «Más de un almacén por sucursal»: el conteo es el de la sucursal, igual que la pantalla; no se agrega ningún supuesto nuevo. «Saber de quién es un pedido»: cancelar deja el token en el evento. Ninguna se cierra | OK |
| Tableta | Integraciones cambia de disposición (seis alcances de escritura en vez de cuatro): maqueta elegida abajo | OK |

## Seguridad (principio V)

| Control | Diseño | Prueba |
| --- | --- | --- |
| Alcance nuevo | `write:purchases` y `write:orders` en `AllAPIScopes` y en el check; un token viejo no los tiene | `TestScopeAllows` (tabla ampliada), `TestMCPNewToolsNeedTheirScope` |
| Permiso | Cada herramienta declara el permiso de su ruta; `TestEveryToolMatchesItsRouteGate` ahora reemplaza **todos** los parámetros de ruta (`{ingredientId}`, `{productId}`) y cubre las 16 | ampliado |
| Despachador | Rutas con dos parámetros enteros (conteo); la lista blanca crece con las rutas nuevas y nada más (`/orders/{id}/pay`, `/refund`, `/cash-sessions/transfer`, `/tips/payouts` siguen rechazadas) | `TestDispatcherRefusesRoutesOutsideCatalog` ampliado |
| Arqueo ciego | Las herramientas de caja solo leen `GET /cash-sessions/current` (filtrado), `/attendance` y `/tips`; nunca `/cash-sessions/{id}` del turno abierto con permiso de configurar ni el resumen de ventas | `TestMCPCashCloseKeepsTheBlindCount` recorre las respuestas de `cash_session_current`, `cash_session_close` (vista previa) y `cash_movement_create` (vista previa) buscando `expected`, `difference` e ingresos no nulos |
| Cierre sin cifra | `domain.MissingCloseDeclarations` decide con el turno filtrado qué falta; se rechaza antes de despachar | `TestMissingCloseDeclarations` (domain), `TestMCPCloseRejectsAMissingFigure` |
| Vista previa de compra | Transacción revertida: ni gasto, ni movimiento, ni costo, ni evento, ni aprendizaje quedan; toma los insumos `nowait` como la captura (una venta en curso = «vuelve a intentar», sin esperar) | `TestPurchasePreviewLeavesNothingBehind`, `TestPurchasePreviewIsolation` (tres casos) |
| Conteo a ciegas | `stock_count_capture` devuelve solo lo capturado; `NEEDS_CONFIRM` llega sin la referencia | `TestMCPCountCaptureIsBlind` |
| Cancelar cobrado | Sin `refund: true`, el handler ya lo rechaza; con tarjeta, exige folio. La herramienta no inventa ni el folio ni la confirmación | `TestMCPOrderCancel` |
| Identidad | Quien abre/cierra/cancela es la persona del token; asistencia sobre ella | `TestMCPCashOpenNeedsClockIn` |
| Lote | Hasta 50 renglones; el lote cuenta como **una** llamada contra el límite por token y despacha cada renglón con el `rateLimitUser` de la ruta si lo tiene | `TestMCPCountCaptureBatch` |

### Ajustes de la revisión de arquitectura (2026-11-10)

Todos entran; cada uno con su prueba.

| # | Hallazgo (revisor) | Decisión |
| --- | --- | --- |
| A1 | Los triggers diferidos de la 0104 (insumo unido) corren en el commit y un rollback nunca los dispara: la vista previa diría «bien» y la compra fallaría (db-architect) | La transacción de la vista previa corre `set constraints all immediate` justo antes del centinela. Test: una compra a un insumo unido falla igual en vista previa y al aplicar |
| A2 | Un renglón con artículo nuevo (`newArticle`) devolvería el id de una fila revertida (db-architect) | La ruta de vista previa **rechaza** `newArticle` (400 «la vista previa no da de alta artículos»); la herramienta MCP nunca lo manda (FR-005: ids explícitos). Test |
| A3 | Costo por renglón engaña con el mismo insumo en dos renglones (db-architect) | `costBefore`/`costAfter` son **por insumo**: antes de aplicar la mercancía y después de la compra completa; el contrato agrupa `costs[]` por insumo aparte de `lines[]`. Test con dos renglones del mismo insumo y promedio ponderado |
| A4 | La vista previa toma candados reales (insumos, existencias, caja, versión del catálogo) (db-architect) | `set local lock_timeout = '2s'` en su transacción; la ruta lleva `rateLimitUser` como la captura y la cuenta el límite por token de la 072. La extracción `createExpenseTx` incluye la validación previa (`openShiftDay`, `validatePayments`, `checkPaymentsCover`), no solo el cuerpo de la transacción. Reusa `errPreviewRollback` |
| A5 | Renglón de `guardedDownCleanup` de la 0112 (db-architect) | `update api_tokens set scopes = array_remove(array_remove(scopes, 'write:purchases'), 'write:orders')`; el Down restaura la lista de cinco y la cardinalidad 1–5 |
| A6 | Textos de Pedidos y Compras (tablet-ui-reviewer) | Ver «Pantallas y maquetas» |
| A7 | La consecuencia de D1 solo se ve al crear un token (tablet-ui-reviewer) | Se dice en el reporte de la subida y en el comentario del issue |

## Casos de borde (antes del código)

| Forma de fallar | Familia | Test |
| --- | --- | --- |
| La vista previa de compra deja un movimiento, un costo o el aprendizaje del proveedor | estado que no sobrevive | `TestPurchasePreviewLeavesNothingBehind` |
| La vista previa reporta el costo con otra regla que la compra (último vs. promedio, compra tardía) | hermano que no se movió | mismo test, con los dos métodos y una compra más reciente |
| Un cierre «ayudado» con lo que el sistema espera | camino nuevo que se salta el control | `TestMCPCloseRejectsAMissingFigure`, `TestMCPCashCloseKeepsTheBlindCount` |
| Un método en cero dictado se toma como «no dictado» | valor vacío que significa algo | `TestMissingCloseDeclarations`: `"0.00"` presente cuenta como declarado |
| El lote de conteo falla a la mitad y el reintento duplica | estado que no sobrevive | `TestMCPCountCaptureBatch` (reintento con la misma clave) |
| Un renglón con insumo y sin empaque/unidad se aplica con la unidad base inventada | valor vacío | `TestMCPPurchaseNeedsExplicitIds` |
| El despachador acepta `{ingredientId}` con un texto | camino nuevo | `TestDispatcherRefusesMissingParams` ampliado |
| Cancelar repone inventario distinto que la pantalla | hermano | `TestMCPOrderCancel` compara movimientos con el mismo caso por la ruta |
| Un token con `write:inventory` de antes de la subida cierra un conteo | consecuencia de D1 | documentado; `TestMCPToolsListFollowsScopeAndPermission` ampliado lo hace explícito |

## Pantallas y maquetas

**Integraciones** (`/integraciones`), hoja «Crear token»: los alcances de escritura pasan de cuatro a
seis. **Opción A**: las mismas casillas de la hoja actual, en dos columnas y tres renglones (58 px
cada una), con su línea de qué permite: «Caja: entradas y salidas de efectivo; abrir y cerrar caja»,
«Almacén: mermas, ajustes de existencia y conteos», «Compras: capturar compras con su mercancía y
recibir las que están por recibir», «Pedidos: cancelar pedidos, también cobrados (devuelve el
dinero)» (FR-002; textos ajustados por el revisor de tableta: el de Pedidos repetía la etiqueta y no
avisaba que mueve dinero ya cobrado). `LABEL` gana «Compras» y «Pedidos»; sin respaldo que muestre el
identificador crudo. Medido con la pantalla real a 1024×600:
las seis casillas y «Crear token» a la vista sin desplazar; la hoja crece 42 px. La tarjeta del token
muestra las etiquetas nuevas con el mismo `Badge`. Sin pantallas nuevas ni componentes nuevos.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Integraciones › Crear token | [Maquetas de Integraciones](https://claude.ai/artifact/UqrTAd7D2v7sK9FxVCwcXS) | A | 26/30 de un evaluador sin contexto (B 23, C 14): única que dice en una línea que Caja incluye abrir y cerrar y Almacén los conteos, con casillas de 58 px; B las parte en 78 px y C no explica qué permite cada una |

## Project Structure

### Documentation (this feature)

```text
specs/073-mcp-escrituras/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── mcp-tools.md       # las 16 herramientas: argumentos, ruta, permiso, alcance
│   └── api.md             # POST /expenses/purchase-preview
└── tasks.md
```

### Source Code (repository root)

```text
server/
├── migrations/0112_api_token_write_scopes.sql
├── internal/domain/apitoken.go (+_test)          # alcances, catálogo, pistas de error nuevas
├── internal/domain/mcp_writes.go (+_test)        # MissingCloseDeclarations, CancelSummary
├── internal/app/expenses.go                      # createExpenseTx + PreviewPurchase (tx revertida)
├── internal/httpapi/handlers_backoffice.go       # PurchasePreview
├── internal/httpapi/router.go                    # POST /expenses/purchase-preview
├── internal/httpapi/mcp_tools.go                 # rutas y lecturas nuevas
├── internal/httpapi/mcp_tools_write.go           # escrituras nuevas (o mcp_tools_write2.go)
├── internal/httpapi/testdata/route_gates.golden
└── internal/integration/mcp_writes_test.go, purchase_preview_test.go, migration_0112_test.go
web/src/
├── api/integrations.ts                           # TokenScope
└── features/admin/IntegrationsPage.tsx (+test)
docs/mecanica/personal-y-permisos.md              # la puerta para agentes: qué agrega la 073
```

**Structure Decision**: monorepo existente; las herramientas nuevas viven junto a las de la 072.

## Decisiones (dudas resueltas con la mejor recomendación)

1. **Vista previa de compra = la captura revertida** (R2), no un cálculo aparte.
2. **Lote de conteo despachado renglón por renglón**: reutiliza `PUT …/lines/{ingredientId}` y
   `…/product-lines/{productId}`; no hay ruta de lote nueva en la API (la pantalla no la necesita).
   La idempotencia trata un fallo a la mitad como 4xx (la clave se libera): capturar fija la cifra,
   así que repetir no duplica.
3. **El cierre de caja valida lo declarado en la capa MCP** con una función pura sobre el turno
   filtrado, antes de despachar; la ruta sigue siendo la barrera de todo lo demás.
4. **Compra por MCP llega recibida por omisión** (`received_on` = hoy); `pending: true` la deja por
   recibir.
5. **Herramientas de lectura de apoyo** (`articles_search`, `units_list`, `presentations_list`,
   `pending_receipts_list`, `stock_count_current`, `cash_denominations_list`,
   `cash_session_attendance`): sin ellas el agente no tiene de dónde sacar los ids explícitos que
   piden las escrituras.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Ruta nueva de solo lectura que corre la captura en una transacción revertida | El costo de la vista previa tiene que ser el que dejará la compra, con todas sus validaciones | Calcularlo en la capa MCP duplicaba la regla de costeo, la conversión de unidades y la regla de compras tardías; la primera que cambie deja a la vista previa mintiendo |
