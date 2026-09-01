# Fase 1 — Contratos

Ningún endpoint nuevo. Uno se cierra, dos cambian de comportamiento y uno cambia de forma.

## `POST /orders` — deja de poder cobrar

**Antes**: `payments` vacío = mandar a cocina; `payments` con líneas = crear **y cobrar** de un
golpe. Ese segundo camino es el que se salta la comanda.

**Ahora**: `payments` deja de aceptarse. Un cuerpo que lo traiga se rechaza con `422` y un mensaje
que dice qué hacer: el pedido se crea primero y se cobra con `POST /orders/{id}/pay`.

| Caso | Respuesta |
| --- | --- |
| Cuerpo válido sin `payments` | `201` con el pedido |
| Cuerpo con `payments` no vacío | `422 UNPROCESSABLE` — cobrar es de `/pay` |
| Cuerpo con `lines` vacío | `400 VALIDATION` — un pedido sin renglones ocuparía folio y sacaría una comanda en blanco |
| Mismo `clientUuid` que un pedido que ya existe | `201` con **ese mismo** pedido, sin crear otro |

**El rechazo es un `422` y no un `400`** porque el cuerpo está bien formado; lo que no se puede es la
operación. Y el mensaje nombra el camino correcto: un error que solo dice "no" manda al operador a
adivinar.

## `POST /orders/{id}/lines` — imprime lo agregado

**Antes**: agrega renglones y devuelve el pedido. Nada sale a cocina.

**Ahora**: además marca esos renglones como enviados a cocina y la respuesta trae **cuáles son**,
para que la estación imprima la comanda del agregado sin volver a preguntar.

| Caso | Respuesta |
| --- | --- |
| Pedido en `abierta` o `lista` | `200` con el pedido y los renglones agregados identificados |
| Pedido `entregada`, `cancelada` o `reembolsada` | `409 CONFLICT`, con el estado en el mensaje |
| `lines` vacío | `400 VALIDATION`, sin tocar el pedido ni sacar papel |
| Pedido ya cobrado por completo | `200`; el saldo pendiente resultante viaja en la respuesta |

**El `409` lleva el estado adentro** porque quien lo recibe es una tableta que estuvo suspendida
media hora: necesita saber que el pedido se entregó, no solo que "no se pudo".

## `GET /orders/unpaid` → `GET /orders/en-curso`

Cambia el conjunto, así que cambia el nombre: seguir llamándole "unpaid" a una lista que incluye
pedidos pagados es un nombre que miente.

Devuelve la **unión** de dos grupos, cada fila diciendo a cuál pertenece:

- **En preparación** — `status in ('abierta','lista')`. Se le puede agregar y cobrar.
- **Con saldo** — debe dinero y no está cancelada ni reembolsada. Incluye el entregado sin cobrar,
  que es el caso caro: el cliente ya se fue.

Cada fila trae lo que la barra pinta: folio, número, monto, saldo pendiente, estado, cuántos
renglones y a qué grupo pertenece. La respuesta trae además el **total pendiente**, que es lo que
hoy se lee de un vistazo en la píldora y no se puede perder.

Sin gate de rol, igual que hoy: quien está en la caja es quien tiene que poder saldarlo.

## `POST /orders/{id}/pay` — sin cambios de forma

Ya existe y ya cobra un pedido que existe. Pasa a ser **el único** camino de cobro. Solo hay que
comprobar que sigue funcionando desde las dos pantallas que lo usan y que no quedó pidiendo una
confirmación que ahí no aplica.

## `PUT /business-settings` — sin cambios

El ajuste `printKitchenTicket` ya viaja. Lo que cambia es el default de la columna, y eso no se ve
desde la API.

## Idempotencia: el `clientUuid` tiene que sobrevivir al reintento

`orders.client_uuid` es único y el servicio ya devuelve el pedido existente cuando se repite. Pero
hoy el front **genera un uuid nuevo dentro de cada intento**
([useMandarPedido.ts](../../web/src/features/pos/useMandarPedido.ts) llama a `uuid()` dentro de
`mutationFn`), así que un reintento tras un corte de red manda un identificador distinto y el
servidor crea un segundo pedido con lo mismo.

**El uuid pasa a vivir en la cuenta**, no en el intento: se genera al abrir la cuenta y se usa tal
cual en cada reintento. Es la mitad del contrato de idempotencia que faltaba, y sin ella el `201`
con el pedido existente nunca se dispara.
