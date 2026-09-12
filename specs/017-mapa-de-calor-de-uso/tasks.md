# Tasks: Un mapa de calor de uso

**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md),
[quickstart.md](./quickstart.md)

**Tests**: obligatorios. El principio IV de la constitución no es negociable, y aquí además hay dos
promesas que solo un test puede sostener: que la medición **no estorba** (US3) y que el volumen
**no crece sin límite** (US4). Las dos se rompen en silencio.

## Lo que hay que tener en la cabeza antes de empezar

Cinco cosas que este repo ya aprendió, y que esta feature toca de lleno:

1. **El anonimato se decide al escribir.** Si el primer evento se guarda con la persona, ya se
   guardó. Por eso US2 va **antes** que el mapa, y no después.
2. **Un test de volumen que inserta el total de golpe pasa en verde mintiendo.** Hay que producir el
   churn real: muchos `update` sobre las mismas filas del día.
3. **Un ticker de 24 h no dispara nunca si el binario se reinicia antes.** Aquí se redespliega en
   cada merge.
4. **La migración se escribe con su test**, y el hook de pre-commit lo exige.
5. **La red mala es peor que la red caída.** La caída falla rápido; la lenta apila lotes encima del
   cobro.

---

## Fase 1: Setup

- [X] T001 Confirmar que `017-mapa-de-calor-de-uso` es la rama y la feature activa
      (`.specify/feature.json`).
- [X] T002 [P] Anotar el tamaño del paquete del POS **antes** de tocar nada: `cd web && bun run
      build`, y dejar el número en el commit. La referencia es 1,133.50 kB (2026-09-11); sin el
      antes, el «no creció» del final no se puede afirmar.

---

## Fase 2: Fundacional (bloquea todo lo demás)

### La migración, con su test antes

- [X] T003 Escribir el test de la migración en
      `server/internal/integration/migracion_uso_test.go`, **antes** de la migración: que las dos
      tablas existen, que `usage_events` **no tiene ninguna columna de usuario**, que la llave de
      `usage_daily` es `unique nulls not distinct` —probándolo: dos upserts con `action` y `role`
      nulos suman en la misma fila, no crean dos—, y que los `check` de longitud rechazan una
      cadena de 5 KB. Verlo fallar.
- [X] T004 Escribir `server/migrations/0069_uso_del_sistema.sql`: las dos tablas con su
      `company_id … references companies(id) on delete cascade` **escrito literal**, el índice
      `(company_id, occurred_at)`, la llave `unique nulls not distinct`, los `check` de longitud, el
      `enable row level security` con `tenant_isolation` en las dos, y los grants: `insert` sobre
      `usage_events` y `select, insert, update` sobre `usage_daily` para `gatobobah_app`.

      El `update` es para el `upsert` y el `select` porque Postgres lo exige para leer `hits` en el
      `set`; los dos van con su comentario, o el siguiente que los lea los quita por «sobran».

- [X] T005 Agregar la política de plataforma en la misma migración:
      `plataforma_lee_todo_el_uso on usage_daily for select to gatobobah_platform using (true)`,
      más `grant select on usage_daily to gatobobah_platform`. **Y ningún grant sobre
      `usage_events`.**
- [X] T006 Test de la política en `migracion_uso_test.go`, **visto en rojo quitándola**: con dos
      empresas sembradas, el rol de plataforma ve el agregado de **las dos**, el rol de la app solo
      el suyo, y `select` sobre `usage_events` desde plataforma da **42501**.
- [X] T007 Escribir el `Down`: borra las dos tablas. Test de que revertir y volver a aplicar deja el
      esquema igual.

### El dominio: lo que se puede contar y cómo se cuenta

- [X] T008 [P] Tests en `server/internal/domain/uso_test.go`, en tabla: la lista blanca acepta lo
      conocido y rechaza lo demás (incluidas cadenas larguísimas y vacías), el pre-agregado de un
      lote suma por `(pantalla, acción, rol)`, y un lote de más de 50 se recorta.
- [X] T009 [P] `server/internal/domain/uso.go`: la lista blanca de pantallas y acciones, el tipo del
      evento, `PreAgregar(lote)` y los sentinels. Puro, sin I/O.

      **Los roles son los CUATRO que existen** (`admin`, `gerente`, `cajero`, `mesero`), no los tres
      que nombra el spec: el enum `user_role` ya tiene el cuarto, y una lista que se olvide de él
      manda a `mesero` al camino de error en vez de contarlo.

      La lista es la que mantiene acotado el número de combinaciones distintas: sin ella, cuántas
      filas puede tener `usage_daily` lo decide el cliente.

- [X] T010 [P] Tests de la regla de k-anonimato en `uso_test.go`: con 0 o 1 usuario activo de ese
      rol, el corte se suprime; con 2 o más, se conserva. Es puro: recibe el conteo, no la base.
