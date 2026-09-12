# Implementation Plan: Un mapa de calor de uso

**Branch**: `017-mapa-de-calor-de-uso` | **Date**: 2026-09-12 | **Spec**: [spec.md](./spec.md)

## Resumen

Contar cuántas veces se abre cada pantalla y cuántas veces se dispara cada acción con nombre,
guardar **el rol y nunca la persona**, y pintarlo como mapa de calor dentro de la consola de
plataforma (spec 016) con CSS propio.

La forma corta: el POS acumula eventos en memoria y los manda **en lote, sin esperar la respuesta**;
el servidor los valida contra una lista blanca, escribe el grano fino y **sube el agregado del día
en la misma transacción**; la consola lee **solo el agregado**, con su rol de base, que no alcanza
el grano fino.

## Contexto técnico

**Lenguaje**: Go 1.27 (backend), TypeScript/React 19 (POS y consola).

**Almacenamiento**: PostgreSQL 16 con RLS por empresa. Dos tablas nuevas.

**Pruebas**: unitarias en `domain`, integración contra Postgres real bajo los tres roles
(`gatobobah`, `gatobobah_app`, `gatobobah_platform`), vitest para el POS y para la consola.

**Dónde corre la pantalla**: en la **computadora** de quien opera la plataforma, no en tableta
(FR-014, corregido el 2026-09-12 — el mapa vive dentro de la consola 016).

**Restricciones medidas en producción el 2026-09-12**, que son las que mandan aquí:

| Qué | Cuánto | Por qué importa |
|---|---|---|
| Pedidos por día (últimos 30) | **3.1** | El volumen de eventos sale de aquí, no de una suposición |
| Usuarios activos | **9**: 4 admin, 5 gerente, **0 cajeros** | FR-009: hoy ningún rol identifica a nadie… y el primer cajero que entre, sí |
| Base completa | **18 MB** | El techo de esta feature se compara contra esto, no contra el disco |
| Disco de la VM | 29 GB, 18 GB libres | El problema nunca fue el disco; es no dejar crecer algo sin límite |
| RAM | 955 MB, 300 libres | Nada de procesos nuevos: el rollup vive dentro del binario que ya corre |

**Escala de diseño**: se dimensiona para un local **ocupado** (100 pedidos/día, 30× el de hoy), no
para el de hoy. Un techo calculado con 3.1 pedidos/día no diría nada.

## Constitution Check

| Principio | Cómo lo cumple este plan |
|---|---|
| **I. Layering** | `httpapi` decodifica el lote y no decide nada · `app.UsageService` orquesta la tx · `domain` tiene la lista blanca, el pre-agregado del lote y la regla de k-anonimato · `store/db` por sqlc |
| **II. Errores envueltos** | Sentinels propios; el endpoint de ingesta **nunca** devuelve un error útil al cliente (responde 204 pase lo que pase) y el error se envuelve hacia el log |
| **III. Dinero** | No aplica: **esta feature no toca un solo importe**, y FR-003 lo prohíbe explícitamente |
| **IV. Test-first** | Cada tarea de código va después de su test. La migración con su test de integración (lo exige el hook de pre-commit) |
| **V. Seguridad** | Ingesta autenticada con el JWT del negocio y acotada por RLS; lista blanca contra valores arbitrarios; límite de tamaño de lote; rate limit por usuario; la consola solo alcanza el agregado |
| **VI. YAGNI** | Cero dependencias nuevas (FR-007). Sin cola persistente, sin reintentos, sin job externo |
| **VII. Comentarios** | El porqué de no contar la recarga, del `jsonb` vacío y de la supresión por rol va en el código |
| **VIII. Puertas** | Ver abajo |

### Las puertas del principio VIII

| Puerta | Qué la cerraría | Qué hace este plan |
|---|---|---|
| **Coordenadas del toque** (FR-013) | Guardar **solo** conteos: un agregado por día no tiene dónde meter un punto | `usage_events` guarda el grano fino con `detail jsonb` vacío hoy. Agregar `{"x":…,"y":…}` después no migra nada |
| **Más de una sucursal** | Un evento que solo sepa de empresa y no pueda decir de dónde salió | El mismo `detail` lo admite, y si se vuelve feature se agrega columna con backfill nulo |
| **Distinguir estaciones** (la puerta abierta de la constitución) | Guardar la persona hoy «para poder cortar mañana» | **No se guarda**. El día que haya estación, es un campo nuevo del evento, y el rol sigue siendo el corte correcto |
| **Medir cuánto rato, no cuántas veces** | — | Se decidió medir aperturas. Medir tiempo exigiría un evento de salida por cada uno de entrada: se puede agregar después sin tocar lo escrito |

**Ninguna puerta se cierra.** La que sí se cierra a propósito es la que el spec cierra: **no se
guarda quién fue**, y eso no se puede «abrir» después para los datos ya escritos — que es justo el
punto de US2.

## Las tres decisiones que ordenan todo

### 1. La medición no está en el camino del operador, ni siquiera un poco

`fetch` con `keepalive`, **sin `await`**, sin reintento y sin tocar el estado de la pantalla. La
cola vive en memoria y se vacía por tres motivos: 20 eventos, 10 segundos, o la pestaña se oculta.
Si falla, **se pierde y nadie se entera** (FR-004).

**Un solo lote en vuelo a la vez.** Si el envío anterior sigue pendiente, se sigue acumulando y el
siguiente se pospone. Con wifi lento pero no caído —el escenario que este diseño existe para
aguantar— un `fetch` tarda 8–15 s, el temporizador dispara otro encima, y esos lotes terminan
compitiendo por la conexión con el `POST /orders/:id/pay`. No bloquean al operador, pero le hacen
más lento el cobro, que es lo mismo que US3 promete evitar.

