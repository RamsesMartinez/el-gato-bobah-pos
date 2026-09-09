---
name: "revision-de-arquitectura"
description: "Lanza en paralelo a los revisores de diseño (db-architect y tablet-ui-reviewer) contra el plan de la feature activa y consolida sus hallazgos. Lo dispara el hook after_plan de spec-kit; también se puede invocar a mano antes de escribir una migración."
argument-hint: "sin argumento = la feature activa | ruta a un plan.md"
user-invocable: true
disable-model-invocation: false
---

Corre los revisores de **diseño** sobre un plan que todavía no es código.

El punto del ciclo importa: en `plan.md` las decisiones de esquema y de pantalla ya están tomadas
pero todavía no cuestan una migración sobre datos vivos ni una pantalla rehecha. Un hallazgo aquí
se arregla editando un documento; el mismo hallazgo en `after_implement` se arregla tocando
producción.

Esta skill **no revisa código** — para eso están `go-backend-reviewer` y `security-auditor`, que
corren cuando ya hay algo que leer.

## Qué plan revisas

Sin argumento, el de la feature activa: lee `feature_directory` de `.specify/feature.json` y usa
`<feature_directory>/plan.md`. Con argumento, la ruta que te den.

Si no hay `plan.md`, **para y dilo**. No inventes una revisión sobre el spec: el spec dice qué se
quiere y el plan dice cómo, y las decisiones que esta revisión mide viven en el segundo. Un plan
faltante es el hallazgo, y significa que la feature se saltó `/speckit-plan`.

## Cómo lo revisas

Lanza los dos agentes **en paralelo, en un solo mensaje**, con el contenido del plan y la ruta de
su `spec.md` hermano:

- **`db-architect`** — siempre que el plan mencione una tabla, una columna, un índice, una
  migración o una consulta nueva. Su sección 7 es la que importa aquí: qué puerta cierra cada
  decisión, contra la tabla del principio VIII de la constitución.
- **`tablet-ui-reviewer`** — siempre que el plan describa una pantalla nueva o cambie una
  disposición en `web/`.

Si el plan no toca ninguno de los dos dominios, dilo en una línea y termina. Correr un revisor que
no tiene nada que revisar produce hallazgos inventados.

## Qué reportas

Consolida los dos reportes en uno solo, sin repetir lo que ambos digan:

1. **Primero lo bloqueante.** Un hallazgo que cierra una puerta del principio VIII, que rompe un
   principio no negociable (I, IV, V) o que puede perder datos va arriba, con `archivo:línea` y el
   fix concreto.
2. Después el resto, por severidad.
3. Cierra con **un solo veredicto**: aprobado, o cambios requeridos con la lista de qué cambiar.

Un hallazgo sin el escenario concreto que lo dispara no se reporta. Y si los dos agentes vuelven
limpios, dilo en dos renglones: un reporte largo que dice "todo bien" enseña a no leer los reportes.

## Qué NO haces

- **No edites el plan.** Esta revisión es de solo lectura; quien decide qué corregir es el dueño.
- **No conviertas un hallazgo de puerta abierta en una petición de features.** El veredicto correcto
  casi siempre es *"no lo construyas, pero deja la columna que permite construirlo"*.
- **No repitas lo que ya dice un linter** ni recites teoría.
