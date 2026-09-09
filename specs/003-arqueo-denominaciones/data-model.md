# Data Model: Conteo de efectivo por denominaciones

## Tablas nuevas

### `cash_denominations` — el catálogo

Qué piezas de dinero existen en cada moneda.

| Columna | Tipo | Por qué |
|---|---|---|
| `id` | identidad | |
| `currency` | `char(3)` | Las denominaciones de una moneda no sirven para otra (FR-011). `register_sessions.currency` ya existe |
| `value` | `numeric(10,2)` con `check (value > 0)` | El valor de la pieza. Numeric y no entero de centavos: es el mismo tipo con el que el resto del sistema maneja dinero, y mezclarlos invita a un error de escala. El `check` porque una fila sembrada en 0 o en negativo rompe en silencio toda suma que dependa de que cada pieza vale dinero real |
| `is_coin` | `boolean` | Moneda o billete. Solo sirve para agrupar en pantalla, pero agruparlas es lo que hace que el operador encuentre la que busca |
| `sort_key` | `int` | El orden en que se presentan. De mayor a menor, que es como se cuenta un cajón |
| `is_active` | `boolean` | Una denominación se retira de circulación sin que desaparezcan los arqueos que la usaron |

**Únicas**: `(currency, value)`. No es una tabla per-tenant: los billetes de México son los mismos
para todos los negocios, así que **no lleva `company_id` ni RLS**. Es la primera tabla del sistema
así, y por eso conviene decirlo en su migración.

**Y por eso mismo, el rol de la app NO la escribe**: `revoke insert, update, delete on
cash_denominations from gatobobah_app`. Sin `company_id` no hay nada que aisle una fila de otra, así
que un endpoint futuro mal filtrado —o un bug— que apague el billete de $1000 lo apagaría para
TODAS las empresas de la base. El catálogo se cambia como operación deliberada de owner, igual que
`units`. Que siga global **no cierra la puerta** a partirlo por empresa: `payment_methods` nació así
en `0002` y `0037` lo partió duplicando filas, al mismo costo que si hubiera nacido partido.

**Siembra**: solo MXN. Monedas de 0.50, 1, 2, 5 y 10; billetes de 20, 50, 100, 200, 500 y 1000.
Fuera quedan las de 10¢ y 20¢ (no circulan) y la moneda de $20 (conmemorativa) — si aparece una, se
captura por el camino del total manual.

### `session_cash_counts` — el conteo

Un conteo pertenece a una sesión y a un momento.

| Columna | Tipo | Por qué |
|---|---|---|
| `company_id` | `bigint not null default current_setting('app.company_id', true)::bigint` → `companies` | Sin ella no hay predicado que comparar contra el GUC del tenant, y **la tabla no puede llevar RLS**. Es el patrón de toda tabla de negocio desde `0024`; omitirlo aquí reproduce en una tabla nueva la fuga que `0024`, `0040`, `0041` y `0061` cerraron a posteriori |
| `session_id` | `bigint` → `register_sessions` | Ver la FK compuesta de abajo. `on delete cascade`: un corte borrado no deja conteos huérfanos |
| `moment` | enum `apertura` \| `cierre` | Los dos momentos en que se cuenta |
| `total` | `numeric(10,2)` | Lo que sumó el conteo. **Derivado, pero guardado**: recalcularlo después exigiría que el catálogo nunca cambie de valor, y una denominación retirada rompería los arqueos viejos |
| `manual_reason` | `text` nullable | Por qué se capturó el total a mano en vez de contar. Nulo = se contó |
| `created_by` | `bigint` → `users` | Quién contó |
| `created_at` | `timestamptz` | |

**Única**: `(session_id, moment)` — un turno tiene un conteo de apertura y uno de cierre, no más.
Y **`unique (id, company_id)`**, que no sirve para prevenir duplicados sino para ser el destino de la
FK compuesta de los renglones (mismo truco que `products_id_company_key` en `0040`).

