# Contrato — rutas de entregados y devolución

El cuerpo de las respuestas no cambia; cambia quién recibe 200 y quién 403.

| Ruta | Antes | Ahora | 403 dice |
| --- | --- | --- | --- |
| `GET /api/v1/orders/delivered` | rol admin o gerente | permiso `orders.delivered_view` | «Tu usuario no puede ver los pedidos entregados» |
| `POST /api/v1/orders/{id}/refund` | rol admin o gerente | permiso `payments.void` | «Tu usuario no puede devolver pagos» |
| `GET /api/v1/orders/{id}/refund-info` | rol admin o gerente | permiso `payments.void` | «Tu usuario no puede devolver pagos» |

`GET /api/v1/orders/{id}` (lo que lee la reimpresión) sigue abierto a toda sesión.

La sesión (`/auth/login`, `/auth/refresh`, `/auth/me`) trae `orders.delivered_view` en `permissions`
para admin, gerente y cajero.
