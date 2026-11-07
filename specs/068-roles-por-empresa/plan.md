# Implementation Plan: Roles y permisos por empresa — entregas 2 y 3

**Branch**: `068-roles-por-empresa` | **Date**: 2026-11-06 | **Spec**: [spec.md](spec.md)

**Input**: `specs/068-roles-por-empresa/spec.md` (entrega 1 cerrada; este plan cubre la 2 y la 3).

## Summary

Los roles salen del código y entran a la base, con la forma que decidió el dueño el 2026-11-06:

- **Universal** (sin empresa, mismo id en todos los ambientes): `permissions`, `system_roles` (1
  admin, 2 gerente, 3 cajero, 4 mesero) y `system_role_permissions` (lo que el producto le da a cada
  sugerido).
- **Por empresa** (con `company_id`, RLS y llaves compuestas): `roles` (una fila por sugerido en cada
  empresa, más los propios) y `role_permission_overrides` (lo agregado o quitado, permiso por
  permiso). `users.role_id` apunta a `roles (company_id, id)`.
- **Herencia** en una sola consulta: permisos de un usuario = (los del sugerido − quitados) ∪
  agregados, si su rol está activo. Un permiso nuevo del producto es una fila en
  `system_role_permissions` y llega a todas las empresas.
- **El permiso se decide contra la base en cada petición** (una consulta por petición, memorizada en
  el contexto): quitar un permiso aplica a la siguiente petición. El rol del JWT deja de decidir.
- Los 42 `RequireRole` pasan a `RequirePermission` con 12 permisos nuevos por capacidad; la siembra
  reproduce exactamente lo de hoy (tabla en [data-model.md](data-model.md)).
- Pantalla «Roles» dentro de Empleados; Empleados asigna cualquier rol activo.
- **Entrega 3**: borra `users.role`, el enum `user_role`, el trigger de convivencia, y pasa las
  columnas de medición a texto.

## Technical Context

**Language/Version**: Go 1.27.3 · TypeScript / React 19

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query, Zustand

**Storage**: Postgres. Migración **0108** (entrega 2) y **0109** (entrega 3). Si otra rama abierta llega antes con su 0108, se renumera.

**Testing**: unitarios en `domain`; integración contra Postgres (rol `gatobobah_app`, `inTheThreeCases`, `TestEveryCompanyTableIsIsolated`); test de la migración contra respaldo real con dos empresas; vitest; recorrido a 1024×600

**Target Platform**: API Linux + tabletas 1024×600

**Project Type**: web-service + web app

**Performance Goals**: una consulta más por petición autorizada, por índice (`users` por id → `roles` por pk → `system_role_permissions` y `role_permission_overrides` por pk). Medida en la integración; sin caché (VI: se agrega si la medición lo pide)

**Constraints**: la API anterior convive con la nueva durante cada subida (IV); sin bloqueos largos sobre `users` (tabla de cientos de filas)

