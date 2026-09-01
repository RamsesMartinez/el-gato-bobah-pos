# Fase 1 — Contratos

Ningún endpoint nuevo. Uno gana un campo, dos cambian de comportamiento.

## `GET /business-settings` y `PUT /business-settings` — un campo más

Ganan `corteDeVista`: `"medianoche"` (default), `"turno"` o `"cierre_de_caja"`. Un valor fuera de
esos tres se rechaza con `400`; el default es para el campo **ausente**, nunca para el presente y
malformado.

`timezone` ya viajaba en los dos sentidos y no cambia.

## `GET /orders/open` — se le quita el filtro de fecha

**Antes**: los pedidos cuya fecha de negocio es la del turno abierto. Fue el arreglo parcial de la
feature 005.

**Ahora**: **todos** los pedidos no terminados, sin importar de qué día sean, más los que deben
dinero del día en curso.

Cada fila gana `desdeCuando`: la fecha de negocio del pedido, para que la pantalla distinga el rezago
de hoy sin volver a implementar la regla.

| Caso | Qué devuelve |
| --- | --- |
| Pedido abierto de hace dos meses | Sale, marcado con su fecha |
| Pedido entregado y pagado | No sale |
| Pedido entregado sin pagar | Sale: es dinero en riesgo |

**Por qué sin límite de antigüedad**: es el mecanismo con el que se limpia el rezago. Un pedido que
nadie ve es un pedido que nadie cierra, y hoy hay once así. Se muestran para forzar la decisión, y
al cerrarlos salen solos.

## `GET /orders/delivered` — el corte lo decide el ajuste

**Antes**: los entregados cuya fecha de negocio es la del **servidor**, en UTC. Se vaciaba a las
18:00 locales.

**Ahora**: los entregados desde el último corte, según el modo configurado:

| Modo | Desde cuándo |
| --- | --- |
| `medianoche` | La medianoche de hoy en la zona del negocio |
| `turno` | La apertura del turno vigente |
| `cierre_de_caja` | El último cierre de caja |

**La medianoche se calcula en la zona, no restando 24 horas.** `America/Tijuana` está en la lista de
zonas que el producto ofrece y sí cambia de horario: dos veces al año la distancia entre dos
medianoches es de 23 o 25 horas.

## Nada más cambia en el servidor

La zona horaria no se aplica en la respuesta: los instantes siguen viajando en UTC, como hoy. Quien
los convierte es la pantalla, que es la que sabe para quién los está pintando.

**Por qué no convertirlos en el servidor**: una fecha ya formateada no se puede reordenar, ni sumar,
ni comparar en el cliente sin volver a parsearla, y el mismo dato lo consumen la pantalla, el papel y
—algún día— una exportación. El instante es el dato; el formato es la vista.
