# Tasks: La consola de plataforma, separada del negocio

**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md), [quickstart.md](./quickstart.md)

**Tests**: obligatorios. El principio IV de la constitución es no negociable, y aquí además **los
tests de aislamiento no son opcionales**: son la única forma de saber si las barreras existen.

## Lo que hay que tener en la cabeza antes de empezar

Cuatro reglas que este repo ya aprendió a la mala y que esta feature toca de lleno:

1. **El aislamiento no se prueba en local.** La API de desarrollo se conecta como dueño de la base:
   ahí RLS y los grants **no aplican** y cualquier test de aislamiento pasaría siempre. Todos van a
   la suite de integración, bajo el rol de verdad.
2. **La migración se escribe con su test, no después.** Hay un hook de pre-commit que lo exige.
3. **Un test que nunca se vio en rojo no prueba nada.** Cada barrera se verifica quitándola.
4. **El `Down` no borra el rol** (ver plan, hallazgo 1).

---

## Fase 1: Setup

- [ ] T001 Crear la rama y confirmar que `016-consola-de-plataforma` es la feature activa.
- [ ] T002 [P] **(manual, fuera del repositorio)** Reservar los subdominios
      `staff-dev.elgatobobah.com` y `staff.elgatobobah.com`, y crear sus dos proyectos de Pages —
      uno por ambiente, como ya existen `el-gato-bobah-pos` y `el-gato-bobah-pos-dev`.

      Ningún test puede comprobar esto: se verifica al desplegar. Va marcado para que nadie lo dé
      por hecho al revisar la lista.

---

## Fase 2: Fundacional (bloquea todo lo demás)

**Nada de las historias puede empezar hasta que esto esté.**

### La migración, con su test antes

- [ ] T003 Escribir el test de la migración en
      `server/internal/integration/migracion_consola_test.go`, **antes** de la migración: que
      `platform_operators` nace vacía y sin `company_id`, que el rol `gatobobah_platform` existe y
      **no** es superusuario ni tiene `bypassrls`, y que `gatobobah_app` **no** puede leer
      `platform_operators`. Verlo fallar.
- [ ] T004 Escribir `server/migrations/0068_consola_de_plataforma.sql`: la tabla, el rol, los
      grants **explícitos** —`usage on schema public` más `select` sobre `companies` y
      `platform_operators`, y nada más— **y la política de RLS**
      `plataforma_lee_todas_las_empresas on companies for select to gatobobah_platform using (true)`.

      **La política no es opcional**: `companies` lleva `company_self`, RLS también le aplica al rol
      de plataforma, y sin ella la consola vería **una empresa de dos**. Lo encontró
      `/speckit-analyze` y está verificado contra datos reales. **Nunca `BYPASSRLS`** — resolvería
      esto y abriría todo lo demás.

      Dos comentarios que tienen que quedar en el archivo, porque son los atajos que alguien con
      prisa va a considerar: **nunca `grant ... on all tables` para este rol** (expondría `orders`,
      `users` y `order_payments` de golpe) y **el `Down` no borra el rol**.

- [ ] T004b Test de la política, **antes** de escribirla: con dos empresas en la base, el rol de
      plataforma ve **las dos**, y `gatobobah_app` sigue viendo **solo la suya**. Verlo fallar
      quitando la política — sin eso no se sabe si es ella la que abre o si algo más está mal.
- [ ] T005 Escribir el `Down`: `alter role gatobobah_platform with nologin` más los revokes de lo
      que otorgó el `Up`. **Sin `drop role`**, con el porqué en el comentario: los roles son objetos
      del servidor y si tienen permisos en otra base el `drop` falla — medido: pasa en CI y en
      producción (1 base) y truena en la VM de pruebas (2) y en local (5).
- [ ] T006 Test del `Down` en el mismo archivo: revertir no deja la tabla, deja el rol **sin poder
      entrar**, y volver a aplicar funciona. Es lo que evita descubrir el problema al revertir.

### Configuración y arranque

- [ ] T007 [P] Agregar `PLATFORM_JWT_SECRET` y `PLATFORM_DATABASE_URL` a `server/internal/config/`,
      con su validación en `config.Validate`: débil o ausente **no arranca**, y **igual a
      `JWT_SECRET` tampoco** — dos secretos iguales son un secreto y el fallo sería silencioso.
- [ ] T008 [P] Test de `config.Validate` en `server/internal/config/config_test.go` para los tres
      rechazos, incluido el de secretos iguales. Verlos en rojo.
