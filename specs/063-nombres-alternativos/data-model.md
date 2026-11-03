# Data model: Nombres alternativos (063)

## Migración 0107 (`server/migrations/0107_alt_names.sql`)

```sql
-- +goose Up
-- Una sola sentencia por tabla: un solo ACCESS EXCLUSIVE, y con tope de espera para no encolar las
-- ventas detrás de una transacción larga (el precedente de la 0093).
set local lock_timeout = '3s';
alter table products add column alt_names text[] not null default '{}'
  constraint products_alt_names_valid check (cardinality(alt_names) <= 10
    and array_position(alt_names, null) is null and array_position(alt_names, '') is null);
alter table ingredients add column alt_names text[] not null default '{}'
  constraint ingredients_alt_names_valid check (cardinality(alt_names) <= 10
    and array_position(alt_names, null) is null and array_position(alt_names, '') is null);
-- +goose StatementBegin
create function catalog_name_key(t text) returns text
  language sql immutable parallel safe
  as $$ select btrim(regexp_replace(translate(lower(t), 'áéíóúüñ', 'aeiouun'), '[\s\u00a0]+', ' ', 'g')) $$;
-- +goose StatementEnd

-- +goose Down
drop function catalog_name_key(text);
alter table ingredients drop column alt_names;
alter table products    drop column alt_names;
```

- Si el candado no llega en 3 s, la migración falla, la API nueva no arranca y el despliegue falla
  ruidoso; la vieja sigue atendiendo. Se reintenta el despliegue.
- `btrim` va **después** de juntar espacios: con `btrim` primero, un tabulador en la orilla quedaba
  como espacio sin recortar y «Café\t» no chocaba con «Cafe». El espacio duro va explícito en la
  clase porque no se puede asumir que `\s` lo cubra en la locale de la base.
- Las expresiones viejas (`IngredientNameTaken`, `FindArticleByName`, el índice de empaques) tienen
  el defecto del tabulador en la orilla; se dejan (no son de esta feature). Las consultas nuevas y las
  que esta feature reescribe usan la función.
- Ninguna vista, índice ni default depende de la función: el Down la puede tirar.
- Al aplicar en producción se lee (solo lectura) `select datcollate, datctype from pg_database`: con
  locale `C`, `lower('Á')` no baja y la paridad con Go se rompería.

- Default constante: Postgres ≥ 11 no reescribe la tabla. El `check` se valida sobre filas que valen
  `{}` dentro del mismo candado.
- El check rechaza elementos nulos o vacíos: escanear un `text[]` con un NULL en `[]string` truena y
  tumbaría `GET /pos/menu` de toda la empresa.
- Sin índice: la búsqueda y el choque recorren el catálogo de una empresa (cientos de filas), igual
  que `IngredientNameTaken` hoy.
- El trigger de sentencia de `products` (0093) sube `catalog_versions` al guardar alternativos: el
  menú del POS se invalida sin tocar la 0093.

## Entidades

| Entidad | Campo nuevo | Reglas |
| --- | --- | --- |
| Producto (`products`) | `alt_names text[]` | 0–10; cada uno 1–60 letras, limpio (`strings.Fields`); orden de captura |
| Artículo (`ingredients`) | `alt_names text[]` | Igual |

## Reglas de dominio (`server/internal/domain/alt_names.go`)

- `MaxAltNames = 10`, `MaxAltName = 60`.
- `CatalogNameKey(s)`: minúsculas, sin `áéíóúüñ`, espacios juntados con `strings.Fields`. Gemela de
  `catalog_name_key` en SQL (prueba de paridad).
- `CleanAltNames(own string, in []string) ([]string, error)`:
  - limpia cada uno con `strings.Fields`; descarta los vacíos;
  - rechaza (400) uno de más de 60 letras, más de 10 tras descartar vacíos, dos con la misma llave
    en la lista, y uno con la llave del nombre propio;
  - conserva el orden;
  - devuelve siempre `[]string{}`, nunca nil: un nil viaja a Postgres como NULL (23502 en una columna
    `not null`) y sale en JSON como `null`. Las escrituras usan además
    `coalesce(sqlc.arg(names)::text[], '{}')`.