- [X] T011 [P] `domain.CorteDeRolPermitido(usuariosActivosDelRol int) bool`, con el porqué escrito:
      decir «el rol gerente hizo 40 acciones» en una empresa con un gerente es decir su nombre.

### Las consultas

- [X] T012 `server/queries/uso.sql` con las cuatro: insertar el grano fino, el `upsert` del
      agregado (`on conflict on constraint usage_daily_llave do update set hits = usage_daily.hits +
      excluded.hits`), contar usuarios activos por rol de la empresa, y la lectura del mapa. Correr
      `make sqlc`.

      Ojo con la del upsert: suma `excluded.hits`, **no** `+ 1` — el lote llega pre-agregado.

**Checkpoint**: el esquema existe, está probado bajo los tres roles y la lista blanca es la única
puerta de entrada. Recién aquí empiezan las historias.

---

## Fase 3: US2 — Por rol, nunca por persona (P1) 🎯

**Va primero a propósito.** Es la única decisión de esta feature que no se puede corregir después:
lo que se escribió con la persona, se escribió.

**Prueba independiente**: con dos roles usando el sistema, ninguna consulta —ni desde la app ni
desde la base— permite reconstruir qué hizo un empleado.

- [X] T013 [US2] Test de integración en `server/internal/integration/uso_anonimo_test.go`, antes del
      código: tras ingerir eventos de un usuario, **ninguna fila de ninguna de las dos tablas
      contiene su id**, y el esquema no tiene ninguna columna que apunte a `users`.
- [X] T014 [US2] Test del caso que de verdad importa: una empresa con **un solo** usuario activo de
      un rol ingiere eventos → las filas del agregado quedan con `role` **nulo**. Con dos usuarios
      del mismo rol → el rol se conserva. Verlo en rojo saltándose `CorteDeRolPermitido`.
- [X] T015 [US2] `server/internal/app/uso.go`: `UsageService.Registrar(ctx, lote)` — valida contra
      la lista blanca, pre-agrega, cuenta los usuarios activos del rol, decide la supresión y hace
      **una sola transacción** con el insert del grano fino y el `upsert` del agregado.
- [X] T016 [US2] Test de que el rol sale del **token** y no del cuerpo: un lote que trae `"rol":
      "admin"` inventado no cambia lo que se guarda.

---

## Fase 4: US3 — Que el sistema no se entere de que está midiendo (P1)

**Prueba independiente**: con la red caída, el operador arma una cuenta y cobra sin ver un error ni
una demora atribuible a la medición.

### El endpoint, que nunca puede estorbar

- [X] T017 [US3] Test en `server/internal/httpapi/uso_test.go`, antes del código: el endpoint
      responde **204 siempre** —lote vacío, cuerpo mal formado, pantallas desconocidas, 500 eventos—
      y nunca un cuerpo de error.
- [X] T017b [US3] **Test del limitador, antes de escribirlo**: pasado el tope por usuario, los
      eventos **no se escriben** y la respuesta sigue siendo 204.

      El principio V no deja mergear un control de seguridad sin su test, y aquí la razón es más
      fuerte que el principio: como el endpoint responde 204 pase lo que pase, un limitador roto
      —o desconectado por un refactor del router— **no se nota por ninguna vía**. El único testigo
      posible es este test.

- [X] T017c [US3] Test de que el servidor **ignora cualquier `detail` que venga en el cuerpo**: un
      lote con `{"detail":{"cliente":"Juan"}}` se guarda con `detail` nulo.

      `detail` existe para las coordenadas del futuro (FR-013). Mientras el cliente pueda escribirlo,
      esa puerta es también un campo libre por donde entra justo lo que FR-003 prohíbe — y a
      diferencia de una columna mal usada, un `jsonb` no avisa.

- [X] T018 [US3] `POST /api/v1/usage` en `server/internal/httpapi/handlers_uso.go`, dentro del grupo
      del negocio con `RequireAuth` y `WithTenant`, con su limitador por usuario, el tope de 50
      eventos por lote y **el `detail` del cuerpo descartado sin mirarlo**.
- [X] T019 [US3] Test de que lo descartado **deja rastro**: una línea de `slog.Warn` con clave
      estable `usage_descartado`, el conteo del lote y **el primer nombre desconocido** (que es lo
      que dice qué hay que arreglar). No es `logging.SecurityEvent`: no es un evento de seguridad,
      es telemetría de la propia medición.

      Sin esa línea, una versión del front que manda nombres viejos deja de medir y nadie se
      entera — el mapa simplemente muestra menos, que es indistinguible de «se usó menos».

### El registrador del POS

