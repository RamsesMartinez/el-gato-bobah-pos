# Research — 008 · La fecha la da el reloj, el folio lo da el turno

Cinco decisiones técnicas. Cada una dice qué se eligió, por qué, y qué se descartó.

---

## R1 · Cómo numerar el folio por turno sin perder la garantía de concurrencia

**Contexto.** Hoy `NextDailyNumber` hace un upsert sobre `order_counters`, cuya PK es
`(company_id, business_date)` desde [0023](../../server/migrations/0023_tenant_columns.sql):

```sql
insert into order_counters (business_date, last_number) values ($1, 1)
on conflict on constraint order_counters_pkey do update set last_number = last_number + 1
returning last_number;
```

Lo que hace segura la numeración concurrente **no es la consulta, es el candado de fila**: el
`on conflict do update` bloquea esa fila hasta el commit, y la lectura de nombres ya repartidos
corre dentro de la misma transacción, protegida por ese mismo candado. Está documentado en el
comentario de `FolioNamesUsedToday`. Cualquier diseño que cambie de qué depende el contador tiene
que conservar exactamente esa propiedad.

**Decisión: tabla nueva `folio_counters`, con PK `(company_id, register_session_id)`.**
La consulta cambia solo de tabla y de llave; la mecánica del candado es idéntica.

`order_counters` **se deja en pie y sin tocar**, aunque quede sin usar.

**Por qué una tabla nueva y no cambiarle la PK a la existente:**

- **El rollback por imagen sigue funcionando.** El repo ya trata "el binario viejo arranca contra
  el esquema nuevo" como requisito de un deploy seguro. Si le cambiamos la PK a `order_counters`,
  el binario anterior deja de poder numerar y la caída se vuelve irreversible sin restaurar.
- **El `Down` es trivialmente correcto**: `drop table folio_counters`. Un `Down` que devuelve una
  PK a su estado anterior tiene que además reconstruir las filas, y eso ya no es reversible de
  verdad.
- Cuesta una tabla muerta. Se borra en una migración posterior, cuando la feature lleve un ciclo
  en producción; queda anotado en la propia migración.

**Descartado:** agregar `register_session_id` a `order_counters` y mover la PK. Más barato en
líneas, pero rompe el rollback y deja un `Down` que miente.

**Semilla obligatoria (FR-006).** La tabla nace poblada desde los pedidos que ya existen:

```sql
insert into folio_counters (company_id, register_session_id, last_number)
select company_id, register_session_id, max(daily_number)
from orders where register_session_id is not null group by 1, 2;
```

Sin esto, el turno abierto del ambiente de pruebas —158 pedidos— repartiría el número 1 en su
siguiente venta y chocaría con uno que ya existe. Se siembran también los turnos cerrados: cuestan
nada y evitan razonar sobre cuáles hacía falta.

**RLS y grants.** La tabla nueva necesita su política `tenant_isolation` y su
`grant select, insert, update` explícito al rol `gatobobah_app`: el grant de
[0024](../../server/migrations/0024_tenant_rls.sql) fue puntual, no hay *default privileges*, y una
tabla sin grant falla con `42501` en el primer request de producción mientras en local —que conecta
como owner— nunca se ve. Sin secuencia que otorgar: la tabla no tiene columna identity.

**`company_id`.** Se declara normal en el `create table`. La limitación de sqlc solo aplica a las
~30 tablas a las que 0023 se lo agregó con `EXECUTE format()`; una tabla nueva lo declara y sqlc lo
ve.

---

## R2 · Cómo corregir las ventas históricas de forma reversible

**Contexto.** FR-007 pide que la fecha de una venta signifique lo mismo en todo el histórico.
Medido antes de decidir:

| Empresa | Filas que cambian | Total |
|---|---|---|
| El Gato Bobah (en operación) | **0** | 31 |
| Bobah Pruebas (misma base) | 2 | 61 |
| Ambiente de pruebas (otra base) | 158 | ~169 |

**Decisión: migración propia, con respaldo de los valores anteriores en una tabla, y `Down` que
los restaura.**

```sql
create table orders_business_date_fix (order_id bigint primary key, previous_date date not null);
insert into orders_business_date_fix select o.id, o.business_date from orders o … where difiere;
update orders o set business_date = … from orders_business_date_fix f where f.order_id = o.id;
```