**Scale/Scope**: 36 permisos (24 + 12), 4 sugeridos, decenas de roles por empresa como máximo

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I. Layering | Reglas puras en `domain/roles.go` (validar nombre, efecto de un cambio, admin conserva `roles.manage`, último administrador). `app/roles.go` orquesta con `WithTx` y candado. `httpapi` fino. SQL solo por sqlc |
| II. Errores | Sentinels nuevos en `domain` (`ErrRoleNameTaken`, `ErrRoleInUse`, `ErrRoleSystem`, `ErrRoleAdminKeepsRoles`, `ErrLastRoleAdmin`, `ErrRoleChanged`), mapeados solo en `httpapi.Error` |
| III. Dinero | No toca dinero; sí quién puede moverlo: la siembra se prueba ruta por ruta |
| IV. TDD | Primero: la tabla de equivalencia (cada ruta × cada rol sugerido, antes y después) como test de integración por el router real; luego el código. Migración con su test contra respaldo de dos empresas. Las tablas nuevas con `company_id` las vigila `TestEveryCompanyTableIsIsolated`, y sus consultas llevan `inTheThreeCases` |
| IV. Convivencia con la API anterior | Ver «Subida» abajo: 0108 es aditiva; un trigger mantiene `role` y `role_id` coherentes mientras viva la API anterior; la 0109 borra en la subida siguiente |
| V. Seguridad | Autorización en el servidor contra la base en cada petición; el front espeja con `can()`. Llaves compuestas impiden asignar un rol de otra empresa aunque RLS fallara (las FK saltan RLS). `security-auditor` en la revisión. «¿Un atacante lo evade?»: un usuario con token viejo de admin al que se le cambió el rol a cajero pide `/users` → 403 en la siguiente petición (test) |
| VI. YAGNI | Sin caché de permisos, sin jerarquía de roles, sin roles por sucursal. Un solo tipo de ajuste (agregar/quitar) para sugeridos y propios |
| VII. Idioma | Tablas, columnas, permisos y acciones de bitácora en inglés; textos en español |
| VIII. Puerta de roles | Se **cruza** con las decisiones del dueño del 2026-11-06. La tabla de puertas se actualiza al cerrar la entrega 3 |
| Roles: modelo híbrido | El código pregunta solo por permiso; `RequireRole` se borra del paquete al terminar |
| Tableta | Pantalla nueva con maquetas; `Picker` para elegir rol; controles ≥ 44 px |

### Subida (convivencia)

| Momento | API anterior (entrega 1) | API nueva (entrega 2) |
| --- | --- | --- |
| Lee | `users.role` (JWT y refresh) | `users.role_id` → `roles` |
| Escribe al crear usuario | `role` | `role_id` (`role` toma su default) |
| Coherencia | Trigger `sync_user_role` (0108): si llega `role` sin `role_id`, o cambia `role` sin cambiar `role_id`, pone el `role_id` del sugerido equivalente; si cambia `role_id`, pone en `role` el código del sugerido, o `mesero` (lo mínimo) si es propio |
| Medición de uso | escribe el enum | escribe el código del sugerido o nulo para un propio |

**Corrección del 2026-11-07**: la API de la entrega 2 sí leía `users.role`: sqlc expande `select *`
y `returning *` de `users.sql` a columnas explícitas. Antes de la release de la entrega 2 se agregó un
cambio de solo código que lista columnas y devuelve `''` en lugar de `role`
(`TestTheAPIWorksWithoutUsersRole` borra la columna y recorre alta, edición, lista, login y
`/auth/me`). Viaja en la misma subida que la 0108, así que la API que corra cuando llegue la 0109 ya no
la nombra.

La 0109 (entrega 3) repite de forma idempotente el relleno de roles y de `role_id`, valida
`role_id is not null` (`check … not valid` y luego `validate`, después `set not null`) y borra el
trigger y `users.role`. **No toca la medición ni el tipo `user_role`**: las columnas `role` de
`usage_daily` y `usage_touches_daily` siguen siendo ese enum, que ahora solo significa «uno de los
cuatro sugeridos» (cambiarlas de tipo reescribe las tablas bajo bloqueo exclusivo y rompe los planes
cacheados de la API de la entrega 2; un quinto sugerido costaría un `alter type add value`).

### Ajustes de la revisión de arquitectura (2026-11-06)

**Trigger `sync_user_role`** (0108; lo borra la 0109):
- `BEFORE INSERT OR UPDATE OF role, role_id`, `security definer`, `set search_path = pg_catalog, public, pg_temp`, filtra por `NEW.company_id` (nunca el GUC): funciona como owner, con `gatobobah_app`, en un script y en `pg_restore`.
- Si viene `role_id`: no consulta nada para resolverlo; pone en `role` el código del sugerido, o `mesero` si es propio. Si cambian los dos a la vez, gana `role_id`.
- Si viene solo `role` (la API anterior): busca el sugerido de esa empresa y, si la empresa no tiene sus roles (la creó la API anterior durante la subida), los crea con `insert … on conflict (company_id, system_role_id) do nothing`.
- No se asume default en `role`: el trigger lo llena antes del `not null`.
- Ventana aceptada: durante el traslape, un usuario con rol propio aparece como `mesero` para la API anterior (segundos; ese rol propio no existe antes de la subida).

