# Data Model — 008 · La fecha la da el reloj, el folio lo da el turno

## Tabla nueva: `folio_counters`

El contador que garantiza que dos ventas simultáneas del mismo turno no reciban el mismo número.
Reemplaza en uso —no en esquema— a `order_counters`.

| Columna | Tipo | Notas |
|---|---|---|
| `company_id` | `bigint not null` | Parte de la PK. Se declara normal: la tabla es nueva y sqlc sí la ve. FK a `companies(id) on delete cascade`, como el resto. |
| `register_session_id` | `bigint not null` | Parte de la PK. FK a `register_sessions(id)`. **Sin `on delete cascade`**: un turno no se borra, se cierra; si algún día se borrara, perder el contador dejaría numerando desde cero. `restrict` obliga a decidirlo a propósito. |
| `last_number` | `int not null` | Último folio repartido en ese turno. |

- **PK `(company_id, register_session_id)`** — el candado de fila sobre esta llave es lo que
  serializa la numeración concurrente. Empieza por `company_id` como todo índice de este esquema,
  porque RLS agrega ese predicado a cada consulta.
- **RLS**: política `tenant_isolation` igual a la del resto (`company_id = current_setting('app.company_id')`).
- **Grants**: `select, insert, update` al rol de la aplicación, explícitos. Sin ellos, `42501` en
  el primer request de producción y nada en local.
- **Sin `delete`**: nada borra contadores.
- **Sin secuencia**: no hay columna identity que otorgar.

### Estado inicial

Se siembra desde los pedidos existentes, incluidos los de turnos ya cerrados:

```
por cada (company_id, register_session_id) de orders con register_session_id no nulo:
    last_number = max(daily_number)
```

Sin esta semilla, el turno abierto del ambiente de pruebas repartiría el número 1 sobre 158
pedidos que ya lo usaron.

---

## Tabla nueva: `orders_business_date_fix`

Respaldo de la corrección histórica. Existe para que la migración sea reversible de verdad.

| Columna | Tipo | Notas |
|---|---|---|
| `order_id` | `bigint primary key` | FK a `orders(id) on delete cascade`. |
| `previous_date` | `date not null` | La fecha que la venta tenía antes de la corrección. |

Solo guarda las filas que **cambiaron**. El `Down` las devuelve y borra la tabla.

No lleva `company_id` ni RLS: no la lee la aplicación, solo la migración. Sin grants al rol de la
aplicación, por lo mismo.

---

## Cambios sobre tablas existentes

### `orders`

**Ninguno de esquema.** Cambia el *significado* de una columna que ya existe:

| Columna | Antes | Después |
|---|---|---|
| `business_date` | La fecha del turno de caja que cobró la venta. | El día de calendario en que ocurrió la venta, en la zona del negocio. |
| `daily_number` | Consecutivo dentro de `business_date`. | Consecutivo dentro de `register_session_id`. |
| `folio_name` | Único entre los repartidos en `business_date`. | Único entre los repartidos en `register_session_id`. |
| `register_session_id` | El turno que cobró. **Sin cambios.** | Igual. Es lo que agrupa el arqueo, y sigue siéndolo. |

La corrección histórica reescribe `business_date` en las filas donde difiere del día de
`opened_at`. **No toca** `daily_number`, `folio_name` ni `register_session_id`.

### `order_counters`

Se deja intacta y deja de usarse. Se borrará en una migración posterior, una vez que la feature
lleve un ciclo en producción; queda anotado en la migración que la jubila.

---

## Invariantes que las pruebas deben sostener

1. Dos ventas del mismo turno nunca comparten `daily_number`.
2. Dos ventas **vivas** del mismo turno nunca comparten `folio_name`.
3. `business_date` de una venta es siempre el día de `opened_at` en la zona del negocio.
4. Cerrar un turno exige que no queden pedidos `abierta` ni `lista`. **Es lo que hace segura la
   renumeración desde 1 al reabrir**, así que su prueba pertenece a esta feature aunque la regla
   sea anterior.
5. Ninguna cifra de un arqueo cerrado cambia por la corrección histórica.
6. Un turno sin ventas tiene cero filas en `folio_counters` hasta su primera venta, y eso es
   correcto: el upsert la crea.
