---
name: "nuevo-spec"
description: "Arranca una feature nueva en su propio worktree: carpeta, rama desde develop, infraestructura y base propias, deploy/.env, dependencias y la API local respondiendo; luego dice qué pegar en la sesión nueva. Úsalo antes de /speckit-specify de cualquier feature, y para cerrar el worktree de una feature ya mergeada."
argument-hint: "NNN slug [descripción de la feature] | cerrar <carpeta> [--datos]"
user-invocable: true
disable-model-invocation: false
---

Toda feature se trabaja en **su propio worktree**: su carpeta, su rama, su proyecto de compose, su
Postgres y su Redis, sus puertos y su `deploy/.env`. Dos features en la misma carpeta o en la misma
base se pisan (migraciones, caché del menú, contraseñas de roles); ver `AGENTS.md` §2, «Varios
worktrees a la vez». Esta skill lo deja listo sin que el dueño corra un solo comando.

## Arrancar: `/nuevo-spec 041 subidas-sin-corte [descripción]`

1. **Número libre.** Revisa que `NNN` no esté usado: `ls specs/`, `git ls-remote --heads origin`
   (ramas `NNN-*`) y las carpetas `../egb-NNN-*`. Si choca, dilo y propone el siguiente. Revisa
   también el número de la siguiente migración en `server/migrations/`, y si otra rama abierta
   ya la tomó (anótalo para el plan).
2. **Crea todo con el script**, en segundo plano si tarda:
   ```bash
   make nuevo-worktree n=NNN slug=SLUG
   ```
   Termina con `✅ Listo` y la API respondiendo `/readyz`, o con `✗` y los últimos errores del log
   (`/tmp/egbNN-start.log`). Si falla, arregla la causa en el script —no a mano en el worktree—
   para que la siguiente feature no tropiece igual.
3. **Datos reales en local**, solo si la feature los necesita (migración, reportes): con la API
   detenida, `make db-restaurar dump=…` dentro del worktree nuevo. Nunca `--no-owner` ni
   `--no-privileges` (constitución IV).
4. **Entrega al dueño**, en pocas líneas:
   - la carpeta, la API y el web (`http://localhost:80NN` y `:30NN`), y dónde está la contraseña
     del admin (`deploy/.env` de ESE worktree; no la copies al chat);
   - el comando para abrir la sesión nueva: `cd <carpeta> && claude`;
   - **el prompt listo para pegar**: `/speckit-specify <descripción>`, con el número, la rama ya
     creada («no crear otra»), el directorio `specs/NNN-slug`, el origen (quién lo pidió y cuándo),
     lo medido que lo motiva, el alcance, lo que queda fuera y las reglas del repo que aplican.
     Si el dueño no dio descripción, pídesela antes de escribir el prompt.

La sesión que trabaja la feature corre los comandos locales ella misma (`make start`,
`make ci-local`, pruebas): el dueño no los teclea.

## Cerrar: `/nuevo-spec cerrar ../egb-041-subidas-sin-corte [--datos]`

Cuando la feature ya está mergeada en develop:
```bash
make cerrar-worktree                                          # desde dentro del worktree; datos=1 borra también su base
make cerrar-worktree dir=../egb-041-subidas-sin-corte         # desde otro worktree
```
Nunca desde `~/git/el-gato-bobah-pos`: es un repositorio bare y no tiene `Makefile`.
Se niega si hay cambios sin commit o commits que no están en origin. Nunca toca el proyecto
`deploy` de la carpeta principal. Borra la rama local solo si ya está en develop.

## Qué NO haces

- No trabajas una feature en la carpeta principal ni en el worktree de otra.
- No apuntas un worktree a la base de otro, ni las pruebas de integración a la base de la API.
- No corres `docker compose down` a mano: siempre `make stop` / `make deps-down` del worktree.
