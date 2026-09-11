---

description: "Tareas de la feature 015 — un solo arqueo de cajón"
---

# Tasks: Un solo arqueo de cajón, y qué entra a él se configura

**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md),
[quickstart.md](./quickstart.md)

**Tests**: obligatorios. El principio IV es no negociable y esta feature reparte dinero entre
métodos: el test se escribe **antes** y se ve fallar por la razón correcta.

## Formato

`- [ ] [ID] [P?] [Story?] Descripción con la ruta exacta`

- **[P]**: se puede hacer en paralelo con otras [P] del mismo bloque (archivos distintos, sin
  dependencia entre ellas).
- **[US1] / [US2] / [US3]**: la historia del spec a la que sirve. Setup, Foundational y Polish no
  llevan etiqueta.

## Lo que la revisión de arquitectura cambió antes de escribir estas tareas

Cuatro hallazgos entraron al plan y por eso aparecen aquí como tareas y no como sorpresas:

1. **El snapshot de `affects_cash_drawer`** en el renglón del corte (T003, T009). Sin él, apagar el
   interruptor reagrupa cortes ya cerrados.
2. **Los tres campos del `PATCH` son punteros** (T017, T018). Con `bool` pelado, un `PATCH` de un
   interruptor apaga los otros.
3. **`requiresEntry` por método** (T012, T013, T021, T023). Sin él, el arqueo ciego deja firmar un
   cierre sin capturar nada: `Number(null) === 0` y la guardia del cierre concluye que nada falta.
4. **Las dos pantallas se miden, no se suponen** (T025, T026). El plan afirmaba que la tabla se
   acorta contando campos en vez de renglones, y diseñaba Ajustes contra 1024 px cuando mide 560.

## Lo que `/speckit-analyze` cambió

Nueve hallazgos, ninguno de diseño: cuatro ediciones al spec, dos al contrato y al plan, y cuatro
renglones en tareas que ya existían.

- **El crítico era el principio V**: el arqueo ciego es un control de seguridad y nadie había
  contestado «¿un atacante lo evade?». Sí, por dos vías, y las dos están ahora en el plan — una se
  acepta por escrito y la otra se cierra en T021.
- **FR-016 es nuevo y su regla es vieja**: el cierre bloqueado hasta capturar todo nunca se había
  escrito en un spec. Vivía en el comentario de `cierreDeCaja.ts` desde el corte de los $1,662.
- FR-009 pasa a tener prueba (T017), FR-004 pasa a tener dueño (T015) y SC-006 pasa a medirse (T025).
- **FR-014 pasa de implícito a asertado** (T023): el total que el operador capturó tiene que seguir
  visible con el arqueo ciego encendido, y eso se dice porque se parece demasiado a lo que sí se
  oculta.

---

## Fase 1: Setup

- [X] T001 Confirmar que `server/migrations/` no tiene un `0067` sin mergear en otra rama viva. La
      más alta hoy es `0066_conteo_de_efectivo.sql`, ya en el ambiente de pruebas.

---

## Fase 2: Foundational — bloquea las tres historias

Sin esquema y sin aritmética no hay nada que cablear, y las dos piezas irreversibles del plan viven
aquí.

- [X] T002 Escribir el test de integración de la migración en
      `server/internal/integration/migracion_arqueo_del_cajon_test.go`, **antes** de la migración y
      viéndolo fallar. Con **dos empresas**, porque con una sola todo camino "por cada otra empresa"
      es un no-op. Cubre:
      - las columnas nuevas existen con su tipo, y `session_cash_counts.difference` es **generada**:
        un `update` directo a esa columna se rechaza;
      - el check por momento: un insert de `moment='cierre'` **sin** `expected` falla con `23514`, y
        uno de `'apertura'` **con** `expected` también;
      - el check del renglón: un `register_session_totals` con `affects_cash_drawer` y
        `declared <> expected` se rechaza;
      - las filas que ya existen (conteos de la 0066) **sobreviven** al `not valid`;
      - `blind_cash_count` nace en `false` para las empresas existentes;
      - el `Down` no deja basura y las columnas de dinero de siempre siguen ahí.
