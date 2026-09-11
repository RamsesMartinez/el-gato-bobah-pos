# Implementation Plan: Conteo de efectivo por denominaciones

**Branch**: `003-arqueo-denominaciones` | **Date**: 2026-09-01 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-arqueo-denominaciones/spec.md`

## Summary

El operador cuenta el cajón a mano y captura un solo número; la suma la hace en una libreta o en la
calculadora del teléfono. Ese paso es la fuente de los faltantes: un billete mal sumado se vuelve una
diferencia que después nadie puede explicar, como el turno con $1,662 declarado en cero.

El enfoque:

1. **Se capturan PIEZAS, el servidor suma.** El total sigue llegando a las mismas columnas de
   siempre, así que el corte, los reportes y la columna generada `difference` no se tocan.
2. **El desglose se guarda**, y es lo que convierte un faltante en algo investigable: distinguir
   "faltan dos billetes de $500" de "falta dinero".
3. **Dos caminos excluyentes**: contar, o escribir el total con un motivo obligatorio. Nunca los dos,
   así que nunca hay dos cifras del mismo dinero compitiendo.

## Technical Context

**Language/Version**: Go 1.27 (backend), TypeScript / React 19 (frontend)

**Primary Dependencies**: chi, pgx + sqlc, goose, shopspring/decimal (backend); Chakra UI v3,
TanStack Query (frontend)

**Storage**: PostgreSQL con RLS por empresa. Tres tablas nuevas; ninguna columna nueva en las de hoy.

**Testing**: `go test` (unitarios en `domain` para la aritmética, integración contra Postgres real
para el arqueo), vitest

**Target Platform**: tabletas de 7 a 10 pulgadas, presupuesto real ~1024×600, táctil

**Project Type**: aplicación web (backend Go + frontend React), monorepo

**Performance Goals**: contar y capturar un cajón típico (menos de 60 piezas) en menos de 2 minutos
(SC-003). El total se actualiza en cada cambio, sin viaje al servidor.

**Constraints**: controles de al menos 44 px; sin `<select>` nativo. La pantalla de cierre YA tiene
dos secciones que compiten por el alto —los pedidos sin entregar y el desglose por cajero— así que el
conteo no puede ser una lista de once renglones apilados.

**Scale/Scope**: 11 denominaciones en MXN, 3 cajas por negocio, 2 o 3 turnos por día. Una pantalla
nueva de captura reutilizada en apertura y cierre; un endpoint nuevo, dos que cambian.

## Constitution Check

*GATE: revisado antes de Fase 0 y de nuevo tras el diseño de Fase 1.*

| Principio | Cómo lo cumple este plan |
| --- | --- |
| **I. Layering estricto** | La aritmética del conteo va a `domain` como función pura. El servicio orquesta y decide los dos caminos; el handler decodifica y mapea. SQL solo por sqlc |
| **II. Errores envueltos** | Sentinels nuevos para "conteo y total a la vez" y "falta el motivo", envueltos con `%w` sobre `ErrValidation`. El mapeo a HTTP sigue viviendo solo en `httpapi.Error` |
| **III. Dinero** | Es el corazón. `Round2` al calcular el total y `ValidMoney` antes de tocar el `numeric`. El servidor recalcula desde las piezas y **ignora** cualquier total que mande el cliente, igual que `BuildOrder` con los precios |
| **IV. Test-first, lógica en `domain`** | La suma piezas × valor es pura y se prueba sin base. El arqueo completo —apertura, cierre, diferencia contra el esperado— es de integración: el error aparece al combinar filas, no en una función aislada |
| **V. Seguridad adversarial** | Superficie chica pero real. Detalle abajo |
| **VI. YAGNI** | Sin conteos parciales a media jornada, sin doble verificación, sin sembrar monedas que no se usan. El catálogo es por moneda porque la columna `currency` **ya existe**, no por si acaso |
| **VII. Comentarios del porqué** | Las dos decisiones no obvias —por qué el total se guarda aunque sea derivable, y por qué las piezas en cero no generan renglón— van como comentario donde viven |

### Principio V, en detalle

| Pregunta adversarial | Respuesta concreta |
| --- | --- |
| ¿Puedo inflar el fondo mandando un total falso? | No. Con `counts` presentes el servidor recalcula y **descarta** cualquier total del cliente |
| ¿Puedo contar denominaciones de otra empresa? | El catálogo **no es per-tenant** —los billetes de México son los mismos para todos— así que no hay nada que aislar. Lo que sí se valida es que la denominación sea de la MONEDA de la sesión |
| ¿Puedo desbordar el `numeric(10,2)` con muchas piezas? | No. El total pasa por `ValidMoney` antes de escribirse, y sale como 400 y no como 500 |
| ¿Puedo mandar piezas negativas para restar del cajón? | No. `check (pieces > 0)` en la base y validación en el dominio |
| ¿Puedo declarar sin dejar rastro de por qué? | No. FR-016: o hay conteo, o hay motivo. El servicio lo exige antes de escribir |
| ¿Puedo escribir el conteo de la sesión de otra empresa? | No. `session_cash_counts` cuelga de `register_sessions`, que ya está bajo RLS |

**Sin violaciones que justificar.**

## Project Structure

### Documentation (this feature)

```text
specs/003-arqueo-denominaciones/
├── plan.md              # Este archivo
├── research.md          # Fase 0 — lo medido sobre el código y producción
├── data-model.md        # Fase 1 — tres tablas, y qué NO cambia
├── quickstart.md        # Fase 1 — un recorrido por historia
├── contracts/api.md     # Fase 1 — un endpoint nuevo, tres que cambian
├── checklists/
└── tasks.md             # Lo crea /speckit-tasks
```

### Source Code

```text
server/
├── migrations/
│   ├── 00NN_denominaciones.sql          # catálogo + siembra de MXN
│   └── 00NN_conteo_de_efectivo.sql      # conteo y sus renglones
├── queries/cash.sql                      # catálogo, guardar conteo, leerlo con el turno
├── internal/
│   ├── domain/
│   │   ├── conteo.go                     # suma piezas × valor, y las dos reglas excluyentes
│   │   └── conteo_test.go
│   ├── app/backoffice.go                 # OpenSession y CloseSession toman piezas, en UNA transacción
│   ├── httpapi/handlers_backoffice.go
│   └── integration/
│       ├── conteo_apertura_test.go
│       └── conteo_cierre_test.go