**Concurrencia**: toda escritura que cambie `users.role_id` o `users.is_active` (alta, edición, baja, reactivación, `-reset-admin`) y toda escritura de roles toma `pg_advisory_xact_lock(28068, empresa)` **antes de leer**. Inactivar toma el rol `for update` y cuenta empleados activos bajo el candado. Cada cambio de un rol hace `update roles set version = version + 1 … where id = $1 and version = $2` y exige una fila; los cambios de ajustes suben la versión del rol en la misma transacción.

**Guardado de permisos por diferencia**: `PATCH /roles/{id}` manda `add` y `remove` (lo que la persona tocó), no la lista completa. Así, un permiso que el producto agregó mientras la hoja estaba abierta no se convierte en «quitado».

**Resolución de permisos**: el resolvedor corre después de `WithTenant`. Login, refresh y `/auth/me` resuelven con la conexión de la empresa de la sesión (`store.AcquireTenant`), nunca sin empresa: sin el ajuste, RLS devolvería cero permisos.

**Down**:
- 0108: se niega (`raise exception`) si hay roles propios o ajustes; si no, borra trigger y función, `users.role_id`, ajustes, roles y catálogos. Su renglón en `guardedDownCleanup` reasigna a los usuarios con rol propio al sugerido `mesero` y borra ajustes y roles propios, solo en la copia de prueba.
- 0109: recrea `users.role` desde `role_id` (propio → `mesero`), lo vuelve `not null`, deja `role_id` nullable y recrea el trigger. Pierde el dato «rol propio» para la columna vieja, así que lleva su renglón en `guardedDownCleanup`.

**Base**: `check (system_role_id is distinct from 1 or active)` (el admin no se inactiva, también en la base). Llaves foráneas a `permissions` con `restrict`. `system_roles.name` guarda el nombre que se muestra (Admin, Gerente, Cajero, Mesero): una sola fuente, que también usa la revisión de choque de nombres.

**Puerta del principio VIII, dicho a propósito**: la medición de uso guarda solo los cuatro sugeridos; lo de un rol propio queda sin rol. Ese uso por rol propio no se podrá reconstruir después; es deliberado (un rol propio puede ser de una sola persona). La bitácora guarda códigos de permiso, no ids.

## Pantallas y maquetas

| Pantalla | Dónde | Cómo queda (opción A) |
| --- | --- | --- |
| **Roles** (nueva) | Empleados → pestaña «Roles» (permiso `roles.manage`) | Lista a todo el ancho con `Page fill` (`flex="1" minH={0}`): una fila por rol de 56 px con nombre, «N personas · M permisos» y, en un segundo renglón, chips «sugerido» / «propio» / «inactivo» / «ajustado +a −q». Tocar una fila abre una **hoja inferior** con el rol |
| Hoja del rol | Hoja inferior existente | Arriba: nombre (campo editable solo en propios; al crear, primer renglón con el teclado abierto) y, al crear, «Copiar permisos de…» con `Picker`. Cuerpo: permisos por área, cada área plegable con «n de m» y «Marcar todas» (44 px); en sugeridos, cada permiso cambiado lleva «agregado»/«quitado»; el admin muestra «Administrar roles» fijo con el motivo «El administrador siempre puede administrar roles». Barra fija abajo: «N cambios» y **Guardar** a la derecha, deshabilitado sin cambios. Cerrar o tocar el fondo con cambios pide confirmar descartar. Al final del cuerpo, separado: «Volver a lo sugerido» (sugeridos ajustados), «Inactivar/Reactivar» y «Borrar» (propio sin usuarios, con confirmación). Inactivar con personas activas responde con cuántas y un botón que lleva a Personal filtrado por ese rol |
| **Empleados** (cambia) | `/empleados` | El selector de rol pasa de cuatro botones fijos a un `Picker` con los roles activos de la empresa; la tabla muestra el nombre del rol |

Una sola hoja por vez: la hoja se abre en el render siguiente (gotcha de `CobrarSheet`, AGENTS.md §3).

