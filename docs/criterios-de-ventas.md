# Criterios de ventas — qué cuenta, cuánto y en qué día

Decisiones del dueño para toda cifra de ventas que se use para facturar o cuadrar el mes. Las
pantallas y consultas que hoy no las cumplen se corrigen en `specs/029-ventas-netas/`.

## Decidido (2026-10-08)

| Pregunta | Decisión |
|---|---|
| ¿En qué mes pega una devolución de dinero? | **En el mes en que se devolvió**, no en el de la venta. Es como sale de la caja y como se factura: lo del mes cerrado no se reescribe. |
| ¿Qué es el «Total» de Ventas? | **Solo lo cobrado, neto de devoluciones.** Un pedido abierto o con saldo pendiente no suma hasta que se cobra. |

## Por qué

- Medido el 2026-10-08 en el ambiente de pruebas: dos devoluciones (parcial $30.67 y total $92)
  dejaron el pedido en `entregada`, y el Total de Ventas siguió sumando los $92 completos de cada
  uno; también sumaba $360 de dos pedidos abiertos sin cobrar. El número que se usaba para
  facturar estaba inflado por los dos lados.
- El estado `reembolsada` ya no lo escribe ningún camino desde la migración 0060: una devolución
  vive en `order_refunds`. Filtrar por estado no encuentra devoluciones nuevas.

## Pendiente de decidir

- **¿En qué día cuentan los productos vendidos?** Propuesta: el dinero, en la hora de cada pago;
  los productos, cuando el pedido queda pagado completo (el último pago); un producto devuelto se
  resta en el día de la devolución. Un pedido pagado en dos partes a ambos lados de la medianoche
  reparte su dinero en dos días y sus productos en el segundo.
