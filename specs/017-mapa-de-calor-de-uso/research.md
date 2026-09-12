# Research: el mapa de calor de uso

Las decisiones que había que tomar antes de escribir código, con lo que se descartó y por qué.

## 1. Cómo se entrega la medición sin tocar al operador

**Decisión**: cola en memoria en el POS, envío **en lote** por `fetch` con `keepalive` y **sin
`await`**. Se vacía a los 20 eventos, a los 10 segundos, o cuando la pestaña se oculta. Si el envío
falla, se descarta sin reintentar. **Y nunca hay más de un lote en vuelo**: si el anterior sigue
pendiente, se sigue acumulando en la cola —hasta el tope de 50— y el envío se pospone.

Lo de «uno en vuelo» lo encontró la revisión de arquitectura y no es teórico: con wifi lento **pero
no caído** —que es el escenario que motiva todo este diseño, no el de la red muerta— un `fetch`
tarda 8–15 s en resolverse, el temporizador de 10 s dispara otro encima, y a la media hora hay
varios lotes compitiendo por la misma conexión flaky **con el `POST /orders/:id/pay`**. Ninguno
bloquea al operador, pero le pueden hacer más lento el cobro, que es exactamente lo que US3 promete
que no pasa.

**Rationale**: FR-004 y FR-005 no piden «que sea rápido»: piden que **no exista** un camino donde la
medición pueda demorar o romper una acción. Lo único que garantiza eso es no esperarla nunca. Y el
lote es lo que evita 200 peticiones sueltas en una hora pico desde una tableta con wifi de
restaurante.

| Alternativa | Por qué no |
|---|---|
| `navigator.sendBeacon` | No admite el header `Authorization`, y esta ingesta va autenticada para que el **servidor** ponga empresa y rol. Mandarlos desde el cliente sería dejar que el cliente diga de qué rol es |
| Cola persistente en `localStorage` con reintentos | Es construir garantías de entrega para un dato que el spec autoriza a perder. Además crece sin dueño en el navegador de una tableta y compite con `egb:ticket:v2`, que sí importa |
| WebSocket / SSE hacia el servidor | Una conexión viva más por tableta para mandar contadores. Más superficie, cero beneficio |
| Medir en el servidor, contando requests | Solo ve lo que llega al backend: no vería abrir una pantalla que se sirve del caché, ni distinguiría «abrí Reportes» de «la pantalla pidió datos tres veces» |

## 2. Qué cuenta como «se abrió una pantalla»

**Decisión**: cuenta **toda entrada a una ruta**, con una sola excepción: **la primera vista después
de una recarga no cuenta**, y eso se sabe preguntándole al navegador —
`performance.getEntriesByType('navigation')[0].type === 'reload'`— en vez de adivinarlo con un
cronómetro.

**Rationale**: el edge case del spec es real —el operador aprieta F5 por costumbre y eso no es
«volvió a entrar»— pero la primera versión de esta decisión lo resolvía con una ventana de 5
segundos en `sessionStorage`, y la revisión de arquitectura la tumbó por los dos lados a la vez:

- **Se le escapa el caso que venía a cubrir.** Un F5 en una tableta con wifi degradado, caché frío y
  1,133 kB de paquete tarda **más** de 5 s en volver a la ruta. Para entonces la marca expiró y la
  recarga cuenta igual.
- **Y subcuenta justo lo que más importa medir.** En el mostrador sí se entra y se sale de una
  pantalla en segundos: un cajero rebota entre el menú y la cuenta varias veces mientras arma un
  pedido. Con la ventana, solo la primera de esas entradas contaba — y esas pantallas de consulta
  frecuente son las que SC-001 quiere poder nombrar.

Preguntarle al navegador si fue una recarga no tiene ninguno de los dos problemas, **y quita
código**: se va el `sessionStorage`, se va la constante de 5 segundos y se va el estado que había
que mantener.

| Alternativa | Por qué no |
|---|---|
| Ventana de N segundos en `sessionStorage` | Lo de arriba: se le escapa la recarga lenta y se come las entradas rápidas legítimas |
| Contar toda entrada, recarga incluida | Infla justo las pantallas que más se recargan, que son las que el operador mira cuando algo va lento. El dato mentiría en la dirección más cara |
| Marca en `localStorage` | Sobrevive al cierre de la pestaña: una pantalla abierta hoy se comería la primera apertura de mañana |

