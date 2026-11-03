---
name: "maquetas"
description: "Genera 2 o 3 maquetas navegables a 1024×600 por cada pantalla nueva o cambiada del plan de la feature activa, las publica como páginas privadas, pide al dueño que elija una por pantalla y deja la decisión escrita en plan.md. Lo dispara el hook after_plan de spec-kit; también se puede invocar a mano."
argument-hint: "sin argumento = la feature activa | ruta a un plan.md"
user-invocable: true
disable-model-invocation: false
---

Convierte la sección de pantallas del plan en **opciones que el dueño puede ver y tocar** antes de
que exista una línea de código.

**Por qué existe.** Una pantalla descrita en una tabla se aprueba sin haberla visto, y el defecto
aparece en la tableta, cuando corregirlo ya es rehacer componentes. El dueño pidió desde el
2026-10-09 «3 opciones navegables por flujo» antes de construir; la regla vive en la constitución
(*Quality gates*). Un recordatorio se olvida; un paso del ciclo, no.

## Qué plan lees

Sin argumento, el de la feature activa: `feature_directory` de `.specify/feature.json` →
`<feature_directory>/plan.md`. Con argumento, la ruta que te den. Lee también el `spec.md` hermano:
las historias dicen qué tiene que lograr cada pantalla.

Si no hay `plan.md`, **para y dilo**: las maquetas salen de las decisiones del plan.

**Si el plan no crea ni cambia ninguna pantalla en `web/`, dilo en una línea y termina.** Una maqueta
de algo que no cambia es tiempo del dueño tirado.

## Qué produces

Por cada pantalla nueva o con disposición cambiada (no por cada texto que cambia):

1. **2 o 3 opciones que de verdad difieran** en lo que el dueño tiene que decidir: dónde vive la
   acción, cuántos toques cuesta, qué se ve sin desplazar. Tres variaciones de color no son tres
   opciones. Si solo hay una disposición razonable, haz una y dilo.
2. **Navegables**: los toques principales cambian de paso (elegir persona → PIN → confirmación), con
   datos de ejemplo. Nada se conecta a la API.
3. **Dentro de un marco fijo de 1024×600** (el presupuesto real de la constitución), con el riel
   lateral de 76 px cuando la pantalla vive dentro del POS. Controles ≥ 44 px, sin `<select>` ni
   `type=time` nativos, texto para quien opera. Una opción que viola eso no se presenta.
4. **Con la apariencia real del POS**: colores y tipografía de `web/src/theme/ui.ts` y los
   componentes que ya existen (`Picker`, `Page`, la hoja inferior). Una maqueta más bonita que el
   producto vende algo que no se va a construir.
5. **Datos de ejemplo inventados.** Nunca nombres de empleados, ventas, costos ni proveedores reales:
   las páginas son privadas, pero se comparten.

Cada pantalla es **una página** con sus opciones como pestañas (A, B, C) y, debajo del marco, una
línea por opción: qué privilegia y qué cuesta (en toques o en espacio). Publícala con la herramienta
de Artifact, cargando antes el skill `artifact-design` como esa herramienta exige.

Cuando un flujo cruza varias pantallas o la decisión es de secuencia, agrega un diagrama del flujo
en la misma página.

## Cómo pides la decisión

Una pantalla a la vez, con `AskUserQuestion`: la liga de la página, las opciones con su
privilegia/cuesta, y tu recomendación primero, con el porqué. El dueño puede elegir una, pedir
cambios (rehaces esa página y vuelves a preguntar) o combinar dos.

No avances a la siguiente pantalla sin respuesta. No elijas por el dueño.

## Dónde queda la decisión

En `plan.md`, sección **«Maquetas»** (la plantilla la trae): una fila por pantalla con la liga de la
página, la opción elegida y por qué en una línea. Después **ajusta la sección de pantallas** del plan
para que describa la opción elegida, sin dejar descrita la descartada. Si la elección contradice
algo de `data-model.md` o `contracts/`, corrígelo también y dilo.

Las tareas de pantalla que genere `/speckit-tasks` deben citar la fila de «Maquetas» que implementan.

## Orden en el ciclo

Corre en `after_plan` **antes** de `/revision-de-arquitectura`: el revisor de tableta revisa la
opción elegida, no tres candidatas. Si el plan ya tiene la sección «Maquetas» completa para todas
sus pantallas, no regeneres nada: dilo y termina.

## Qué NO haces

- No escribes código de la app ni tocas `web/`.
- No publicas una opción que viole la restricción de tableta para «mostrar el contraste».
- No conviertes la maqueta en un spec nuevo: si al verla el dueño pide algo que el spec no tiene,
  anótalo y sugiere `/speckit-clarify`.
