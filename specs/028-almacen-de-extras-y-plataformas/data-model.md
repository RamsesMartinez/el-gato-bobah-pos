# Data Model: Almacén de extras, paquetes y plataformas (0078)

## Estado de la composición

En `products`, `modifier_options` e `ingredients`:

| Columna | Regla |
|---|---|
| composition_status | text null, check `in ('estimated','confirmed')`; nulo = sin composición capturada |
| composition_confirmed_by | bigint null → users; nulo en lo que confirmó la migración |
| composition_confirmed_at | timestamptz null; not null cuando `confirmed` |

Relleno: lo que hoy tiene receta o producto ligado (vino de FUDO) queda `estimated`. **Un producto
con `track_stock` queda `confirmed`**: su composición es él mismo, por diseño. Sin esta regla, los
productos de existencias propias, que son una parte grande del catálogo, inundarían el filtro «sin
capturar» desde el primer día. En esas filas el confirmador queda nulo: lo confirmó la migración, no
una persona.

## stock_movements (cambia)

| Columna | Regla |
|---|---|
| modifier_option_id | bigint null, FK a `modifier_options` **on delete set null**: el extra que originó el movimiento |
| component_of_product_id | bigint null, FK a `products` **on delete set null**: el paquete del que salió este componente |

`set null` y no `restrict` ni `cascade`: el libro del almacén es inmutable. Borrar un extra viejo no
debe quedar bloqueado por sus movimientos, y tampoco debe borrarlos; se pierde la etiqueta, no el
movimiento. Por eso aquí la FK es simple y no compuesta: una compuesta con `set null` pondría en
nulo también `company_id`. La empresa la cuida el trigger que llena el movimiento con lo de su
propia fila.

## order_line_components (nueva)

Copia al vender de los componentes de un renglón de paquete. `order_line_id` lleva FK compuesta
`(order_line_id, company_id)` → `order_lines (id, company_id)`, con cascade. **La 0078 agrega ese
único a `order_lines`**, que no lo tiene: `order_line_modifiers` usa FK simple. Lleva
`lock_timeout`, como 0060 y 0076. Un reporte sobre esta tabla une `order_lines` y excluye los
renglones cancelados. `product_id` (FK compuesta, restrict), `quantity numeric(14,4) > 0`,
`company_id` con RLS (`nullif`) y grants `select, insert`.

## platform_incoming_order_lines (cambia)

`modifier_option_id` bigint null, FK compuesta **on delete restrict**, como su hermano `product_id`
(0073): la opción del POS emparejada al registrar el pedido.

## Funciones (domain)

- `ExpandSale`: producto o paquete × cantidad, más extras × cantidad → `[]StockDelta{ItemType,
  IngredientID|ProductID, QtyBase, OptionID?, ComponentOf?}`.
- `ValidateComposition`: rechaza ciclos, paquete en paquete y unidades de otro tipo.
