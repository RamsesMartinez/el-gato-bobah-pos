# Research: Rediseño del módulo Caja

Hechos verificados en el código de `origin/develop` @1c31b551 antes de diseñar. Rutas relativas a
`server/` o `web/src/`.

## R1. Una sola regla de diferencia

**Hoy, tres cálculos distintos:**

| Dónde | Cómo | Resultado con el cajón $120 corto |
| --- | --- | --- |
| Histórico, `ListSessions` (`queries/cash.sql:149-179`) | SQL: Σ `register_session_totals.difference` + `session_cash_counts.difference` del cierre | −$120 |
| Detalle, `SessionDetailFor` (`app/backoffice.go:2511`) | No da total: el web suma difs por método + `drawerDifference` (`CashPage.tsx:98-103`) | −$120 |
| Diálogo «Caja cerrada» (`CashPage.tsx:1283-1306`) | `TotalsTable` sin `withTotalRow` ni `drawerDifference` | **$0** en efectivo, sin total |

Desde la 0067 el cierre fuerza `declared = expected` en los métodos que tocan el cajón
(`backoffice.go:1948-1955`), así que su renglón de conciliación es $0 por construcción y la
diferencia del cajón vive solo en `session_cash_counts`.

**Cortes viejos** (verificado por migración):
- Antes de la 0066: sin conteo de cierre; el efectivo se declaraba por método y su diferencia es real.
- Entre la 0066 y la 0067: conteo de cierre con `expected` nulo → `difference` nula.
- Desde la 0067: cajón en el conteo, efectivo de conciliación en $0.

**Decision**: `domain.CutDifference(methods []MethodDiff, drawer *Drawer, corrections) CutDiff` con
tres partes: `Drawer` (nil si no hay conteo de cierre con esperado), `OtherMethods` (Σ de los métodos
que **no** se cubren con el arqueo) y `Total`. Un método «se cubre con el arqueo» solo cuando hay
`Drawer`: así un corte anterior a la 0067 sigue contando su efectivo en la conciliación y uno
posterior no lo cuenta dos veces. La lista deja de sumar en SQL: `ListSessions` devuelve las piezas
(`methods_difference_uncovered`, `drawer_expected`, `drawer_counted`) y Go calcula con la misma
función que el detalle y el cierre. Las terminales (`session_terminal_counts`) son un desglose del
método tarjeta: no se suman aparte (hoy tampoco).

**Por qué en `domain` y no en SQL**: es la única forma de que lista, detalle, cierre y, en la fase 3,
la corrección usen literalmente el mismo código (constitución III, «el mismo predicado»).

**Alternatives**: una vista SQL compartida — no cubre el turno abierto (`liveDetail`) ni las
correcciones sin repetir la regla en Go.

## R2. El motivo de apertura

- Se escribe con `SetOpeningExtras` (`cash.sql:767-774`) desde `OpenSession`; **ninguna lectura lo
  devuelve**.
- `LastClosingCountOfRegister` (`cash.sql:742-751`) ya identifica el corte anterior de la misma caja
  (última `cerrada` por `closed_at desc, id desc`).

**Decision**: `SessionHistoryRow` y `SessionDetailView` agregan `openingReason`,
`openingReasonNote`, `previousSessionId` (el corte que esta apertura comparó) y
`explainedByNext {sessionId, reason, note}` (el turno siguiente de la misma caja, si abrió con
motivo). Las dos relaciones salen de un `left join lateral` por fila (`o.register_id =
s.register_id and o.id > s.id order by o.id limit 1`, y `o.id < s.id and o.status = 'cerrada'
order by o.id desc limit 1` para el anterior), el mismo criterio que `is_latest_of_register`.
**No** una ventana `lead/lag`: correría después del filtro de fechas y del `limit`, y la última fila
de cada página saldría sin su turno siguiente (revisión del `db-architect`). El turno siguiente
puede estar `abierta`: su motivo cuenta igual. Sin índice nuevo (`register_sessions` no tiene
`(register_id, id)`): son 1 a 3 turnos por día y caja, el mismo costo que ya paga
`is_latest_of_register`; `// ponytail:` en la consulta con el techo.

**MCP**: `cash_sessions_history` llama al mismo handler; `withoutCustomerText` (`mcp_tools.go:859`)
borra las llaves `note`/`notes`, no `openingReasonNote`. No hay herramienta de detalle: FR-009 se
cumple con el histórico, que ya trae una fila por corte.

## R3. El arqueo ciego

`canSee = PermCashConfigure` (`handlers_backoffice.go:502,518`); la bandera de empresa es
`business_settings.blind_cash_count`. Lo que se oculta hoy:
- Histórico: la diferencia del **último corte cerrado de cada caja** (`backoffice.go:2489-2496`).
- Detalle cerrado: fondo dejado y retiro (`ocultarElFondoDejado`, 265) y, en el último turno, el
  arqueo y los totales (`ocultarElArqueoDelUltimoCierre`, 291).
- Turno abierto: esperado y diferencia (`ocultarLoEsperado`, 1058).

