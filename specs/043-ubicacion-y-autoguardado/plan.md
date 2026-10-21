# Implementation Plan: Ubicación, autoguardado de la ficha y empaques claros

**Branch**: `043-ubicacion-y-autoguardado` | **Date**: 2026-10-20 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/043-ubicacion-y-autoguardado/spec.md`

## Summary

Seguimiento de la 042 con las decisiones del dueño del 2026-10-20. Casi todo es pantalla:

- **Ubicación**: el «lugar para contar» de la spec 040 se llama «Ubicación» en las pantallas del
  almacén, con «Sin definir» y alta desde el campo, como Categoría. En el servidor solo cambia un
  permiso: crear una ubicación pasa de `inventory.count` a `inventory.configure`.
- **Autoguardado de la ficha**: cada campo manda su propio `PATCH` tras 2 segundos sin escribir o al salir del campo, en una
  cola de uno a la vez, con un indicador junto al título. Se quita «Guardar», y «¿Descartar
  cambios?» solo aparece si algo quedó sin guardar. Antes, #120 se arregla con su prueba.
- **Hoja de empaque nuevo**: botones base fijos por clase de unidad, «Otro» sin preseleccionar,
  frase viva «1 bolsa = 5 kg».
- **Existencia negativa** en rojo con «Revisar con un conteo».
- **Pruebas de pantalla**: `gestion-de-articulos.spec.ts` nueva y la de la 040 al flujo nuevo.

**Sin migración**: `count_places` ya normaliza el nombre (`count_places_name_key`) y lo topa a 30
caracteres (0095). Los empaques base no se guardan.

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc; Chakra UI v3, TanStack Query. Sin dependencias nuevas.

**Storage**: PostgreSQL 16 con RLS. Sin cambios de esquema.

**Testing**:
- Integración: crear ubicación sin `inventory.configure` da 403; con él, una repetida devuelve la que
  existe; `PATCH` de un solo campo no toca los demás; el mismo mínimo dos veces deja el mismo valor.
- vitest: cola de guardado (orden, uno a la vez, conservar lo escrito al fallar por la red y revertir con un
  rechazo del servidor, «‹ Artículos» espera), campo
  Ubicación (búsqueda, «Crear "X"», aviso sin permiso, ejemplo sin ubicaciones), hoja de empaque
  (botones por clase, «Otro», frase viva) y existencia negativa.
- Playwright a 1024×600 contra el ambiente de pruebas: `gestion-de-articulos.spec.ts` y
  `lugares-para-contar.spec.ts`.

**Target Platform**: tabletas de 7 a 10 pulgadas (1024×600)

**Project Type**: web (server + web)

**Constraints**: controles de 44 px o más, sin selector nativo, nada se guarda a medias.

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | El permiso se decide en `router.go`. La cola de guardado es lógica de pantalla, en un hook propio con sus pruebas. Los empaques base son una constante en `web/` |
| II Errores | Ubicación repetida responde la existente (como hoy); sin permiso, 403. Un `PATCH` sobre una ficha que cambió, 409 `STALE` (ya existe) |
| III Dinero | No toca dinero. «Corregir costo» conserva su hoja |
| IV Tests | #120 se reproduce con una prueba que falla antes de tocar el código. Cada borde de la cola deja su vitest. El cambio de permiso deja su prueba de integración en los tres casos |
| V Seguridad | Crear ubicación pasa a `RequirePermission(inventory.configure)`; la pantalla espeja con `can()`, solo como UX. Ningún `RequireRole` nuevo |
| VI YAGNI | Sin tabla de empaques base (#121 lo hará si una empresa lo pide). Sin selección múltiple (#122) |
| VII Idioma | Identificadores en inglés; texto de pantalla en español |
| VIII Puertas | Ver abajo |
| Tableta | `Picker` con buscador y alta inline; indicador junto al título sin robar alto; botones de empaque de 44 px |
| Migración tolerante | No hay migración. El cambio de permiso solo restringe: la versión anterior de la pantalla, que deja crear desde el conteo con `inventory.count`, recibe 403 y su aviso de error durante el traslape |

### Pregunta VIII: ¿se puede agregar después al mismo costo?

- **Varios almacenes (#65)**: no se cierra. «Ubicación» sigue siendo solo el orden del conteo, y la
  pantalla no la llama almacén.
- **Roles por empresa**: el control nuevo pregunta por permiso.
- **Catálogo de empaques editable (#121)**: los base viven en una constante; pasarlos a la base después
  cuesta una tabla y una semilla, sin tocar datos existentes.

## Pantallas y maquetas

| Pantalla | Qué cambia | Alto a 1024×600 |
|---|---|---|
| **Lista de artículos** | «Lugar» → «Ubicación» en columna y filtro; «Sin lugar» → «Sin definir». Sin botón en el renglón | Igual: 6 renglones de 44 px |
| **Ficha del artículo** | Sin «Guardar». Texto y números guardan tras 2 s sin escribir y al salir del campo; Enter es atajo. Junto al título, un indicador: un punto discreto con cambios sin mandar, «Guardando…» con spinner, «Guardado ✓» o «No se pudo guardar · Reintentar». Sin campo que se deshabilite al guardar. Campo «Ubicación» con buscador, «Crear "X"», ejemplo «Ej. Refri, Bodega, Barra» sin ubicaciones y «Pídele a un administrador que la cree» sin permiso. Existencia negativa en rojo con «Revisar con un conteo». Costo, unidad y archivar sin cambios de forma; su botón de confirmar con spinner | Encabezado de 44 px igual que hoy: el indicador ocupa el lugar de «Guardar» |
| **Hoja de empaque nuevo** | «¿En qué empaque lo compras?» con botones base por clase más los aprendidos, ninguno elegido, «Otro» abre el campo «Ej. Cubeta, Garrafa». Una línea de ayuda arriba. Sin la fila de ubicación. Debajo de «Trae», la frase «1 bolsa = 5 kg». Selector de unidad igual al de «Mínimo» | Hoja de ~420 px |

### Maquetas

Decisión ya tomada por el dueño (2026-10-20): una sola opción por pantalla.

| Pantalla | Maqueta | Elegida | Por qué |
|----------|---------|---------|---------|
| Ficha del artículo | https://claude.ai/artifact/Ap2awvT4Kc2W97XFSrdpNh | Única: autoguardado con indicador | Decisión del dueño tras la prueba en tableta |
| Hoja de empaque nuevo | https://claude.ai/artifact/LsDzLDo5JZsLLjCE5hwhEt | Única: botones base y frase viva | Decisión del dueño tras la prueba en tableta |

El código de cada maqueta está en [maquetas/](maquetas/). La lista solo cambia texto: no lleva maqueta.

## Capas

| Pieza | Capa | Dónde |
|---|---|---|
| Crear ubicación pide `inventory.configure` | httpapi | `router.go` (`POST /stock/count-places`) |
| `PATCH /stock/articles/{id}` acepta un solo campo sin tocar los demás y responde el `updatedAt` nuevo (R3) | app + httpapi | `app/ingredients_admin.go`, `handlers_ingredients_admin.go` |
| Arreglo de #120 donde esté la causa | por determinar | prueba primero |
| Cola de autoguardado: pausa de 2 s o salir del campo; uno a la vez, en orden; un campo con un guardado pendiente se reemplaza por su valor más nuevo en vez de mandar dos; al fallar por la red conserva lo escrito y revierte solo con un rechazo del servidor; esperar al salir | web | `features/backoffice/articles/useAutosave.ts` |
| Ficha con autoguardado, indicador (punto, «Guardando…», «Guardado ✓»), Ubicación y existencia negativa | web | `ArticleSheet.tsx` |
| Bitácora del artículo junta los guardados seguidos del mismo campo y usuario en menos de un minuto | sqlc + app + web | `queries/ingredients_admin.sql` (`ArticleAudit`), `app/ingredients_admin.go`, `ArticleAuditSheet.tsx` |
| Campo de ubicación con alta según permiso y textos nuevos | web | `CountPlaceField.tsx` |
| Lista: «Ubicación» y «Sin definir» | web | `ArticlesTab.tsx`, `NewArticleSheet.tsx` |
| Empaques base por clase y hoja nueva | web | `shared/units.ts` o `articleUnits.ts`, `PresentationsSheet.tsx` |
| Pruebas de pantalla | e2e | `web/e2e/gestion-de-articulos.spec.ts`, `web/e2e/lugares-para-contar.spec.ts` |

## Bordes (se piensan antes del código)

| # | Borde | Cómo se cierra | Test |
|---|---|---|---|
| E1 | Dos cambios seguidos en campos distintos | Cola: el segundo sale cuando responde el primero, con el `updatedAt` que devolvió | vitest |
| E2 | El guardado falla | Por la red: el campo conserva lo escrito, marcado, y «Reintentar» manda lo que se ve. Rechazo del servidor (409, 400): vuelve a lo guardado y dice por qué | vitest |
| E3 | Otra persona cambió la ficha | 409 `STALE`: se avisa y se recarga; no se pisa | vitest e integración (ya existe) |
| E4 | Salir con un guardado en curso, con el foco todavía en el campo, o con el último guardado fallido | Toda salida guarda el campo enfocado y espera la cola; con algo sin guardar pregunta «¿Descartar cambios?» (R2) | vitest |
| E12 | Pausa a media cifra o valor que todavía no es número («1.», «-», vacío) | La pausa de 2 s solo manda valores completos; lo incompleto se valida al salir del campo (FR-022) | vitest con reloj simulado |
| E13 | Bitácora llena de guardados de un mismo campo | Se juntan al leer: mismo artículo, campo, usuario y acción, cada uno a menos de 60 s del anterior (reloj de la base), con el antes del primero y el después del último | integración de la consulta y vitest de la hoja |
| E5 | Salir de un campo sin cambiarlo | No se encola nada | vitest |
| E6 | Dato inválido | Se marca y no se encola | vitest |
| E7 | El mismo mínimo dos veces (#120) | Prueba que guarda dos veces y compara | vitest e integración |
| E8 | Ubicación que solo difiere en acentos | El servidor devuelve la existente (0095) | integración (ya existe) y vitest |
| E9 | Crear ubicación sin permiso desde una pantalla vieja | 403 con aviso | integración en los tres casos |
| E10 | Empaque aprendido igual a uno base | Se muestra una vez | vitest |
| E11 | Recargar con un guardado en curso | Se pierde lo no guardado; la ficha muestra lo del servidor | aceptado, documentado |

## Revisión after_plan (2026-10-20)

`/revision-de-arquitectura`: `tablet-ui-reviewer` aprobó con cambios; `db-architect` no corrió porque
el plan no agregaba tablas, columnas, índices ni consultas. La segunda ronda de decisiones cambia la
consulta `ArticleAudit` (E13): `db-architect` la revisa antes de implementarla (T027). Lo que se aplica:

**Altos**
- **R1 — el guardado no desmonta la ficha.** Hoy `refresh()` deja la ficha en espera y vuelve a llenar
  el formulario completo desde el servidor: el teclado se cierra y se pierde lo que se escribía en
  otro campo. El éxito de un guardado solo actualiza `updatedAt` y lo guardado de ese campo; un campo
  con borrador sin guardar no se sobrescribe. Se quita el aviso flotante «Guardado» (el indicador ya
  lo dice). Prueba: «escribo en B mientras A guarda».
- **R2 — salir sin perder.** En la tableta tocar un botón no siempre quita el foco del campo, así que
  salir debe forzar el guardado del campo enfocado. «‹ Artículos», el fondo y Escape pasan por la misma
  función: guarda el campo enfocado, espera la cola y, si algo quedó sin guardar, pregunta
  «¿Descartar cambios?». Se agregan a E4 «foco todavía en el campo» y «la espera termina en error».

**Medios**
- **R3 — `PATCH` por campo es tarea fija**, con prueba: un campo no toca los demás, tampoco con `null`
  explícito, y la respuesta trae el `updatedAt` nuevo (si no, el siguiente guardado da 409 contra sí
  mismo).
- **R4 — «Crear "X"» va primero**, debajo del buscador, cuando lo escrito no coincide con ninguna:
  con el teclado abierto en 600 px quedaba tapado. El buscador no toma el foco al abrir.
- **R5 — hoja de empaque con teclado**: la frase viva va pegada al pie junto a «Guardar empaque». Se
  mide en e2e con el alto reducido por el teclado. La fila de ubicación sale de la hoja (decisión del
  dueño, 2026-10-20): se elige en la ficha.
- **R6 — el empaque aproximado se conserva**: un artículo en piezas sigue pudiendo decir «1 caja ≈ N
  piezas» por peso o volumen. Los botones base no lo quitan.
- **R7 — g/kg del mínimo guarda al tocarlo**, solo si el campo tiene número, y conserva la cantidad
  base (cambia cómo se ve, no cuánto es).
- **R8 — permiso en el conteo**: el mismo campo se usa al contar; sin `inventory.configure` tampoco
  ofrece crear ahí y dice «Pídele a un administrador que la cree». Sus avisos dicen «ubicación».

**Bajos**
- La existencia negativa va en su propio renglón, con texto además del rojo.

**Decisiones del dueño (2026-10-20, tras esta revisión)**
- Si un guardado falla por la red, el campo conserva lo escrito, marcado, con «Reintentar»; vuelve a lo
  guardado solo si el servidor lo rechaza. Al salir con algo sin guardar aparece «¿Descartar
  cambios?»; con todo guardado, nunca.
- Ningún campo se deshabilita mientras guarda (cerraría el teclado): la cola cubre la concurrencia. En
  listas e interruptores, un segundo toque mientras guardan se ignora.
- La hoja de empaques pierde la fila de ubicación.

**Decisiones del dueño (2026-10-20, segunda ronda)**
- Texto y números se guardan tras 2 segundos sin escribir y también al salir del campo. Enter es atajo,
  nunca requisito: en tableta casi nadie lo usa y tocar una zona vacía no siempre quita el foco.
- Un punto discreto junto al título mientras hay cambios sin mandar; luego «Guardando…» y «Guardado ✓».
- La bitácora junta los guardados seguidos del mismo campo y usuario en menos de un minuto.

### Cómo se junta la bitácora (propuesta)

`audit_events` no admite `update` ni `delete` (trigger de la 0089), así que juntar al escribir exigiría
cambiar esa regla. Se junta al **leer**, en `ArticleAudit`:
- Dos eventos van en la misma entrada si son del mismo artículo, la misma acción (`ingredient.updated`),
  el mismo usuario y el mismo y único campo cambiado, y cada uno ocurrió a menos de 60 segundos del
  anterior según `occurred_at` (reloj de la base). Una cadena puede durar más de un minuto mientras no
  haya un hueco de 60 s.
- La entrada muestra el `before` del primero y el `after` del último, la hora del último y «N cambios».
- Un evento que cambia dos campos, de otro usuario o de otra acción, corta la cadena.
- Se agrupa en SQL con funciones de ventana (`lag` para marcar dónde empieza cada grupo), antes de
  paginar, para que «Ver más» no parta una entrada en dos páginas.
- Nada se pierde: Negocio › Bitácora sigue mostrando cada evento.

## Al construir

- **#120 era de pantalla, no del servidor, y era la misma causa que #119.** Tras guardar, `refresh()`
  vaciaba el formulario y el efecto que lo llena lo volvía a llenar en ese instante con la ficha vieja
  de la caché, antes de que llegara la nueva. El mínimo regresaba a su valor anterior (vacío, en el
  caso común), el guardado siguiente lo mandaba como `null`, y así alternaba en cada guardado, en
  todas las clases de unidad. La ficha también quedaba «con cambios» contra lo recién guardado, de ahí
  el «¿Descartar cambios?» falso de #119. Reproducido en local a 1024×600 con g, kg, ml y piezas. Se
  arregla esperando la ficha nueva y llenando el formulario con ella; lo cubre la prueba «tras guardar,
  la ficha muestra lo guardado y salir no pregunta (#120, #119)». «Nuevo artículo» no tiene el
  defecto: abre la ficha ya creada.
- **El guardado por campo podía borrar datos durante una subida.** El front (Cloudflare Pages) y la API
  se despliegan por separado. Probado en local contra la API anterior: guardar solo el nombre dejó el
  mínimo en nulo, porque la API anterior toma un campo ausente como vacío. Por eso la pantalla manda la
  ficha completa en cada guardado (lo que cambió más lo último guardado de lo demás). La API nueva
  sigue aceptando un solo campo (R3) y deja en la bitácora solo lo que cambió. Si la respuesta no trae
  la versión nueva (la API anterior responde 204), la pantalla pide la ficha para seguir.
- **Al volver a abrir la ficha aparecía el nombre de antes de guardar.** El formulario se llenaba con
  la ficha en caché antes de que llegara la nueva. Ahora se llena solo con la que se pide al abrir, y
  cada guardado actualiza esa caché.
- **Un error dentro del éxito de un guardado dejaba la cola en «Guardando…» para siempre**: ahora cae en
  el mismo manejador de fallos.
- **`db-architect` revisó `ArticleAudit` agrupada antes de escribirla (T027): cambios requeridos, aplicados.**
  `jsonb_object_keys` truena con un `after` que no es objeto, y eso tumbaba la bitácora entera del
  artículo: ahora `jsonb_typeof` va primero, dentro de un `case`. Sin agregados (`row_number`,
  `first_value`), para que sqlc tipe bien las columnas y la entrada lleve la hora y el id de su último
  evento, que es la llave del cursor. Límite aceptado: un evento que se inserte con una hora vieja (una
  transacción larga) puede partir o unir una entrada ya vista. Cada página relee la historia del
  artículo (`ponytail:` en la consulta).

## Project Structure

```text
specs/043-ubicacion-y-autoguardado/
├── spec.md, plan.md, tasks.md
└── checklists/requirements.md

server/internal/httpapi/router.go
server/internal/app/ingredients_admin.go (PATCH por campo, R3)
server/internal/integration/count_places_test.go, ingredients_admin_test.go
server/queries/ingredients_admin.sql (ArticleAudit agrupada, E13)

web/src/features/backoffice/
├── articles/ (ArticleSheet, ArticlesTab, NewArticleSheet, ArticleAuditSheet, useAutosave, articleUnits)
├── CountPlaceField.tsx, PresentationsSheet.tsx
web/e2e/gestion-de-articulos.spec.ts, lugares-para-contar.spec.ts
```

**Structure Decision**: monorepo existente, sin paquetes nuevos.

## Complexity Tracking

Sin excepciones.
