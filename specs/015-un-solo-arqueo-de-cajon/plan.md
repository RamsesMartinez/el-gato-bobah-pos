# Implementation Plan: Un solo arqueo de cajón, y qué entra a él se configura

**Branch**: `015-un-solo-arqueo-de-cajon` | **Date**: 2026-09-10 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/015-un-solo-arqueo-de-cajon/spec.md`

## Summary

El cajón se arquea **una vez contra una cifra**: fondo + todo el efectivo cobrado que llega a ese
cajón + entradas − salidas. El cierre deja de pedir una cifra por cada método de efectivo, la
diferencia del arqueo pasa a ser un hecho propio de la fila del conteo, y los dos interruptores que
deciden qué entra al cajón —si un método está activo y si su efectivo llega— se vuelven operables
desde Ajustes del negocio. El arqueo ciego se agrega como interruptor del negocio y enmienda FR-005
de la 003 por escrito.

El detalle de las decisiones está en [research.md](./research.md); el modelo, en
[data-model.md](./data-model.md); los contratos, en [contracts/api.md](./contracts/api.md).

## Technical Context

**Language/Version**: Go 1.27 (backend) · TypeScript 5 con React 19 (front)

**Primary Dependencies**: chi · pgx + sqlc · goose (migraciones embebidas) · Chakra UI v3 · TanStack Query

**Storage**: PostgreSQL con RLS por empresa. Dinero en `numeric`, redondeado en cada frontera con `domain.Round2`

**Testing**: unitarios en `domain` (table-driven) · integración contra Postgres real con `-tags=integration`, varios bajo el rol `gatobobah_app` · vitest en el front · Playwright a 1024×600 contra el stack completo

**Target Platform**: tabletas de 7 a 10 pulgadas, presupuesto real ~1024×600, táctil

**Project Type**: monorepo web (`server/` + `web/`)

**Performance Goals**: sin objetivos nuevos. El esperado del cajón sale de la misma consulta que ya corre al abrir la pantalla de caja

**Constraints**: datos reales de un negocio en operación — los cortes ya cerrados no pueden cambiar de cifra (SC-005). El alto de `/caja` ya está agotado: 1,494 px contra un viewport de 600

**Scale/Scope**: 10 métodos de pago configurados, 3 cajas, un local. Tres historias, dos tablas tocadas, una migración

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo lo cumple este plan |
|---|---|
| **I. Layering estricto** | El cálculo del esperado del cajón y la resolución de qué métodos entran es **lógica pura en `domain`** (`EsperadoDelCajon`, `MetodosDelCajon`). `app` orquesta y persiste; `httpapi` decodifica y mapea. Nada de SQL fuera de sqlc |
| **II. Errores envueltos** | Los rechazos nuevos son sentinels de `domain` envueltos con `%w`; el mapeo a HTTP sigue viviendo solo en `httpapi.Error` |
| **III. Dinero clasificado una sola vez** | **Es el principio que este spec viene a hacer cumplir.** Hoy el mismo peso se declara dos veces (en el conteo del cajón y en la cifra tecleada de la app). La diferencia única del cajón, en una columna generada, es lo que impide que dos restas del mismo dinero difieran |
| **IV. Test-first** | Cada historia arranca por su test. La migración lleva el suyo **antes** y corre con dos empresas. Los bordes de esta feature están enumerados abajo, antes de escribir código |
| **V. Seguridad adversarial** | Los dos interruptores nuevos son configuración con impacto directo en la reconciliación: exigen rol de administración y dejan evento de seguridad, como ya lo hace `SetPaymentMethodAutoDeclare`. Un método de cajón que llegue en `declared` se **rechaza**, no se ignora en silencio |
| **VI. YAGNI** | Cero conceptos nuevos: los dos interruptores ya existen como columnas, el arqueo ciego es un booleano en una tabla que ya es la casa de los ajustes. No se construye conteo por sobres ni restricción por rol de quién ve la diferencia |
| **VII. Comentarios del porqué** | Cada decisión no obvia —por qué la diferencia vive en el conteo, por qué el filtro de método activo cambia— va comentada donde vive el código |
| **VIII. No cerrar puertas** | Ver abajo |

### La pregunta del principio VIII, por escrito

> ¿Esto se puede agregar después al mismo costo?

- **El arqueo ciego**: sí. Es un booleano y una pantalla que oculta columnas. Se agrega o se quita sin
  tocar un dato. Gana el VI, se construye porque el dueño lo pidió.
- **Los dos interruptores**: sí, y de hecho las columnas ya existen. Lo que se agrega es quién las
  escribe.
- **El esperado del cajón guardado en la fila del conteo**: **no**. Un corte que se cierra sin
  guardar contra qué se comparó su conteo no se puede reconstruir después: el esperado se calcula de
  `order_payments`, y una venta cancelada o reembolsada más tarde lo mueve. Es un hecho del momento
  del cierre, como `order_lines.unit_price`, y por eso se guarda en vez de recalcularse.
- **Guardar en el renglón del corte SI ese método tocaba el cajón**: **no**, y esto lo encontró la
  revisión de arquitectura. Hoy `affects_cash_drawer` se reconstruye en vivo por join, y es inofensivo
  porque nadie puede cambiarlo; FR-007 es justo lo que le da un `PATCH` por primera vez. El día que
  el dueño apague «el efectivo de Didi llega al cajón», todo corte cerrado antes se reagrupa con el
  flag de hoy: las cifras no cambian —FR-008 se cumple en el número— pero la **forma del reporte
  histórico** sí. Es el mismo defecto que la constitución ya nombró para la categoría de un producto:
  recategorizar reescribe el pasado. Se guarda el flag en `register_session_totals`, poblado en el
  insert que ya corre.

**Esas dos son las piezas irreversibles del plan** y son la razón de que la migración vaya en la
primera fase, antes de cualquier pantalla.

Ninguna puerta de la tabla del principio VIII se cierra. Dos se acercan y conviene decirlo:

- **«Más de una caja vendiendo a la vez»**: el arqueo del cajón es por turno y por caja, como hoy. El
  esperado se ata a `register_session_id` y no a una ventana de tiempo, que es lo que ya hace
  correcta la consulta del esperado por construcción.
- **«Cuánto deja cada plataforma»** (cruzada por la 014): el efectivo que un repartidor de la app se
  lleva sale del cajón y entra a la conciliación de la liquidación. Este plan no cambia esa
  conciliación; solo deja de contarlo dos veces.

## «¿Un atacante lo evade?», contestado

El principio V no deja mergear un control de seguridad sin responder esta pregunta con un caso
concreto. El arqueo ciego es un control, y la respuesta es **sí, por dos vías**. Las dos se
encontraron en `/speckit-analyze`:

**1. Por la pantalla de Ventas, si quien cierra es gerente o administrador.** `/sales` y
`/sales/summary` están abiertos a esos dos roles y muestran la venta en efectivo del día: el
esperado se deriva de ahí antes de contar. **Se acepta**, y queda escrito en los supuestos del spec:
el control apunta a quien está en el mostrador —rol cajero, que no alcanza Ventas—, y cerrar esa vía
exigiría bloquear una pantalla legítima en su uso normal para una protección que seguiría siendo
porosa (los datos de ayer estiman los de hoy). Lo que de verdad lo cerraría es que el que cuenta no
sea el que ve, y eso está nombrado como puerta abierta en el Out of Scope del spec: no se construye
y no se cierra.

**2. Por el detalle del turno abierto, y esta sí se cierra.** `GET /cash-sessions/{id}` está abierto
a **cajero** y acepta el id del turno abierto, así que nulificar el esperado solo en `/current`
dejaría la cifra a un request de distancia. El nulificado va en **los dos** endpoints mientras el
turno esté abierto, y el test de T021 los nombra a los dos.

## Los bordes, enumerados antes de escribir código

La constitución pide pensarlos primero. Estos son los que ya tienen dueño en las tareas:

1. **El valor vacío que significa algo**: un método de cajón que llega en `declared` con `0` no es
   "declaró cero", es un cliente viejo mandando el cuerpo de antes. Se rechaza nombrando el método.
2. **El estado que no sobrevive**: el interruptor del arqueo ciego se lee al abrir la pantalla. Si
   alguien lo cambia mientras otro tiene el cierre a medio capturar, el segundo no puede pasar de ver
   la diferencia a no verla — el cambio aplica al siguiente cierre y la pantalla lo dice.
3. **El camino nuevo que se salta el control viejo**: `PATCH /payment-methods/{id}` gana dos campos.
   El evento de seguridad y el rol ya están en ese handler para `autoDeclare`; los dos campos nuevos
   tienen que pasar por lo mismo y no por una rama nueva sin control.
4. **El hermano que no se movió**: `faltanPorContar` en el front y el `where pm.is_active` del
   esperado son dos lugares que hoy asumen "todo método no-auto se teclea" y "solo los activos
   cuentan". Los dos cambian, y hay que buscar a todos los que llamaban a lo viejo — incluido el
   detalle del corte, que pinta la misma tabla.
5. **El cajón sin nada que contar**: una caja secundaria sin efectivo no tiene arqueo de cajón y el
   cierre no debe exigirlo. Ya resuelto en la 003 (sin efectivo declarado no se guarda conteo) y hay
   que no romperlo.
6. **El corte viejo**: un corte cerrado antes de esta feature tiene renglones por método con su
   `declared` tecleado y **sin** fila de conteo con esperado. La pantalla tiene que leerlo como
   siempre; el histórico, sumar su diferencia como siempre.
7. **El arqueo ciego apaga la guardia del cierre** (lo encontró la revisión de pantalla, y es el
   borde más grave de los siete). Si `expected` viaja en null, `Number(null) === 0` y
   [faltanPorContar](../../web/src/features/backoffice/cierreDeCaja.ts) concluye que **ningún** método
   esperaba dinero: devuelve lista vacía, `porContar.length > 0` deja de deshabilitar el botón y el
   operador puede firmar el cierre sin capturar nada. Es el faltante inventado de $1,662 por la
   puerta de atrás. La pantalla necesita saber qué falta por capturar **por un camino que no pase por
   `expected`**: el servidor dice qué métodos exigen captura, en vez de que el cliente lo deduzca de
   si la cifra es cero.
8. **Un `PATCH` con `bool` pelado apaga lo que nadie pidió.** El handler de hoy usa
   `AutoDeclare bool`, donde un campo ausente y un `false` explícito son indistinguibles. Copiado tal
   cual para tres interruptores, un `PATCH {"isActive": false}` resetea `affectsCashDrawer` a falso y
   saca del arqueo el dinero de un método. Los tres campos van como `*bool`.
9. **El interruptor que más daño hace es el que menos se nota.** «Activo» saca un método del cobro al
   instante; los otros dos son configuración de reconciliación. Tres interruptores idénticos y
   pegados en la misma fila hacen que un dedo que falla por milímetros desactive un método a media
   jornada.

## Project Structure

### Documentation (this feature)

```text
specs/015-un-solo-arqueo-de-cajon/
├── plan.md              # Este archivo
├── research.md          # Las seis decisiones, con lo que se midió
├── data-model.md        # Las dos tablas que cambian y cómo se leen
├── quickstart.md        # Cómo comprobar que hace lo que dice
├── contracts/api.md     # Lo que cambia en la frontera HTTP
├── checklists/          # requirements.md (de /speckit-specify)
└── tasks.md             # Lo genera /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── migrations/
│   └── 0067_arqueo_del_cajon.sql        # expected + difference en el conteo; blind_cash_count
├── queries/
│   ├── cash.sql                          # el esperado incluye inactivos con pagos; guardar el esperado del conteo
│   └── settings.sql                      # el interruptor del arqueo ciego
├── internal/
│   ├── domain/
│   │   └── cajon.go                      # EsperadoDelCajon, MetodosDelCajon (puro, sin I/O)
│   ├── app/
│   │   └── backoffice.go                 # CloseSession arquea el cajón una vez; los dos interruptores
│   └── httpapi/
│       ├── handlers_backoffice.go        # PATCH de método; el cuerpo del cierre rechaza métodos de cajón
│       └── handlers_settings.go          # el interruptor en los ajustes
└── internal/integration/
    ├── migracion_arqueo_del_cajon_test.go
    ├── arqueo_del_cajon_test.go          # US1
    ├── metodos_configurables_test.go     # US2
    └── arqueo_ciego_test.go              # US3

