# Contratos HTTP (084)

Todas bajo `/api/v1`, con `RequireAuth` + `RequirePermission`. Errores por `httpapi.Error`
(sentinels de `domain`). Cantidades en unidad base, como texto decimal (patrón de existencias).
Cada ruta nueva entra en `testdata/route_gates.golden` con quién pasa.

## «Se acabó» y «No lo encuentro» — `inventory.report_out`

### `GET /stock/out-reports/search?q=`
Artículos activos (insumos y productos con existencia propia) cuyo nombre u otro nombre empieza con
alguna palabra de `q` sin acentos ni plural. Máx. 20. `q` vacío → 400.
`200 {"items":[{"itemType":"ingrediente","id":12,"name":"Queso crema","unit":"g","openOut":false}]}`

### `GET /stock/out-reports/suggestions`
`200 {"frequent":[…hasta 8, más reportados en 30 días], "recent":[…hasta 8, reportados hoy]}`

### `POST /stock/out-reports`
`{"itemType":"ingrediente","id":12,"clientKey":"<uuid>"}` — **no acepta cantidad** (un
campo `quantity` es 400). Respuesta `201` (o `200` si `clientKey` ya existía, con el mismo cuerpo):
`{"id":7,"kind":"report|confirmation","notificationId":31,"delta":"-3","undoUntil":"…","confirmations":2}`
Errores: 404 artículo inexistente o archivado; 409 `ITEM_MERGED`; 429 `REPORT_LIMIT` (20 por hora y
persona, con el mensaje para pantalla).

### `POST /stock/out-reports/{id}/undo`
Solo quien reportó (o `inventory.configure`), hasta 90 s después (reloj de la base), sin movimiento
ni conteo posterior del artículo en la sucursal. `200 {"undone":true}`; 409 `UNDO_EXPIRED`,
`UNDO_AFTER_MOVEMENT`; 404.

### `POST /stock/not-found`
`{"text":"servitoallas","clientKey":"<uuid>"}` (1..80 caracteres tras recortar). `201 {"id":8,"notificationId":32}`.
No toca existencias. Mismo tope de 20 por hora.

## Notificaciones — `inventory.report_out`

### `GET /notifications?status=open|closed&limit=50`
Las de la empresa; las `not_found` solo si la persona tiene `inventory.configure`. `status`
desconocido → 400. Cada una: `id, kind, status, itemName, queryText, branchName, onHandSeen,
minSeen, needsCount, openedAt, openedByName, resolvedAt, resolvedByName, resolvedReason, read,
confirmations, banner` (`banner` = el artículo tiene franja y la persona no la ha visto).
Arreglo vacío como `[]`, nunca `null`.

### `GET /notifications/unread-count` → `200 {"unread":3}`

### `POST /notifications/read` — `{"ids":[31,32]}` (máx. 100) → `204`. Por persona.

### `POST /notifications/{id}/banner-seen` → `204`.

### `POST /notifications/{id}/resolve` — `{"action":"arrived"|"dismiss"|"created"|"alias", "ingredientId":12}`
- `arrived`: solo `stock_out`, permiso `inventory.receive`.
- `dismiss`, `created`, `alias`: permiso `inventory.configure`; `created` y `alias` solo `not_found`.
  `alias` agrega el texto a los otros nombres del insumo por el mismo servicio de `PUT …/alt-names`
  (su candado y su regla de choque) en la misma transacción.
- `409 NOTIFICATION_CLOSED` si ya no está abierta.

## Configuración de alertas — `inventory.configure`

### `GET`/`PUT /stock/articles/{id}/low-stock` y `/stock/products/{id}/low-stock`
`{"enabled":true,"minStock":"5","onMin":true,"onOut":true,"banner":false,"coverageDays":14,"branchMinStock":null,"expectedUpdatedAt":"…"}`
Con `?preview=1` no guarda y responde el efecto:
`{"effect":{"opens":["stock_low"],"closes":[],"onHand":"2","min":"5","needsCount":false},"summary":"…"}`.
Sin `preview` guarda, marca `asked`, evalúa al instante, audita y responde el mismo `effect`.
`409 STALE` si `expectedUpdatedAt` no coincide. Un producto sin existencia propia → 400.

### `POST /stock/articles/{id}/low-stock/asked` → `204` (respondió «No» a la pregunta).

### `POST /stock/low-stock/batch` (`?preview=1`)
`{"items":[{"itemType":"ingrediente","id":12,"enabled":true,"minStock":"5"}]}` (máx. 100). Una
transacción. La herramienta MCP `low_stock_update` (alcance `write:inventory`) la despacha con
`dry_run` (= `preview`) e `idempotency_key`.

### `GET /settings/notification-recipients` / `PUT`
`{"morningAt":"08:00","people":[{"userId":3,"name":"…","role":"Gerente","email":"…","hasEmail":true,"stock":true,"notFound":true}],"extras":[{"email":"compras@…","stock":true,"notFound":false}]}`
`PUT` manda la lista completa; correos normalizados, máx. 10 extra; personas sin el permiso → 400.
`morningAt` en pasos de 15 min.

## Lista de compra — `inventory.view`

### `GET /stock/shopping-list`
`{"items":[{"itemType":"ingrediente","id":12,"name":"…","unit":"g","onHand":"1000","min":"2000","dailyUse":"2000","measured":true,"coverageDays":14,"supplier":"…","suggested":{"packs":3,"packName":"Caja 10 kg","qty":"30000"},"why":"se usan ~2 kg al día, quedan 1 kg, objetivo 14 días","needsCount":false}]}`
Sin consumo medido: `suggested: null`, `why: "sin consumo medido"`.

## Tiempo real

SSE `GET /events`: evento `notifications.changed` (sin datos) cuando cambia algún aviso de la empresa.
La pantalla vuelve a pedir el contador y la lista.
