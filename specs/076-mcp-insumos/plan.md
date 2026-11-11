# Implementation Plan: Crear y editar insumos desde el servidor MCP, y empaques por insumo

**Branch**: `076-mcp-insumos` | **Date**: 2026-11-11 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/076-mcp-insumos/spec.md`

## Summary

Dos escrituras nuevas y un filtro sobre la puerta para agentes de la 072–074, sin ruta nueva en la
API, sin migración y sin alcance nuevo:

| Herramienta | Ruta que despacha | Lecturas de su vista previa | Permiso de la ruta | Alcance |
| --- | --- | --- | --- | --- |
| `ingredient_create` | `POST /api/v1/stock/articles` | `GET /stock/articles/similar?name=` (configurar), `GET /stock/articles?q=` (ver el almacén) | `inventory.configure` | `write:inventory` |
| `ingredient_update` | `PATCH /api/v1/stock/articles/{id}` y luego `PUT /api/v1/stock/articles/{id}/alt-names` | `GET /stock/articles/{id}` (la ficha y su versión), las dos búsquedas de arriba, `GET /stock/articles/{id}/alt-names/similar?name=` | `inventory.configure` | `write:inventory` |
| `presentations_list` (+ `ingredient_id`) | `GET /api/v1/stock/presentations` o `GET /api/v1/stock/ingredients/{id}/presentations` | — | `inventory.receive` | `read` |

Más: las pistas para el agente de tres códigos de la API (`NAME_TAKEN`, `ALT_NAME_TAKEN`, `STALE`),
que hoy pierden el id del insumo que ya tiene el nombre; y el texto del alcance de almacén en
Integraciones (FR-013).

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript + React 19 (web)

**Primary Dependencies**: las de la 072 (SDK de MCP); ninguna nueva.

**Storage**: PostgreSQL — ningún cambio. **No hace falta migración**: la 0113 queda libre (revisado
el 2026-11-11 contra `origin/develop`, las ramas remotas y `fix/offset-sin-tope`).

**Testing**: integración contra Postgres real con la API completa servida como `gatobobah_app`
(`internal/integration/mcp_076_test.go`), vista en rojo antes del código; unitarios de las pistas de
error en `domain`; `TestEveryToolMatchesItsRouteGate` cubre las dos herramientas nuevas sin cambio;
vitest del texto de Integraciones; caso de aceptación con cliente MCP real contra `make start` y el
respaldo de producción del 2026-11-11.

**Target Platform**: la API en la VM; la pantalla en tableta a 1024×600.

**Project Type**: web-service + web-app (monorepo existente).

**Performance Goals**: la vista previa del alta cuesta dos lecturas; la de la edición, hasta 2 + 10
(una por cada otro nombre nuevo, tope de la ruta: 10 otros nombres). Aplicar cuesta una lectura de la
ficha y una o dos escrituras.

**Constraints**: ninguna herramienta puede más que la pantalla; ninguna regla de negocio en la capa
MCP (ni comparar nombres, ni validar unidades, ni topes de largo); la API anterior y la nueva
conviven (no hay cambio de esquema).

**Scale/Scope**: 2 herramientas nuevas (54 en total) y un parámetro opcional.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple | Estado |
| --- | --- | --- |
| I. Layering | `domain/apitoken.go`: catálogo (dos renglones nuevos y la descripción de `presentations_list`) y `toolFixByCode` (tres códigos) más la pista con el id del insumo que ya existe. `httpapi/mcp_tools_076.go`: las herramientas, que solo arman el cuerpo de la pantalla y despachan. `app` y `store` no cambian | OK |
| II. Errores | `NAME_TAKEN`, `ALT_NAME_TAKEN` y `STALE` entran a la pista con su corrección en palabras; el resto lo mapea `httpapi.Error` como siempre. El error del segundo paso de la edición se envuelve diciendo que el nombre ya quedó | OK |
| III. Dinero | El costo inicial viaja como texto a la ruta, que valida y redondea | OK |
| IV. Test-first | `mcp_076_test.go` se escribe y se ve en rojo (herramienta desconocida / parámetro ignorado) antes de `mcp_tools_076.go`; cada borde de la tabla de abajo es un subtest; la pista con id, en `apitoken_test.go`, en rojo antes | OK |
| IV. Aislamiento | Un token de otra empresa no crea con el catálogo de ésta, no edita su insumo ni lista sus empaques (subtest por herramienta). Las rutas ya tienen su prueba de los tres casos (042, 063, 038); no se agrega consulta | OK |
| IV. Migración | No hay | N/A |
| V. Seguridad | Ver «Seguridad». `security-auditor` en la revisión de código | OK, a revisar |
| VI. YAGNI | Sin archivar, reactivar, unir, unidad, costo, densidad ni composición; sin verbos de «agregar un otro nombre» (la lista completa es lo que manda la ficha) | OK |
| VII. Comentarios / idioma | Identificadores y nombres de herramienta en inglés; descripciones y errores en español | OK |
| VIII. Puertas | Ninguna se toca: el insumo ya es de la empresa y la existencia inicial cae en la sucursal que resuelve la ruta | OK |
| Tableta | Solo cambia el texto de un alcance; sin disposición nueva (ver «Pantallas y maquetas») | OK |

## Seguridad (principio V)

| Control | Diseño | Prueba |
| --- | --- | --- |
| Alcance | Cada herramienta declara su alcance en el catálogo; `apply` lo revisa en cada llamada | `token sin write:inventory`: no se ofrece y, llamada directa, se niega nombrando el alcance |
| Permiso | El de la ruta; `TestEveryToolMatchesItsRouteGate` comprueba contra el router que el permiso del catálogo es el de la primera ruta | rol con `inventory.view` y `inventory.receive` pero sin `inventory.configure`, con el token de alcance completo: no se ofrece; la ruta responde 403 y nada cambia |
| Despachador | Lista blanca: cada herramienta solo sus rutas y las lecturas de su vista previa. Archivar, reactivar, unir, unidad, costo y densidad quedan fuera | `TestDispatcherRefusesRoutesOutsideCatalog` ampliado con `…/archive`, `…/merge`, `…/unit`, `…/cost` |
| Versión de la ficha | La edición lee la ficha en la misma llamada y manda su versión; si otro la cambió en medio, la ruta responde 409 `STALE` y `apply` libera la clave | `STALE` simulado cambiando la ficha entre la lectura y la escritura no es determinista: se prueba la pista de `STALE` en `domain` y el camino 409 con un nombre que choca |
| Idempotencia | La de `apply`. La edición son dos peticiones: si la segunda falla con 4xx la clave queda libre y el primer paso queda aplicado y dicho en el error | `diez veces la misma clave`, `nombre aplicado y otros nombres rechazados` |
| Bitácora | Cada llamada deja `mcp.tool_called`; el alta deja `ingredient.created` y la edición `ingredient.updated` (los pone la ruta) | subtest que lee el evento con el después |
| Empresa | La del token: las rutas corren bajo la sesión del token y RLS | subtest de otra empresa por herramienta |
| Ensanche de tokens (spec D2) | Sin cambio de esquema: un token emitido con `write:inventory` gana las dos escrituras si el rol tiene el permiso. Se dice en Integraciones y en el issue | `whoami` contra local con la fila del token «Token Gestion Claude» del respaldo (quickstart) |

### Ajustes de la revisión de código (2026-11-11)

| # | Hallazgo (revisor) | Decisión |
| --- | --- | --- |
| C1 | La vista previa de `ingredient_update` hacía una lectura por cada otro nombre sin tope: el de 10 solo lo revisa la ruta al guardar, y una llamada con 260 mil nombres (cabe en 1 MiB) eran 260 mil lecturas contando como una contra el límite del token (security-auditor, medio) | `check()` rechaza más de `domain.MaxAltNames` antes de despachar y la vista previa no repite nombres; `TestIngredientUpdateCapsAltNamesBeforeDispatching`, visto en rojo |
| C2 | Si la ficha se guardaba y los otros nombres fallaban con 5xx (o el PATCH no devolvía versión), el agente recibía «puede que no se haya aplicado» (go-backend-reviewer, medio) | 409 `PARTIAL` «La ficha ya quedó cambiada; no se sabe si los otros nombres»: la clave queda libre y repetir termina el trabajo; `TestIngredientUpdateSaysTheSheetWasSavedWhenTheAltNamesFail`, visto en rojo en los dos casos |
| C3 | Otros nombres agregados en la tableta entre la vista previa y aplicar se pisan sin STALE (go-backend-reviewer, bajo) | **Residuo aceptado**, marcado `ponytail:` en `ingredientUpdateRun`: cerrarlo exige mandar la versión de la vista previa como `fingerprint` |
| C4 | Con un nombre de solo espacios la vista previa buscaba parecidos de «» (go-backend-reviewer, bajo) | Se salta la búsqueda; la ruta decide al aplicar |
| C5 | Dos archivos podían registrar la misma herramienta y ganaba el último en silencio (go-backend-reviewer, bajo) | `mustRegisterOnce` truena al arrancar |
| C6 | En la edición a medias la bitácora MCP registra la llamada como rechazada (409), aunque la ficha cambió (security-auditor, bajo) | **Residuo aceptado**, escrito en `docs/mecanica/personal-y-permisos.md`: el cambio queda en `ingredient.updated` de la ruta, con el token como dispositivo |
| C7 | Faltaba fijar con prueba que otra empresa no ve los parecidos ni el id de un insumo de ésta, ni edita con su categoría o ubicación (security-auditor, bajo) | Subtests en `mcp_076_test.go`. Pasaron al escribirlos: el código ya lo cumplía y quedan como guarda |

## Casos de borde (antes del código)

| Forma de fallar | Familia | Test |
| --- | --- | --- |
| La vista previa del alta no dice nada de un parecido, o lo pone después del resumen | camino nuevo que se salta el control viejo (la hoja «Nuevo artículo» pregunta «¿Es alguno de estos?») | `la vista previa pone primero «quizá ya existe»` (nombre parecido) y `…por otro nombre` (coincide con un otro nombre) |
| El nombre repetido se crea, o el error no dice cuál ya existe | hermano que no se movió (la pantalla ofrece «Abrir ese»; el agente pierde el id) | `409 con el id del que ya existe` y unitario de la pista |
| Mandar existencia inicial y después la compra duplica la existencia | valor que significa algo | la nota de la vista previa (FR-004) |
| `ingredient_update` sin `category_id` la borra, o sin `alt_names` los vacía | valor vacío que significa algo | `cambiar solo el mínimo conserva categoría, ubicación y otros nombres` |
| `alt_names: []` no los quita | valor vacío | `lista vacía los quita` |
| Los otros nombres se aplican antes que el nombre y la ruta descarta «Jamón de pierna» por ser el nombre vigente | estado que no sobrevive | `renombrar y dejar el nombre viejo como otro nombre` |
| El segundo paso falla y el agente cree que no cambió nada | estado que no sobrevive | `nombre aplicado y otros nombres rechazados: el error lo dice` |
| `clear_category` con `category_id` | valor contradictorio | rechazo de argumentos |
| `presentations_list` con `ingredient_id` devuelve los de todos | parámetro presente ignorado (constitución V) | `filtra por insumo` (visto en rojo: hoy el parámetro se ignora) |
| Un token de otra empresa crea con una categoría o edita un insumo de ésta | aislamiento | subtest de otra empresa |

## Pantallas y maquetas

**Integraciones › Crear token**: solo cambia la línea del alcance de almacén; la disposición, los
componentes y la altura de cada casilla no cambian.

| Alcance | Hoy | Después |
| --- | --- | --- |
| Almacén | Mermas, ajustes de existencia, conteos y empaques de compra | Insumos y sus empaques, mermas, ajustes de existencia y conteos |

La línea nueva (63 caracteres) mide menos que la de Compras (70, la más larga hoy), así que ninguna
casilla gana un renglón a 1024×600. `/maquetas` no genera opciones para un cambio de texto (su
propia regla), y no hay maqueta que elegir.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Integraciones › Crear token | No aplica: cambia solo el texto de un alcance | Sin cambio de disposición | La línea más larga sigue siendo la de Compras; el revisor de tableta revisa el texto |

## Project Structure

### Documentation (this feature)

```text
specs/076-mcp-insumos/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/mcp-tools.md   # las dos herramientas y el parámetro nuevo
└── tasks.md
```

### Source Code (repository root)

```text
server/
├── internal/domain/apitoken.go (+_test)          # catálogo: 2 herramientas; pista con id; 3 códigos
├── internal/httpapi/mcp.go                        # toolHint pasa existingId/archived a la pista
├── internal/httpapi/mcp_tools_076.go             # nuevo: ingredient_create/update, presentations_list con filtro
├── internal/httpapi/mcp_tools_073.go             # presentations_list deja su simpleRead (se mueve a 076)
├── internal/httpapi/mcp_dispatch_test.go         # lista blanca ampliada
└── internal/integration/mcp_076_test.go          # nuevo
web/src/features/admin/IntegrationsPage.tsx (+test)
docs/mecanica/personal-y-permisos.md              # la puerta para agentes: qué agrega la 076
```

**Structure Decision**: monorepo existente; las herramientas nuevas en su propio archivo junto a las
de la 073 y la 074.

## Decisiones (dudas resueltas con la mejor recomendación)

Las de producto viven en el spec (D1–D9). Las técnicas, en [research.md](research.md) (R1–R6).

## Complexity Tracking

Sin violaciones.
