# Phase 1 — API Contract: Visualizador e impresión del ticket

**Feature**: 001-ticket-preview-print · **Date**: 2026-08-27

Todo cuelga de `/api/v1` detrás de `RequireAuth`, dentro del grupo autenticado de
[router.go](../../server/internal/httpapi/router.go). Los errores salen con la forma que ya produce
[`httpapi.Error`](../../server/internal/httpapi/respond.go); aquí solo se listan los códigos.

## GET /business-settings

Ya existe. **Se amplía la respuesta**; los campos actuales no cambian de nombre ni de tipo, así que
[CheckoutSheet.tsx](../../web/src/features/pos/CheckoutSheet.tsx) sigue funcionando sin tocarse.

**Autorización**: cualquier usuario autenticado (el POS necesita el costo de envío y el ticket
necesita el encabezado).

```json
{
  "deliveryFee": "20.00",
  "businessName": "El Gato Bobah",
  "address": "Av. Siempre Viva 742",
  "phone": "55 1234 5678",
  "headerNote": "Sucursal Centro · Wi-Fi: gatobobah",
  "footerNote": "¡Gracias por su compra!",
  "autoPrintOnClose": true,
  "hasLogo": true,
  "logoUpdatedAt": "2026-08-27T18:40:00Z"
}
```

`address`, `phone`, `headerNote` y `footerNote` pueden venir vacíos: el ticket omite ese renglón.
`logoUpdatedAt` es `null` cuando no hay logo. **Nunca** incluye los bytes de la imagen.

`autoPrintOnClose` es lo que decide si el POS imprime el ticket solo al cerrar una venta. Lo lee
cualquier autenticado porque quien tiene que obedecerlo es la caja, no el panel de administración.

| Código | Cuándo |
| --- | --- |
| 200 | siempre que haya sesión |
| 401 | sin sesión válida |

## PUT /business-settings

Ya existe para el costo de envío. **Se amplía el cuerpo** con la identidad del negocio, los dos
textos del ticket y el interruptor de impresión automática. Los campos ausentes no se tocan, para
que la pantalla de envío y la de ticket puedan guardar por separado sin pisarse.

**Autorización**: `RequireRole(admin, gerente)` — ya está así en el router.

```json
{
  "deliveryFee": "20.00",
  "businessName": "El Gato Bobah",
  "address": "Av. Siempre Viva 742",
  "phone": "55 1234 5678",
  "headerNote": "Sucursal Centro · Wi-Fi: gatobobah",
  "footerNote": "¡Gracias por su compra!",
  "autoPrintOnClose": true
}
```

| Código | Cuándo |
| --- | --- |
| 200 | guardado; devuelve el mismo objeto del GET |
| 400 | `businessName` vacío o cualquier campo fuera de su largo: nombre 60, dirección 120, teléfono 30, `headerNote` y `footerNote` **400** |
| 401 / 403 | sin sesión / sin rol. El 403 deja `forbidden` en el log de seguridad |

## PUT /business-settings/ticket-logo

**Nuevo.** Sube o reemplaza el logo del ticket.

**Autorización**: `RequireRole(admin, gerente)`.

**Petición**: `multipart/form-data` con un solo campo `file`. El cuerpo va acotado con
`http.MaxBytesReader` **antes** de parsear, igual que en
[`ExtractPurchaseDoc`](../../server/internal/httpapi/handlers_purchasedoc.go).

**Respuesta 200**: el mismo objeto de `GET /business-settings`, con `hasLogo: true` y el
`logoUpdatedAt` nuevo — así el front invalida su caché sin una segunda llamada.

| Código | Cuándo |
| --- | --- |
| 200 | logo guardado |
| 400 | no vino el campo `file`; el archivo no es PNG o JPEG **según su contenido**; excede 1024 px de lado; o pasa de 256 KB |
| 401 / 403 | sin sesión / sin rol → evento `forbidden` |

El `Content-Type` declarado en el multipart **ni se mira**: lo escribe el cliente y el contenido no
miente. SVG se rechaza siempre, y WebP tampoco entra (ver D5 en [research.md](../research.md)).

## DELETE /business-settings/ticket-logo

**Nuevo.** Quita el logo subido; los tickets vuelven al default.

**Autorización**: `RequireRole(admin, gerente)`.

| Código | Cuándo |
| --- | --- |
| 200 | logo eliminado; devuelve el objeto de settings con `hasLogo: false` |
| 401 / 403 | sin sesión / sin rol → evento `forbidden` |

Borrar cuando no hay logo es idempotente: 200, no 404. El operador quiere el estado final, no una
lección sobre el estado previo.

## GET /business-settings/ticket-logo

**Nuevo.** Devuelve el binario del logo.

**Autorización**: cualquier usuario autenticado — el ticket lo necesita en cada caja, no solo en el
panel de administración.

**Respuesta 200**: los bytes, con estas cabeceras obligatorias:

| Cabecera | Valor | Por qué |
| --- | --- | --- |
| `Content-Type` | el mime guardado | el detectado al subir, no el que dijo el cliente |
| `X-Content-Type-Options` | `nosniff` | que el navegador no reinterprete el binario como HTML |
| `ETag` | derivado de `logo_updated_at` | el front revalida sin volver a bajar la imagen |
| `Cache-Control` | `private, must-revalidate` | es dato del negocio, no de un CDN |

| Código | Cuándo |
| --- | --- |
| 200 | hay logo |
| 304 | el `If-None-Match` coincide |
| 404 | no hay logo subido → el front usa el default empaquetado, **no** es un error que mostrar |
| 401 | sin sesión |

## Lo que NO cambia

- `GET /orders/{id}` ya devuelve el pedido con sus líneas y modificadores; la reimpresión lo usa tal
  cual. **Sin endpoint nuevo para el ticket**: el ticket se arma en el front a partir de datos que
  ya viajan.
- No hay endpoint de "imprimir". El servidor no imprime nada en esta fase; eso es lo que hace que
  local y desplegado se comporten igual (FR-009).
