# Contrato — roles

Todas bajo `/api/v1`, con sesión y empresa. Errores con el formato de siempre.

| Ruta | Permiso | Cuerpo / respuesta |
| --- | --- | --- |
| `GET /permissions` | `roles.manage` | `{items: [{code, area, label}]}` — el catálogo, en orden fijo |
| `GET /roles` | `roles.manage` o `employees.manage` (Empleados necesita la lista para asignar) | `{items: [{id, systemCode|null, name, active, version, users, permissions: [code], added: [code], removed: [code]}]}` |
| `POST /roles` | `roles.manage` | `{name, permissions: [code]}` → rol creado (un rol propio no tiene base: aquí sí va la lista) |
| `PATCH /roles/{id}` | `roles.manage` | `{version, name?, add?: [code], remove?: [code]}` → rol. Solo lo que la persona tocó: un permiso que el producto agregó mientras la hoja estaba abierta no se vuelve «quitado» |
| `POST /roles/{id}/reset` | `roles.manage` | `{version}` → rol sugerido sin ajustes |
| `POST /roles/{id}/deactivate` · `/reactivate` | `roles.manage` | `{version}` |
| `DELETE /roles/{id}` | `roles.manage` | — |

| Error | HTTP | Código | Cuándo |
| --- | --- | --- | --- |
| `ErrRoleNameTaken` | 409 | `ROLE_NAME_TAKEN` | Nombre repetido |
| `ErrRoleInUse` | 409 | `ROLE_IN_USE` | Inactivar con empleados activos / borrar con usuarios (mensaje con cuántos) |
| `ErrRoleSystem` | 409 | `ROLE_SYSTEM` | Borrar o renombrar un sugerido |
| `ErrRoleAdminKeepsRoles` | 409 | `ROLE_ADMIN_KEEPS_ROLES` | Quitar `roles.manage` al admin o inactivarlo |
| `ErrLastRoleAdmin` | 409 | `LAST_ROLE_ADMIN` | Dejar a la empresa sin usuario activo con `roles.manage` |
| `ErrRoleChanged` | 409 | `ROLE_CHANGED` | `version` vieja |
| validación | 400 | | Nombre vacío / > 40 / permiso desconocido |

## Usuarios

`POST /users` y `PATCH /users/{id}` aceptan `roleId` (rol activo de la empresa). Durante la subida
aceptan también `role` (código de sugerido) si no viene `roleId`. Respuesta con `roleId`, `roleName`.

## Sesión

`/auth/login`, `/auth/refresh`, `/auth/me`: `user.roleId`, `user.roleName`, `user.role` (código del
sugerido o `""`), `user.permissions` (vigentes).
