# AGENTS.md — El Gato Bobah POS

Cara **operativa** del repo: cómo se corre, con qué se construye y qué quirks tiene el tooling.
Vendor-neutral (estándar [agents.md](https://agents.md)); las herramientas de IA (Claude Code,
Codex, Cursor) leen esto.

**Los principios de ingeniería NO viven aquí.** Viven en
[`.specify/memory/constitution.md`](.specify/memory/constitution.md) — layering, errores, dinero,
testing, seguridad, YAGNI y comentarios — y ese archivo es la única copia. Este documento lo
importa para que estén siempre en contexto:

@.specify/memory/constitution.md

> Si tu herramienta no resuelve imports `@`, abre
> `.specify/memory/constitution.md` ahora, antes de escribir código. Y si encuentras una regla
> repetida en los dos archivos, bórrala de aquí: la constitución manda.

## 1. Proyecto y stack

POS propio para un solo local (reemplaza a FUDO). Monorepo:

- **`server/`** — Go 1.27 · chi · pgx + **sqlc** · goose (migraciones **embebidas**) · Redis.
- **`web/`** — React 19 · Vite · Chakra UI v3 · TanStack Query · Zustand. Bun, **nunca npm**.
- **`deploy/`** — docker-compose + Caddy (TLS, headers de seguridad).
- **`web/src/consola/`** — la **consola de plataforma** (spec 016): otro producto, para quien VENDE
  el sistema. Otro build (`bun run build:consola` → `dist-consola/`), otro proyecto de Pages, otro
  dominio (`staff.elgatobobah.com`, `staff-dev` en pruebas) y otro `tsconfig`. **No comparte código
  con el POS**, y `eslint.config.js` lo impone en las dos direcciones: su peso no viaja a la tableta
  y un error suyo no bloquea un deploy del mostrador. En el backend cuelga de `/api/v1/platform`,
  firma con `PLATFORM_JWT_SECRET` y se conecta con el rol `gatobobah_platform`, que solo puede leer
  `companies` y `platform_operators` — las barreras y sus pruebas están en
  [docs/security-owasp.md](docs/security-owasp.md).
- **`specs/`** — un directorio por feature (`NNN-slug/`), generado por spec-kit.
- **`docs/`** — referencia viva, histórico y fixtures; el índice manda ([docs/README.md](docs/README.md), ver §6).
- **`http/`** — peticiones a las APIs de las plataformas, en texto plano, para la extensión
  **REST Client** de VS Code (`humao.rest-client`, recomendada en `.vscode/extensions.json`). Son
  **solo lectura a propósito**: el `PUT` de menú de Uber reemplaza el menú publicado completo y no
  hay deshacer, así que ningún endpoint que escriba vive en un archivo que se dispara con un clic.
  Las credenciales van en `.vscode/settings.json` —ignorado por git— copiado de
  [.vscode/settings.json.example](.vscode/settings.json.example).
- **`references/` YA NO EXISTE EN EL REPOSITORIO**, y es una regla, no un accidente. Los exports de
  FUDO traen el **costo de compra de cada insumo**, el **nombre de cada proveedor**, las **ventas por
  día y por hora** y los **gastos por categoría, incluido el pago a colaboradores**. Eso es la
  estructura de márgenes del negocio, y este repositorio es **público**. Viven en
  `~/gatobobah-datos/references/`; `make fudo-import` los lee de ahí y falla con un mensaje claro si
  no están. Se apunta a otro lado con `FUDO_DIR=... make fudo-import`.

  **La regla general, que aplica a cualquier cosa que se quiera agregar:**

  | Va en el repositorio | No va, nunca |
  | --- | --- |
  | Código, esquema, pruebas | Exports, respaldos, reportes de ventas o de gastos |
  | El **porqué** de una decisión | Los **números del negocio** que la motivaron |
  | Hechos del mundo — «Uber cobra 30%», «la API exige NDA» | Cuánto vende el negocio, cuánto le cuesta un insumo, a quién le compra |
  | Un techo medido del sistema — «la tabla pesa 16.5 MB» | Documentos fiscales, nómina, datos de clientes |

  La vara: **si el dato sigue siendo cierto en otro restaurante, va; si solo es cierto en éste, no.**
  Un documento que necesite las dos mitades cita la de afuera sin copiarla — como hace
  [docs/plataformas-digitales.md](docs/plataformas-digitales.md), que documenta las comisiones
  medidas y deja los CFDI fuera del repo a propósito.

### Listas filtradas, ordenables y paginadas

El patrón ya está resuelto y se **copia**, no se reinventa. Referencias: `ListExpenses`/`CountExpenses`
en [server/queries/expenses.sql](server/queries/expenses.sql) y las cinco de
[server/queries/sales.sql](server/queries/sales.sql).

- Filtro opcional = `sqlc.narg('x')`, nunca SQL concatenado.
- Orden por columna = `case when @sort::text = … and @dir::text = …`, con **whitelist en el dominio**
  y su espejo de tipos en el front ([SortHead](web/src/components/SortHead.tsx)). Un `sort`
  desconocido se rechaza; ignorarlo deja la tabla ordenada por algo distinto de lo que dice su
  encabezado.
- Toda lista paginada trae su `Count…` gemela **con el mismo `where`**, y ese `where` y el del
  resumen viven en el mismo archivo y se editan juntos.
- **El índice de soporte empieza por `company_id`**: RLS agrega ese predicado a toda consulta del rol
  `gatobobah_app`, y un índice que arranca por la fecha se queda descartando filas de otras empresas
  dentro del scan. Ver [0042](server/migrations/0042_sales_index.sql).
- **Un agregado no se une a dos tablas 1:N en la misma consulta.** `order_payments` y `order_lines`
  son las dos 1:N con `orders`: unirlas multiplica las filas (2 pagos × 3 líneas = 6) y duplica las
  sumas. Se pre-agrega cada rama por `order_id`, o se hacen consultas separadas.
- **Un agregado que se puede vaciar va con `coalesce`, y el cast NO lo salva.** `string_agg`,
  `array_agg`, `sum`, `min`, `max` sobre un conjunto **vacío** devuelven NULL. Escribir
  `(select string_agg(...) ...)::text` no lo arregla —un `NULL::text` sigue siendo NULL— pero sí
  hace que **sqlc tipe la columna como no nulable** (`string` en vez de `*string`), y entonces el
  scan revienta con `cannot scan NULL into *string`. Lo grave no es el NULL: **no falla ese renglón,
  falla la consulta entera**. Costó que `/catalogo/opciones` respondiera 500 en producción durante
  horas porque 21 grupos de modificadores no tenían opciones activas — un estado ordinario: es el de
  un grupo recién creado. Ver `AdminListGroups` en [modifiers_admin.sql](server/queries/modifiers_admin.sql).
  `count()` es la excepción: sobre conjunto vacío da 0, no NULL.
- **Un slice nil de Go sale como `null`, no como `[]`, y eso tumba una pantalla sin un solo error en
  el servidor.** `json.Marshal` de un `[]T` nil escribe `null`; de un `[]T{}` escribe `[]`. Un mapa
  al que se le pide una llave ausente devuelve el cero del valor —nil— así que
  `Lines: porPedido[r.ID]` es nil en cuanto ese pedido no tiene renglones vivos.

  Costó la pantalla de pedidos de producción el 2026-09-13: un pedido cuya única línea se canceló
  llegó con `lines: null`, el tablero hace `o.lines.filter(...)` al pintar cada tarjeta, y el
  mostrador se quedó con «La pantalla no se pudo mostrar» **con la API respondiendo 200 y sin una
  sola línea de error en el log**. No falla nada: se entrega un contrato distinto del prometido, y
  el 5xx que uno busca no existe.

  La regla: **toda respuesta que prometa un arreglo lo entrega, aunque esté vacío.** Se normaliza
  antes de devolver —ver `conRenglones` en [orders.go](server/internal/app/orders.go)— y el test va
  sobre el **JSON crudo**, porque deserializar a una estructura de Go borra justo la diferencia
  entre `null` y `[]`. Del lado del front, el campo se declara **opcional** para que el compilador
  obligue a la guarda; un test estático se olvida de un archivo nuevo, `tsc` no.
- **sqlc NO conoce `company_id`** en las ~30 tablas a las que se lo agregó
  [0023](server/migrations/0023_tenant_columns.sql) con `EXECUTE format()`: su parser no lee DDL
  dinámico. Nombrar esa columna en una consulta rompe `sqlc generate` con "column does not exist"
  por una columna que sí existe en Postgres. No hace falta: RLS la aplica sola.

## 2. Comandos (todos en `Makefile`; `make help` los lista)

- `make install` — setup completo (valida entorno, instala deps y herramientas). `make check` solo verifica prereqs.
- `make start` — levanta todo: Postgres (default **:5490**), Redis (**:6390**), mailpit (**:8095**/**:1095**), API (default **:8080**) y web (Vite default **:3000**). **Ningún puerto es fijo**: un default, por raro que sea, choca con el postgres/redis de otro compose local. `start.sh` reusa el puerto que ya publica el contenedor vivo o toma el primero libre desde el default, y exporta `PG_PORT`/`REDIS_PORT`/`MAILPIT_*` para el compose y `dev-api.sh`; el `Makefile` los lee igual (`PG_PORT=… make db-migrate`). Pregunta los puertos de API/web (defaults auto-ajustados al primer libre) y **detecta puertos ocupados** antes de levantar. Fíjalos sin preguntar con `BACKEND_PORT=… FRONTEND_PORT=… make start`. El binario Go lee `PORT`; `vite.config.ts` lee `BACKEND_PORT` (proxy `/api`) y `FRONTEND_PORT`.
- **Varios worktrees a la vez: cada uno con su propia infraestructura** (decidido el 2026-10-12).
  `docker compose` toma el nombre del proyecto de la carpeta del compose (`deploy/`), que es la
  misma en todos los worktrees: sin esto, cada `make start` cae en los MISMOS `deploy-postgres-1` y
  `deploy-redis-1`. Lo que costaba, medido: una rama migra la base de otra (dos ramas con la misma
  migración 0089 dejan a la otra API sin arrancar), el caché del menú y los bloqueos de login se
  comparten en Redis, y las pruebas de integración —que fijan las contraseñas de los roles de TODO
  el servidor de Postgres— dejaron «db_down» la API local que usaba ese servidor.

  **Cada feature arranca en su propio worktree, con un solo comando** (decidido el 2026-10-17). Desde
  cualquier worktree (**no desde `~/git/el-gato-bobah-pos`**: es un repositorio bare, solo guarda la
  historia y no tiene `Makefile`): **`make nuevo-worktree n=041 slug=subidas-sin-corte`**
  ([nuevo-worktree.sh](scripts/nuevo-worktree.sh)). Crea `../egb-041-subidas-sin-corte` con la rama
  `041-subidas-sin-corte` desde `origin/develop`, corre `worktree-env`, crea `deploy/.env` con
  secretos nuevos y `POSTGRES_PASSWORD=gatobobah` (la del compose de desarrollo), instala las
  dependencias de `web/` (`node_modules` no viaja con el worktree), levanta `make start` en segundo
  plano (log en `/tmp/egbNN-start.log`), espera `/readyz` y crea el admin con contraseña y PIN
  nuevos que imprime una sola vez: no quedan en ningún archivo. Arrancar la 041 a mano costó cinco
  vueltas, cada error visible solo al fallar el anterior. Al terminar la feature:
  **`make cerrar-worktree`** desde dentro de ese worktree, o `make cerrar-worktree dir=../egb-041-…` desde
  otro ([cerrar-worktree.sh](scripts/cerrar-worktree.sh); `datos=1` borra también sus volúmenes). Baja
  también el Postgres de pruebas (perfil `test`). Se niega con cambios sin commit o commits que no
  están en origin, y nunca toca el proyecto `deploy` de la carpeta principal. Lo cubre
  [cerrar-worktree-test.sh](scripts/tests/cerrar-worktree-test.sh). La skill `/nuevo-spec` los usa.

  Lo que hace por dentro, si hay que hacerlo a mano: antes del primer `make start`, **`make worktree-env n=NN`**, con NN el número
  de la feature (035 → 35). Escribe `.env.worktree.local` (ignorado por git), que leen el Makefile,
  `start.sh`, `stop.sh`, `dev-api.sh` y `ci-local.sh`:

  | Pieza | Puerto | Nota |
  | --- | --- | --- |
  | Proyecto de compose | `egbNN` | Contenedores y volúmenes propios |
  | Postgres de la API | 5500+NN | |
  | Postgres de pruebas | 5600+NN | Servicio `postgres-test` (perfil `test`, en memoria); lo usa `make test-integration` |
  | Postgres de `make ci-local` | 5700+NN | Antes fijo en 5510: dos `ci-local` a la vez chocaban |
  | Redis | 6400+NN | |
  | API · web | 8000+NN · 3000+NN | |
  | mailpit (web · SMTP) | 8200+NN · 1100+NN | |

  Reglas:
  - **Las credenciales son las del `deploy/.env` de ese worktree**; si no existe, `make check-env`
    lo crea con contraseñas nuevas. Como cada worktree tiene su propio servidor de Postgres, los
    roles y sus contraseñas ya no se pisan.
  - **Las pruebas de integración van contra el Postgres de pruebas, nunca contra el de la API**:
    `make test-integration`, no `TEST_DATABASE_URL=<la base de la API>`.
  - **Un worktree que ya trabaja se cambia solo entre tareas**, nunca a media corrida de pruebas:
    cambiar el proyecto de compose deja su base anterior fuera de su alcance (los datos siguen en el
    contenedor viejo; se traen con `make db-restaurar` si hacen falta).
  - **La carpeta principal no se toca**: el script se niega a correr ahí, y sin el archivo todo
    sigue con los puertos de siempre (5490, 6390, 8080, 3000). Producción, por supuesto, tampoco.
  - **Nunca `docker compose down` (y menos `down -v`) directo en la terminal: siempre `make stop` o
    `make deps-down`.** El proyecto de cada worktree vive en `.env.worktree.local`, que solo cargan
    el Makefile y los scripts; un `docker compose` a mano cae en el proyecto `deploy`, el de la
    carpeta principal. Pasó el 2026-10-12: un `down -v` así borró `deploy_pgdata_dev` y hubo que
    restaurarla de un respaldo. `make stop` y `make deps-down` se niegan a bajar `deploy` desde un
    worktree ([guard-shared-compose.sh](scripts/guard-shared-compose.sh)).
  - **`make db-restaurar` restaura en la base del worktree, y desde un worktree se niega a tocar la
    de la carpeta principal**, aunque se le pase `PG_CONTAINER=deploy-postgres-1`. Pasó el
    2026-10-13: el script tenía el contenedor fijo y un restore desde un worktree borró la base de
    la carpeta principal. Lo vigila
    [restaurar-respaldo-test.sh](scripts/tests/restaurar-respaldo-test.sh), que corre en el
    pre-commit cuando cambia el script. Para restaurar la carpeta principal, se corre desde ahí.
- `make api-dev` (hot reload con air) · `make api-build` (= `cd server && go build ./...`) · `make api-test` (= `cd server && go test ./...`).
- **La base local se comporta como producción: RLS y GRANT aplican también en dev.** La regla vive
  en la constitución (IV, *La base local niega lo mismo que producción*); esto es la mecánica, y
  **no se afloja para que algo "funcione en local"**.

  | Pieza | Cómo queda en local |
  | --- | --- |
  | Migraciones y bootstrap | Como owner (`DATABASE_URL`), igual que en producción |
  | Requests del POS | Como `gatobobah_app` (`APP_DATABASE_URL`), sujeto a RLS |
  | Consola de plataforma | Como `gatobobah_platform` (`PLATFORM_DATABASE_URL`), solo sus GRANT |
  | Passwords de los dos roles | `APP_DB_PASSWORD` y `PLATFORM_DB_PASSWORD` en `deploy/.env`, obligatorios también en dev (`make check-env` los exige y los genera al crear el archivo) |

  [scripts/dev-api.sh](scripts/dev-api.sh) arma las dos URLs y **se niega a arrancar** si falta un
  password; el bootstrap se los fija a los roles, y el arranque corre `assertRLSEnforced` y
  `AssertPlatformGrants` igual que allá. Se sobreescriben con `DEV_APP_DATABASE_URL` /
  `DEV_PLATFORM_DATABASE_URL`, pero apuntarlas al owner es justo lo prohibido.

  **Datos de producción en local: `make db-restaurar`** (el `.dump` más reciente de
  `backups/prod/`, o `dump=…`). Detén la API antes: recrea la base entera. Restaura **con**
  dueños y GRANT y termina comprobando que `gatobobah_app` puede leer `orders` sin saltar RLS. Los
  dos flags que "hacen que el restore no se queje" están prohibidos:
  - `--no-privileges`: el rol de servicio queda sin un solo GRANT y la API no arranca.
  - `--no-owner`: `candidatas_del_aviso` (0073) pasa a ser del owner en vez de `gatobobah_webhook`,
    y el webhook resuelve tiendas con un bypass que en producción no existe.

  **Cómo se ve si alguien lo rompe**: datos de las dos empresas mezclados en una pantalla. Pasó el
  2026-09-25 con el selector de plataformas, que salió con cada una dos veces: la consulta
  (`ListPlatformsWithMarkup`) no filtra por empresa porque ese filtro lo pone RLS. **Un `psql -U
  gatobobah` a mano también es owner**: para ver lo que ve la API, `set role gatobobah_app; set
  app.company_id = '2';` antes de la consulta.
- `make web-dev` · `make web-build` · `make web-test` (vitest).
- **Toda prueba corre en local** (constitución 1.25.0; el ambiente de pruebas solo si el dueño lo pide). El subagente
  usa `http://localhost:30NN` del worktree (el `FRONTEND_PORT` de `.env.worktree.local`), con
  `make start` arriba y una base restaurada (`make db-restaurar`), a 1024×600 y `hasTouch`. Corrige,
  recarga (Vite lo hace solo) y vuelve a probar sin PR ni despliegue. La vuelta final corre además
  contra el build servido en local (`bun run build && bun run preview`).
- **`bun run e2e`** (en `web/`) — Playwright a 1024×600 contra el **`make start` del worktree**:
  `E2E_BASE_URL=http://localhost:30NN E2E_API_URL=http://localhost:80NN/api/v1 E2E_SLUG=… E2E_PASSWORD=… E2E_PIN=… bun run e2e`.
  `E2E_PIN` es el PIN de `E2E_USER`: con la asistencia encendida, las pruebas de caja marcan su
  entrada y su salida ([asistencia.ts](web/e2e/asistencia.ts)) y sin él fallan diciendo que falta.
  Sin esas variables apunta al ambiente de pruebas, que se usa solo cuando el dueño lo pide. No
  simula el backend a propósito: lo que estas pruebas atrapan es el desacuerdo entre lo que la
  pantalla calcula y lo que el servidor cobra, y con un backend simulado los dos están de acuerdo por
  construcción; la API local es la real. En Windows los binarios del navegador los bloquea Smart App
  Control, así que corre en contenedor como el resto de los gates:

  ```bash
  MSYS_NO_PATHCONV=1 docker run --rm --network host -v "d:/git/el-gato-bobah-pos/web:/w"     -v gatobobah_e2e_modules:/w/node_modules -w /w -e CI=1     mcr.microsoft.com/playwright:v1.64.1-noble     sh -c "bun install --frozen-lockfile --silent; npx playwright test"
  ```

  Un fallo en el primer `goto` casi siempre es la VM spot apagada, no el código: revísalo antes de
  buscar el defecto. Los casos y su porqué están en dos matrices que se editan junto con los tests:
  [docs/matriz-de-cobro.md](docs/matriz-de-cobro.md) (por dónde se pierde dinero) y
  [docs/matriz-de-pantallas.md](docs/matriz-de-pantallas.md) (por dónde una pantalla dice algo que
  no es cierto). Las dos declaran también lo que **no** está cubierto.

  **El login se comparte, y no es cosmética.** `/auth` está limitado a 60 peticiones por minuto y
  por IP, y la suite lo tumbaba sola: cinco archivos tenían su propia copia del helper de login y
  entraban en cada test. Medido contra el ambiente de pruebas, una corrida dejó **8 respuestas
  429**, y lo que se ve es un test esperando 30 segundos a una pantalla que nunca entra —
  intermitente, y que pasa al reintentar. El token vive en
  [ambiente.ts](web/e2e/ambiente.ts) (`tokenDeApi` / `tokenDeRequest`), cacheado 5 minutos. Si
  agregas un spec, úsalo: volver a escribir el login reintroduce la intermitencia y el gate deja de
  creerse.

  **La suite COBRA los pedidos que crea.** El ambiente es compartido con una persona, y un pedido de
  prueba que se queda abierto aparece en la barra del POS, suma a "por cobrar" y bloquea el cierre de
  caja — le hace creer a quien opera que hay dinero pendiente. El `globalSetup` anota qué pedidos ya
  estaban abiertos y el `globalTeardown` cierra solo los que la suite abrió
  ([web/e2e/limpiar-lo-que-cree.ts](web/e2e/limpiar-lo-que-cree.ts)); sin esa marca no toca nada,
  porque cerrarle a alguien una cuenta viva es peor que dejar basura. Si escribes un script suelto
  que cree pedidos, ciérralos tú: entregar (`POST /orders/:id/deliver`) y cobrar
  (`POST /orders/:id/pay`, **no** `/charge`), y un pedido de plataforma solo acepta el método de SU
  plataforma.

  **Contra la base local, la suite abre la caja principal si la encuentra cerrada y la cierra al
  terminar** ([caja-principal.ts](web/e2e/caja-principal.ts)): un respaldo restaurado la trae
  cerrada, y las pruebas de caja dan por hecho un turno abierto. Las personas que una prueba
  necesita las crea y las da de baja ella ([personas.ts](web/e2e/personas.ts)); ningún id ni nombre
  de la base es fijo.

  **Las capturas de pantalla no se versionan; las e2e las guardan en `test-results/`** (ignorado por
  git). Sin excepciones, con datos reales o inventados: en el repo no tienen uso, y lo observado en una
  prueba se escribe en texto. Ningún spec, plan ni task pide guardarlas en `specs/` ni en otra carpeta
  del repo. Una prueba corrida contra una base restaurada pinta el catálogo, las existencias y los
  costos del negocio, y este repositorio es público. Pasó el
  2026-10-22: el recorrido de la 044 escribía por omisión en `specs/044-…/capturas/`, se versionaron
  29 PNG con costos de compra reales y hubo que reescribir la historia de 12 ramas, `main` incluida.
  Una e2e nueva toma su carpeta de capturas de una variable de entorno con `test-results/` por
  omisión, como las que ya existen.
- **`make ci-local`** — lo mismo que los jobs de CI que gatean el despliegue (backend con lint y govulncheck, integración con `sqlc diff`/`sqlc vet` contra un Postgres desechable en `:5510`, y frontend), en paralelo y en ~2 min. **Ya no es paso obligatorio** (decidido el 2026-10-21): cuando termina el flujo de spec-kit de una feature, se empuja y el CI del PR es el gate —un PR no se mergea sin CI verde—. Mientras se implementa, los commits se quedan locales. **Si el CI del PR falla, las correcciones se validan en local**: se arregla, se corre `make ci-local` y solo con verde se vuelve a empujar, en vez de una vuelta de CI por intento. Medido el 2026-10-21 sobre las últimas 10 corridas: el CI de un PR tarda de 2 a 5 min y el de develop (imagen y despliegue al ambiente de pruebas) de 2 a 3 min. El pre-push de lefthook sigue corriendo (`go test`, `govulncheck`, `bun run build`). No cubre el entorno de GitHub (versiones de las actions, el Go que instala `setup-go`), que es justo lo que se rompió el día de go1.27.2; CI sigue siendo el que despliega.
- `make lint` (golangci-lint + gosec) · `make vuln` (govulncheck) · `make web-lint` (eslint + tsc) · `make sec` (todos).
- **Notas por área: se leen ANTES de tocar esa área** (movidas aquí el 2026-11-07: con ellas dentro,
  las instrucciones que carga cada sesión pasaban de 150 mil caracteres). Cada archivo junta lo que
  no se ve en el código y ya costó un defecto. Una feature nueva anota lo suyo en el archivo de su
  área (o en uno nuevo, con su renglón aquí), no en este documento:

  | Área | Antes de tocar… | Archivo |
  | --- | --- | --- |
  | Plataformas de reparto | precios por plataforma, emparejamiento, menús publicados, pedidos crudos y credenciales | [docs/mecanica/plataformas.md](docs/mecanica/plataformas.md) |
  | Catálogo y sucursales | sucursales, versión del menú y cambios de catálogo por SQL | [docs/mecanica/catalogo-y-sucursales.md](docs/mecanica/catalogo-y-sucursales.md) |
  | Almacén, recetas y costo | existencias, recetas y paquetes, costo vigente, compras, conteo, merma, lugares, artículos, unir insumos, nombres alternativos, densidad | [docs/mecanica/almacen.md](docs/mecanica/almacen.md) |
  | Gastos y sus pagos | pagos de gasto en efectivo, arqueo por hora, renglones excluidos, editar y anular gastos, lo que salió de la caja | [docs/mecanica/gastos.md](docs/mecanica/gastos.md) |
  | Caja y cobro | dividir la cuenta, terminales de tarjeta, fondo fijo y retiro, abrir y cerrar caja con asistencia | [docs/mecanica/caja-y-cobro.md](docs/mecanica/caja-y-cobro.md) |
  | Personal, permisos y bitácora | roles y permisos por empresa, asistencia y bitácora de auditoría | [docs/mecanica/personal-y-permisos.md](docs/mecanica/personal-y-permisos.md) |
  | Medición de uso y mapa de toques | instrumentar pantallas para la consola de plataforma | [docs/mecanica/medicion-de-uso.md](docs/mecanica/medicion-de-uso.md) |
- **`company_id = 1` NO es El Gato Bobah.** Es **«Bobah Pruebas»**, con su propio catálogo muy
  parecido al bueno; el negocio real es **`company_id = 2`, slug `gatobobah`**. Filtrar por el id
  «porque es el primero» devuelve un catálogo plausible y equivocado —172 productos en vez de 174,
  cero excepciones de precio en vez de 7, y un catálogo que parece llevar meses quieto cuando se
  editó anteayer— **sin que nada falle**. En toda consulta manual, filtra por `slug`.
- **Toda columna nueva que no sea número, fecha, booleano o enum se clasifica para el respaldo
  anónimo** (#133, #134). `TestEveryColumnIsCoveredByTheAnonymousBackup` recorre el catálogo de la
  base migrada: cada columna de texto, json, binario, IP, uuid o arreglo la borra el paso 5 de
  `scripts/respaldo-anonimo.sh` (y la cuenta su verificación `FUGAS`), o está en `noPersonalData`
  con el porqué. Una que no está en ninguno truena en la integración, se haya creado con
  `create table` o con `alter table`. El paso que borra corre después de migrar, y si algo falla
  antes de que `FUGAS` diga 0, el script tira la base restaurada entera.
- **Un Down que se niega con datos lleva su limpieza de prueba** (#116). Las pruebas de migración
  bajan un respaldo real hasta antes de su migración, y con datos de producción cada guarda de
  Down (`raise exception`) que cruzan se niega: ocho pruebas de la 0064 a la 0082 morían en la 0088
  por los cobros con terminal. `migrarAbajoHasta` baja una migración a la vez y antes de cada Down
  aplica su renglón de `guardedDownCleanup` (en
  [harness_restaurado_test.go](server/internal/integration/harness_restaurado_test.go)): borra o
  pone en neutro exactamente lo que nombra la guarda, solo en la copia de prueba.
  `TestEveryGuardedDownHasItsTestCleanup` truena si un Down nuevo se niega y no tiene su renglón, o
  su razón en `guardedDownWithoutCleanup` (hoy solo la 0076). Un Down que pierde un dato que su Up
  no vuelve a aceptar también lleva renglón aunque no se niegue (la 0072, con los descuentos). Cada
  prueba trabaja sobre su propia copia de `TEST_RESTORED_DATABASE_URL`, así que la base restaurada
  no cambia entre pruebas ni entre corridas.
- **Consola de plataforma** (en `web/`): `bun run dev:consola` (puerto 3100, con proxy a la API),
  `bun run typecheck:consola`, `bun run build:consola`. Van aparte de los del POS a propósito: un
  typo en la consola no puede dejar varado un arreglo de cobro en tableta. El primer operador se
  crea con `PLATFORM_OPERATOR_USERNAME=… PLATFORM_OPERATOR_PASSWORD=… ./api -reset-platform-operator`
  (la tabla nace vacía: una credencial sembrada por una migración vive en este repositorio, que es
  público).
- **En producción no se prueba; después de subir solo se revisa la salud** (decidido el
  2026-10-17). Las pruebas —e2e, quickstart, usuario sin contexto, scripts que escriben— corren en
  local, y en el ambiente de pruebas (`app-dev`/`api-dev`) solo si el dueño lo pide; repetirlas en producción es
  redundante y escribe en los datos de un negocio que está operando. Tras una subida a `main`, lo que
  se revisa es de solo lectura y sin sesión:
  - `GET https://api.elgatobobah.com/healthz` → `{"status":"ok"}` (el proceso vive) y
    `GET /readyz` → `{"status":"ready"}` (además alcanza la base; 503 `db_down` si no).
  - El job de despliegue de CI en verde: la imagen lleva el SHA del commit, así que eso dice qué
    versión corre. (`/api/v1/version` también lo dice, pero pide sesión.)
  - Los logs de la API en el journal, sin errores ni 5xx desde la subida.
  Antes de subir se toma el respaldo (`/usr/local/bin/respaldo-gatobobah.sh` en la VM): solo lee la
  base y guarda una copia aparte; no la detiene ni la sobrescribe.
- `make deploy-image` — deploy del backend **sin compilar**: baja de ghcr.io la imagen que publicó CI y la sube **azul/verde** con [deploy-azul-verde.sh](scripts/deploy-azul-verde.sh) (spec 041): la nueva arranca junto a la vieja, Caddy cambia de lado con un reload cuando la nueva responde `/readyz`, y la vieja termina lo que traía. Si la nueva no queda lista, el tráfico no se mueve y el deploy falla. Es lo que corre el VPS. `make deploy` (compila local) queda como fallback si CI está caído y pasa por el mismo script. Runbook: [docs/despliegue-sin-corte.md](docs/despliegue-sin-corte.md).
  - **Nunca `docker compose -f deploy/docker-compose.yml up -d` sin nombrar servicio**: levanta los dos lados de la API.
  - **Una migración tolera a la versión anterior corriendo** (constitución IV): durante la subida las dos versiones usan la misma base.
- **La bitácora de la API se guarda 30 días en el journal de la VM**, no en el archivo del
  contenedor: ése se borra en cada deploy y llegaba a ~5 días. `make deploy-image` instala
  [deploy/journald.conf](deploy/journald.conf) (necesita `sudo` sin password en la VM). Ahí queda la
  IP y el usuario de cada request; se lee con
  `sudo journalctl SYSLOG_IDENTIFIER=api --since "…" -o cat`, **en hora UTC** — los dos lados (azul y
  verde) escriben con esa etiqueta. Lo anterior a la spec 041 está bajo `CONTAINER_NAME=deploy-api-1`. Lo demás
  (postgres, redis, caddy) sigue en el archivo del contenedor: cambiarles el driver los recrea, y
  postgres se reinicia a media operación.
- `make sqlc` (regenera código de queries) · `make migrate-new name=xxx` (nueva migración goose).
- **Frontend siempre con bun** (`bun install`, `bun run`, `bun audit`). `web/package.json` bloquea npm (`preinstall: only-allow bun`). Nunca crees `package-lock.json`.

## 3. Dependencias y supply chain

Que un CVE bloquea el merge es principio (constitución, *Restricciones del producto*). Esto es la
mecánica:

- **Frescura ≠ pre-commit.** Actualizar deps al día lo maneja **Dependabot** ([.github/dependabot.yml](.github/dependabot.yml)): github-actions, gomod (`/server`), **bun** (`/web`, no `npm`: es lo único que actualiza `bun.lock`) y docker (`/server`+`/deploy`), semanal. Chequeo manual: `go list -u -m all` (Go), `bun outdated` (web). **No** añadas un pre-commit de "deps desactualizadas" (ruidoso, bloquea cambios ajenos).
- **Dónde corre cada scanner**: Go → `govulncheck` (pre-push lefthook **+** CI). Frontend → `bun audit --audit-level=high` en CI, sin `|| true`.
- **Runtimes**: con línea LTS → la última LTS (Node = 24, `.nvmrc`); Go no tiene LTS → el último minor estable (hoy `go1.27.3`).
- **Pin fuerte**: imágenes base por **digest** (`server/Dockerfile`, `deploy/docker-compose.yml`), GitHub Actions por **SHA** ([ci.yml](.github/workflows/ci.yml)), toolchain Go fijado en `go.mod` (hoy `go 1.27.3`). **No agregues una línea `toolchain` igual al `go` directive**: `go mod tidy` la borra por redundante en cada corrida — cuando coinciden, el `go` directive ES el pin.
- **El backend NO se compila en el VPS.** La VM es un e2-micro (1 vCPU, 1 GB): compilar el módulo
  ahí tarda ~30 min y puede morir por OOM — se vio al subir a Go 1.27, que invalidó todas las capas
  cacheadas. El job `image` de [ci.yml](.github/workflows/ci.yml) construye y publica
  `ghcr.io/ramsesmartinez/el-gato-bobah-pos/api` y `deploy-backend` solo entra por SSH a bajarla
  (`make deploy-image`). Detalles que importan:
  - **Es gratis**: el repo es público → minutos de Actions ilimitados y paquetes públicos en ghcr.io
    sin costo. Publicar usa el `GITHUB_TOKEN` del workflow con `packages: write` **solo en ese job**;
    no hace falta un PAT ni federación OIDC con GCP (eso solo aplicaría si se publicara en Artifact
    Registry, o para cambiar el `VPS_SSH_KEY` por credenciales efímeras).
  - El paquete **hereda la visibilidad del repo**: al crearlo el workflow de un repo público queda
    público solo, y el VPS lo baja sin credenciales (verificado pidiendo el manifest a ghcr.io sin
    autenticación → 200). Si algún día el repo se vuelve privado, el `pull` del VPS empieza a fallar
    con 401 y hay que darle credenciales a la VM o publicar el paquete a mano.
  - **El tag lleva el SHA del commit** (`:sha-abc1234`), no un tag móvil: reintentar un deploy vuelve
    a poner el mismo binario. El `:develop` existe solo para levantar a mano.
  - **`platforms: linux/amd64` explícito**: el VPS es amd64 y sin fijarlo coincide por casualidad con
    el runner.
  - `concurrency: deploy-vps` con `cancel-in-progress: false`: dos deploys a la vez correrían
    `compose up` sobre la misma VM, y cancelar a media substitución de contenedor deja la API abajo.
  - El deploy termina verificando `/readyz` — un deploy que deja la API caída en silencio es peor que
    uno que falla ruidoso.
- **Una hoja que nace con `open` puesto no se monta si otra se está cerrando en la misma
  actualización.** Apareció al subir `@chakra-ui/react` de 3.36.1 a 3.37.0: tocar "Cobrar" dentro de
  la hoja de *Pedidos por cobrar* cierra esa hoja y monta la de cobro en el mismo `onClick`, y con
  3.37 quedaban CERO `[role="dialog"]` en el árbol — el camino con el que se cobra desde el botón
  naranja dejaba de abrir nada.

  Arreglado en [CobrarSheet](web/src/shared/CobrarSheet.tsx): la hoja monta **cerrada** y se abre en
  el render siguiente, para que haya una transición de cerrado a abierto de verdad. Lo cubre
  `PedidosEnCurso.test.tsx` › *tocar Cobrar cierra la lista Y abre la hoja de cobro*.

  Dos alternativas que se probaron y NO sirven, para que nadie las reintente: `defaultOpen` no
  dispara la transición, y partir el cierre y la apertura en dos commits con `flushSync` arregla
  unos casos y deja otros rotos. La que quedó necesita apagar `react-hooks/set-state-in-effect` en
  esa línea: es un render extra al montar, acotado, y React no ofrece otra forma de provocar una
  transición al montar.

- **`overflowY="auto"` SIN un alto no hace scroll: la caja crece.** No falla, no avisa y en el
  monitor de quien programa se ve bien — el precio lo paga la tableta de 600 px, donde cada renglón
  de la lista empuja fuera de la pantalla lo que viene debajo. Va con `maxH` en **dvh** (lo que se
  reparte es el alto de la tableta, no un número de píxeles) o con el patrón `Page fill` +
  `flex="1" minH={0}` que documenta [Page.tsx](web/src/components/Page.tsx). Pasó en la lista de
  diferencias de la 020, que sacaba de la pantalla la lista de tiendas y el formulario de alta.
  **Y un componente que se pinta DENTRO de otra pantalla no trae su propio `Page`**: anidarlos
  duplica 48 px de relleno que no separa nada y recorta el ancho dos veces. Lo cubren
  `MenuDePlataformaPage.test.tsx` y `PlataformasPage.test.tsx`, que cuentan los contenedores con la
  firma del `Page` (`max-width: 1150px`) — en jsdom `getComputedStyle` sí resuelve lo que emite
  Chakra, así que el defecto de disposición sí deja test.
- **GOTCHA al subir el toolchain de Go (¡lee esto antes de bumpear Go!):** las herramientas de análisis basadas en Go (golangci-lint, govulncheck) hacen un self-check y **rechazan** analizar un módulo cuyo Go sea de un **minor mayor** al Go con que se compiló la herramienta. Al subir `toolchain`/`go` en go.mod:
  - **golangci-lint**: sube en `ci.yml` el input `version:` a una release compilada con Go del **mismo minor o mayor** (verifica con `go version $(which golangci-lint)`). Además el `golangci-lint-action` debe ser **v7+** para soportar golangci-lint v2. El self-check compara por **minor** (1.27.x sirve para cualquier toolchain 1.27.y), no por patch. Al subir a 1.27 se pinó `v2.13.1` (compilada con go1.27.0). **Y no basta el minor**: con go1.27.2 ese binario truena con *export data version 5 is greater than maximum supported version 4* al leer la biblioteca estándar. Por eso CI lo compila con el Go del job (`install-mode: goinstall`, hoy `v2.14.0`) y el contenedor del hook lo instala con `go install` sobre `golang:1.27`. Las herramientas **locales** también: `go install …@latest` desde un directorio SIN go.mod, porque dentro del módulo aplica el `toolchain` y las recompila con el Go viejo — el hook queda roto con un panic del type-checker.
  - **govulncheck**: la action lo compila con el Go del `go-version-file` (= go.mod), así que se resuelve solo si el `go` directive es coherente.
- **`make install` y lefthook con Go 1.27**: `github.com/evilmartians/lefthook@latest` (módulo v1) **ya no compila** — su dep `go-json-experiment/json` referencia `json.SkipFunc`/`json.DiscardUnknownMembers`, que `encoding/json/v2` movió al entrar a la stdlib. El módulo **`/v2`** sí compila y valida el `lefthook.yml` actual sin tocarlo (`lefthook validate` → *All good*), así que el Makefile instala `lefthook/v2@latest`. Si el hook deja de sincronizarse, revisa primero eso y no `--no-verify`.
- **Go 1.27 — lo que cambia para este repo.**
  - `encoding/json` ahora corre sobre la implementación de v2, pero la **API v1 no cambió de
    semántica**: verificado en 1.27.0 que sigue aceptando nombres duplicados (gana el último) y
    UTF-8 inválido. La estrictez vive en `encoding/json/v2`, que es opt-in. Si algún día se quiere
    rechazar duplicados en la frontera es cambiando de paquete a propósito, no algo que el bump
    trajo gratis — no asumas que el 1.27 endureció el decode de los handlers.
  - El `uuid` de la stdlib reemplazó a `github.com/google/uuid` (mismo `[16]byte`, pgx lo encodea
    con su codec de uuid igual que antes; verificado contra Postgres real en los tests de
    integración). Ojo: `uuid.Nil` es **función** (`uuid.Nil()`) y no existe `NewString()`.
  - `strings.CutLast` es lo que usa `rateKeyIP` para tomar el último XFF.
  - `new(expr)` ya acepta un valor: `new(x)` en vez de `v := x; &v`.
- **`overrides` en `web/package.json`: hoy hay DOS, y los dos cruzan un major que el padre no
  acepta** (2026-10-31). Los arreglos de `minimatch` (GHSA-g3hv-75wg-m249) y de `glob`
  (GHSA-3g3m-2fjr-6pw4) solo existen en un major que sus padres, ya en su última versión, todavía no
  piden:
  - `minimatch: "^11.0.1"`: `eslint` (`@eslint/config-array`) y `typescript-eslint` piden `^10`.
    Verificado que `eslint .` sigue encontrando archivos y que la barrera entre la consola y el POS
    sigue tronando.
  - `glob: "^14.0.2"`: `workbox-build` 7.4.1 pide `^11`. Verificado que el service worker precarga
    los mismos 16 archivos que antes, y que en Chromium se instala y avisa de la versión nueva.

  Se borra cada uno en cuanto su padre pida el major corregido. **Cuando el padre SÍ acepta la
  versión corregida y bun conserva la vieja en el lockfile** (pasó con `zod`, `fast-uri` y
  `serialize-javascript`), no se deja override: se pone un momento para mover el lockfile, se quita,
  y se comprueba que `bun install --frozen-lockfile` instala y que `bun audit` sigue limpio. Una
  resolución desde cero tomaría la misma versión, porque está dentro del rango del padre.

  **Lo que ningún override arregla**: `deepmerge` ≤ 4.3.1 (GHSA-gcqx-w48m-w2h4, *low*), que entra por
  `vite-plugin-pwa` → `workbox-build` → `@rollup/plugin-node-resolve`, no tiene versión corregida y
  solo corre al construir. Y en Go, GO-2026-5932 (`x/crypto/openpgp` sin mantenimiento) tampoco
  tiene arreglo; `govulncheck` lo reporta solo por módulo, porque el código no importa ese paquete.

  Antes de éstos existieron otros para
  forzar la versión parcheada de un CVE *high* que vivía en una transitiva de desarrollo cuando el
  padre pineaba un rango que no alcanzaba el parche (`eslint`→`ajv`→`fast-uri`,
  `vite-plugin-pwa`→`workbox-build`→`glob`→`brace-expansion`, `jsdom`→`undici`,
  `vite`→`postcss`→`nanoid`, `browserslist`). Los cinco se borraron el 2026-09-03 al comprobar que
  los padres ya resuelven a versiones limpias: `bun audit` sin ellos no encuentra nada en ningún
  nivel salvo un *moderate* de postcss que ya estaba y va por debajo del gate.

  Si vuelve a hacer falta uno, dos reglas que costaron caro:
  - **Fija el piso REAL del aviso, no el primero que veas.** `brace-expansion: ">=2.1.3"` resolvía a
    4.x, que tiene su propio rango vulnerable.
  - **Acota el major cuando el consumidor depende de internals**: `undici: "^7.29.0"` y no
    `>=7.29.0`, porque la 8.x movió lo que jsdom requiere y deja los tests en "no tests".

  Y **verifica quitándolo de verdad**: borrar el override y correr `bun install` NO basta si el
  lockfile ya tiene la versión parcheada — la conserva. Hay que ver a qué resuelve el padre
  (`bun.lock`) y correr `bun audit` sin filtro de nivel. Un override que sobra son paquetes
  duplicados en el árbol: quitar estos cinco bajó de 699 a 689.
- **KNOWN NON-ISSUE (no lo "arregles"):** el aviso de deprecación de `golang.org/x/crypto/blowfish` que aparece dentro de `x/crypto/bcrypt` es **esperado** — bcrypt usa blowfish internamente. `bcrypt.GenerateFromPassword` ([auth.HashSecret](server/internal/auth/password.go)) es la forma correcta y vigente de hashear passwords y **no** está deprecada. No lo cambies por AES ni otro cifrado.

## 3-bis. La VM de producción no se toca sin permiso

`pos-vps` está detrás de un hook: [proteger-produccion.sh](scripts/hooks/proteger-produccion.sh) mira
cada comando ANTES de correrlo y pide aprobación si nombra esa máquina o su IP. Pasan solos los
respaldos (`pg_dump` y traerse el archivo), que es la excepción que se autorizó: no cambian nada y
son lo que uno quiere poder hacer rápido antes de una migración.

**`pos-vps` tiene respaldo verificado cada noche a las 00:45** (hora del centro) y snapshot del
disco a las 02:00; desde el 2026-10-05 ya no se apaga de noche. Runbook:
[docs/respaldos-produccion.md](docs/respaldos-produccion.md).

`pos-vps-dev` **no** está gateado: ahí se prueba, y frenar cada comando volvería inútil la
verificación contra el ambiente desplegado.

Pasan solos también el script de respaldo de la VM (`respaldo-gatobobah.sh`) y las **lecturas de
salud** (`journalctl`, `docker ps`/`logs`, `curl` a `readyz`, `df`, `uptime`) cuando el comando no
trae nada que escriba, reinicie, entre a la base o lea el `.env` (decidido el 2026-11-01: un encargo
de punta a punta se detuvo 9 horas esperando aprobar el respaldo previo a subir). Tampoco pregunta
un comando que solo **nombra** la VM o `ssh` entre comillas, como el patrón de un `grep`: solo
cuenta el programa que conecta en posición de comando, salvo que algo vuelva a ejecutar el texto
(`bash -c`, `eval`, `xargs`), y entonces se revisa completo (decidido el 2026-11-02: un `grep
"ssh\|pos-vps"` frenó un `/goal`). Y pasan solas las **consultas de solo lectura a la base**:
`docker exec deploy-postgres-1 psql` con `begin read only`, sin nada que cierre esa transacción
(`commit`, `end`), la cambie a escritura o escriba (`insert`, `set_config`, `\!`, SQL leído de un
archivo); `set role` y `app.company_id` sí se permiten (decidido el 2026-11-06: investigar un reporte
de quien opera se detuvo en cada `select`). Lo cubre
[proteger-produccion-test.sh](scripts/tests/proteger-produccion-test.sh).

**Por qué es un hook y no una nota.** El 2026-09-19 hubo que auditar doce transcripciones de sesión
a mano para responder «¿algún agente escribió en producción?». La respuesta fue que no —solo
lecturas del `.env` y respaldos—, pero nada impedía que la siguiente vez sí. Un recordatorio se
olvida; un comando que se detiene, no. Es el mismo argumento de
[migracion-con-test.sh](scripts/hooks/migracion-con-test.sh).

## 4. Commits / PR

Qué corre cada hook de **lefthook** ([lefthook.yml](lefthook.yml)):

- **pre-commit**: `gofmt`, golangci-lint (+ gosec), web lint + typecheck, y **migración nueva sin su test de integración** ([migracion-con-test.sh](scripts/hooks/migracion-con-test.sh)). Este último existe porque la regla ya estaba en la constitución y aun así se rompió: la migración 0037 se escribió antes que sus tests, y los tests encontraron después un defecto real. Un recordatorio se olvida; un commit que falla, no.
- **pre-push**: `go test ./...`, `govulncheck`, `bun run build`.

Que deban quedar verdes y que `--no-verify` no se use son quality gates de la constitución.

**Un PR hacia `develop` se mergea en cuanto su CI termina en verde, sin pedir OK** (decidido el
2026-10-14): develop solo despliega al ambiente de pruebas, y detenerse ahí frena las capturas y las
pruebas que vienen después. Si CI falla, se arregla; no se mergea. **Producción (`main`, `pos-vps`)
sigue pidiendo OK explícito del dueño.**

**Las pruebas corren en el PR, no al mergear a develop** (decidido el 2026-10-15; el job `changes`
de [ci.yml](.github/workflows/ci.yml) lo decide). Un PR corre solo lo que su cambio toca —uno de
puros `specs/` o `docs/` no corre nada— y el push a develop solo construye y despliega. Lo que lo
hace seguro: el PR se mergea **al día** con develop y con sus checks en verde, así que lo probado es
lo que queda. Hoy lo cuida quien mergea; la protección de develop que lo exige (checks requeridos y
«al día») la activa el dueño. Si el PR va atrás, se trae develop **en local** (commit firmado),
`make ci-local`, se empuja y se espera el CI de nuevo; no se mergea sin eso. **No** con
`gh pr update-branch` ni el botón «Update branch»: ese merge lo firma GitHub y no el dueño, y rompe la
autoría única de la constitución. **El push a `main` no repite lo que ya se probó** (decidido el
2026-11-02). La llave es el **árbol** de archivos (`git rev-parse HEAD^{tree}`), no el SHA: el merge
crea un commit nuevo, pero con develop al día sus archivos son idénticos a los de la cabeza del PR.
- Pruebas: si un PR en verde probó ese árbol ([ci-tree-tested.sh](scripts/ci-tree-tested.sh)), no
  corren; si no —un push directo, un PR que no estaba al día—, corre la suite completa.
- Imagen: el push a develop la publica también como `api:tree-<árbol>`; main la encuentra y solo le
  pone sus etiquetas, sin compilar. Producción corre el mismo binario que el ambiente de pruebas, y
  por eso `/api/v1/version` dice el SHA de develop que la construyó. Un hotfix trae árbol nuevo y sí
  se construye.
- El frontend se construye otra vez: la URL de la API queda dentro del build y es otra por ambiente.
Un test que lea archivos de otra carpeta (como los de nombres de empresa, que leen `web/src`) se
agrega también al filtro de su job, o un PR de esa carpeta lo deja de correr.

### Identidad, firma y autoría

Se clona y se pushea **por SSH** (`git@github.com:…`, nunca HTTPS) y **todo commit va firmado con
GPG**. La config es **local a este repo** — la máquina tiene otros repos con otra identidad, así
que no se toca `--global`:

| clave | valor |
| --- | --- |
| `user.name` / `user.email` | `Ramses Martinez` / `ramses.mtz96@gmail.com` |
| `user.signingkey` | `6B7243B7F63FCCA0A645AC7603570B54632AB5C1` (ed25519 `[SC]`, expira **2028-08-26**) |
| `commit.gpgsign` / `tag.gpgsign` | `true` |
| `gpg.program` | `C:/Program Files/Git/usr/bin/gpg.exe` (solo Windows; en Linux/mac basta el `gpg` del PATH) |
| `core.sshCommand` | `ssh -i ~/.ssh/id_ed25519 -o IdentitiesOnly=yes` |

- **`gpg.program` con ruta de Windows**: el `git.exe` que corre desde PowerShell o VS Code no resuelve el `/usr/bin/gpg` de Git Bash y falla con *gpg failed to sign the data*.
- **La llave no tiene passphrase, a propósito**: los hooks de lefthook y los agentes firman sin TTY y en Git Bash el pinentry no se renderiza — con passphrase el commit se queda colgado. La protección es la cuenta de Windows; el riesgo aceptado es una llave exportable si alguien ya tiene la sesión.
- **Renuévala antes de 2028-08-26** (`gpg --quick-set-expire <fp> 2y`): una llave expirada hace fallar todo commit con `commit.gpgsign=true`, no lo firma sin avisar.
- Verificar: `git log -1 --pretty="%G? %GK"` → `G` + el key id. El badge *Verified* de GitHub exige además tener la pública subida (`gh gpg-key add` con scope `admin:gpg_key`).
- **Ningún commit lleva `Co-Authored-By`** — es quality gate de la constitución. Claude Code lo agrega por default: [.claude/settings.json](.claude/settings.json) lo apaga con `"includeCoAuthoredBy": false`. Otro harness que lo reinyecte, se borra del mensaje antes de commitear.

## 5. Spec-driven development

Las features nuevas pasan por spec-kit: `/speckit-specify` → `/speckit-plan` → `/speckit-tasks` →
`/speckit-analyze` → `/speckit-implement`. Cada una crea rama `NNN-slug` desde `develop` y su
directorio `specs/NNN-slug/`. El paso `analyze` **no es opcional** aquí (ver *Quality gates*).
Los principios de la constitución son la vara con la que `analyze` mide spec, plan y tasks.

### De punta a punta, sin que el dueño lo pida paso por paso

Cuando el dueño encarga un issue o una feature «hasta producción» (con `/goal` o con esas palabras),
la sesión que lo recibe hace TODO esto por su cuenta, en este orden:

1. **Worktree propio, creado ANTES de abrir la sesión que trabaja**: la sesión que orquesta (la del
   checkout principal) corre `/nuevo-spec NNN slug` y le entrega al dueño `cd <worktree> && claude`
   más el prompt, que dice «ya estás en tu worktree, no crees otro». La sesión que trabaja nace
   dentro de él. Solo si nadie orquesta, la propia sesión lo crea antes de tocar código. Nunca se
   trabaja en el checkout principal, que se queda en `develop` y limpio. **Varias specs a la vez**
   son varios worktrees, uno por spec, cada uno con su número `NNN` libre (revisa `specs/`, las ramas
   remotas y `../egb-NNN-*`) y su número de migración; la que llegue primero a `develop` se queda con
   el número de migración y la otra renumera.
2. **Feature** → ciclo de spec-kit completo, invocando cada `/speckit-*` con la herramienta Skill
   (no leyéndola) y con sus hooks (`/maquetas`, `/revision-de-arquitectura`), hasta que
   `/speckit-analyze` quede sin CRITICAL ni HIGH. **Defecto** → rama `fix/slug`, test que falla por
   la razón correcta, y el arreglo.
3. **Implementar con TDD** y después `/revision-de-codigo`; lo que encuentre se corrige con su test.
4. **Prueba con usuario nuevo** a 1024×600 contra el `make start` del worktree, si hay pantalla.
5. **Gates**: `make ci-local` verde antes de empujar.
6. **PR a `develop`**, esperar su CI y mergear en verde; luego **PR de release `develop` → `main`**,
   esperar su CI, mergear y seguir el deploy hasta `readyz` 200 en producción. Con autorización del
   dueño para aprobar PR, no se le vuelve a preguntar.
7. **Cerrar**: `make cerrar-worktree dir=… datos=1`, borrar la rama remota ya mergeada, y comentar y
   cerrar el issue con lo que entró y lo que quedó fuera.

Las dudas se resuelven con la mejor recomendación (un subagente que puntúe opciones si hace falta) y
se escriben en el spec o el plan; solo se le pregunta al dueño lo que cruza una puerta del principio
VIII o pierde datos.

**Si git dice «this operation must be run in a work tree» en el checkout principal**, alguien le
puso `core.bare = true` (git lo trata como un repositorio sin archivos): se quita con
`git config --local core.bare false`. Así fue como el script del worktree dejó de correr ahí el
2026-11-01.

## 6. Documentación — dónde vive qué

`docs/` tiene un índice que dice qué sigue vigente y qué es histórico:
**[docs/README.md](docs/README.md)**. Léelo antes de citar cualquier cosa de ahí.

- **Vigente**: [docs/security-owasp.md](docs/security-owasp.md) (respaldo del principio V; obligatorio antes de tocar auth/config/middleware/logging) y [docs/email-zoho.md](docs/email-zoho.md) (runbook de SMTP).
- **Histórico, no lo sigas**: [docs/design/](docs/design/) son los planes de la construcción inicial, ya ejecutados y desactualizados en los detalles — sirven para entender el porqué del diseño, no el estado de hoy. [docs/reorg/](docs/reorg/) es el reorg del menú ya aplicado (patrón: SQL numerado + rollback gemelo, para **datos**; el esquema se migra con goose).
- **No son docs**: `docs/tickets/` son PDFs reales que usan como fixtures `purchasedoc_test.go`, `expenseDraft.test.ts` y `make parse-doc`. No los renombres — hay tests atados a esos nombres.

Un documento nuevo: spec de feature → `specs/` (vía spec-kit); principio → la constitución;
comando o quirk general → este archivo; lo que no se ve de un área (lo que antes iba como «cosas que no se
ven» de cada feature) → su archivo en [docs/mecanica/](docs/mecanica/), nunca aquí: este archivo se carga
en cada sesión y tiene techo; runbook operativo → `docs/` **y** su renglón en el índice.

## 7. Windows (Git Bash) — quirks del entorno de dev

La caja de dev es Windows 11 + Git Bash. Lo que muerde ahí y no en Linux/mac:

> **Regla, solo en Windows: NO intentes compilar ni arrancar el binario en el equipo. Usa Docker desde el principio.**
> Smart App Control decide por reputación y un ejecutable recién compilado no tiene ninguna, así que
> `go run`, `go test` y cualquier herramienta que acabes de instalar pueden fallar hoy y funcionar
> mañana sin que nada cambie. Intentarlo, verlo fallar, recompilarlo y volver a intentar es tiempo
> tirado: el veredicto no depende de ti. Todos los caminos ya tienen su fallback en contenedor
> ([go-test.sh](scripts/hooks/go-test.sh), [govulncheck.sh](scripts/hooks/govulncheck.sh),
> [golangci-lint.sh](scripts/hooks/golangci-lint.sh)) y la API de desarrollo se levanta con el
> `docker run` de más abajo. En Linux y macOS nada de esto aplica: ahí se corre normal.

- **Smart App Control bloquea binarios recién compilados.** Está **on por default** en Windows 11 y no se puede excluir un archivo: o se apaga entero (y volver a encenderlo exige reinstalar Windows) o se convive con él. Bloquea por reputación, así que es errático — el síntoma es *"Una directiva de Control de aplicaciones bloqueó este archivo"* y en `Microsoft-Windows-CodeIntegrity/Operational` un evento 3077/3118. Lo confirmado:
  - El binario que `go run` deja en `%TEMP%\go-build…` **se bloquea siempre** → la API moría al arrancar desde `make start`. Por eso [scripts/dev-api.sh](scripts/dev-api.sh) compila a `server/tmp/api` (ruta estable, la misma de air). **No lo regreses a `go run`** — pero tampoco lo tomes por arreglado: el veredicto es **por binario**, así que un `go build` nuevo puede quedar bloqueado aunque el anterior corriera desde esa misma ruta. Ya pasó: la API arrancó bien y, tras un cambio de código, el binario nuevo quedó bloqueado.
  - **La salida confiable es levantar la API en contenedor** (Linux, fuera del alcance de SAC), contra el postgres/redis del compose dev. Es lo que hay que usar cuando `make start` muere con *Permission denied* en `server/tmp/api`:

    ```bash
    # Los roles de producción también aquí (ver §2, «La base local se comporta como producción»).
    APP_PW=$(grep '^APP_DB_PASSWORD=' deploy/.env | cut -d= -f2-)
    PLAT_PW=$(grep '^PLATFORM_DB_PASSWORD=' deploy/.env | cut -d= -f2-)
    MSYS_NO_PATHCONV=1 docker run --rm --name gatobobah-api-dev --network deploy_default -p 8080:8080 \
      -v "d:/git/el-gato-bobah-pos/server:/src" -v "d:/git/el-gato-bobah-pos/deploy/.env:/env/.env:ro" \
      -v gatobobah_gocache:/root/.cache/go-build -v gatobobah_gomod:/go/pkg/mod -w /src \
      -e DATABASE_URL='postgres://gatobobah:gatobobah@postgres:5432/gatobobah?sslmode=disable' \
      -e APP_DATABASE_URL="postgres://gatobobah_app:$APP_PW@postgres:5432/gatobobah?sslmode=disable" \
      -e PLATFORM_DATABASE_URL="postgres://gatobobah_platform:$PLAT_PW@postgres:5432/gatobobah?sslmode=disable" \
      -e REDIS_URL='redis://redis:6379' -e APP_ENV=development -e PORT=8080 -e ENV_FILE=/env/.env \
      -e SMTP_HOST=mailpit -e SMTP_PORT=1025 -e APP_BASE_URL='http://localhost:3000' \
      golang:1.27 go run ./cmd/api
    ```

    Los volúmenes de caché no son opcionales: sin ellos cada arranque vuelve a bajar el módulo entero. El front sigue corriendo en el host con `bun run dev` y su proxy a `:8080`.
  - **`go test`, `govulncheck` y `golangci-lint` ya no se ejecutan en el host.** El binario que `go test` deja en `%TEMP%` es NUEVO en cada corrida, así que es el peor caso posible para SAC: bloquea un paquete al azar —hoy `internal/cache`, mañana otro— con el resto de la suite en verde. No es un test que falle, es un proceso que no arranca. `govulncheck.exe` se bloquea siempre, lo recompiles como lo recompiles (probado con `-ldflags="-s -w"` y desde otra ruta). `golangci-lint` corrió sin problema desde el 26-ago y el 29-ago empezó a dar *Permission denied* sin que nada cambiara: **el veredicto de SAC se mueve solo**, así que ninguna herramienta está a salvo por haber corrido ayer. Los dos hooks ya traen la salida y **no hay nada que hacer a mano** — si el binario local no arranca, el mismo escáner/linter corre en contenedor con la versión que usa CI:
    - [scripts/hooks/govulncheck.sh](scripts/hooks/govulncheck.sh) → `golang:1.27`.
    - [scripts/hooks/golangci-lint.sh](scripts/hooks/golangci-lint.sh) → `golang:1.27` + `go install` de golangci-lint `v2.14.0`. La versión está fijada en el script y **debe moverse junto con la de `ci.yml`** por el self-check del §3.
    - [scripts/hooks/web-lint.sh](scripts/hooks/web-lint.sh) → `oven/bun:1`. **Con bun el síntoma es
      distinto**: no sale el mensaje de SAC sino un lacónico `bun: unknown error:` y un exit 1, sin
      decir qué script falló — mientras `tsc` y `eslint` corridos a mano pasan limpios. `node_modules`
      va en un volumen y no en el bind mount: las deps con binarios nativos (esbuild, rolldown) se
      instalan para Linux dentro del contenedor y montarlas encima dejaría al host sin poder correr
      nada.

    Lo que **no** se hace es `--no-verify`: el gate no se afloja, se corre en otro lado.
  - **`lefthook` TAMBIÉN se bloquea, y cuando pasa los hooks no corren y casi no se nota.** El
    síntoma es una línea suelta entre la salida de git: *Can't find lefthook in PATH*, aunque
    `which lefthook` lo encuentre — el shim de `.git/hooks/` no distingue "no está" de "el sistema
    no me deja ejecutarlo", y el commit y el push siguen adelante **sin gates**. Confírmalo
    corriendo `lefthook version`: si dice *Permission denied* sobre el binario, es SAC.
    Reinstalarlo NO ayuda: el binario recién compilado es el peor caso posible para SAC, porque no
    tiene reputación ninguna (probado con `go install …/lefthook/v2@latest`, bloqueado al primer
    intento).

    Mientras dure, los gates se corren a mano antes de cada commit y CI queda de respaldo real —
    corre la suite completa, incluida la de integración, sobre lo que subiste:

    ```bash
    cd server && bash ../scripts/hooks/golangci-lint.sh && go build ./...
    cd ../web && bun run lint && bun run vitest run && bun run build
    ```

    Lo que **no** se hace es dar por buenos los gates porque "el commit pasó": con lefthook
    bloqueado, que el commit pase no significa nada.
  - `go build` a una ruta del repo, `go test` y el resto de las herramientas (`sqlc`, `goose`, `air`) corren sin problema **hoy** — con la advertencia de arriba: eso puede cambiar de un día para otro.
- **El working tree va en LF y [.gitattributes](.gitattributes) lo fuerza** (`* text=auto eol=lf`). Sin él, el `core.autocrlf=true` que Git for Windows deja por default checa out los 113 `.go` en CRLF y `gofmt -l` —lo primero que corre el pre-commit— los marca todos: no se puede commitear Go sin `--no-verify`, que no se usa. Si te tocó un working tree ya en CRLF: `git config --local core.autocrlf false`, quítales el `\r` y corre `git add --renormalize .`. Ese último paso importa — el contenido queda idéntico pero el stat cache no se refresca solo, y `git status` se queda marcando cientos de archivos "modificados" que `git diff` ve iguales.
- **`GOTOOLCHAIN` vacío se comporta como `local`**: con Go 1.26 instalado, `go build` falla con *"go.mod requires go >= 1.27.0"* en vez de bajar el toolchain. Se arregla con `go env -w GOTOOLCHAIN=auto` (baja go1.27.0 al module cache) o instalando Go 1.27.
- **No hay `lsof`**, así que `scripts/start.sh` no detecta puertos ocupados ni `scripts/stop.sh` mata la API/web por puerto. Fija los puertos (`BACKEND_PORT=8080 FRONTEND_PORT=3000 make start`) y cierra a mano lo que quede vivo.
- **`docker run -v` con rutas POSIX**: Git Bash reescribe `/src` a `C:/Program Files/Git/src`. Prefija `MSYS_NO_PATHCONV=1`.
- **La firma GPG también topa con Windows**: ver `gpg.program` en §4.
