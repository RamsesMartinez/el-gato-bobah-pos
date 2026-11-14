# Data model: Rediseño del módulo Caja

## Fases 1 y 2: sin esquema nuevo

Solo cambian lecturas. Tipos de `domain` (puros, sin I/O):

```text
MethodDiff   { Method string; Expected, Declared float64; TouchesDrawer bool }
DrawerCount  { Expected *float64; Counted float64 }          // conteo de cierre
CutDiff      { Drawer *float64; OtherMethods float64; Total *float64;
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
| `session_id` | bigint not null | FK `(company_id, session_id)` → `register_sessions (company_id, id)` on delete restrict |
| `kind` | text not null | `count_correction`, `float_correction`, `note`, `reversal` |
| `before_amount` | numeric(10,2) | lo que decía antes de esta corrección (el firmado o la corrección vigente) |
| `after_amount` | numeric(10,2) | ≥ 0 |
| `reason` | text not null | `miscount`, `unrecorded_movement`, `wrong_register`, `float_error`, `other` |
| `note` | text not null | 1..200, recortado |
| `reverses_id` | bigint | FK `(company_id, reverses_id)` → `cash_session_adjustments (company_id, id)` |
| `created_by` | bigint not null | FK `(company_id, created_by)` → `users (company_id, id)` on delete restrict |
| `created_at` | timestamptz not null default now() | |

Checks:
- `(kind = 'reversal') = (reverses_id is not null)`.
- `kind in ('count_correction','float_correction')` ⇒ `before_amount` y `after_amount` no nulos.
- `kind in ('note','reversal')` ⇒ `before_amount` y `after_amount` nulos.

Índices: `unique (company_id, id)` (para la FK propia), `unique (reverses_id) where reverses_id is not
null` (un reverso por corrección, sin `company_id` a propósito, como 0088),
`(company_id, session_id, created_at)`.

Seguridad: RLS `tenant_isolation` con `nullif`, `grant select, insert` a `gatobobah_app` (sin
`update` ni `delete`: solo se agrega).

Respaldo anónimo: `note` se reemplaza por 'nota anonimizada' en el paso 5 de
`scripts/respaldo-anonimo.sh` y se cuenta en `FUGAS`; `kind` y `reason` van a `noPersonalData` como
«valor de una lista cerrada (check)», igual que `register_sessions.opening_reason`.

Down: se niega con `raise exception` si hay filas; si no, `drop table`. Lleva su renglón en
`guardedDownCleanup` (`harness_restaurado_test.go`).

### Reglas (en `domain`)

- `CanAdjustSession(status)`: solo `cerrada`.
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
