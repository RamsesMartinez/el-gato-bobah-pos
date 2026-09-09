# Modelo de datos: folio de plataforma y liquidación

**Migración**: `0065_folio_y_liquidacion_de_plataforma.sql` (la última aplicada es
[0064](../../server/migrations/0064_familia_de_refresh.sql)).

Va **una sola migración** porque las dos piezas comparten la restricción que las define: la
liquidación de un pedido no se puede conciliar sin el folio, y el índice único del folio y la FK de
la liquidación apuntan a la misma fila de `orders`. Separarlas dejaría una ventana en la que la
segunda mitad no tiene con qué probarse.

> **Antes de aplicarla corre el `db-architect`** (hook `after_plan`, y la regla de
> [CLAUDE.md](../../CLAUDE.md) para cualquier migración nueva).

---

## 1. `orders` gana tres columnas

```sql
alter table orders
  add column platform_order_ref  text,
  add column platform_ref_set_by bigint references users(id),
  add column platform_ref_set_at timestamptz;
```

| Columna | Tipo | Nulo | Por qué así |
|---|---|---|---|
| `platform_order_ref` | `text` | sí | Uber es UUID de 36, DiDi entero de 19 dígitos, Rappi de 10: ningún entero los cubre. Se guarda **completo** — del código corto de 5 caracteres no se reconstruye el UUID |
| `platform_ref_set_by` | `bigint` → `users(id)` | sí | El rastro de quién escribió el valor con el que se concilia un depósito. Mismo trato que `product_platform_prices.updated_by`. Sin `on delete`: los usuarios se desactivan, no se borran |
| `platform_ref_set_at` | `timestamptz` | sí | Cuándo. Nullable porque los pedidos que ya están en producción no tienen folio y no se les inventa uno |

### Checks

```sql
alter table orders add constraint orders_platform_ref_forma check (
  platform_order_ref is null
  or (platform_order_ref = btrim(platform_order_ref, E' \t\n\r')
      and length(platform_order_ref) between 1 and 64)
);

alter table orders add constraint orders_platform_ref_solo_plataforma check (
  platform_order_ref is null or delivery_platform_id is not null
);

alter table orders add constraint orders_platform_ref_rastro check (
  (platform_order_ref is null) = (platform_ref_set_by is null)
  and (platform_ref_set_by is null) = (platform_ref_set_at is null)
);
```

- **El de forma** cierra la cadena vacía y la que trae espacios en los extremos. `""` no es lo mismo
  que no tener folio: guardarlo haría que dos pedidos sin folio chocaran contra la unicidad, o peor,
  que pasaran. El tope de 64 es cota de cordura, no regla de negocio.
- **El de plataforma** hace que "pedido de mostrador con folio" sea **imposible por construcción**, no
  una validación de pantalla que un camino nuevo se salta. Es el modo de falla que la constitución
  llama *"el camino nuevo que se salta el control viejo"*.
- **El de rastro** es todo o nada: o están las tres columnas o no está ninguna. Sin él, un camino
  futuro que escriba el folio sin su `set_by`/`set_at` —o que borre uno de los tres al corregir—
  deja el rastro de D-12 roto sin que nada avise, y el rastro existe justamente para poder confiar
  en él meses después. Es la forma del check de cancelación que ya vive en `orders`
  (`status='cancelada'` ⇔ los tres campos de cancelación). El de reembolso **no** lo tiene, así que
  no es convención universal del repo: se pone aquí porque D-12 lo necesita vivo.

### El `unique (id, company_id)` que la liquidación necesita

```sql
alter table orders add constraint orders_id_company_key unique (id, company_id);
```

No es para `orders`: es el índice destino de la **FK compuesta** de `platform_settlements` (§2). Va
aquí porque el destino tiene que existir antes que la llave. Mismo patrón con el que
[0037](../../server/migrations/0037_platform_prices.sql) preparó
`delivery_platforms_id_company_key`.

### Índice único parcial

```sql
create unique index orders_platform_ref
  on orders (company_id, delivery_platform_id, platform_order_ref)
  where platform_order_ref is not null;
```

Copia la forma de [0057](../../server/migrations/0057_cobro_idempotente.sql). El `where` es lo que
impide que el índice cargue con todo el histórico de mostrador sin servir para nada. `company_id`
va al frente porque RLS lo agrega a toda consulta del rol de app.

### Índice de soporte del filtro de pendientes

