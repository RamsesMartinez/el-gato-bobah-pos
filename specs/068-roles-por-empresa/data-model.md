# Data model — 068, entregas 2 y 3

## Universal (sin `company_id`, mismo id en todos los ambientes)

### `permissions`
| Columna | Tipo | Regla |
| --- | --- | --- |
| `id` | smallint PK | Asignado a mano en la migración; nunca se reusa |
| `code` | text unique not null | `^[a-z_]+\.[a-z_]+$`; nombre estable (`orders.delivered_view`) |
| `area` | text not null | Agrupa en pantalla: `orders`, `cash`, `sales`, `reports`, `expenses`, `inventory`, `menu`, `platforms`, `business`, `attendance`, `team` |

Grant `select` a `gatobobah_app`. Sin RLS (no es de una empresa).

### `system_roles`
| `id` smallint PK | `code` text unique (`admin`, `gerente`, `cajero`, `mesero`) | `name` text unique (Admin, Gerente, Cajero, Mesero) |
1 admin · 2 gerente · 3 cajero · 4 mesero. Grant `select`.

### `system_role_permissions`
`(system_role_id → system_roles, permission_id → permissions on delete restrict)` PK compuesta. Lo que el producto da.
Grant `select`. Un permiso nuevo del producto = fila nueva aquí (herencia).

## Por empresa (RLS `tenant_isolation` con `nullif`, grant a `gatobobah_app`)

### `roles`
| Columna | Tipo | Regla |
| --- | --- | --- |
| `id` | bigint identity PK | |
| `company_id` | bigint not null default GUC, → `companies` on delete cascade | |
| `system_role_id` | smallint null → `system_roles` | Nulo = rol propio |
| `name` | text null | Solo propios: 1–40 caracteres; `check ((system_role_id is null) = (name is not null))` |
| `active` | boolean not null default true | |
| `version` | integer not null default 1 | Sube en cada cambio; la pantalla lo manda (FR-010) |
| `created_at`, `updated_at` | timestamptz | |

- `unique (company_id, id)` — destino de las llaves compuestas.
- `unique (company_id, system_role_id)` — un sugerido por empresa.
- `check (system_role_id is distinct from 1 or active)` — el admin no se inactiva.
- Índice único `(company_id, catalog_name_key(name)) where name is not null`. El choque con los nombres
  de los sugeridos («Admin», «Gerente», «Cajero», «Mesero») lo revisa la app bajo el candado 28068.

### `role_permission_overrides`
| `company_id`, `role_id` | FK compuesta → `roles (company_id, id)` on delete cascade |
| `permission_id` | smallint → `permissions` on delete restrict |
| `granted` | boolean not null — true agrega, false quita |
PK `(company_id, role_id, permission_id)`. En un rol propio solo hay `granted = true` (lo cuida la app;
un `false` en un propio no cambia nada).

### `users.role_id`
bigint null, FK `(company_id, role_id) → roles (company_id, id)` on delete restrict. Índice
`(company_id, role_id)` para contar empleados por rol. Nulo solo durante la subida; la 0109 lo vuelve
`not null`.

## Permisos efectivos de un usuario

```
si el usuario está activo y su rol está activo:
  sugerido: system_role_permissions(rol) − overrides(false) ∪ overrides(true)
  propio:   overrides(true)
si no: ninguno
```

## Permisos nuevos y su siembra (reproduce el router de hoy)

| Permiso | Área | admin | gerente | cajero | mesero | Reemplaza |
| --- | --- | --- | --- | --- | --- | --- |
| `cash.operate` | cash | ✓ | ✓ | ✓ | | `/cash-sessions`, `GET /cash-registers`, `GET`/`POST /cash-concepts`, `GET /card-terminals`, `GET /cash/denominations`; `/caja`, «abrir caja» del POS |
| `orders.platform_ref` | orders | ✓ | ✓ | ✓ | | `PATCH /orders/{id}/platform-ref` |
| `platform_prices.capture` | platforms | ✓ | ✓ | ✓ | | `PUT`/`DELETE /platform-prices/*` |
| `sales.view` | sales | ✓ | ✓ | | | `/sales`, `/sales/summary`, `/cash-alerts`, `GET /orders/{id}/settlement`, `/platform-settlements/summary`; `/ventas` |
| `platform_settlements.capture` | sales | ✓ | ✓ | | | `PUT /orders/{id}/settlement` |
| `reports.view` | reports | ✓ | ✓ | | | `/reports/*`; `/reportes` |
| `expenses.capture` | expenses | ✓ | ✓ | | | `/expenses` (lista, alta, detalle, pagar, leer documento), categorías (alta/edición), proveedores, unidades, insumos, sugerencias, `supplier-items`; `/gastos` |
| `platforms.configure` | platforms | ✓ | ✓ | | | `/admin/platform-menus/*`; `/plataformas` y emparejar |
| `platforms.credentials` | platforms | ✓ | | | | credenciales y llaves de pedidos de plataforma |
| `business.configure` | business | ✓ | ✓ | | | `PATCH /company`, métodos de pago (todos/editar), ajustes del negocio y logo, correos del resumen, modo de conteo por sucursal; `/negocio`, `/impresion` |
| `employees.manage` | team | ✓ | | | | `/users/*`; `/empleados` |
| `roles.manage` | team | ✓ | | | | `/roles/*`; pestaña Roles |

Permisos que ya existían y absorben rutas con `RequireRole(admin, gerente)` (mismo conjunto de roles):

| Permiso | Absorbe |
| --- | --- |
| `menu.configure` | `/admin/categories`, `/admin/recipes(+confirm)`, `/admin/products/*`, `/admin/modifier-options/*`, `/admin/groups/*`, `POST /admin/reload`, `GET /products/{id}/costing`; `/catalogo`, edición en el POS y en la hoja de extras |
| `cash.configure` | `PATCH`/merge de conceptos de caja, alta/edición de terminales, `/cash-registers/all`, alta/edición de cajas; pestaña «Administrar» de Caja |
| `cash.adjust_count` | `POST /cash-movements/{id}/correct` |
| `inventory.view` | `GET /stock/levels`, `GET /stock/movements`, `GET …/composition`; `/almacen` |
| `inventory.count` | `POST /stock/movements` (ajuste manual: reemplaza lo que el sistema creía tener) |
| `inventory.configure` | `PUT`/confirm de la composición de un insumo preparado |

## Medición de uso

`rolesPorPantalla` → `permisoPorPantalla`: pantalla → permiso que la abre (o ninguno). La dimensión
`role` de `usage_daily`/`usage_touches_daily` guarda el código del sugerido o nulo, y sigue siendo el
enum `user_role` (la 0109 no la toca).

## Bitácora

Acciones nuevas: `role.created`, `role.renamed`, `role.permissions_changed`, `role.reset`,
`role.deactivated`, `role.reactivated`, `role.deleted`, `user.role_changed`. Detalle: permisos
agregados y quitados (códigos), nombre anterior y nuevo, rol anterior y nuevo.

## Respaldo anónimo

`permissions.code`, `permissions.area`, `system_roles.code`, `system_roles.name` → `noPersonalData`
(catálogo del producto). `roles.name` lo borra el paso 5 de `scripts/respaldo-anonimo.sh`: un rol
propio puede llevar el nombre de una persona.
