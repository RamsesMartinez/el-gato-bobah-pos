# Personal, permisos y bitácora

Notas de mecánica de esta área: lo que no se ve en el código ni en la pantalla y ya costó un defecto. Se leen **antes** de tocar roles y permisos por empresa, asistencia y bitácora de auditoría. Vienen de `AGENTS.md` (se movieron el 2026-11-07 porque las instrucciones que carga cada sesión pasaron de 150 mil caracteres); los principios siguen en la constitución.

- **Roles por empresa** (spec 068, 0108). Cinco cosas que no se ven:
  - **El permiso de cada petición se lee de la base**, una vez por petición (`MemoizedPermissions`,
    después de `WithTenant`): lo que puede un sugerido es `system_role_permissions` menos lo quitado
    más lo agregado en `role_permission_overrides`; un rol propio, solo lo agregado; un rol inactivo,
    nada. El rol del JWT no decide nada (solo va al log). Login, refresh y `/auth/me` resuelven con la
    conexión de la empresa de la sesión.
  - **Un permiso nuevo del producto son tres lugares**: la constante en `domain/permission.go` (con
    área, etiqueta y mensaje), una migración con su fila en `permissions` y, si lo trae un sugerido, en
    `system_role_permissions`, y su espejo en `web/src/app/permissions.ts`.
    `TestThePermissionCatalogMatchesTheDomain` truena si la base y el dominio no dicen lo mismo. Llega a
    todas las empresas sin migrarlas una por una (herencia), salvo a las que lo quitaron.
  - **Una ruta nueva va con `RequirePermission`**, y `testdata/route_gates.golden` dice qué rol
    sugerido pasa por cada ruta: `TestNoSuggestedRoleGainsOrLosesARoute` truena con una ruta nueva sin
    referencia o con un rol que gana o pierde una. Regenerarlo (`UPDATE_ROUTE_GATES=1`) es decidir a
    propósito quién puede qué: va en el PR con el porqué.
  - **Toda escritura del rol de un empleado, de si está activo o de un rol toma el candado 28068
    antes de leer** y termina exigiendo que quede al menos una persona activa que administre roles
    (`ensureARoleAdminRemains`). Un camino nuevo que asigne rol o dé de baja pasa por ahí.
  - **`users.role` ya no existe (0109)**: el rol de una persona es `users.role_id`, que apunta al rol de
    su empresa. Las consultas de `users.sql` listan sus columnas en vez de `*`: sqlc expande el `*` al
    generar y una columna borrada después tumba a la versión anterior de la API durante la subida
    (pasó con `users.role`; lo cazó `TestTheAPIWorksWithoutUsersRole` antes de subir). El enum
    `user_role` sigue siendo la dimensión «rol sugerido» de la medición de uso; lo de un rol propio
    se mide sin rol.
