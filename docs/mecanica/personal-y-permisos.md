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
- **Cambio obligatorio de contraseña** (#311). `users.must_change_password` lo enciende el alta y el
  restablecimiento por un admin. Antes solo lo hacía cumplir la pantalla de login con un desvío en
  memoria, y una recarga lo saltaba. Las formas de evadirlo que se enumeraron antes de arreglarlo, y
  qué cierra cada una:
  1. **Recargar la página**: el desvío vivía en la memoria del login. Ahora la aplicación entera se
     cambia por la pantalla de cambio en cuanto la sesión trae el indicador, venga del login, del
     refresh, del relevo por PIN o de `/auth/me`.
  2. **Llamar a la API sin pantalla** con el token: `RequirePasswordChanged` rechaza con 403
     `PASSWORD_CHANGE_REQUIRED` y el evento `password_change_required`. Pasan solo
     `POST /me/password`, `GET /auth/me` y `POST /auth/logout` (que no exige sesión).
  3. **Un token emitido antes del restablecimiento**, o renovado con el refresh: el indicador se lee
     de la base en cada petición, nunca del token. Un admin que restablece a alguien con la sesión
     abierta lo detiene en la siguiente petición.
  4. **Entrar como esa persona con su PIN** (relevo): la sesión que sale trae el indicador y queda
     bajo el mismo control. Quien tiene el indicador tampoco puede usar `pin-switch` ni
     `unlock-options`.
  5. **Las rutas fuera del grupo con empresa**: `/events` (pedidos en vivo) y `/version` llevan el
     mismo control, que abre su propia conexión de la empresa.
  6. **Un token de agente (MCP) de la misma persona**: cada herramienta despacha una ruta del router
     y pasa por el mismo control. La herramienta responde rechazada hasta que la persona cambie su
     contraseña; el token no se revoca.
  7. **«Cambiar» a la misma contraseña que puso el admin**: se rechaza; el admin la conoce.
  8. **La lectura del indicador falla**: se rechaza la petición (cerrado), nunca se deja pasar.
  9. **La consola de plataforma**: otra firma y otra tabla (`platform_operators`); su token no
     abre ninguna ruta del POS, y el del POS no abre la consola.
  10. **Adivinar la contraseña del admin desde `/me/password`**, la única ruta que queda abierta
      (con el PIN del empleado nuevo basta para llegar ahí): se frena por persona (`pwchange:<id>`,
      el mismo bloqueo que el login) y por IP, con el evento `password_change_failed`.
  11. **«Recuperarla» por correo con la misma contraseña**: `/auth/reset` también la rechaza.
  12. **Un `/events` abierto antes del restablecimiento**: el stream vuelve a revisar el indicador en
      cada ping (25 s) y se corta.

  `TestEveryAuthenticatedRouteEnforcesTheForcedChange` recorre el router entero con una sesión
  pendiente: una ruta nueva sin el control truena ahí. Las exentas están nombradas en la prueba, cada
  una con su razón. **Lo que se nota al operar**: con alguien con el cambio pendiente en la tableta no
  hay relevo por PIN (su `pin-switch` es 403) y la pantalla del cambio solo ofrece cambiarla o cerrar
  sesión; si esa persona entra por relevo, la aplicación de quien estaba se desmonta (el carrito sí
  sobrevive, vive en `egb:ticket:v2`).
- **Dadas de baja** (#236). Cinco cosas que no se ven:
  - **Quién administra a una baja se decide con lo que puede su rol asignado, no con lo que puede
    ahora** (`manageablePermissions`). Ahora una baja no puede nada, así que `WithinPermissions`
    pasaba siempre: quien solo administra personal le fijaba el PIN a un admin dado de baja y entraba
    como admin cuando lo reactivaban. Lo vigila `TestNobodyManagesAnInactivePersonWhoCouldDoMore`.
  - **La baja revoca sus refresh tokens** en la misma transacción. Sin eso, el refresh que se quedó
    en una tableta volvía a dar sesión al reactivarla: el 401 de `Refresh` durante la baja revierte
    también su propia revocación. Lo vigila `TestDeactivatingRevokesTheSessionsOfBefore`.
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
- **Toda escritura declara los ids que manda (`refs`, #335) y se revisan antes que todo**, en la vista
  previa y al aplicar, con RLS (`FindReferences`): un id de otra empresa y uno que no existe reciben el
  mismo «El producto N no existe en esta empresa. Búscalo con …». Va antes de `route` porque varias
  rutas leen el registro para armar la escritura y cada una lo decía distinto. Hacía falta por dos
  razones: la vista previa no despacha la escritura (decía que todo estaba bien), y **muchas llaves
  foráneas son de una sola columna, y Postgres las revisa saltándose RLS**: con un id de otra empresa
  el insert pasaba y la fila quedaba apuntando a una ajena (medido: crear un extra en el grupo de otra
  empresa, un producto en su categoría, un gasto con su proveedor o su categoría). El esquema sigue
  así para la pantalla: #344. Una herramienta nueva con un `*_id` sin `refs` truena
  en `TestEveryWriteToolWithIDsDeclaresItsRefs`.
- **Una llave de idempotencia queda en `failed` (con el error) cuando el despacho no confirmó nada**:
  un rechazo 4xx, una `guard` que no pasa, o un 500 sin ninguna confirmación. Lo sabe el rastreador de
  `store.TrackWrites`, que anota cada `COMMIT` intentado y cada sentencia que no es `select` fuera de
  transacción. Una `failed` se retoma con la misma llave, con o sin los argumentos corregidos.
  `unknown` queda solo para lo que pudo haberse confirmado.
- **Un 500 le dice al agente el id del error y la causa por categoría**, nunca el texto: la petición
  interna lleva el `X-Request-Id` de la llamada, así que el correo del administrador y el error dicen
  el mismo id. La causa sale de `domain.InternalCause` (el campo `cause` de la respuesta de un 500).
- **Una FK que frena una referencia (23503 «is not present in table») responde 422
  `REFERENCE_NOT_FOUND`** en toda la API (`store.DomainError`), no 500. Con RLS Postgres no dice el
  valor, solo la tabla; de ahí sale el sustantivo.

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


### Lo que agregó la spec 090 (pedidos completos, correcciones de cobro, costo, recetas, modificadores y productos)

- **Tres alcances crecieron con la subida.** `write:orders` (antes solo cancelar) gana capturar, mandar
  a cocina, entregar, descontar y cobrar un pedido, y anular, mover de terminal o reembolsar un pago;
  `write:catalog` gana recetas, grupos, extras, ligas y el alta de productos; `write:inventory` gana
  `ingredient_cost_set`. Un token ya emitido con esos alcances las gana si el rol de su dueño tiene el
  permiso de su pantalla. El texto de cada alcance en Integraciones lo dice.
- **Seis escrituras no piden permiso, porque su pantalla tampoco**: crear, enviar y descartar una
  cuenta, entregar, descontar y cobrar. El catálogo las nombra en `screenWritesWithoutPermission`
  (`apitoken_test.go`); toda otra escritura sin permiso truena ese test. La barrera del token es el
  alcance, que elige quien lo crea.
- **El despachador acepta UUID para la cuenta en captura** (`dispatchDraft`). El id llega como
  `uuid.UUID`, no como texto: el camino solo lleva hex y guiones. `parseDraftID` solo acepta la forma
  canónica. Lo vigila `TestDispatcherRejectsNonUUIDDraftID`.
- **El id de la cuenta, el de cada renglón y el `clientUuid` del cobro se derivan de la clave**
  (`domain.DerivedUUID`, versión 8): un reintento manda el mismo id y la ruta lo reconoce aunque el
  registro de idempotencia se haya perdido.
- **La vista previa de una cuenta es `POST /pos/drafts/quote`**, que corre `priceDraft`, la misma
  cuenta que la cuenta guardada: la cifra de la vista previa es la del pedido al centavo
  (`TestDraftQuoteMatchesTheDraftItBecomes`). Cada renglón trae `priceSource` (`platform`, `markup`,
  `base`, `counter`) y una nota por cada precio que no es el propio de la plataforma.
- **Un pedido de plataforma se cobra por monto**, como en la pantalla: `all_remaining` es de la cuenta
  dividida y la ruta lo rechaza; la vista previa lo dice con lo que falta.
- **Cambiar el método de un pago es anular y volver a cobrar**; no hay herramienta propia.
- **`refund-info` trae `pending`**, lo que queda por devolver con la misma cuenta que `/refund`.
- **El motivo del costo (0116) es opcional para la pantalla y obligatorio para el MCP.** El historial y
  la bitácora lo traen; el respaldo anónimo lo reemplaza por «motivo anonimizado».
- **`recipe_set` exige `based_on` al aplicar** y no copia la receta a los extras homónimos salvo
  `also_in_same_name: true`. Con existencia propia, la vista previa trae las dos existencias y qué
  mandar; un 409 o un 400 de la ruta llega con la corrección para el agente.
- **El `PATCH` de extras ya es parcial** (defecto del 15-nov): el servicio lee la opción con candado y
  solo cambia lo que llega. `GET /admin/modifier-options/{id}` lee una opción aunque su grupo esté
  apagado; la lista sigue trayendo solo los de grupos activos.
### Lo que agregó la spec 077 (desactivar y reactivar insumos)

- **`write:inventory` creció otra vez, sin herramienta nueva.** `ingredient_update` acepta `active`
  y despacha las rutas de la hoja «Archivar» (`…/archive` con `{confirmOnHand}`, `…/reactivate`); la
  vista previa lee `…/archive-check`. Un token con almacén emitido antes de la 077 gana desactivar y
  reactivar con la subida si el rol de su dueño tiene `inventory.configure`.
- **`active` va solo en la llamada.** Son rutas distintas de la ficha y de los otros nombres; juntas
  dejarían un estado a medias. `confirm_on_hand` solo con `active: false`. Las dos reglas se revisan
  antes de despachar.
- **Ninguna regla del archivado vive en la capa MCP.** Existencia sin confirmar (409
  `ARCHIVE_HAS_STOCK`), receta activa o conteo abierto (409 `IN_USE`), nombre tomado al reactivar y
  unión los decide el servicio; la vista previa solo los dice, y la pista de cada código dice qué
  hacer (merma o ajuste, o la pantalla para las recetas).
- **Sin el alcance, la herramienta no existe para ese token**: llamarla es «unknown tool», no un
  mensaje que nombre el alcance. Eso es de la 072 para todas las herramientas.
- **`articles_search` con `include_inactive`** lee además Almacén › Archivados
  (`/stock/articles?status=archived`, `inventory.view`): un rol que solo captura compras recibe una
  nota en vez de una lista que callaría a los inactivos. Sin el parámetro, la búsqueda es la de la
  captura de compras, que a propósito no ofrece archivados.

- **`inventory.report_out` (spec 084, id 38)**: «Se acabó», «No lo encuentro», la campana y la
  franja. Llega por herencia a los cuatro roles sugeridos (lo puede usar cualquiera que esté en el
  local); a los roles propios de cada empresa, no. Configurar alertas, destinatarios y la lista de
  compra cuelgan de los permisos de almacén que ya existían (`inventory.configure`,
  `inventory.view`). Quién recibe por correo sigue al permiso `inventory.configure` más los correos
  extra; nunca al nombre de un rol.
