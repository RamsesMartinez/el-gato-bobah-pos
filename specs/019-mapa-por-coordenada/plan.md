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
| **Una rejilla más fina** | Nada: la celda se calcula en el cliente y la tabla guarda un número | Cambiar la resolución es cambiar una constante; los datos viejos quedan con la suya y el mapa lo dice |
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

### 3. Una tabla de conteos, no de toques — y esta vez se sabe por qué

Gemela de `usage_daily`: `(empresa, día, pantalla, orientación, celda, rol) → veces`.

**No hay entidad «toque»**, y no es una optimización: es lo que hace posible cumplir a la vez FR-003
y FR-012. La 017 ya pagó esta lección —un renglón por evento con su instante se cruza con
`register_sessions.closed_by` y deshace el anonimato entero, y al tope del limitador eran 4 GB en
dos semanas—. Aquí el volumen sería mucho peor: son miles de toques por turno, no decenas de
aperturas.

Con conteos, **las filas las acota la rejilla**: 84 celdas × 5 cortes de rol × las pantallas
instrumentadas. Mil toques en la misma zona suben un contador y no crean nada.

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
└── src/consola/RejillaDeToques.tsx         # la rejilla, CSS propio
```

## Lo que este plan NO construye

- **Pintar sobre una captura.** Prohibido por FR-009 y es la mitad del spec.
- **El recorrido del dedo.** Exige secuencia; la secuencia es tiempo.
- **Instrumentar todas las pantallas.** Se empieza por el POS, que es donde el dedo está todo el
  día. Agregar otra es una línea en la lista, y la 017 dirá cuál.
- **Zoom o rejilla configurable.** La resolución es una constante del código.
