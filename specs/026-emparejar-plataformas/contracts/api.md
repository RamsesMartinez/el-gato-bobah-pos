# Contratos de API (admin y gerente, bajo `/api/v1/admin/platform-menus`)

## GET /connections/{id}/pairing

Reemplaza a la versión con `?kind=`: trae platillos **y** opciones en una respuesta.

```json
{
  "read": { "id": 12, "finishedAt": "2026-10-03T16:42:00Z" },
  "counts": { "unpaired": 33, "toReview": 9, "done": 23, "excluded": 0 },
  "items": [
    {
      "externalId": "chai-latte", "kind": "platillo", "name": "Chai Latte",
      "price": "85.05", "available": true,
      "group": "done",
      "link": { "localKind": "producto", "localId": 41, "localName": "Chai Latte",
                "isCapturePrice": true, "confirmedBy": "UX Admin", "confirmedAt": "…" },
      "proposal": null,
      "excluded": false
    }
  ],
  "lastSync": { "changed": 3, "changes": [ { "name": "Capuccino", "old": "80.00", "new": "85.05" } ] }
}
```

Precios como texto decimal con 2 cifras. Arreglos siempre presentes, aunque vacíos.

## GET /connections/{id}/candidates?externalId=…&q=…

Productos u opciones del POS según el `kind` del renglón, ordenados por parecido. Cada uno trae
`linkedCount` (cuántos platillos de esta tienda ya lo usan). Máximo 20.

## PUT /connections/{id}/links/{externalId}

```json
{ "localKind": "producto", "localId": 41, "replace": false, "capturePrice": true }
```

- 409 `LINK_EXISTS` si ya hay pareja confirmada y `replace` es falso.
- 422 `CAPTURE_PRICE_REQUIRED` si el producto ya tiene otra pareja en la tienda y no se dice si esta
  da el precio de captura.
- 422 `LINK_KIND_MISMATCH` si el destino no corresponde al tipo del renglón.

## POST /connections/{id}/links/batch

```json
{ "confirm": ["chai-latte", "capuccino"] }
```

Confirma las propuestas vigentes de esos renglones en una transacción. Responde las que quedaron y
las que ya no tenían propuesta (por ejemplo, otra persona las cambió), sin fallar el lote.

## DELETE /connections/{id}/links/{externalId}

Sin cambios. Si la pareja borrada daba el precio de captura y el producto tiene otras en la tienda,
la más reciente pasa a darlo.

## PUT / DELETE /connections/{id}/exclusions/{externalId}

«Solo existe en la plataforma». `PUT` sin cuerpo; `DELETE` la revierte.

## PUT / DELETE /connections/{id}/local-exclusions

```json
{ "localKind": "producto", "localId": 41 }
```

## PUT /platform-prices/product (existente)

409 `PLATFORM_PRICE_MANAGED` si la fila tiene `source = platform`. Igual para las opciones.
