# Tasks: Dónde cae el dedo

**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md),
[quickstart.md](./quickstart.md)

**Tests**: obligatorios (principio IV). Y aquí hay tres promesas que **solo un test sostiene**,
porque las tres se rompen en silencio: que el arrastre no cuente, que las capas de encima no se
cuenten, y que el volumen no crezca con los dedos.

## Lo que hay que tener en la cabeza antes de empezar

1. **Se reusa el registrador de la 017 entero.** Cola, lote, `keepalive`, uno en vuelo, limitador,
   descarte: nada de eso se vuelve a escribir. Si una tarea de abajo parece pedirlo, está mal leída.
2. **La celda se calcula en la tableta.** Al servidor nunca llega un `(x, y)`.
3. **Las capas de encima no cambian de ruta.** Hojas, diálogos y el bloqueo por PIN contaminan la
   pantalla de abajo si no se excluyen — y el PIN es el peor porque siempre cae en el mismo sitio.
4. **El techo se cuenta con las DOS orientaciones.** Ya se me olvidó una vez.
5. **La migración se escribe con su test**, y el hook de pre-commit lo exige.

---

## Fase 1: Setup

- [X] T001 Confirmar rama y feature activa (`019-mapa-por-coordenada` en `.specify/feature.json`).
- [X] T002 [P] Anotar el tamaño del paquete del POS **antes**: `cd web && bun run build`. La
      referencia es 1,134.90 kB, y el criterio de éxito es no crecer más de 5 kB.

---

## Fase 2: Fundacional (bloquea todo lo demás)

### La migración, con su test antes

- [X] T003 Test de la migración en `server/internal/integration/migracion_toques_test.go`, **antes**
      de escribirla: la tabla existe; **no tiene ninguna columna de tiempo más fina que el día** ni
      columna de usuario; la llave suma con `role` nulo en vez de crear filas; y los cuatro `check`
      rechazan lo suyo —celda 84, orientación `'landscape'`, `hits` negativo y una pantalla de 5 KB—.
      Verlo fallar.
- [X] T004 Escribir `server/migrations/0070_toques_por_zona.sql`: la tabla con `fillfactor = 70`,
      su llave `unique nulls not distinct`, los cuatro `check`, el índice por `day`, RLS con
      `tenant_isolation`, los grants de la app y de la consola, y la política
      `plataforma_lee_todos_los_toques`.

      El `check` de la orientación es el que más importa y el que menos se nota: sin él una tableta
      que mande `'landscape'` crea un **balde invisible** —la fila entra, pasa el rango de celda
      porque 0..83 vale en las dos formas, y la consola nunca la muestra—.

- [X] T005 Test de la política, **visto en rojo quitándola**: con dos empresas, la consola ve las
      dos y el rol del negocio solo la suya.
- [X] T006 El `Down` y su test: revertir borra la tabla y volver a aplicar la deja usable.

### El dominio: de un toque a una celda

- [X] T007 [P] Tests en `server/internal/domain/toque_test.go`, en tabla: la celda válida es 0..83;
      la orientación solo acepta dos valores; una pantalla fuera de la lista instrumentada se
      rechaza; y el par (rol, pantalla) se valida como en la 017.
- [X] T008 [P] `server/internal/domain/toque.go`: el tipo del toque, la lista **corta** de pantallas
      instrumentadas (hoy solo `pos`), la validación y las constantes de la rejilla (12 × 7).

      Las constantes viven aquí y no en la base **a propósito**: cambiar la resolución tiene que
      verse en un diff. Y hacia una rejilla más fina no se puede volver — no existe el toque fino
      del cual recalcularla.

- [X] T008b [P] Test de que la lista instrumentada es **subconjunto** de `pantallasMedibles` (017).

      El toque entra por el mismo endpoint y se valida contra aquella lista: una pantalla
      instrumentada que no esté allá tiene **todos** sus toques descartados en silencio, y la
      rejilla sale vacía sin un solo error. Es el modo de falla de toda esta familia de features —no
      falla, mide menos— y «menos» se lee como «nadie lo usa».

