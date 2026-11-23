# Research: Alerta continua de pedido entrante

Lo que se leyó del código antes de diseñar, y las decisiones que salen de ahí.

## Lo que existe hoy (spec 021)

| Pieza | Dónde | Qué hace |
|---|---|---|
| Pedidos pendientes | `platform_incoming_orders` (0073), estado `pendiente/aceptado/rechazado/cancelado/expirado` | Se crea al llegar el webhook; `decide_before` = llegada + 11.5 min |
| Listar | `GET /orders/platform/pending` → `PedidosDePlataformaService.Pendientes` | Sin gate de permiso. No filtra por `decide_before` |
| Aceptar | `POST /orders/platform/{id}/accept` → `Aceptar` | Primero la plataforma, después la base; `AcceptIncomingOrder` con `where state='pendiente'` resuelve la carrera |
| Rechazar | `uber.Client.RechazarPedido` y la consulta `DenyIncomingOrder` existen | **Ningún servicio ni ruta los usa** |
| Cancelación de la plataforma | `domain.ClasificarAviso` reconoce `orders.cancel`/`orders.failure` | `procesar` la confirma y **no hace nada**: el pedido queda `pendiente` para siempre |
| Eventos | `platform.order.received` (webhook), `platform.order.decided` (aceptar), broker en memoria por proceso, `/events` SSE con ping cada 25 s | El front reconecta con `alReconectar` y sondea cada 30 s |
| Pantalla | `AvisoDePlataforma` solo en `POSPage` | Franja con el más urgente + Aceptar; hoja con la lista; sin sonido, sin rechazar |

## Decisiones

### D1. Módulo común: tipo y contrato comunes, despacho por origen con un `switch`

- **Decision**: `domain.IncomingOrder` (forma común) y `app.IncomingOrdersService`, que junta lo
  pendiente de cada origen y despacha aceptar/rechazar por `source` con un `switch`. Hoy un solo
  caso: `platform`, que delega en `PedidosDePlataformaService`. Rutas nuevas
  `/orders/incoming[/{source}/{id}/accept|reject]` y un solo evento `incoming.changed`.
