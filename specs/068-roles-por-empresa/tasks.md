---

description: "Tareas de las entregas 2 y 3 de la 068: roles por empresa en la base"
---

# Tasks: Roles y permisos por empresa — entregas 2 y 3

**Input**: `specs/068-roles-por-empresa/` (plan.md, spec.md, data-model.md, contracts/, research.md)

**Tests**: TDD obligatorio (constitución IV). Cada test se ve fallar antes de su código. La entrega 1 está cerrada (historial del archivo).

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

- [X] T001 Confirmar que la 0108 sigue libre en `origin/develop` y en las ramas 065–067 (`git ls-tree`, `../egb-06*/server/migrations`); si no, renumerar en todo el spec.

---

## Phase 2: Foundational — catálogo, roles en la base y resolución por petición

**Purpose**: bloquea todas las historias. Al terminar, cada petición se decide contra la base con los mismos permisos que hoy.

### Tests primero

- [X] T002 [P] Test de dominio en `server/internal/domain/permission_test.go`: los 12 permisos nuevos (data-model.md, «Permisos nuevos y su siembra») existen, tienen área y mensaje sin nombres de rol; `SuggestedPermissions(code)` (renombre de `PermissionsFor`) reproduce la siembra de la tabla para admin/gerente/cajero/mesero.
- [X] T003 [P] Tests de dominio en `server/internal/domain/roles_test.go` (tabla): `EffectivePermissions(base, overrides)` (sugerido: base − quitados ∪ agregados; propio: solo agregados; un `false` en propio no cambia nada; un permiso desconocido se ignora); `ValidateRoleName` (vacío, >40, espacios, choque con nombre de sugerido sin distinguir mayúsculas ni acentos vía `CatalogNameKey`); `ApplyRoleEdit` (add/remove sobre un sugerido produce overrides mínimos; quitar `roles.manage` al admin → `ErrRoleAdminKeepsRoles`); `CanDeactivate(activeUsers)`; `CanDelete(isSystem, anyUsers)`.
- [X] T004 Test de migración `server/internal/integration/migration_0108_test.go` contra respaldo restaurado con dos empresas (`TEST_RESTORED_DATABASE_URL`): cada empresa queda con sus 4 sugeridos activos; cada usuario con `role_id` del sugerido equivalente a su `role`; ninguna FK cruza empresas; `system_role_permissions` coincide con `domain.SuggestedPermissions` (`TestThePermissionCatalogMatchesTheDomain`, también códigos y áreas contra `domain.AllPermissions`).
- [X] T005 Test de integración `server/internal/integration/roles_sync_trigger_test.go` como `gatobobah_app` y como owner: insert con solo `role` → `role_id` del sugerido; insert con solo `role_id` → `role` con el código (propio → `mesero`); update de `role` sin `role_id` → `role_id` sigue; cambian los dos → gana `role_id`; empresa sin roles (creada a mano) → el trigger crea sus sugeridos; con `search_path` vacío (como `pg_restore`) funciona.
- [X] T006 Test de integración `server/internal/integration/roles_isolation_test.go` con `inTheThreeCases` sobre `roles`, `role_permission_overrides` y la consulta de permisos efectivos; un usuario cuyo rol está inactivo (puesto así por SQL) recibe cero permisos y 403 en toda ruta con permiso (FR-017); asignar a un usuario un `role_id` de otra empresa falla por la FK compuesta aun como owner.

### Implementación

