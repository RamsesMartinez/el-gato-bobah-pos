# Implementation Plan: Empaques de compra y edición de compras guardadas desde el servidor MCP

**Branch**: `074-mcp-presentaciones` | **Date**: 2026-11-11 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/074-mcp-presentaciones/spec.md`

## Summary

Tres escrituras nuevas sobre la puerta para agentes de la 072/073, sin ruta nueva en la API, sin
migración y sin alcance nuevo:

| Herramienta | Ruta que despacha | Permiso de la ruta | Alcance |
| --- | --- | --- | --- |
| `presentation_create` | `POST /api/v1/stock/ingredients/{id}/presentations` | `inventory.configure` | `write:inventory` |
| `presentation_update` | `PUT /api/v1/stock/presentations/{id}` | `inventory.configure` | `write:inventory` |
| `expense_edit` | `POST /api/v1/expenses/{id}/edit` (vista previa: `?preview=1`) | `expenses.edit` | `write:expenses` |

Más un cambio a la vista previa de `purchase_create` (sugerir el empaque que falta, D4) y el texto
de dos alcances en Integraciones (FR-016).

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: las de la 072 (SDK de MCP); ninguna nueva.

**Storage**: PostgreSQL — ningún cambio. La siguiente migración libre (0113) queda libre.

**Testing**: integración contra Postgres real con la API completa servida como `gatobobah_app`
(`internal/integration/mcp_074_test.go`), vista en rojo antes del código; unitarios de las pistas
de error en `domain`; `TestEveryToolMatchesItsRouteGate` (golden de rutas) ampliado; vitest del
texto de Integraciones; prueba con cliente MCP real contra `make start` y el respaldo restaurado.

**Target Platform**: la API en la VM; la pantalla en tableta a 1024×600.

**Project Type**: web-service + web-app (monorepo existente).

**Performance Goals**: cada llamada cuesta la petición de la pantalla más las lecturas de su vista
previa (a lo más tres despachos internos).

**Constraints**: ninguna herramienta puede más que la pantalla; ninguna regla de negocio en la capa
MCP; la API anterior y la nueva conviven (no hay cambio de esquema).

**Scale/Scope**: 3 herramientas nuevas (52 en total).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple | Estado |
| --- | --- | --- |
| I. Layering | `domain/apitoken.go`: catálogo (tres renglones) y `toolFixByCode` (dos códigos). `httpapi/mcp_tools_074.go`: las herramientas, que solo arman el cuerpo de la pantalla y despachan. `app` y `store` no cambian | OK |
| II. Errores | `EXPENSE_EDIT_STALE` y `DECISION_REQUIRED` entran a `toolFixByCode` con su corrección en palabras; el resto lo mapea `httpapi.Error` como siempre | OK |
| III. Dinero | La capa MCP no suma ni redondea: importes y cantidades viajan como texto a la ruta, que valida y redondea. El importe y estado después de la edición los calcula `domain.ExpenseAmountAfterEdit` dentro de la ruta | OK |
| IV. Test-first | `mcp_074_test.go` se escribe y se ve en rojo (herramienta desconocida) antes de `mcp_tools_074.go`; cada borde de la tabla de abajo es un subtest | OK |
| IV. Aislamiento | Un token de otra empresa no edita el gasto ni el empaque de ésta (subtest por herramienta). Las rutas ya tienen su prueba de los tres casos (053, 038); no se agrega consulta | OK |
| IV. Migración | No hay | N/A |
| V. Seguridad | Ver «Seguridad». `security-auditor` en la revisión de código | OK, a revisar |
| VI. YAGNI | Sin «cambiar renglón» propio (D6), sin crear insumos, sin entregas parciales ni excluir renglones | OK |
| VII. Comentarios / idioma | Identificadores y nombres de herramienta en inglés; descripciones y errores en español | OK |
| VIII. Puertas | Ninguna se toca: los empaques ya son por insumo y la edición no asume caja ni almacén nuevos | OK |
| Tableta | Solo cambia el texto de dos alcances; sin disposición nueva (ver «Pantallas y maquetas») | OK |

## Seguridad (principio V)

| Control | Diseño | Prueba |
| --- | --- | --- |
| Alcance | Cada herramienta declara su alcance en el catálogo; `apply` lo revisa en cada llamada | `token sin alcance` por herramienta: no se ofrece y, llamada directa, se niega nombrando el alcance |
| Permiso | El de la ruta; `TestEveryToolMatchesItsRouteGate` comprueba contra el router que el permiso del catálogo es el de su ruta | rol sin `inventory.configure` / sin `expenses.edit` (un cajero con el token de alcance completo): no se ofrece; la ruta responde 403 |
| Permiso adicional de la edición | Mover el almacén exige `inventory.receive`: lo decide la ruta con el mismo resolvedor del router; la capa MCP no lo copia | rol con `expenses.edit` y sin `inventory.receive` quitando un renglón recibido → rechazo nombrando el permiso |
| Despachador | Lista blanca: `expense_edit` solo `POST …/{id}/edit` y `GET /expenses/{id}`; el `?preview=1` viaja como consulta, no como parte del camino. `presentation_*` solo sus rutas y las lecturas de su vista previa | `TestDispatcherRefusesRoutesOutsideCatalog` ampliado (`/expenses/{id}/deliveries`, `/expenses/{id}/exclude` siguen fuera) |
| Huella e idempotencia | Aplicar sin `fingerprint` se rechaza antes de despachar; con una vencida, la ruta responde 409 y `apply` libera la clave (rama 4xx existente) | `huella vencida` y `reintento tras huella vencida con la misma clave` |
| Bitácora | Cada llamada deja `mcp.tool_called`; la edición deja además `expense.edited` con el token en el dispositivo (lo pone la ruta) | subtest que lee el evento |
| Ensanche de tokens (D3) | Sin cambio de esquema: un token emitido con `write:inventory`/`write:expenses` gana las herramientas si el rol tiene el permiso. Se dice en Integraciones y en el issue | `whoami` contra local con el token de «Gestión Claude» (quickstart) |

## Casos de borde (antes del código)

| Forma de fallar | Familia | Test |
| --- | --- | --- |
| `presentation_update` sin `brand` borra la marca, o sin `qty` cambia el contenido | valor vacío que significa algo | `editar solo la marca conserva nombre y contenido`, `desactivar conserva la marca` |
| Mandar `brand: ""` no la quita (o la quita sin pedirlo) | valor vacío | `marca vacía la quita` |
| Empaque aproximado editado como si no lo fuera (contenido × unidad) | hermano que no se movió | `editar el nombre de un aproximado conserva su unidad de compra` |
| `expense_edit` aplica con una huella de otra vista previa | estado que no sobrevive | `huella vencida` (cambiar el gasto entre vista previa y aplicar) |
| La clave queda «en curso» tras un 409 y el reintento correcto no puede usarla | estado que no sobrevive | `reintento tras huella vencida con la misma clave aplica` |
| Decisión faltante se toma como «ajustar» en silencio | valor vacío | `decisión faltante` con un conteo cerrado después de la llegada |
| Quitar y agregar en dos llamadas deja la compra a medias (existencia baja entre las dos) | camino nuevo | la herramienta lo hace en una; el caso de aceptación compara contra la compra bien capturada |
| El renglón agregado sin `arrived_on` queda por recibir y la existencia baja | valor vacío que significa algo | `sin arrived_on el renglón queda por recibir` + nota en la vista previa |
| La vista previa de compra sugiere crear un empaque que ya existe con esa marca | hermano | la sugerencia lista los empaques con su marca; crear el duplicado → 409 |
| Marca de 41 letras / marca repetida | — | `400 marca larga`, `409 marca repetida` |

## Pantallas y maquetas

**Integraciones › Crear token**: solo cambia la línea de dos alcances; la disposición, los
componentes y la altura de cada casilla no cambian.

| Alcance | Hoy | Después |
| --- | --- | --- |
| Gastos | Capturar, pagar y anular gastos; dar de alta proveedores | Capturar, pagar, corregir y anular gastos; dar de alta proveedores |
| Almacén | Mermas, ajustes de existencia y conteos | Mermas, ajustes de existencia, conteos y empaques de compra |

Las dos líneas nuevas miden menos que la de Compras (la más larga hoy), así que ninguna casilla gana
un renglón a 1024×600. `/maquetas` no genera opciones para un cambio de texto (su propia regla: «no
por cada texto que cambia»), y no hay maqueta que elegir.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Integraciones › Crear token | No aplica: cambia solo el texto de dos alcances | Sin cambio de disposición | La línea más larga sigue siendo la de Compras; el revisor de tableta revisa el texto |

## Project Structure

### Documentation (this feature)

```text
specs/074-mcp-presentaciones/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/mcp-tools.md   # las tres herramientas y el cambio a purchase_create
└── tasks.md
```

### Source Code (repository root)

```text
server/
├── internal/domain/apitoken.go (+_test)          # catálogo: 3 herramientas; toolFixByCode: 2 códigos
├── internal/httpapi/mcp_tools_074.go             # nuevo: presentation_create/update, expense_edit
├── internal/httpapi/mcp_tools_073.go             # vista previa de purchase_create: empaques con marca y sugerencia
├── internal/httpapi/testdata/route_gates.golden  # si el golden lista herramientas
└── internal/integration/mcp_074_test.go          # nuevo
web/src/features/admin/IntegrationsPage.tsx (+test)
docs/mecanica/personal-y-permisos.md              # la puerta para agentes: qué agrega la 074
```

**Structure Decision**: monorepo existente; las herramientas nuevas en su propio archivo junto a las
de la 073.

## Decisiones (dudas resueltas con la mejor recomendación)

Las de producto viven en el spec (D1–D7). Las técnicas, en [research.md](research.md) (R1–R6).

## Complexity Tracking

Sin violaciones.