```sql
create index orders_plataforma_sin_folio
  on orders (company_id, business_date)
  where delivery_platform_id is not null and platform_order_ref is null;
```

Parcial sobre un subconjunto chico (solo plataforma, solo sin folio) para que el filtro de la US-2 no
escanee el histórico.

**Este índice solo sirve si la consulta lleva el predicado LITERAL**, y por eso D-8 dejó de usar el
patrón `narg is null or (…)` de los otros filtros. Medido contra Postgres real con 150k pedidos: con
el patrón OR y **plan genérico** —al que pgx cae solo, porque usa statements con nombre y no hay
override de `QueryExecMode` en [store.go](../../server/internal/store/store.go)— Postgres no puede
probar que el predicado del índice parcial se cumple, porque el valor del parámetro es desconocido al
planear. El plan cae a `Bitmap Index Scan on orders_date_status` con el filtro aplicado a mano,
**incluso con el parámetro en `true`**. Con el predicado literal usa el índice parcial hasta con
`plan_cache_mode = force_generic_plan`.

Es lo que sostiene SC-008, y se verifica con `EXPLAIN (ANALYZE, BUFFERS)` sobre la ejecución real
**vía pgx**, no con psql y literales — que es justo el escenario en el que el defecto no se ve.

### Índice de la búsqueda por folio

```sql
create index orders_platform_ref_busqueda
  on orders (company_id, platform_order_ref)
  where platform_order_ref is not null;
```

El índice único **no sirve** para buscar: arranca por `(company_id, delivery_platform_id, …)` y deja
la plataforma en medio, pero quien pega un folio del documento no la conoce. Este lleva el folio
inmediatamente después de `company_id`, que es lo que RLS ya fija. Parcial por el mismo motivo que
los otros dos: sin el `where` cargaría con todo el histórico de mostrador.

### `lock_timeout`

La migración abre con `set local lock_timeout = '3s'`, como
[0042](../../server/migrations/0042_sales_index.sql): `alter table` y `create index` toman
`ACCESS EXCLUSIVE` sobre `orders`, que es la tabla que toca cada venta. El riesgo no es el tamaño
—son ~70 filas— sino la **cola**: si al desplegar hay una transacción larga abierta, el ALTER se
encola detrás y arrastra toda lectura nueva. Falla limpio en 3 s antes que dejar el POS mudo.

**Y el lock es de toda la migración, no de cada instrucción**: goose corre el archivo en una sola
transacción, así que el `ACCESS EXCLUSIVE` de los tres `add column`, los tres checks, el
`unique (id, company_id)` y los tres `create index` se sostiene hasta el `commit` final. Con las
decenas de filas de hoy son milisegundos; se dice aquí para que no se re-derive el día que `orders`
sea grande y alguien copie este patrón.

---

## 2. `platform_settlements` — la liquidación

Uno a uno con el pedido. **Su existencia es el estado del dinero**: hay fila = el documento llegó y
se capturó; no hay fila = todavía no se sabe. Por eso no es una columna de estado.

```sql
create table platform_settlements (
  -- FK COMPUESTA, no simple: ver la nota de abajo. La PK sigue siendo solo order_id.
  order_id            bigint primary key,
  reported_gross      numeric(10,2) not null,
  commission_amount   numeric(10,2) not null,
  commission_pct      numeric(5,2),
  discount_total      numeric(10,2) not null default 0,
  discount_platform   numeric(10,2) not null default 0,
  withholdings        numeric(10,2) not null default 0,
  net_amount          numeric(10,2) not null,
  payout_reference    text,
  document_ref        text,
  captured_by         bigint not null references users(id),
  captured_at         timestamptz not null default now(),
  updated_at          timestamptz not null default now(),
  company_id          bigint not null default current_setting('app.company_id', true)::bigint
                      references companies(id) on delete cascade,
  constraint platform_settlements_order_fkey
    foreign key (order_id, company_id) references orders (id, company_id)
);
```

**Por qué la FK es compuesta y no `references orders(id)` a secas**: los chequeos de integridad
referencial de Postgres **saltan RLS por diseño**, así que una FK simple aceptaría sin protestar una
liquidación cuyo `company_id` es de una empresa y cuyo `order_id` es de otra. La fila quedaría
invisible en todo join bajo RLS y el error aparecería en el resumen de dinero, no en el `insert`.
Es exactamente el hueco que [0041](../../server/migrations/0041_fk_compuesta_tenant.sql) cerró para
las tablas que agrupan dinero.