- [X] T007 Migración `server/migrations/0108_company_roles.sql`: tablas universales con ids fijos y siembra (data-model.md); `roles` y `role_permission_overrides` con RLS `nullif`, grants, índices y checks; `users.role_id` + FK compuesta + índice; función `ensure_company_roles(company_id)`; relleno de roles por empresa y de `users.role_id`; trigger `sync_user_role` como dice plan.md «Ajustes de la revisión de arquitectura»; Down que se niega con roles propios o ajustes.
- [X] T008 Renglón de `guardedDownCleanup` para la 0108 en `server/internal/integration/harness_restaurado_test.go` (reasigna usuarios con rol propio a `mesero`, borra ajustes y roles propios); clasificar las columnas de texto nuevas para el respaldo anónimo (`noPersonalData` para catálogo; `roles.name` al paso 5 de `scripts/respaldo-anonimo.sh` y su verificación `FUGAS`).
- [X] T009 Consultas sqlc en `server/queries/roles.sql`: `UserEffectivePermissions` (por usuario, sin nombrar `users.company_id`), `ListRolesWithCounts`, `GetRoleForUpdate`, `BumpRoleVersion` (`where id and version`), `InsertRole`, `UpdateRoleName`, `SetRoleActive`, `DeleteRole`, overrides (upsert/delete/list), `ListPermissionCatalog`, `CountActiveUsersWithRole`, `CountUsersWithRole`, `CountActiveRoleAdmins`; `make sqlc`.
- [X] T010 `server/internal/domain/permission.go`: 12 constantes nuevas, área y etiqueta por permiso, mensajes; `PermissionsFor` → `SuggestedPermissions(SystemRoleCode)` con la siembra; `server/internal/domain/roles.go` con las reglas de T003 y sentinels (`ErrRoleNameTaken`, `ErrRoleInUse`, `ErrRoleSystem`, `ErrRoleAdminKeepsRoles`, `ErrLastRoleAdmin`, `ErrRoleChanged`, `ErrRoleInactive`); mapeo en `server/internal/httpapi/respond.go`.
- [X] T011 Resolución por petición: `PermissionResolver` pasa a `func(ctx, AuthUser) ([]domain.Permission, error)` en `server/internal/httpapi/middleware.go`; middleware que carga y memoriza los permisos en el contexto después de `WithTenant`; `RequirePermission`/`RequireAnyPermission` leen de ahí; `permissionsOf` y los `h.permissions(u.Role)` de handlers (backoffice, attendance, expense_edit, waste) pasan a la misma fuente. Servicio `app.RolesService.EffectivePermissions` en `server/internal/app/roles.go`.
- [X] T012 Sesión (`server/internal/app/auth.go`, `server/internal/httpapi/handlers.go`): login, refresh y `/auth/me` resuelven permisos con la conexión de la empresa de la sesión y devuelven `roleId`, `roleName`, `role` (código o `""`), `permissions`. JWT: `role` = código del sugerido o `""`. Test en `server/internal/integration/permissions_http_test.go` (sigue siendo arreglo; un usuario con rol propio recibe los suyos; un token emitido sin `roleId`, como los de la API anterior, se resuelve por el usuario y funciona igual).
- [X] T013 `CreateCompany` (`server/queries/companies.sql` / `server/internal/app/…`) llama `ensure_company_roles` en la misma transacción; `-create-company` y `-reset-admin` (`server/cmd/api/main.go`, `server/internal/app/reset_admin.go`) asignan el rol admin por `role_id`. Test: empresa nueva nace con sus 4 sugeridos y su admin con rol admin.
- [X] T014 Fixture `makeUserIn` en `server/internal/integration/harness_test.go`: sigue recibiendo el código de rol; el trigger asigna `role_id` (entrega 2). Los resolvers propios de las pruebas (`func(domain.Role) …`) pasan a la firma nueva.

**Checkpoint**: `go test ./...` y `make test-integration` verdes; cada petición se decide contra la base.

---

## Phase 3: US1 — Nada cambia para quien ya opera (P1) 🎯

**Goal**: los 42 `RequireRole` y los gates del front pasan a permisos sin cambiar lo que puede ningún rol sugerido.

### Tests primero

