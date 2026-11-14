# Data model: Rediseño del módulo Caja

## Fases 1 y 2: sin esquema nuevo

Solo cambian lecturas. Tipos de `domain` (puros, sin I/O):

```text
MethodDiff   { Method string; Expected, Declared decimal.Decimal; TouchesDrawer bool }
DrawerCount  { Expected *decimal.Decimal; Counted decimal.Decimal }          // conteo de cierre
CutDiff      { Drawer *decimal.Decimal; OtherMethods decimal.Decimal; Total *decimal.Decimal;
               DrawerCovered bool }                         // nil = no se sabe (arqueo ciego o sin conteo)
```

`CutDifference(methods, drawer, effectiveCounted)`:
- `Drawer` = `effectiveCounted − drawer.Expected` si hay conteo de cierre con esperado; si no, nil.
- `DrawerCovered` = `Drawer != nil`. Con `DrawerCovered`, los métodos `TouchesDrawer` no suman a
  `OtherMethods` y su renglón de conciliación se pinta «ver arqueo».
- `OtherMethods` = Σ (`Declared − Expected`) de los métodos no cubiertos.
- `Total` = `OtherMethods + coalesce(Drawer, 0)`; nil si el esperado se ocultó.
- Todo con `Round2`.

`effectiveCounted` es el conteo firmado en las fases 1, 2 y 4 sin correcciones; desde la fase 3, el
`after` de la corrección de conteo vigente (ver abajo).

Campos nuevos en la respuesta (no en la base):

| Vista | Campo | Origen |
| --- | --- | --- |
| `SessionHistoryRow`, `SessionDetailView` | `difference {drawer, otherMethods, total}` | `CutDifference` |
| ídem | `openingReason`, `openingReasonNote` | `register_sessions` (0088) |
| ídem | `explainedByNext {sessionId, reason, note}` | apertura siguiente de la misma caja con motivo |
| `SessionDetailView` | `previousSessionId` | corte que la apertura comparó |
| `SessionHistoryRow` | `openedByName`, `closedByName` | ya en el detalle; la lista los necesita en pantallas anchas |

`totalDifference` sigue en `SessionHistoryRow` (= `difference.total`) mientras exista un cliente que
lo lea (la consola de MCP y la PWA anterior durante la subida); se quita en una subida posterior.

## Fase 3: `cash_session_adjustments` (migración 0113)

| Columna | Tipo | Regla |
| --- | --- | --- |
| `id` | bigint identity PK | |
| `company_id` | bigint not null | default `nullif(current_setting('app.company_id', true), '')::bigint`, FK `companies` on delete cascade |
| `session_id` | bigint not null | FK `(company_id, session_id)` → `register_sessions (company_id, id)` (`no action`: no se borra un corte con correcciones; `no action` y no `restrict` para que el borrado en cascada de una empresa no dependa del orden) |
| `kind` | text not null | `check (kind in ('count_correction','float_correction','note','reversal'))` |
| `before_amount` | numeric(10,2) | ≥ 0; lo que decía antes (el firmado o la corrección vigente). Nulo solo en `float_correction` de un corte anterior a 0088, que no guardó fondo |
| `after_amount` | numeric(10,2) | ≥ 0 |
| `reason` | text not null | `check (reason in ('miscount','unrecorded_movement','wrong_register','float_error','other'))` |
| `note` | text not null | `check (note = btrim(note) and char_length(note) between 1 and 200)` |
| `reverses_id` | bigint | FK `(company_id, session_id, reverses_id)` → `cash_session_adjustments (company_id, session_id, id)`: un reverso solo revierte una corrección **del mismo corte** |
| `created_by` | bigint not null | FK `(company_id, created_by)` → `users (company_id, id)` (`no action`) |
| `created_at` | timestamptz not null default now() | |

Checks:
- `(kind = 'reversal') = (reverses_id is not null)`.
- `kind = 'count_correction'` ⇒ `before_amount` y `after_amount` no nulos; `kind = 'float_correction'` ⇒ `after_amount` no nulo.
- `after_amount >= 0` y `before_amount >= 0`.
- `kind in ('note','reversal')` ⇒ `before_amount` y `after_amount` nulos.

Índices: `unique (company_id, session_id, id)` (para la FK del reverso; su prefijo cubre las lecturas por corte), `unique (reverses_id) where reverses_id is not
null` (un reverso por corrección; sin `company_id` a propósito, como 0088: es id de una tabla con
identity propia y no colisiona entre empresas).

Seguridad: RLS `tenant_isolation` con `nullif`, `grant select, insert` a `gatobobah_app` (sin
`update` ni `delete`: solo se agrega).

Respaldo anónimo: `note` se reemplaza por 'nota anonimizada' en el paso 5 de
`scripts/respaldo-anonimo.sh` y se cuenta en `FUGAS`; `kind` y `reason` van a `noPersonalData` como
«valor de una lista cerrada (check)», igual que `register_sessions.opening_reason`.

Down: se niega con `raise exception` si hay filas; si no, `drop table`. Lleva su renglón en
`guardedDownCleanup` (`harness_restaurado_test.go`).

### Reglas (en `domain`)

- `CanAdjustSession(status)`: solo `cerrada`.
- Corregir el conteo de un corte **sin conteo de cierre con esperado** (anterior a la 0067) se
  rechaza con 422 `NO_SIGNED_COUNT`: no hay arqueo que corregir; se deja una nota.
- Revertir un `reversal` lo impide `CanReverseAdjustment` (sin trigger: el servicio es el único
  que escribe y toma el corte `FOR UPDATE`).
- `CanReverseAdjustment(adj, alreadyReversed)`: no se revierte un `reversal` ni algo ya revertido.
- **Vigente** de un tipo: la última corrección de ese tipo no revertida. Conteo efectivo =
  `after_amount` de la vigente `count_correction`, o el conteo firmado. Fondo efectivo, igual con
  `float_correction`.
- `before_amount` lo pone el servidor (el valor efectivo al momento), nunca el cliente.
- Una corrección cuyo `after` es igual al efectivo actual se rechaza (400): no deja rastro de nada.

### Transiciones

```text
corte cerrada ──corrección──▶ (corte con corrección vigente) ──reverso──▶ (vuelve al valor anterior)
```

El corte nunca cambia de estado; `session_cash_counts`, `register_session_totals` y
`register_sessions.float_left` no se escriben.

## Fase 4: tolerancia (migración 0114)

`business_settings.cash_difference_tolerance numeric(10,2) not null default 0 check (>= 0)`.
Número: no entra al respaldo anónimo. Down: `drop column` (no pierde un hecho, solo configuración;
se avisa en el plan).

`CutNeedsExplanation(diff, tolerance, hasNote, nextOpeningReason) bool`: |Total| > tolerancia, sin
nota vigente y sin motivo en la apertura siguiente. Con `Total` nil (ciego) → se calcula en el
servidor con las piezas sin ocultar y solo se expone la bandera a quien ve el arqueo; al cajero no se
le dice nada (FR-033).
