# Contratos de la API: 092

Todas bajo `/api/v1`, con sesión. Errores con el mapeo de `httpapi.Error`.

## `GET /admin/products` (existente) — campos nuevos por renglón

```json
{ "soldAtCounter": true, "soldOnWeb": true, "webPrice": "79.00" | null,
  "allowOversell": true, "measuresStock": true, "description": "…" | "" }
```

`measuresStock`: si el producto descuenta algo del almacén (decide si se ofrece «Vender sin
existencia»).

## `POST /admin/products` y `PATCH /admin/products/{id}` (existentes) — objeto opcional `sales`

```json
{ "sales": { "counter": true, "web": false, "webPrice": "79.00" | null,
             "allowOversell": false, "description": "Con leche de la casa" } }
```

- `sales` ausente → no se toca nada de esto (compatibilidad con el front viejo, R-8).
- Dentro de `sales`, **todos los campos son obligatorios**: un objeto a medias es 400 (el defecto del
  PATCH parcial de la 090 no vuelve a entrar por aquí).
- `webPrice`: `null` = hereda; string con hasta 2 decimales y `> 0`; `0`, negativo, NaN, más de 2
  decimales o más del tope → **400 `VALIDATION`**.
- `description`: hasta 500 caracteres; más → 400.
- Permiso: `menu.configure` (el mismo del PATCH de hoy).
- Se guarda en la misma transacción que el resto del PATCH.

## `GET /pos/menu/sold-out` (nuevo)

```json
{ "productIds": [12, 340] }
```

- Sesión cualquiera que puede abrir Vender (mismo grupo de rutas que `GET /pos/menu`).
- Sucursal: `current_branch_id()`, la misma con la que vende la caja principal.
- `productIds` siempre arreglo, nunca `null` (AGENTS.md, slices nil).
- `Cache-Control: no-store`.

## `GET /admin/web-sales/summary` (nuevo)

```json
{ "onWeb": 412, "withWebPrice": 3, "total": 540 }
```

- Permiso `business.configure` (lo ve la sección «Venta en web» de Configuración).
- `total`: productos activos del catálogo (sin el genérico de plataforma).

## Venta de mostrador: rechazos nuevos

| Caso | Respuesta |
| --- | --- |
| Producto oculto en mostrador, pedido sin plataforma | 422 `MENU_CHANGED` (el mismo de un producto archivado) |
| Lo pedido de un producto con «Vender sin existencia» apagado supera la existencia | 409 `OUT_OF_STOCK`, `message`: «Se agotó Coca-Cola: queda 1» (o «no queda») |
| Pedido con plataforma | Ninguno de los dos aplica |

Aplica a `POST /orders`, a agregar renglones a un pedido abierto y a enviar una cuenta en captura.
