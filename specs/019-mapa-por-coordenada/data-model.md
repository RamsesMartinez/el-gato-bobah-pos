# Data model: los toques por zona

Una tabla. De conteos, no de toques.

## `usage_touches_daily`

| Columna | Tipo | Por qué |
|---|---|---|
| `day` | `date not null` | El día del **negocio**, con su zona, calculado en Go — igual que la venta y que la 017 |
| `screen` | `text not null` | De la lista corta de pantallas instrumentadas |
| `orientation` | `text not null` | `horizontal` o `vertical`. **No se mezclan** (FR-016): la misma celda es otro lugar en cada forma |
| `cell` | `smallint not null` | El número de celda, `0..83`. La rejilla es 12×7 en horizontal y 7×12 en vertical |
| `role` | `user_role` | Nulo = **sin corte**, con la misma regla que la 017 |
| `hits` | `bigint not null default 0` | El conteo |
| `company_id` | `bigint not null default current_setting('app.company_id', true)::bigint` **`references companies(id) on delete cascade`** | Como toda tabla por empresa |

```sql
constraint usage_touches_llave
  unique nulls not distinct (company_id, day, screen, orientation, cell, role)
constraint usage_touches_celda_en_rango check (cell between 0 and 83)
-- Los mismos tres que su gemela `usage_daily`, y por las mismas razones. Se le habían caído al
-- copiar el patrón, y ésa es exactamente la forma en que un control se pierde: nadie lo quita a
-- propósito, se olvida al escribir la tabla siguiente.
constraint usage_touches_orientacion check (orientation in ('horizontal', 'vertical'))
constraint usage_touches_hits_no_negativo check (hits >= 0)
constraint usage_touches_pantalla_acotada check (char_length(screen) <= 40)
```

El de la orientación no es celo: sin él, una versión vieja de la tableta que mande `'landscape'` o
`'Horizontal'` crea un **balde invisible**. La fila entra, pasa el rango de la celda —0..83 vale en
las dos formas— y la consola, que solo pide `horizontal` y `vertical`, nunca la muestra. Los toques
de esa zona desaparecen sin un solo error.

El índice de la llave arranca por `company_id`, así que sirve para mirar **una** empresa. Para
mirarlas **todas juntas** hace falta el suyo, igual que en la 0069:

```sql
create index usage_touches_daily_dia on usage_touches_daily (day);
```

Y la tabla nace con `fillfactor = 70`: sus filas se **actualizan** muchas veces al día —una por cada
lote que cae en esa celda— y en Postgres cada `update` deja muerta la versión vieja. Dejar espacio
libre en la página permite que esas versiones nuevas quepan al lado (actualización HOT) en vez de
ensuciar el índice. Medido en la 017: sin eso, las filas calientes de un día crecen 3.6× antes de
que autovacuum llegue.

**No existe una entidad «toque»**, y ésa es la decisión que hace posible cumplir a la vez FR-003 (no
se puede saber cuándo) y FR-012 (el volumen no crece con los toques). La 017 ya pagó la lección
contraria: un renglón por evento con su instante se cruza con `register_sessions.closed_by` y
etiqueta al empleado aunque su nombre no esté.

**`nulls not distinct`** por lo mismo que en la 017: `role` es nulo en todo lo suprimido por
k-anonimato, y sin esa cláusula cada uno crearía una fila nueva en vez de sumar.

**Sin columna de tiempo más fina que el día.** Ni `created_at`, ni `updated_at`: un `updated_at` en
una fila que se toca con cada lote diría a qué hora estuvo activa esa zona, que es la mitad del
camino de vuelta a identificar a alguien.

## El techo (FR-013)

| | Cuenta | |
|---|---|---|
| Celdas por pantalla | 84 | 12 × 7 |
| **Orientaciones** | **2** | horizontal y vertical son filas distintas, y **una empresa puede usar las dos**: la tableta montada en el mostrador y otra en mano para tomar pedidos en piso. Se me había olvidado multiplicar por esto |
| Cortes de rol | 5 | cuatro roles + «sin corte» |
| Pantallas instrumentadas | 1 | hoy solo el POS (FR-015) |
| **Filas por día, tope absoluto** | **840** | y eso es si se toca *cada* celda, en *las dos* formas, con *cada* rol |
| Retención | 92 días | |
| **Filas, tope absoluto** | **77,280** | |