**Lo que se pierde, y se acepta**: tras una recarga, la vista que el operador sí está mirando no se
cuenta esa vez. Una apertura de menos es más barata que inflar sistemáticamente las pantallas
lentas.

## 3. Dónde y con qué forma se guarda

**Decisión**: dos tablas. `usage_events` con el grano fino y `detail jsonb` (vacío hoy), retención
**14 días**; `usage_daily` con el conteo por empresa, día, pantalla, acción y rol, retención
**13 meses**. La consola lee **solo** `usage_daily`.

**Rationale**: el grano fino es lo que mantiene abierta la puerta de las coordenadas (FR-013) y lo
que permite recalcular un agregado si mañana se cuenta distinto; el agregado es lo que hace que el
volumen no crezca (FR-010) y lo único que la pantalla necesita leer. Los 13 meses son para poder
comparar un mes contra el mismo mes del año pasado, que es la comparación que un negocio de comida
pide primero.

| Alternativa | Por qué no |
|---|---|
| Solo el agregado | Cierra la puerta de FR-013 de golpe: un conteo por día no tiene dónde meter un punto de coordenadas |
| Solo el grano fino, agregando al leer | Un año de eventos escaneado en cada apertura de la pantalla, y sin techo declarable. Con 100 pedidos/día son ~700k filas/año por empresa |
| Una tabla de series de tiempo / extensión | Dependencia nueva en una VM de 955 MB, para contar decenas de filas al día |

## 4. Cómo se cumple «por rol, nunca por persona» de verdad

**Decisión**: el evento guarda el rol y **no** el `user_id`. Y al escribir el agregado, si ese rol
tiene **menos de dos usuarios activos** en esa empresa, el renglón se guarda con **rol nulo**.

**Rationale**: guardar el rol y confiar en que la pantalla no lo muestre deja el dato escrito —y
quien tenga la base lo lee—. La supresión al escribir es la única que no se puede deshacer. Además
la consola **no podría** decidirlo al leer: su rol de base no tiene permiso sobre `users` para
contar cuántos hay, y dárselo sería abrir la puerta que la spec 016 cerró.

**Medido en producción el 2026-09-12**: 4 admin, 5 gerentes, **cero cajeros**. Hoy ningún corte
identifica a nadie; en cuanto entre el primer cajero, sus eventos caen solos en «sin corte».

| Alternativa | Por qué no |
|---|---|
| Guardar `user_id` y no mostrarlo | Es exactamente lo que US2 prohíbe: lo que se escribió, se escribió |
| Guardar un hash del usuario | Un hash con un espacio de nueve personas se invierte en un segundo. Da falsa sensación de anonimato |
| Decidir la supresión en la pantalla | La consola no puede contar la plantilla de un cliente, y no debe poder |

## 5. Qué se puede contar, y qué pasa con lo demás

**Decisión**: **lista blanca** de pantallas y de acciones en `domain`. Lo que no está en la lista se
descarta en el servidor, en silencio, y el request responde 204 igual.

**Rationale**: sin lista, el número de valores distintos lo decide el cliente: un bucle o una versión
vieja del front llenan la tabla de basura y el agregado deja de agregarse. La lista además es la que
hace legible el mapa — «cobrar» es una acción, «clic en el botón azul» no.

**Por qué 204 y no 400**: el cliente no espera la respuesta ni la puede usar; un 400 solo serviría
para que alguien lo vea en la pestaña de red y crea que algo se rompió. Lo que sí queda es un
contador en el log del servidor: si una versión del front manda nombres que no existen, se ve ahí.

## 6. Qué pinta el mapa, sin librerías

**Decisión**: una rejilla CSS de celdas con intensidad por `background-color` calculada contra el
máximo del periodo, **y el número escrito dentro de cada celda**.

**Rationale**: FR-007 y el principio VI. Una escala de color son cuatro líneas de CSS; una librería
de gráficas son 200 KB, un calendario de versiones ajeno y un CVE que bloquea el merge. Y el número
escrito no es decoración: una escala de color sola es ilegible para quien no distingue esos tonos, y
además obliga a adivinar cuánto es «más oscuro».

| Alternativa | Por qué no |
|---|---|
| Recharts / Chart.js / D3 | Dependencia nueva para pintar rectángulos. La constitución lo prohíbe donde la stdlib alcanza, y aquí alcanza CSS |
| Canvas propio | Más código que CSS, no se puede seleccionar el texto y hay que reimplementar el redibujado |
| Solo color, sin número | Ilegible para daltonismo y obliga a comparar tonos a ojo. El número cuesta cero |