- [X] T009 [P] Tests de la traducción toque→celda en `web/src/app/celda.test.ts`: una posición
      relativa cae en la celda que le toca, los bordes no se salen del rango, y en vertical la
      rejilla es 7 × 12.

      **Y el caso que fija FR-004**: con la página desplazada, el mismo punto físico de la pantalla
      da la **misma** celda. Es lo que separa «qué parte del vidrio usa la mano» de «qué contenido
      se tocó», y sin ese test la decisión vive solo en un comentario.
- [X] T010 [P] `web/src/app/celda.ts`: la función pura que convierte `(x, y, ancho, alto)` en
      `{celda, orientacion}`. Sin React, sin DOM: es la que se prueba en tabla.

### Las consultas

- [X] T011 `server/queries/toques.sql`: el `upsert` del conteo —que recibe el día ya calculado, como
      el de la 017— la lectura de la rejilla, y el `delete` del recorte **con su propia constante**
      (`RetencionDeToquesEnDias = 92`, no la de 396 del agregado). `make sqlc`.

**Checkpoint**: el esquema existe y está probado bajo los tres roles.

---

## Fase 3: US2 — Nunca una captura, nunca la persona (P1) 🎯

**Va primero**, como en la 017: es lo que no se puede corregir después.

- [X] T012 [US2] Test de integración en `server/internal/integration/toques_anonimos_test.go`: tras
      registrar toques, ninguna fila tiene columna de usuario ni de hora; un rol con un solo
      empleado activo queda **sin rol**; y una ráfaga de mil toques en la misma celda **no cambia el
      número de filas**.
- [X] T013 [US2] Test de que el cuerpo del request **no puede traer `x` ni `y`**: aunque los mande,
      no hay dónde guardarlos y no se guardan.
- [X] T014 [US2] Guardia estático en `web/src/api/uso-orden.test.ts` (o su gemelo): **ninguna parte
      del front manda una imagen ni texto de la pantalla** por el endpoint de medición.

      Es FR-009 y es la promesa que el spec llama «la mitad de la feature». Un guardia que lee el
      código es lo único que atrapa la forma prohibida antes de que exista.

---

## Fase 4: US3 — Que el dedo no se entere (P1)

- [X] T015 [P] [US3] Tests en `web/src/app/MedidorDeToques.test.tsx`, antes del código: un toque
      cuenta; un **arrastre de más de 10 px no**; dos contactos simultáneos no se mezclan (cada uno
      con su `pointerId`); y un toque dentro de un `[role="dialog"]` **no cuenta**.
- [X] T016 [US3] Darle `role="dialog"` y `aria-modal` a `web/src/features/auth/LockScreen.tsx`, con
      su test. **Va antes del escuchador**: es lo que hace que el filtro de capas sirva de algo.

      Sin esto, el teclado del PIN —que siempre cae en el mismo sitio— deja una zona caliente en el
      centro de la pantalla de abajo que dentro de seis meses alguien va a leer como «un control muy
      usado». Y el rol además es lo correcto: es un modal que bloquea todo.

- [X] T017 [US3] `web/src/app/MedidorDeToques.tsx`: el escuchador de `pointerdown`/`pointerup` en el
      documento, con el mapa por `pointerId`, el umbral de 10 px y el filtro de capas.

- [X] T018 [US3] Filtrar en el **cliente** por la lista de pantallas instrumentadas, reusando
      `pantallaDe()`: lo que el servidor va a tirar no se manda.
- [X] T019 [US3] `medirToque()` en `web/src/api/uso.ts`, que encola en la MISMA cola de la 017.
- [X] T020 [US3] Test de que el escuchador **no interfiere**: un toque sobre un botón sigue
      disparando su `onClick`, y el desplazamiento de una lista sigue funcionando.

