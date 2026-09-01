# Fase 0 — Investigación

Todo lo de aquí está medido contra el código de `develop` o contra la base de producción. Lo que no
se pudo medir se dice.

## Hallazgo 1 — La barrera de "no cobrar sin confirmar" es UNA condición, no un rediseño

`POST /orders` acepta `Payments []PaymentInput`, y el comentario del campo lo dice:
*"Vacío = enviar a cocina sin cobrar"* ([app/orders.go:48](../../server/internal/app/orders.go)).
Con pagos, la misma llamada crea **y cobra**. Y ya existe `POST /orders/{id}/pay`
(`ChargeOrder`), que cobra un pedido que ya existe.

**Consecuencia**: cobrar pasa a ser exclusivamente `POST /orders/{id}/pay`, y `POST /orders` rechaza
`payments` no vacío. No hace falta endpoint nuevo ni cambiar el modelo — hace falta cerrar un camino
que quedó abierto.

## Hallazgo 2 — "Confirmado" NO necesita un estado nuevo; el RENGLÓN sí necesita una marca

Los estados son `abierta → lista → entregada | cancelada | reembolsada`
([0007_orders.sql:2](../../server/migrations/0007_orders.sql), `0018`). Hoy "mandado a cocina" y "el
pedido existe" son lo mismo, y al cerrar el camino del hallazgo 1, **todo pedido nace confirmado**.
Un estado nuevo no respondería ninguna pregunta que "el pedido existe" no responda ya.

Pero la comanda de solo-lo-agregado sí necesita una pregunta que hoy nadie puede responder:
**¿este renglón ya salió en una comanda?** `order_lines` tiene `created_at`, `cancelled_at`, y nada
sobre impresión ([0007_orders.sql:39](../../server/migrations/0007_orders.sql)).

Sin esa marca, "lo agregado" solo se puede calcular en el instante del agregado, y una impresión
que falla se pierde sin dejar rastro. Con ella, se puede sacar exactamente lo que no ha salido.

**Decisión**: columna en `order_lines`, no estado en `orders`. Es lo mínimo que responde la pregunta.

## Hallazgo 3 — El pedido desaparece porque `closeTab` corre siempre

`useMandarPedido.onSuccess` llama a `closeTab(cuenta.id)` sin condición
([useMandarPedido.ts](../../web/src/features/pos/useMandarPedido.ts)), y `CheckoutSheet` hace lo
mismo. Por eso mandar a cocina borra la cuenta de la pantalla, que es lo que el dueño reportó tras
probarlo en dev.

## Hallazgo 4 — La barra de en curso NO cuesta alto: la fila ya existe

El POS ya tiene una fila única con cuentas, buscador y botones, y su comentario explica por qué:
*"Una sola fila (cuentas · buscador · toggles): recupera ~56px de alto en 7" landscape. Las cuentas
scrollean solas"* ([POSPage.tsx:297](../../web/src/features/pos/POSPage.tsx)). Dentro va
`TicketTabs`, con chips de 44 px y desplazamiento horizontal, y al lado la `PorCobrarPill`.

**Consecuencia**: los pedidos en curso entran como chips en esa misma fila, junto a las cuentas
locales sin mandar. El presupuesto de alto no cambia, que es lo que exige SC-005. Y fundir la
píldora ahí (D1) **libera** ancho en vez de consumirlo.

## Hallazgo 5 — "Por cobrar" y "en curso" NO son el mismo conjunto

`ListUnpaidOrders` ([queries/orders.sql:230](../../server/queries/orders.sql)) devuelve los del día
con `paid < total` y estado distinto de cancelada/reembolsada. Eso incluye **pedidos entregados sin
cobrar** —el caso que la píldora existe para gritar, "el cliente ya se fue"— y excluye los
**pedidos ya pagados que aún no se entregan**, a los que sí se les puede agregar.

Fundir las dos cosas sin pensarlo perdería el caso caro. La barra muestra la **unión**: no
terminales (se les puede agregar) ∪ con saldo pendiente (hay dinero en riesgo), y distingue
visualmente cuál es cuál.

## Hallazgo 6 — Agregar renglones ya existe, con su gate

`POST /orders/{id}/lines` está en el router con el mismo gate que crear, y su comentario ya
contempla el caso: *"la libreta vuelve de la mesa con 'pidieron dos más'"*
([router.go:109](../../server/internal/httpapi/router.go)). Lo que falta no es la operación: es que
imprima solo lo agregado y que se llegue a ella en un toque.

## Hallazgo 7 — La comanda se dispara desde un componente sin UI, y ya existe el patrón del fallo

`<KitchenTicket order={lastOrder} />` sale del pedido recién mandado
([POSPage.tsx:518](../../web/src/features/pos/POSPage.tsx)), y el documento está resuelto en
[printKitchen.ts](../../web/src/utils/printKitchen.ts): sin precios, folio grande, notas y
adicionales gratis.

El comportamiento ante impresora caída ya se resolvió para el ticket del cliente en la feature 001
(commit `db01e03`): un aviso que informa y no bloquea, con la venta ya registrada. **Se copia ese
criterio**, no se inventa otro.

## Hallazgo 8 — Producción: el camino viejo existe y nadie lo usa

Medido contra la base de producción el 2026-09-01, empresa `gatobobah`:

| Qué | Cuánto |
| --- | --- |
| Pedidos con un renglón agregado más de 2 minutos después de abrirse | **0** |
| Pedidos por día (29–31 ago) | 4, 10, 6 |
| Renglones por pedido | 2.2 promedio, 6 máximo |
| Productos activos | 174 |

El selector "Agregar a un pedido en curso" vive dentro de la hoja de cobro
([CheckoutSheet.tsx:414](../../web/src/features/pos/CheckoutSheet.tsx)) y cuesta cinco toques.

## Hallazgo 9 — Dos empresas en la misma base

`bobah-pruebas` (id 1) y `gatobobah` (id 2), cada una con su catálogo, sus usuarios y sus ajustes.
Cualquier consulta nueva se mide por empresa y cualquier test de migración corre **con las dos**: con
una sola, un defecto de alcance es un no-op que pasa verde.

## Hallazgo 10 — El ajuste de comanda hoy nace apagado

`print_kitchen_ticket` se agregó en [0043](../../server/migrations/0043_comanda_cocina.sql) con
default `false`. Cambiar el default afecta **solo a empresas nuevas**: las que ya tienen fila
conservan su valor, que es lo que pide FR-019. Para El Gato Bobah hay que encenderlo aparte.

## Lo que NO se investigó, y por qué

- **Cómo se comporta la impresión con dos tabletas mandando a la vez.** La impresión la dispara el
  navegador de cada estación contra su propia impresora; no hay cola compartida. Queda fuera de esta
  feature: si el negocio pone una sola impresora de cocina en red, es otro problema y otro spec.
- **Tiempo real entre estaciones.** El spec asume refresco periódico (30 s, el que ya usa la
  píldora). No se midió si hace falta menos.
