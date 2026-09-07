---
name: "revision-de-codigo"
description: "Corre los revisores de código (go-backend-reviewer, security-auditor, tablet-ui-reviewer) sobre el diff, eligiendo cuáles aplican según qué archivos cambiaron, y consolida sus hallazgos en un solo veredicto. Lo dispara el hook after_implement de spec-kit; también sirve antes de abrir un PR."
argument-hint: "sin argumento = diff contra develop | un rango de git"
user-invocable: true
disable-model-invocation: false
---

Corre los revisores sobre **código que ya existe**. Para revisar decisiones que todavía son un
documento está `/revision-de-arquitectura`, que corre después de `/speckit-plan`.

## Qué diff revisas

Sin argumento: `git diff develop...HEAD --stat` más los archivos completos que cambiaron. Con
argumento, el rango que te den.

Si el diff está vacío, dilo en una línea y termina.

## Qué revisor aplica

**Míralo en los archivos que cambiaron, no lo adivines.** Correr un revisor sobre un dominio que el
cambio no tocó produce hallazgos inventados, y correr de menos deja pasar el que importaba.

| Corre | Cuando el diff toca |
|---|---|
| `go-backend-reviewer` | Cualquier `.go` bajo `server/` |
| `security-auditor` | `internal/auth/`, `internal/config/`, `internal/httpapi/middleware.go`, `internal/httpapi/router.go`, `internal/httpapi/ratelimit.go`, `internal/logging/`, o cualquier cosa que decida quién puede hacer qué |
| `tablet-ui-reviewer` | Una pantalla nueva o un cambio de disposición en `web/src/` |
| `db-architect` | `server/migrations/` o `server/queries/` — **solo si no corrió ya** en `/revision-de-arquitectura` sobre este mismo cambio; si ya corrió, no lo repitas |

El `security-auditor` no es negociable cuando el diff toca esa lista: el principio V lo exige antes
de mergear, y el hook no puede decidirlo por ti porque depende de qué cambió.

Lánzalos **en paralelo, en un solo mensaje**.

## Qué reportas

Un solo reporte consolidado, sin repetir lo que dos agentes digan igual:

1. **Primero lo bloqueante**: pérdida de datos, un principio no negociable roto (I, IV, V), una
   puerta del principio VIII cerrada. Con `archivo:línea`, el escenario que lo dispara y el fix.
2. Después el resto, por severidad.
3. **Un solo veredicto**: aprobado, o cambios requeridos con la lista.

Un hallazgo sin el fallo concreto que evita no se reporta. Si todos vuelven limpios, dilo en dos
renglones — un reporte largo que dice "todo bien" enseña a no leer los reportes.

## Qué NO haces

- **No arregles nada aquí.** Esta revisión es de solo lectura; quien decide qué corregir es el dueño.
- **No repitas lo que ya dice `golangci-lint`, `gosec` o `eslint`.** Esos ya corren en los hooks de
  lefthook y en CI; estos agentes existen para lo que un linter no ve.
- **No des por bueno un gate porque el commit pasó.** Si lefthook está bloqueado por Smart App
  Control (`AGENTS.md` §7), que el commit pase no significa nada — los gates se corren a mano.