- [ ] T015 [US1] Test de equivalencia `server/internal/integration/roles_equivalence_test.go`: recorre las rutas del router (chi `Walk`) y, para cada ruta con gate, para cada rol sugerido, compara permitido/negado contra la tabla esperada (la de hoy, escrita en el test a partir de los `RequireRole` actuales). Debe fallar mientras falte un permiso en la siembra.
- [ ] T016 [P] [US1] Test estático `server/internal/httpapi/no_role_gates_test.go`: ningún `RequireRole(` en `router.go` ni en el paquete (falla hoy).
- [ ] T017 [P] [US1] Tests de front: `web/src/app/roles.test.ts` → `routes.test.ts` (`ROUTE_PERMISSIONS`/`canOpen` por permiso, nunca por rol); `AppShell.test.tsx` con sesiones por permisos; `POSPage`, `ModifierSheet`, `CashPage`, `PlataformasPage` muestran sus controles según `can()` (un admin sin el permiso no los ve); test estático que no quede `role ===` en `web/src` fuera de la consola.

### Implementación

- [ ] T018 [US1] `server/internal/httpapi/router.go`: cada `RequireRole` a su permiso según data-model.md (tabla y «absorben»); borrar `RequireRole` y su test de `middleware.go`/`middleware_test.go`; actualizar los tests de integración que fijaban 403/2xx por rol solo si la tabla de equivalencia lo confirma idéntico.
- [ ] T019 [US1] `server/internal/domain/uso.go` y `toque.go`: `rolesPorPantalla` → `permisoPorPantalla`; `PantallaPermitida(pantalla, permisos)`; la medición guarda el código del sugerido o nulo; `server/internal/app/uso.go` cuenta activos por sugerido vía `roles`; `server/queries/uso.sql` sin `users.role`. Tests de `uso_test.go`/`toque_test.go` y de integración ajustados (mismo comportamiento para los sugeridos; rol propio → sin rol).
- [ ] T020 [P] [US1] Front: `web/src/app/roles.ts` → `ROUTE_PERMISSIONS` + `canOpen(path, user)`; `RequireAuth.tsx`, `AppShell.tsx`, `App.tsx`; `POSPage.tsx` (`menu.configure`, `cash.operate`), `ModifierSheet.tsx` (`menu.configure`), `CashPage.tsx` (`cash.configure`), `PlataformasPage.tsx` (`platforms.credentials`); `permissions.ts` con los 12 permisos nuevos; `web/src/stores/session.ts` con `roleId`/`roleName`.
- [ ] T021 [US1] Front: refrescar `/auth/me` al volver al frente y al recibir un 403, y actualizar `permissions` en el store (FR-018), en `web/src/api/client.ts` o el hook de sesión; test vitest.

**Checkpoint**: T015–T017 verdes; equivalencia ruta por ruta.

---

## Phase 4: US2 — La administradora ajusta lo que puede un rol (P1)

### Tests primero

- [ ] T022 [US2] Integración `server/internal/integration/roles_http_test.go`: `PATCH /roles/{cajero}` con `add`/`remove` cambia lo que puede; la siguiente petición de un cajero con sesión abierta se niega/permite sin renovar; `version` vieja → 409 `ROLE_CHANGED`; quitar `roles.manage` al admin → 409; `reset` deja el sugerido sin ajustes; un permiso agregado a `system_role_permissions` después llega al cajero si no se quitó y no llega si se quitó (herencia, FR-004); la pantalla que guardó sin conocer el permiso nuevo no lo quita.
- [ ] T023 [P] [US2] Vitest `web/src/features/admin/RolesPage.test.tsx`: abrir un rol abre la hoja (render siguiente), áreas plegadas con «n de m», marcar/desmarcar cuenta «N cambios», Guardar manda solo `add`/`remove`, Cerrar con cambios pide confirmar, el admin muestra «Administrar roles» fijo.

### Implementación