**Por qué `fetch(keepalive)` y no `navigator.sendBeacon`**: el beacon no deja poner el header
`Authorization`, y esta ingesta va autenticada para que el servidor sepa la empresa y el rol sin
que el cliente los mande. Con `keepalive` el navegador termina el envío aunque la pestaña se cierre,
que es lo único que el beacon daba de más.

**El orden nunca se invierte**: primero ocurre la acción, después se encola. Un `await` colado ahí
convertiría un cobro en dependiente de la red, y eso la constitución lo prohíbe en sus
*Restricciones del producto*.

### 2. El agregado se sube al escribir, no en un trabajo aparte

Cada lote hace, en **una transacción**: insertar el grano fino y `upsert` del conteo del día,
**pre-agregando dentro del lote** — se agrupa por `(pantalla, acción, rol)` y sale un solo
`update … set hits = hits + n` por combinación, no uno por evento. Medido: sin eso, las 135 filas de
un día con 2,000 incrementos encima pasan de 64 kB a 232 kB de versiones muertas antes de que
autovacuum llegue, y con 54,000 filas vivas autovacuum tarda días en disparar.

**Por qué no un rollup por hora**, que era lo obvio: un trabajo periódico que falla es invisible
—nadie mira un mapa que lleva tres días sin moverse y asume que nadie usó el sistema— y obliga a
mirar un mapa que siempre va una hora atrás. Con 3.1 pedidos/día medidos (y 100 en el escenario de
diseño) la contención del `upsert` no existe: son decenas de filas por día y por empresa.

`// ponytail:` si algún día una empresa mete miles de eventos por minuto, ese `upsert` se vuelve el
punto caliente y ahí sí toca un rollup por lotes. El techo está escrito en el código.

Lo único periódico es **borrar lo viejo**, dentro del binario que ya corre. No se agrega un cron ni
un contenedor: la VM tiene 300 MB de RAM libres.

**Corre al arrancar y luego cada 24 horas**, y el «al arrancar» es el que importa: un ticker de 24 h
se reinicia con el proceso, y aquí se redespliega en cada merge. Un binario que no vive un día
entero seguido **nunca** llega a recortar, sin que nada falle ni se vea.

### 3. El anonimato se decide al ESCRIBIR, y por eso no se puede deshacer

FR-009 dice que no se ofrezca un corte por rol que identifique a una persona por eliminación. La
tentación es resolverlo en la pantalla —«si hay un solo gerente, no muestres el corte»— y está mal:
esa decisión la tomaría quien lee, y **la consola no puede tomarla**, porque su rol de base no tiene
permiso sobre `users` para contar cuántos hay.

**Decisión**: al escribir el agregado, el servidor cuenta cuántos usuarios activos tiene ese rol en
esa empresa. Si son menos de dos, el renglón se guarda con **rol nulo** («sin corte»). Lo que nunca
se escribió no se puede consultar, ni con acceso a la base.

**Medido hoy**: 4 admin y 5 gerentes — ningún corte identifica a nadie. **Cero cajeros**: el día que
entre el primero, sus eventos caen en «sin corte» solos. Esa es exactamente la forma en que esta
regla tiene que funcionar, sin que nadie se acuerde de encenderla.

## Estructura

```text
server/
├── migrations/0069_uso_del_sistema.sql        # dos tablas, grants, RLS, política de plataforma
├── queries/uso.sql                            # ingesta, agregado y lectura de la consola
├── internal/domain/uso.go                     # lista blanca, anti-rebote, k-anonimato (puro)
├── internal/app/uso.go                        # UsageService: la tx de ingesta, la lectura y el recorte
└── internal/httpapi/handlers_uso.go           # POST /usage (negocio) · GET /platform/usage

web/
├── src/api/uso.ts                             # el registrador del POS: cola, lote, keepalive
├── src/app/rutas-medidas.ts                   # qué pantalla es cada ruta (lista blanca del front)
├── src/consola/MapaDeUso.tsx                  # el mapa, CSS propio, sin librerías
└── src/consola/etiquetas-de-uso.ts            # cómo se llama cada pantalla EN LA CONSOLA
```

La última es una **copia deliberada**, no un descuido: la consola no puede importar la lista del POS
—`eslint.config.js` se lo prohíbe— y no debería, porque son dos productos con dos ciclos de vida. El
POS manda slugs (`pos`, `caja`) y la consola decide cómo se leen.

**Dos piezas separadas y no una**: el registrador vive del lado del POS y el mapa del lado de la
consola. No se tocan, y `eslint.config.js` ya lo impide en las dos direcciones.

## La forma del mapa: un solo eje

Pantallas **ordenadas por uso**, cada una con su celda sombreada y su número escrito, y sus acciones
desplegables debajo. **No hay eje temporal**: no es una rejilla de días tipo calendario de
contribuciones. El periodo se elige y el mapa muestra ese periodo completo.

Se escribe aquí porque «mapa de calor» y «rejilla» invitan a construir dos dimensiones, y el
contrato no las tiene. La pregunta que esta pantalla responde —cuál se usa mucho, cuál no usa
nadie— se contesta con un eje; comparar periodos entre sí es otra feature y está descartada abajo.

## Lo que este plan NO construye

- **El mapa por coordenadas.** Es la puerta abierta, no la feature (FR-013).
- **Tiempo de permanencia.** Se cuentan aperturas; medir rato exige un evento de salida por cada
  entrada y el spec ya explica por qué no (la tableta que se queda abierta toda la noche).
- **Alertas, comparativas entre periodos, exportar a CSV.** No los pide nadie todavía.
- **Medir la consola a sí misma.** Se instrumenta el POS, que es donde está la pregunta.
