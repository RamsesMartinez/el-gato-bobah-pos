# Implementation Plan: Dónde cae el dedo

**Branch**: `019-mapa-por-coordenada` | **Date**: 2026-09-12 | **Spec**: [spec.md](./spec.md)

## Resumen

Contar en qué **zona** de una pantalla cae cada toque, guardar el rol y nunca la persona, nunca el
instante y nunca una imagen, y pintarlo como una rejilla en la consola de plataforma.

La forma corta: el POS ya tiene un registrador que no estorba —cola en memoria, lote sin esperar
respuesta, uno en vuelo— y **se reusa tal cual**; lo único nuevo del lado de la tableta es traducir
un toque a una celda. El servidor suma esa celda en una tabla de conteos, gemela de la de la 017. La
consola pinta la rejilla con CSS propio.

## Contexto técnico

**Lenguaje**: Go 1.27 (backend), TypeScript/React 19 (POS y consola).

**Almacenamiento**: PostgreSQL 16 con RLS por empresa. **Una** tabla nueva de conteos.

**Pruebas**: unitarias en `domain` (la traducción de toque a celda es pura), integración bajo los
tres roles, vitest para el registrador y para la rejilla.

**Lo que NO se construye porque ya existe**: la cola, el lote, el `keepalive`, la guarda de «uno en
vuelo», el limitador, el descarte silencioso y el borrado periódico. Todo eso es de la 017 y esta
feature entra por el mismo camino.

**Restricciones medidas** (producción, 2026-09-12): 3.1 pedidos/día · base completa de 18 MB · VM
con 1 GB de RAM y 18 GB de disco libres · paquete del POS en 1,134.90 kB.

## Constitution Check

| Principio | Cómo lo cumple |
|---|---|
| **I. Layering** | `domain` traduce toque→celda y valida (puro) · `app` orquesta la tx · `httpapi` no decide nada · `store/db` por sqlc |
| **II. Errores envueltos** | Sentinels propios; la ingesta sigue respondiendo 204 pase lo que pase |
| **III. Dinero** | No aplica: esta feature **no toca un importe**, y FR-009 prohíbe hasta el texto de la pantalla |
| **IV. Test-first** | La migración con su test antes; la traducción a celda es pura y se prueba en tabla |
| **V. Seguridad** | Mismo camino autenticado y limitado de la 017; lista blanca de pantallas; ninguna imagen sale de la tableta |
| **VI. YAGNI** | Cero dependencias. Se reusa el registrador entero en vez de escribir otro |
| **VII. Comentarios** | El porqué de no guardar el instante, de medir contra el vidrio y de la celda gruesa va en el código |
| **VIII. Puertas** | Ver abajo |

### Las puertas del principio VIII

| Puerta | Qué la cerraría | Qué hace este plan |
|---|---|---|
| **Distinguir tamaños de tableta** | Guardar solo la celda, sin decir de qué forma era la pantalla | Se guarda la **orientación**. Una proporción más fina es una columna nueva con backfill nulo |
| **Saber qué control se toca** | Guardar el elemento tocado hoy «por si acaso» — y con él, texto de la pantalla | **No se guarda**. Esa pregunta ya la responde la 017 con las acciones con nombre, sin coordenadas |
| **Una rejilla más GRUESA** | Nada | Se recalcula fusionando celdas: 12×7 sale de 24×14 sumando de a cuatro. Reversible |
| **Una rejilla más FINA** | Ya está cerrada, y hay que decirlo | **No se puede recalcular**: no existe el toque fino del cual derivarla — es la decisión central de esta feature. Cambiarla obliga a declarar el corte y perder la comparación con lo de antes |
| **El recorrido del dedo** | — | **Cerrada a propósito**: exige secuencia, y la secuencia es tiempo (FR-003). Está en *out of scope* del spec |

**La puerta que esta feature cierra, y es deliberado**: no se va a poder saber *cuándo* se tocó algo,
ni reconstruir una sesión. Es el precio de que el anonimato no dependa de que nadie cruce dos tablas.

## Las tres decisiones que ordenan todo

### 1. El toque se convierte en celda **en la tableta**, no en el servidor

Lo que sale del navegador es un número de celda, no una coordenada. El servidor nunca ve un punto.

**Por qué importa**: si viajara `(x, y)` con precisión de píxel, el dato fino existiría —en el
cuerpo del request, en el log de acceso de un proxy, en la memoria del servidor— aunque después se
redondeara. Redondear en el origen es la única forma de que el punto exacto **no exista en ningún
lado**, y es lo que hace verdadera la promesa de FR-003 sin depender de que nadie guarde de más.

La rejilla es **gruesa a propósito**: 12 × 7 celdas sobre el área visible. En 1024×600 son zonas de
unos 85 × 86 px, del tamaño de un botón del POS. Más fina sería un mapa de puntos con otro nombre, y
traería de vuelta el volumen que esta feature evita por diseño.

### 2. Se reusa el registrador de la 017, entero