- [ ] T024 [US2] `server/internal/app/roles.go`: `Update`, `Reset` con candado 28068, `GetRoleForUpdate`, versión con `where version`, overrides mínimos vía `domain.ApplyRoleEdit`, bitácora en la misma transacción.
- [ ] T025 [US2] `server/internal/httpapi/handlers_roles.go` + rutas en `router.go` (`GET /permissions`, `GET /roles`, `PATCH /roles/{id}`, `POST /roles/{id}/reset`) con `roles.manage` (y `GET /roles` también `employees.manage`).
- [ ] T026 [US2] `web/src/features/admin/RolesPage.tsx` (Maquetas: fila «Roles», opción A): lista con `Page fill`, filas de 56 px, hoja inferior del rol con áreas plegables, barra fija con «N cambios» y Guardar; `web/src/api/admin.ts` con las llamadas; pestañas Personal/Roles en `EmployeesPage.tsx` según permisos (sin barra si hay una sola).

---

## Phase 5: US3 — Rol propio y asignación (P1)

### Tests primero

- [ ] T027 [US3] Integración en `roles_http_test.go`: `POST /roles` crea un propio; nombre repetido (también contra «Cajero», sin acentos ni mayúsculas) → 409; asignarlo con `PATCH /users/{id}` (`roleId`) y comprobar ruta por ruta lo que puede; asignar un rol inactivo → 409; un rol de otra empresa → 404/409; un permiso nuevo del producto no llega al propio; renombrar un propio funciona y a un nombre repetido → 409; renombrar un sugerido → 409 `ROLE_SYSTEM`.
- [ ] T027b [US3] Integración en `server/internal/integration/roles_last_admin_test.go`: cambiar de rol o dar de baja al último usuario activo con `roles.manage` → 409 `LAST_ROLE_ADMIN`; con dos, la primera baja pasa; dos bajas simultáneas de los dos únicos (dos transacciones) dejan al menos uno activo; quitar `roles.manage` a un rol propio cuyo único usuario es el último administrador → 409.
- [ ] T028 [P] [US3] Vitest `EmployeesPage.test.tsx`: `Picker` de rol con los roles activos + el actual inactivo marcado; el alta exige elegir; el `Picker` abierto sobre el diálogo de edición funciona (la hoja responde y cerrar el `Picker` no cierra el diálogo).

### Implementación

- [ ] T029 [US3] `app/roles.go` `Create` (candado 28068, nombre vía `domain.ValidateRoleName` contra propios y `system_roles.name`); `POST /roles`.
- [ ] T030 [US3] `server/internal/app/users.go` y `handlers_users.go`: `roleId` (y `role` por compatibilidad durante la subida); candado 28068 en alta, edición, baja y reactivación; rol activo de la empresa; `ErrLastRoleAdmin` al cambiar de rol o dar de baja al último usuario activo con `roles.manage`; reactivar con rol inactivo → `ErrRoleInactive`; `server/queries/users.sql` sin `role` (escribe `role_id`). `SetUserSecretsByUsername`/reset-admin con el candado.
- [ ] T031 [US3] `web/src/features/admin/EmployeesPage.tsx`: `Picker` de roles (Maquetas: fila «Empleados»), tabla con el nombre del rol, alta sin rol preelegido; si el `Picker` sobre el diálogo choca en T028, el diálogo pasa a hoja inferior. Crear rol en `RolesPage.tsx`: nombre y «Copiar permisos de…» (`Picker`) en un renglón, sin abrir el teclado solo.

---

## Phase 6: US4 — Inactivar, reactivar y borrar (P2)

- [ ] T032 [US4] Integración en `roles_http_test.go`: inactivar con empleados activos → 409 con cuántos; sin ellos → OK y no se ofrece; reactivar conserva permisos; admin no se inactiva (409 y `check` de la base); borrar sugerido → 409; propio sin usuarios → OK; propio con usuario dado de baja → 409; carrera inactivar contra asignar (dos transacciones) deja un estado válido.
- [ ] T033 [US4] Primero el Vitest de las acciones en `RolesPage.test.tsx` (en rojo); después `app/roles.go` `Deactivate`, `Reactivate`, `Delete` con candado y `for update`; rutas; `RolesPage.tsx`: acciones al final del cuerpo, «Borrar» con `ConfirmSheet`, rechazo de inactivar con botón a Personal filtrado por el rol.