El `Down` reasigna desde `orders_business_date_fix` y borra la tabla, así que `Up → Down → Up`
deja la base igual. Una corrección de datos sin respaldo no tiene marcha atrás, y este repo ya
tuvo una migración que declaraba "sin rollback" y resultó ser un bloqueador ([0052]).

**Va en migración aparte de la de `folio_counters`**: son dos cambios con causas distintas y
`Down` distintos; juntos, revertir uno obliga a revertir el otro.

**Qué NO toca**, y hay que probarlo: `register_session_id`, `daily_number`, `folio_name`, y toda
cifra de un arqueo. El arqueo agrupa por turno y no lee `business_date`
([cash.sql:176](../../server/queries/cash.sql)), que es lo que hace seguro este cambio.

**Descartado:** no corregir. Dejaría la columna significando la fecha del turno antes de cierto día
y la del reloj después, sin nada que lo diga. Descartado también un script suelto tipo
`docs/reorg/`: la corrección tiene que correr en los tres ambientes sin que nadie se acuerde.

---

## R3 · De dónde saca la fecha el servicio de pedidos

**Decisión: el mismo patrón que ya usa `SalesService.Location`** — `GetBusinessTimezone` desde el
store, `domain.LoadBusinessLocation` para resolver, y `domain.BusinessDate(now, loc)` para el día.
Las tres piezas ya existen y no se rehacen.

El fallback ya está resuelto y es el correcto: zona vacía o inválida cae al **default del
producto**, no a UTC. Caer a UTC corre la fecha seis horas y se ve plausible, que es el peor modo
de fallo posible.

**Costo:** una lectura de una fila por venta creada. Se acepta: es un `select` de una fila por
`company_id` y el camino ya hace varias lecturas más caras (catálogo, precios, depleción). No se
cachea — sería config para un valor que casi nunca cambia, y un caché mal invalidado aquí archiva
ventas en el día equivocado, que es justo el defecto que venimos a cerrar.

---

## R4 · Las ventas de un corte

**Decisión: consulta nueva `SessionSales` + su gemela `CountSessionSales`, con el mismo `where`.**

- Filtra por `o.register_session_id = $1`. No por ventana de tiempo: es la misma razón por la que
  `ExpectedByMethodForSession` ya lo hace así.
- Devuelve folio (número y nombre), hora, estado, tipo y total.
- Tope de 200 filas, el `MaxListLimit` que ya existe. El `Count` gemelo viaja siempre, para que la
  pantalla pueda decir cuántas hay en total (FR-011). **Un recorte silencioso se lee como "esto es
  todo"**, y por eso el conteo no es opcional.
- El total que encabeza la sección **excluye canceladas y reembolsadas** y **excluye propinas**, y
  lo dice en pantalla. Es el principio de dinero: cada peso se clasifica una vez y lo que no es
  ingreso no entra al total. Las canceladas siguen listadas, con su estado.

**Descartado:** derivar la lista de la de Ventas con un filtro por turno. Es exactamente lo que el
dueño descartó, y por la razón correcta: convivir con el filtro de fechas deja llegar a una
pantalla vacía sin explicación.

---

## R5 · Cómo se entera quien opera de que su turno es viejo

**Decisión: lo decide el servidor y se cuelga de `GET /cash-status`, que el POS ya consulta.**

Hoy devuelve `{"open": true}`. Pasa a devolver también desde cuándo está abierto el turno de la
caja que recibe ventas y si su día de apertura ya no es hoy.

**El servidor decide, no la pantalla.** La zona del negocio vive en el servidor; que la tableta
compare fechas con su propio reloj es precisamente la familia de defectos que este repo ya tiene
identificada. La pantalla solo pinta lo que le dicen.

**Se cuelga de un endpoint que ya se consulta** en vez de agregar uno: el POS ya pregunta por el
estado de caja para decidir si puede cobrar, así que el aviso no cuesta un viaje más ni un estado
nuevo que sincronizar.

**No bloquea (FR-013).** Es un aviso con la acción de ir a cerrar el turno. Un negocio en operación
prefiere una fecha corrida a no poder cobrar; y bloquear el cobro por un turno viejo convertiría un
descuido administrativo en una caja parada.

**Compara días, no horas transcurridas.** Un turno abierto ayer a las 23:00 ya es de ayer aunque
lleve una hora abierto. Contar horas dejaría pasar justo el caso del turno nocturno.