- **Rationale**: lo intercambiable es el contrato (lo que la pantalla y las rutas ven), y eso queda
  fijo desde hoy. El pedido web (#328) agrega un caso al `switch` y una fuente a la lista, sin tocar
  alerta, indicador ni sección.
- **Alternatives**: una interfaz `IncomingSource` con una sola implementación — la constitución VI
  la prohíbe hasta que exista el segundo consumidor; el `switch` da la misma costura sin
  abstracción. Copiar la alerta en el pedido web — es justo lo que el issue pide evitar.

### D2. «Pendiente» lo decide el servidor, incluido el vencimiento y la cancelación

- **Decision**: pendiente = `state = 'pendiente'` **y** (`decide_before` nulo o futuro). Aceptar y
  rechazar fuera de plazo devuelven `ErrPedidoYaDecidido`. El webhook de cancelación pasa el pedido
  a `cancelado` y avisa a las tabletas.
- **Rationale**: con alerta continua, un pedido que la plataforma ya canceló sonaría para siempre
  (hoy la cancelación se ignora). El plazo como filtro de lectura no necesita un proceso que corra
  cada minuto: es la misma regla aplicada donde se lee. No se escribe `expirado` en la fila; el
  estado real de la fila sigue siendo `pendiente` y nadie lo puede decidir.
- **Alternatives**: un job que marque `expirado` — otra pieza que puede fallar, para el mismo
  resultado en pantalla; se deja para cuando un reporte necesite contar vencidos.

### D3. Rechazar con motivo en palabras de quien opera

- **Decision**: lista cerrada en `domain` con 5 motivos (`closed`, `out_of_stock`, `too_busy`,
  `address`, `other`), cada uno con su texto en español y su código de la plataforma
  (`STORE_CLOSED`, `ITEM_AVAILABILITY`, `CAPACITY`, `ADDRESS`, `OTHER`). Se guarda el código propio
  en `deny_reason`. Primero la plataforma, después `DenyIncomingOrder` (`:execrows`, 0 filas =
  otra tableta ganó).
- **Rationale**: los 11 códigos de la plataforma incluyen varios que solo entiende un integrador
  (`POS_NOT_READY`, `POS_OFFLINE`, `PRICING`). El pedido web tendrá su propia traducción del mismo
  motivo propio.
- **Rastro (FR-008)**: `decided_by`, `settled_at` y `deny_reason` ya existen en la fila. Aceptar ya
  deja `order.created` en la bitácora; rechazar no crea pedido, y la fila es el rastro.

### D4. Permiso `orders.accept_incoming` (id 39)

- **Decision**: migración 0119 inserta el permiso, lo da a los 4 roles sugeridos en
  `system_role_permissions` (herencia), y a cada rol **propio** existente le agrega un override
  `granted = true` una sola vez. Las rutas nuevas y las viejas de plataforma lo exigen.
- **Rationale**: hoy cualquier sesión ve y acepta; un rol propio no hereda permisos nuevos
  (constitución), así que sin el override sus usuarios dejarían de oír pedidos al subir. El override
  es una decisión de la empresa ya tomada en los hechos, no herencia: los roles propios que se creen
  después no lo traen.
- **Riesgo de número**: 0119 y el id 39 los pueden tomar las ramas del #325/#326. La que llegue
  después a `develop` renumera las dos cosas.

### D5. La alarma: Web Audio con un búfer generado en bucle

- **Decision**: un `AudioContext` único; se genera en memoria un búfer de ~2 s (dos pitidos de
  880 Hz y silencio) y se reproduce con `loop = true`. Desbloqueo: escucha global de `pointerdown` y
  `keydown` en captura que llama `resume()`. Estado `bloqueado` si `ctx.state !== 'running'` con
  pendientes: se ve el aviso «Toca para activar el sonido». Al volver a primer plano se intenta
  `resume()` otra vez.
- **Rationale**: un bucle de audio no depende de `setInterval`, que el navegador frena en segundo
  plano; generarlo evita un archivo de audio y su caché. Cambiar el tono es cambiar una función
  (FR-013).
- **Alternatives**: `<audio loop>` con archivo — agrega un recurso y el mismo bloqueo; Notification
  API — pide permiso del sistema y no suena continuo.

### D6. Resistencia: cuatro caminos para enterarse

| Camino | Cubre |
|---|---|
| Evento `incoming.changed` por `/events` | El caso normal, < 1 s |
| `alReconectar` al reabrir el canal | Red caída, subida azul/verde |
| `visibilitychange` → visible | Tableta suspendida |
| Sondeo cada 15 s | Canal abierto pero muerto (TCP a medias) |

Un error de red conserva los datos de la última respuesta (TanStack Query no los borra), así que
la alarma sigue sonando sin red (FR-021). 15 s y no 5: con 3 tabletas son 12 peticiones por minuto
y cada una queda en el journal; el criterio de 5 s lo cumplen los otros tres caminos.

### D7. Probar en local sin la plataforma real

- **Decision**: `PLATFORM_ORDERS_SIMULATED=true` hace que el decisor confirme aceptar y rechazar sin
  red. `config.Validate` se niega a arrancar con él fuera de `APP_ENV=development`. Los pedidos se
  siembran con un script de desarrollo que inserta filas pendientes en la base local.
- **Rationale**: Uber está apagado y no hay pedido de prueba posible; sin esto, aceptar y rechazar
  no se pueden probar de punta a punta. La VM de pruebas corre con `APP_ENV=production`, así que ahí
  tampoco se enciende.
- **Alternatives**: un servidor falso de Uber — exige volver configurable la URL de la plataforma,
  que hoy es constante a propósito.

### D8. Compatibilidad durante la subida

- Las rutas `/orders/platform/pending` y `/orders/platform/{id}/accept` y los eventos
  `platform.order.*` se conservan esta subida: una tableta con el front viejo en caché los sigue
  usando. Se quitan en una subida posterior (anotado en el plan).
- La migración solo agrega filas: la API vieja no lee el permiso nuevo y sigue funcionando.
- Front nuevo contra API vieja (minutos entre el deploy de Pages y el de la API): `/orders/incoming`
  da 404, la sección se queda vacía y el sondeo lo recupera en cuanto sube la API.
