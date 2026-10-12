# Data Model — migración 0089

Patrón de 0088 en todas las tablas nuevas: `company_id bigint not null default
nullif(current_setting('app.company_id', true), '')::bigint references companies(id) on delete
cascade`; `unique (company_id, id)` (`*_tenant_key`); RLS con política `tenant_isolation` y
`nullif`; `usage` sobre la secuencia; índices que empiezan por `company_id`; vigiladas por
`TestEveryCompanyTableIsIsolated`. Todas las FK a `users` son compuestas `(company_id, x)` →
`users (company_id, id)` con **`no action`** (R6). Ninguna usa el trigger genérico de sucursal.

## Columnas nuevas

| Tabla | Columna | |
|---|---|---|
| business_settings | `attendance_enabled boolean not null default false` | |
| business_settings | `attendance_measure_schedule boolean not null default false` | |
| business_settings | `attendance_grace_in_min smallint not null default 0 check (between 0 and 60)` | |
| business_settings | `attendance_grace_out_min smallint not null default 0 check (between 0 and 60)` | |
| users | `tracks_attendance boolean not null default true` | |

## attendance_entries — `grant select, insert` + `update` solo de `(clock_in_at, clock_out_at, status, corrected, business_date, out_ip, out_device, out_device_key)`

Nunca se actualizan `user_id`, `source`, `created_by` ni `company_id`. Trigger `before update`: si
cambia `clock_in_at`, o cambia `clock_out_at` de un valor a otro, exige que en la misma transacción
exista su fila en `attendance_corrections` (`corrected_at = now()`); una salida normal pasa de nulo
a valor y no la pide. Además, un turno `closed` o `missing_out` no vuelve a `open`, y uno `closed`
no pasa a `missing_out`.

| Columna | Tipo | Nota |
|---|---|---|
| id | bigserial PK | |
| user_id | bigint not null | FK compuesta a users |
| branch_id | bigint null | solo si la empresa tiene una sucursal; FK compuesta a branches |
| business_date | date not null | snapshot al insertar |
| clock_in_at | timestamptz not null | hora del servidor |
| clock_out_at | timestamptz null | `check (clock_out_at > clock_in_at)` |
| status | text not null | `check in ('open','closed','missing_out')`; `check ((status = 'closed') = (clock_out_at is not null))` → solo `closed` lleva salida |
| source | text not null | `check in ('clock','manual')` — «capturado a mano» |
| corrected | boolean not null default false | |
| in_ip / out_ip | inet null | |
| in_device / out_device | jsonb null | descripción |
| in_device_key / out_device_key | text null | `check (~ '^[0-9a-f]{32}$')` |
| created_by | bigint null | quien capturó a mano; FK compuesta |

Transiciones: `open → closed` (salida) · `open → missing_out` (nueva entrada tras 16 h) ·
`missing_out → closed` y `closed → closed` (corrección).

- Único parcial `(company_id, user_id) where status = 'open'` (FR-003; dos tabletas a la vez).
- Solape (captura a mano, corrección): primero `select … from users where (company_id, id) = … for
  update` para serializar por persona (un `for update` sobre las entradas no bloquea nada cuando no
  hay filas, y dos gerentes capturarían el mismo turno dos veces); después `domain` valida contra
  las entradas de la persona, tratando una entrada abierta como si terminara en `+∞`. Sin
  `exclusion constraint` para no requerir `btree_gist`. Checar usa el mismo candado.
- Índices: `(company_id, business_date)`, `(company_id, user_id, business_date)`.

## attendance_corrections — `grant select, insert`

`id, company_id, entry_id` (FK compuesta a attendance_entries, `no action`), `field text check in
('clock_in_at','clock_out_at','created')`, `before timestamptz null`, `after timestamptz not null`,
`reason text not null check (char_length between 3 and 200)`, `corrected_by` (FK compuesta),
`corrected_at timestamptz not null default now()`. Sin `update` en el grant: la base impone que el
valor original no se pierde (FR-010). Índice `(company_id, entry_id)`.

