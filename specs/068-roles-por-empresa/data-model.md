# Data model — 068, entrega 1

Sin cambios de esquema.

| Rol sugerido | `orders.delivered_view` | `payments.void` |
| --- | --- | --- |
| admin | sí (nuevo) | sí (sin cambio) |
| gerente | sí (nuevo) | sí (sin cambio) |
| cajero | sí (nuevo) | no |
| mesero | no | no |

El mapa vive en `domain.PermissionsFor`; es la semilla que la entrega 2 pasará a la base.
