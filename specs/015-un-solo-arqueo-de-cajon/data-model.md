# Data Model: un solo arqueo de cajón

No hay tablas nuevas. Cambian dos columnas de configuración que ya existen, se agregan dos a la fila
del conteo y un booleano a los ajustes del negocio.

## Cambia: `session_cash_counts` — el conteo pasa a ser el arqueo

La fila del conteo ya guarda **lo que el operador declaró** (`total`, y `manual_reason` si no contó
pieza por pieza). Le falta contra qué se comparó.

| Columna | Tipo | Por qué |
|---|---|---|
| `expected` | `numeric(10,2)` **nullable** | Lo que el cajón debía tener en el momento del cierre. Nulo en el momento `apertura`: al abrir no hay nada que esperar — el fondo *es* lo que se contó |
| `difference` | `numeric(10,2)` **generada** `total - expected` | Igual que `register_session_totals.difference`: la resta la hace Postgres para que nadie la escriba a mano y para que no puedan existir dos versiones del mismo faltante |

**Se guarda y no se recalcula**, por la misma razón que `order_lines.unit_price`: el esperado sale de
`order_payments`, y una venta cancelada o reembolsada después del cierre lo movería. Un arqueo
firmado tiene que poder reconstruirse tal como se firmó. Es una de las **dos** piezas irreversibles
de esta feature —la otra es el snapshot del renglón, más abajo— y por eso su migración va primero.

**Un `expected` nulo en un cierre significa "corte anterior a esta feature"**, y la pantalla lo lee
como siempre: por método, con las cifras que quedaron guardadas. No se rellena con un cero — cero
diría que el cajón debía estar vacío.

**Y esa regla se fija en el esquema, no en la disciplina de la app** (lo pidió la revisión de
arquitectura):

```sql
constraint session_cash_counts_expected_por_momento check (
  (moment = 'apertura' and expected is null) or (moment = 'cierre' and expected is not null)
) not valid;
```

`not valid` perdona las filas que ya existen sin escanear la tabla, pero obliga a **todo insert
nuevo**. Sin él, un `CloseSession` que algún día olvide setear el esperado deja un nulo que se lee
igual que un corte viejo: el bug se disfraza de historia. Con él, falla ruidoso con un `23514`.

## Cambia: `register_session_totals` — el renglón guarda si ese método tocaba el cajón

| Columna | Tipo | Por qué |
|---|---|---|
| `affects_cash_drawer` | `boolean not null` | **Snapshot.** Hoy el flag se reconstruye en vivo por join a `payment_methods`, y es inofensivo porque nadie puede cambiarlo. FR-007 le da un `PATCH` por primera vez: el día que el dueño apague «el efectivo de Didi llega al cajón», todo corte cerrado antes se reagruparía con el flag de hoy. Las cifras no cambian, pero la **forma del reporte** sí, y eso es reescribir el pasado — el mismo defecto que la constitución nombra para la categoría de un producto |

Se puebla en el mismo `insert` que ya corre al cerrar: el dato está ahí, viene de la consulta del
esperado. Y con el flag en la misma fila, el invariante que hoy solo protege el código se puede
expresar en la base:

```sql
constraint register_session_totals_cajon_no_se_declara check (
  not affects_cash_drawer or declared = expected
) not valid;
```

Es la mitad de FR-003 que ningún código puede garantizar solo: si un camino futuro escribe un
declarado propio para un método de cajón, la base lo rechaza en vez de dejar que el faltante
reaparezca repartido.

## Cambia: `payment_methods` — dos interruptores que ya existen y nadie podía mover

| Columna | Qué decide | Estado hoy |
|---|---|---|
| `is_active` | Si el método se ofrece para cobrar | Existe, se lee en todas las consultas, **ningún endpoint la escribe** |
| `affects_cash_drawer` | Si el dinero de ese método está en el cajón y por lo tanto se cuenta | Existe y se lee; **ningún endpoint la escribe** |

`kind` **no se toca** y sigue significando otra cosa: identifica al único método dueño del fondo de
apertura y de los movimientos de caja. Confundir los dos ya costó: sumar el fondo a cada método que
toca el cajón lo contó cuatro veces e inventó $4,500 de faltante.

