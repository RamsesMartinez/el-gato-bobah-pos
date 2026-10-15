# Contrato: GET /api/v1/pos/menu

- Respuesta 200: el documento de siempre + `ETag: "<schema>-<company>-<version>-<YYYYMMDD>"` y
  `Cache-Control: private, no-cache`.
- Petición con `If-None-Match` igual al ETag vigente → **304** sin cuerpo, con el mismo `ETag`.
- Cualquier otro valor o ausente → 200.

# Contrato: error MENU_CHANGED

Rechazos de `POST /pos/drafts`, `POST /pos/drafts/{id}/lines`, `PATCH /pos/drafts/{id}/lines/{line}`
y cualquier otro que arme un renglón: `422 {"error":{"code":"MENU_CHANGED","message":"…","details":{"productId":…}}}`
cuando el producto ya no se vende, la opción no pertenece al producto o excede el máximo por renglón.
`details` solo cuando el error es `ProductUnavailable` (como hoy).