- [X] T003 Escribir `server/migrations/0067_arqueo_del_cajon.sql` con el `set local lock_timeout`
      que ya trae la 0066 —una columna generada `stored` reescribe la tabla bajo `ACCESS
      EXCLUSIVE`— y las cinco piezas de [data-model.md](./data-model.md): `expected` +
      `difference` generada + check por momento en `session_cash_counts`; `affects_cash_drawer` +
      check en `register_session_totals`; `blind_cash_count` en `business_settings`.
- [X] T004 Correr la migración contra un respaldo anonimizado de producción y contra el ambiente de
      pruebas, que ya tiene conteos de la 0066. Verificar el tiempo del `alter` con datos.
- [X] T005 [P] Escribir `server/internal/domain/cajon_test.go`, table-driven, **viéndolo fallar**:
      el esperado del cajón es fondo + lo cobrado con los métodos que lo tocan + neto de movimientos;
      un método que **no** toca el cajón no suma; el redondeo en la frontera; un esperado que excede
      el tope de dinero se rechaza como validación y no revienta; cero métodos de cajón da cero y no
      es error.
- [X] T006 Escribir `server/internal/domain/cajon.go` hasta que T005 pase. Sin I/O: recibe las
      cifras ya leídas, como `PiezaContada` recibe el valor y no el id.
- [X] T007 [P] Escribir en `server/queries/cash.sql` y correr `make sqlc`:
      - `ExpectedByMethodForSession` incluye los métodos **inactivos con pagos en este turno**
        (`where pm.is_active or op.id is not null`) — el filtro actual haría desaparecer del arqueo
        el dinero ya cobrado con un método que se apague a media jornada;
      - `SaveSessionTotal` guarda el snapshot de `affects_cash_drawer`;
      - guardar `expected` en el conteo de cierre;
      - `ListSessions` suma la diferencia del conteo como **segunda subconsulta correlacionada**, no
        como join: con join, dos filas de conteo por turno duplican cada diferencia por método.

**Checkpoint**: el esquema y la aritmética están listos y probados. Ninguna pantalla cambió todavía.

---

## Fase 3: User Story 1 — el arqueo del cajón es una sola cifra (P1)

**Meta**: el conteo se compara contra una cifra, la diferencia es una, y el cierre deja de pedir un
reparto que nadie puede hacer.

**Prueba independiente**: cerrar un turno con una venta en efectivo de mostrador y otra en efectivo
de app, contar la suma de las dos, y ver diferencia $0 sin un faltante y un sobrante cancelándose.

### Servidor

- [X] T008 [P] [US1] Escribir `server/internal/integration/arqueo_del_cajon_test.go` en rojo:
      - un turno con fondo $500, $200 de mostrador y $100 de app en efectivo espera **$800** en el
        cajón, y contar $800 deja diferencia **$0**;
      - contar $750 deja **un** faltante de $50 en el conteo, y **cero** diferencia en cada renglón
        de método de cajón — el caso del turno 3 del spec, que hoy sale como −64.80 en un método y 0
        en otro;
      - un método cuyo efectivo **no** llega al cajón no entra al esperado;
      - los métodos que no tocan el cajón siguen declarándose con su cifra;
      - `ListSessions` reporta ese faltante en la diferencia total del corte. **Es el caso que hoy
        daría cero**: si nadie suma la diferencia del conteo, el histórico muestra cuadrado un corte
        que no cuadra.
- [X] T009 [US1] Escribir el test del **snapshot** en `server/internal/integration/arqueo_del_cajon_test.go`: cerrar un turno, cambiar
      después `affects_cash_drawer` del método de la app, y verificar que ese corte **no se
      reagrupa** — ni sus cifras ni la forma de su reporte. Sin el snapshot pasa lo contrario, y es
      reescribir el pasado.
- [X] T010 [US1] Cambiar `BackofficeService.CloseSession` en `server/internal/app/backoffice.go`:
      resolver el esperado del cajón con `domain`, guardar el conteo con su `expected`, y escribir
      `declared = expected` para los métodos de cajón. Todo dentro del `WithTx` que ya tiene.
- [X] T011 [US1] En `server/internal/httpapi/handlers_backoffice.go`, rechazar con 400 un método de
      cajón que llegue en `declared`, **nombrándolo** y diciendo que hay que recargar: el caso
      realista es una tableta con el front viejo en caché, no un atacante. Su test comprueba el
      **status**, no solo que falle.
