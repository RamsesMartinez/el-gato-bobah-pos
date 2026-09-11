# Tasks: Conteo de efectivo por denominaciones

**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md),
[quickstart.md](./quickstart.md)

**Tests**: obligatorios. El principio IV de la constitución es no negociable y esta feature es
aritmética de dinero: el test se escribe **antes** y se ve fallar por la razón correcta.

## Formato

`- [ ] [ID] [P?] [Story] Descripción con la ruta exacta`

- **[P]**: se puede hacer en paralelo con otras [P] del mismo bloque (archivos distintos, sin
  dependencia entre ellas).
- **[US1] / [US2] / [US3]**: la historia del spec a la que sirve. Setup, Foundational y Polish no
  llevan etiqueta.

## Dos desviaciones del plan, con su razón

1. **Una sola migración, no dos.** El plan proponía `denominaciones` y `conteo_de_efectivo` por
   separado. No se pueden revertir por separado —los renglones del conteo referencian el catálogo—
   así que son una migración partida en dos, con dos `Down` que solo funcionan en un orden. Va una:
   `0066_conteo_de_efectivo.sql`.
2. **El número es 0066.** La más alta hoy es `0065_folio_y_liquidacion_de_plataforma.sql`, ya en
   producción.

---

## Fase 1: Setup

- [X] T001 Confirmar que `server/migrations/` no tiene un `0066` sin mergear en otra rama viva
      (`git branch -a` + `git ls-tree`), antes de tomar el número.

---

## Fase 2: Foundational — el esquema y la aritmética

**Bloquea todo.** Ninguna historia puede empezar antes de que esto esté verde.

### El esquema

- [X] T002 Escribir el test de integración de la migración en
      `server/internal/integration/migracion_conteo_de_efectivo_test.go`, **antes** de la migración
      (`scripts/hooks/migracion-con-test.sh` lo exige, y la 0037 es por qué). Corre contra Postgres
      real, con **al menos dos empresas** —con una sola, todo camino "por cada otra empresa" es un
      no-op— y bajo `appRoleStore` (rol `gatobobah_app`, no el owner: para el owner las políticas no
      existen). Debe cubrir, cada uno fallando con un mensaje que nombre lo que se rompió:
      - las tres tablas existen con sus columnas y tipos;
      - `cash_denominations` tiene `check (value > 0)` y `unique (currency, value)`;
      - la siembra de MXN dejó exactamente las 11 denominaciones esperadas, activas y ordenadas;
      - **RLS aísla**: una empresa no ve los conteos de otra (usar `conexionDeEmpresa()`, no
        `st.Pool.Query` con el ctx del tenant — un test escrito así pasa en verde con RLS borrada);
      - **la FK compuesta rechaza cruzar empresas**: insertar un `session_cash_counts` con el
        `company_id` de A y el `session_id` de B falla;
      - los grants del rol de la app: `select, insert` sí; `update` y `delete` **no**;
      - el rol de la app **no puede escribir** `cash_denominations`;
      - borrar una denominación referenciada por un conteo **falla** (`on delete restrict`), y no se
        lleva en silencio piezas de un arqueo firmado;
      - el `Down` deja el esquema como estaba.
- [X] T003 Escribir `server/migrations/0066_conteo_de_efectivo.sql` con las tres tablas de
      [data-model.md](./data-model.md) hasta que T002 pase: `cash_denominations` (global, con su
      `revoke insert, update, delete … from gatobobah_app`), `session_cash_counts` y
      `session_cash_count_lines` (las dos con `company_id` por default del GUC, `enable row level
      security`, policy `tenant_isolation`, grant sin `update` ni `delete`, y **FK compuestas**).
      Abre con `set local lock_timeout = '3s'`. `Down` gemelo, en orden inverso de FK.
- [X] T004 Correr la migración contra un respaldo anonimizado de producción
      (`make respaldo-anonimo`) y confirmar que aplica y revierte sin tocar datos vivos.

### Las consultas

