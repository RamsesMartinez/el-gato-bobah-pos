# Plan — Devolver el dinero de una venta que no fue

**Spec**: [spec.md](spec.md) · **Rama**: `007-devolver-el-dinero` · **Creado**: 2026-09-03

## Contexto técnico

| Qué | Decisión |
| --- | --- |
| Lenguaje / stack | Go 1.27, pgx + sqlc, goose embebido · React 19 + Chakra v3 |
| Migración nueva | `0060_devoluciones.sql` |
| Tablas nuevas | `order_refunds` (el libro de devoluciones) |
| Columnas nuevas | `stock_movements.order_line_id` |
| Reusa | `register_cash_movements` (kind `salida`), `order_lines.cancelled_at`, `order_lines.enviado_a_cocina_at`, `RecalcOrderTotals` |

## Lo que YA existe y no se rehace

Medido antes de planear, no supuesto:

- `order_lines` tiene `cancelled_at`, `cancelled_by`, `cancel_reason`. Lo que falta es la
  **operación** que las escriba, no el esquema.
- `RecalcOrderTotals` ya excluye los renglones cancelados del total del pedido.
- `order_lines.enviado_a_cocina_at` ya responde la pregunta de D4 (`NULL` = no salió a cocina).
- `register_cash_movements` ya tiene `kind in ('entrada','salida')` y el corte ya lo lee y lo resta.
- `orders` ya tiene `refunded_at`, `refunded_by`, `refund_reason`, `refund_amount` (migración 0018).
- `order_payments` guarda `payment_method_id`, así que se sabe **por qué método entró cada peso**.

## Constitution Check

| Principio | Cómo aterriza |
| --- | --- |
| **I — Layering** | Las reglas puras (cuánto se puede devolver, cómo se reparte por método, si repone) van a `domain`, sin I/O. La orquestación y la transacción, a `app`. El SQL, a `queries/` vía sqlc. Los handlers solo decodifican y mapean el error. |
| **II — Errores** | Sentinels nuevos en `domain` (`ErrDevolucionExcede`, `ErrSinCobrosQueDevolver`, `ErrRenglonYaEntregado`), envueltos con `%w`; el mapeo a HTTP solo en `httpapi.Error`. |
| **III — Dinero** | `Round2` en cada frontera. **Cada peso devuelto se clasifica una sola vez**: sale del método por el que entró, y el libro de devoluciones es la única fuente — `orders.refund_amount` pasa a ser su suma recalculada, no un número que se escribe aparte. |
| **IV — Test-first** | Las reglas puras se prueban sin base. Lo que solo se ve en Postgres —el arqueo, el reparto por método sobre datos reales, la reposición— va a integración, con dos empresas. |
| **V — Seguridad** | `POST /orders/{id}/cancel` gana `RequireRole`: hoy no lo tiene y mueve el mismo dinero que el reembolso, que sí lo exige. Monto y motivo validados en la frontera. |
| **VI — YAGNI** | Sin tabla de "motivos de devolución" ni flujo de aprobación: nadie los pidió. El libro de devoluciones sí, porque D3 lo exige. |

## Diseño

### 1. El libro de devoluciones

`orders.refund_amount` es un escalar y D3 pide devolver renglones sueltos, varias veces, por métodos
distintos. Un escalar no puede responder "¿qué se devolvió y por qué método".

```
order_refunds
  id, company_id, order_id, order_line_id (null = la cuenta entera),
  payment_method_id, amount, reason, refunded_by, created_at,
  cash_movement_id (null = no salió del cajón)
```

`orders.refund_amount` **se conserva** y pasa a recalcularse como la suma de este libro, igual que
`RecalcOrderTotals` hace con el total. Así los reportes que ya lo leen (`RefundsByDay`) siguen
funcionando sin tocarlos, y no hay dos verdades sobre el mismo dinero.

### 2. El reparto por método (la regla que más importa)

Es `domain`, puro y con test propio: dado lo que entró por cada método y cuánto se devuelve, decide
de dónde sale cada peso.

- Se devuelve **por método, en el orden en que entró**, hasta agotar el monto.
- Nunca se devuelve por un método más de lo que entró por él: eso inventaría una salida.
- El efectivo genera **salida de caja**; lo demás, solo el renglón del libro (D2).

### 3. La regla de reposición

`domain.ReponeInventario(enviadoACocina)`: `NULL` → repone; con fecha → no. Se decide en el dominio
para que la pantalla no la reimplemente, y se **anuncia antes de confirmar**.

`stock_movements` gana `order_line_id` para que la reposición revierta **los movimientos que de
verdad salieron** y no un recálculo con la receta de hoy. Lo histórico queda en `NULL` y un renglón
viejo no se repone con precisión: se dice, no se adivina.

### 4. Cancelar con devolución, en una transacción

`Cancel` deja de ignorar los cobros. Si el pedido tiene pagos, exige la devolución en el mismo
comando y las dos cosas van en la misma `WithTx`: un pedido cancelado sin su devolución es el
agujero que esto cierra, y dos llamadas separadas lo dejan abierto en el hueco entre una y otra.

## Fases

1. **Dominio** — reglas puras y sus tests. Sin base.
2. **Esquema** — `0060` con su test de integración (la migración se prueba con su test, no después).
3. **Servicio** — devolución, cancelar-con-devolución, cancelar renglón. Integración con dos empresas.
4. **Frontera** — handlers, rol en `cancel`, validación de monto y motivo.
5. **Pantalla** — hoja de devolución con selección por renglón; cancelar renglón desde la tarjeta;
   el aviso de "ya salió a cocina, el insumo no vuelve".
6. **Matriz** — renglones nuevos en `docs/matriz-de-pantallas.md`, con su test.