- [X] T012 [US1] Exponer en las dos vistas del turno (`SessionView` y `SessionDetailView`) el objeto
      `drawer` de [contracts/api.md](./contracts/api.md) y **`requiresEntry` por método**. Las dos,
      derivadas una sola vez: dos derivaciones de la misma regla son dos pantallas que pueden
      diferir.

### Frontend

- [X] T013 [P] [US1] `web/src/features/backoffice/cierreDeCaja.ts`: test primero, en rojo.
      `faltanPorContar` y `diferenciasDelCierre` dejan de mirar `expected` para decidir qué falta y
      usan **`requiresEntry`**. Casos: un método de cajón no exige captura propia; el conteo del
      cajón sí; con `expected` en null (arqueo ciego) **sigue exigiendo** lo que falta —hoy
      `Number(null) === 0` lo apagaría todo—; la diferencia del cajón es una.
- [X] T014 [US1] `web/src/features/backoffice/CashPage.tsx`: el renglón del cajón con su conteo y su
      diferencia, el botón de contar movido a ese renglón, y el **tercer estado** de la columna
      «Declarado» con el texto **«Va al cajón»** en gris — la misma cadena que el encabezado de
      Ajustes, porque es el mismo concepto y dos textos enseñan que son dos cosas. No reusar «Automático»: un método
      auto-declarado lo resuelve el servidor y uno de cajón se cuenta físicamente.
- [X] T015 [US1] Test de pantalla en `web/src/features/backoffice/CashPage.test.tsx`: el renglón del
      cajón, los métodos informativos sin control, y que un corte **anterior** a esta feature —sin
      `drawer`— se pinte como siempre. **Incluye FR-004**: el corte sigue informando cuánto entró en
      efectivo por cada canal. Sin dueño, "no rompas el reporte por canal" es lo primero que se
      rompe al reescribir la tabla.
- [X] T016 [US1] El detalle del corte y el histórico muestran la diferencia del cajón, en
      `web/src/features/backoffice/CashPage.tsx` (componentes `CorteSummary` y el histórico), con su
      test en `web/src/features/backoffice/CashPage.test.tsx`.

**Checkpoint**: un turno con efectivo de mostrador y de app cierra con una sola diferencia.

---

## Fase 4: User Story 2 — configurar los métodos de pago (P2)

**Meta**: los dos interruptores se mueven desde la aplicación, y apagar un método no desaparece
dinero.

**Prueba independiente**: apagar un método, ver que deja de ofrecerse al cobrar, y comprobar que el
arqueo del turno abierto sigue esperando lo que ya cobró.

- [X] T017 [P] [US2] Escribir `server/internal/integration/metodos_configurables_test.go` en rojo:
      - `PATCH` con un solo campo **no pisa los otros dos** — con `bool` pelado, `{"isActive":
        false}` resetea `affectsCashDrawer` y saca dinero del arqueo;
      - un cuerpo sin ninguno de los tres campos se rechaza;
      - un método desactivado **deja de ofrecerse para cobrar** un pedido nuevo (FR-009, que era el
        único requisito sin prueba);
      - desactivar un método **con pagos en el turno abierto** no lo saca del esperado;
      - un corte ya cerrado no cambia de cifras ni de forma al cambiar los interruptores;
      - sin rol de administración, 403;
      - cada cambio deja evento de seguridad.
- [X] T018 [US2] En `server/internal/httpapi/handlers_backoffice.go`, los tres campos del `PATCH`
      como `*bool` y por el **mismo** camino de rol y evento de seguridad que ya tiene
      `autoDeclare` — no una rama nueva sin control.
- [X] T019 [US2] `web/src/features/admin/BusinessSettingsPage.tsx`: la lista de métodos pasa a
      **tabla** con columnas **Método / Activo / Va al cajón / Automático**, como ya hace
      `ManageRegistersTab`. Las etiquetas viven en el encabezado —en 520 px de ancho útil no caben
      repetidas por renglón—, los interruptores llevan `size="lg"`, y «Activo» queda separado de los
      otros dos: es el único con efecto inmediato en el mostrador.
- [X] T020 [US2] Test de pantalla en `web/src/features/admin/BusinessSettingsPage.test.tsx`: la
      tabla de métodos, y que apagar «Activo» no toque los otros dos interruptores del mismo
      renglón.