- [X] T005 Escribir en `server/queries/cash.sql`: `ListDenominations` (por moneda, solo activas,
      ordenadas de mayor a menor), `SaveCashCount`, `SaveCashCountLine`, `GetCashCount` (por sesión y
      momento) y `ListCashCountLines`. **No nombrar `company_id` en el `WHERE`** — sqlc no la conoce
      en las tablas de 0023 y RLS la aplica sola; sí en las tablas nuevas, donde sqlc sí la ve.
      Correr `make sqlc`.

### La aritmética, en `domain`

- [X] T006 [P] Escribir `server/internal/domain/conteo_test.go`, table-driven, **viéndolo fallar**:
      la suma de piezas × valor redondeada a 2; piezas negativas y no enteras rechazadas; un total
      que excede `MaxMoney` rechazado como validación y no como pánico; conteo vacío = $0 y **no**
      es error (una caja puede arrancar vacía); los dos caminos excluyentes (piezas y total juntos →
      error, ninguno de los dos → error, total sin motivo → error).
- [X] T007 Escribir `server/internal/domain/conteo.go` hasta que T006 pase. Sin I/O. Sentinels
      nuevos envueltos con `%w` sobre `domain.ErrValidation`, mapeados en `httpapi.Error` por
      `errors.Is` — no repartir `http.Error` por los handlers.

**Checkpoint**: `make api-build && make api-test` en verde, integración en verde, `make lint` en 0.

---

## Fase 3: User Story 1 — contar el fondo al abrir (P1) 🎯 MVP

**Meta**: abrir la caja capturando piezas; el servidor suma y el operador no escribe ningún total.

**Prueba independiente**: abrir un turno con 6 monedas de $10 y 3 billetes de $50 y verificar que el
fondo registrado es $210, sin que nadie haya escrito "210".

### Backend

- [X] T008 [P] [US1] Escribir `server/internal/integration/conteo_apertura_test.go`, en rojo: abrir
      con piezas deja `register_sessions.opening_cash` en la suma exacta; **el total que mande el
      cliente junto a las piezas se ignora** (mandar piezas por $210 y `openingCash: 999` deja 210);
      abrir por el camino manual exige `manualReason` y lo guarda; mandar los dos caminos a la vez
      se rechaza; abrir sin ninguno de los dos se rechaza, salvo piezas vacías; y **una denominación
      de otra moneda se rechaza** con validación, no se suma (FR-011).
- [X] T009 [US1] Escribir el test que prueba que **la apertura es atómica**, en
      `conteo_apertura_test.go`: si el conteo falla después de crear la sesión, no queda una sesión
      abierta sin conteo y sin motivo. Hoy `OpenSession` no usa `WithTx` porque hace un solo
      `insert`; con el conteo son tres escrituras, y una sesión huérfana además **bloquea la caja**
      por `one_open_session_per_register` hasta arreglarla a mano.
- [X] T010 [US1] Cambiar `BackofficeService.OpenSession` en `server/internal/app/backoffice.go`:
      recibe piezas o total+motivo, valida por `domain`, recalcula el total en el servidor y escribe
      las tres filas dentro de **un solo** `s.store.WithTx`. Valida además que **cada denominación
      sea de la moneda de la sesión** (`register_sessions.currency`): la regla la declara el plan y
      no la hacía cumplir nadie. Un turno en USD que recibe ids de MXN suma un fondo sin significado,
      y el arqueo después cuadra contra una cifra imposible.
- [X] T011 [US1] Handler `GET /cash/denominations` en
      `server/internal/httpapi/handlers_backoffice.go` + su ruta en `router.go`. La moneda es un
      parámetro de frontera: **un valor desconocido se rechaza**, no cae a MXN en silencio.
- [X] T012 [US1] Adaptar el handler de apertura al cuerpo nuevo de
      [contracts/api.md](./contracts/api.md). Handler fino: decodifica, arma el `cmd`, llama al
      servicio, mapea el error.

### Frontend