- **Dadas de baja** (#236). Tres cosas que no se ven:
  - **Una baja conserva su usuario y su PIN.** Los dos índices únicos (`users_company_username_key`
    y `users_pin_lookup_unico`) cubren también a las bajas, así que reactivar a alguien nunca choca
    con otra persona: nadie pudo tomarlos mientras estuvo fuera. El alta con el usuario de una baja
    responde `USERNAME_TAKEN`; antes caía en el mismo 23505 que el PIN y decía «ese PIN ya lo usa
    otra persona». Un índice único nuevo en `users` se distingue por nombre (`isUniqueViolationOf`).
  - **Reactivar es el mismo `PATCH /users/{id}` con `isActive: true`**, bajo el candado 28068 y con
    `WithinPermissions`. Con su rol inactivo responde `ROLE_INACTIVE`: la hoja de reactivar pide otro
    rol en el mismo paso en vez de reactivar el rol entero. Su contraseña, PIN y asistencia de antes
    siguen; vuelve a entrar en cuanto queda activa.
  - **La lista de bajas es `GET /users?status=inactive`** (`ListInactiveUsers`, sin paginar como la
    de activos). Sin `status` sigue siendo la plantilla activa, y uno desconocido es 400: caer a
    activos pintaría gente activa bajo «Dadas de baja».
- **Asistencia y bitácora de auditoría** (spec 033, 0089). Cinco cosas que no se ven:
  - **Todo camino nuevo que mueva dinero, caja, precios o asistencia escribe su evento** con
    `recordAudit` (o `auditOrder` / `auditCash`) **dentro de su transacción**, y su acción va en la
    lista blanca de `domain/audit.go` y en las etiquetas de `web/src/shared/audit/labels.ts`. Sin la
    misma transacción, una acción que se revierte deja evento y una que se confirma puede no
    dejarlo. `TestEveryAuditActionIsProduced` falla si una acción de la lista no tiene prueba que la
    produzca.
  - **La bitácora no se borra, ni como owner.** Un test que revierte migraciones por debajo de la
    0089 con pedidos creados pasa por `migrarAbajoHasta`, que la vacía antes (solo en pruebas, por
    `guardedDownCleanup`); el Down de la 0089 se niega con datos, igual que en producción.
  - **`attendance_entries.branch_id` queda nulo con varias sucursales y no se rellena después**: es
    lo que el selector de sucursal tendrá que resolver. Los eventos de pedido y caja sí guardan la
    sucursal del pedido o de la caja.
  - **La clave del aparato (`X-Device-Key`) la genera el navegador**: sirve para auditar, no
    identifica. CORS la anuncia junto con `X-Device-Info`; un header propio que CORS no anuncia
    tumba todas las peticiones desde otro dominio.
  - **Corregir asistencia propia es otro permiso** (`attendance.correct_own`). El router no puede
    decidirlo —depende de quién es el dueño del registro— y lo decide `domain.CanCorrect`.

## La puerta para agentes: servidor MCP (spec 072)

- **`/mcp` no llama servicios: despacha rutas de esta misma API.** Cada herramienta arma una
  petición a una ruta de su lista blanca (`mcpToolRoutes` en
  [mcp_tools.go](../../server/internal/httpapi/mcp_tools.go)) y la sirve el mismo router con la
  identidad del token. Así hereda el `RequirePermission` de la ruta (leído de la base en cada
  llamada), el `rateLimitUser`, la invalidación del menú y los eventos de seguridad que viven en el
  handler. Una herramienta nueva es una entrada en `domain.MCPTools()` (nombre, alcance, permiso) y su
  ruta en la lista blanca; `TestEveryToolMatchesItsRouteGate` truena si el permiso declarado no es el
  de la ruta (así se encontró que `/expense-categories` no pide permiso).
- **El despachador nunca recibe una ruta en texto.** Los parámetros de ruta son enteros y el patrón
  se compara con el de chi antes de servir. Sin esto, el principal interno —que `RequireAuth` acepta
  sin JWT— alcanzaría rutas que solo piden sesión, como `/me/pin`, que no pide el PIN actual.
- **La petición interna empieza su ruteo de cero** (`chi.RouteCtxKey` en nil). Heredar el contexto de
  ruta del `POST /mcp` hacía que todo GET despachado respondiera 405 sin cuerpo.
- **Ninguna conexión se retiene durante el despacho**: el token, los permisos, la idempotencia y la
  bitácora usan transacciones cortas propias. El despacho toma la suya; retener otra agota el pool.
- **El token** es `egbpat_<empresa>_<id>_<secreto hex>`. Se busca por id bajo RLS de la empresa que
  declara (una empresa cambiada no encuentra la fila) y se compara su sha256 en tiempo constante. Un
  rechazo no dice por qué; la razón va al evento `api_token_rejected`.
- **Qué no apaga un token**: quitarle `integrations.manage` a su dueño, resetearle la contraseña o
  desactivar la empresa. Lo apaga revocarlo, que venza, o dar de baja a la persona o a su rol.
- **Escrituras**: `dry_run` ausente es vista previa; aplicar pide `idempotency_key`. Una clave que
  termina en 5xx queda «desconocida» y no se reintenta sola: peor un rechazo de más que un gasto
  doble.
- **Los rechazos que la pantalla resuelve con una hoja** (caja sin turno, día ya cortado) llevan su
  corrección en palabras en `domain.ToolErrorHintFor`, porque el agente no ve esa hoja.

### Lo que agregó la spec 073 (compras, conteos, cancelar pedidos, abrir y cerrar caja)

- **Dos alcances nuevos, y dos viejos que crecieron.** `write:purchases` (compras con mercancía) y
  `write:orders` (cancelar) entraron con la 0112. Además `write:inventory` abarca ahora los conteos y
  `write:cash` abrir y cerrar caja: un token emitido antes de la 073 los ganó con la subida, sin que
  nadie lo volviera a crear. Lo deja escrito `TestMCPToolsListFollowsScopeAndPermission`.
- **El arqueo ciego por MCP es el de la ruta, no uno propio.** Las herramientas de caja solo leen
  `GET /cash-sessions/current` (filtrado por `vistaDelTurnoAbierto`) y la asistencia; nunca el detalle
  de un turno abierto ni el resumen de ventas. Lo propio de la capa MCP es que **cerrar exige cada
  cifra dictada** (`domain.MissingCloseDeclarations`): el servidor toma un método ausente como cero,
  correcto para la hoja (que no deja enviar sin capturar) y peligroso para un agente, que podría
  «ayudar» llenándolo con lo esperado. Lo vigila `TestMCPCashCloseKeepsTheBlindCount`. Quien tiene
  el permiso de ver ventas puede reconstruir el esperado igual que en la pantalla de Ventas: eso no
  cambia.
- **Una revisión contra el estado de ahora va en `guard`, no en `route`.** `route` corre antes de la
  clave de idempotencia; una revisión ahí hace que repetir un cierre ya aplicado falle con «no hay
  turno abierto» en vez de devolver lo guardado. `guard` corre después.
- **La vista previa de una compra es la captura revertida** (`PreviewPurchase`, `POST
  /expenses/purchase-preview`): la misma `prepareExpense` + `createExpenseIn` que guarda, con
  `lock_timeout` de 2 s, los constraints diferidos forzados antes de revertir (la guarda de insumos
  unidos corre en el commit) y sin dar de alta artículos. El costo es por insumo, de la compra
  completa. Un camino nuevo que cambie la captura de un gasto lo cambia para los dos.
- **El lote de conteo** despacha un `PUT` por renglón con `confirmable: true` y se detiene en el
  primero que falla; capturar fija la cifra, así que repetirlo con la misma clave no duplica nada.

### Lo que agregó la spec 074 (empaques de compra y corregir un gasto guardado)

- **Sin alcance nuevo: dos viejos que crecieron otra vez.** `presentation_create`/`presentation_update`
  van en `write:inventory` (permiso `inventory.configure`) y `expense_edit` en `write:expenses`
  (`expenses.edit`). Un token emitido antes de la 074 las gana con la subida si el rol de su dueño
  tiene el permiso; el texto de cada alcance en Integraciones lo dice.
- **Cuando la vista previa ES la operación, va en `strictPreview`.** La edición de un gasto corre
  entera en una transacción revertida (`?preview=1`): lo que la ruta rechaza ahí (gasto anulado,
  renglón ajeno, permiso de recibir) llega al agente como error. Con `preview`, que solo agrega notas,
  esos rechazos salían como una vista previa «exitosa».
- **La huella de la edición y la clave de idempotencia no se estorban.** Aplicar exige el
  `fingerprint` de la última vista previa; uno vencido es un 409 que libera la clave (rama 4xx de
  `apply`), así que el agente repite con la huella nueva y la misma clave. Tras aplicar con éxito,
  otra huella con la misma clave son otros argumentos: 409 de idempotencia.
- **«Cambiar un renglón» es quitar y agregar en la misma llamada.** La ruta no tiene otro verbo. El
  renglón agregado sin `arrived_on` queda por recibir (no entra al almacén): la vista previa lo dice.
  Quitar el único renglón deja el gasto en $0 y la ruta lo rechaza: eso es anular.
- **`presentation_update` parte del empaque de hoy**: la ruta reemplaza el registro, así que lo no
  mandado se manda como está (un normal, con su contenido en la unidad base; un aproximado, con su
  unidad de compra). La marca y la tara solo viajan si el agente las manda. **Residuo aceptado**: entre
  la lectura y el `PUT` no hay control de versión; si alguien cambia el contenido en la pantalla en
  ese instante, el agente lo regresa al de antes. Es el mismo patrón de la pantalla y de
  `product_update`; cerrarlo exige que la ruta acepte cambios parciales o una versión.
- **Las lecturas auxiliares piden sus propios permisos.** `presentation_update` lee
  `GET /stock/presentations` (`inventory.receive`) y, sin `qty`, `GET /units` (`expenses.capture`): un rol
  con solo `inventory.configure` recibe el rechazo de esa lectura. Falla cerrado.
- **La vista previa de una compra sugiere el empaque que falta** con insumo y unidad llenos, y deja
  marca y contenido para que los confirme la persona: adivinarlos del texto del ticket podría
  equivocar el contenido, que es lo que mueve el almacén.

### Lo que agregó la spec 076 (dar de alta y editar insumos; empaques de un insumo)

- **`write:inventory` creció otra vez.** `ingredient_create` e `ingredient_update` van ahí con
  `inventory.configure`. Un token con almacén emitido antes de la 076 las gana con la subida si el rol
  de su dueño tiene el permiso; el texto del alcance en Integraciones lo dice.
- **El aviso de duplicado va en `attention`, que sale primero.** El texto que lee el agente es el JSON
  de la respuesta con las llaves en orden alfabético: una nota al final de `notes` se lee después de
  `before` y `would_send`. Una vista previa que devuelve `attentionFirst` pone sus avisos en esa llave.
  Lo vigila el subtest «la vista previa pone primero el insumo parecido» de `TestMCPIngredientCreate`.
- **Los parecidos salen de dos búsquedas de la pantalla, no de una regla propia.** La de la hoja
  «Nuevo artículo» (`/stock/articles/similar`) no lee otros nombres; la de la lista de Almacén
  (`/stock/articles?q=`) sí, pero pide `inventory.view`. Si una de las dos falla, la vista previa lo
  dice: callarlo haría creer que no hay duplicado.
- **La edición son dos peticiones, en orden: ficha y después otros nombres.** Al revés, la ruta de
  otros nombres descarta el nombre vigente y «Jamón de pierna» no podría quedar como otro nombre de
  «Jamón». Si la segunda falla, la primera ya quedó y el error empieza con «La ficha ya quedó
  cambiada». La versión de la ficha (`expectedUpdatedAt`) se lee en la misma llamada; el agente no la
  manda.
- **Si la segunda petición falla con 5xx, el agente recibe 409 `PARTIAL`**, no el «puede que no se
  haya aplicado» de siempre: la ficha sí quedó, y con el 409 la clave queda libre para repetir. La
  bitácora MCP registra esa llamada como rechazada; el cambio de la ficha está en el
  `ingredient.updated` de la ruta, con el token como dispositivo. **Residuo aceptado**: otros nombres
  agregados en la tableta entre la vista previa y aplicar se pisan (la versión se lee al aplicar).
- **El tope de 10 otros nombres se revisa antes de la vista previa**, que hace una lectura por
  nombre: sin él, una sola llamada eran miles de lecturas contra un solo cupo del token.
- **Un rol que configura y no ve el almacén no edita insumos**: la lectura de la ficha pide
  `inventory.view` y se niega. Igual en la pantalla, que sin ese permiso no abre la ficha.