**Decision**: la regla única corre **después** del ocultamiento, con las mismas piezas nulas: si el
esperado se ocultó, `CutDiff` sale con las partes nulas («—»), nunca $0. El motivo de apertura no se
oculta (P2). El enlace «explicada al abrir el turno N» se oculta junto con el arqueo del último
corte, porque su existencia revela que hubo diferencia.

## R4. Cerrar deshabilitado contra lo que se nombra

`disabled` (`CashPage.tsx:1246`) tiene **9** condiciones; el aviso (`cierreDeCaja.ts:150-162`) nombra
4 (cajón, métodos, terminales, fondo). Las otras: `faltaQuien`, `faltaDecidir` (propinas),
`fondoExcedeLoContado` (sin aviso en la lista), `pending` y `owing`. El issue dice 11 contando el
gate de asistencia y el spinner; el spec cuenta 11 y SC-004 se mide contra las 9 del botón más las
2 de `CierreConAsistencia`.

**Decision**: `cierreDeCaja.ts` exporta `pendientesDelCierre(estado): Pendiente[]` (cada uno con
clave, texto y acción) y el botón es `pendientes.length > 0`. Un test enumera las 11 entradas y
falla si una condición deshabilita sin pendiente. Fase 4.

## R5. Partir el servidor (fase 2)

`app/backoffice.go` tiene 3,137 líneas. Lo de caja, por responsabilidad (líneas de hoy):
- Turno: `OpenSession` 1192, `CurrentByRegister` 1493, `CloseSession` 1812, `vistaDelTurnoAbierto`,
  `sessionWithExpected`, `closeAttendance`, `SellingRegisterOpen`, `EstadoDeCaja`, y el conteo
  (`conteosDelTurno` 993, `guardarConteo` 1422, `saveTerminalCounts` 2100, `terminalCounts` 2132,
  `arqueoGuardado` 1149, los `ocultar*`).
- Movimientos: `RecordCashMovement` 2247, `CorrectCashOut` 2303, `Transfer` 2368.
- Reporte: `SessionHistory/For` 2460, `SessionDetail/For` 2504, `liveDetail` 2642, `SessionSalesPage`
  3045, `voidedPayments`, `sessionRefunds`, `sessionExpenses`.
- Cajas: `CashRegisters`, `AllCashRegisters`, `SetFixedFloat`, `CreateCashRegister`,
  `UpdateCashRegister`, `MoveSales`.

**Decision**: tres servicios nuevos con tipo concreto, no cinco: `CashShiftService` (turno y conteo:
el conteo solo existe dentro de abrir y cerrar, en su misma transacción; un `CashCountService`
aparte sería una interfaz de un solo consumidor), `CashMovementService` y `CashReportService`. Las
cajas se quedan donde están. El de correcciones (`CashAdjustmentService`) nace en la fase 3. Cada
uno recibe el `store` y lo que hoy usa del `BackofficeService` por campo, sin interfaces. Los
handlers cambian de receptor; las rutas y las respuestas no cambian.

Consultas: `cash.sql` (68) se parte en `cash_sessions.sql`, `cash_counts.sql`,
`cash_movements.sql` y `cash_history.sql`. sqlc genera un `.sql.go` por archivo con los mismos
nombres de función, así que el cambio en Go es cero.

**Red de pruebas** (FR-020), corre antes y después sin tocarla: integración `cash_test.go`,
`money_shift_test.go`, `salio_de_la_caja_test.go`, `arqueo_*`, `conteo_apertura`, `corte_*`,
`cash_history_pages_test.go`, `cash_attendance_test.go`; vitest de `features/backoffice/*Caja*`,
`CashPage.test.tsx` (89 casos); e2e `caja-propinas-tarjeta`, `caja-y-asistencia`, `arqueo-ciego`,
`contar-el-cajon`, `presupuesto-del-cierre`.

## R6. Partir la pantalla y las direcciones (fase 2)

`CashPage.tsx` (2,038 líneas): `CashPage` 543 con `Tabs` sin control (`operar`, `historico`,
`gestion`), `RegistersTab` 565 con `selectedId` local, `HistoryTab` 1607 con `detailId` local.
Router: `react-router` ^8.5.1 en `App.tsx` (`/caja` en :70).

**Decision**: `features/caja/` con `CajaPage` (contenedor y rutas hijas), `turno/` (RegisterPanel
partido en Operar y Cierre), `arqueo/`, `conciliacion/`, `movimientos/`, `reporte/` (Histórico y
detalle), `administrar/`, y `diferencia.ts` (formato compartido). Rutas: `/caja` (cajas),
`/caja/:cajaId`, `/caja/historico`, `/caja/historico/:corteId`, `/caja/administrar`. La pestaña es la
ruta. Los tests de `CashPage.test.tsx` se mueven con su componente sin cambiar aserciones (solo
imports y el `MemoryRouter`).

**Administrar**: `AjustesDeCaja` (en `/negocio`, `business.configure`) trae conceptos, terminales,
modo de arqueo por sucursal y correos del resumen. Pasa a `/caja/administrar` junto a las cajas; cada
bloque conserva su permiso (`cash.configure` para cajas, conceptos y terminales; `business.configure`
para modo de arqueo y correos). `/negocio` deja un enlace a Caja → Administrar en lugar del bloque.