**La granularidad "depende de la plataforma" ya está resuelta** porque cada plataforma tiene su
propio método de efectivo. Configurar Rappi distinto de DiDi es mover el interruptor de
`Rappi efectivo`, no un concepto nuevo.

## Cambia: `business_settings` — el interruptor del arqueo ciego

| Columna | Tipo | Por qué |
|---|---|---|
| `blind_cash_count` | `boolean not null default false` | Si quien cuenta ve lo que el sistema espera. **Default en falso**: el comportamiento de hoy es el que ya está implementado y probado (FR-005 de la 003), así que la migración no cambia lo que el negocio ve |

En pantalla el interruptor de cajón se llama **«Va al cajón»** (encabezado de columna) y su ayuda
dice «su efectivo entra al cajón», que es la frase del spec. No «Afecta el cajón»: *afecta* no dice
ni la dirección ni la consecuencia, y el texto de la interfaz es para quien opera el negocio.

Va aquí y no en `cash_registers` ni en `users` porque es política del negocio: por caja no
significaría nada, y por usuario invitaría a exentarse a sí mismo, que es justo lo que el control
evita.

## Cómo se lee el arqueo, después de esto

| Qué | De dónde sale |
|---|---|
| Lo que el cajón debía tener | `session_cash_counts.expected` del momento `cierre` |
| Lo que se contó | `session_cash_counts.total` (y sus renglones, si se contó por piezas) |
| La diferencia del cajón | La columna generada. **Una sola** |
| Cuánto entró por cada canal en efectivo | `register_session_totals` de los métodos con `affects_cash_drawer`, como **reporte**: su `declared` queda igual a su `expected` porque nadie declara por método |
| La diferencia de un método que no toca el cajón | Su renglón de `register_session_totals`, como hoy |
| La diferencia total de un corte, en el histórico | Σ de las diferencias de los métodos que **no** tocan el cajón, **más** la diferencia del conteo de cierre. Hoy es solo la primera suma, y con este cambio esa suma da cero para el efectivo: si no se agrega la segunda, el histórico muestra cortes cuadrados que no cuadran |

Ese último renglón es el que hay que no olvidar: es un cambio de consulta, no de esquema, y es donde
un descuido deja el faltante invisible justo en la pantalla donde alguien lo audita.

**Va como segunda subconsulta correlacionada, no como join** — igual que la que ya está escrita:

```sql
+ coalesce((select difference from session_cash_counts c
            where c.session_id = s.id and c.moment = 'cierre'), 0)
```

Con un join, cada turno trae hasta dos filas de conteo (apertura y cierre) y N de totales por
método: es el patrón "un agregado unido a dos tablas 1:N" que `AGENTS.md` ya documenta para
`order_payments`/`order_lines`, y **duplicaría cada diferencia** por el número de conteos. Queda
escrito aquí porque este archivo existe justo para que ese defecto no se vuelva a escribir.

## Lo que NO cambia

- `register_session_totals` conserva su `not null` en `declared` y su columna generada; lo único que
  gana es el snapshot del flag. Se
  descartó volverla nulable para poder decir "no se declara por método": el radio de cambio —sqlc, el
  dominio, la vista del corte, el histórico, el front— no se justifica para expresar algo que la fila
  del conteo ya dice. El detalle está en [research.md](./research.md) §1.
- El catálogo de denominaciones y los renglones del conteo, intactos.
- La conciliación de la liquidación de plataforma, intacta.

## La migración

`0067`, y lleva el `set local lock_timeout` que ya trae la 0066. No es ceremonia: agregar una columna
generada `stored` **reescribe la tabla** bajo `ACCESS EXCLUSIVE`. Hoy es gratis porque
`session_cash_counts` no tiene datos en producción, pero el día que la 0066 llegue allá deja de
serlo, y una migración sin lock_timeout que espera un lock detiene la caja.

El `boolean not null default false` de `business_settings` no reescribe nada: desde PG11 un default
constante es metadata.
