# Data Model

## catalog_versions (nueva)

| Columna | Tipo | Nota |
| --- | --- | --- |
| company_id | bigint PK, FK companies on delete cascade | una fila por empresa, nace con el primer cambio |
| version | bigint not null | sube 1 por sentencia que toca el catálogo de esa empresa |
| changed_at | timestamptz not null default now() | último cambio, para diagnóstico |

- RLS encendido, política `tenant_isolation` con `nullif(current_setting('app.company_id', true), '')`.
- `grant select` a `gatobobah_app`. Sin insert/update: solo la función del trigger escribe.
- Ausencia de fila = versión 0.

## Función y triggers

`bump_catalog_version()` (`security definer`, `search_path` fijo) y por cada una de las diez tablas
de [plan.md](plan.md): `<tabla>_catalog_version_ins|upd|del`, `after … for each statement`.