- [ ] T009 Abrir el **tercer pool** en `server/cmd/api/main.go` con `PLATFORM_DATABASE_URL`, junto a
      los dos que ya existen.
- [ ] T010 `assertPlatformGrantsEnforced` en `main.go`, gemela de la `assertRLSEnforced` que ya
      está: aborta el arranque si el rol es superusuario o tiene `bypassrls`, y si un `select`
      canario sobre `orders` **no** falla con `42501`.

      Escenario que cierra: un `PLATFORM_DATABASE_URL` copiado de `DATABASE_URL` por descuido
      serviría con bypass total y nadie lo notaría hasta una auditoría a mano.

- [ ] T011 Test de integración de T010: con el rol correcto arranca; con el owner **no**.

### Las dos identidades

- [ ] T012 [P] `server/internal/domain/plataforma.go`: qué es un operador y sus sentinels. Puro,
      sin I/O.
- [ ] T013 [P] Tests de dominio en `plataforma_test.go`, en tabla.
- [ ] T014 Segundo `auth.Manager` firmado con `PLATFORM_JWT_SECRET`.
- [ ] T015 **El test que justifica todo el diseño**, en `server/internal/auth/`: un token emitido
      por un manager **no valida** en el otro, en las dos direcciones. Verlo en rojo usando un solo
      secreto para los dos.

**Checkpoint**: las barreras existen y están probadas. Recién aquí empiezan las historias.

---

## Fase 3: US1 — Entrar a la consola, y solo a la consola (P1) 🎯

**Meta**: dos superficies que se rechazan entre sí.

- [ ] T016 [US1] Tests de integración en
      `server/internal/integration/consola_separada_test.go`, **antes del código**: una credencial
      de plataforma no entra al POS; una de negocio no entra a la consola **aunque sea admin de su
      empresa**; un token de plataforma contra una ruta del negocio da 401 y al revés también.
- [ ] T017 [US1] `queries/platform.sql` con las consultas del operador, y `make sqlc`.
- [ ] T018 [US1] `app.PlatformService`: autenticar operador, rechazar inactivo.
- [ ] T019 [US1] `POST /api/v1/platform/auth/login` en `httpapi/handlers_plataforma.go`, con el
      grupo `/platform` y su middleware propio en `router.go`.
- [ ] T020 [US1] Limitador por IP y por cuenta en ese login, reusando el que ya existe.
- [ ] T021 [US1] Evento de seguridad en éxito y en fallo (FR-012), con el usuario intentado y
      **nunca** la contraseña.
- [ ] T022 [US1] Test de que un operador inactivo se rechaza **igual que uno inexistente**: misma
      respuesta y misma latencia.
- [ ] T023 [US1] Test de FR-003: una credencial de plataforma en el login del negocio responde como
      una inexistente **y tarda lo mismo**. Medirlo, no suponerlo — es la fuga por temporización que
      `auth.CheckDummySecret` ya cierra en el resto del login.
- [ ] T024 [US1] Bandera `-reset-platform-operator` en el binario para crear al primero desde
      variables de entorno (FR-014).

      **No copiar la forma de `-reset-admin`**: ése filtra solo por `username` y su propio
      comentario advierte que no está resuelto para multi-empresa. Aquí se busca en
      `platform_operators`, donde no hay empresa.

- [ ] T025 [US1] Test de que desactivar al operador le corta el acceso **sin esperar** a que caduque
      la sesión (FR-013).

**Checkpoint**: se entra a la consola y solo a la consola.

---

## Fase 4: US3 — Que el permiso cruzado no alcance al dinero (P1)

Va **antes** que US2 a propósito: es la barrera, y US2 es lo que se apoya en ella.

- [ ] T026 [US3] Test de integración **bajo el rol `gatobobah_platform`**: `select` sobre `orders`,
      `order_payments`, `register_sessions`, `expenses` y `users` da `42501`. Y sobre `companies`,
      un número.
- [ ] T026b [US3] Y que **tampoco puede escribir** (FR-009): `insert`, `update` y `delete` sobre
      `companies` dan `42501`.

      No es redundante con T026: los grants son de `select`, así que la escritura está denegada
      **por omisión** — y lo que se omite no se nota hasta que alguien lo agrega. Este test es lo
      que impide que la spec 018 ocurra por accidente en vez de por decisión.
- [ ] T027 [US3] Test de la simetría: `gatobobah_app` no puede leer `platform_operators`.
- [ ] T028 [US3] Verificar cada test de T026 **quitando el grant correspondiente** y viéndolo pasar
      a verde indebidamente. Un test de permisos que nunca se vio fallar no prueba que el permiso
      esté puesto.

