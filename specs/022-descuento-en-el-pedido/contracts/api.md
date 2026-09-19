# Contrato de API: descuento del pedido

## `POST /api/v1/orders` — crear el pedido con descuento

Dos campos nuevos en el body, **mutuamente excluyentes** y los dos opcionales:

```jsonc
{
  "clientUuid": "…",
  "serviceType": "domicilio",
  "deliveryPlatformId": 1,
  "lines": [ /* … */ ],
  "discountAmount": 50.00,    // pesos; o
  "discountPercent": 20       // 0–100, se resuelve contra el subtotal DEL SERVIDOR
}
```

- Ausentes los dos = sin descuento.
- Presentes los dos = `400` (`ErrValidation`). No "gana uno": la ambigüedad se rechaza.
- El **total nunca se manda**; lo calcula el servidor, igual que los precios.

## `PUT /api/v1/orders/{id}/discount` — cambiar o quitar el descuento

```jsonc
{ "discountAmount": 30.00 }   // o { "discountPercent": 10 }, o {} para quitarlo
```

| Caso | Respuesta |
|---|---|
| Pedido abierto o cobrado en parte | `200` con el `OrderView` completo, total ya recalculado |
| Pedido cobrado por completo | `409` (`ErrConflict`) |
| Pedido cancelado o reembolsado | `409` (`ErrConflict`) |
| Descuento mayor que el subtotal | `422` (`ErrDescuentoMayorQueLaVenta`), con el máximo en el mensaje |
| Entrada malformada | `400` (`ErrValidation`) |

Autenticación: `RequireAuth` + los mismos roles que ya pueden capturar y cobrar pedidos.

## Respuesta — `OrderView` gana un campo

```jsonc
{
  "id": 42,
  "subtotal": "385.00",
  "discount": "50.00",     // NUEVO: siempre presente, "0.00" cuando no hubo
  "deliveryFee": "35.00",
  "total": "370.00",
  "outstanding": "370.00",
  "lines": [ /* … */ ]
}
```

`discount` viaja **siempre**, incluso en cero: un campo que a veces falta obliga a cada pantalla a
adivinar, y el front ya aprendió esa lección con los arreglos que llegaban como `null`.

Quién lo aplicó y cuándo viajan solo en el **detalle de la venta**, no en el tablero: el tablero
pinta tarjetas de cocina y no tiene dónde ponerlo.
