# Implementation Plan: Una sola puerta para cobrar — la cuenta vive en el servidor

**Branch**: `030-una-sola-puerta` | **Date**: 2026-10-08 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/030-una-sola-puerta/spec.md` (decisiones D-1…D-12, no
se reabren).

## Summary

La cuenta que se está capturando deja de vivir en la tableta (`egb:ticket:v2`) y pasa al servidor
desde el primer producto, en una entidad propia (`order_drafts`) que **nunca** es un pedido: ninguna
consulta de dinero la puede contar. Al mandarla a cocina se convierte en pedido por el camino de hoy
(`Create` / `AddLines`), en la misma transacción que la marca enviada. Una sola lista del servidor
(`GET /pos/accounts`) une cuentas en captura y pedidos no cerrados con un estado derivado en `domain`,
y en el POS reemplaza a las pestañas locales y al botón naranja. «Cobrar» siempre envía primero y abre
la hoja con los tres modos sobre el pedido. Diseño en [research.md](./research.md), esquema en
[data-model.md](./data-model.md), frontera entre backend y frontend en
[contracts/api.md](./contracts/api.md).

## Technical Context

**Language/Version**: Go 1.27 (server) · TypeScript / React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose (embebidas) · Vite, Chakra UI v3, TanStack Query,
Zustand. Ninguna dependencia nueva.

**Storage**: PostgreSQL 16 con RLS por empresa. Migración `0082_order_drafts.sql` (solo tablas
nuevas). Redis no interviene.

**Testing**: `go test` (unitarios de `domain`), integración con tag `integration` contra `egb030-pg`
(:5502) bajo `appRoleStore` + `inTheThreeCases`; vitest; Playwright contra el ambiente de pruebas a
1024×600.

**Target Platform**: tabletas de 7–10" (~1024×600), Chrome/PWA; API en e2-micro.

**Project Type**: web (monorepo `server/` + `web/`).

**Performance Goals**: `GET /pos/accounts` ≤ 30 ms con 30 mil pedidos (se pide cada 30 s por
tableta y en cada evento); agregar un producto ≤ 150 ms p95 en local (es un toque).

**Constraints**: capturar exige conexión (D-4); 44 px mínimo; cero `<select>` nativo; cero diálogos
del sistema; la fila de la pantalla Vender ya estaba medida en el límite (667.6 px pedidos de 612.6).

**Scale/Scope**: ~40–60 pedidos/día, 2–3 tabletas, ≤ ~15 cuentas vivas a la vez.

## Constitution Check

*GATE: antes de Phase 0 y otra vez tras el diseño.*

| Principio | Cómo se cumple | Estado |
|---|---|---|
| I. Layering | Tabla «Dónde aterriza cada pieza». Reglas puras (estado de cuenta, fusión, 12 h, recibir renglones, nombres) en `domain`; transacciones en `app`; handlers finos; SQL solo sqlc | ✅ |
| II. Errores | Sentinels nuevos en `domain` envolviendo `ErrConflict`/`ErrValidation`; mapeo solo en `httpapi.Error` | ✅ |
| III. Dinero | El total de «lo que se debe» de la lista suma solo pedidos: una cuenta en captura no es deuda (test que lo nombra). El borrador no guarda precios; se reprecia en cada lectura y al enviar con `BuildOrder`. `outstanding` de la lista y su suma salen del mismo predicado. El borrador no entra a ningún total de dinero (FR-002), con test por consulta | ✅ |
| IV. Test-first | Tasks ordenadas test → código; bordes enumerados abajo; migración con su test sobre respaldo de dos empresas; tablas nuevas bajo `TestEveryCompanyTableIsIsolated` y servicios con `inTheThreeCases` | ✅ |
| V. Seguridad | Sin rol nuevo (mismo gate que crear pedido). El descuento en la cuenta lleva el mismo tope por usuario y el mismo evento de seguridad que `PUT /orders/{id}/discount`, y su autor viaja al pedido (`discount_set_by`): el camino nuevo no se salta el control viejo. RLS + FKs compuestas (los chequeos de FK saltan RLS). Topes en la frontera (`MaxDraftLines`, `ValidQty`, largo de nota/cliente, 20 cuentas por import). Parámetros inválidos → 400, nunca default | ✅ |
| VI. YAGNI | Sin job (barrido perezoso), sin `branch_id` en el borrador, sin endpoint enviar-y-cobrar, sin snapshot de precio. Un `// ponytail:` en la ventana de 90 días | ✅ |
| VII. Comentarios / idioma | Identificadores nuevos en inglés (`order_drafts`, `DraftsService`, `AccountState`); textos y comentarios en español | ✅ |
| VIII. Puertas | Respuesta por escrito en research R-16. `opened_by` desde el nacimiento (puerta «Saber de quién es un pedido»); descartadas conservadas. Ninguna puerta se cierra: nada asume una caja (el borrador no cuelga de turno) ni una sucursal | ✅ |
| Restricciones del producto | Offline aplazado: D-4 vuelve dependiente de la red **agregar un producto**, que hoy no lo es; está aceptado por escrito en la 013 (2026-09-08) y en D-4 | ✅ (aceptado) |