web/src/
├── features/backoffice/
│   ├── ContadorDeEfectivo.tsx            # la rejilla, compartida por apertura y cierre
│   ├── conteo.ts                         # el total en vivo, puro
│   ├── conteo.test.ts
│   └── CashPage.tsx                      # la usa en los dos momentos
└── api/backoffice.ts
```

**Structure Decision**: monorepo existente, sin carpetas nuevas. El contador vive en `backoffice/`
junto a la pantalla de caja que lo usa dos veces.

**El número de las migraciones se asigna al implementar**, no ahora: el spec 004 también agrega y la
que aterrice segunda toma el siguiente libre.

## Fases

### Fase 0 — Investigación ✅

Ver [research.md](./research.md). Nueve hallazgos. Los que más pesan:

- **El total debe seguir llegando a las columnas de hoy** (`opening_cash`, `declared`), o habría dos
  cifras del mismo dinero. `difference` es columna generada y se recalcula sola.
- **La pantalla de cierre ya tiene dos secciones nuevas** compitiendo por el alto, así que el conteo
  no puede apilar once renglones.

### Fase 1 — Diseño ✅

- [data-model.md](./data-model.md) — tres tablas, cero columnas nuevas en las de hoy
- [contracts/api.md](./contracts/api.md) — un endpoint nuevo, tres que cambian
- [quickstart.md](./quickstart.md) — un recorrido por historia, con el fallo esperado de cada uno

### Fase 2 — Tareas

La genera `/speckit-tasks`. El orden que sugiere el diseño:

1. **El catálogo** (tablas, siembra de MXN, endpoint). No cambia comportamiento por sí solo.
2. **La aritmética en `domain`**, con sus tests. Es donde vive el valor y no toca nada más.
3. **US1, la apertura.** Es la mitad simple —una cifra, sin esperado contra el cual comparar— y ya
   entrega el valor completo: se acabó la libreta.
4. **US2, el cierre.** Donde el error cuesta dinero.
5. **US3, ver el desglose después.** Lo que convierte un faltante en algo investigable.

## Riesgos, y qué hace el plan con cada uno

| Riesgo | Qué haría | Cómo se cierra |
| --- | --- | --- |
| El total se recalcula desde el catálogo al consultarlo | Cambiar el valor de una denominación reescribiría arqueos ya firmados | El total se **guarda**. Es la decisión menos obvia del modelo y va comentada |
| El conteo empuja el resumen fuera de pantalla | El operador no ve la diferencia antes de cerrar, que es el punto | Rejilla compacta, no lista. El quickstart lo verifica en 1024×600 con las otras dos secciones presentes |
| Once campos de texto | Once oportunidades de dedazo, que es el error que la feature viene a quitar | Se reusa el gesto de los botones de billete del cobro, que el operador ya conoce |
| Un total del cliente que el servidor acepte | Vuelve la suma manual, disfrazada | El servidor recalcula desde las piezas y descarta el total, con test |
| Los dos caminos coexisten | Dos cifras del mismo dinero y nadie sabe cuál manda | Se rechaza en el servicio, no solo en la pantalla |

## La captura es una HOJA PROPIA, no un bloque más en el scroll de la caja

Decidido el 2026-09-08, tras medir. Es la restricción de diseño que manda sobre todo lo demás de
esta feature, y estaba implícita en el plan original — que es como no estar.

**Lo medido, en el escenario más vacío posible** (turno con $0 en todo, sin pedidos pendientes, sin
"sin cobrar", un solo cajero) a 1024×600:

| | |
|---|---|
| Alto total de `/caja` | **1,494 px** contra un viewport de 600 |
| Encabezado + tabs + chips + resumen del corte | **572 px** — deja 28 px de los 600 |
| Título "Cierre — declarado por método" | y = **727** |
| El renglón "Efectivo", donde iría la rejilla | y = **796** a 849 |
| Botón "Cerrar caja" | y = **1,426** |

Con cualquier dato real esos números solo empeoran. **Inline es imposible**: el punto de inserción
ya está a 200 px del fold antes de dibujar el primer renglón, así que FR-013 no se cumple por más
compacta que sea la rejilla. No es un problema de la rejilla; es que ese lugar de la pantalla ya no
existe.

En consecuencia:

- `ContadorDeEfectivo` **abre como hoja propia** y ocupa los 1024×600 completos, como
  `TransferDialog` ya hace en esa misma pantalla. El operador toca "Contar efectivo", cuenta sin
  desplazarse, confirma, y vuelve al cierre con la cifra puesta. Hoy, llegar al campo de efectivo
  cuesta dos o tres gestos de scroll.
- **Footer fijo en `dvh`**, el patrón de la hoja de la liquidación
  ([docs/presupuesto-de-pantalla-1024x600.md](../../docs/presupuesto-de-pantalla-1024x600.md)). El
  teclado numérico se come ~250 px al abrirse y `CashPage` usa `<Page>` **sin `fill`**: es scroll de
  documento plano, así que sin footer fijo el total y el botón de confirmar se van debajo del
  teclado justo cuando se está tecleando.

## FR-005 no tiene hoy dónde vivir, y el plan lo daba por resuelto

El plan decía que la diferencia se ve porque "el resumen del corte queda visible". Medido: el
resumen que queda visible es `IngresosEgresosCard` —monto inicial, ingresos, egresos— y **no tiene
columna de diferencia**. La tabla que sí la tiene (`TotalsTable`) se pinta en dos lugares y ninguno
sirve: el detalle de un corte histórico, y **el diálogo que aparece DESPUÉS de cerrar**, cuando el
servidor ya cerró la sesión. La tabla del cierre en vivo tiene Método, Esperado y Declarado, y ahí
se acaba.

Así que FR-005 —"mostrar la diferencia ANTES de confirmar"— exige construir algo que no existe: una
diferencia **calculada en el cliente** contra lo declarado, incluido el total del conteo, visible
junto al botón de cerrar y con el mismo peso visual que el aviso de "falta por contar". Es trabajo
del alcance de esta feature, no un efecto secundario de tenerla.

## Cómo se cuentan 40 monedas

`research.md` §8 eligió "tocar una denominación y ver el total subir" y rechazó once campos de texto
por el riesgo de dedazo. Le falta el caso de bulto: **si tocar es +1, contar 40 monedas de $1 son 40
taps**, y eso está dentro del tope de SC-003 (menos de 60 piezas en menos de 2 minutos). Un campo
numérico son cuatro.

El contador hace las dos cosas, pero **no en el mismo orden que decía este plan** (corregido el
2026-09-09, decisión del dueño): el camino principal es **teclear el número de piezas en el campo de
su denominación**, porque es como se cuenta en la operación diaria — se agarra el montón de monedas
de $1, se cuentan, y se escribe. El `+`/`−` queda como **ajuste opcional** para corregir una pieza
de más o de menos sin volver a teclear, y por eso son dos controles chicos al lado del campo, no la
tecla grande de la que cuelga todo.

Por qué importa el orden y no es cosmética: si el `+` es el gesto principal, el campo se diseña como
escape y termina angosto y apretado entre las dos teclas; con el campo como principal, es él el que
se lleva el ancho y el alto de toque, y las teclas se acomodan alrededor. La vara de UX del POS es
minimizar taps, y 40 taps para una denominación la rompe.

## El interruptor entre los dos caminos

FR-014 y FR-015 exigen elegir entre contar y escribir el total, y advertir antes de descartar lo
capturado. Ningún documento decía con qué control. Queda escrito: **Tabs o Switch, nunca
`<select>`** (restricción de producto), y **separado físicamente de la rejilla de teclas**. Un tap
accidental sobre un interruptor pegado a las teclas que más se tocan cuesta el conteo entero, y la
confirmación de FR-015 no lo arregla: la arregla no poner el interruptor ahí.

## Revisión de arquitectura del 2026-09-08

El plan es del 1 de septiembre y se escribió contra la constitución **1.7.0**. Al retomarse se pasó
otra vez por `/revision-de-arquitectura` con la **1.9.0**, que ganó dos restricciones que este plan
no pudo considerar: que se diseña para varias empresas y cadenas, y que la conexión es requisito.
Lo que cambió, y por qué se deja escrito aquí en vez de corregirlo en silencio:

| Hallazgo | Qué se corrigió |
|---|---|
| **Las dos tablas de negocio nacían sin `company_id`** y por lo tanto sin poder llevar RLS | `company_id` con default del GUC, policy `tenant_isolation` y grant explícito en `session_cash_counts` y `session_cash_count_lines`. Es el patrón de toda tabla de negocio desde `0024`, y omitirlo reproducía en tablas frescas la fuga que `0040`, `0041` y `0061` cerraron a posteriori |
| **FK simple hacia `register_sessions`** | FK **compuesta** `(company_id, session_id)`, y `unique (id, company_id)` en el conteo para que los renglones cuelguen igual. Los chequeos de integridad de Postgres saltan RLS: con la FK simple, un data-fix como owner podía colgar un conteo de una empresa del turno de otra |
| **`denomination_id` sin `ON DELETE` elegido** | `on delete restrict` explícito. Heredarlo por omisión invita a copiar el `cascade` del renglón de arriba, y eso se llevaría piezas de arqueos ya firmados |
| **`cash_denominations` global y escribible por la app** | Sigue global —partirla después cuesta lo mismo, como `payment_methods` en `0037`— pero se le **revoca escritura** al rol de la app: sin `company_id` no hay nada que impida que apagar el billete de $1000 lo apague para todas las empresas |
| **`OpenSession` no usa `WithTx`** | Hoy hace un solo `insert` y no lo necesita; con el conteo pasa a ser tres escrituras. Sin transacción, un fallo después del primer `insert` deja la caja **abierta sin conteo y sin motivo** —lo que FR-016 y SC-007 prohíben— y bloqueada por `one_open_session_per_register` hasta arreglarla a mano |

Lo que el revisor dio por bueno: la reversibilidad del `Down` (tres tablas nuevas, cero columnas en
tablas vivas), el trato de los arqueos ya cerrados, guardar el `total` en vez de recalcularlo, y los
tipos de dinero. Ninguna puerta del principio VIII se cierra: el conteo cuelga de `session_id`, así
que agregar sucursal o una segunda caja que venda no obliga a tocar estas tablas.

## Complexity Tracking

Sin violaciones a la constitución. Nada que justificar.
