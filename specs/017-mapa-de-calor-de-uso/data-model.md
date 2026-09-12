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
| `company_id` | `bigint not null default current_setting('app.company_id', true)::bigint` | Igual que el resto del sistema; RLS lo aplica solo |

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
| `company_id` | `bigint not null default …` | |

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

| | Filas | Tamaño estimado |
|---|---|---|
| `usage_events`, 2,000 eventos/día × 14 días | 28,000 | ~3.4 MB (fila ~80 B + índice ~40 B) |
| `usage_daily`, ~135 combinaciones/día × 400 días | 54,000 | ~5.4 MB |
| **Total por empresa** | | **~9 MB** |

**Techo declarado: menos de 15 MB por empresa.** Para comparar: la base **completa** de producción
—meses de operación, pedidos, productos, usuarios— pesa hoy **18 MB**, y la VM tiene 18 GB libres.

Lo verifica un test que siembra un año de uso simulado y mide `pg_total_relation_size` de las dos
tablas. Un techo que solo está escrito en un documento no es verificable, y este spec pide que lo
sea.

## Permisos: quién escribe, quién lee, quién no alcanza

| Rol | `usage_events` | `usage_daily` |
|---|---|---|
| `gatobobah_app` (el POS) | `insert` | `select`, `insert`, `update` |
| `gatobobah_platform` (la consola) | **nada** | `select` |
| `gatobobah` (dueño) | todo — es quien recorta | todo |

Tres cosas que hay que decir en voz alta:

1. **El `update` del rol de la app es para el `upsert`**, no para editar historia: `insert … on
   conflict do update set hits = usage_daily.hits + 1`. Postgres exige `select` sobre la columna
   que se lee en el `set`, y por eso el `select` también está.
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

Una vez al día, dentro del binario que ya está corriendo —la VM tiene 300 MB de RAM libres y
agregar un cron es otra pieza que se puede olvidar.

Corre con una conexión **de dueño abierta para eso y cerrada al terminar**, no con la de servicio:
el rol de la app está bajo RLS y solo borraría las filas de *su* empresa, así que el recorte se
quedaría a medias en una instalación multi-empresa — y sin que nada fallara.

## Lo que este modelo no resuelve, y hay que saberlo

- **El día es el del servidor en UTC**, no el día del negocio con su zona. Para contar aperturas de
  pantalla eso basta; si algún día se quiere cruzar uso contra ventas por día de negocio, hay que
  pasar `day` por la zona de la empresa, y eso **sí** exigiría recalcular lo escrito.
- **Una empresa que cambia de plantilla** cambia qué cortes se suprimen, y las filas viejas
  conservan la decisión que se tomó al escribirlas. Es a propósito: la alternativa es reescribir el
  pasado.