Ajustes de la revisión de tableta (2026-11-06):
- **Quién ve qué en `/empleados`**: entra quien tiene `employees.manage` o `roles.manage`. «Personal» con `employees.manage`, «Roles» con `roles.manage`; con una sola pestaña visible no se pinta la barra de pestañas.
- **Áreas plegadas al abrir la hoja**, cada una con «n de m»; «Borrar» confirma con `ConfirmSheet`. «Cerrar» va arriba, en el encabezado; «Guardar» abajo a la derecha.
- **Crear un rol**: nombre y «Copiar permisos de…» en el mismo renglón; el teclado no se abre solo.
- **Fila del rol**: chips en el mismo renglón que «N personas · M permisos», máximo dos (tipo y estado); «ajustado» se escribe «+2 agregados · −1 quitado» dentro de la hoja.
- **`Picker` de rol en Empleados**: opciones = roles activos + el rol actual del empleado aunque esté inactivo (con «inactivo»). El alta nace sin rol elegido y exige escoger. Reactivar a un empleado con rol inactivo pide elegir uno activo (el servidor lo rechaza). Sin buscador con pocos roles (`searchThreshold` del `Picker`).
- **`Picker` (hoja) dentro del diálogo de edición de Empleados**: se prueba en Vitest con la hoja abierta sobre el diálogo; si choca, el diálogo pasa a hoja inferior.

## Maquetas

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Roles | https://claude.ai/artifact/VgJecdviTYJzbYWKZZxaEp | A · Lista y hoja | 20/25 contra 17 (B) y 10 (C), medido a 1024×600: reutiliza la hoja inferior del POS, aísla la edición de un rol y da el ancho completo a los nombres de permiso; C pone 216 interruptores con un solo Guardar. Elegida por la sesión por delegación del dueño, 2026-11-05 |
| Empleados | — | — | Cambio a un componente común (`Picker`); sin opciones que decidir |

## Project Structure

### Documentation

```text
specs/068-roles-por-empresa/
├── plan.md · research.md · data-model.md · quickstart.md
├── contracts/roles-api.md · contracts/permission-map.md
└── tasks.md
```

### Source Code

```text
server/migrations/0108_company_roles.sql (+ su test de integración)
server/migrations/0109_drop_user_role.sql (+ su test)           # entrega 3
server/queries/roles.sql                                        # nuevas
server/queries/users.sql, uso.sql, toques.sql                   # sin `role`
server/internal/domain/permission.go                            # 12 permisos nuevos, áreas, etiquetas
server/internal/domain/roles.go (+ _test)                       # reglas puras
server/internal/domain/uso.go, toque.go                         # pantalla → permiso
server/internal/app/roles.go (+ users.go, auth.go, uso.go, company)  # servicio, sesión, alta
server/internal/httpapi/router.go, middleware.go, handlers_roles.go, handlers_users.go
server/internal/integration/*                                   # equivalencia, aislamiento, migración
web/src/app/permissions.ts, roles.ts (→ ROUTE_PERMISSIONS), RequireAuth.tsx, AppShell.tsx, App.tsx
web/src/features/admin/RolesPage.tsx (nueva), EmployeesPage.tsx
web/src/features/pos/POSPage.tsx, ModifierSheet.tsx, backoffice/CashPage.tsx, admin/PlataformasPage.tsx
web/src/shared/audit/labels.ts
```

**Structure Decision**: monorepo existente; ningún paquete nuevo.

## Complexity Tracking

| Decisión | Por qué | Alternativa descartada |
| --- | --- | --- |
| Catálogo de permisos en la base **y** constantes en `domain` | El dueño pidió ids universales en la base y la herencia se resuelve en SQL; las constantes evitan strings sueltos en el router | Solo en código: no hay llave foránea para los ajustes ni catálogo que la pantalla pueda leer. La doble fuente la vigila `TestThePermissionCatalogMatchesTheDomain` |
| Trigger de convivencia `sync_user_role` | La API anterior escribe `role` durante la subida | Partir en más subidas: alarga la ventana con dos verdades; el trigger se borra en la 0109 |
