# Data model: el uso del sistema

Dos tablas. Una guarda lo que pasó, la otra lo que la pantalla lee.

## `usage_events` — el grano fino

| Columna | Tipo | Por qué |
|---|---|---|
| `id` | `bigint generated always as identity` | |
| `occurred_at` | `timestamptz not null default now()` | **La pone el servidor.** El reloj de la tableta no manda, igual que en la venta (spec 008): una tableta con la fecha mal puesta corrompería el día al que cuenta |
| `screen` | `text not null` | Nombre de la lista blanca (`pos`, `caja`, `reportes`…). Lo que no está en la lista no llega hasta aquí |
| `action` | `text` | Nombre de la acción, o **nulo si fue una apertura de pantalla**. Las dos cosas viven en la misma tabla porque son el mismo hecho: alguien hizo algo en una pantalla |
| `role` | `user_role` | El rol de quien lo hizo. **Nulo = sin corte**, cuando ese rol identificaría a una persona (FR-009) |
| `detail` | `jsonb` | **Hoy siempre nulo.** Es la puerta de FR-013: el día que se midan coordenadas, entran aquí como `{"x":…,"y":…}` sin migrar una sola fila |
| `company_id` | `bigint not null default current_setting('app.company_id', true)::bigint` **`references companies(id) on delete cascade`** | La FK se escribe literal, no se da por sobreentendida: las 50 tablas por empresa la llevan y 0066 la pone en la misma línea del default. Darla por obvia es el mismo olvido que 0024 documenta con los grants |

Y un `check` de longitud, que no es celo:

```sql
constraint usage_events_nombres_acotados check (
  char_length(screen) <= 40 and (action is null or char_length(action) <= 60))
```

La lista blanca vive en Go y es la barrera buena, pero estas dos columnas reciben **directo** lo que
viene en el cuerpo del POST. Un front roto —o una ruta futura que se salte la validación— puede
escribir cadenas de kilobytes por evento e inflar justo el volumen que FR-010 promete acotar. Un
control que solo vive en Go se rodea por otra ruta; uno en la columna, no.

**Sin `user_id`, y ese es el punto** (US2, FR-002). No es que no se muestre: no se escribe. Lo que no
se escribió no se puede consultar ni con acceso a la base.

Índice: `(company_id, occurred_at)` — empieza por `company_id` porque RLS agrega ese predicado a
toda consulta del rol de la app, y un índice que arranque por la fecha se queda descartando filas de
otras empresas dentro del scan (ver [0042](../../server/migrations/0042_sales_index.sql)).

**Retención: 14 días.** Es lo que se necesita para poder recalcular un agregado si mañana se cuenta
distinto, y para que las coordenadas del futuro tengan dónde caer. Más allá de eso, lo que vale es
el conteo.

## `usage_daily` — lo único que lee la consola

| Columna | Tipo | Por qué |
|---|---|---|
| `day` | `date not null` | El día del negocio, no el del reloj de nadie |
| `screen` | `text not null` | |
| `action` | `text` | Nulo = fue una apertura de pantalla |
| `role` | `user_role` | Nulo = sin corte |
| `hits` | `bigint not null default 0` | El conteo |
| `company_id` | `bigint not null default …` **`references companies(id) on delete cascade`** | Igual que arriba, escrita literal |

```sql
constraint usage_daily_llave
  unique nulls not distinct (company_id, day, screen, action, role)
```

**`nulls not distinct` no es un detalle**: sin eso, cada evento con `action` nula (toda apertura de
pantalla) o con `role` nulo (todo evento suprimido por k-anonimato) crearía una fila nueva en vez de
sumar, porque en SQL `null <> null`. El agregado dejaría de agregar en silencio y la tabla crecería
como la de eventos. Es de Postgres 15+ y la base corre 16.

**Retención: 13 meses.** Trece y no doce para poder comparar un mes contra el mismo mes del año
pasado, que es la primera comparación que pide un negocio de comida.

## El techo, calculado y verificable (FR-011)

Dimensionado para un local **ocupado**: 100 pedidos/día, que son **30× los 3.1 medidos en producción
el 2026-09-12**. Con el volumen de hoy esto sobra por dos órdenes de magnitud.

| | Filas | Estimado a ojo | **Medido en Postgres 16** |
|---|---|---|---|
| `usage_events`, 2,000 eventos/día × 14 días | 28,000 | ~3.4 MB | **3.9 MB** |
| `usage_daily`, ~135 combinaciones/día × 400 días | 54,000 | ~5.4 MB | **9.4 MB** |
| **Total por empresa** | | ~9 MB | **13.3 MB** |

**La estimación a ojo se quedó corta y por eso está medida.** El índice único de cinco columnas de
`usage_daily` pesa **103 bytes por fila**, no los ~40 que se habían supuesto: él solo son 5.4 MB. Lo
midió la revisión de arquitectura sembrando exactamente estos volúmenes, no calculándolos.

**Y hay un costo que un `insert` limpio no muestra: el churn del `upsert`.** Cada fila del día
recibe un `update` por evento, y en Postgres cada update deja la versión vieja muerta. Medido: las
135 filas de un día, con 2,000 incrementos encima, pasaron de 64 kB a **232 kB** antes de que
autovacuum las tocara — y con ~54,000 filas vivas el umbral de autovacuum tarda **días** en
dispararse.