**Re-evaluación tras Phase 1**: sin violaciones. La única regresión deliberada (capturar sin red) es
D-4 y está firmada.

## Dónde aterriza cada pieza

| Pieza | Capa | Archivo |
|---|---|---|
| Tablas, RLS, grants, índices únicos parciales | migración | `server/migrations/0082_order_drafts.sql` |
| Consultas de borradores, lista de cuentas vivas, barrido, soltar nombre | sqlc | `server/queries/drafts.sql` (nuevo), `server/queries/orders.sql` (lista de pedidos vivos), `server/queries/folios.sql` (soltar con guarda de `taken_at`) |
| `AccountState`, `AccountGroup`, `MergeTarget`, `DraftExpired`, `ValidateDraftLine`, `CanReceiveLines`, `AvailableNames`, sentinels | domain | `server/internal/domain/draft.go`, `account.go`, `order.go` (reemplaza `PuedeRecibirLineas`), `folio.go`, `errors.go` |
| `DraftsService` (crear, agregar, cambiar, quitar, cabecera, descartar, enviar, importar, barrer, vista) | app | `server/internal/app/drafts.go` (nuevo) |
| `createInTx` / `addLinesInTx` extraídos; `resolverFolio` con nombre amarrado y nombres vivos | app | `server/internal/app/orders.go` |
| `AccountsService.Live` (lista unificada) y `liveAccounts` en la vista del turno | app | `server/internal/app/accounts.go` (nuevo), `backoffice.go` |
| Handlers, rutas, eventos `draft.updated`, códigos nuevos | httpapi | `handlers_drafts.go` (nuevo), `router.go`, `respond.go` |
| API cliente y tipos | web/api | `web/src/api/pos.ts`, `web/src/types/pos.ts` |
| Estado: selección persistida, consultas, mutaciones optimistas, sin conexión | web/stores + hooks | `web/src/stores/pos.ts` (reemplaza `stores/ticket.ts`), `features/pos/useCuenta.ts`, `useCuentasVivas.ts`, `useSinConexion.ts` |
| Migración única de `egb:ticket:v2` | web | `features/pos/subirCuentasViejas.ts` |
| Fila de cuentas, hoja «+N», ticket en tres secciones, pie, hoja de descartar, banner | web/features/pos | `FilaDeCuentas.tsx`, `TodasLasCuentasSheet.tsx`, `Ticket.tsx`, `DescartarCuentaSheet.tsx`, `AvisoSinConexion.tsx` |
| Hoja de confirmación y de motivo de la app | web/components | `ConfirmSheet.tsx`, `ReasonSheet.tsx` |
| «Abrir cuenta» en el tablero, cierre de caja con la lista | web/features | `orders/OrdersBoardPage.tsx`, `backoffice/CashPage.tsx` |

## Bordes → test (se escriben antes que el código)

Las cuatro familias de la constitución, aplicadas:

| Borde | Familia | Test |
|---|---|---|
| Dos tabletas abren cuenta a la vez y se proponen el mismo nombre | hermano / concurrencia | IT `TestTwoDraftsNeverShareAName` (dos goroutines, mismo nombre propuesto) |
| La bolsa se vacía mientras una cuenta tiene nombre; luego se descarta | estado que no sobrevive | IT `TestDiscardAfterBagRefillKeepsTheNewOwner` |
| `move_lines` crea un pedido destino y se lleva el nombre de una cuenta viva | camino nuevo salta control viejo | IT `TestMoveLinesSkipsLiveDraftNames` |
| Enviar se reintenta tras un timeout que sí entró | estado que no sobrevive | IT `TestSendIsIdempotent` (una comanda, un pedido) |
| Enviar mientras otra tableta agrega | concurrencia | IT `TestSendWhileAddingNeverLosesALine` (lo agregado entra al pedido o queda en una cuenta viva, nunca se pierde) |
| Pestaña vieja que ya se había enviado (D-12) | estado que no sobrevive | IT `TestImportDoesNotResendASentTab` |
| Pedido entregado **con saldo de $0.01** por redondeo | valor vacío que significa algo | unit `CanReceiveLines` usa `PedidoSaldado`, no `paid >= total` |
| Pedido de $0 (todo regalado) entregado | valor vacío | unit: saldado ⇒ cerrado, no aparece en la lista |
| Quitar el último renglón | valor vacío | IT: la cuenta queda vacía y viva con su nombre; descartar no pregunta (vitest) |
| «−» con versión vieja tras un reintento que sí se aplicó | estado | vitest: 409 con la cuenta ya en la cantidad pedida no avisa |
| Producto desactivado mientras la cuenta esperaba | hermano | IT: la vista lo marca `available=false`; enviar → 422 con su nombre |
| Cancelar un pedido que tiene «Nuevo» viva | hermano que no se movió | IT `TestCancelledOrderDiscardsItsNew` |
| Cliente sin turno abierto envía | control viejo | IT: 409 `NO_OPEN_REGISTER`, la cuenta intacta |
| Conexión reciclada / sin empresa | RLS (tres casos) | IT `TestDraftsInTheThreeCases` sobre cada consulta nueva |
| FK a producto de otra empresa | FK salta RLS | IT: `product_id` de la empresa B rechazado (llave compuesta) |
| `GET /pos/accounts` responde `items: null` | slice nil | IT sobre JSON crudo |
| `?cuenta=` inválido en la URL | parámetro de frontera | vitest: se ignora con aviso, no abre otra cuenta |
| F5 a media captura | estado | e2e: recargar conserva todo |
| Tableta suspendida pierde SSE | estado | vitest: el refresco de 30 s actualiza y el aviso sale |
| Cuenta que cruza un cierre de turno y choca con un nombre del turno nuevo | estado | IT `TestDraftAcrossShiftsKeepsItsAnimal` («Persa 2», nunca otro animal) |
| Vaciar la bolsa con cuentas vivas | estado | IT `TestBagRefillSkipsLiveDrafts` |
| Dos tabletas agregan el mismo producto a la vez | concurrencia | IT `TestConcurrentAddsMergeIntoOneLine` (un renglón, qty 2, posiciones únicas) |
| Opción de modificador borrada con la cuenta viva | hermano | IT: enviar → 422 nombrando el producto |
| Barrido y envío a la vez | concurrencia | IT: la cuenta queda enviada, no descartada |
| `olderDebts=maybe` | parámetro de frontera | IT HTTP: 400 |

## Diseño de pantalla (presupuesto 1024×600)

Cifras estimadas con el ancho medido antes (612.6 px para la fila con el panel abierto); la prueba
en navegador (`cabe-en-la-tableta.spec.ts`) las vuelve medidas.

- **Fila 2 de Vender**: `[FilaDeCuentas · flex] [Buscar] [precios 48] [lápiz 48]`. `PedidosEnCurso`
  y `TicketTabs` desaparecen; su ancho pasa a la fila de cuentas.
  - **Buscador**: `clamp(120px, 20%, 200px)` con el panel cerrado; con el panel abierto se pliega a un
    botón de 44 px que despliega el campo encima de la fila (el foco y el texto no cambian).
  - **Fichas de ancho fijo 120 px** × 44 px: nombre truncado arriba, estado corto en color y lo que
    falta abajo (Capturando gris · En cocina azul · Pagada·en cocina verde · Pago parcial ámbar ·
    Entregada·debe rojo). **Sin ✕** (el descarte vive en el ⋮ del ticket: acción destructiva lejos
    de lo que se toca todo el día).
  - `FilaDeCuentas` **mide** su ancho (`useContainerWidth`) y pinta solo fichas completas; después
    «+N» (44×44) y «+» cuenta nueva (44×44), siempre visibles. Nunca scroll horizontal escondido.
  - **Objetivo**: panel abierto ≥ 2 fichas (612.6 − 44 − 96 − 40 gaps − 88 ≈ 344 px → 2 de 120); panel
    cerrado (el arranque a 600 px de alto) ≥ 4 fichas. El e2e cuenta las fichas visibles en los dos.
  - **Orden**: la seleccionada, luego las que deben dinero, luego por antigüedad.
