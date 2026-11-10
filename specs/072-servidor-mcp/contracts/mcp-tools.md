# Contrato: herramientas MCP

Endpoint: `POST {API}/mcp` (producción: `https://api.elgatobobah.com/mcp`), transporte HTTP
streamable sin sesión, respuesta JSON. Header obligatorio `Authorization: Bearer egbpat_…`.

## Reglas comunes

- **Dinero**: texto decimal (`"845.50"`). **Fechas**: `AAAA-MM-DD`, día del negocio.
- **Listas**: `page` (desde 1) y `page_size` (tope por herramienta); la respuesta dice el total.
- **Escrituras**: `dry_run` (default `true`) e `idempotency_key` (obligatoria con `dry_run: false`;
  8 a 100 caracteres `[A-Za-z0-9_.:-]`). La respuesta de una vista previa es
  `{"preview": true, "before": …, "would_send": …, "notes": […]}`; la de una aplicación es
  `{"applied": true, "result": …, "before": …, "after": …}`.
- **Errores**: `isError: true` con texto `«causa». «cómo corregirlo».`; nunca rastros internos.
- **Visibilidad**: `tools/list` muestra solo las herramientas cuyo alcance tiene el token **y** cuyo
  permiso tiene hoy el rol de la persona.

## Catálogo

Permiso «—» = el mismo que la pantalla: cualquier usuario autenticado de la empresa.

| Herramienta | Área | Lee/escribe | Ruta que despacha | Permiso | Alcance |
| --- | --- | --- | --- | --- | --- |
| `whoami` | — | Lee | (local: token, alcances, permisos) | — | `read` |
| `sales_summary` | Ventas | Lee | `GET /sales/summary` | `sales.view` | `read` |
| `sales_list` | Ventas | Lee | `GET /sales` | `sales.view` | `read` |
| `order_get` | Ventas | Lee | `GET /orders/{id}` | — | `read` |
| `report_products_sold` | Reportes | Lee | `GET /reports/products-sold` | `reports.view` | `read` |
| `attendance_report` | Personal | Lee | `GET /admin/attendance/report` | `attendance.report` | `read` |
| `cash_registers_list` | Caja | Lee | `GET /cash-registers` | `cash.operate` | `read` |
| `cash_session_current` | Caja | Lee | `GET /cash-sessions/current` | `cash.operate` | `read` |
| `cash_sessions_history` | Caja | Lee | `GET /cash-sessions` | `cash.operate` | `read` |
| `cash_concepts_list` | Caja | Lee | `GET /cash-concepts` | `cash.operate` | `read` |
| `cash_movement_create` | Caja | Escribe | `POST /cash-sessions/movements` | `cash.operate` | `write:cash` |
| `expenses_list` | Gastos | Lee | `GET /expenses` | `expenses.capture` | `read` |
| `expense_get` | Gastos | Lee | `GET /expenses/{id}` | `expenses.capture` | `read` |
| `expense_categories_list` | Gastos | Lee | `GET /expense-categories` | — | `read` |
| `suppliers_list` | Gastos | Lee | `GET /suppliers` | `expenses.capture` | `read` |
| `payment_methods_list` | Gastos | Lee | `GET /payment-methods` | — | `read` |
| `expense_create` | Gastos | Escribe | `POST /expenses` | `expenses.capture` | `write:expenses` |
| `expense_pay` | Gastos | Escribe | `POST /expenses/{id}/pay` | `expenses.capture` | `write:expenses` |
| `expense_cancel` | Gastos | Escribe | `POST /expenses/{id}/cancel` | `expenses.cancel` | `write:expenses` |
| `supplier_create` | Gastos | Escribe | `POST /suppliers` | `expenses.capture` | `write:expenses` |
| `stock_levels` | Inventario | Lee | `GET /stock/levels` | `inventory.view` | `read` |
| `stock_movements` | Inventario | Lee | `GET /stock/movements` | `inventory.view` | `read` |
| `waste_reasons_list` | Inventario | Lee | `GET /waste/reasons` | `inventory.write_off` | `read` |
| `waste_record` | Inventario | Escribe | `POST /waste` | `inventory.write_off` | `write:inventory` |
| `stock_adjust` | Inventario | Escribe | `POST /stock/movements` (tipo ajuste) | `inventory.count` | `write:inventory` |
| `products_list` | Catálogo | Lee | `GET /admin/products` | `menu.configure` | `read` |
| `modifier_options_list` | Catálogo | Lee | `GET /admin/modifier-options` | `menu.configure` | `read` |
| `platform_prices_list` | Catálogo | Lee | `GET /pos/menu` (solo plataformas y precios) | — | `read` |
| `product_update` | Catálogo | Escribe | `PATCH /admin/products/{id}` | `menu.configure` | `write:catalog` |
| `modifier_option_update` | Catálogo | Escribe | `PATCH /admin/modifier-options/{id}` | `menu.configure` | `write:catalog` |
| `platform_price_set` | Catálogo | Escribe | `PUT /platform-prices/product` o `/modifier-option` | `platform_prices.capture` | `write:catalog` |
| `platform_price_delete` | Catálogo | Escribe | `DELETE /platform-prices/product` o `/modifier-option` | `platform_prices.capture` | `write:catalog` |
| `audit_list` | Bitácora | Lee | `GET /admin/audit` | `audit.view` | `read` |