La excepción que se aceptó en `product_platform_prices` (0037: *"las dos columnas son FKs a tablas
per-tenant con identity global, así que dos empresas nunca comparten un id"*) **no aplica aquí**: eso
es catálogo y esto es dinero. La tabla nace vacía, así que no hace falta el bloque `do $mig$` de
verificación previa que sí necesitaron 0040 y 0041.

| Columna | Qué es, y por qué está | Signo |
|---|---|---|
| `order_id` **PK** | FR-014 (a lo más una) lo garantiza el motor, y "reemplaza, no duplica" es un `on conflict do update`: una instrucción, sin ventana de inconsistencia. **FK compuesta con `company_id`** (arriba). Sin `on delete`: no existe ningún `delete from orders` en el repo | — |
| `reported_gross` | Lo que la plataforma **reporta** como venta. No es `orders.total` y no se compara con él aquí: cuadrarlos es la feature de conciliación | ≥ 0 |
| `commission_amount` | Monto real del documento. **Snapshot, nunca calculado** con un porcentaje configurado: recalcular el pasado con la tasa de hoy reescribe la historia | ≥ 0 |
| `commission_pct` | **Nullable a propósito**: hay documentos que dan el monto sin declarar la tasa, y un `0` ahí afirmaría "cobró 0%", que es medible y falso | 0–100 |
| `discount_total` | El descuento del pedido según el documento | ≥ 0 |
| `discount_platform` | **La parte que financió la plataforma.** Rappi la parte en `amount_by_rappi`/`amount_by_partner` y cambia por campaña; sin esta columna cada promoción se registra como pérdida propia del restaurante | 0 ≤ x ≤ `discount_total` |
| `withholdings` | Retenciones (IVA + ISR) atribuidas al pedido | ≥ 0 |
| `net_amount` | Lo que llegó al banco por este pedido. **Puede ser negativo** y se acepta: es lo que de verdad pasa con una promoción financiada por el restaurante | libre |
| `payout_reference` | `payment_id` de Rappi / *Payout reference ID* de Uber. Texto porque **expira**: Rappi conserva 3 meses, Uber 31 días. Es lo que convierte "conciliar por monto y fecha" en una unión exacta. Tope 128 | — |
| `document_ref` | De qué documento salió esta captura. Tope 256, que es el largo de un nombre de archivo con ruta | — |
| `captured_by` / `captured_at` | Quién y cuándo. FR-014 pide el rastro; una recaptura los reescribe, que es lo correcto: lo que vale es la última lectura del documento vigente | — |

**La parte del restaurante NO es columna**: es `discount_total − discount_platform`, se calcula en
`domain` y viaja en la respuesta. Guardar los tres deja dos verdades sobre el mismo hecho, y un
documento corregido que mueva una y no la otra deja la fila contradiciéndose. Ver D-4 de
[research.md](research.md). **FR-012 se enmendó el 2026-09-07** para decir exactamente esto, así que
ya no hay desviación entre el spec y el esquema.

### Checks

```sql
constraint ps_montos_no_negativos check (
  reported_gross >= 0 and commission_amount >= 0
  and discount_total >= 0 and discount_platform >= 0 and withholdings >= 0
),
constraint ps_tasa_en_rango check (commission_pct is null or (commission_pct >= 0 and commission_pct <= 100)),
constraint ps_descuento_de_la_plataforma_cabe check (discount_platform <= discount_total),
constraint ps_referencias_acotadas check (
  (payout_reference is null or (payout_reference = btrim(payout_reference) and length(payout_reference) between 1 and 128))
  and (document_ref is null or (document_ref = btrim(document_ref) and length(document_ref) between 1 and 256))
)
```

`net_amount` **no lleva check de signo**. Es la única columna de dinero del esquema que no lo lleva,
y es deliberado: el escenario 4 del spec es un neto negativo real.

Las dos referencias de texto llevan tope por el mismo motivo que el folio —cota de cordura contra un
pegado accidental de media pantalla— aunque el riesgo es menor: las captura admin o gerente, no el
cajero en hora pico. La cadena vacía se rechaza ahí también: una referencia de depósito `""` no es
una referencia.

### Índices, RLS y grants

```sql
create index platform_settlements_company on platform_settlements (company_id, captured_at desc);

create trigger trg_platform_settlements_updated before update on platform_settlements
  for each row execute function set_updated_at();

alter table platform_settlements enable row level security;
create policy tenant_isolation on platform_settlements
  using (company_id = current_setting('app.company_id', true)::bigint)
  with check (company_id = current_setting('app.company_id', true)::bigint);

grant select, insert, update, delete on platform_settlements to gatobobah_app;
```

**El `grant` no es opcional.** El de [0024] fue `on all tables in schema public`, que es **puntual**:
no hay default privileges, así que cada tabla creada después necesita el suyo. Sin él la migración
pasa, los tests pasan y `make start` pasa —dev sirve como owner, sin RLS ni grants— y en producción
el primer request devuelve `42501 permission denied`.

**`updated_at` por trigger y no por la query**: un upsert que olvide setearlo deja la auditoría
congelada en la fecha de la primera captura (patrón de 0009).

### Lo que NO tiene esta tabla, y por qué

| Ausencia | Por qué |
|---|---|
| Columna con la **base** de cálculo de la comisión | En Uber sigue sin resolverse ([§4-bis](../../docs/plataformas-digitales.md)). No se cierra con una columna que hoy nadie puede llenar bien |
| Entidad de depósito / de documento de pago | Fuera de alcance por spec; se agregan después al mismo costo |
| Estado "provisional / asentado" | Es exactamente la presencia o ausencia de esta fila. Una columna de estado sería el mismo hecho dos veces |

---

## 3. El `Down` de la migración

Reversible y sin data-fix, porque nada depende todavía de estas filas:

```sql
drop table if exists platform_settlements;
-- Va DESPUÉS del drop de la tabla: mientras exista, su FK compuesta depende de este unique.
alter table orders drop constraint if exists orders_id_company_key;
drop index if exists orders_platform_ref_busqueda;
drop index if exists orders_plataforma_sin_folio;
drop index if exists orders_platform_ref;
alter table orders drop constraint if exists orders_platform_ref_rastro;
alter table orders drop constraint if exists orders_platform_ref_solo_plataforma;
alter table orders drop constraint if exists orders_platform_ref_forma;
alter table orders drop column if exists platform_ref_set_at,
                   drop column if exists platform_ref_set_by,
                   drop column if exists platform_order_ref;
```

**Lo que el `Down` destruye, dicho por su nombre**: todos los folios y todas las liquidaciones
capturadas. Es irrecuperable —un folio que no se capturó no se recupera, y el reporte de Uber solo
expone 31 días— así que después de la primera captura real el `Down` deja de ser una operación de
rollback y pasa a ser una pérdida de datos. Vale para la ventana entre aplicar y capturar, y no más.

---

## 4. Lo que sqlc **no** puede ver

`company_id` en `orders` la agregó [0023](../../server/migrations/0023_tenant_columns.sql) con
`EXECUTE format()`, y el parser de sqlc no lee DDL dinámico: **nombrarla en una consulta rompe
`sqlc generate`** con *"column does not exist"* por una columna que sí existe en Postgres. No hace
falta nombrarla — RLS la aplica sola.

En `platform_settlements` la columna sí está en el DDL estático, así que sqlc **sí** la conoce. Aun
así no se nombra en el `where` de ninguna consulta: la aísla RLS, como en las demás tablas
per-tenant.

---

## 5. Tipos de dominio

```go
// domain/platform_ref.go
func NormalizePlatformRef(raw string) (string, error)   // recorta extremos; vacío → ErrValidation
const MaxPlatformRefLen = 64

// domain/settlement.go
type Settlement struct {
    ReportedGross, CommissionAmount        decimal.Decimal
    CommissionPct                          *decimal.Decimal // nil = el documento no la declaró
    DiscountTotal, DiscountPlatform        decimal.Decimal
    Withholdings                           decimal.Decimal
    NetAmount                              decimal.Decimal  // puede ser negativo
    PayoutReference, DocumentRef           string
}
func (s Settlement) Validate() error        // cotas, tasa 0–100, plataforma ≤ total
func (s Settlement) DiscountRestaurant() decimal.Decimal  // derivado, nunca almacenado

// domain/limits.go (agregado)
func ValidSignedMoney(v decimal.Decimal) bool  // |v| ≤ MaxMoney y escala sana
```

`ValidMoney` **no sirve** para el neto: exige no negativo. Ese es el único validador nuevo que esta
feature necesita.
