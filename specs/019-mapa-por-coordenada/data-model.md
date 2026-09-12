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
```

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
| Cortes de rol | 5 | cuatro roles + «sin corte» |
| Pantallas instrumentadas | 1 | hoy solo el POS (FR-015) |
| **Filas por día, tope absoluto** | **420** | y eso es si se toca *cada* celda con *cada* rol |
| Retención | 92 días | |
| **Filas, tope absoluto** | **38,640** | ≈ 5 MB con su índice |

**Techo declarado: menos de 10 MB por empresa.** Y es un tope **estructural**, no una estimación: no
depende de cuántos toques lleguen, porque los toques suben contadores. Con cuatro pantallas
instrumentadas seguiría por debajo de 20 MB.

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
abierta y cerrada para eso. Una tabla más en el mismo `delete`, no un trabajo nuevo.

## Lo que este modelo no resuelve, y hay que saberlo

- **La celda no dice qué control era.** En una pantalla que se desplaza, la misma zona es contenido
  distinto en momentos distintos (FR-004). Lo que responde es qué parte del vidrio usa la mano.
- **Cambiar la resolución de la rejilla** deja los datos viejos con la suya. Si algún día se cambia,
  o se recalcula o se declara el corte — y por eso la resolución vive en el código y no en la base:
  para que el cambio sea visible en un diff.
- **Una empresa con tabletas de proporciones distintas** mezcla dos formas bajo la misma
  orientación. Hoy todas son iguales; el día que no, la proporción fina es una columna nueva.