**Medido** (Postgres 16, con el patrón de escritura real: los días pasados fríos y el día en curso
recibiendo 20,000 incrementos de a uno): **16.5 MB en 77,364 filas**, de los cuales **7.5 MB son
índices** — 223 bytes por fila. La estimación de este documento decía «≈ 10 MB con su índice» y se
quedó **40 % corta**; el número bueno es el medido.

**Techo declarado: 20 MB por empresa y POR PANTALLA INSTRUMENTADA**, que deja margen para el churn
de los `update` entre pasadas de autovacuum. Es un tope **estructural** y no una estimación: no
depende de cuántos toques lleguen, porque los toques suben contadores. Con una sola pantalla
instrumentada ya se ocupa el **82 %** de ese techo.

Con cuatro pantallas instrumentadas el tope sube a cuatro veces eso —66 MB medidos, más que la base
completa del negocio— y ahí ya habría que decidir si se acorta la retención o se instrumenta menos.
**No se agregan pantallas sin rehacer esta cuenta**, y el mensaje de fallo de
`toques_volumen_test.go` lo nombra como una de las tres causas posibles.

Para comparar: la base completa de producción pesa hoy **18 MB**.

## Permisos y RLS

Idénticos a los de `usage_daily` (017), y por las mismas razones:

```sql
grant select, insert, update on usage_touches_daily to gatobobah_app;  -- select por el upsert
grant select on usage_touches_daily to gatobobah_platform;

create policy tenant_isolation on usage_touches_daily using (…) with check (…);
create policy plataforma_lee_todos_los_toques on usage_touches_daily
  for select to gatobobah_platform using (true);
```

La política de plataforma **no es opcional**: `tenant_isolation` también le aplica a ese rol y su
conexión no fija `app.company_id`, así que sin ella la consola vería cero filas y la rejilla saldría
vacía **sin que nada fallara** — que es la peor forma de fallar. Medido en la 017.

## El recorte

Lo hace la misma pasada diaria que ya corre (`UsageService.Recortar`), con su conexión de dueño
abierta y cerrada para eso. **Un `delete` más dentro de esa pasada, con su propia constante**
(`RetencionDeToquesEnDias = 92`), no una reutilización de la de `usage_daily` (396): son dos
retenciones distintas a propósito y confundirlas se vería como que los toques duran un año.

Dos retenciones en la misma familia tiene una razón: el agregado de la 017 se mira para comparar un
mes contra el mismo mes del año pasado; un mapa de toques se mira para decidir un cambio de
disposición **ahora**, y contra el trimestre anterior.

## Lo que este modelo no resuelve, y hay que saberlo

- **La celda no dice qué control era.** En una pantalla que se desplaza, la misma zona es contenido
  distinto en momentos distintos (FR-004). Lo que responde es qué parte del vidrio usa la mano.
- **Cambiar la resolución de la rejilla tiene una dirección barata y otra imposible.** Ir a una más
  GRUESA se recalcula fusionando celdas —con letra chica: 12 columnas se fusionan exacto a 6, 4, 3 o
  2, pero **7 filas es primo**, así que en el eje vertical la única fusión limpia es la de una sola
  franja—. Ir a una más **FINA no se puede**: no existe el toque fino
  del cual derivarla, que es justamente la decisión que hace segura esta feature. Cambiarla hacia
  abajo obliga a declarar el corte y perder la comparación con lo anterior. Por eso la resolución
  vive en el código: para que ese cambio se vea en un diff y no en una tabla de configuración.
- **Una empresa con tabletas de proporciones distintas** mezcla dos formas bajo la misma
  orientación. Hoy todas son iguales; el día que no, la proporción fina es una columna nueva.
