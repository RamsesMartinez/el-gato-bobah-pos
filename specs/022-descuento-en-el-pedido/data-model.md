# Data Model: El descuento que se le hace a un pedido

## Columnas de `orders`

| Columna | Tipo | Estado | Qué guarda |
|---|---|---|---|
| `discount_total` | `numeric(10,2) not null default 0` | **ya existe** (0007) | Los pesos que el negocio dejó de cobrar en este pedido |
| `discount_set_by` | `bigint references users(id)` | **nueva** | Quién lo aplicó. Nula cuando no hay descuento |
| `discount_set_at` | `timestamptz` | **nueva** | Cuándo. Nula cuando no hay descuento |

`discount_set_by` va **sin `on delete`** (= `no action`), como sus cuatro hermanas de esta misma
tabla (`opened_by`, `cancelled_by`, `refunded_by`, `platform_ref_set_by`): los usuarios se
desactivan, nunca se borran, y ninguna query los borra.

## Los checks

```sql
check (discount_total >= 0)
check (total >= 0)
-- El rastro y el dinero van juntos: un descuento positivo SIEMPRE tiene autor.
check ((discount_total > 0) = (discount_set_by is not null))
check ((discount_set_by is null) = (discount_set_at is null))
```

**Por qué el check del rastro no es opcional**: el único control contra el abuso que esta feature
deja es saber quién descontó (D5 de [research.md](./research.md) apoya en eso la decisión de no
pedir rol ni PIN). Sin el check, ese rastro es una promesa de la aplicación: basta que
`SetOrderDiscount` olvide limpiar las dos columnas al quitar un descuento —o que una ruta nueva
escriba el monto sin el actor— para que quede dinero descontado sin nadie que responda por él, y
nada falle.

**Está anclado en `> 0` y no en `is not null`** porque `discount_total` es `not null default 0`: un
pedido sin descuento tiene cero, y cero no tiene autor.

**El check que NO va, y por qué**: `discount_total <= subtotal` parece obvio y rompería la
cancelación de un renglón. Cancelar baja el subtotal y **no** baja el descuento (D4), así que un
pedido de $385 con $50 de descuento al que se le cancela casi todo queda legítimamente con subtotal
$20 y descuento $50 — total $0. El check lo volvería un error de Postgres en una operación normal.

## Cómo se aplica la migración

`0072_descuento_del_pedido.sql`, con las dos precauciones que esta tabla ya exige
([0065](../../server/migrations/0065_folio_y_liquidacion_de_plataforma.sql) las documentó para
`orders`):

```sql
set local lock_timeout = '3s';
alter table orders
  add column discount_set_by bigint references users(id),
  add column discount_set_at timestamptz,
  add constraint orders_descuento_no_negativo check (discount_total >= 0) not valid,
  add constraint orders_total_no_negativo    check (total >= 0) not valid,
  add constraint orders_descuento_con_rastro
    check ((discount_total > 0) = (discount_set_by is not null)) not valid,
  add constraint orders_rastro_completo
    check ((discount_set_by is null) = (discount_set_at is null)) not valid;

alter table orders validate constraint orders_descuento_no_negativo;
-- … y las otras tres, cada una en su sentencia
```

`not valid` + `validate` aparte no es ceremonia: un `check` validado en el mismo `alter` escanea la
tabla entera bajo `ACCESS EXCLUSIVE`, y si al desplegar hay una transacción larga abierta, toda
lectura nueva se encola detrás. El `validate` toma un lock mucho más suave. El `lock_timeout` es la
red: prefiere fallar el deploy a dejar el mostrador esperando.

**Número de migración**: el 0072 de esta rama y el `0072_pedidos_de_plataforma.sql` de la rama
`021-recibir-pedidos-uber` son dos archivos distintos con el mismo número. Esta feature llega
primero a `develop` y la base de pruebas está en la versión 71, así que 0072 es suyo; **la 021 tiene
que renumerar la suya a 0073 al rebasar**. Dos migraciones con el mismo número hacen que goose dé
por aplicada la que no es, y la API arrancaría sin las columnas que sus queries nombran.

## Aritmética, en un solo lugar de cada lado

```text
subtotal = Σ line_total de los renglones VIVOS (no cancelados)
descuento = monto capturado, ya resuelto a pesos
total    = max(subtotal − descuento, 0) + delivery_fee
```

Go: `domain.AplicarDescuento` entre `BuildOrder` y `ApplyDeliveryFee`.
SQL: `RecalcOrderTotals`, para cuando el pedido cambia después.
Web: `web/src/domain/descuento.ts`, que es lo único que la pantalla usa para pintar el total.

## Las queries que cambian

| Query | Cambio | Por qué |
|---|---|---|
| `CreateOrder` | Suma `discount_total`, `discount_set_by`, `discount_set_at` a la lista de columnas | Hoy **no las nombra** y depende del `default 0`: sin esto, la Historia 1 guardaría el descuento en ningún lado |
| `RecalcOrderTotals` | `total = greatest(subtotal − o.discount_total, 0) + o.delivery_fee` | Es el único punto por el que pasan agregar renglones y cancelarlos |
| `SetOrderDiscount` (nueva) | `for update` sobre el pedido antes de leer subtotal y escribir | Mismo motivo que `GetOrderForUpdate`: sin el lock, el tope "descuento ≤ subtotal" se valida contra un subtotal que otra estación ya movió, o se pisa un total que un cobro paralelo acaba de saldar |

## Reglas de validación

| Entrada | Resultado |
|---|---|
| Nada | Sin descuento (`0`), sin autor |
| `discountAmount` y `discountPercent` juntos | `domain.ErrValidation` → 400 |
| `discountAmount` < 0, NaN, ±Inf, fuera de `MaxMoney` | `domain.ErrValidation` → 400 |
| `discountPercent` fuera de `[0, 100]`, NaN, no numérico | `domain.ErrValidation` → 400 |
| Monto resuelto > subtotal | `domain.ErrDescuentoMayorQueLaVenta` → 422 |
| Monto resuelto con más de 2 decimales | `Round2` antes de validar y de persistir |

## Estados en los que se puede cambiar el descuento

| Estado del pedido | ¿Admite cambio? |
|---|---|
| Abierta / lista, sin cobrar o cobrada en parte | Sí |
| Cobrada por completo | No — `domain.ErrConflict` (movería el total contra pagos ya registrados) |
| Cancelada / reembolsada | No — son terminales |

## Lo que NO se modela

Quién financió el descuento (negocio o plataforma), el motivo, la campaña, el descuento por renglón
y cualquier vigencia. Ver *Assumptions* del spec: es una exención explícita del dueño, no un olvido.

## Lo que se verificó y NO hace falta

- **Ningún grant ni política nueva.** `orders` ya tiene RLS y el grant de
  [0024](../../server/migrations/0024_rls.sql) es a nivel de **tabla**: agregarle columnas no pide
  grant nuevo. Lo que sí queda es el test bajo `appRoleStore`, porque eso no se puede suponer.
- **`total >= 0` no rompe nada vivo.** Ninguna ruta de reembolso toca `total`: `RefundOrder` y
  `RecalcOrderRefundAmount` escriben `refund_amount`, que es otra columna.