Por eso la ingesta **pre-agrega dentro del lote**: agrupa por `(screen, action, role)` antes de
escribir y emite un solo `update … set hits = hits + n` por combinación, en vez de uno por evento.
Con lotes de hasta 50, son hasta 50× menos escrituras físicas. La transacción ya existe; lo único
que cambia es cuántos `update` salen de ella.

**Techo declarado: menos de 25 MB por empresa**, contra los 13.3 MB medidos del caso limpio y el
margen que el churn necesita entre pasadas de autovacuum. Para comparar: la base **completa** de
producción —meses de operación, pedidos, productos, usuarios— pesa hoy **18 MB**, y la VM tiene
18 GB libres.

Lo verifica un test que siembra un año de uso **con el patrón de escritura real** —muchos updates
pequeños sobre las filas del día, como los produce el endpoint— y mide `pg_total_relation_size`. Un
test que inserte el total de golpe mediría un escenario que la operación nunca produce y pasaría en
verde mintiendo.

## Permisos: quién escribe, quién lee, quién no alcanza

| Rol | `usage_events` | `usage_daily` |
|---|---|---|
| `gatobobah_app` (el POS) | `insert` | `select`, `insert`, `update` |
| `gatobobah_platform` (la consola) | **nada** | `select` |
| `gatobobah` (dueño) | todo — es quien recorta | todo |

Tres cosas que hay que decir en voz alta:

1. **El `update` del rol de la app es para el `upsert`**, no para editar historia: `insert … on
   conflict do update set hits = usage_daily.hits + n`, con la `n` que trae pre-agregado el lote.
   Postgres exige `select` sobre la columna que se lee en el `set`, y por eso el `select` también
   está.
2. **La consola no toca `usage_events`.** Lee conteos, no hechos. Y cuando el grano fino lleve
   coordenadas, seguirá sin tocarlo salvo que alguien lo decida a propósito.
3. **El grant no es opcional**: el de [0024](../../server/migrations/0024_tenant_rls.sql) fue
   puntual (`on all tables`, sin default privileges), así que una tabla nueva no hereda nada. Sin
   estas líneas la migración pasa, los tests pasan, `make start` pasa —dev sirve como owner— y en
   producción el primer evento devuelve 42501.

## RLS: las dos políticas, y por qué son dos

```sql
-- Cada empresa ve lo suyo. Igual que las otras ~30 tablas.
create policy tenant_isolation on usage_events  using (…) with check (…);
create policy tenant_isolation on usage_daily   using (…) with check (…);

-- Y la consola ve TODO el agregado, acotada a select y a su rol.
create policy plataforma_lee_todo_el_uso on usage_daily
  for select to gatobobah_platform using (true);
```

La segunda es la gemela de `plataforma_lee_todas_las_empresas` de la 0068, y por la misma razón
medida: **`select` a secas no alcanza**, porque `tenant_isolation` también le aplica al rol de
plataforma y la consola vería el uso de **una empresa de dos** —o de ninguna, porque su conexión no
fija `app.company_id`—. Sin esta política, FR-008 («unificado o por empresa») es imposible.

**Nunca `BYPASSRLS`**, y el arranque del binario ya lo rechaza (`store.AssertPlatformGrants`).

## La lista blanca NO es una tabla

Las pantallas y las acciones que vale la pena contar viven en `domain`, en Go, no en un catálogo en
la base.

**Por qué**: una tabla de catálogo tendría que sembrarse por migración, mantenerse sincronizada con
las rutas del front y, el día que alguien agregue una pantalla, fallaría **en producción** con una
FK en vez de fallar al compilar. La lista en el código se revisa en el diff, viaja con el binario
que la valida y no tiene estado que se desincronice.

Lo que sí queda en la base es la consecuencia: solo entran valores de esa lista, así que el número
de combinaciones distintas está acotado por construcción y el agregado no se puede inflar.

## El recorte, y con qué conexión corre

Dentro del binario que ya está corriendo —la VM tiene 300 MB de RAM libres y agregar un cron es otra
pieza que se puede olvidar.

**Corre al ARRANCAR y luego cada 24 horas**, y lo primero no es un detalle: un `time.Ticker` de 24 h
se reinicia con el proceso, y este repo redespliega en **cada merge**. Si el binario no vive un día
entero seguido —el modo normal mientras hay desarrollo activo— el recorte **no dispara nunca**, en
silencio, y FR-010 queda siendo una promesa que nadie cumple. Lo encontró la revisión de
arquitectura; el ticker solo, que era lo obvio, era justo lo que no funcionaba aquí.

La goroutine cuelga del `context` que ya se cancela en el apagado, para que tenga condición de
término (principio II).

Corre con una conexión **de dueño abierta para eso y cerrada al terminar**, no con la de servicio:
el rol de la app está bajo RLS y solo borraría las filas de *su* empresa, así que el recorte se
quedaría a medias en una instalación multi-empresa — y sin que nada fallara. Es el mismo patrón que
`main.go` ya usa con el pool de bootstrap: se abre, se usa, se cierra.

## Lo que este modelo no resuelve, y hay que saberlo

- **El día es el del servidor en UTC**, no el día del negocio con su zona. Para contar aperturas de
  pantalla eso basta; si algún día se quiere cruzar uso contra ventas por día de negocio, hay que
  pasar `day` por la zona de la empresa, y eso **sí** exigiría recalcular lo escrito.
- **Una empresa que cambia de plantilla** cambia qué cortes se suprimen, y las filas viejas
  conservan la decisión que se tomó al escribirlas. Es a propósito: la alternativa es reescribir el
  pasado.
