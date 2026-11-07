# Research — 068, entrega 1

## Nombre del permiso

- **Decision**: `orders.delivered_view`.
- **Rationale**: sigue la forma `<área>.<objeto>_view` de `inventory.health_view`; es una capacidad
  que el operador entiende («ver los entregados») y se guardará como dato en la entrega 2.
- **Alternatives**: `orders.view_delivered` (rompe la forma ya usada); `orders.reprint` (nombra un
  botón, no una capacidad, y la lista sirve también para cobrar lo que se debe).

## Devolver dinero de un pedido entregado

- **Decision**: `POST /orders/{id}/refund` y `GET /orders/{id}/refund-info` piden `payments.void`.
- **Rationale**: es la misma capacidad que devolver un pago (sacar dinero cobrado del corte hacia el
  cliente) y hoy las dos son de admin y gerente: el cambio no mueve a nadie. Al tocar estas rutas,
  la constitución pide pasarlas a permiso; dejar el `RequireRole` obligaría a la pantalla a seguir
  preguntando por rol para ofrecer «Devolver».
- **Alternatives**: un permiso nuevo `orders.refund` (dos permisos para una capacidad: lo que la
  regla «uno por capacidad» evita); dejar `RequireRole` (la pantalla tendría que espejar el rol).

## Cuándo ve la cajera el permiso nuevo

- **Decision**: no se fuerza nada; los permisos viajan en la sesión (`/auth/login`, `/auth/refresh`,
  `/auth/me`), así que llegan en la siguiente renovación.
- **Rationale**: el servidor decide al instante (pregunta por el rol de la sesión contra el mapa
  nuevo); solo la pantalla espera a la renovación.

## Migración

- **Decision**: ninguna. El permiso nace en el mapa fijo de `domain`, que todas las empresas
  comparten. No hay tabla con permisos que rellenar.