- [X] T020 [P] [US3] Tests en `web/src/api/uso.test.ts`, antes del código: el lote se manda a los 20
      eventos, a los 10 segundos y al ocultarse la pestaña; **nunca hay dos envíos en vuelo a la
      vez**; un `fetch` que rechaza no lanza al llamador ni reintenta; y la cola se recorta a 50.
- [X] T021 [US3] `web/src/api/uso.ts`: la cola, el lote, `fetch` con `keepalive` **sin `await`**, y
      la guarda de «uno en vuelo».

      El caso que cubre la guarda es el de wifi lento, no el de wifi caído: con envíos de 8–15 s el
      temporizador dispara otro encima y terminan compitiendo con el `POST /orders/:id/pay`.

- [X] T022 [P] [US3] Test de que **la recarga no cuenta** como apertura: con
      `performance.getEntriesByType('navigation')[0].type === 'reload'`, la primera vista no se
      encola; con `navigate`, sí. Y que dos entradas legítimas seguidas a la misma pantalla **sí**
      cuentan las dos.
- [X] T023 [US3] `web/src/app/rutas-medidas.ts` y el enganche en el router: cada ruta dice qué
      pantalla es, y lo que no está en el mapa **no se mide** (no se inventa un nombre).
- [X] T024 [US3] Enganchar las acciones con nombre en los lugares donde ya ocurren (cobrar, cerrar
      caja, contar efectivo, editar producto, traspaso), **siempre después** de que la acción
      ocurra.
- [X] T025 [US3] Test de humo del orden: en el camino de cobro, el registro se encola **después** de
      que el cobro respondió. Un `await` colado ahí es el defecto que esta historia existe para
      impedir.

---

## Fase 5: US1 — Ver qué pantallas y qué acciones se usan (P1)

**Prueba independiente**: con un día de uso, la pantalla muestra al menos una pantalla con conteo
mayor que cero, y ese conteo sube al volver a abrirla.

- [ ] T026 [US1] Test de integración en `server/internal/integration/uso_consola_test.go`, antes del
      código: `GET /api/v1/platform/usage` con sesión de plataforma devuelve las pantallas
      **ordenadas de más a menos usada**, incluye las de conteo **cero**, y trae `rol: null` para lo
      suprimido.
- [ ] T026b [US1] Test que **fija qué incluye cada cifra**, con números que no cuadran por
      casualidad: `aperturas` cuenta solo las vistas de pantalla, `acciones[].veces` solo las
      acciones con nombre, y `porRol[].veces` es **el total de las dos** repartido por rol — de modo
      que `sum(porRol) == aperturas + sum(acciones)`.

      Sin esto, quien lea la respuesta va a sumar dos de las tres y va a reportar un número que no
      existe. Es el mismo defecto que el principio III persigue con el dinero, en otra moneda.

- [ ] T027 [US1] Test de lo que **no** puede traer, buscado en el JSON crudo: ninguna cifra de
      dinero, ningún dato de empleados, ningún id de usuario (FR-003).
- [ ] T028 [US1] Test de que una sesión **del negocio** contra esa ruta da **401**, no 403.
- [ ] T029 [US1] `UsageService.Mapa(ctx, desde, hasta, empresa)` y el handler
      `GET /api/v1/platform/usage`, dentro del grupo de plataforma.
- [ ] T030 [US1] Test del rango: un `desde` más viejo que la retención se **rechaza** con
      `ErrValidation`. No se recorta en silencio a lo que hay — una pantalla que devuelve otro rango
      del que pide miente (principio V).
- [ ] T031 [P] [US1] `web/src/consola/etiquetas-de-uso.ts`: cómo se llama cada pantalla en la
      consola. **Copia deliberada**, no import del POS: son dos productos y el linter lo impide.
- [ ] T032 [US1] Tests de la pantalla en `web/src/consola/MapaDeUso.test.tsx`: pinta las pantallas
      ordenadas, muestra el número dentro de la celda, dice «todavía no hay uso registrado» con cero
      datos (FR-015), y nombra «sin corte» cuando el rol viene nulo.
- [ ] T033 [US1] `web/src/consola/MapaDeUso.tsx`: la rejilla CSS con intensidad por color **y el
      número escrito**, el selector de periodo y el de empresa. Sin una sola dependencia nueva.

      **Un solo eje**: pantallas ordenadas por uso, acciones desplegables debajo. No es un
      calendario de contribuciones.

- [ ] T034 [US1] Enganchar el mapa en la consola (la 016 hoy solo tiene la lista de empresas).

---

## Fase 6: US4 — Que quepa en el disco del negocio (P2)

**Prueba independiente**: simulado un año de uso, el espacio ocupado se mantiene bajo el techo
declarado.

- [ ] T035 [US4] Test de volumen en `server/internal/integration/uso_volumen_test.go`: siembra un
      año **con el patrón de escritura real** —lotes que incrementan repetidamente las filas del
      día— y mide `pg_total_relation_size` de las dos tablas. Falla si pasa de **25 MB** por
      empresa.

      **Un `insert` con el total ya sumado no sirve**: mide un escenario que la operación nunca
      produce y esconde el costo de las versiones muertas.

