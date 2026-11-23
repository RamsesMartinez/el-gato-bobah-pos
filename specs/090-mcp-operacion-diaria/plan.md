# Implementation Plan: MCP para la operación diaria (paquete 1 del #289)

**Branch**: `090-mcp-operacion-diaria` | **Date**: 2026-11-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/090-mcp-operacion-diaria/spec.md`

## Summary

Tres defectos corregidos con su prueba de regresión, y 22 herramientas nuevas (más dos ampliadas)
sobre la puerta para agentes de la 072. Como en las specs 073–084, **cada herramienta despacha en
proceso a la misma ruta que usa la pantalla**: hereda su `RequirePermission`, su servicio de `app/`,
su tope por usuario, la invalidación del menú y sus eventos de seguridad. La capa MCP solo arma el
cuerpo, lee el estado para la vista previa y pone la clave de idempotencia.

Lo único que se agrega a la API, y por qué:

| Pieza nueva | Por qué no alcanza lo que hay |
| --- | --- |
| `POST /pos/drafts/quote` | La vista previa de una cuenta nueva tiene que dar el total **que calcula el servidor** sin crear nada. Hoy ese cálculo solo corre sobre una cuenta guardada (`buildDraftView`). Se extrae y se reusa; no hay segunda regla de precio |
| `GET /admin/modifier-options/{id}` | La lista de extras oculta las opciones de grupos inactivos (`where mg.is_active`): es la causa del «No existe el extra». Una lectura por id, con `groupActive`, es lo que la herramienta y el `PATCH` parcial necesitan |
| `reason` en `POST /stock/articles/{id}/cost` y migración **0116** | La corrección de costo no guarda motivo. Columna nullable: la pantalla sigue mandando solo `{cost}` |
| `inactiveGroupCount` en `GET /admin/products/` | La lista de productos solo cuenta grupos activos; el producto con un grupo apagado no se distingue |

## Herramientas

| Herramienta | Ruta que despacha | Vista previa | Permiso de la ruta | Alcance |
| --- | --- | --- | --- | --- |
| `order_draft_create` | `POST /pos/drafts` | `POST /pos/drafts/quote` (estricta: un rechazo es error) | — (la de crear pedido) | `write:orders` |
| `order_draft_send` | `POST /pos/drafts/{id}/send` | `GET /pos/drafts/{id}` | — | `write:orders` |
| `order_draft_discard` | `POST /pos/drafts/{id}/discard` | `GET /pos/drafts/{id}` (y su versión) | — | `write:orders` |
| `order_deliver` | `POST /orders/{id}/deliver` | `GET /orders/{id}` | — | `write:orders` |
| `order_discount_set` | `PUT /orders/{id}/discount` | `GET /orders/{id}` | — | `write:orders` |
| `order_pay` | `POST /orders/{id}/pay` | `POST /orders/{id}/quote`, `GET /orders/{id}`, `GET /payment-methods` | — | `write:orders` |
| `order_payment_void` | `POST /orders/{id}/payments/{paymentId}/void` | `GET /orders/{id}` | `payments.void` | `write:orders` |
| `order_payment_terminal_set` | `POST /orders/{id}/payments/{paymentId}/terminal` | `GET /orders/{id}`, `GET /card-terminals` | `payments.change_terminal` | `write:orders` |
| `order_refund` | `POST /orders/{id}/refund` | `GET /orders/{id}/refund-info`, `GET /orders/{id}` | `payments.void` | `write:orders` |
| `card_terminals_list` | `GET /card-terminals` | — | `cash.operate` | `read` |
| `ingredient_cost_set` | `POST /stock/articles/{id}/cost` | `GET /stock/articles/{id}`, `GET /stock/articles/{id}/cost-history` | `inventory.configure` | `write:inventory` |
| `recipe_get` | `GET /admin/products/{id}/composition` o `GET /admin/modifier-options/{id}/composition` | — | `menu.configure` | `read` |
| `recipe_set` | `PUT …/composition` (producto o extra) | las dos `GET …/composition`, `GET /stock/articles/{id}` | `menu.configure` | `write:catalog` |
| `categories_list` | `GET /admin/categories` | — | `menu.configure` | `read` |
| `modifier_groups_list` | `GET /admin/groups` | — | `menu.configure` | `read` |
| `modifier_group_create` | `POST /admin/groups` | `GET /admin/groups` (parecidos) | `menu.configure` | `write:catalog` |
| `modifier_group_update` | `PATCH /admin/groups/{id}` | `GET /admin/groups`, `GET /admin/groups/{id}/products` | `menu.configure` | `write:catalog` |
| `modifier_option_create` | `POST /admin/groups/{id}/options` | `GET /admin/groups/{id}/options`, `GET /admin/groups` | `menu.configure` | `write:catalog` |
| `modifier_option_update` (ampliada) | `PATCH /admin/modifier-options/{id}` | `GET /admin/modifier-options/{id}` | `menu.configure` | `write:catalog` |
| `product_groups_get` | `GET /admin/products/{id}/groups` | — | `menu.configure` | `read` |
| `product_group_link` | `POST /admin/products/{id}/groups` | `GET /admin/products/{id}/groups`, `GET /admin/groups` | `menu.configure` | `write:catalog` |
| `product_group_unlink` | `DELETE /admin/products/{id}/groups/{groupId}` | `GET /admin/products/{id}/groups` | `menu.configure` | `write:catalog` |
| `product_create` | `POST /admin/products/` | `GET /admin/products/?search=` (parecidos), `GET /admin/categories` | `menu.configure` | `write:catalog` |
| `product_update` (ampliada) | `PATCH /admin/products/{id}` | `GET /admin/products/`, `GET /admin/categories` | `menu.configure` | `write:catalog` |

Contrato de argumentos y respuestas: [contracts/mcp-tools.md](contracts/mcp-tools.md).

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript + React 19 (web)

**Primary Dependencies**: las de la 072 (SDK de MCP); ninguna nueva.

**Storage**: PostgreSQL — migración **0116** aditiva (`ingredient_cost_corrections.reason`). Libre al
2026-11-22: la última en `origin/develop` es la 0115 y ninguna rama remota ni el otro worktree vivo
(`egb-089-…`) trae una 0116 (revisado con `git ls-tree` de cada rama remota antes de escribirla; se
vuelve a revisar antes del PR).

**Testing**: integración contra Postgres real con la API servida como `gatobobah_app`
(`internal/integration/mcp_090_test.go`, una prueba por herramienta con su rechazo de pantalla y su
permiso), regresiones de los tres defectos vistas en rojo, unitarios en `domain` (origen del precio,
UUID derivado de la clave), test de migración 0116, vitest de la insignia y del aviso de archivar, y
prueba de punta a punta con cliente MCP real contra `make start` y la base restaurada.

**Target Platform**: la API en la VM; la pantalla en el equipo de mostrador a 1280×854.

**Project Type**: web-service + web-app (monorepo existente).

**Performance Goals**: ninguna vista previa hace más de 4 lecturas internas; `order_draft_create`
acota a `domain.MaxDraftLines` renglones antes de despachar.

**Constraints**: ninguna regla de negocio en la capa MCP (precios, validación de mínimos, nombres
repetidos: todo lo decide el servicio); la API vieja y la nueva conviven durante la subida (la 0116 es
una columna nullable, y la versión vieja no la lee).

**Scale/Scope**: 22 herramientas nuevas y 2 ampliadas (de 55 a 77); 4 piezas de API; 1 migración; 2
cambios de texto y una insignia en la pantalla.

## Constitution Check

| Principio | Cómo se cumple | Estado |
| --- | --- | --- |
| I. Layering | `domain`: catálogo de herramientas, `PlatformPriceSource`, `DerivedUUID`, pistas de error. `app`: `DraftsService.Quote` (extraído de `buildDraftView`), `UpdateOptionFields` con parche, `GetOption`, motivo en `CorrectArticleCost`. `store`: consultas sqlc nuevas. `httpapi`: handlers finos de las 2 rutas nuevas y las herramientas en `mcp_tools_090*.go` que solo arman cuerpos | OK |
| II. Errores | Los rechazos salen de los servicios con sus sentinels; `httpapi.Error` mapea; la pista del agente se arma en `domain.ToolErrorHintFor` con los códigos nuevos (receta con existencia, receta que cambió) | OK |
| III. Dinero | Precios, descuentos, cobros y costos viajan como texto decimal a la ruta, que valida y redondea. El total de la vista previa lo calcula el servidor con la misma función que la cuenta guardada: no hay segunda suma | OK |
| IV. Test-first | Cada defecto: test en rojo antes del arreglo. Cada herramienta: subtest de integración en rojo (herramienta desconocida) antes de su código | OK |
| IV. Aislamiento | Las rutas existentes ya tienen sus tres casos. Lo nuevo que lee una tabla de empresa (`GetOption`, `inactive_group_count`, `reason`, `Quote`) lleva subtest `inTheThreeCases` bajo `appRoleStore` | OK |
| IV. Migración | `migration_0116_test.go` contra base restaurada con dos empresas: sube, la pantalla guarda sin motivo, el MCP con motivo, y baja | OK |
| IV. Tolera la versión anterior | Columna nullable sin default; la API vieja inserta sin ella. Ninguna ruta cambia de forma para la pantalla: el `PATCH` de extras sigue aceptando el cuerpo completo | OK |
| V. Seguridad | Ver «Seguridad». `security-auditor` y `/security-review` en la revisión | OK, a revisar |
| VI. YAGNI | Sin herramienta de «cambiar método» (anular + cobrar, como la pantalla); sin duplicar producto; archivar = `active` | OK |
| VII. Comentarios / idioma | Identificadores, herramientas y columna en inglés; descripciones y mensajes en español | OK |
| VIII. Puertas | «Saber de quién es un pedido»: el pedido del MCP guarda como `opened_by` al dueño del token y el dispositivo dice el token, igual que el resto de escrituras MCP. «Más de una caja»: el cobro usa la ruta, que resuelve la caja como hoy. Ninguna puerta se cierra | OK |
| Tableta | Insignia en una celda existente y texto de un aviso y de dos alcances; sin disposición nueva | OK |

## Seguridad (principio V)

| Control | Diseño | Prueba |
| --- | --- | --- |
| Alcance | Cada herramienta lo declara en el catálogo; `apply` lo revisa en cada llamada | token sin `write:orders`: las de pedido no se ofrecen y, llamadas, se niegan |
| Permiso | El de la ruta; `TestEveryToolMatchesItsRouteGate` lo compara contra el router | rol sin `payments.void`: anular y reembolsar no se ofrecen; la ruta responde 403 |
| Despachador | Lista blanca por herramienta. **Parámetro UUID nuevo** para la cuenta: solo se acepta un valor que `uuid.Parse` lee, y se escribe con su forma canónica; nunca texto del agente en el camino | `TestDispatcherRejectsNonUUIDDraftID` (`../`, `%2F`, vacío) |
| Reenvío de un cobro | `clientUuid` del cobro y `id` de la cuenta se derivan de (empresa, token, clave): un reenvío que esquive el registro de idempotencia cae en la red de la ruta | misma clave dos veces cobra una; unitario de la derivación |
| Topes por usuario | Los de la ruta (descuento, cobro dividido, anular, terminal) cuentan al dueño del token | se heredan; sin prueba nueva |
| Eventos de seguridad | Los de la ruta (`order_payment_voided`, `order_refund`, `order_discount_set`…) | subtest que lee el evento de anular |
| Bitácora | `mcp.tool_called` por llamada, con antes y después | un subtest por familia |
| Ensanche de tokens | Un token ya emitido con `write:orders` (hoy solo cancelar) gana capturar, cobrar, anular y reembolsar si el rol tiene el permiso; uno con `write:catalog` gana recetas, grupos y productos. Se dice en el texto del alcance de Integraciones y en el comentario del #289 | revisión del texto |

## Casos de borde (antes del código)

| Forma de fallar | Familia | Test |
| --- | --- | --- |
| `PATCH` de extra con solo `name` deja precio 0 y tope 1 | valor vacío que significa algo | `TestOptionPatchKeepsWhatItDoesNotSend` (rojo hoy) |
| La herramienta no ve una opción de un grupo inactivo | hermano que no se movió (la lista filtra el grupo) | `TestOptionUpdateFindsOptionOfInactiveGroup` (rojo hoy) |
| El producto con grupo apagado se ve igual que los demás | estado que no se ve | `inactive_group_count` en integración + vitest de la insignia |
| La vista previa de la cuenta suma con otra regla que la de enviar | dos implementaciones de la misma cifra | prueba: total de la vista previa = total del pedido enviado, con plataforma y extras |
| Precio sin propio de plataforma pasa sin aviso | valor que significa algo | origen `markup` y nota |
| Reintento del envío o del cobro duplica | estado que no sobrevive | misma clave dos veces; envío repetido devuelve el mismo pedido |
| `order_discount_set` sin monto ni porcentaje borra el descuento sin querer | valor vacío | `clear: true` explícito para quitarlo; ambos ausentes es error de argumentos |
| Anular un pago de turno cerrado | camino nuevo sin el control viejo | lo rechaza el servicio; subtest |
| Reembolso con tarjeta sin folio | camino nuevo | rechazo igual al de la pantalla |
| Corrección de costo sin motivo por MCP | valor vacío | rechazo de argumentos antes de despachar |
| La pantalla deja de poder corregir sin motivo | hermano que no se movió | test de la ruta sin `reason` |
| `recipe_set` copia a homónimos sin pedirlo | valor que significa algo | por omisión `alsoOptionIds` vacío |
| Receta con existencia propia sin la cifra | estado que no se ve | vista previa con la pregunta; aplicar rechaza con mensaje que dice qué falta |
| Receta guardada por otra persona entre vista previa y aplicar | estado que no sobrevive | `based_on` obligatorio al aplicar; 409 con pista |
| `product_update` con solo nombre mueve la categoría, borra el horario o el costo | valor vacío | `cambiar solo el nombre conserva lo demás` |
| `modifier_group_update` con solo `active` cambia nombre o mínimos | valor vacío | lee el grupo y solo cambia lo mandado |
| Ligar un grupo sin mínimo y máximo los pone en 0 | valor vacío | sin ellos hereda (`override: false`) |
| Token de otra empresa usa ids de ésta | aislamiento | subtest por familia |

## Pantallas y maquetas

No hay pantalla nueva ni cambio de disposición: `/maquetas` no genera opciones (su propia regla).

| Pantalla | Cambio | Maquetas |
| --- | --- | --- |
| Menú › Productos (lista) | En la celda «Grupos» (el número sigue contando solo los activos, como hoy), una insignia corta «1 apagado» / «2 apagados» con `whiteSpace="nowrap"` cuando hay grupos inactivos ligados; el detalle va en `title` («2 grupos inactivos: no salen en la venta»), como la insignia de ajustes. Se mide a 1280×854: ancho de columna sin scroll horizontal y altura de renglón igual a la de hoy | No aplica: insignia en celda existente |
| Menú › Modificadores (archivar grupo) | El aviso que ya sale («Grupo archivado», con Deshacer) lleva en su descripción «Ya no se ofrece en 1 producto» / «en N productos» cuando N > 0, contando solo productos activos; el aviso de «Grupo reactivado» no repite el conteo | No aplica: texto de un aviso existente |
| Integraciones › Crear token | Texto de tres alcances: Almacén («Insumos, sus costos y empaques, mermas, ajustes y conteos»), Menú y precios («Productos, grupos, extras y recetas; precios por plataforma»), Pedidos («Capturar, entregar y cobrar; anular pagos, cancelar y reembolsar», 66 car.) | No aplica: texto; la más larga sigue siendo Compras (69 car.); se mide la hoja a 1280×854 |

## Project Structure

### Documentation (this feature)

```text
specs/090-mcp-operacion-diaria/
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
├── migrations/0116_cost_correction_reason.sql
├── queries/admin.sql                       # AdminGetOption (+ AdminLockOption `for update of mo`), inactive_group_count
├── queries/ingredients_admin.sql           # InsertCostCorrection con reason; historial con reason (`null::text` en las ramas que no son corrección)
├── internal/domain/apitoken.go (+_test)    # catálogo y pistas
├── internal/domain/platform_price.go (+_test) # PlatformPriceSource
├── internal/domain/mcp_writes.go (+_test)  # DerivedUUID
├── internal/app/drafts.go                  # Quote, priceDraft extraído
├── internal/app/modifiers_admin.go         # UpdateOptionFields con parche, GetOption
├── internal/app/ingredients_admin.go       # motivo
├── internal/httpapi/router.go, handlers_drafts.go, handlers_admin.go, handlers_ingredients_admin.go
├── internal/httpapi/mcp.go                 # despacho con UUID
├── internal/httpapi/mcp_tools_090_orders.go, mcp_tools_090_catalog.go
├── internal/httpapi/mcp_tools_write.go     # product_update y modifier_option_update ampliadas
└── internal/integration/mcp_090_test.go, migration_0116_test.go, option_patch_test.go
scripts/respaldo-anonimo.sh                 # reason se borra en el respaldo anónimo
web/src/features/admin/ProductsAdminPage.tsx, ModifierOptionsPage.tsx, IntegrationsPage.tsx (+tests)
web/src/api/admin.ts                        # inactiveGroupCount
docs/mecanica/personal-y-permisos.md        # la puerta para agentes: qué agrega la 090
```

**Structure Decision**: monorepo existente; las herramientas nuevas en dos archivos propios
(pedidos y catálogo), como las de la 073, 074 y 076.

## Maquetas

Sin pantalla nueva ni disposición cambiada (ver «Pantallas y maquetas»).

## Complexity Tracking

Sin violaciones.

## Revisión de arquitectura (2026-11-22)

Aprobado con ajustes, ya incorporados arriba y en research/data-model:

| # | Hallazgo (revisor) | Decisión |
| --- | --- | --- |
| A1 | `for update` sobre opción + grupo bloquea también el grupo (db-architect) | `for update of mo` |
| A2 | El check no garantiza «sin espacios a los lados» (db-architect) | `reason = btrim(reason) and char_length(reason) between 1 and 200`; el servicio recorta antes |
| A3 | El respaldo anónimo debe dejar `reason` válido para el check (db-architect) | `set reason = 'motivo anonimizado' where reason is not null`, contado en `FUGAS` |
| A4 | `union all` del historial es posicional (db-architect) | `null::text` en las ramas que no son corrección |
| A5 | La insignia «N inactivo(s)» ensancha la celda y es notación de programador (tablet-ui-reviewer) | «2 apagados» sin salto (en táctil el `title` no se ve: revisión de código), detalle en `title`, plural real; se mide a 1280×854 |
| A6 | «Deja de salir en N productos» es ambiguo y sin singular (tablet-ui-reviewer) | «Ya no se ofrece en 1 producto / N productos», en la descripción, solo activos |
| A7 | El hint de Pedidos (72 car.) superaba a Compras (tablet-ui-reviewer) | «Capturar, entregar y cobrar; anular pagos, cancelar y reembolsar» (66) |

## Revisión de código (2026-11-22)

`/revision-de-codigo` (go-backend-reviewer, security-auditor, tablet-ui-reviewer) y el recorrido de punta a
punta contra la base restaurada. Cada arreglo con su prueba:

| # | Hallazgo (origen) | Decisión |
| --- | --- | --- |
| C1 | Mínimos, máximos, tope por renglón y orden que no caben en smallint/integer dan la vuelta (65536 → 0) o truenan un check como 500; `price_delta` fuera de numeric(10,2) da 500. Las rutas ya existían; la 090 se las abre al agente (security-auditor, go-backend-reviewer) | `domain.ValidSelectRange`, `ValidMaxPerLine`, `ValidPosition`, `ValidPriceDelta` en grupos, ligas y extras (crear y editar); `TestModifierLimits`, `TestOptionCreateAndPatchShareTheirLimits` |
| C2 | `recipe_set` con homónimos tomaba los stamps de al aplicar: una receta guardada en medio se pisaba (go-backend-reviewer) | La vista previa da `also_based_on` y aplicar lo exige y lo reenvía; subtest «alguien le guarda receta al homónimo» |
| C3 | Ensanche de tokens ya emitidos (security-auditor) | **Riesgo aceptado** (plan, «Seguridad»): texto de los alcances en Integraciones, `docs/mecanica/personal-y-permisos.md` y el comentario del #289 |
| C4 | Ids derivados sin prueba propia; barrera de permiso probada solo por omisión (security-auditor) | `TestMCPDerivedIDsSurviveALostIdempotencyRecord`, `TestRefundRouteRefusesWithoutPermission` |
| C5 | Motivos sin recortar (go-backend-reviewer) | La herramienta los recorta antes de despachar |
| C6 | `product_update` lee y reescribe el producto completo (go-backend-reviewer) | **Residuo aceptado**, `ponytail:` en `mcp_tools_write.go`: es el mismo techo de la pantalla; el camino es un PATCH parcial |
| C7 | La insignia «N inact.» no se entiende en táctil (tablet-ui-reviewer) | «1 apagado / 2 apagados» |
| C8 | Recorrido: nombre repetido decía solo «conflicto»; «1 productos activos» | «ya hay un extra/grupo con ese nombre»; plural real |
