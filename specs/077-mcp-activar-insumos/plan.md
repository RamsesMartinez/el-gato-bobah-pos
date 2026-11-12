# Implementation Plan: Desactivar y reactivar insumos desde el servidor MCP

**Branch**: `077-mcp-activar-insumos` | **Date**: 2026-11-12 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/077-mcp-activar-insumos/spec.md`

## Summary

Una acción nueva en una herramienta que ya existe, un parámetro de lectura y dos huecos del archivado
cerrados en la consulta, sin ruta nueva, sin migración y sin alcance nuevo:

| Pieza | Ruta que despacha o lee | Permiso de la ruta | Alcance |
| --- | --- | --- | --- |
| `ingredient_update` + `active: false` | `POST /api/v1/stock/articles/{id}/archive` (`{confirmOnHand}`); vista previa: `GET /stock/articles/{id}` y `GET /stock/articles/{id}/archive-check` | `inventory.configure` (la revisión, `inventory.view`) | `write:inventory` |
| `ingredient_update` + `active: true` | `POST /api/v1/stock/articles/{id}/reactivate`; vista previa: `GET /stock/articles/{id}` | `inventory.configure` | `write:inventory` |
| `articles_search` + `include_inactive` | `GET /api/v1/articles?q=` como hoy; con el parámetro, además `GET /stock/articles?status=archived&q=` | `expenses.capture`; el agregado, `inventory.view` | `read` |
| `stock_levels` | `GET /api/v1/stock/levels` (ahora sin insumos archivados y con `is_active`) | `inventory.view` | `read` |
| Sugerencias de compra | `LookupSupplierItem` y `SuggestFromSupplierItems` dejan fuera lo aprendido que apunta a un insumo archivado | (la de la ruta, sin cambio) | — |

Más: las pistas para el agente de `ARCHIVE_HAS_STOCK` e `IN_USE`, y la de nombre repetido de un
archivado, que hoy manda a la pantalla (spec D6).

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript + React 19 (web)

**Primary Dependencies**: las de la 072 (SDK de MCP); ninguna nueva.

**Storage**: PostgreSQL — ningún cambio de esquema. **No hace falta migración**: la 0113 queda libre
(revisado el 2026-11-12 contra `origin/develop` y todas las ramas remotas). Cambian dos consultas
(`supplier_items.sql`, `stock.sql`) y su código generado por sqlc.

**Testing**: integración contra Postgres real con la API completa servida como `gatobobah_app`
(`internal/integration/mcp_077_test.go`), vista en rojo antes del código; el comportamiento del
archivado (conteo, compras, sugerencias, existencias, historial) en `internal/integration/archived_ingredient_test.go`
con los tres casos de aislamiento donde cambia una consulta; unitarios de las pistas en `domain`;
`TestEveryToolMatchesItsRouteGate` sin cambio; vitest del texto de la tabla vacía de Existencias
(A1); caso de aceptación con cliente MCP real contra `make start` y el respaldo de
producción restaurado.

**Target Platform**: la API en la VM; la pantalla en tableta a 1024×600.

**Project Type**: web-service + web-app (monorepo existente).

**Performance Goals**: la vista previa de desactivar cuesta dos lecturas (ficha y revisión); la de
reactivar, una. `include_inactive` agrega una lectura.

**Constraints**: ninguna regla de negocio en la capa MCP (ni existencia, ni recetas, ni nombre); la
API anterior y la nueva conviven (no hay cambio de esquema; las consultas cambiadas solo filtran).

**Scale/Scope**: 0 herramientas nuevas (54 en total), 3 parámetros (`active`, `confirm_on_hand`,
`include_inactive`), 3 consultas.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple | Estado |
| --- | --- | --- |
| I. Layering | `domain/apitoken.go`: descripciones del catálogo y pistas (`ARCHIVE_HAS_STOCK`, `IN_USE`, archivado en `ToolErrorHintWith`). `httpapi/mcp_tools_076.go`: la acción de `ingredient_update` y `articles_search` con el parámetro (se mueve de la 073), que solo arman el cuerpo de la pantalla y despachan. `store/queries`: las tres consultas, regeneradas con sqlc. `app` no cambia: `ArchiveArticle`/`ReactivateArticle` ya tienen las reglas | OK |
| II. Errores | Los rechazos los mapea `httpapi.Error` como siempre; la capa MCP solo agrega la corrección en palabras por código | OK |
| III. Dinero | No toca dinero ni cantidades (la existencia se muestra como la devuelve la revisión) | N/A |
| IV. Test-first | Primero las pruebas del archivado (verde donde ya cumple; rojo donde hay hueco: sugerencias aprendidas y existencias); después `mcp_077_test.go` en rojo (parámetro desconocido) antes del código | OK |
| IV. Aislamiento | Cambian `ListStockLevels`, `LookupSupplierItem` y `SuggestFromSupplierItems`: cada una con su prueba bajo `appRoleStore` e `inTheThreeCases`. Un token de otra empresa no desactiva ni ve la revisión de un insumo de ésta | OK |
| IV. Migración | No hay. La consulta nueva tolera a la versión anterior corriendo (solo filtra) | N/A |
| V. Seguridad | Ver «Seguridad». `security-auditor` en la revisión de código | OK, a revisar |
| V. Parámetro inválido | `confirm_on_hand` sin `active: false` y `active` con otros cambios se rechazan; no se ignoran | OK |
| VI. YAGNI | Sin quitar de recetas por MCP, sin borrar, sin cascada, sin regla nueva de compras con id archivado | OK |
| VII. Comentarios / idioma | Identificadores y argumentos en inglés; descripciones, pistas y errores en español | OK |
| VIII. Puertas | Ninguna se toca: archivar no cambia existencias por sucursal ni recetas. ¿Se puede agregar después al mismo costo? Sí, todo lo de esta feature | OK |
| Tableta | La pestaña Existencias deja de listar insumos archivados; sin disposición nueva (ver «Pantallas y maquetas») | OK |

## Seguridad (principio V)

| Control | Diseño | Prueba |
| --- | --- | --- |
| Alcance | El de `ingredient_update` (`write:inventory`), revisado por `apply` en cada llamada | token sin `write:inventory`: la herramienta no existe para ese token (072), así que no se ofrece y llamarla responde «unknown tool»; el insumo sigue activo |
| Permiso | El de la ruta (`inventory.configure`) | rol con ver y recibir pero sin configurar, con token de alcance completo: no se ofrece; aplicar responde 403 y nada cambia |
| Despachador | Lista blanca de `ingredient_update`: se agregan `…/archive`, `…/reactivate` y `GET …/archive-check`; unir, unidad, costo y densidad siguen fuera. `articles_search` agrega solo `GET /stock/articles` | `TestDispatcherRefusesRoutesOutsideCatalog`: `ingredient_create` no despacha archivar ni reactivar; `ingredient_update` no despacha unir, unidad, costo ni densidad |
| Confirmación de existencia | El agente no puede confirmar sin la persona por accidente: `confirm_on_hand` es explícito, la vista previa dice que solo va si la persona lo decide, y la descripción del catálogo lo repite. La regla es la del servicio | aplicar sin confirmar con existencia → 409 y nada cambia |
| Idempotencia | La de `apply`; archivar y reactivar responden 204 y una repetición del servicio no cambia nada | diez veces la misma clave → un solo evento `ingredient.archived` |
| Bitácora | `mcp.tool_called` con antes y después (la vista de la ficha incluye `isActive`); el servicio deja `ingredient.archived`/`ingredient.reactivated` con el token como dispositivo | subtest que lee los dos eventos |
| Empresa | La del token: rutas bajo su sesión y RLS | token de otra empresa → «no existe», sin revisión ni cambio |
| Ensanche de tokens (spec D5) | Un token emitido con `write:inventory` gana desactivar y reactivar si el rol tiene configurar | `whoami` contra local con el token del respaldo (quickstart) |

### Ajustes de la revisión de código (2026-11-12)

| # | Hallazgo (revisor) | Decisión |
| --- | --- | --- |
| C1 | La vista previa de reactivar decía «vuelve tal como estaba» sin avisar que el nombre tomado o la unión lo rechazan (go-backend-reviewer, el más alto) | Nota que dice las dos negativas; las decide el servicio al aplicar. Subtest en `TestMCPIngredientReactivate`, visto en rojo |
| C2 | `articles_search` dejó de pasar por la barrera `forAgent` al dejar `simpleRead` (go-backend-reviewer) | Los renglones de las rutas pasan por `forAgent`; los avisos de la herramienta van fuera, porque la barrera borra la llave `notes` |
| C3 | La prueba de «solo compras» no comprobaba que el archivado no se filtrara (security-auditor, bajo) | Subtest ampliado: ni el nombre, ni `onHand`, ni un renglón `active: false`. Pasó al escribirlo: queda como guarda |
| C4 | El mapa `seen` de `articles_search` parecía código muerto (go-backend-reviewer) | Se queda, con comentario: si el insumo se archiva entre las dos lecturas saldría dos veces |
| C5 | Los renglones archivados no dicen por cuál otro nombre coincidieron (go-backend-reviewer, bajo) | **Residuo aceptado**: el renglón trae `altNames` completo |
| C6 | La pista de `NAME_TAKEN` al reactivar dice «elige otro nombre», y `active` va solo (go-backend-reviewer, bajo) | **Residuo aceptado**: el error ya trae el id del activo que tiene el nombre; renombrar es otra llamada |
| C7 | `TestEveryToolMatchesItsRouteGate` solo revisa la primera ruta de cada herramienta (security-auditor, bajo) | **Residuo aceptado, previo a la 077**: cada ruta despachada pasa por su `RequirePermission` del router; ampliar la prueba es trabajo aparte |

## Casos de borde (antes del código)

| Forma de fallar | Familia | Test |
| --- | --- | --- |
| La vista previa no dice la existencia, o la dice después del resumen | camino nuevo que se salta el control viejo (la hoja la muestra arriba) | `attention` primero con la cantidad y la sucursal |
| Existencia negativa tratada como cero | valor que significa algo | insumo con existencia negativa → aviso y rechazo sin confirmar |
| El agente manda `confirm_on_hand` sin decirlo la persona | camino nuevo | nota y descripción; la regla sigue en el servicio |
| Desactivar con receta activa desactiva el producto | hermano que no se movió | rechazo y producto/receta sin cambio |
| `active` con `name` aplica uno y no el otro | estado a medias | rechazo de argumentos antes de despachar |
| `confirm_on_hand` con `active: true` se ignora | parámetro presente ignorado (V) | rechazo de argumentos |
| Reactivar con el nombre tomado por otro activo | estado que no sobrevive | 409 con el id del otro |
| Un renglón aprendido sigue autollenando el archivado | hermano que no se movió (la búsqueda filtraba, las sugerencias no) | prueba de `SuggestForLine` en rojo hoy |
| La lista de existencias marca «bajo» un archivado | hermano que no se movió | prueba de `/stock/levels` en rojo hoy |
| `include_inactive` sin ver el almacén responde «no hay inactivos» | callar una lectura que falló | la respuesta trae la nota de permiso |
| `include_inactive` repite un renglón | duplicado | el agregado no repite id |

## Pantallas y maquetas

**Almacén › Existencias**: deja de listar insumos archivados (la consulta los omite). La tabla, sus
columnas, sus controles y su altura por renglón no cambian; solo hay menos renglones. Un solo texto
cambia: la tabla vacía decía «Sin movimientos aún», que deja de ser cierto cuando el único insumo con
existencia se archiva (sus movimientos siguen); pasa a «Sin existencias que mostrar» (revisión de
arquitectura, A1). El tipo `StockLevel` del front gana `is_active` opcional, que la pantalla no pinta.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Almacén › Existencias | No aplica: no cambia disposición ni componentes; deja de listar archivados y cambia el texto de la tabla vacía | Sin cambio de disposición | `/maquetas` no genera opciones sin cambio de disposición; el revisor de tableta lo confirmó (A1) |

## Ajustes de la revisión de arquitectura (2026-11-12)

| # | Hallazgo (revisor) | Decisión |
| --- | --- | --- |
| A1 | Archivar el último insumo con existencia deja la tabla en «Sin movimientos aún», que es falso (tablet-ui-reviewer, medio) | Texto a «Sin existencias que mostrar», con su test en `StockPage.test.tsx` visto en rojo |
| A2 | Una existencia archivada con `confirm_on_hand` deja de verse en Existencias (tablet-ui-reviewer, bajo; db-architect, menor) | Sigue en Almacén › Insumos › Archivados (con su existencia) y en la revisión previa; la nota de la vista previa MCP lo dice; `docs/mecanica/almacen.md` lo documenta. La hoja de la pantalla ya dice «Se queda así, sin ajustarse»: sin cambio de pantalla |
| A3 | `coalesce(i.is_active, p.is_active)` depende de la inferencia de nulabilidad de sqlc (db-architect, menor) | `coalesce(i.is_active, p.is_active, true)`; se confirma `bool` en el código generado |
| A4 | Un producto archivado con control de existencias sigue en la lista, ahora marcado (db-architect, menor) | A propósito (spec D7). Documentado en `almacen.md`; la prueba de existencias lo cubre |
| A5 | Con el aprendido filtrado, la sugerencia de mayor puntaje puede ser otro insumo parecido (db-architect, menor) | Pasos 2 y 3 son sugerencias, no autollenan. Prueba: aprendido hacia un archivado con un activo parecido → sale el activo como sugerencia, no como «aprendido» |

## Project Structure

### Documentation (this feature)

```text
specs/077-mcp-activar-insumos/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/mcp-tools.md
└── tasks.md
```

### Source Code (repository root)

```text
server/
├── queries/supplier_items.sql, queries/stock.sql   # filtros de archivado; make sqlc
├── internal/store/db/*.go                          # generado
├── internal/domain/apitoken.go (+_test)            # catálogo y pistas
├── internal/httpapi/mcp_tools_076.go (+_test)      # active / confirm_on_hand; articles_search con include_inactive
├── internal/httpapi/mcp_tools_073.go               # articles_search deja su simpleRead
├── internal/httpapi/mcp_dispatch_test.go           # lista blanca ajustada
├── internal/integration/archived_ingredient_test.go  # nuevo: qué hace un archivado
└── internal/integration/mcp_077_test.go            # nuevo
web/src/api/backoffice.ts                           # StockLevel.is_active opcional
web/src/features/backoffice/StockPage.tsx (+test)   # texto de la tabla vacía (A1)
docs/mecanica/personal-y-permisos.md                # la puerta para agentes: qué agrega la 077
docs/mecanica/almacen.md                            # archivado: sugerencias aprendidas y existencias
```

**Structure Decision**: monorepo existente; la acción vive junto a `ingredient_update` en su archivo.

## Decisiones (dudas resueltas con la mejor recomendación)

Las de producto viven en el spec (D1–D8). Las técnicas, en [research.md](research.md) (R1–R6).

## Complexity Tracking

Sin violaciones.
