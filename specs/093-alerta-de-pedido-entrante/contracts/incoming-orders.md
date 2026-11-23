# Contrato: pedidos entrantes

Todas bajo `/api/v1`, con sesión y `RequirePermission(orders.accept_incoming)`.

## GET /orders/incoming

200:

```json
{
  "orders": [
    {
      "source": "platform",
      "id": 42,
      "sourceName": "Uber Eats",
      "displayId": "A1B2C",
      "customerName": "Ana",
      "customerPhone": "",
      "address": "",
      "serviceType": "domicilio",
      "total": "245.00",
      "placedAt": "2026-11-23T19:02:00Z",
      "decideBefore": "2026-11-23T19:13:30Z",
      "lines": [
        { "name": "Taro grande", "quantity": "2", "unitPrice": "85.00", "matched": true,
          "options": [ { "name": "Perlas", "quantity": "1", "unitPrice": "10.00", "matched": true, "options": [] } ] }
      ]
    }
  ]
}
```

- `orders`, `lines` y `options` son SIEMPRE arreglos, nunca `null` (test sobre el JSON crudo).
- Ordenado por urgencia (ver data-model).
- Sin el permiso: 403. Bajo RLS: solo la empresa de la sesión.

## POST /orders/incoming/{source}/{id}/accept

Cuerpo vacío. 200: `{ "id": 9001, "number": 14, "platformRef": "…" }` (el pedido del POS creado).

| Caso | Respuesta |
|---|---|
| `source` desconocido o `id` no positivo | 400 |
| No existe en la empresa | 404 |
| Ya decidido, cancelado o vencido | 409 `ErrPedidoYaDecidido` |
| La plataforma no confirmó | 409 `ErrConflict`; el pedido sigue pendiente |
| Sin credenciales de la plataforma | el mapeo actual de `ErrPlataformaSinCredenciales` |

## POST /orders/incoming/{source}/{id}/reject

Cuerpo `{ "reason": "out_of_stock" }`. 204.

| Caso | Respuesta |
|---|---|
| `reason` ausente o fuera de la lista | 400 |
| Resto | igual que aceptar |

## Evento SSE `incoming.changed`

`data: {}`. Se publica a la empresa al registrar un pedido nuevo, al aceptarlo, al rechazarlo y al
recibir su cancelación. La tableta vuelve a pedir `GET /orders/incoming`; el evento no lleva datos a
propósito (la lista ya sale filtrada por empresa y permiso).

## Compatibilidad (esta subida)

`GET /orders/platform/pending`, `POST /orders/platform/{id}/accept` y los eventos
`platform.order.received` / `platform.order.decided` se conservan para tabletas con el front viejo
en caché, ahora con el mismo permiso. Se retiran en una subida posterior.
