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

Forma validada con **triggers** (`catalog_image_url_shape`), no con un check: un check, aun
`not valid`, se evalúa en cada `update` de la fila, y un producto con una URL vieja escrita a mano
quedaría sin poder cambiar su precio (lo encontró la revisión de código; lo cubre la prueba de la
migración). El trigger corre al insertar con `image_url` y al actualizar **solo si `image_url`
cambia**, y exige `^https?://[^\s?#]+-1200\.webp$` (`http` porque en local el dominio es
`http://localhost:93NN`). Una URL vieja con otra forma nunca se borra del almacenamiento (la guarda
de prefijo la descarta) y se reemplaza al subir una foto nueva.

## Tipo nuevo: `food_family`

Enum de Postgres (no requiere clasificación para el respaldo anónimo):

```
cold_drink, coffee, tea, frappe, soda, soup, snack, wings, dessert, breakfast, dish
```

Espejo en `domain.FoodFamily` con `ValidFoodFamily`, y en el front (`FOOD_FAMILIES`) con su nombre en
español y su `<symbol>` del sprite. Agregar una familia: `alter type … add value` en una migración,
la constante en `domain`, el renglón en el front y el `<symbol>`; nada que llenar después.

## `categories.food_family food_family null`

Null = genérica (`dish` al mostrar). Elegirla no depende del nombre de la categoría (FR-016).

## Tabla nueva: `media_pending_deletions`

| Columna | Tipo | Nota |
|---|---|---|
| `id` | `bigint identity` | PK |
| `company_id` | `bigint not null default current_setting(app.company_id)` | FK a `companies` on delete cascade |
| `object_key` | `text not null` | Llave relativa al bucket; check de columna `^[0-9]+/(products|categories)/[0-9]+/[0-9a-f]{32}-(400|1200)\.webp$` y check de tabla `object_key like company_id::text || '/%'` |
| `attempts` | `int not null default 1` | Check `>= 1` |
| `last_error` | `text not null` | Check longitud ≤ 300; el código lo recorta a 300 y quita la querystring de cualquier URL |
| `created_at` / `updated_at` | `timestamptz not null default now()` | |

- `unique (company_id, object_key)`: el mismo borrado fallido no se apila.
- RLS `tenant_isolation` con `nullif`, `grant select, insert, update (attempts, last_error,
  updated_at), delete` a `gatobobah_app`.
- Índice: la `unique` empieza por `company_id`, sirve para el barrido por empresa.
- Respaldo anónimo: `media_pending_deletions.object_key` y `.last_error` van a `noPersonalData` en
  `respaldo_anonimo_test.go` (llave derivada de ids y un
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

## Down

Quita, en orden: la tabla, los cuatro triggers y su función, `categories.food_family` y el tipo.
**Se niega** (`raise exception`) si `media_pending_deletions` tiene filas: bajar perdería las llaves
y esos archivos quedarían en el bucket para siempre. Lleva su renglón en `guardedDownCleanup`
(`harness_restaurado_test.go`), que vacía la tabla en la copia de prueba, y otro para
`food_family`, que el Up no vuelve a llenar.

## Duplicar un producto

`CloneProductRow` (`server/queries/admin.sql:95`) deja de copiar `image_url`: la llave del objeto
lleva el id del producto original y dos filas con la misma URL harían que quitar la foto de una
borrara la de la otra. Durante el traslape la API vieja todavía la copia; la guarda de prefijo
(la URL no trae el id del duplicado) impide que la nueva la borre, y la foto se reemplaza al subir
otra.
