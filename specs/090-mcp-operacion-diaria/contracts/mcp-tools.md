# Contrato: herramientas MCP de la 090

Toda escritura lleva `dry_run` (ausente = vista previa) e `idempotency_key` (obligatoria al aplicar).
Dinero y cantidades como texto decimal. La vista previa devuelve `{preview, before, would_send,
resolved?, notes?, attention?}`; aplicar, `{applied, result, before, after}` (como la 072).

## Pedidos (`write:orders`)

| Herramienta | Argumentos | Vista previa | Notas |
| --- | --- | --- | --- |
| `order_draft_create` | `service_type` (`mostrador`, `para_llevar` o `domicilio`), `platform_id?`, `platform_order_ref?`, `customer_name?`, `delivery_fee?`, `discount_amount?` \| `discount_percent?`, `lines[]` {`product_id`, `qty`, `modifiers[]` {`option_id`, `qty?`, `portion?`}, `notes?`} (1 a `MaxDraftLines`) | Cotización del servidor: subtotal, descuento, total, `markup_pct`; por renglón y extra `unit_price`, `price_source`, `base_price`. Nota por cada `markup` | Aplicar crea la cuenta con id derivado de la clave; responde la cuenta con su `id` (UUID) |
| `order_draft_send` | `draft_id` (UUID) | La cuenta y su total | Idempotente |
| `order_draft_discard` | `draft_id` | La cuenta | Manda la versión leída en la misma llamada |
| `order_deliver` | `order_id` | El pedido | |
| `order_discount_set` | `order_id`, `amount?` \| `percent?` \| `clear: true` | El pedido con su descuento actual | Exactamente uno de los tres |
| `order_pay` | `order_id`, `method_id`, `all_remaining?` \| `amount?` (un pedido de plataforma, por monto), `tip?`, `reference?`, `terminal_id?` | `quote` del servidor, el pedido y el método | `clientUuid` derivado de la clave |
| `order_payment_void` | `order_id`, `payment_id`, `reason` | El pedido con sus pagos | `payments.void` |
| `order_payment_terminal_set` | `order_id`, `payment_id`, `terminal_id`, `from_terminal_id?`, `reason` | El pedido y las terminales | `payments.change_terminal` |
| `order_refund` | `order_id`, `reason`, `amount?`, `line_id?`, `card_folio?` | `refund-info` (cuánto, terminal, si pide folio) y el pedido | `payments.void` |
| `card_terminals_list` | — | — | Lectura (`cash.operate`) |

## Costo e inventario (`write:inventory`)

| Herramienta | Argumentos | Vista previa |
| --- | --- | --- |
| `ingredient_cost_set` | `ingredient_id`, `cost` (por unidad base, hasta 6 decimales), `reason` (1 a 200) | Ficha (costo actual, unidad, recetas que lo usan) y las últimas entradas del historial |

## Catálogo (`write:catalog`; lecturas `read`)

| Herramienta | Argumentos | Vista previa |
| --- | --- | --- |
| `recipe_get` | `product_id` \| `option_id` | — (lectura) |
| `recipe_set` | `product_id` \| `option_id`, `mode` (`items`, `package`, `product`, `nothing`), `items[]` {`ingredient_id`, `quantity`, `unit_id`}, `linked_product_id?`, `components[]` {`product_id`, `quantity`}, `based_on` (obligatorio al aplicar si ya hay receta), `also_in_same_name` (por omisión `false`), `also_based_on` (con `also_in_same_name`, obligatorio al aplicar: el de la vista previa), `actual_stock?`, `seen_product_on_hand?`, `seen_ingredient_on_hand?` | Receta actual y nueva, `based_on`, homónimos que cambiarían, y si aplica la pregunta de existencia con las cifras |
| `categories_list` | — | — |
| `modifier_groups_list` | `search?`, `status?` (`act`, `inact`) | — |
| `modifier_group_create` | `name`, `default_min?` (0), `default_max?` (1) | Grupos con nombre parecido |
| `modifier_group_update` | `group_id`, `name?`, `default_min?`, `default_max?`, `active?` | Grupo actual; con `active: false`, los productos donde deja de salir |
| `modifier_option_create` | `group_id`, `name`, `price_delta?` (0), `max_per_line?` (1) | Opciones del grupo; aviso si el grupo está inactivo |
| `modifier_option_update` | `option_id`, `name?`, `price_delta?`, `max_per_line?`, `active?` | Opción actual con su grupo; aviso si el grupo está inactivo |
| `product_groups_get` | `product_id` | — |
| `product_group_link` | `product_id`, `group_id`, `title?`, `min_select?`, `max_select?` (los dos o ninguno: sin ellos hereda), `position?` | Grupos del producto; aviso si el grupo está inactivo |
| `product_group_unlink` | `product_id`, `group_id` | Grupos del producto |
| `product_create` | `name`, `category_id`, `price`, `cost?`, `track_stock?`, `favorite?` | Productos parecidos (primero, en `attention`) y la categoría |
| `product_update` | `product_id`, `name?`, `category_id?`, `price?`, `cost?` \| `cost_from_recipe: true`, `favorite?`, `active?` | Producto actual |

## Errores con pista nueva

| Código de la API | Pista |
| --- | --- |
| Receta cambió (`STALE` de composición) | Vuelve a leer con `recipe_get` y repite con el `based_on` nuevo |
| Falta la cifra de existencia (FR-014a de la 037) | Pregunta a la persona cuántas hay de verdad y manda `actual_stock` con las dos cifras de la vista previa |
| Existencia cambió entre vista previa y aplicar | Repite la vista previa |
