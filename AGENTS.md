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
- **`specs/`** — un directorio por feature (`NNN-slug/`), generado por spec-kit.
- **`docs/`** — referencia viva, histórico y fixtures; el índice manda ([docs/README.md](docs/README.md), ver §6).
- **`references/`** — exports reales de FUDO (fuente del importador de catálogo).

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
- **sqlc NO conoce `company_id`** en las ~30 tablas a las que se lo agregó
  [0023](server/migrations/0023_tenant_columns.sql) con `EXECUTE format()`: su parser no lee DDL
  dinámico. Nombrar esa columna en una consulta rompe `sqlc generate` con "column does not exist"
  por una columna que sí existe en Postgres. No hace falta: RLS la aplica sola.

## 2. Comandos (todos en `Makefile`; `make help` los lista)

- `make install` — setup completo (valida entorno, instala deps y herramientas). `make check` solo verifica prereqs.
- `make start` — levanta todo: Postgres (default **:5490**), Redis (**:6390**), mailpit (**:8095**/**:1095**), API (default **:8080**) y web (Vite default **:3000**). **Ningún puerto es fijo**: un default, por raro que sea, choca con el postgres/redis de otro compose local. `start.sh` reusa el puerto que ya publica el contenedor vivo o toma el primero libre desde el default, y exporta `PG_PORT`/`REDIS_PORT`/`MAILPIT_*` para el compose y `dev-api.sh`; el `Makefile` los lee igual (`PG_PORT=… make db-migrate`). Pregunta los puertos de API/web (defaults auto-ajustados al primer libre) y **detecta puertos ocupados** antes de levantar. Fíjalos sin preguntar con `BACKEND_PORT=… FRONTEND_PORT=… make start`. El binario Go lee `PORT`; `vite.config.ts` lee `BACKEND_PORT` (proxy `/api`) y `FRONTEND_PORT`.
- `make api-dev` (hot reload con air) · `make api-build` (= `cd server && go build ./...`) · `make api-test` (= `cd server && go test ./...`).
- `make web-dev` · `make web-build` · `make web-test` (vitest).
- **`bun run e2e`** (en `web/`) — Playwright contra el **ambiente de pruebas desplegado**, a
  1024×600. No monta un servidor local a propósito: lo que estas pruebas atrapan es el desacuerdo
  entre lo que la pantalla calcula y lo que el servidor cobra, y con el backend simulado los dos
  están de acuerdo por construcción. En Windows los binarios del navegador los bloquea Smart App
  Control, así que corre en contenedor como el resto de los gates:

  ```bash
  MSYS_NO_PATHCONV=1 docker run --rm --network host -v "d:/git/el-gato-bobah-pos/web:/w"     -v gatobobah_e2e_modules:/w/node_modules -w /w -e CI=1     mcr.microsoft.com/playwright:v1.62.1-noble     sh -c "bun install --frozen-lockfile --silent; npx playwright test"
  ```

  Un fallo en el primer `goto` casi siempre es la VM spot apagada, no el código: revísalo antes de
  buscar el defecto. Los casos y su porqué están en [docs/matriz-de-cobro.md](docs/matriz-de-cobro.md).

  **La suite COBRA los pedidos que crea.** El ambiente es compartido con una persona, y un pedido de
  prueba que se queda abierto aparece en la barra del POS, suma a "por cobrar" y bloquea el cierre de
  caja — le hace creer a quien opera que hay dinero pendiente. El `globalSetup` anota qué pedidos ya
  estaban abiertos y el `globalTeardown` cierra solo los que la suite abrió
  ([web/e2e/limpiar-lo-que-cree.ts](web/e2e/limpiar-lo-que-cree.ts)); sin esa marca no toca nada,
  porque cerrarle a alguien una cuenta viva es peor que dejar basura. Si escribes un script suelto
  que cree pedidos, ciérralos tú: entregar (`POST /orders/:id/deliver`) y cobrar
  (`POST /orders/:id/pay`, **no** `/charge`), y un pedido de plataforma solo acepta el método de SU
  plataforma.
- `make lint` (golangci-lint + gosec) · `make vuln` (govulncheck) · `make web-lint` (eslint + tsc) · `make sec` (todos).
- `make deploy-image` — deploy del backend **sin compilar**: baja de ghcr.io la imagen que publicó CI y hace `up -d`. Es lo que corre el VPS. `make deploy` (compila local) queda como fallback si CI está caído.
- `make sqlc` (regenera código de queries) · `make migrate-new name=xxx` (nueva migración goose).
- **Frontend siempre con bun** (`bun install`, `bun run`, `bun audit`). `web/package.json` bloquea npm (`preinstall: only-allow bun`). Nunca crees `package-lock.json`.

## 3. Dependencias y supply chain

Que un CVE bloquea el merge es principio (constitución, *Restricciones del producto*). Esto es la
mecánica:

- **Frescura ≠ pre-commit.** Actualizar deps al día lo maneja **Dependabot** ([.github/dependabot.yml](.github/dependabot.yml)): github-actions, gomod (`/server`), **bun** (`/web`, no `npm`: es lo único que actualiza `bun.lock`) y docker (`/server`+`/deploy`), semanal. Chequeo manual: `go list -u -m all` (Go), `bun outdated` (web). **No** añadas un pre-commit de "deps desactualizadas" (ruidoso, bloquea cambios ajenos).
- **Dónde corre cada scanner**: Go → `govulncheck` (pre-push lefthook **+** CI). Frontend → `bun audit --audit-level=high` en CI, sin `|| true`.
- **Runtimes**: con línea LTS → la última LTS (Node = 24, `.nvmrc`); Go no tiene LTS → el último minor estable (hoy `go1.27.0`).
- **Pin fuerte**: imágenes base por **digest** (`server/Dockerfile`, `deploy/docker-compose.yml`), GitHub Actions por **SHA** ([ci.yml](.github/workflows/ci.yml)), toolchain Go fijado en `go.mod` (hoy `go 1.27.0`). **No agregues una línea `toolchain` igual al `go` directive**: `go mod tidy` la borra por redundante en cada corrida — cuando coinciden, el `go` directive ES el pin.
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
- **NO SUBAS `@chakra-ui/react` a 3.37.0 sin arreglar antes las hojas encimadas.** Medido el
  2026-09-03: con 3.37.0 (y `@ark-ui/react` 5.39.0), tocar "Cobrar" dentro de la hoja de *Pedidos
  por cobrar* cierra esa hoja y **la hoja de cobro no monta** — cero `[role="dialog"]` en el árbol.
  Se cae el camino con el que se cobra desde el botón naranja, que es dinero.

  El patrón nuestro que lo dispara: `CobrarSheet` monta su `DrawerRoot` ya en `open`
  (`if (!order) return null` y `<DrawerRoot open …>`), así que no hay transición false→true; en
  3.36.1 eso funciona y en 3.37.0 no, pero solo cuando OTRA hoja se está cerrando en la misma
  actualización. Montada sola sigue funcionando — por eso los 18 tests de `CobrarSheet.test.tsx`
  pasan con 3.37 y el que falla es el de `PedidosEnCurso.test.tsx`.

  Antes de aceptar ese bump hay que darle a la hoja de cobro un `open` de verdad (montarla siempre
  y controlar el estado) o separar el cierre de una del montaje de la otra. El PR de Dependabot del
  grupo web viene con este bump adentro y su CI está verde: corrió contra un develop que todavía no
  tenía ese test.

- **GOTCHA al subir el toolchain de Go (¡lee esto antes de bumpear Go!):** las herramientas de análisis basadas en Go (golangci-lint, govulncheck) hacen un self-check y **rechazan** analizar un módulo cuyo Go sea de un **minor mayor** al Go con que se compiló la herramienta. Al subir `toolchain`/`go` en go.mod:
  - **golangci-lint**: sube en `ci.yml` el input `version:` a una release compilada con Go del **mismo minor o mayor** (verifica con `go version $(which golangci-lint)`). Además el `golangci-lint-action` debe ser **v7+** para soportar golangci-lint v2. El self-check compara por **minor** (1.27.x sirve para cualquier toolchain 1.27.y), no por patch. Al subir a 1.27 se pinó `v2.13.1` (compilada con go1.27.0). Las herramientas **locales** también: `go install …@latest` desde un directorio SIN go.mod, porque dentro del módulo aplica el `toolchain` y las recompila con el Go viejo — el hook queda roto con un panic del type-checker.
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
- **`overrides` en `web/package.json`: hoy NO hay ninguno, y así debe quedar.** Existieron para
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

## 4. Commits / PR

Qué corre cada hook de **lefthook** ([lefthook.yml](lefthook.yml)):

- **pre-commit**: `gofmt`, golangci-lint (+ gosec), web lint + typecheck, y **migración nueva sin su test de integración** ([migracion-con-test.sh](scripts/hooks/migracion-con-test.sh)). Este último existe porque la regla ya estaba en la constitución y aun así se rompió: la migración 0037 se escribió antes que sus tests, y los tests encontraron después un defecto real. Un recordatorio se olvida; un commit que falla, no.
- **pre-push**: `go test ./...`, `govulncheck`, `bun run build`.

Que deban quedar verdes y que `--no-verify` no se use son quality gates de la constitución.

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

## 6. Documentación — dónde vive qué

`docs/` tiene un índice que dice qué sigue vigente y qué es histórico:
**[docs/README.md](docs/README.md)**. Léelo antes de citar cualquier cosa de ahí.

- **Vigente**: [docs/security-owasp.md](docs/security-owasp.md) (respaldo del principio V; obligatorio antes de tocar auth/config/middleware/logging) y [docs/email-zoho.md](docs/email-zoho.md) (runbook de SMTP).
- **Histórico, no lo sigas**: [docs/design/](docs/design/) son los planes de la construcción inicial, ya ejecutados y desactualizados en los detalles — sirven para entender el porqué del diseño, no el estado de hoy. [docs/reorg/](docs/reorg/) es el reorg del menú ya aplicado (patrón: SQL numerado + rollback gemelo, para **datos**; el esquema se migra con goose).
- **No son docs**: `docs/tickets/` son PDFs reales que usan como fixtures `purchasedoc_test.go`, `expenseDraft.test.ts` y `make parse-doc`. No los renombres — hay tests atados a esos nombres.

Un documento nuevo: spec de feature → `specs/` (vía spec-kit); principio → la constitución;
comando o quirk → este archivo; runbook operativo → `docs/` **y** su renglón en el índice.

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
    MSYS_NO_PATHCONV=1 docker run --rm --name gatobobah-api-dev --network deploy_default -p 8080:8080 \
      -v "d:/git/el-gato-bobah-pos/server:/src" -v "d:/git/el-gato-bobah-pos/deploy/.env:/env/.env:ro" \
      -v gatobobah_gocache:/root/.cache/go-build -v gatobobah_gomod:/go/pkg/mod -w /src \
      -e DATABASE_URL='postgres://gatobobah:gatobobah@postgres:5432/gatobobah?sslmode=disable' \
      -e REDIS_URL='redis://redis:6379' -e APP_ENV=development -e PORT=8080 -e ENV_FILE=/env/.env \
      -e SMTP_HOST=mailpit -e SMTP_PORT=1025 -e APP_BASE_URL='http://localhost:3000' \
      golang:1.27 go run ./cmd/api
    ```

    Los volúmenes de caché no son opcionales: sin ellos cada arranque vuelve a bajar el módulo entero. El front sigue corriendo en el host con `bun run dev` y su proxy a `:8080`.
  - **`go test`, `govulncheck` y `golangci-lint` ya no se ejecutan en el host.** El binario que `go test` deja en `%TEMP%` es NUEVO en cada corrida, así que es el peor caso posible para SAC: bloquea un paquete al azar —hoy `internal/cache`, mañana otro— con el resto de la suite en verde. No es un test que falle, es un proceso que no arranca. `govulncheck.exe` se bloquea siempre, lo recompiles como lo recompiles (probado con `-ldflags="-s -w"` y desde otra ruta). `golangci-lint` corrió sin problema desde el 26-ago y el 29-ago empezó a dar *Permission denied* sin que nada cambiara: **el veredicto de SAC se mueve solo**, así que ninguna herramienta está a salvo por haber corrido ayer. Los dos hooks ya traen la salida y **no hay nada que hacer a mano** — si el binario local no arranca, el mismo escáner/linter corre en contenedor con la versión que usa CI:
    - [scripts/hooks/govulncheck.sh](scripts/hooks/govulncheck.sh) → `golang:1.27`.
    - [scripts/hooks/golangci-lint.sh](scripts/hooks/golangci-lint.sh) → `golangci/golangci-lint:v2.13.1`. La versión está fijada en el script y **debe moverse junto con la de `ci.yml`** por el self-check del §3.
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