- **Hoja «+N»** (`TodasLasCuentasSheet`): hoja inferior `maxH="85dvh"`, grupos con su conteo
  (Capturando · En cocina · Entregadas que deben · De días anteriores; un grupo vacío no se pinta),
  renglones de 56 px con hora, antigüedad y lo que falta. Buscador solo con más de 8 cuentas (con el
  teclado abierto la lista se queda en ~150 px). Al abrirse pide `olderDebts=true`.
- **Ticket** (panel `clamp(300px,32%,380px)` o la píldora). Presupuesto estimado del panel a 600 px:
  cabecera 48 + totales 70 + pie 56 + motivo 20 ≈ 194 px de cromo → ~350 px de renglones.
  - **Nuevo · aún no va a cocina** va **primero**, con −/+ (renglón de 64 px).
  - **En cocina** y **Pagado** van debajo, compactos (40 px, sin −/+), y se pliegan solos con más de
    3 renglones («En cocina · 5 ›»).
  - **Objetivo**: con las tres secciones se ven ≥ 4 renglones (3 de «Nuevo» + encabezados plegados).
  - «En cocina» solo muestra la marca de entregado; **quitar** sale del ⋮ del renglón y abre la hoja de
    quitar existente (motivo, contador). «Pagado» lleva candado y no tiene ⋮.
  - En una cuenta en captura solo existe «Nuevo».
  - Cabecera con ⋮: cliente, canal, descuento, envío, **Descartar cuenta** (solo en captura; rojo,
    separado) y **Cancelar pedido** (enviada; permiso). El pie **solo** lleva totales y dos botones
    (caso 30).
- **Pie**: `Enviar N a cocina` (secundario) · `Cobrar $X` o, si hay algo en «Nuevo», **`Enviar y
  cobrar $X`** (primario; research R-5). Apagados con su motivo en una línea debajo cuando hay algo
  guardando o sin conexión. Si el envío falla, la hoja de cobro no se abre y el motivo queda ahí.
- **Descartar** (`DescartarCuentaSheet`): «¿Descartar la cuenta de Levkoy? Se pierden 2 productos
  ($74); no se ha mandado a cocina». Acción principal «Seguir capturando»; «Descartar» rojo y
  separado. Cuenta vacía → se descarta sin hoja.
- **Sin conexión**: `AvisoSinConexion` en la franja del encabezado (donde vive `AvisoDePlataforma`),
  **superpuesto**, sin empujar nada, visible con el panel abierto o cerrado. Los productos del menú
  se ven apagados con el motivo en el aviso.
- **Cambio en otra tableta**: toast con el nombre («Siamés se cobró en otra tableta»).
- **Textos**: nunca «borrador», «draft», «versión», «409». `?cuenta=` inválido → «Esa cuenta ya no
  existe».
- **Tablero**: «Abrir cuenta» en cada tarjeta → `/pos?pedido=<id>`.
- **Cierre de caja**: bloqueantes arriba con «Abrir»; las que no bloquean en una sección plegada
  «Cuentas pendientes (N)» con «Abrir» y «Descartar» (`minH="44px"`, separados; «Descartar» confirma
  con `ConfirmSheet`). «Cerrar caja» confirma con `ConfirmSheet`. La vista pide `olderDebts=true`.

## Orden de construcción y dos implementadores

El contrato ([contracts/api.md](./contracts/api.md)) se congela en la Fase 1 de tasks. Desde ahí:

- **Backend** (`server/`): dominio → migración → consultas → `DraftsService`/`AccountsService` →
  handlers → cambios en `Create`/`AddLines`/`resolverFolio`/cierre.
- **Frontend** (`web/`): contra mocks de `posApi` en vitest (como hoy) construye componentes y
  hooks; la integración real entra cuando el backend publica la fase de handlers.
- **Punto de encuentro**: la fase de verificación corre la suite e2e contra la imagen de la rama en el
  ambiente de pruebas. Ninguna tarea de frontend toca `server/` ni al revés.

## Project Structure

### Documentation (this feature)

```text
specs/030-una-sola-puerta/
├── plan.md · research.md · data-model.md · quickstart.md
├── contracts/api.md
└── tasks.md            # /speckit-tasks
```

### Source Code

