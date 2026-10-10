# Implementation Plan: Caja, propinas y cobro con tarjeta

**Branch**: `032-caja-propinas-tarjeta` | **Date**: 2026-10-09 | **Spec**: [spec.md](spec.md)

## Summary

Se separa la propina del dinero del negocio en todo el ciclo de caja (cobro → entrega → cierre →
corte → correo), se obliga concepto en toda salida con corrección por reverso, se cuenta la
apertura a ciegas por denominación y se registra terminal y débito/crédito en cada cobro con
tarjeta, con arqueo por terminal configurable por sucursal y folio obligatorio en devoluciones.

## Technical Context

- **Lenguaje**: Go 1.27 (chi, pgx + sqlc, goose embebido) · React 19 + Chakra v3 + TanStack Query.
- **Almacenamiento**: PostgreSQL con RLS por `company_id`; Redis solo caché (no se usa aquí).
- **Testing**: unitarios table-driven en `domain`; integración contra Postgres real bajo
  `appRoleStore` con `inTheThreeCases`; vitest en `web/`; e2e Playwright contra pruebas.
- **Plataforma**: tabletas 1024×600, táctil, controles ≥44 px, `Picker` en lugar de `<select>`.
- **Restricciones**: dinero `float64` con `Round2`/`ValidMoney`; producción con datos reales;
  migraciones con su test de integración (hook pre-commit).
- **Escala**: un local hoy, varias cajas y sucursales mañana (puertas VIII).
- **Siguiente migración libre en develop**: `0087`. Verificar al implementar que otra rama viva no
  la haya tomado.

## Constitution Check

| Principio | Cómo se cumple | Estado |
| --- | --- | --- |
| I Layering | Cálculo de pendiente, clasificación del corte, validación de reverso, diferencia de apertura y arqueo por terminal en `domain`; orquestación y tx en `app`; handlers finos; SQL vía sqlc | OK |
| II Errores | Sentinels nuevos en `domain` (`ErrTipExceedsPending`, `ErrAlreadyReversed`, `ErrConceptRequired`, `ErrRefundFolioRequired`, `ErrOpeningReasonRequired`) mapeados solo en `httpapi.Error` | OK |
| III Dinero | Propina clasificada una vez; test que falla nombrando el concepto duplicado (`TestTipIsNeitherSaleNorExpense`, `TestCardTipPaidInCashLeavesDrawerOnce`) | OK |
| IV Test-first | Bordes EB-01..EB-42 enumerados en el spec antes del diseño; cada uno con test en `tasks.md` antes de su implementación; migraciones con test a dos empresas | OK |
| V Seguridad | Rutas nuevas con `RequireAuth`+`RequireRole`; `ValidMoney` en toda frontera; parámetros inválidos → 400; correo sin PII de clientes; apertura ciega se garantiza en el servidor (no se envía el cierre anterior) | OK |
| VI YAGNI | Motivos de apertura y tipo débito/crédito como constantes; sin reparto automático de propinas; sin catálogo de empleados | OK |
| VII Comentarios / idioma | Identificadores nuevos en inglés; textos en español | OK |
| VIII Puertas | Ver abajo | OK |

### Pregunta VIII: ¿se puede agregar después al mismo costo?

- **Más de una caja**: el pendiente de propina se calcula por **turno y caja** (`register_sessions.register_id`), nunca «la caja abierta». Lo heredado se busca por el turno anterior de la misma caja. Puerta abierta.
- **Más de una sucursal**: terminales y modo de arqueo cuelgan de `branches`; el correo diario agrupa por empresa y lista sucursales. Puerta abierta.
- **Saber de quién es**: la entrega guarda quién entregó y quién recibió; el folio de devolución guarda quién lo capturó. Irrecuperable si no se guarda hoy: se guarda.
- **Snapshot**: la entrega guarda el nombre de quien recibe al momento (snapshot) además del id, para que renombrar un usuario no reescriba el corte. La terminal se guarda por id y su nombre se copia al pago (renombrar una terminal no reescribe el pasado).

## Capas

| Pieza | Capa |
| --- | --- |
| `TipLedger` (pendiente por método, validación de entrega, herencia) | `domain/tips.go` |
| Clasificación de movimientos para corte y gastos | `domain/cash_movements.go` |
| Reverso (`CanReverse`) y día del gasto (`ExpenseDay`) | `domain/cash_movements.go` |
| Diferencia de apertura (`OpeningDiff`, tolerancia 0 tras `Round2`) | `domain/opening.go` |
| Arqueo de tarjeta por modo | `domain/card_count.go` |
| Servicios `TipsService`, `CashConceptsService`, `TerminalsService`; ampliaciones a cobro, cierre, apertura y devolución | `app/` |
| Correo diario: trabajo programado en `cmd/api` que itera empresas con `store` por empresa | `app/daily_summary.go` + `mailer` |
| Pantallas: hoja «Entregar propina», pregunta al cerrar, apertura por denominación, selector de concepto (`Picker` con alta en línea), terminal en cobro, folio en devolución, Configuración (conceptos, terminales, modo de arqueo) | `web/src/features/caja`, `web/src/features/admin` |

## Project Structure

```text
specs/032-caja-propinas-tarjeta/
├── spec.md · plan.md · research.md · data-model.md · quickstart.md · tasks.md
└── contracts/api.md
server/migrations/0087_cash_concepts_and_tips.sql   (+ 0088_card_terminals.sql)
server/queries/{cash,tips,cash_concepts,terminals}.sql
server/internal/{domain,app,httpapi,integration}/
web/src/features/{caja,admin,vender}/
```

