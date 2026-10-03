# Data Model: Emparejar (migración 0077)

## platform_item_links (cambia)

| Columna | Cambio | Regla |
|---|---|---|
| product_id | pasa a nulable | `check ((local_kind = 'producto') = (product_id is not null))` |
| modifier_option_id | nueva, bigint null | FK compuesta `(company_id, modifier_option_id)` → `modifier_options`; `check ((local_kind = 'opcion_de_modificador') = (modifier_option_id is not null))` |
| is_capture_price | nueva, boolean not null default false | únicos parciales `(connection_id, product_id) where is_capture_price` y `(connection_id, modifier_option_id) where is_capture_price`: varias opciones de Uber pueden ir a la misma opción del POS, y la regla es la misma que con productos |

Relleno: las parejas de producto existentes que son la única de su producto en su tienda quedan con
`is_capture_price = true`; con varias, la más reciente.

**Parejas de opción guardadas como producto** (revisión de diseño): `GuardarPareja` escribe siempre
`product_id`, así que una pareja de una opción de Uber pudo quedar apuntando a un producto cuyo id
coincide por azar con el de la opción. Contar `local_kind = 'opcion_de_modificador'` no lo detecta.
La migración **aborta** si existe cualquier pareja cuyo platillo de la tienda sea de tipo `opcion`
(columna `kind` de la pareja), con el conteo y los ids; no se adivina qué se quiso ligar. Se mide
sobre el respaldo de producción antes de aplicar, y si hay alguna se decide con el dueño.

## platform_item_exclusions (nueva)

«Solo existe en la plataforma». PK `(connection_id, external_id)`; `kind`; `decided_by`, `decided_at`;
`company_id` con RLS (`nullif`), FK compuesta a `platform_connections` **on delete cascade**: la
decisión es de esa tienda. Grants select, insert, delete.

## local_item_exclusions (nueva)

«No se vende en la plataforma». `connection_id` (FK compuesta, on delete cascade); uno de
`product_id` / `modifier_option_id` (check), con FK compuesta **on delete restrict**, igual que las
parejas (0071): el reorg de datos borra productos, y una decisión manual no debe desaparecer en
silencio. `decided_by`, `decided_at`; únicos por (tienda, producto) y (tienda, opción). RLS.

**Borrar una tienda** se lleva sus parejas y sus decisiones: el aviso previo (`ParejasQueSePierden`)
cuenta las tres cosas, no solo las parejas.

## platform_price_changes (nueva)

Lo que cambió una lectura. `read_id` (FK a `platform_menu_reads`, on delete cascade: se poda con la
lectura); `external_id`; uno de `product_id` / `modifier_option_id`; `old_price` (nulo si no había),
`new_price`; `company_id` con RLS. Solo se insertan cambios reales.

## product_platform_prices y modifier_option_platform_prices (cambian)

| Columna | Regla |
|---|---|
| source | `text not null default 'manual'`, check `in ('manual','platform')` |
| synced_at | timestamptz null; not null cuando `source = 'platform'` (check) |

`updated_by` sigue not null: en la sincronización es quien disparó la lectura.

## Estados de un renglón de la tienda (calculado en domain, no guardado)

| Grupo | Condición |
|---|---|
| Listos | tiene pareja confirmada |
| Por revisar | sin pareja confirmada y con propuesta (nombre normalizado igual a uno solo del POS) |
| Sin pareja | lo demás, salvo que tenga decisión «solo existe en Uber» |
| (fuera de los grupos) | decisión «solo existe en Uber»: se ve en un filtro propio y se revierte |

## Producto genérico de plataforma (historia 6)

- `products.system_kind text null`, check `in ('platform_unpaired')`, único parcial
  `(company_id, system_kind) where system_kind is not null`. Así el código lo encuentra sin depender
  del nombre, que quien opera puede cambiar.
- Uno por empresa: nombre «Platillo de plataforma sin pareja», `needs_prep = true` (sale en la
  comanda), `is_active = false` (no aparece al vender), sin receta ni `track_stock` (no descuenta),
  en una categoría «Plataformas» que se crea si no existe, precio 0.01 si el `check` de precio lo
  exige (el renglón lleva el precio de Uber, no el del producto).
- La 0077 lo crea para cada empresa existente; `CreateCompany` lo crea al dar de alta una empresa.
- Al aceptar, el renglón sin pareja usa ese producto; `product_name` lleva el nombre de Uber y
  `notes` las opciones con su precio.