**FK COMPUESTA hacia el turno**: `foreign key (company_id, session_id) references register_sessions
(company_id, id) on delete cascade`, apoyada en `register_sessions_tenant_key` que ya existe desde
`0061`. No es adorno: los chequeos de integridad referencial de Postgres **saltan RLS** por diseño,
así que con una FK simple un data-fix corriendo como owner puede colgar un conteo con `company_id`
de una empresa del `session_id` de otra. La fila queda invisible para las dos y el turno ajeno
aparece con piezas que nadie contó ahí. Es la razón textual de `0041` y de `0061`.

**RLS y grant**: `enable row level security` + policy `tenant_isolation`, y
`grant select, insert on session_cash_counts to gatobobah_app` — **sin `update` ni `delete`**: un
arqueo firmado no se edita ni se borra desde la app.

### `session_cash_count_lines` — las piezas

| Columna | Tipo | Por qué |
|---|---|---|
| `company_id` | `bigint not null default current_setting('app.company_id', true)::bigint` → `companies` | Igual que su padre, y por lo mismo: sin ella no hay RLS que aplicar |
| `count_id` | `bigint` → `session_cash_counts` | FK **compuesta** `(company_id, count_id)` contra el `unique (id, company_id)` del padre, `on delete cascade` |
| `denomination_id` | `bigint` → `cash_denominations` | **`on delete restrict`, explícito.** El catálogo se retira con `is_active`, no borrando; pero si alguien borra una fila —limpiando una semilla mal cargada al agregar otra moneda— un `cascade` copiado por inercia del renglón de arriba se llevaría en silencio piezas de arqueos ya firmados. Dinero contado y declarado no desaparece por una limpieza de catálogo |
| `pieces` | `int` con `check (pieces > 0)` | **Solo se guarda lo que hay.** Una denominación en cero no genera renglón: guardar once ceros por arqueo es ruido, y "no hay" y "no se capturó" son lo mismo aquí |

**Única**: `(count_id, denomination_id)`. Con RLS, policy y
`grant select, insert on session_cash_count_lines to gatobobah_app`, sin `update` ni `delete`.

## Lo que NO cambia, y por qué importa decirlo

- **`register_sessions.opening_cash`** y **`register_session_totals.declared`** siguen recibiendo el
  total. El conteo cambia de dónde sale ese número, no dónde vive. Así el corte, los reportes y
  `difference` —que es columna generada— siguen funcionando sin tocarlos.
- **Los arqueos ya cerrados** no ganan filas y por lo tanto no ganan desglose. Un corte sin conteo se
  muestra como siempre (FR-008).
- **Los métodos que no son efectivo** siguen declarándose con una cifra.

## Reglas que el dominio debe hacer cumplir

| Regla | Dónde vive | Qué rompe si falta |
|---|---|---|
| Piezas es un entero ≥ 0 | dominio, función pura | Un negativo restaría del cajón y el arqueo cuadraría contra una cifra imposible |
| El total = Σ piezas × valor, redondeado a 2 | dominio, con test | Es la única razón de ser de la feature: si el servidor no recalcula, el operador sigue sumando |
| El total no pasa los topes de dinero | `domain.ValidMoney`, ya existe | Un desbordamiento del `numeric(10,2)` sale como 500 en vez de un error accionable |
| Solo denominaciones de la moneda de la sesión | servicio | Contar dólares en un arqueo en pesos daría un total sin significado |
| O hay conteo, o hay motivo — nunca ninguno | servicio, al guardar | FR-016. Sin esto vuelve a haber cifras de efectivo sin explicación, que es el problema original |
| Los dos caminos no coexisten | servicio | FR-015. Dos cifras del mismo dinero y nadie sabe cuál manda |

## Por qué el total se guarda aunque sea derivable

Es la decisión menos obvia del modelo. Recalcularlo desde las piezas exigiría que
`cash_denominations.value` **nunca** cambie, y una denominación retirada o corregida reescribiría en
silencio arqueos ya firmados. El mismo razonamiento por el que el nombre de folio del pedido se
guarda en vez de derivarse: un número que ya se declaró y se firmó no puede cambiar de significado
con un despliegue.
