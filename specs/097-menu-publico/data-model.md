# Data Model: Menú público v1

Migración **0125_public_menu.sql** (renumerada al mergear: develop ya traía 0123 y 0124 de los traspasos de la spec 094).
Todo es aditivo: la API anterior no lee nada de esto y sigue funcionando durante el traslape.

## Tablas nuevas

### `public_menus` — configuración del menú de cada empresa

| Columna | Tipo | Regla |
|---|---|---|
| `id` | `bigint generated always as identity` PK | Propio: el menú por sucursal (puerta VIII) se agrega con `branch_id` nullable sin reconstruir la FK del historial |
| `company_id` | `bigint not null unique`, FK `companies(id) on delete cascade` | Uno por empresa hoy; `unique (company_id, id)` para la FK compuesta |
| `qr_token` | `text not null unique` | 12 hex aleatorios; nunca cambia; check `~ '^[0-9a-f]{12}$'` |
| `brand_palette` | `text not null default 'brand'` | check `in ('brand','blue','teal','green','cyan','purple','pink','orange')` |
| `template_key` | `text not null default 'classic'` | check `in ('classic')`; el #330 agrega valores |
| `created_at`, `updated_at` | `timestamptz not null default now()` | |

### `public_menu_names` — nombre vigente e historial

| Columna | Tipo | Regla |
|---|---|---|
| `id` | `bigint generated always as identity` PK | |
| `company_id` | `bigint not null` | |
| `menu_id` | `bigint not null`, FK `(company_id, menu_id)` → `public_menus(company_id, id) on delete cascade` | El nombre y su menú, de la misma empresa |
| `name` | `text not null unique` | **Único global**, vigentes y retirados: un nombre nunca pasa a otra empresa. check `~ '^[a-z0-9]+(-[a-z0-9]+)*$'` y longitud 3–40 |
| `retired_at` | `timestamptz` | null = vigente |
| `created_at` | `timestamptz not null default now()` | |

- Índice único parcial `(company_id) where retired_at is null`: un solo vigente por empresa.
- El `unique (name)` ya sirve para buscar por nombre. Índice `(company_id)` para la lista del historial
  y la FK (empieza por `company_id`, como pide RLS).
- `unique (company_id, id)` no hace falta: nada apunta a estas filas.

### RLS y permisos

Las dos tablas: `enable row level security` (sin `force`, como 0117: el relleno y `CreateCompany` corren como owner sin `app.company_id`), política `tenant_isolation` con
`nullif(current_setting('app.company_id', true), '')::bigint` en `using` y `with check`.

| Rol | `public_menus` | `public_menu_names` |
|---|---|---|
| `gatobobah_app` | select, insert, update | select, insert, update |
| `gatobobah_public_menu` (nologin, nobypassrls) | select (política `using (true)` solo para él) | select (ídem) |
| `gatobobah_platform` | — | — |

`gatobobah_public_menu` también recibe `grant select (id, is_active) on companies` —por columna: ni
`slug` ni `name`— con política `using (true)` para él. El rol se crea con el patrón idempotente de
0073 y el Down **no** lo borra (es de clúster): lo deja `nologin`.

## Vista `public_menu_routes` (dueño `gatobobah_public_menu`)

```text
name          -- cualquier nombre, vigente o retirado
company_id
is_current    -- retired_at is null
current_name  -- el vigente de esa empresa
qr_token
```

Solo filas de empresas activas. `with (security_barrier = true)`, `revoke all from public`, `grant
select` a `gatobobah_app`. Expone `company_id` junto al nombre a cualquier consulta del rol app: es
aceptable porque nombres y códigos QR son públicos por diseño, y se dice en el comentario. No proyecta `slug`, ni nombre del
negocio, ni nada más: un `select *` sin filtro revela, como mucho, qué nombres de menú y códigos QR
existen —que son públicos por diseño—.

## Funciones SQL

- `public_menu_slugify(text) returns text` (`immutable`, no depende de la collation), en este orden:
  `translate` de una **sola tabla de acentos** (`áàâäãéèêëíìîïóòôöõúùûüñç` y sus mayúsculas →
  vocal, `n`, `c`) ANTES de `lower()` → `lower()` → todo lo que no sea `[a-z0-9]` a guión → colapsar
  guiones → recortar guiones de las orillas → truncar a 40 → recortar guiones otra vez. Espejo
  exacto de `domain.SlugifyMenuName` (misma tabla de acentos); una prueba de integración compara
  los dos sobre la misma tabla de casos, incluidos 40 y 41 caracteres y un guion justo en el corte.
- `public_menu_free_name(base text, company_id bigint) returns text` (`volatile`: lee tablas): base
  vacía → `menu-<company_id>`; base de 1–2 caracteres → `menu-<base>`; si la base está tomada o
  reservada, `<base truncada a 40 - len(sufijo), sin guion final>-2`, `-3`… Nunca devuelve algo que
  viole el check. La usan el relleno y `CreateCompany`.
- Lista de reservados: tabla de valores dentro de la función y espejo `domain.ReservedMenuNames`
  (misma prueba que el slugify).

## Relleno (en la misma migración)

