# Data model — migración 0111

Solo agrega: dos tablas y un permiso. Convive con la versión anterior (no la lee nadie más; ver
«Versión anterior»). Revisado por `db-architect` el 2026-11-10; sus hallazgos están incorporados.

## `api_tokens`

| Columna | Tipo | Nota |
| --- | --- | --- |
| `id` | bigint `generated always as identity` PK | va dentro del token; se conoce después del insert |
| `company_id` | bigint not null, default del GUC, FK `companies` on delete cascade | RLS |
| `user_id` | bigint not null | FK compuesta `(company_id, user_id)` → `users`, `no action` |
| `name` | text not null | check `char_length(btrim(name)) between 1 and 60` |
| `secret_hint` | text not null | 4 primeros caracteres del secreto; check `~ '^[0-9a-f]{4}$'`. El prefijo visible se compone en Go con empresa e id |
| `secret_hash` | bytea not null | `sha256(secreto)`; check `octet_length = 32`; **sin** unique (256 bits aleatorios; la búsqueda es por id) |
| `scopes` | text[] not null | check `scopes <@ array['read','write:expenses','write:cash','write:inventory','write:catalog'] and 'read' = any(scopes) and array_position(scopes, null) is null and cardinality(scopes) between 1 and 5` |
| `created_at` | timestamptz not null default now() | |
| `expires_at` | timestamptz not null | check `expires_at > created_at and expires_at <= created_at + interval '181 days'` |
| `last_used_at` | timestamptz null | `update … where id = $1 and (last_used_at is null or last_used_at < now() - interval '1 minute')`; su error no tumba la llamada |
| `revoked_at` | timestamptz null | |
| `revoked_by` | bigint null | FK compuesta → `users`; check `(revoked_at is null) = (revoked_by is null)` |

- `unique (company_id, id)` para las FK compuestas; índices `(company_id, created_at desc)` (lista)
  y `(company_id, user_id)` (FK).
- **Revocado es terminal en la base**: trigger `before update` que lanza excepción si
  `old.revoked_at is not null and new.revoked_at is distinct from old.revoked_at`. Revocar es
  `update … where id = $1 and revoked_at is null` (idempotente, no pisa a quien revocó).
- RLS **enable** (no `force`, como todas) y política `tenant_isolation` con `nullif`, igual que 0108.
- `gatobobah_app`: `select, insert, update(last_used_at, revoked_at, revoked_by)`. Sin `delete`.
  Prueba bajo `appRoleStore`: `update(name)` responde `42501`.

## `mcp_idempotency_keys`

| Columna | Tipo | Nota |
| --- | --- | --- |
| `company_id` | bigint not null, default GUC | RLS |
| `token_id` | bigint not null | FK `(company_id, token_id)` → `api_tokens (company_id, id)` on delete cascade |
| `idem_key` | text not null | check `~ '^[A-Za-z0-9_.:-]{8,100}$'` |
| `tool` | text not null | check `char_length(tool) between 1 and 80` |
| `args_hash` | bytea not null | sha256 de herramienta + argumentos canónicos; check `octet_length = 32` |
| `status` | text not null | check `in ('pending','done','unknown')` |
| `result` | jsonb null | check `(status = 'done') = (result is not null)` y `pg_column_size(result) < 262144` |
| `created_at` | timestamptz not null default now() | |

- PK `(company_id, token_id, idem_key)`; índice `(company_id, token_id, created_at)` para la purga.
- Estados: `pending` → `done` (2xx) · `pending` → borrada (4xx) · `pending` → `unknown` (5xx o
  cortada). `unknown` bloquea esa clave hasta la purga: el agente revisa y usa otra.
- Purga: al registrar una clave nueva se borran las de más de 7 días **del mismo token**.
  `// ponytail:` un token que deja de escribir conserva sus filas; volumen mínimo; techo: una tarea
  programada si alguna vez pesa.
- RLS enable + `tenant_isolation`. `gatobobah_app`: `select, insert, update(status, result), delete`.

## Permiso

`permissions (37, 'integrations.manage', 'business')` y `system_role_permissions (1, 37)` — solo el
sugerido admin (la siembra de 0108 llegó hasta el 36). En `domain.AllPermissions()` va en la
posición 37. Llega a toda empresa por herencia.

## Down

Con guarda al estilo de 0108: si hay filas en `api_tokens` o `mcp_idempotency_keys`, o
`role_permission_overrides` sobre el permiso 37, `raise exception` (las FK a `permissions` son
`restrict` y borrar tokens vivos pierde credenciales). Luego borra `(1,37)` de
`system_role_permissions`, el 37 de `permissions` y las dos tablas. La guarda lleva su renglón en
`guardedDownCleanup` (`harness_restaurado_test.go`).

## Versión anterior (principio IV)

La API vieja no conoce el permiso 37: `effectivePermissions` filtra con `domain.KnownPermission` y
el catálogo de roles recorre `domain.AllPermissions()`. Lo prueba el test de migración: con la 0111
aplicada, listar roles y calcular los permisos de un admin con el código actual sin la constante
nueva no truena (se simula filtrando el 37 del dominio).

## Bitácora (sin columnas nuevas)

- `mcp.tool_called` — entidad `api_token`/id; `after`: `{tool, mode: read|preview|apply, status,
  args, entity_before?, entity_after?}`.
- `api_token.created`, `api_token.revoked` — entidad `api_token`/id; `after`: nombre, alcances,
  vigencia (nunca el secreto, la huella ni la pista).
- `device` de los eventos que la operación despachada ya deja: `{"agent": "mcp", "token":
  "<nombre> (#id)", "tool": "<herramienta>"}` (el nombre no cambia: el grant no deja editarlo).

## Respaldo anónimo

`truncate api_tokens, mcp_idempotency_keys` en el paso 5 de `respaldo-anonimo.sh` (precedente:
`refresh_tokens`), y sus `count(*)` en la verificación `FUGAS`. `scopes`, `tool` y `status` van a
`noPersonalData`; si `TestEveryColumnIsCoveredByTheAnonymousBackup` no reconoce el truncate como
cobertura, se le enseña como con `refresh_tokens`.

## Puertas (principio VIII)

Ninguna se cierra. Notas: un `branch_id` nullable en `api_tokens` se puede agregar después al mismo
costo si llega la segunda sucursal (las herramientas siguen sin aceptar sucursal como argumento);
`user_id not null` se relaja barato si algún día hace falta una cuenta de servicio.