El mismo `POST /api/v1/usage`, la misma cola, el mismo lote, el mismo limitador, el mismo descarte.
Un evento de toque es un evento de uso que además trae celda.

**Por qué no un endpoint nuevo**: todo lo que hace que la medición no estorbe —no esperar, no
reintentar, uno en vuelo, tope por usuario— ya está escrito **y probado**, incluido un caso de
Playwright con la red muerta. Un camino nuevo sería reescribir esas garantías y volver a
demostrarlas, a cambio de nada.

Lo único que se agrega del lado de la tableta es un escuchador de `pointerup` que compara con el
`pointerdown`: si el dedo se movió más de 10 px, **fue un arrastre y no cuenta** (FR-006). Sin eso,
cada desplazamiento de la lista de productos dejaría un rastro y el mapa mediría scroll en vez de
intención.

Tres detalles que la revisión de arquitectura encontró y que no son detalles:

- **El punto de bajada se guarda por `pointerId`**, en un mapa y no en una variable. En el mostrador
  hay dos manos —una sostiene comida, la otra toca— y dos contactos se solapan: con un solo estado
  compartido, el `down` de un dedo se compara con el `up` del otro y salen arrastres falsos.
- **Los toques dentro de una hoja, un diálogo o el bloqueo NO se cuentan.** Ni `Picker`, ni
  `CobrarSheet`, ni `LockScreen` cambian de ruta: son capas encima de la misma pantalla, así que sus
  toques se atribuirían a la de abajo. El caso que más contamina es el **teclado del PIN**: dejaría
  una zona caliente en el centro que dentro de seis meses alguien va a leer como «un control muy
  usado». Se filtra por el ancestro `[role="dialog"]` —que las hojas de Chakra ya llevan— y a
  `LockScreen` se le pone ese rol, que además es lo correcto: es un modal que bloquea todo.
- **La lista blanca también corre en la tableta.** El escuchador vive en la raíz y ve toda la
  aplicación; sin filtrar en el cliente, encolaría toques de pantallas no instrumentadas durante
  todo el turno para que el servidor los tire — wifi de restaurante gastado en nada. Reusa
  `pantallaDe()` de la 017.

### 3. Una tabla de conteos, no de toques — y esta vez se sabe por qué

Gemela de `usage_daily`: `(empresa, día, pantalla, orientación, celda, rol) → veces`.

**No hay entidad «toque»**, y no es una optimización: es lo que hace posible cumplir a la vez FR-003
y FR-012. La 017 ya pagó esta lección —un renglón por evento con su instante se cruza con
`register_sessions.closed_by` y deshace el anonimato entero, y al tope del limitador eran 4 GB en
dos semanas—. Aquí el volumen sería mucho peor: son miles de toques por turno, no decenas de
aperturas.

Con conteos, **las filas las acota la rejilla**: 84 celdas × 5 cortes de rol × las pantallas
instrumentadas. Mil toques en la misma zona suben un contador y no crean nada.

## La leyenda: 84 números sin referencia no son un mapa

La rejilla se pinta sola —FR-009 prohíbe la captura— y eso deja una pregunta abierta que la revisión
nombró: dentro de seis meses, o con otra persona mirando, ¿qué es la fila 0?

Va una **leyenda de texto, corta y fechada**, al lado de la rejilla: «fila 0: categorías · columnas
9-11: la cuenta · fila 6: barra inferior», con la fecha del layout al que corresponde. Texto y nunca
una imagen, así que no choca con FR-009.

Y va **fechada a propósito**: el día que la pantalla se rediseñe, la leyenda vieja describe un layout
que ya no existe. Con la fecha a la vista, quien mire datos de hace tres meses sabe que la
referencia es de entonces; sin ella, creería que sigue vigente.

## Estructura

```text
server/
├── migrations/0070_toques_por_zona.sql     # una tabla de conteos, grants, RLS, política
├── queries/toques.sql                      # el upsert y la lectura de la rejilla
├── internal/domain/toque.go                # celda válida, orientación, lista de pantallas
├── internal/app/uso.go                     # +Registrar acepta toques · +Rejilla(...)
└── internal/httpapi/handlers_plataforma.go # +GET /api/v1/platform/touches

web/
├── src/api/uso.ts                          # +medirToque(pantalla, celda, orientación)
├── src/app/MedidorDeToques.tsx             # el escuchador: pointerdown/up → celda
├── src/consola/zonas-del-pos.ts            # la leyenda fechada de qué es cada fila y columna
└── src/consola/RejillaDeToques.tsx         # la rejilla, CSS propio
```

## Lo que este plan NO construye

- **Pintar sobre una captura.** Prohibido por FR-009 y es la mitad del spec.
- **El recorrido del dedo.** Exige secuencia; la secuencia es tiempo.
- **Instrumentar todas las pantallas.** Se empieza por el POS, que es donde el dedo está todo el
  día. Agregar otra es una línea en la lista, y la 017 dirá cuál.
- **Zoom o rejilla configurable.** La resolución es una constante del código.
