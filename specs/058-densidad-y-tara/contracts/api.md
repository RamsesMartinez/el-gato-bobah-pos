# Contratos HTTP (bajo `/api/v1/backoffice/stock`, con sesión)

## Densidad — `inventory.configure`

### `PUT /articles/{id}/density`

```json
{ "method": "weighing", "fullG": "1250", "emptyG": "150", "contentMl": "1000" }
{ "method": "label", "netG": "4300", "contentMl": "3785" }
{ "method": "manual", "density": "1.10" }
```

- 200 → `{ "density": { "value": "1.1", "method": "weighing", "setByName": "Ana", "setAt": "…" } }`
- 400 `VALIDATION`: método desconocido, campo de más o de menos, vacío ≥ lleno, contenido ≤ 0,
  densidad fuera de 0.30–3.00 (el mensaje dice el valor calculado), artículo en pieza o cm.
- 409: artículo archivado; insumo tomado por otra escritura (`vuelve a intentar`).
- 404: de otra empresa o inexistente.

### `DELETE /articles/{id}/density`

- 204. Sin densidad ya, 204 igual (idempotente, sin evento).
- 409 `DENSITY_IN_USE`: hay una compra pendiente del artículo en la otra clase de unidad que todavía
  no fijó su densidad.

## Tara — `inventory.configure`

`POST /ingredients/{id}/presentations` y `PUT /presentations/{id}` aceptan `"tareG": "120"` o
`null`. 400 si ≤ 0, pasa de 100,000 g o el artículo es pieza/cm. `GET` la devuelve
(`tareG`, nulo si no tiene). Ausente en el cuerpo de un `PUT` = sin cambio (una tableta vieja no la
borra).

## Captura de gasto (sin cambio de forma)

Un renglón masa↔volumen se acepta si el artículo tiene densidad. Sin ella, 400 `UNIT_KIND_MISMATCH`
con `details: { ingredientId, name }` para el atajo. Las vistas de renglón (`ListStockLines`,
detalle, por recibir, vista previa de edición) agregan `densityGPerMl`.

`GET /articles` (búsqueda de la captura) agrega `densityGPerMl`.

## Cambio de unidad — `inventory.configure`

`POST /articles/{id}/unit/preview` y `POST /articles/{id}/unit`:

- masa↔volumen: `factor` opcional. El servidor lo deriva de la densidad; si viene distinto del
  derivado (6 decimales), 409 `DENSITY_CHANGED`. Sin densidad, 400 «primero captura la densidad».
- La vista previa agrega `density` (nula salvo masa↔volumen) y `factor`.

## Conteo — `inventory.count`

`PUT /counts/{countId}/lines/{ingredientId}` agrega:

```json
{ "qty": "3000", "gross": { "weight": "850", "weightUnitId": 1, "presentationId": 7, "containers": 2,
  "seenTareG": "120", "seenDensity": "1.1" } }
```

- `qty` es lo contado en empaques cerrados y suelto (unidad base); lo pesado se suma.
- 400: `gross` y `weight` juntos; presentación de otro artículo, inactiva o sin tara; unidad que no
  es de masa; empaques fuera de 1–999; bruto menor que la tara (`pesa menos que N empaques vacíos`);
  base ml sin densidad.
- 409 `COUNT_INPUTS_CHANGED`: la tara o la densidad ya no son las que vio la pantalla.
- La respuesta trae `counted` en la unidad base, como hoy, y `grossNet` (lo pesado ya convertido).
- `GET` del conteo: `CountItem.densityGPerMl`, `CountItem.grossWeighable`,
  `CountPresentation.tareG`.