## R7. Correcciones con rastro (fase 3)

Precedente: el reverso de una salida (0088): fila `kind='reverso'`, `reverses_id` con FK compuesta,
`check ((kind='reverso') = (reverses_id is not null))` e índice único parcial sobre `reverses_id`;
`CorrectCashOut` toma la fila `FOR UPDATE`, decide con `domain.CanReverse` y traduce el
unique violation a `ErrAlreadyReversed`. Bitácora con `auditCash(ctx, q, action, sessionID, …)`
(`app/audit.go:113`) en la misma `q`.

**Decision**: tabla `cash_session_adjustments` (nombre en inglés, constitución VII) con el mismo
patrón; modelo en [data-model.md](data-model.md). Permiso `cash.adjust_count` (admin y gerente por
omisión, 0108): ya es «lo que mueve el esperado de un corte». Solo sobre cortes `cerrada`, tomando la
sesión `FOR UPDATE`. Acciones de bitácora nuevas `cash.session_adjusted` y
`cash.session_adjustment_reversed`, grupo `AuditGroupCash`.

**Migración**: siguiente libre **0113** (verificado el 2026-11-13 en `origin/develop`, `origin/main`,
todas las ramas remotas y el worktree `egb-081`: la más alta es 0112). Se reconfirma al implementar.
Aditiva: la API anterior no la lee ni la escribe. Down se niega si hay filas.

## R8. Tolerancia y «por explicar» (fase 4)

`business_settings` (0019) ya guarda `blind_cash_count` (0067). **Decision**: columna
`cash_difference_tolerance numeric(10,2) not null default 0 check (>= 0)` en la migración **0114**.
Aditiva con default: la API anterior no la ve.

«Por explicar» = |`Total`| > tolerancia **y** no hay nota vigente en `cash_session_adjustments` **y**
la apertura siguiente no trae motivo. Lo calcula `domain.CutNeedsExplanation`, usado por el
Histórico y el detalle. Explicar en el último paso del cierre = insertar una nota en la misma
transacción del cierre, solo si quien cierra tiene `cash.configure` (ve el esperado); el servidor
rechaza la nota en un cierre ciego para que no se pueda usar como sonda.

## R9. Tema de un toque (fase 1)

next-themes con `attribute="class"`, `defaultTheme="system"`, llave `theme` en `localStorage`: ya es
por dispositivo. **Decision**: `TemaRapido` en la franja fija de `AppShell.tsx:133` llama a
`setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')`. Sin estado nuevo.

## R10. Formato de dinero y diferencia (fase 1)

`money()` (`utils/format.ts:7`) pone 0 o 2 decimales según haya centavos; se usa en todo el POS.
**Decision**: no cambiar `money()` (cambiaría tickets y precios del POS); Caja usa `moneyFixed()`
(siempre 2 decimales) y `<Diferencia valor=…/>` en `features/caja/diferencia.tsx`: palabra, signo
tipográfico (U+2212), ícono y color semántico nuevo `fg.shortage`/`fg.overage` (rojo anaranjado y
verde azulado, contraste ≥ 7:1 en los dos temas, verificado con un test de contraste sobre los
valores del token). `diffColor` y `montoOSinDato` salen de `CashPage` a ese archivo.

## R11. Disposición por ancho (fase 4)

Breakpoints de Chakra sin personalizar: sm 480, md 768, lg 1024, xl 1280, 2xl 1536. Caja usa
`<Page maxW="920px">` (`CashPage.tsx:547`) y un solo `xl` en `HistoryTab`.

**Decision**: agregar al tema dos breakpoints, `3xl: 1440px` y `4xl: 2560px` (sin mover los
existentes, para no tocar otras pantallas). Caja usa `Page` sin tope (`maxW="none"`) y un grid por
ancho: base una columna; `lg` dos (Operar | Cierre; lista | detalle); `3xl` tres; `4xl` cajas abiertas
lado a lado. El texto corrido lleva `maxW="70ch"`. 840 px del issue se aproxima con `md` (768) para
la hoja de detalle; lo que cambia en 840 es solo el detalle en hoja, y 768–1023 ya es tableta.

## R11-bis. Otros lectores de diferencias

- `queries/daily_summary.sql:54` (correo del resumen diario): se manda **al cerrar**, antes de que
  exista cualquier corrección. Es una foto de lo firmado y así se queda; no se reenvía al corregir.
- `ListSessionCounts` (`cash.sql:659`): el conteo firmado; el detalle lo pinta junto a la
  corrección vigente, nunca en su lugar.
- `terminals.sql:66`: diferencias por terminal; las correcciones de esta feature no tocan
  terminales.
- «Por explicar» se calcula con la tolerancia de **hoy**: cambiarla reclasifica cortes pasados. Es
  estado derivado, no un hecho; se dice en la ayuda de la tolerancia.

## R12. Lo que no se toca

- `money()` global, `order_refunds`, `expense_payments`: las salidas siguen naciendo de su hecho.
- Editar `session_cash_counts`: fuera de alcance (revierte la 015).
- Reabrir cortes (P3).
