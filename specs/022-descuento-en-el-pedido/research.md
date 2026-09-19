# Research: El descuento que se le hace a un pedido

Phase 0. Lo que se resolvió antes de diseñar, con lo que se descartó y por qué.

## D1 — Dónde vive el descuento: ¿columna del pedido o tabla aparte?

**Decisión**: en `orders`, reusando `discount_total`, que existe desde
[0007](../../server/migrations/0007_orders.sql) y siempre ha valido cero.

**Rationale**: el descuento es un atributo del pedido, uno por pedido. Una tabla aparte solo
serviría para guardar varios descuentos por pedido o un catálogo de promociones, y el spec excluye
las dos cosas (principio VI). Además, reusar la columna hace que todos los agregados que ya suman
`orders.total` sigan siendo correctos sin tocarlos.

**Alternativas consideradas**:
- *Tabla `order_discounts`*: permitiría varios descuentos y su motivo. Descartada: abstracción
  especulativa para un caso que nadie pidió.
- *Un renglón negativo en `order_lines`*: descartada y vale decir por qué — rompería `sum(line_total)`
  como definición de subtotal, contaminaría el costeo (un renglón sin producto), y convertiría el
  descuento en algo que la cocina podría imprimir.

## D2 — ¿Se guarda el porcentaje?

**Decisión**: no. Se guarda **solo el monto en pesos**; el porcentaje es una forma de teclear.

**Rationale**: lo que el negocio dejó de cobrar es una cantidad de dinero, ya ocurrida. Guardar la
fórmula la deja viva: un pedido al que se le agrega un café vería su descuento crecer solo, sin que
nadie lo decidiera, y el ticket que el cliente ya tiene en la mano dejaría de cuadrar.

**Alternativas consideradas**: guardar los dos (monto + porcentaje) para poder mostrar "20 %" en el
ticket. Descartada: dos cifras para el mismo hecho es el corolario del principio III que ya costó un
turno con $4,500 de faltante inexplicable; y la que manda —el monto— no necesita a la otra.

## D3 — ¿El descuento come el envío?

**Decisión**: no. `total = subtotal − descuento + envío`.

**Rationale**: el envío no es venta de comida; es un cobro que el negocio traslada. Descontar sobre
él cambiaría lo que se le paga al repartidor por una promoción que aplica al menú. Y el caso que
apura —plataformas— ni siquiera cobra envío propio (`cobraEnvio` es falso con plataforma).

## D4 — ¿Qué pasa cuando el subtotal cambia después del descuento?

**Decisión**: el descuento se queda como está; el total tiene **piso en cero**
(`greatest(subtotal − descuento, 0) + envío`) y la resta vive dentro de `RecalcOrderTotals`.

**Rationale**: es donde ya se recalcula el pedido al agregar renglones y al devolver
([devolucion.go](../../server/internal/app/devolucion.go)). Ponerlo en otro lado dejaría un camino
que recalcula sin descontar — el defecto silencioso más probable de toda esta feature.

**Alternativas consideradas**: recortar el descuento al nuevo subtotal. Descartada: cambiar en
silencio una cifra de dinero que una persona capturó es justo lo que el principio V prohíbe. El piso
en cero no cambia el descuento registrado; solo impide un total negativo.

## D5 — ¿Autorización?

**Decisión**: ninguna barrera nueva; cualquiera que pueda capturar un pedido puede descontar, y
queda el rastro de quién y cuándo.

**Rationale**: decisión del dueño (2026-09-19). Un pedido de plataforma con promoción llega a
cualquier hora, y exigir a alguien con rol bloquearía la captura. El rastro permite auditar después,
y poner un tope más adelante no rompe nada de este diseño.

## D6 — ¿Cómo se teclea en una tableta de 600 px de alto?

**Decisión**: el acceso al descuento vive **dentro de la fila del `Total`** que ya existe (un botón
chico a su derecha), y despliega el campo con el selector `$` / `%` de dos botones tipo segmento.

**Rationale**: el 95 % de los pedidos no lleva descuento y el panel del ticket ya está al límite de
alto. Una fila propia de 44 px + margen baja los renglones de producto visibles de ≈3.5 a ≈2.9 en un
pedido a domicilio — medido por la revisión de arquitectura sobre `Ticket.tsx`. Metido en la fila
del Total, el costo es de 4 px: los que la fila crece para cumplir el mínimo tappable de 44.

**Alternativas consideradas**:
- *Un botón `+ Descuento` en su propia fila*: era el diseño original de este plan. Descartado por lo
  de arriba: le cobraba alto a todos los pedidos para servir a unos pocos.
- *Meter el descuento en la píldora flotante o en la barra angosta*, que son la vista por default a
  1024×600. Descartado: **ahí no hay ancho** — la barra ya se desborda antes de agregarle nada. El
  precio de esa decisión es un toque extra (abrir el panel) para quien descuenta, y está declarado
  en el plan y en SC-001 en vez de escondido.
- *Un `Picker` para elegir entre `$` y `%`*: descartado — el `Picker` existe para reemplazar al
  `<select>` nativo en listas; para dos opciones excluyentes una hoja inferior son dos toques donde
  bastaba uno.

## D7 — ¿Qué pasa con el teclado numérico abierto?

**Decisión**: `scrollIntoView` al enfocar el campo y `maxH` en **dvh** para la zona de totales.

**Rationale**: el teclado de Android en landscape ocupa ~40–45 % del alto (240–270 px de 600). Sin
esto, el botón COBRAR puede quedar fuera de la pantalla y el único rescate sería el scroll de la
página entera, anidado con el de la lista de renglones. `overflowY` sin un alto no hace scroll: la
caja crece, y el precio lo paga la tableta.

**Lo que no se puede probar**: ningún test de este repo simula el teclado de Android. Un test de
jsdom verifica que el contenedor tiene alto acotado; que COBRAR siga alcanzable con el teclado
abierto se verifica **a mano en la tableta**, y así está dicho en el plan.