```text
server/
├── migrations/0082_order_drafts.sql
├── queries/drafts.sql · orders.sql · folios.sql
└── internal/
    ├── domain/draft.go · account.go · order.go · folio.go · errors.go (+ _test.go)
    ├── app/drafts.go · accounts.go · orders.go · move_lines.go · backoffice.go
    ├── httpapi/handlers_drafts.go · router.go · respond.go
    └── integration/drafts_*_test.go · accounts_*_test.go · migration_order_drafts_test.go
web/
├── src/api/pos.ts · src/types/pos.ts
├── src/stores/pos.ts                      (reemplaza stores/ticket.ts)
├── src/components/ConfirmSheet.tsx · ReasonSheet.tsx
├── src/features/pos/ FilaDeCuentas · TodasLasCuentasSheet · Ticket · DescartarCuentaSheet ·
│   AvisoSinConexion · useCuenta · useCuentasVivas · useSinConexion · subirCuentasViejas · POSPage
├── src/features/orders/OrdersBoardPage.tsx · src/features/backoffice/CashPage.tsx · ExpensesPage.tsx
├── src/shared/CobrarSheet.tsx · shared/cobro/ModePicker.tsx
└── e2e/una-sola-puerta.spec.ts · cabe-en-la-tableta.spec.ts · limpiar-lo-que-cree.ts (+ las 6 que citan «Cuenta 1»)
```

**Structure Decision**: monorepo existente; sin paquetes nuevos.

## Revisión de arquitectura

Corrida el 2026-10-08 (`db-architect` + `tablet-ui-reviewer`). Veredicto: cambios requeridos, ya
aplicados en este plan, [research.md](./research.md) y [data-model.md](./data-model.md):

| Hallazgo | Corrección |
|---|---|
| FR-009 («de cualquier día») contra la ventana de 90 días | Dos alcances: la fila cada 30 s mira 90 días; `olderDebts=true` (hoja «+N» y cierre) mira todo (R-7) |
| FK a `products` con `on delete restrict` abortaría el borrado en cascada de una empresa | `no action`, como 0079 |
| `opened_by` sin acción; `discarded_by` sin FK | Las dos compuestas a `users` con `no action` |
| FK a `delivery_platforms` en el orden equivocado | `(delivery_platform_id, company_id) → (id, company_id)`; la migración no crea índices en tablas existentes |
| `position` y la fusión se pisan entre tabletas | `for update` sobre la cuenta antes de toda escritura de renglones |
| Carrera cuenta contra pedido por el nombre | Nombres vivos leídos dentro de la tx del pedido; test de cruce de turno y de vaciar la bolsa con cuentas vivas |
| `discard_reason` sin check; `order_draft_adds` sin índice por cuenta; crecimiento sin techo escrito | Check, índice y techo en la migración |
| Opción de modificador borrada con la cuenta viva | 422 con el producto, nunca 500 (test) |
| Barrido dentro de un GET pisando un envío | `where status = 'capturando'` en el `update` |
| R-16 decía que la sucursal no existe; `opened_by` parecía resolver «de quién es» | Corregido |
| La fila queda en 1–2 fichas con el panel abierto | Fichas de 120 px fijas, buscador plegable, objetivo ≥ 2 / ≥ 4 medido en e2e |
| «Cobrar» envía a cocina sin decirlo | «Enviar y cobrar $X» cuando hay algo nuevo |
| Banner sin conexión invisible con el panel oculto / roba alto | En la franja del encabezado, superpuesto |
| Ticket de tres secciones sin presupuesto | «Nuevo» primero; «En cocina»/«Pagado» compactos y plegables; objetivo ≥ 4 renglones |
| ✕ de 32 px en la ficha y bote junto a la marca de entregado | Descartar al ⋮ del ticket; quitar al ⋮ del renglón |
| Cierre de caja largo con botones `xs` | Sección plegada, 44 px, `ConfirmSheet` |

## Complexity Tracking

| Violación | Por qué | Alternativa más simple descartada |
|---|---|---|
| Tres tablas para una entidad | `order_draft_adds` es la única forma de que el «+» sea idempotente y conmutativo a la vez (D-5 exige sumar) | Versión única por cuenta: dos tabletas en hora pico chocarían en cada toque |

## Riesgos que el plan acepta

- **Capturar exige red** (D-4). Un wifi intermitente se siente en cada toque. Mitigación: renglón
  optimista «guardando», reintento, banner.
- **Fiados de más de 90 días** no están en la fila ni en su «+N»; aparecen al abrir la hoja y en el
  cierre (research R-7).
- **Cobrar envía antes de abrir la hoja** (research R-5): cerrar la hoja sin cobrar deja la cuenta en
  cocina.
- **e2e se reescribe a la vez que el flujo**: 6 specs y 2 vitest citan «Cuenta 1» / «Pedidos por
  cobrar» y se reescriben en esta feature; mientras tanto, la suite e2e de `develop` no aplica a esta
  rama.