- `AltNameTaken{Name, OwnerID, OwnerName, OwnerArchived, OwnerAlt}`: `Unwrap() = ErrAltNameTaken`
  (envuelve `ErrConflict`) → 409 `ALT_NAME_TAKEN`. El mensaje nombra el choque:
  «“café frío” ya es otro nombre de «Latte»» / «ya es el nombre de «Frappé de café»», con
  « (inactivo)» o « (archivado)».
- `ArticleNameTaken` gana `ViaAlt string`: al crear o renombrar un artículo con un nombre que ya es
  alternativo de otro, el 409 sigue siendo `NAME_TAKEN` con `existingId` (la pantalla ya ofrece el
  existente) y el mensaje dice «ya es otro nombre de …».

## Consultas (sqlc)

| Archivo | Consulta | Qué hace |
| --- | --- | --- |
| `queries/alt_names.sql` | `LockProductNames` | `pg_advisory_xact_lock(28063, empresa)` |
| | `FirstProductNameClash(id, names[])` | Primer nombre propuesto que choca: con el `name` de cualquier producto o con un alternativo de otro; devuelve nombre, dueño, activo y si fue por alternativo. Sin `system_kind` |
| | `SetProductAltNames(id, names[])` | Escribe la lista (`system_kind is null`), `:execrows` |
| | `ProductNameClashesAlt(id, name)` | Para crear/renombrar: ¿el nombre es alternativo de otro producto? |
| | `FirstArticleNameClash`, `SetArticleAltNames`, `ArticleAltNames` | Lo mismo en `ingredients`, sin contar los orígenes de una unión (R12) |
| | `ProductAltNameCandidates`, `ArticleAltNameCandidates` | Nombre y alternativos de los demás, para el aviso de parecidos (R11) |
| `queries/ingredients_admin.sql` | `FindArticleByName`, `FindOtherArticleByName` | Miran también `alt_names`; devuelven `via_alt` |
| `queries/admin.sql` | `AdminListProducts` | Busca por nombre o alternativo con `catalog_name_key`; devuelve `alt_names` y `matched_alt` |
| `queries/ingredients_admin.sql` | `ListArticles`, `GetArticle` | Igual; la ficha trae `alt_names` |
| `queries/menu.sql` | `MenuProducts` | Trae `alt_names` |
| `queries/ingredients.sql` | `ListIngredients`, `SearchArticles` | Traen `alt_names` (los productos con existencia en `SearchArticles` también) |
| `queries/stock_counts.sql` | renglones del conteo actual | Traen `alt_names` del artículo |

## Bitácora

- Artículo: `ingredient.updated` con `before/after.altNames` en la misma transacción. No es acción
  nueva.
- Producto: sin evento (hoy no hay evento de nombre).

### `matched_alt`, una sola forma

```sql
case when sqlc.narg('q')::text is null
       or strpos(catalog_name_key(x.name), catalog_name_key(sqlc.narg('q'))) > 0 then ''
     else coalesce((select a from unnest(x.alt_names) with ordinality t(a, n)
                     where strpos(catalog_name_key(a), catalog_name_key(sqlc.narg('q'))) > 0
                     order by n limit 1), '') end::text as matched_alt
```

- `with ordinality … order by n`: el primero **en el orden capturado** (spec, casos de borde).
- El `coalesce` cubre el NULL de la subconsulta; el `::text` final es seguro y hace que sqlc lo tipe
  `string`. Se verifica con `make sqlc`.
- Todo `where` de búsqueda lleva la guarda `q is null or …`: `strpos(x, '')` vale 1 y coincidiría
  con todo.

### `updated_at`

`SetArticleAltNames` **no** toca `updated_at`: el `PATCH` de la ficha no escribe `alt_names`, así que
no hay carrera entre los dos, y moverlo haría responder `STALE` a otra tableta por un cambio que su
`PATCH` no pisa.
