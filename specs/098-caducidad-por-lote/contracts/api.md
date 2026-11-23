# API: Caducidad por lote (098)

Todas bajo `/api/v1`, con sesión. Fechas `YYYY-MM-DD`. Un `warehouseId` vacío o ajeno: 400 / 404,
como en la 094.

## GET /stock/expiries?warehouseId=

Permiso `inventory.view`. Sin `warehouseId`: todos los almacenes de la sucursal.

```json
{
  "today": "2026-11-23",
  "counts": { "expired": 1, "within7": 2, "within30": 4, "within60": 6 },
  "lots": [
    {
      "ingredientId": 12, "name": "Perlas de tapioca", "unitCode": "g",
      "warehouseId": 3, "warehouseName": "Bodega",
      "expiresOn": "2026-12-10", "qty": 3000, "daysLeft": 17,
      "dailyUse": 100, "finishesInDays": 30, "leftoverAtExpiry": 1200,
      "noRecentUse": false, "expired": false, "pendingWaste": 0
    }
  ]
}
```

- `lots`: todos los de `qty > 0`, del que caduca antes al que caduca después (la pantalla agrupa; lo de
  más de 60 días va cerrado en «Más adelante»).
- `pendingWaste`: lo que mermas por aprobar ya piden de ese lote.
- `dailyUse` nulo y `noRecentUse: true` sin ventas en 30 días. `leftoverAtExpiry` nulo si se termina a
  tiempo.

## GET /stock/expiries/summary?warehouseId=

Permiso `inventory.view`. `{ "expired": 1, "within7": 2 }` para el aviso de Existencias.

## Compras

`expiresOn` (opcional, nulo = sin fecha) en cada renglón de los cuerpos que ya reciben renglones
(alta y edición del gasto, recibir de «Por recibir»). Fuera de 2000-01-01..hoy+10 años: 400. Solo
renglones de artículo; en un producto: 400.

## Conteo

`PUT` de captura de un artículo: `lots` opcional, `[{ "expiresOn": "2026-12-10", "qty": 300 }]`.
Ausente = no cambia el desglose; `[]` = lo borra. La lectura del conteo trae además `knownDates` por
artículo (las fechas de sus lotes en el almacén, sin cantidad). Validación de R6 → 400 `LOTS_EXCEED_COUNTED` si la
suma pasa de lo contado. La lectura del conteo devuelve `lots` por artículo.

## Merma

`POST` de merma: `lotExpiresOn` opcional, solo para un artículo (en producto: 400). Permiso el de
siempre (`inventory.write_off`).

## Notificaciones

Kind nuevo `expired_left` en la lista de la campana, con `itemName`, `unitCode` y `warehouseId`.
