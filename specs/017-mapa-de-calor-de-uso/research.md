# Research: el mapa de calor de uso

Las decisiones que había que tomar antes de escribir código, con lo que se descartó y por qué.

## 1. Cómo se entrega la medición sin tocar al operador

**Decisión**: cola en memoria en el POS, envío **en lote** por `fetch` con `keepalive` y **sin
`await`**. Se vacía a los 20 eventos, a los 10 segundos, o cuando la pestaña se oculta. Si el envío
falla, se descarta sin reintentar.

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

**Decisión**: cuenta la **entrada a una ruta**. No cuenta si esa misma pantalla ya se contó **hace
menos de 5 segundos en la misma pestaña**. La marca vive en `sessionStorage` —sobrevive a un F5 y
muere con la pestaña—.

**Rationale**: el edge case del spec es real: el operador aprieta F5 por costumbre y eso no es
«volvió a entrar». Cinco segundos cubren la recarga (1–2 s) y el doble toque, sin tragarse una
navegación de verdad: nadie entra a Caja, sale y vuelve en menos de cinco segundos a propósito.

| Alternativa | Por qué no |
|---|---|
| Contar toda entrada, incluida la recarga | Infla justo las pantallas que más se recargan, que son las que el operador mira cuando algo va lento. El dato mentiría en la dirección más cara |
| Marca en memoria (no en `sessionStorage`) | Una recarga borra la memoria: el caso que el anti-rebote existe para cubrir es exactamente el que se le escaparía |
| Marca en `localStorage` | Sobrevive al cierre de la pestaña y al día siguiente: una pantalla abierta hoy se comería la primera apertura de mañana |

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