- [X] T013 [P] [US1] Escribir `web/src/features/backoffice/conteo.test.ts` en rojo: el total en vivo
      = Σ piezas × valor, con los **mismos casos de redondeo** que `conteo_test.go` (si el front y el
      servidor no comparten fixtures, la pantalla puede mostrar un número y el servidor guardar otro).
      Y los bordes del **campo tecleado**, que ahora es el camino principal:
      - **vacío no es cero capturado**: se lee con `parseNumero` de
        [web/src/domain/numeros.ts](../../web/src/domain/numeros.ts), que distingue ausente de cero.
        Borrar el campo para reescribirlo no debe apagar nada ni mandar un renglón;
      - **una entrada inválida no cae a cero en silencio**: `40 piezas`, `1,000` o `4o` se rechazan
        visiblemente. `parseFloat('1,000')` devuelve 1 y ES finito, así que la guarda ingenua no lo
        atrapa — es el caso que ese archivo existe para cerrar;
      - **las piezas son ENTERAS**: `parseNumero` acepta `1.5` como válido y aquí no lo es. Media
        moneda no existe, y aceptarla mete un total que el cajón no puede formar. Ese chequeo es
        nuevo: no lo hace `numeros.ts`, va en `conteo.ts`.
- [X] T014 [US1] Escribir `web/src/features/backoffice/conteo.ts` — puro, sin React.
- [X] T015 [US1] `web/src/api/backoffice.ts`: tipos y llamadas del catálogo y de la apertura con
      piezas.
- [X] T016 [US1] Escribir `web/src/features/backoffice/ContadorDeEfectivo.tsx` como **hoja propia a
      pantalla completa**, no como bloque en el scroll de la caja (ver *La captura es una HOJA
      PROPIA* en el plan: `/caja` mide 1,494 px y el punto de inserción está en y=796). Con:
      - **footer fijo en `dvh`**, como la hoja de la liquidación — el teclado numérico se come
        ~250 px y `CashPage` usa `<Page>` sin `fill`, así que sin footer fijo el total y el botón se
        van debajo del teclado;
      - **el campo del número de piezas es el control principal** de cada denominación —se cuenta
        el montón y se escribe— y el `+` / `−` es **ajuste opcional** al lado, para corregir una
        pieza sin volver a teclear. En ese orden: con tap = +1 como gesto principal, contar 40
        monedas son 40 taps y un cajón típico de SC-003 (menos de 60 piezas) no cabe en los 2
        minutos que ese criterio exige;
      - todo control tappable con `minH="44px"`;
      - el total en vivo, siempre visible.
- [X] T017 [US1] Escribir `web/src/features/backoffice/ContadorDeEfectivo.test.tsx`: el total se
      actualiza en cada cambio; **un cajón completo se captura sin tocar ni una vez `+` / `−`**;
      escribir 40 en el campo no exige 40 taps; una denominación en cero no manda renglón; un campo
      con basura no manda renglón **ni suma cero**, avisa; el interruptor entre contar y capturar el total **advierte antes de descartar**
      lo capturado (FR-015).
- [X] T018 [US1] Cablearlo en la apertura desde `web/src/features/backoffice/CashPage.tsx`. El
      interruptor entre los dos caminos es **Tabs o Switch, nunca `<select>`** (restricción de
      producto) y va **separado físicamente** de la rejilla: un tap accidental pegado a las teclas
      que más se tocan cuesta el conteo entero.

**Checkpoint**: abrir la caja ya no pide sumar a mano. La pantalla de cierre sigue como hoy.

---

## Fase 4: User Story 2 — contar el efectivo al cerrar (P1)

**Meta**: el cierre toma el conteo como efectivo declarado, y la diferencia se ve **antes** de
confirmar.

**Prueba independiente**: cerrar un turno con ventas conocidas capturando piezas que sumen
exactamente lo esperado, y ver la diferencia en $0 sin haber escrito ningún total.

- [X] T019 [P] [US2] Escribir `server/internal/integration/conteo_cierre_test.go` en rojo: el conteo
      alimenta el declarado **del método de efectivo** y solo de ése; los demás métodos siguen
      mandando su cifra; mandar conteo Y declarado para efectivo se rechaza; `difference` —columna
      generada— sale correcta sin tocarla; el conteo se escribe **dentro de la misma transacción**
      que ya usa `CloseSession`, no después.
