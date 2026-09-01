# Data Model: Conteo de efectivo por denominaciones

## Tablas nuevas

### `cash_denominations` — el catálogo

Qué piezas de dinero existen en cada moneda.

| Columna | Tipo | Por qué |
|---|---|---|
| `id` | identidad | |
| `currency` | `char(3)` | Las denominaciones de una moneda no sirven para otra (FR-011). `register_sessions.currency` ya existe |
| `value` | `numeric(10,2)` | El valor de la pieza. Numeric y no entero de centavos: es el mismo tipo con el que el resto del sistema maneja dinero, y mezclarlos invita a un error de escala |
| `is_coin` | `boolean` | Moneda o billete. Solo sirve para agrupar en pantalla, pero agruparlas es lo que hace que el operador encuentre la que busca |
| `sort_key` | `int` | El orden en que se presentan. De mayor a menor, que es como se cuenta un cajón |
| `is_active` | `boolean` | Una denominación se retira de circulación sin que desaparezcan los arqueos que la usaron |

**Únicas**: `(currency, value)`. No es una tabla per-tenant: los billetes de México son los mismos
para todos los negocios, así que **no lleva `company_id` ni RLS**. Es la primera tabla del sistema
así, y por eso conviene decirlo en su migración.

**Siembra**: solo MXN. Monedas de 0.50, 1, 2, 5 y 10; billetes de 20, 50, 100, 200, 500 y 1000.
Fuera quedan las de 10¢ y 20¢ (no circulan) y la moneda de $20 (conmemorativa) — si aparece una, se
captura por el camino del total manual.

### `session_cash_counts` — el conteo

Un conteo pertenece a una sesión y a un momento.

| Columna | Tipo | Por qué |
|---|---|---|
| `session_id` | `bigint` → `register_sessions` | `on delete cascade`: un corte borrado no deja conteos huérfanos |
| `moment` | enum `apertura` \| `cierre` | Los dos momentos en que se cuenta |
| `total` | `numeric(10,2)` | Lo que sumó el conteo. **Derivado, pero guardado**: recalcularlo después exigiría que el catálogo nunca cambie de valor, y una denominación retirada rompería los arqueos viejos |
| `manual_reason` | `text` nullable | Por qué se capturó el total a mano en vez de contar. Nulo = se contó |
| `created_by` | `bigint` → `users` | Quién contó |
| `created_at` | `timestamptz` | |

**Única**: `(session_id, moment)` — un turno tiene un conteo de apertura y uno de cierre, no más.

### `session_cash_count_lines` — las piezas

| Columna | Tipo | Por qué |
|---|---|---|
| `count_id` | `bigint` → `session_cash_counts` | `on delete cascade` |
| `denomination_id` | `bigint` → `cash_denominations` | |
| `pieces` | `int` con `check (pieces > 0)` | **Solo se guarda lo que hay.** Una denominación en cero no genera renglón: guardar once ceros por arqueo es ruido, y "no hay" y "no se capturó" son lo mismo aquí |

**Única**: `(count_id, denomination_id)`.

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
