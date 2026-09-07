@AGENTS.md

# Claude Code

Los **principios** de ingeniería viven en `.specify/memory/constitution.md`; la **mecánica** del repo (stack, comandos, quirks de tooling) en `AGENTS.md`, que importa la constitución. Ambos entran por el import de arriba. Esto es solo lo específico del harness — no dupliques reglas aquí.

## CodeGraph (preguntas estructurales)

Este repo tiene un índice CodeGraph (`.codegraph/`). Para preguntas **estructurales** — dónde se define un símbolo, quién lo llama, qué firma tiene, qué rompe un cambio — usa `codegraph_*`, no grep:

- "¿Dónde está X? / firma / fuente de X" → `codegraph_search` / `codegraph_node`.
- "¿Qué llama a Y? / ¿qué rompe si cambio Z?" → `codegraph_callers` / `codegraph_impact`.
- "Entender un área / el flujo entre símbolos" → `codegraph_explore` (una sola llamada devuelve la fuente verbatim de varios símbolos).

**Confía en los resultados de codegraph** (vienen de un parse AST completo): no los re-verifiques con grep. Reserva grep/Read para **texto literal** (contenido de strings, mensajes de log) o para confirmar un detalle puntual.

`codegraph sync` al empezar y tras cada cambio de rama (un `git checkout` reescribe muchos archivos y deja el índice viejo). El watcher va ~1s detrás de una escritura: no re-consultes un archivo que acabas de editar en el mismo turno.

## Skills

Disponibles: `/code-review`, `/security-review`, `/simplify`, `/verify`, `/run`. Para trabajo de seguridad, `/security-review` complementa el principio V de la constitución.

Subagentes especializados en `.claude/agents/` (`go-backend-reviewer`, `security-auditor`, `db-architect`, `tablet-ui-reviewer`), y **dos skills que los lanzan en abanico y consolidan un solo veredicto**: `/revision-de-arquitectura` (los de diseño, sobre un `plan.md`) y `/revision-de-codigo` (los de código, sobre un diff, eligiendo cuáles aplican según qué archivos cambiaron).

**Dentro del ciclo de spec-kit ya no hay que acordarse de correrlos**: [.specify/extensions.yml](.specify/extensions.yml) los engancha en `after_plan` y `after_implement`, y ninguno es `optional`. Fuera del ciclo sí hay que invocarlos a mano — sobre todo el **`db-architect` ANTES de aplicar una migración nueva** y ante cualquier cambio en `server/migrations/` o `server/queries/`, y el **`tablet-ui-reviewer` ante cualquier pantalla nueva o cambio de disposición en `web/`**: el sistema vive en tabletas y esa restricción se olvida sola.

Las features nuevas van por spec-kit (`/speckit-*`), en `specs/NNN-slug/`; `/speckit-analyze` corre siempre antes de `/speckit-implement` y **exige `spec.md`, `plan.md` y `tasks.md`** — una feature que se saltó `plan` o `tasks` no es que pase el gate: es que el gate no puede correr.

## Quirks

- **Bun, nunca npm** (`AGENTS.md` §2). No sugieras `npm install` ni generes `package-lock.json`.
- Antes de dar por bueno un cambio de backend: `cd server && go build ./... && go test ./...` (= `make api-build && make api-test`).
- No hagas `git commit`/`push` salvo que el usuario lo pida; los hooks de lefthook deben pasar (`AGENTS.md` §4).