**Checkpoint**: el dueño configura qué se cobra y qué entra al cajón sin ayuda de un desarrollador.

---

## Fase 5: User Story 3 — arqueo ciego (P3)

**Meta**: quien cuenta no ve lo que el sistema espera, y la diferencia aparece al confirmar.

**Prueba independiente**: encender el interruptor y comprobar que ninguna cifra esperada llega a la
pantalla, y que el cierre sigue bloqueado hasta capturar todo.

- [X] T021 [P] [US3] Escribir `server/internal/integration/arqueo_ciego_test.go` en rojo: con el
      interruptor encendido y el turno **abierto**, `drawer.expected` y **todos** los
      `totals[].expected` viajan en null —no ocultos en el cliente, que los deja legibles en la
      respuesta— y `requiresEntry` **sigue** diciendo qué falta capturar. Con el turno cerrado, las
      cifras viajan.
- [X] T022 [US3] La columna en `server/queries/settings.sql` y el par `GET`/`PATCH` de
      `blindCashCount` en `server/internal/httpapi/handlers_settings.go`, con rol de administración.
- [X] T023 [US3] `web/src/features/backoffice/CashPage.tsx`: sin columna «Esperado» y sin bloque de
      diferencia mientras el interruptor esté encendido, y **con el total de lo que el operador
      capturó siempre visible** (FR-014). Lo segundo se dice porque al implementar «quitar la columna
      y el bloque» es fácil llevarse también el total propio, y entonces el operador cuenta a ciegas
      de verdad: sin la respuesta *y* sin su propia suma. Su test asserta las tres cosas: que el
      esperado no está, que la diferencia no está, y que el total capturado sí.
      **Test primero** del borde que la revisión encontró: con `expected` en null, el botón «Cerrar
      caja» sigue **bloqueado** hasta capturar todo (FR-016). Y el ajuste se lee al abrir el cierre y
      se conserva hasta terminarlo: un cierre a medio capturar no pasa de mostrar la diferencia a
      ocultarla.
- [X] T024 [US3] **Enmendar FR-005 de la 003** en `specs/003-arqueo-denominaciones/spec.md`
      (FR-015): el requisito pasa a depender de un ajuste del negocio, y se dice ahí. No se corrige
      por debajo.

**Checkpoint**: el negocio elige si quien cuenta ve la respuesta.

---

## Fase 6: Polish y cierre

- [X] T025 [P] Medir con Playwright a 1024×600, en `web/e2e/`, el alto de la tabla del cierre
      **antes y después**: tiene tres campos menos y un renglón más, y el neto no se supone. Si
      empeoró, el recorte va antes del merge — esa pantalla ya mide 1,494 px contra un viewport de
      600. **Y contar los toques que cuesta cerrar**, antes y después: es SC-006, y es el único
      criterio que protege contra "arreglar" el arqueo agregando dos pantallas más.
- [X] T026 [P] Medir la **línea base** de Ajustes del negocio: cuántos de los diez renglones de
      «Corte de caja» se ven hoy sin desplazarse, y cuántos después de la tabla. Ese *antes* no
      existe en ningún documento, y sin él la comparación que pide el quickstart no se puede hacer.
- [X] T027 Casos e2e del arqueo ciego en `web/e2e/`: ninguna cifra esperada en la respuesta ni en la
      pantalla, y el botón bloqueado sin captura completa.
- [X] T028 [P] Agregar los renglones de esta feature a
      [docs/matriz-de-cobro.md](../../docs/matriz-de-cobro.md) —el doble conteo del efectivo de
      plataforma y el corte que cuadraba cancelando dos diferencias— y a
      [docs/matriz-de-pantallas.md](../../docs/matriz-de-pantallas.md).
- [X] T029 [P] Actualizar [docs/presupuesto-de-pantalla-1024x600.md](../../docs/presupuesto-de-pantalla-1024x600.md)
      con lo medido en T025 y T026.
- [X] T030 Correr [quickstart.md](./quickstart.md) completo, incluido el caso del front viejo en
      caché.
- [X] T031 Gates completos: `make api-build`, `make api-test`, integración con `-tags=integration`,
      `make lint`, `make vuln`, `bun run lint`, `bun run vitest run`, `bun run build`,
      `bun audit --audit-level=high`.

---