---

## Phase 7: US5 — Bitácora (P2)

- [ ] T034 [US5] Acciones `role.created`, `role.renamed`, `role.permissions_changed`, `role.reset`, `role.deactivated`, `role.reactivated`, `role.deleted`, `user.role_changed` en `server/internal/domain/audit.go` (con detalle en códigos) y `web/src/shared/audit/labels.ts`; cada una producida por un test (`TestEveryAuditActionIsProduced`); `recordAudit` dentro de la transacción en `app/roles.go` y `app/users.go`.

---

## Phase 8: Polish (entrega 2)

- [ ] T035 [P] `AGENTS.md`: sección «Roles por empresa» (la consulta de permisos por petición, el trigger de convivencia hasta la 0109, cómo se agrega un permiso nuevo del producto: constante + migración con su fila en `permissions` y `system_role_permissions`, y el test que lo exige). Constitución: tabla de puertas, renglón de roles marcado como cruzado (spec 068), y «Roles: modelo híbrido» sin la frase de los `RequireRole` que quedan; enmienda MINOR con su versión.
- [ ] T036 Gates: `go build ./... && go test ./...`, `make test-integration`, `bun run lint && bun run typecheck && bun run vitest run && bun run build`; `explain (analyze, buffers)` de la consulta de permisos en la integración con ~100 usuarios, anotado en research.md.
- [ ] T037 Prueba a 1024×600 con usuario nuevo contra el `make start` del worktree con respaldo restaurado (quickstart.md, paso 4); capturas solo en `test-results/` o scratchpad.
- [ ] T038 `/revision-de-codigo`, corrección con test de cada hallazgo; `make ci-local`; PR a develop, release a main y despliegue hasta `/readyz` 200, con respaldo previo.

---

## Phase 9: Entrega 3 — borrar `users.role` (subida posterior)

- [ ] T039 Test de migración `server/internal/integration/migration_0109_test.go` contra respaldo con dos empresas: rellena roles y `role_id` faltantes (empresa creada por la API anterior), `role_id` queda `not null`, `users.role` y el trigger desaparecen, la medición sigue con su enum; con la API de la entrega 2 sirviendo, login y una ruta con permiso siguen funcionando.
- [ ] T040 Migración `server/migrations/0109_drop_user_role.sql` (relleno idempotente, `check not valid` → `validate` → `set not null`, `drop trigger/function`, `drop column role`); Down que recrea `role` desde `role_id`; su renglón en `guardedDownCleanup`.
- [ ] T041 Fixture `makeUserIn` asigna `role_id` por código de sugerido (sin `role`); `sqlc.yaml` y `server/queries/users.sql` sin `role`; `make sqlc`; gates; PR, release y despliegue hasta `/readyz` 200.
- [ ] T042 Cerrar: comentar y cerrar #232 y #61, borrar la rama remota, `make cerrar-worktree datos=1`.

---

## Dependencies & Execution Order

- Phase 2 bloquea todo (T007 depende de T004–T006 en rojo; T009 de T007; T011–T014 de T009–T010).
- US1 (Phase 3) antes que US2–US5: la equivalencia fija la siembra que los demás editan.
- US2 → US3 → US4 comparten `roles_http_test.go` y `RolesPage.tsx`: en orden. US5 después de US2–US4 (las acciones ya existen).
- Phase 9 solo después de que la entrega 2 esté en producción.

## Parallel Example

```text
Phase 2 tests: T002 ‖ T003 ‖ T005 ‖ T006
US1: T016 ‖ T017 (rojo) → T018 ‖ T020
```

## Implementation Strategy

Entrega 2 = Phases 2–8 en una subida (aditiva, con trigger de convivencia). Entrega 3 = Phase 9 en la
subida siguiente. US1 es la red de seguridad: sin su equivalencia en verde no se sube nada.