- [X] T020 [US2] Cambiar `BackofficeService.CloseSession` en `server/internal/app/backoffice.go`
      para recibir el conteo y escribirlo dentro del `WithTx` que ya tiene.
- [X] T021 [US2] Adaptar el handler de cierre al cuerpo de [contracts/api.md](./contracts/api.md).
- [X] T022 [US2] **Construir la diferencia en vivo** en `web/src/features/backoffice/CashPage.tsx`.
      No existe hoy: la tabla del cierre en vivo es Método / Esperado / Declarado, y la única con
      columna de diferencia (`TotalsTable`) se pinta en el diálogo **posterior** al cierre, cuando el
      servidor ya cerró la sesión. Se calcula en el cliente contra lo declarado —incluido el total
      del conteo— y se muestra junto al botón de cerrar, con el mismo peso visual que el aviso de
      "falta por contar". Es FR-005 y hoy no se cumple.
- [X] T023 [US2] Test de esa diferencia en
      `web/src/features/backoffice/CashPage.test.tsx`: con piezas que suman menos de lo esperado, el
      faltante aparece **sin haber tocado Cerrar caja**.
- [X] T024 [US2] Cablear el contador en el cierre desde `CashPage.tsx`, reusando la misma hoja de
      T016.
- [X] T024b [US2] **El segundo cierre no pisa el conteo del primero, y lo dice.** Escribir primero el
      caso en `server/internal/integration/conteo_cierre_test.go`: guardar un conteo de cierre y
      volver a guardarlo devuelve un conflicto con un mensaje accionable, **no** un 500 crudo ni un
      pisado en silencio. Hoy `unique (session_id, moment)` lo rechaza a nivel de base y ese error
      sube como fallo interno. No es hipotético: **las dos tabletas comparten cuenta**, las dos
      pueden abrir el cierre y la segunda confirmar. Traducir el `23505` a `domain.ErrConflict` en
      `server/internal/app/backoffice.go` nombrando que ya hay un conteo guardado, y mostrarlo en
      `CashPage.tsx` con la salida de recargar para ver el que sí quedó.

**Checkpoint**: los dos momentos del turno se cuentan igual, y el faltante se ve antes de firmar.

**Lo que US2 agregó y estas tareas no decían** (se descubrió al implementar, se deja escrito aquí
porque el hueco estaba en el contrato y no en el código):

1. **Declarar el efectivo del cierre a mano exige motivo**, igual que al abrir. FR-014 y FR-016 no
   distinguen entre los dos arqueos del turno; el contrato sí lo hacía, y sin esta regla todo lo que
   la apertura cerraba se reabría por el lado del cierre. Los tests que cerraban con cifras ahora
   pasan por `cierreAMano`, que lleva su motivo de fixture.
2. **`MethodTotal` gana `kind`**, para que la pantalla sepa cuál método es el del cajón sin
   compararlo por nombre. Los métodos de plataforma en efectivo también tocan el cajón: es el mismo
   error que sumó el fondo cuatro veces.
3. **Cerrar sin declarar efectivo no guarda conteo.** Nil no es cero: un conteo en cero afirma "conté
   y estaba vacío", que se lee después como un hecho.

---

## Fase 5: User Story 3 — explicar una diferencia después (P2)

**Meta**: quien revisa un corte al día siguiente ve cuántas piezas de cada denominación se
declararon, o por qué no se contó.

**Prueba independiente**: cerrar con un faltante deliberado, reabrir ese corte y ver el desglose.

- [X] T025 [P] [US3] Escribir el test en `conteo_cierre_test.go`: `SessionDetail` de un corte con
      conteo trae su desglose; **un corte cerrado ANTES de esta feature** trae su total como siempre
      y **no** inventa un desglose que nadie capturó (FR-008 / SC-006); un corte cerrado por el
      camino manual trae su motivo.