Un bloque `do $$ … for r in select … from companies c left join business_settings b … order by
c.id loop … end loop`: por empresa, una fila en `public_menus` (token
`substr(replace(gen_random_uuid()::text, '-', ''), 1, 12)`, sin pgcrypto) y su nombre vigente
`public_menu_free_name(public_menu_slugify(coalesce(c.name, b.business_name)), c.id)` (el de la empresa primero: la empresa de pruebas de producción imprime «El Gato Bobah» en sus tickets y le quitaba el nombre al negocio real). El loop ve
lo que insertó la vuelta anterior: la empresa más vieja gana un nombre empatado.

En los datos restaurados: «El Gato Bobah» → `el-gato-bobah`; «Bobah Pruebas» → `bobah-pruebas`
(coincide con su slug por casualidad; lo puede cambiar).

## Empresa nueva

- `CreateCompany` (consola, owner) agrega en su misma consulta la fila de `public_menus` y el nombre.
- Una empresa creada por la versión anterior durante el traslape no tiene menú: `GET
  /admin/public-menu` lo crea si falta (`EnsurePublicMenu`, idempotente con `on conflict do nothing`),
  y el menú público responde 404 mientras tanto.

## Consultas (sqlc)

| Consulta | Archivo | Para |
|---|---|---|
| `WebOfferProducts` (ampliada) | `web_sales.sql` | Suma nombre, descripción, `image_url`, `category_id` y `sort_key`, y filtra categoría activa. **Sigue siendo la única regla** de qué se vende en web y a qué precio |
| `PublicMenuCategories` | `public_menu.sql` | Categorías activas: id, nombre, `food_family`, `image_url`, `sort_key` |
| `ResolvePublicMenuName`, `ResolvePublicMenuToken` | `public_menu.sql` | Sobre la vista |
| `LockPublicMenu` (`for update`), `GetPublicMenu`, `EnsurePublicMenu`, `PublicMenuNames`, `CountRetiredMenuNames` | `public_menu.sql` | Bajo RLS |
| `RetireCurrentMenuName`, `ReviveMenuName`, `InsertMenuName`, `SetMenuPalette` | `public_menu.sql` | Cambio de nombre en una transacción |
| `BusinessNameForMenu` | `public_menu.sql` | `business_settings.business_name` o `companies.name` |

`WebSalesCounts` cambia igual (categoría activa) para que el resumen y el menú sigan midiendo sobre
la misma base.

## Transición de nombre (dominio puro: `domain.PlanMenuRename`)

```text
entrada: nombre pedido (ya normalizado), nombres de la empresa (vigente + retirados), ¿existe en otra empresa?
- igual al vigente                       → sin cambio
- forma inválida                          → ErrValidation
- reservado o de otra empresa             → ErrMenuNameUnavailable
- retirado de esta empresa                → retirar vigente, revivir ese
- nuevo, con 20 retirados ya              → ErrMenuNameLimit (no se acaparan nombres globales)
- nuevo                                   → retirar vigente, insertar nuevo
```

- **La transacción toma primero `select … from public_menus where company_id = $1 for update`**:
  dos PATCH a la vez de la misma empresa (dos pestañas, doble toque) se serializan, y se leen los
  nombres y se planea DESPUÉS del candado.
- Revivir es `update … set retired_at = null where company_id = $1 and name = $2 and retired_at is
  not null` y exige 1 fila.
- El 23505 se distingue por `constraint_name`: el `unique` del nombre (otra empresa ganó la carrera)
  → `ErrMenuNameUnavailable`; cualquier otro se devuelve tal cual (no se disfraza de «no
  disponible»).

## Respaldo anónimo

`public_menu_names.name`, `public_menus.qr_token`, `brand_palette`, `template_key` van en
`noPersonalData` (son públicos por diseño o enumeraciones).

## Down

Orden: vista → funciones → políticas (`companies`, `public_menus`, `public_menu_names`) → revocar
grants → tablas. El rol **no** se borra (es de clúster) y queda `nologin`. Pierde los nombres
elegidos y los códigos QR: al volver a subir, el Up genera códigos nuevos y un QR impreso deja de
funcionar. No se niega (no hay a dónde guardarlos); se anota en el Down y la 0125 entra a
`guardedDownWithoutCleanup` con esa razón si la prueba de Downs lo pide.

## Concurrencia conocida y aceptada

Dos altas de empresa simultáneas desde la consola con el mismo nombre de negocio: la segunda falla
con 23505 en `CreateCompany` y se reintenta. Solo pasa en la consola y es raro.

## Puertas (constitución VIII)

¿Se puede agregar después al mismo costo?

- Subdominio por negocio (#331): sí; el nombre ya es único global y vale como subdominio.
- Plantillas (#330): sí; `template_key` es texto con check ampliable.
- Activar por paquete (#333): sí; tabla nueva que se llena con «todas tienen `public_menu`».
- Menú por sucursal: sí; columna `branch_id` nullable en `public_menus` sin tocar lo guardado.
- **Lo que no se podría recuperar después** y por eso se guarda hoy: el historial de nombres (sin él,
  un QR o enlace viejo no tendría a dónde ir). Ninguna puerta de la tabla se cierra.