web/src/
├── api/backoffice.ts                     # tipos del arqueo del cajón y de los interruptores
├── features/backoffice/
│   ├── cierreDeCaja.ts                   # faltanPorContar y las diferencias, con el cajón como uno
│   └── CashPage.tsx                      # la tabla del cierre: un renglón de cajón, sin campos de más
└── features/admin/
    └── BusinessSettingsPage.tsx          # los dos interruptores por método + el arqueo ciego
```

**Structure Decision**: monorepo existente, sin directorios nuevos. La lógica pura entra en un
archivo propio de `domain` (`cajon.go`) porque es aritmética de dinero con varios bordes y merece sus
tests sin base de datos; el resto son cambios en archivos que ya existen.

## Lo que cambia en la pantalla del cierre

**Corregido tras la revisión de pantalla.** La primera versión de esta sección afirmaba que la tabla
se acorta, y lo contaba mal: contaba los campos de captura que se quitan y no los **renglones**.
Contando renglones, la tabla **crece**: los cuatro métodos cuyo efectivo llega al cajón siguen
siendo renglones —el corte informa por canal— y se suma **uno nuevo** para el cajón.

| | Hoy | Después |
|---|---|---|
| Renglones de método | 10 | 10 |
| Renglón de cajón | — | 1 |
| Campos de captura | 7 | 4 |
| Columna «Esperado» con arqueo ciego | visible | no se pinta |

Lo que se ahorra en campos no compensa por sí solo un renglón nuevo con su botón, y esta pantalla ya
mide 1,494 px contra un viewport de 600: **se mide con Playwright antes de darlo por bueno**, no se
supone. La tarea de medición está en el Polish y su resultado puede obligar a recortar.

**El tercer estado de la columna «Declarado», que faltaba.** Hoy esa columna solo sabe decir dos
cosas: «Automático» (texto mudo) o traer un control. Los métodos que se cuentan dentro del cajón son
un tercer estado y **no pueden reusar «Automático»**: un método auto-declarado lo resuelve el
servidor, y uno de cajón se cuenta físicamente. Confundirlos es la clase de ambigüedad que ya costó
$4,500. El texto es **«Va en el cajón»**, en gris y sin control.

**Y el botón de contar cambia de renglón.** Hoy vive en la fila «Efectivo»; pasa al renglón del
cajón, que es de quien es el conteo.

## Lo que cambia en Ajustes del negocio

**Corregido tras la revisión de pantalla.** La primera versión decía "a 1024 px de ancho caben en la
fila". Es falso:
[BusinessSettingsPage](../../web/src/features/admin/BusinessSettingsPage.tsx) usa
`<Page maxW="560px">`, así que con el padding el ancho útil de cada renglón es **~520 px**. Diseñar
contra 1024 ahí es el mismo error que diseñar contra el monitor de quien programa, al revés.

- **La lista de métodos pasa a ser una tabla**, como ya hace `ManageRegistersTab` en el mismo repo:
  columnas **Método / Activo / Va al cajón / Automático**. Las etiquetas viven una sola vez en el
  encabezado en vez de repetirse diez veces al lado de cada interruptor, que es lo único que cabe en
  520 px.
- **Los tres interruptores necesitan tamaño explícito.** Dato del repo que esta revisión sacó:
  **ninguno de los 19 `<Switch>` del front fija `size` ni `minH`**, así que el control táctil ya está
  por debajo de los 44 px que la constitución exige — en todas las pantallas que los usan. Aquí se
  fija `size="lg"` y la celda de la tabla da la separación.
- **«Activo» va en su propia columna, separada de las otras dos.** Es el único de los tres con efecto
  inmediato en el mostrador: apagarlo saca el método del cobro. Los otros dos son configuración de
  reconciliación. Pegados e idénticos, un dedo que falla desactiva un método a media jornada.
- **La copia del interruptor del cajón sale del spec, no del nombre de la columna**: «Va al cajón» en
  el encabezado, y el detalle —«su efectivo entra al cajón»— en la ayuda del bloque. «Afecta el
  cajón» no le dice al operador ni la dirección ni la consecuencia.

Falta una **línea base**: nadie ha medido cuántos de los diez renglones de «Corte de caja» se ven
hoy sin desplazarse, y sin ese *antes* la comparación que pide el quickstart no se puede hacer. Es
una tarea de medición, no una suposición.

## Complexity Tracking

> Sin violaciones que justificar. El plan no agrega proyectos, ni capas, ni dependencias, ni
> abstracciones: usa dos columnas que ya existen, agrega dos a una tabla que todavía no llega a
> producción y un booleano a la tabla de ajustes.