---

## Fase 5: US1 — Ver dónde se toca (P1)

- [ ] T021 [US1] Test de integración de `GET /api/v1/platform/touches`: devuelve **las 84 celdas**
      —incluidas las de cero—, la rejilla y la orientación; con sesión del negocio da **401**; un
      rango más viejo que la retención da **400**; y `empresa` ausente suma todas.
- [ ] T021b [US1] Test de que **las dos orientaciones no se suman** (FR-016): con la misma celda
      sembrada en horizontal y en vertical, pedir una devuelve solo la suya.

      Mezclarlas pinta una rejilla que nadie tocó nunca: la celda 37 es otro lugar en cada forma.
- [ ] T022 [US1] `UsageService.Rejilla(...)` y el handler, dentro del grupo de plataforma.
- [ ] T023 [P] [US1] `web/src/consola/zonas-del-pos.ts`: la leyenda **fechada** de qué es cada fila
      y columna del layout vigente. Texto, nunca una imagen.
- [ ] T024 [US1] Tests de `web/src/consola/RejillaDeToques.test.tsx`: pinta la rejilla con la
      proporción correcta, el número dentro de las celdas con conteo, dice «todavía no hay toques»
      con cero datos, y muestra la leyenda con su fecha.
- [ ] T025 [US1] `web/src/consola/RejillaDeToques.tsx` con CSS propio, y engancharla en la consola
      junto al mapa de la 017.

---

## Fase 6: US4 — Que quepa (P1)

- [ ] T026 [US4] Test de volumen en `server/internal/integration/toques_volumen_test.go`: siembra un
      trimestre **al tope, con las dos orientaciones y con el patrón real de escritura** (muchos
      `update` sobre las mismas filas, no un `insert` con el total) y falla si pasa de **20 MB**.
- [ ] T027 [US4] Sumar el `delete` de toques a `UsageService.Recortar`, con su constante propia, y
      su test: lo de más de 92 días se va, lo de dentro no se toca.

---

## Fase 7: Cierre

- [ ] T027b [US3] Extender `web/e2e/medir-no-estorba.spec.ts` para que **toque** la pantalla, no
      solo navegue: con el endpoint de medición colgado, capturar tocando rápido tiene que seguir
      respondiendo igual. Es SC-004, y hoy solo lo comprueba una persona leyendo el quickstart.
- [ ] T028 Correr [quickstart.md](./quickstart.md) completo, **incluido el paso 3-bis** (que el
      Picker y el PIN no cuenten) y el de la red muerta.
- [ ] T029 [P] Medir el paquete del POS contra T002 (no más de +5 kB) **y comprobar que
      `web/package.json` y `web/bun.lock` no cambiaron**, que es SC-007 medido en vez de prometido.
- [ ] T030 [P] Renglones en `docs/matriz-de-pantallas.md` y en `docs/security-owasp.md`: qué cubre
      cada test y **qué no**.
- [ ] T031 [P] Documentar en `AGENTS.md` dónde se agrega una pantalla instrumentada (son dos
      lugares, como en la 017) y que la leyenda se actualiza con cada rediseño.
- [ ] T032 Gates completos y despliegue.

---

## Dependencias

- **La fase 2 bloquea todo.**
- **US2 antes que las demás**: es lo que no se corrige después.
- **T016 antes que T017**: sin el rol en `LockScreen`, el filtro de capas del escuchador no lo
  atrapa, y el peor contaminante entra igual.
- **T009/T010 son [P]** con el dominio de Go: son archivos y lenguajes distintos.

## Estrategia

El corte más chico que vale desplegar es **fases 2, 3 y 4**: se empieza a contar con el anonimato
resuelto y sin estorbar, aunque la rejilla todavía no exista. Los datos se acumulan mientras se
construye, y así la primera vez que se abra ya dice algo — una rejilla estrenada el mismo día que se
escribe está vacía y no prueba nada.
