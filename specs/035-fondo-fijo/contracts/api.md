# Contratos HTTP

## PUT /api/v1/cash-registers/{id}/fixed-float

`RequirePermission(PermCashConfigure)`.

```json
{ "fixedFloat": 1000.00 }   // null = quitar el fondo fijo
```

- 200 → la caja con `fixedFloat`.
- 400 si negativo, NaN, sobre `MaxMoney` o malformado. 404 si la caja no es de la empresa.

## GET /api/v1/cash-registers/all (existe, configuración)

Cada caja agrega `fixedFloat: number | null`. Lo lee Ajustes › Caja.

`GET /api/v1/cash-registers` (la del cajero) **no** lo trae: rompería la apertura a ciegas.

## Vista del turno abierto (existe)

Agrega `fixedFloat: number | null` de su caja: la hoja de cierre lo pone en el campo.

## POST cierre de caja (existe, `CierreCmd`)

Campos nuevos/cambiados:

```json
{ "floatLeft": 800.00, "fixedFloatShown": 1000.00, "withdrawalUserId": 12 }
```

- `floatLeft` ausente → `min(fijo, contado)`, si no hay fijo lo contado. Presente y > contado → 400.
- `fixedFloatShown`: el fijo que la hoja mostró; es el snapshot `fixed_float_at_close`. Ausente → el fijo vigente.
- `withdrawalUserId` ausente con retiro > 0 → quien cierra. Usuario de otra empresa o inactivo → 400.
- Un segundo cierre del mismo turno → el 409 que ya existe; nunca un segundo retiro (índice único).

Respuesta (`SessionView`) agrega:

```json
{ "floatLeft": 800.00, "fixedFloatAtClose": 1000.00,
  "withdrawal": { "amount": 4500.00, "userName": "Ana" } }   // null si no hubo retiro
```

`withdrawal` y `floatLeft` siempre presentes (null si no aplica), nunca omitidos.
