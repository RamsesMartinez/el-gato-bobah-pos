# Data Model: Fotos de productos y categorías

Migración **0117** (`0117_catalog_media.sql`). Solo agrega: la versión anterior de la API no lee
nada de esto y sigue funcionando durante el traslape (constitución IV).

## Columnas existentes que empiezan a escribirse

| Tabla | Columna | Valor |
|---|---|---|
| `products` | `image_url text` | URL pública de la variante de 1200 px, o `null` |
| `categories` | `image_url text` | Igual |

Forma de la URL: `<MEDIA_PUBLIC_BASE_URL>/<company_id>/<products|categories>/<id>/<hash32>-1200.webp`.
La de 400 px es la misma con `-400.webp`. Un `update` a estas columnas ya sube `catalog_versions`
(triggers de la 0093), así que el menú guardado de cada tableta se renueva solo.

Restricción nueva (sin perder datos: hoy todas valen `null`, medido en producción antes de la
migración con `select count(*) … where image_url is not null`):

- `products_image_url_shape` / `categories_image_url_shape`: `image_url is null or image_url ~
  '^https?://[^\s]+-1200\.webp$'`. Un `https?` y no solo `https` porque en local el dominio es
  `http://localhost:93NN`. Si el conteo en producción no diera 0, el check va `not valid` y el plan
  se corrige.

## Tipo nuevo: `food_family`

Enum de Postgres (no requiere clasificación para el respaldo anónimo):

```
cold_drink, coffee, tea, frappe, soda, soup, snack, wings, dessert, breakfast, dish
```

Espejo en `domain.FoodFamily` con `ValidFoodFamily`, y en el front (`FAMILIAS`) con su nombre en
español y su `<symbol>` del sprite. Agregar una familia: `alter type … add value` en una migración,
la constante en `domain`, el renglón en el front y el `<symbol>`; nada que llenar después.

## `categories.food_family food_family null`

Null = genérica (`dish` al mostrar). Elegirla no depende del nombre de la categoría (FR-016).

## Tabla nueva: `media_pending_deletions`

| Columna | Tipo | Nota |
|---|---|---|
| `id` | `bigint identity` | PK |
| `company_id` | `bigint not null default current_setting(app.company_id)` | FK a `companies` on delete cascade |
| `object_key` | `text not null` | Llave relativa al bucket; check `^[0-9]+/(products|categories)/[0-9]+/[0-9a-f]{32}-(400|1200)\.webp$` y que empiece con el `company_id` de la fila |
| `attempts` | `int not null default 1` | Check `>= 1` |
| `last_error` | `text not null` | Check longitud ≤ 300; mensaje del almacenamiento, sin secretos |
| `created_at` / `updated_at` | `timestamptz not null default now()` | |

- `unique (company_id, object_key)`: el mismo borrado fallido no se apila.
- RLS `tenant_isolation` con `nullif`, `grant select, insert, update (attempts, last_error,
  updated_at), delete` a `gatobobah_app`.
- Índice: la `unique` empieza por `company_id`, sirve para el barrido por empresa.
- Respaldo anónimo: `object_key` y `last_error` van a `noPersonalData` (llave derivada de ids y un
  hash; error del proveedor).
- `TestEveryCompanyTableIsIsolated` la cubre sola.

## Transiciones

Foto de un producto (igual para categoría):

```
sin foto --poner--> con foto(A)
con foto(A) --poner B--> con foto(B)   [borra A después de confirmar; si falla → pendiente]
con foto(A) --poner A--> con foto(A)   [misma huella: no borra nada]
con foto(A) --quitar--> sin foto       [borra A; si falla → pendiente]
```

Orden en la API: comprobar que la entidad existe (bajo RLS) → validar → subir 400 y 1200 → en una
transacción `select … for update`, guardar la URL nueva y la bitácora → confirmar → borrar las
variantes viejas. Si la transacción falla, se borran las recién subidas (salvo que sean las mismas
de antes).

## Bitácora de auditoría

`auditEntity` con acciones nuevas `product.image_set`, `product.image_removed`,
`category.image_set`, `category.image_removed`, `category.family_set` (forma `entidad.verbo` de
`domain/audit.go`); antes y después = la URL o la familia.