---

## Fase 5: US2 — Ver qué empresas hay (P1)

- [ ] T029 [US2] Test de integración de `GET /api/v1/platform/companies`: lista **todas** las
      empresas —con dos en la base, salen las dos—, trae la versión de esquema **una sola vez**, y
      **no trae**: ninguna cifra de dinero (FR-008), ningún dato de empleados de un cliente (FR-016)
      ni la última actividad (FR-007b, que queda para la spec 017).

      Los tres «no trae» se afirman explícitamente y no se dan por buenos porque nadie los escribió:
      una respuesta se llena sola cuando alguien agrega un campo «de paso».
- [ ] T030 [US2] Implementar el endpoint.
- [ ] T031 [US2] Test de cero empresas: la respuesta lo dice y la pantalla no pinta una tabla vacía
      (FR-015).

---

## Fase 6: US4 — Que la tableta no cargue nada de esto (P1)

**El orden importa: la barrera automática ANTES que el código de la consola.** Si se escribe al
revés, el primer import cruzado entra sin que nada lo detenga.

- [ ] T032 [US4] Regla en `web/eslint.config.js`, dentro del bloque `FRONTERAS` que ya existe:
      nadie importa `**/consola/**` desde fuera de `src/consola/`.
- [ ] T033 [US4] Verificar que **muerde**: agregar un import cruzado de prueba y ver `bun run lint`
      en rojo. Sin esto, la única barrera contra contaminar la tableta podría estar mal escrita y
      nadie se enteraría.
- [ ] T034 [US4] Separar los `tsconfig`: el del POS excluye `src/consola`, y la consola tiene el
      suyo.

      Escenario que cierra: `bun run build` es `tsc --noEmit && vite build` y es de lo que dependen
      los despliegues del POS. Un typo en la consola dejaría **varado un arreglo urgente de cobro en
      tableta**.

- [ ] T035 [US4] `web/vite.consola.config.ts` con entrada propia, **`outDir: 'dist-consola'`** y
      **`publicDir` propio**.

      Los dos son escenarios concretos: sin `outDir` el segundo build **borra** el `dist/` del POS
      justo antes de que CI lo suba a Pages; sin `publicDir` la consola queda instalable como PWA
      con el nombre y el logo del restaurante.

- [ ] T036 [US4] La pantalla de la consola en `web/src/consola/`: login y lista de empresas. Sin
      dependencias nuevas.
- [ ] T037 [US4] Medir el paquete del POS y compararlo con el **antes**: chunk principal 1,133.50 kB
      el 2026-09-11. Si creció, la consola se coló.

---

## Fase 7: Despliegue y cierre

- [ ] T038 Agregar a `ci.yml` el build y el despliegue de la consola a sus dos proyectos de Pages,
      **sin tocar** los pasos del POS.
- [ ] T039 [P] Documentar el ambiente nuevo en `AGENTS.md` (subdominios, variables) y el control en
      `docs/security-owasp.md`, que es el respaldo del principio V.
- [ ] T040 [P] Renglones en `docs/matriz-de-pantallas.md`: qué cubre cada test de aislamiento y **qué
      no**.
- [ ] T041 Correr [quickstart.md](./quickstart.md) completo, incluidos los pasos que exigen ver una
      barrera fallar.
- [ ] T042 Gates completos: `make api-build`, `api-test`, integración, `make lint`, `make vuln`,
      `bun run lint`, `bun run vitest run`, `bun run build`, `bun audit --audit-level=high`.

---

## Dependencias

- **Fase 2 bloquea todo.** Sin las barreras no hay nada que probar.
- **US3 antes que US2**: la barrera antes de lo que se apoya en ella.
- **T032 y T033 antes que T036**: la regla de importación antes del código que podría cruzarla.
- **T003 antes que T004**, y **T005 antes que T006**: la migración con su test antes, que además el
  hook de pre-commit exige.
- T007–T015 son [P] entre sí: archivos distintos.

## Estrategia

Las cuatro historias son **P1**, y eso no es un descuido del spec: sin US1 y US3 no hay separación,
sin US2 no hay nada que mirar y sin US4 la consola viaja en la tableta. El corte más chico que tiene
sentido desplegar es **fases 2, 3 y 4** —las barreras y su prueba—, aunque todavía no haya pantalla.

**Dónde parar y validar**: al terminar la fase 4, correr los pasos 1 a 7 del quickstart. Si alguna
barrera no se puede ver fallar, no seguir: significa que el test no prueba lo que dice.