## Riesgos

- **Migración de datos de propinas históricas**: se hace como corrección de datos aparte (SQL
  numerado + rollback gemelo, `docs/reorg/` patrón), con lista aprobada por el dueño y respaldo.
  No va en la migración de esquema.
- **Cobros viejos sin terminal**: la columna es nulable; un check exige terminal solo para pagos
  creados después de la migración (`created_at >= fecha de corte`) — ver research R3.
- **Correo**: si el mailer falla, se registra evento y se reintenta en la siguiente corrida; la
  llave única `(company_id, business_date)` evita duplicados.
- **Diseño de «Entregar propina»**: tres opciones en maquetas; la elección del dueño puede mover
  pantallas, no el modelo.

## Complexity Tracking

Sin violaciones que justificar.

## Revisión after_plan (db-architect + tablet-ui-reviewer, 2026-10-09)

Hallazgos incorporados como reglas del plan; `tasks.md` los cubre.

**Base de datos**
- D1 `tip_payout_sources`, `session_terminal_counts` y `daily_summary_sends` llevan `company_id` (cascade a `companies`), RLS con `nullif` y grant.
- D2 FKs: hechos contables con `on delete restrict` (`tip_payout_sources.*`, `session_terminal_counts.*`, `cash_concepts.merged_into_id`, `register_cash_movements.concept_id/recipient_user_id`, `card_terminals.branch_id`); `user_preferences.default_card_terminal_id` con `set null`.
- D3 Únicos parciales con `company_id`: `(company_id, name_key) where archived_at is null`, `(company_id, branch_id, name) where archived_at is null`. El único parcial de `reverses_id` va sin `company_id` porque la columna ya es per-tenant (excepción declarada).
- D4 Montos `numeric(10,2)` con `check (> 0)` / `(>= 0)`; índice `(company_id, order_payment_id)` en `tip_payout_sources`; `(company_id, session_id, kind)` en movimientos.
- D5 El pendiente pre-agrega cada rama (propinas cobradas, devueltas, entregadas) por pago en subconsultas; test con 2 pagos × 2 devoluciones × 2 entregas.
- D6 Obligatoriedad de terminal y de concepto solo para filas nuevas: trigger `before insert` (y `update` de esas columnas), sin `now()` en un check; probado sobre respaldo con históricos.
- D7 Check de forma por `kind` (propina exige receptor, snapshot y método; reverso exige `reverses_id`). El Down falla si existen filas `propina`/`reverso`; nunca las borra.
- D8 El reverso va en el turno abierto actual (FR-012) y su signo se resuelve por `reverses_id`.
- D9 Siembra por empresa dentro de la migración, `gatobobah` por slug; test con dos empresas. `card_terminals.branch_id` sigue la convención de `0076`.
- D10 El conteo por terminal se deriva de `order_payments.register_session_id`.

**Tableta**
- U1 En la hoja de cobro, tarjeta muestra una fila «Débito | Crédito» (52 px) y la terminal como chip «Cambiar»; sin terminal por omisión, `Picker` primero. Reemplaza el bloque «Recibido», no se apila.
- U2 Entregar propina: monto precargado con el pendiente; el método de origen no se pregunta (se descuenta primero lo cobrado en efectivo). El contrato acepta `amount` omitido = todo el pendiente.
- U3 La decisión de propinas al cerrar va dentro del paso de cierre como dos botones ≥52 px, sin valor preseleccionado; «Entregar ahora» encadena la entrega y regresa.
- U4 Apertura: rejilla de dos columnas con −/+ de 44 px por denominación y total en pie fijo, hoja con scroll interno en dvh; motivo con `Picker` + texto. Se reutiliza el conteo existente de la caja.
- U5 Concepto con `Picker` + fila «Agregar “texto”» que crea y selecciona sin cerrar. «Corregir» vive en la hoja de detalle de la salida, no en la fila.
- U6 Conteo por terminal en paso propio «Tarjeta» del cierre; en modo automático no se muestra.
- U7 Mensajes de error en lenguaje de operador («Escribe el folio que imprimió la terminal»).
- U8 Avisos de salidas sin concepto con botón que lleva a corregirlas.
- U9 Modo de arqueo con control segmentado de 44 px; fusionar conceptos con `Picker` de destino y confirmación, separado de «Archivar».

## Decisiones del dueño del 2026-10-09 (D-A, D-B, D-C del spec)

- `domain.SplitEven(pending, n)` devuelve pesos enteros por persona y el sobrante; `domain.ValidWholePesos` rechaza centavos. El sobrante no se guarda aparte: el pendiente se recalcula siempre y lo incluye.
- La pregunta del cierre usa `domain.TipsDecisionRequired(pending) = pending >= 1`; un sobrante menor a $1 se hereda solo, sin preguntar.
- Correos del resumen: `business_settings.daily_summary_emails text[]`, validados en `domain.ValidSummaryEmails`. Reemplaza el supuesto del correo de recuperación.

## Límites conocidos de la entrega de propinas (revisión de código, 2026-10-09)

- Lo heredado (`tips_carried_over`) es un solo número: al entregarlo en el turno siguiente cuenta
  como efectivo, así que la «propina de tarjeta pagada en efectivo» de ese turno sale menor. Se
  corrige heredando por separado efectivo y otros medios.
- Una devolución de propina posterior al cierre no baja lo ya heredado: el turno siguiente puede
  ofrecer entregar propina que ya se devolvió.
- En modo ajustado los montos no se precargan con el reparto parejo (revisor de tableta).
