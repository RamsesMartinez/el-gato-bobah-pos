# Fase 1 — Modelo de datos

Una columna nueva y un cambio de default. Cero tablas nuevas y cero estados nuevos.

## Lo que NO cambia, y por qué

| Qué | Por qué se deja igual |
| --- | --- |
| El enum `order_status` | Confirmar **es** crear el pedido: al cerrar el camino de crear-y-cobrar de un golpe, todo pedido nace confirmado. Un estado `confirmada` no respondería ninguna pregunta que "el pedido existe" no responda, y el principio VI lo prohíbe |
| `orders.folio_name` | Ya identifica al pedido en la comanda y es con lo que se canta. La comanda del agregado lo reusa |
| `products.needs_prep` | La confirmación es obligatoria para todos los pedidos (A1), así que esta feature no la consulta. Sigue sirviendo al tablero de cocina |
| `order_payments` | El cobro no cambia de forma; cambia de puerta |
| Los pedidos ya existentes | Nacieron sin pasar por "confirmar" y siguen siendo cobrables y entregables (FR-020). Nada los toca |

## Columna nueva: `order_lines.enviado_a_cocina_at`

```text
order_lines
  …
  enviado_a_cocina_at  timestamptz            -- NULL = todavía no salió en ninguna comanda
```

**Por qué en el renglón y no en el pedido.** La pregunta que hay que poder responder es *"¿qué
renglones de este pedido no han salido a cocina?"*, y un pedido con dos comandas —la del confirmado
y la del agregado— tiene renglones en los dos lados. Una marca por pedido no puede distinguirlos.

**Por qué un timestamp y no un booleano.** Cuando cocina reclame que no le llegó algo, la pregunta
es *cuándo* salió, no *si* salió. Un booleano obliga a cruzar con los logs para responderla, y el
costo de la columna es el mismo.

**Nullable a propósito.** `NULL` es el estado inicial y también el de todos los renglones que ya
existen en producción: nadie sabe si salieron en papel, y decir que sí sería inventar. Un renglón
viejo con `NULL` no reimprime nada solo — la comanda de agregado se dispara al **agregar**, no al
leer.

**El backfill NO marca los renglones viejos.** Marcarlos como enviados sería afirmar algo que no
consta; dejarlos en `NULL` es decir "no se sabe", que es la verdad. Ninguna pantalla los usa para
decidir nada.

### Índice

Ninguno nuevo. La única consulta que la usa es *"los renglones sin enviar de ESTE pedido"*, que ya
entra por `order_lines_order (order_id)`. Un índice sobre una columna que solo se filtra dentro de
un pedido de 6 renglones no compra nada y hay que mantenerlo.

## Cambio de default: `business_settings.print_kitchen_ticket`

```text
print_kitchen_ticket  boolean not null default true   -- era: default false
```

Cambiar el `DEFAULT` de la columna **no toca ninguna fila existente** — es exactamente lo que pide
FR-019. Las empresas que ya tienen fila conservan su valor; las nuevas nacen con la comanda
encendida.

**Ojo con el sembrado.** `SeedBusinessSettings` inserta la fila de una empresa nueva; si nombra la
columna explícitamente con `false`, el default no aplica y el cambio es decorativo. Hay que
verificarlo, no asumirlo.

**El Gato Bobah no se enciende solo.** Su fila ya existe con `false`. Encenderlo es una decisión del
dueño y va como cambio de datos aparte, del lado operativo, no dentro de esta feature.

## Reglas de negocio, y dónde viven

| Regla | Dónde | Por qué ahí |
| --- | --- | --- |
| Un pedido no se crea ya cobrado | Servicio, al construir el pedido | Es la barrera de FR-002 y tiene que estar del lado del servidor |
| Un pedido sin renglones no se crea | `domain`, con los que ya validan el pedido | Es lógica pura y ya hay dónde |
| No se agregan renglones a un pedido terminal | `domain`, junto a `CanTransition` | Es la misma pregunta que ya responde ese archivo: qué se puede hacer según el estado |
| Qué renglones no han salido a cocina | `domain`, función pura sobre los renglones | Se prueba sin base de datos y es lo que decide qué imprime la comanda |
| Un pedido en curso es "no terminal ∪ con saldo" | Consulta | Es un conjunto de filas, no una regla |

## El conjunto "pedidos en curso"

No es el de `ListUnpaidOrders`. Es la **unión** de dos cosas que la barra tiene que distinguir:

| Grupo | Predicado | Qué se puede hacer | Por qué está |
| --- | --- | --- | --- |
| En preparación | `status in ('abierta','lista')` | Agregarle y cobrarlo | Es al que el cliente le pide más |
| Con saldo | `paid < total` y no cancelada/reembolsada | Cobrarlo | Es dinero en riesgo. Incluye el entregado sin cobrar, que es el caro: el cliente ya se fue |

Un pedido puede estar en los dos. Fundir la píldora sin esta unión perdería el segundo grupo, que es
justo para lo que la píldora se construyó.

## Concurrencia entre estaciones

- **Agregar es append.** Dos estaciones agregando al mismo pedido insertan renglones distintos; no
  hay campo que se pise. No hace falta bloqueo.
- **Agregar después de cobrar deja saldo.** Es correcto y ya se ve: el pedido reaparece en el grupo
  "con saldo". No se inventa nada.
- **Agregar a un pedido terminal se rechaza**, con el estado en el mensaje: la tableta que estuvo
  suspendida media hora tiene que enterarse de qué pasó, no crear un renglón huérfano.
- **Confirmar dos veces la misma cuenta no crea dos pedidos.** `orders` ya tiene el `client_uuid`
  del cliente para eso; hay que comprobar que el camino nuevo lo sigue usando y no lo regenera en
  cada reintento.
