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