## Argumentos de las escrituras

- `expense_create`: `category_id`, `amount`, `expense_date`, `description?`, `supplier_id?`,
  `doc_folio?`, `payments?: [{method_id, amount, register_id?, reference?, paid_on?,
  include_in_count?}]` (`include_in_count` es la casilla «Incluir en el arqueo» de la pantalla; por
  omisión `true`, como la pantalla). Sin pagos el
  gasto queda pendiente de pago. Vista previa: nombres de categoría, proveedor, método y caja, y si
  la caja de un pago en efectivo tiene turno abierto.
- `expense_pay`: `expense_id`, `method_id`, `amount`, `register_id?`, `reference?`, `paid_on?`.
  Antes: el gasto con lo pagado y lo que falta.
- `expense_cancel`: `expense_id`, `reason`. Antes: el gasto.
- `supplier_create`: `name`, `phone?`, `notes?`. Vista previa: proveedores con nombre parecido.
- `cash_movement_create`: `register_id`, `kind` (`entrada`|`salida`), `amount`; una salida exige
  `concept_id` (de `cash_concepts_list`, igual que la pantalla) y una entrada `concept` en texto.
  Antes: el turno abierto con su esperado; después: el turno.
- `waste_record`: `reason_id`, `ingredient_id` o `product_id`, `qty`, `unit_id?`, `note?`. Antes:
  la existencia del artículo.
- `stock_adjust`: `ingredient_id` o `product_id`, `quantity` (con signo), `reason`. Antes/después:
  existencia.
- `product_update`: `product_id`, `price?`, `active?` (al menos uno). Antes/después: el producto.
- `modifier_option_update`: `option_id`, `price_delta?`, `active?` (al menos uno).
- `platform_price_set`: `platform_id`, `product_id` o `option_id`, `price`.
- `platform_price_delete`: `platform_id`, `product_id` o `option_id`.

## Errores (estado del handler → pista)

| Estado | Texto que recibe el agente |
| --- | --- |
| 400 / 422 | El mensaje del servidor + «Revisa los argumentos y vuelve a intentar» |
| 401 | «El token no es válido, está revocado o venció. Crea otro en Integraciones» |
| 403 (permiso) | El mensaje del permiso + «Pide a quien administra los roles que se lo dé a tu rol» |
| 403 (alcance) | «Este token no tiene el alcance `write:x`. Crea uno con ese alcance en Integraciones» |
| 404 | «No existe en esta empresa. Búscalo con `<herramienta de lista>`» |
| 409 | El mensaje del servidor (p. ej. caja cerrada, gasto ya pagado) |
| 429 | «Demasiadas llamadas. Espera N segundos» |
| 5xx | «Error interno; puede que no se haya aplicado. Revisa con una lectura antes de reintentar» |