## work_schedule_versions — `grant select, insert`

`id, company_id, user_id` (FK compuesta), `effective_from date not null`, `created_by`, `created_at`.
Único `(company_id, user_id, effective_from)`. Cambiar el horario = nueva versión completa; un día
sin fila en la versión vigente es libre (resuelve el sábado que «resucitaba»).

## work_schedule_days — `grant select, insert`

`id, company_id, version_id` (FK compuesta a work_schedule_versions), `weekday smallint check
(between 0 and 6)`, `shift_no smallint not null default 1 check (shift_no >= 1)`, `start_time time
not null`, `end_time time not null check (end_time <> start_time)`. Único `(company_id, version_id,
weekday, shift_no)`. La pantalla solo captura `shift_no = 1`. Versión y días se insertan en la misma
transacción; el servicio nunca agrega días a una versión existente.

## audit_events — `grant select, insert`

| Columna | Tipo | Nota |
|---|---|---|
| id | bigserial PK | |
| branch_id | bigint null | del objeto auditado; FK compuesta a branches |
| occurred_at | timestamptz not null default now() | |
| actor_kind | text not null | `check in ('user','platform','system')` |
| actor_user_id | bigint null | FK compuesta; `check ((actor_kind = 'user') = (actor_user_id is not null))` |
| actor_label | text null | p. ej. `uber` |
| action | text not null | lista blanca en `domain`; sin `check` en la base para poder agregar acciones sin migración |
| entity_type / entity_id | text / bigint not null | |
| order_id | bigint null | **sin FK, a propósito**: el evento sobrevive a un reorg de datos. Sin chequeo de tenant ahí; RLS filtra la lectura |
| subject_user_id | bigint null | persona afectada (asistencia); FK compuesta |
| ip | inet null | |
| device | jsonb null | |
| device_key | text null | `check (~ '^[0-9a-f]{32}$')` |
| before / after | jsonb null | |

- Índices: `(company_id, occurred_at desc, id desc)` (el orden de la bitácora); `(company_id,
  order_id, occurred_at) where order_id is not null`; `(company_id, actor_user_id, occurred_at
  desc)`; `(company_id, device_key, occurred_at desc) where device_key is not null`. Por objeto y
  por persona afectada no hay índice: ninguna consulta los usa todavía, y agregarlos después cuesta
  lo mismo.
- Triggers: `before update or delete` y `before truncate` (R6).
- Un traslado entre pedidos escribe un evento por cada pedido, cada uno con el otro en `after`.

## Down de 0089

Se niega con `raise exception` si hay filas en `audit_events`, `attendance_entries`,
`attendance_corrections` o `work_schedule_versions` (patrón de 0079): son la única copia de esos
hechos. Sin filas, suelta triggers y funciones antes de los `drop table`; esa ruta lleva test. Tras
desplegar, el rollback es restaurar el respaldo.

## Acciones (domain.AuditAction)

Cada acción pertenece a un grupo de la bitácora (`domain.AuditGroupOf`): `money` (pedidos y cobros),
`cancel` (cancelaciones, devoluciones, pagos devueltos, mermas), `price`, `cash` (caja, salidas,
gastos, propinas), `attendance` (asistencia, horarios, ajustes).


`order.created`, `order.lines_added`, `order.line_removed`, `order.lines_moved`, `order.delivered`,
`order.cancelled`, `order.discount_set`, `order.platform_ref_set`, `payment.charged`,
`payment.voided`, `payment.refunded`, `stock.written_off`, `cash.opened`, `cash.closed`,
`cash.movement` (entrada, salida, propina o traspaso), `cash.movement_corrected`, `expense.created`, `expense.paid`, `expense.cancelled`, `tip.paid_out`,
`price.set`, `price.deleted`, `price.synced`, `attendance.clock_in`, `attendance.clock_out`,
`attendance.corrected`, `attendance.manual_entry`, `attendance.settings_changed`,
`attendance.schedule_set`, `user.tracks_attendance_changed`.
