# Implementation Plan: Servidor MCP del POS

**Branch**: `072-servidor-mcp` | **Date**: 2026-11-10 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/072-servidor-mcp/spec.md`

## Summary

Un endpoint `/mcp` dentro de la misma API, con el transporte HTTP streamable **sin sesión** del SDK
oficial de MCP para Go. Cada llamada se autentica con un token de acceso personal
(`egbpat_<empresa>_<id>_<secreto>`) que se busca bajo RLS fijando la empresa que el propio token
declara y se compara contra una huella sha256. Cada herramienta **se despacha en proceso al mismo
router de la API** con la identidad del token: el handler, el gate de permiso, el límite por usuario,
la invalidación del menú y los eventos de seguridad son exactamente los de la pantalla. Encima, la
capa MCP pone lo que la pantalla no tiene: alcances del token, vista previa por omisión, clave de
idempotencia, límite por token y un evento de bitácora por llamada. Una pantalla «Integraciones»
crea, lista y revoca tokens.

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; **nueva**: `github.com/modelcontextprotocol/go-sdk` v1.8.0 (ver research R1). Web: Chakra UI v3, TanStack Query.

**Storage**: PostgreSQL — dos tablas nuevas con RLS (`api_tokens`, `mcp_idempotency_keys`), un permiso nuevo. Redis solo para el contador del límite por token (mismo `rateLimiter`).

**Testing**: `go test` (unitarios en `domain`, httpapi con `httptest`), integración contra Postgres real bajo `appRoleStore` con `inTheThreeCases`, vitest en web, y una prueba manual con un cliente MCP real contra `make start`.

**Target Platform**: la API en la VM (detrás de Caddy, que ya enruta todo el dominio de la API); la pantalla en tableta a 1024×600.

**Project Type**: web-service + web-app (monorepo existente).

**Performance Goals**: una llamada de lectura cuesta lo mismo que la petición equivalente de la pantalla más dos lecturas por llave primaria (token y permisos) y una escritura de bitácora.

**Constraints**: ninguna herramienta puede más que la pantalla; la migración convive con la versión anterior; el valor del token no se guarda ni se registra.

**Scale/Scope**: 33 herramientas (22 de lectura, 11 de escritura), un dueño de tokens por ahora; pocos tokens por empresa.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple | Estado |
| --- | --- | --- |
| I. Layering | `domain/apitoken.go` (formato y análisis del token, alcances, vigencias, decisión de idempotencia, catálogo de herramientas con su alcance y permiso: puro, sin I/O). `app/api_tokens.go` (crear, listar, revocar, autenticar, idempotencia, evento de bitácora por llamada; transacciones). `queries/api_tokens.sql` + sqlc. `httpapi/mcp*.go` (transporte: middleware del token, servidor MCP, despacho) y `httpapi/handlers_integrations.go` (CRUD de tokens). Ninguna herramienta toca la base: llaman a handlers que llaman a servicios | OK |
| II. Errores | Sentinels de `domain` (`ErrUnauthorized`, `ErrForbidden`, `ErrValidation`, `ErrConflict`); el despacho traduce el estado HTTP del handler a un error de herramienta con causa y corrección (`domain.ToolErrorHint`, probado en tabla). `ctx` del request se propaga al despacho interno | OK |
| III. Dinero | Las herramientas reciben dinero como texto decimal y lo pasan tal cual al handler, que ya valida y redondea (`ValidMoney`, `Round2`). La capa MCP no hace aritmética de dinero; la vista previa de una salida de caja muestra el esperado que calcula el servidor | OK |
| IV. Test-first | Unitarios de dominio antes del código (formato del token, alcances, idempotencia, pistas de error, catálogo coherente con el router). Integración: aislamiento de las dos tablas en los tres casos; token de lectura no escribe; revocado/vencido/dado de baja no entra; otra empresa no ve nada; idempotencia deja un solo gasto. La migración lleva su test | OK |
| IV. Migración tolera la versión anterior | Solo agrega: dos tablas y un permiso. La versión anterior no conoce el permiso y `effectivePermissions` ya descarta los que el binario no conoce. Nada se renombra ni se quita | OK |
| V. Seguridad | Ver «Seguridad» abajo. `security-auditor` sobre este plan y sobre el código | OK, a revisar |
| VI. YAGNI | Sin OAuth ni registro dinámico de clientes (un token personal basta para Claude Code). Sin sesiones MCP, sin recursos ni prompts MCP: solo herramientas. Sin reintentos ni colas. El SDK es dependencia nueva porque la stdlib no habla MCP (R1) | OK |
| VII. Comentarios / idioma | Identificadores, nombres de herramienta, alcances y claves de bitácora en inglés; descripciones de herramienta y errores en español | OK |
| VIII. Puertas | Ninguna puerta se cierra. «Saber de quién es un pedido»: las herramientas no crean pedidos. El token guarda persona y nombre del token, y cada evento lo dice: deja abierta la distinción por estación/agente | OK |
| Tableta | Pantalla nueva con maqueta elegida (A); controles de 44 px, vigencia con `Picker`, sin `<select>` nativo, confirmación separada para revocar | OK |

## Seguridad (principio V)

| Control | Diseño | Prueba que lo demuestra |
| --- | --- | --- |
| Identidad | `Authorization: Bearer egbpat_<empresa>_<id>_<secreto>`; secreto de 32 bytes aleatorios en hex (64 caracteres). Se guarda `sha256(secreto)` y una pista de 4 caracteres; el prefijo visible se compone al leer. Comparación en tiempo constante | `TestParseAPIToken*` (domain), `TestAPITokenAuth*` (integración) |
| Empresa | La empresa del token fija el GUC; la búsqueda del token es por id **bajo RLS**: un token que declara otra empresa no se encuentra. Ninguna herramienta recibe empresa ni sucursal | `TestAPITokenFromOtherCompanyIsUnknown`, `TestMCPOtherCompanySeesNothing` |
| Rechazo uniforme | No existe, revocado, vencido, persona dada de baja o rol inactivo → el mismo 401, mismo cuerpo, siempre se calcula la huella | `TestAPITokenRejectionsLookAlike` |
| Permisos | Alcance (lo checa la capa MCP antes de despachar) **y** permiso del rol (lo checa el `RequirePermission` de la ruta, leído de la base en cada llamada). `tools/list` muestra solo lo que pasa ambos | `TestReadScopeCannotWrite`, `TestRoleLosesPermissionMidway`, `TestEveryToolMatchesItsRouteGate` |
| Principal interno | El despacho pone en el contexto una identidad con una llave **no exportada**; `RequireAuth` la honra. Una petición de afuera no puede fabricar un valor de contexto | `TestRequireAuthIgnoresHeadersPretendingInternal`, revisión del auditor |
| Límite | Por IP antes de autenticar (300/min) y por token después (120/min); las rutas de escritura conservan su `rateLimitUser` | `TestMCPRateLimitPerToken` |
| Idempotencia | Clave obligatoria al aplicar; única por token; misma clave + mismos argumentos = respuesta guardada; distintos = error; en curso o desconocido = error que pide revisar | `TestIdempotencyDecision` (domain), `TestSameKeyTenTimesOneExpense` (integración) |
| Vista previa | `dry_run` vale `true` si no se manda: aplicar exige `dry_run: false` explícito | `TestWriteWithoutDryRunIsPreview` |
| Bitácora | Un evento `mcp.tool_called` por llamada (persona, token por id y nombre, herramienta, modo, estado, argumentos, antes y después en escrituras). Los eventos que la operación ya deja llevan en `device` `{"agent":"mcp","token":…,"tool":…}`. Crear y revocar dejan `api_token.created` / `api_token.revoked` | `TestEveryCallLeavesAnAuditEvent` |
| Secretos en logs | El token nunca se loguea (el `RequestLogger` no escribe headers; el secreto no viaja en la URL). Los argumentos siguen la regla del cuerpo (solo debug o 5xx, redactado) | `TestMCPLogsNeverContainTheToken` |
| Gestión de tokens | `integrations.manage`, solo admin por omisión; un token siempre es de quien lo crea; vigencia obligatoria 30/90/180 días; un token **no** puede usarse para crear tokens (las rutas de integraciones no son herramientas) | golden de rutas, `TestTokensAreAlwaysOwnedByTheCreator` |
| DNS rebinding / CSRF | Ver ajuste L3 abajo: `Origin` no permitido se rechaza, token solo por header, `GET`/`DELETE` 405 | `TestMCPRejectsForeignOrigin` |

### Ajustes de la auditoría de seguridad del plan (2026-11-10)

Todos entran; cada uno con su prueba.

| # | Hallazgo | Decisión |
| --- | --- | --- |
| H1 | El principal interno pasa `RequireAuth` y abre rutas sin gate (`/me/pin` no pide el PIN actual: un token de lectura podría fijar el PIN del admin y desbloquear la tableta) | **Lista blanca en el despachador**: recibe el nombre de la herramienta, sus parámetros de ruta como `int64` y su query como `url.Values`, nunca una ruta en texto; resuelve con `chi` y exige que el patrón coincida con el del catálogo; si no, rechaza sin llegar al handler y deja `mcp_dispatch_denied`. Tipo del principal y despachador sin exportar. Pruebas: `TestDispatcherRefusesRoutesOutsideCatalog` (`/me/pin`, `/me/password`, `/users`, `/roles`, `/integrations/tokens`, `/auth/pin-switch`, `/orders/{id}/pay`) y `TestPATIsRejectedOnRegularRoutes` |
| H2 | El `device` del agente se perdería (solo se guardan cinco llaves) y la IP sería la de la petición sintética | El principal interno lleva el `RequestOrigin` de la petición de afuera (IP real con `rateKeyIP`) y el aparato `{"agent":"mcp","token":…,"tool":…}` armado en el servidor; `withRequestOrigin` no lo sobreescribe cuando hay principal interno. Integración: un gasto aplicado deja la IP del cliente y `device.agent = "mcp"` |
| H3 | Retener una conexión durante el despacho agota el pool (10) con llamadas paralelas | **Ninguna conexión se retiene a través del despacho**: token, permisos, idempotencia y bitácora van cada uno en su propio `store.WithTenant` corto. Prueba con pool de 2 conexiones y 3 llamadas paralelas |
| H4 | Un PIN visto en la tableta basta para crear un token de 180 días | Crear un token pide la **contraseña** actual (con el limitador de intentos de login por usuario), lleva `rateLimitUser` y deja `logging.SecurityEvent("api_token_created")`. El correo de aviso queda fuera (VI) |
| H5 | Texto de terceros (notas de pedidos de plataforma) puede inyectar instrucciones al agente | Anotaciones MCP `readOnlyHint` en lecturas y `destructiveHint` en escrituras; `order_get` no devuelve nombre ni notas del cliente; la pantalla y el quickstart dicen que las escrituras no se aprueban en automático y recomiendan un token de solo lectura para consultar |
| M1 | `dry_run` como `bool` con cero = aplicar | `*bool`: ausente o `null` = vista previa; solo `false` explícito aplica |
| M2 | Ctx cancelado deja la clave «en curso» y la llamada sin evento | `context.WithoutCancel` al cerrar idempotencia y bitácora; la herramienta va dentro de la huella; argumentos canónicos (struct tipado → JSON) |
| M3 | La vista previa no debe leer por fuera de las rutas | Toda lectura de vista previa y de antes/después se despacha como `GET` del catálogo; si da 403 se omite con su nota |
| M4 | `blocked`+`record` deja pasar ráfagas; Redis caído = sin tope | El límite por token usa `recordAndOver` (atómico, falla cerrado) y cuenta por `tools/call`. El `rateLimitUser` de las rutas de escritura se comparte con la tableta del mismo usuario: aceptado |
| M5 | Un lote JSON-RPC evadiría el límite | Se rechaza un cuerpo que empiece con `[` (400) |
| M6 | 401 sin evento de seguridad | `SecurityEvent("api_token_rejected", token_id, reason, ip)`; la razón nunca va a la respuesta; prueba de que el log no contiene `egbpat_` |
| M7 | Un token pegado en el repo público | Hook de pre-commit que rechaza el patrón del token, con su test |
| L1 | Base64url mete `_` en el secreto | Secreto en **hex** (64 caracteres); regex canónica; sin ceros a la izquierda; la empresa del principal sale de la fila |
| L3 | La protección de localhost del SDK no aplica detrás de Caddy | Se rechaza un `Origin` presente que no esté permitido; el token solo por header (nunca `?token=`); `GET`/`DELETE` 405 |
| L4 | Estados terminales solo en la aplicación | `check (expires_at <= created_at + interval '181 days')` y trigger que impide volver `revoked_at` a nulo |
| L5 | Bitácora y asistencia mandan datos del personal al proveedor del modelo | Se dice en el spec (Assumptions); el alcance `read` lo incluye a propósito: es el mismo dato que ve el admin |
| A6 (código) | Cada lectura escribe un evento: un agente en bucle deja del orden de 170 mil filas al día por token (estimado) | Aceptado: el límite por token lo acota, cada evento es chico y la bitácora es lo que pidió el dueño («cada llamada»). Si crece, se agrupan las lecturas; agregarlo después cuesta lo mismo |
| A8 (código) | `product_update` manda el registro completo: un cambio de nombre hecho en la tableta entre la lectura y el envío se pisa | Aceptado y documentado: es la misma ventana que tiene la pantalla con dos tabletas abiertas en el mismo producto |
| L6 | Empresa inactiva y reset de contraseña | El token no mira `companies.is_active` (el login tampoco); un reset de contraseña no revoca tokens: se documenta en `docs/mecanica` |

## Pantallas y maquetas

**Integraciones** (`/integraciones`, ítem propio del riel al final, antes de «Interfaz»), visible
con `integrations.manage`. Opción **A** elegida: `Page fill`; encabezado con título, una línea de qué
es y botón «Crear token» (44 px) a la derecha; debajo, la lista de tokens como tarjetas (nombre,
estado, persona, vence, último uso, alcances como chips, «Revocar» de 44 px en la tarjeta)
con scroll interno. «Crear token» abre una hoja inferior: nombre + vigencia (`Picker` 30/90/180) en
un renglón, los cinco alcances como filas tocables y «Crear token»; al guardar, la misma hoja muestra
el token **una vez** con «Copiar token», los dos pasos para conectarlo, el comando y «Ya lo copié».
Revocar con `ConfirmSheet` destructiva. Componentes existentes: `Page`, `Picker`, drawer,
`ConfirmSheet`, botones y badges del tema.

Ajustes que pidieron la evaluación de la opción A y el `tablet-ui-reviewer` (se implementan):

- **La hoja del token no se cierra por fondo, Escape ni arrastre**: la única salida es «Ya lo
  copié»; «Copiar token» cambia a «Copiado». Test: el token sigue visible tras tocar el fondo.
- **La hoja dice qué hacer, no cómo funciona**: arriba solo el token y «Copiar token»; «Cómo
  conectarlo» (`InfoButton`, patrón de `PrintSettingsPage`) abre los pasos como instrucción («1. En
  tu computadora, guarda el token como POS_MCP_TOKEN. 2. Pega el comando en una terminal. 3. Abre
  Claude Code y pregunta "¿cuánto vendimos ayer?"») y el comando con «Copiar comando». El token va
  por variable de entorno porque así lo pidió el dueño; el comando no lo lleva incrustado.
- El comando sale de la URL de la API del build y del slug de la empresa, nunca fijo; se muestra en
  14 px con saltos antes de cada opción, sin partir palabras.
- Se conserva la palabra «token» (la del dueño); el subtítulo dice qué es en una frase.
- La tarjeta no muestra el prefijo del token: el nombre que puso quien lo creó basta para
  reconocerlo.
- **Activos primero**; revocados y vencidos plegados detrás de «Ver revocados y vencidos (n)».
- Nombre por omisión «Claude de <nombre de quien crea>»: el caso común no abre el teclado.
- Los cuatro alcances de escritura en dos columnas; «Consultar» es una línea fija, no una casilla
  desactivada. Medido con Playwright a 1024×600 que caben con «Crear» a la vista.
- Casillas, tarjetas y título con los componentes comunes (`ListRow`, el patrón de lista de
  `PlataformasPage`); si falta una casilla táctil común, se amplía `ListRow`, no se inventa otra.
- Ítem del riel al final (antes de «Interfaz»), con `permission: 'integrations.manage'`, ícono
  `LuKeyRound`: cajero y mesero no ven nada nuevo.
- Crear pide la contraseña actual (auditoría H4): campo de contraseña en la hoja, debajo de los
  alcances.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Integraciones | [Maquetas de Integraciones](https://claude.ai/artifact/5sWHuxoV3se6d936boLX35) | A | La que mejor se entiende sola (9/10 contra 7 y 6 de un evaluador sin contexto): quién, qué puede, vence y último uso en la tarjeta, «Revocar» junto a cada token y 2 toques para crear. B esconde «Crear» bajo el pliegue y C obligaba a meter pestañas en Negocio |

## Project Structure

### Documentation (this feature)

```text
specs/072-servidor-mcp/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── mcp-tools.md       # catálogo de herramientas: argumentos, ruta, permiso, alcance
│   └── api.md             # endpoints de gestión de tokens
└── tasks.md
```

### Source Code (repository root)

```text
server/
├── migrations/0111_api_tokens.sql               # tablas, RLS, grants, permiso 37
├── queries/api_tokens.sql                       # sqlc
├── internal/domain/apitoken.go (+_test)         # token, alcances, vigencias, idempotencia, catálogo, pistas
├── internal/domain/permission.go                # PermIntegrationsManage
├── internal/domain/audit.go                     # acciones mcp.tool_called, api_token.*
├── internal/app/api_tokens.go                   # servicio
├── internal/httpapi/mcp.go                      # middleware del token, servidor MCP, despacho
├── internal/httpapi/mcp_tools.go                # las 33 herramientas
├── internal/httpapi/handlers_integrations.go    # GET/POST tokens, revocar
├── internal/httpapi/middleware.go               # RequireAuth honra el principal interno
├── internal/httpapi/request_origin.go           # device del agente
├── internal/httpapi/router.go                   # /mcp y /integrations
├── internal/httpapi/testdata/route_gates.golden
└── internal/integration/api_tokens_test.go, mcp_test.go, migration_0111_test.go
web/src/
├── app/permissions.ts, routes.ts, nav.ts, App.tsx # permiso, ruta /integraciones e ítem del riel
└── features/admin/IntegrationsPage.tsx (+test)
docs/mecanica/personal-y-permisos.md             # cómo funciona la puerta para agentes
```

**Structure Decision**: monorepo existente; la capa MCP es otro transporte dentro de `httpapi`, al
lado de los handlers que despacha.

## Decisiones (dudas resueltas con la mejor recomendación)

1. **Despachar al router y no llamar a los servicios directo.** Varios efectos viven en el handler y
   no en el servicio: el evento de seguridad del precio por plataforma, la invalidación del menú y el
   aviso a las tabletas, el `rateLimitUser`, el `RequirePermission`, el `canAdjust` de anular un
   gasto. Llamar al servicio directo obligaría a copiarlos uno por uno, y el primero que cambie en el
   handler deja a la herramienta atrás («el hermano que no se movió»). El costo es serializar dos
   veces una petición pequeña.
2. **Las ediciones que en la pantalla reemplazan el registro completo** (producto; nombre + precio +
   tope de un extra) se arman leyendo el registro actual y cambiando solo lo pedido: la herramienta
   nunca manda un campo que el agente no pidió cambiar con un valor inventado. El tope por renglón de
   un extra se lee de las opciones de su grupo, porque la lista de extras no lo trae.
3. **Vista previa = lectura del estado actual + resolución de nombres + lo que se mandaría.** No
   ejecuta la escritura en una transacción que se revierte: los servicios abren su propia
   transacción y anidarla está prohibido (`errNestedTx`). La vista previa no garantiza el resultado
   y lo dice.
4. **Antes y después en la bitácora**: al aplicar, la herramienta corre la misma lectura de la vista
   previa antes y después del despacho.
5. **Idempotencia fuera de la transacción del handler.** Se marca «en curso» antes de despachar,
   «terminada» con el resultado si fue 2xx, se borra si fue 4xx (nada se aplicó), y queda
   «desconocida» si fue 5xx o se cortó: la siguiente con esa clave pide revisar en vez de repetir.
   Peor caso: un reintento rechazado de más, nunca una escritura doble.
6. **Sin `dry_run` en lecturas**; todas las listas con página y tope.
7. **Conexión desde Claude Code**: `claude mcp add --transport http` con el header
   `Authorization: Bearer ${POS_MCP_TOKEN}` (ver research R6 y quickstart).
8. **Pedidos solo de lectura** (spec): ninguna herramienta crea, cobra ni cancela.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Dependencia nueva (SDK de MCP) | El protocolo (negociación de versión, esquemas, transporte) cambia por versiones y Claude Code lo sigue | Un servidor JSON-RPC propio de ~300 líneas se quedaría atrás en la siguiente versión del protocolo y nadie se enteraría hasta que el cliente fallara |
| Principal interno en `RequireAuth` | Despachar al router con la identidad del token sin fabricar un JWT | Emitir un JWT interno pondría una credencial válida en memoria; llamar servicios directo duplica efectos (decisión 1) |