## Fase 7: lo que encontró la revisión de código (2026-09-10)

Todas con su test **en rojo antes del arreglo**. No estaban planeadas: salieron de `/revision-de-codigo`
y de validar la 015 contra las specs que toca.

- [X] T032 Rechazar el cierre de un cajón que espera dinero y llega **sin conteo**
      (`domain.ErrCajonSinContar`). CRÍTICO: cada método del cajón declara su esperado, así que sin
      conteo el corte reportaba $0 de diferencia con $835 sin contar — peor que antes de la feature.
      Test: `TestCerrarSinContarElCajonSeRechaza`.
- [X] T033 Devolver el efectivo de una app que va al cajón registra la salida de caja (FR-018):
      `SaleDelCajon` sale de `affects_cash_drawer` y no de `kind = 'efectivo'`. Enmienda escrita a
      D2 de la spec 007. Tests: `TestDevolverElEfectivoDeUnaAppSaleDelCajon`,
      `TestSaleDelCajonLoQueEstabaEnElCajon`.
- [X] T034 Rechazar mover «va al cajón» con dinero de ese método ya en el turno abierto (FR-017):
      `domain.CambioDeCajonPermitido`. Tests: `TestCambioDeCajonPermitido`,
      `TestApagarVaAlCajonNoBorraElDineroQueYaEstaEnElCajon`.
- [X] T035 `UpdatePaymentMethod` leía y escribía sin transacción: ahora va en `store.WithTx` con
      `LockPaymentMethod` (`for update`), y la 0067 agrega el `check`
      `payment_methods_cajon_no_se_autodeclara` que lo respalda donde sí es atómico.
- [X] T036 Desactivar «Efectivo» borraba el fondo de apertura del arqueo: `ExpectedByMethodForSession`
      conserva siempre el renglón de tipo efectivo. Test:
      `TestDesactivarElEfectivoNoBorraElFondoDelArqueo`. Y la 0067 ancla que haya **uno solo** por
      empresa (`payment_methods_un_efectivo_por_empresa`): con dos, el fondo se sumaría dos veces.
- [X] T037 Un test que podía pasar vacuamente en `arqueo_ciego_test.go` (el default de la bandera
      coincidía con lo esperado si el método desaparecía de la lista).
- [X] T038 El mensaje del cierre rechazado explicaba el mecanismo: ahora dice la acción primero.
- [X] T039 Medir la separación horizontal entre los tres interruptores: **46 px**, no los 16 que
      estimó la revisión. Queda afirmada en P2 porque el hueco lo dan los encabezados.

---

## Dependencias

- **La Fase 2 bloquea todo.** Las dos piezas irreversibles —el esperado guardado y el snapshot del
  flag— viven ahí, y una pantalla construida antes se construye contra un modelo que va a cambiar.
- **US1 → US2**: US2 cambia los interruptores que US1 usa para decidir qué entra al cajón. Se puede
  empezar el servidor de US2 en cuanto T007 exista, pero su prueba independiente necesita US1 para
  ver el efecto en el arqueo.
- **US1 → US3**: el arqueo ciego oculta lo que US1 construye. T013 es la bisagra: si `requiresEntry`
  no existe antes de US3, el arqueo ciego apaga la guardia del cierre.
- **T024 depende de que US3 exista**: enmendar el requisito de la 003 antes de que su reemplazo
  funcione deja la 003 diciendo algo que el código no hace.
- **T025 y T026 dependen de T014 y T019**: no se puede medir una pantalla que no existe.

## Paralelo, por fase

- **Fase 2**: T005 (dominio) y T007 (consultas) van juntas; T002 y T003 son secuenciales entre sí.
- **US1**: T008 y T013 van juntas — una es servidor y la otra front puro.
- **US2**: T017 mientras alguien hace T019.
- **Polish**: T025, T026, T028 y T029 son cuatro archivos distintos.

## Estrategia

El MVP es **US1 sola**: cierra el defecto que hoy está en operación y no necesita ninguna de las
otras dos. US2 lo vuelve correcto para el siguiente cliente y US3 es el control que el negocio pidió
para cuando entre personal nuevo.

Si hay que entregar en dos partes, el corte natural es después del checkpoint de US1: el arqueo
queda correcto con la configuración actual, y los interruptores siguen cambiándose como hoy hasta
que llegue US2.