- [X] T026 [US3] Agregar el desglose y el motivo a `SessionDetailView` **y a `SessionView`** en
      `server/internal/app/backoffice.go`, y su consulta. Las dos, porque el contrato lo dice de "la
      vista del turno" y el turno **abierto** también necesita mostrar lo que se contó al abrir —
      derivarlo dos veces es de donde salen dos pantallas que no coinciden. `subtotal` viaja
      calculado por la misma razón: si lo multiplica la pantalla, las dos multiplicaciones pueden
      diferir y quien compara contra su cajón no sabe cuál creer.
- [X] T027 [US3] Mostrarlo en el detalle del corte en `CashPage.tsx`: piezas por denominación, o el
      motivo. Un corte viejo no muestra ni una cosa ni la otra, sin explicaciones raras.
- [X] T028 [US3] Test de pantalla del detalle en `CashPage.test.tsx`, incluido el corte viejo.

**Checkpoint**: un faltante deja de ser un número sin historia.

---

## Fase 6: Polish y cierre

- [X] T029 Casos e2e a 1024×600 en `web/e2e/`, contra el ambiente desplegado: la hoja del contador
      cabe sin desplazarse con el teclado abierto; el botón de confirmar sigue visible; los controles
      miden al menos 44 px **medidos después de que la animación asiente** (`boundingBox()` devuelve
      la caja transformada — ver Z3 de [docs/matriz-de-pantallas.md](../../docs/matriz-de-pantallas.md)).
- [X] T030 [P] Agregar los renglones de esta feature a
      [docs/matriz-de-cobro.md](../../docs/matriz-de-cobro.md): por dónde se pierde dinero al contar,
      y qué queda **sin** cubrir (el conteo físico sigue dependiendo de quien cuenta).
- [X] T031 [P] Agregar a [docs/matriz-de-pantallas.md](../../docs/matriz-de-pantallas.md) lo medido
      de la hoja del contador, y actualizar
      [docs/presupuesto-de-pantalla-1024x600.md](../../docs/presupuesto-de-pantalla-1024x600.md) con
      el alto real de `/caja`.
- [X] T032 Correr [quickstart.md](./quickstart.md) completo, incluidos los tres casos que agregó la
      revisión: la hoja propia, las 40 monedas y la diferencia antes de cerrar.
      **Cómo se corrió, y en qué se desvió de la tarea**: los pasos se automatizaron como C5, C6 y
      C7 de `contar-el-cajon.spec.ts` en vez de recorrerlos a mano, y se corrieron contra el **stack
      completo en local** (API Go + Postgres real, sin mocks) y no contra el desplegado. Automatizado
      vale más que un recorrido manual —se hace una vez, esto corre en cada suite— y en local se
      itera sin esperar un deploy por cada corrección. Queda pendiente **repetirlos en dev tras el
      deploy**, que es lo que la tarea pedía. Lo que sigue siendo manual y no se automatizó: el
      juicio visual de si la pantalla "se ve bien", que no lo decide un assert.
- [X] T033 Gates completos: `make api-build`, `make api-test`, integración con
      `-tags=integration`, `make lint`, `make vuln`, `bun run lint`, `bun run vitest run`,
      `bun run build`, `bun audit --audit-level=high`.

---

## Dependencias

- **Fase 2 bloquea todo.** Sin esquema y sin aritmética no hay nada que cablear.
- **US1 → US2**: el cierre reusa la hoja y el servicio de la apertura. Se puede empezar US2 en
  cuanto T016 exista, pero no antes.
- **US3 depende de US2**: no hay desglose que mostrar hasta que el cierre lo guarde.
- Dentro de cada historia: **el test primero, y visto en rojo por la razón correcta.**

## Paralelo posible

- T006 y T013 (la aritmética en Go y en TypeScript) son archivos distintos y comparten solo las
  fixtures: se pueden escribir a la vez.
- T030 y T031 son documentos distintos.
- Todo lo demás dentro de una historia es secuencial: toca los mismos archivos.

## Estrategia de entrega

1. Fase 2 → esquema y aritmética listos, sin cambio visible.
2. **US1 → se acabó la libreta al abrir.** Entregable por sí solo; el cierre sigue como hoy.
3. US2 → el cierre, que es donde el error cuesta dinero.
4. US3 → el desglose que hace investigable un faltante.

El MVP es la Fase 2 más US1.