- [ ] T036 [US4] Test del recorte: con filas de hace 20 días y de hace 14 meses, tras una pasada no
      queda ninguna fuera de la retención, y las de dentro **no se tocan**.
- [ ] T037 [US4] `UsageService.Recortar(ctx)` en `server/internal/app/uso.go`: borra
      `usage_events` de más de 14 días y `usage_daily` de más de 13 meses, con una conexión de
      **dueño** abierta y cerrada para eso —el rol de la app está bajo RLS y solo borraría lo de su
      empresa—.

      Va en `app` y **no** en un paquete `tareas` nuevo: la arquitectura de este repo tiene cuatro
      lugares (`httpapi`, `app`, `domain`, `store/db`) y un quinto para «cosas que corren solas» se
      convierte en el cajón de sastre donde termina la lógica que nadie sabe dónde poner.
- [ ] T038 [US4] Cablearlo en `server/cmd/api/main.go`: **corre al arrancar** y luego cada 24 h,
      colgado del contexto que ya se cancela en el apagado.

      El «al arrancar» es el que hace que exista: un ticker de 24 h se reinicia con el proceso y
      aquí se redespliega en cada merge, así que sin eso no dispararía **nunca**.

- [ ] T039 [US4] Test de que la goroutine **termina** al cancelar el contexto (principio II: ninguna
      goroutine sin condición de término).
- [ ] T039b [US4] Test de la puerta de FR-013 (SC-006): escribir `{"x":120,"y":340}` en el `detail`
      de una fila existente y leerlo de vuelta, **sin tocar el esquema**. Es la única forma de
      comprobar que «agregar coordenadas después no exige migrar» antes de necesitarlo.

---

## Fase 7: Cierre

- [ ] T039c [US3] Caso de Playwright en `web/e2e/`, contra el ambiente de pruebas: con la red
      bloqueada (`page.route` abortando `/usage`), armar una cuenta y cobrar. Pasa si el cobro
      responde igual y la pantalla no muestra un solo aviso.

      SC-003 es la promesa central de US3 y hoy solo la comprueba un humano leyendo el quickstart.
      Una promesa que solo se verifica a mano se rompe el día que nadie tiene tiempo de verificarla.

- [ ] T040 Correr [quickstart.md](./quickstart.md) completo, **incluido el paso 4-bis** (red lenta
      con throttling, verificando que no se apilan envíos). El paso de la red caída es el fácil.
- [ ] T041 [P] Medir el paquete del POS y compararlo contra el número de T002 (si creció más de unos
      pocos KB, algo del mapa se coló) **y comprobar que `web/package.json` y `web/bun.lock` no
      cambiaron** — que es SC-007 medido en vez de prometido.
- [ ] T042 [P] Renglones en `docs/matriz-de-pantallas.md`: qué cubre cada test de esta feature y
      **qué no** — en particular, que nadie mide que el mapa sea legible con datos reales.
- [ ] T043 [P] Documentar en `AGENTS.md` el endpoint de ingesta y la lista blanca (dónde se agrega
      una pantalla nueva), y en `docs/security-owasp.md` por qué esta feature no guarda identidad.
- [ ] T044 Gates completos: `make api-build`, `api-test`, integración, `make lint`, `make vuln`,
      `bun run lint`, los dos `typecheck`, `bun run vitest run`, los dos `build` y
      `bun audit --audit-level=high`.

---

## Dependencias

- **La fase 2 bloquea todo.** Sin el esquema y la lista blanca no hay dónde escribir ni qué validar.
- **US2 antes que US1 y US3**, y no es preferencia: es la decisión que no se puede corregir después
  de la primera fila.
- **US3 antes que US1** en la práctica: el mapa sin eventos no se puede ver funcionando de verdad.
- **T008–T011 son [P] entre sí** (dominio puro, archivos distintos), igual que T020/T022 (front) y
  todo lo de la fase 7 marcado [P].
- **T003 antes que T004**, y **T005 antes que T006**: el test primero, que además lo exige el hook.

## Estrategia

El corte más chico que tiene sentido desplegar es **fases 2, 3 y 4**: se empieza a medir, con el
anonimato ya resuelto, aunque todavía no haya nada que mirar. Los datos se acumulan mientras se
construye el mapa, y así la primera vez que se abra tendrá algo que decir — abrirlo el mismo día que
se construye muestra un mapa vacío que no prueba nada.

**Dónde parar y validar**: al terminar la fase 4, correr los pasos 1 a 5 del quickstart. Si la
medición se nota en el mostrador, no seguir: el resto de la feature no vale lo que cuesta.
