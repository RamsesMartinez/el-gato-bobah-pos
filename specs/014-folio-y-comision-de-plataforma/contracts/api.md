# Contratos de API: folio de plataforma y liquidación

Todo bajo `/api/v1`, con `RequireAuth`. Los errores usan el sobre de siempre
(`{error:{code,message,details?}}`) y el mapeo a HTTP vive **solo** en
[`httpapi.Error`](../../../server/internal/httpapi/respond.go).

**Autorización, en una tabla, para que no haya que buscarla** (FR-019, FR-020 — el front es espejo,
nunca la barrera):

| Endpoint | Rol | Por qué |
|---|---|---|
| `POST /orders` (campo nuevo) | el de hoy: cualquiera que venda | El folio se captura en el mismo acto que el pedido |
| `PATCH /orders/{id}/platform-ref` | admin · gerente · cajero | Es la corrección del mismo dato, movida en el tiempo. Mismo criterio que los precios de plataforma en 0037 |
| `PUT /orders/{id}/settlement` | admin · gerente | Es dinero que no pasó por la caja |
| `GET /orders/{id}/settlement` | admin · gerente | Idem |
| `GET /sales` · `/sales/summary` (filtro nuevo) | admin · gerente | Ya lo eran |
| `GET /platform-settlements/summary` | admin · gerente | Ya es el gate de la pantalla de Ventas |

---

## 1. `POST /orders` — gana un campo

```jsonc
{
  "clientUuid": "…",
  "serviceType": "domicilio",
  "deliveryPlatformId": 6,
  "platformOrderRef": "4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44",  // ← nuevo, opcional
  "lines": [ /* sin cambios */ ]
}
```

| Regla | Respuesta |
|---|---|
| Ausente o `null` | Se crea sin folio. El pedido aparece en el filtro de pendientes |
| Presente con `deliveryPlatformId: null` | `400 VALIDATION` — un pedido de mostrador no tiene folio de plataforma |
| Vacío o solo espacios | `400 VALIDATION` — la ausencia se representa como ausencia, nunca como `""` |
| Más de 64 caracteres | `400 VALIDATION` |
| Ya usado por otro pedido de **esa** empresa y **esa** plataforma | `409 PLATFORM_REF_TAKEN`, y el mensaje **dice cuál pedido lo tiene** |
| Ya usado en **otra** empresa | Se acepta. La unicidad es por empresa y plataforma, nunca global |

El valor se guarda **tal cual**, recortando solo los espacios de los extremos: ni mayúsculas, ni
guiones, ni longitud. Es lo que lo hace comparable contra el documento de pago.

**El mensaje del duplicado**, porque un aviso genérico manda al operador a buscar a ciegas:

```json
{"error":{"code":"PLATFORM_REF_TAKEN",
          "message":"Ese folio de Uber Eats ya está en el pedido Tigre (#187) del 5 de septiembre",
          "details":{"orderId":187,"folioName":"Tigre","dailyNumber":187,"businessDate":"2026-09-05"}}}
```

No filtra nada entre empresas: la búsqueda del pedido que ya lo tiene corre **bajo RLS**, así que
solo puede encontrar pedidos de la misma empresa.

`OrderView` (la respuesta) gana `platformOrderRef: string | null`.

## 2. `PATCH /orders/{id}/platform-ref` — escribir o corregir después

```jsonc
// request
{ "platformOrderRef": "4B2E9A10-…" }
// response 200
{ "id": 187, "platformOrderRef": "4B2E9A10-…" }
```

- **No hay borrado.** Sobrescribe siempre; `null` o vacío responden `400 VALIDATION`. El folio es el
  único dato irrecuperable de esta feature y corregir un dedazo es sobrescribir, no vaciar: un camino
  que lo destruye no resuelve ningún caso que el otro no resuelva ya.
- Mismas validaciones que el punto 1, más `404` si el pedido no existe **en esta empresa**.
- **Funciona sobre un pedido cobrado, cancelado o de un arqueo ya cerrado**, y no mueve ninguna cifra
  de venta, corte ni arqueo (FR-006). Escribe `platform_order_ref`, `platform_ref_set_by`,
  `platform_ref_set_at` y `updated_at`; nada más. Las tres del folio se escriben **juntas** — el
  esquema tiene un check todo-o-nada que lo garantiza.
- Un pedido **sin plataforma** responde `400 VALIDATION`.
- Lleva `rateLimitUser`, como los precios de plataforma: es una escritura que alcanza el rol cajero, y
  el tope va **después** de `RequireRole` para contar solo a quien sí tenía permiso.

**Por qué `PATCH` a un sub-recurso y no `PATCH /orders/{id}`**: no existe hoy un endpoint genérico de
edición de pedido, y abrirlo para una columna pondría todas las demás al alcance del mismo gate.

## 3. `PUT /orders/{id}/settlement` — registrar la liquidación

Idempotente por definición: **reemplaza** (`insert … on conflict (order_id) do update`), no duplica
(FR-014).

```jsonc
// request — todo lo que dice el documento de pago, nada calculado
{
  "reportedGross":     "220.00",
  "commissionAmount":  "33.00",
  "commissionPct":     "30.00",   // null si el documento no la declara
  "discountTotal":     "110.00",
  "discountPlatform":  "0.00",    // lo que puso la plataforma; el resto lo puso el restaurante
  "withholdings":      "9.96",
  "netAmount":         "51.77",   // PUEDE SER NEGATIVO
  "payoutReference":   "PAY-2026-09-12-0043",
  "documentRef":       "uber-payments-2026-09-08.csv"
}
```

```jsonc
// response 200
{
  "orderId": 187,
  "reportedGross": "220.00", "commissionAmount": "33.00", "commissionPct": "30.00",
  "discountTotal": "110.00", "discountPlatform": "0.00",
  "discountRestaurant": "110.00",       // DERIVADO: total − plataforma. No se almacena
  "withholdings": "9.96", "netAmount": "51.77",
  "payoutReference": "PAY-2026-09-12-0043", "documentRef": "uber-payments-2026-09-08.csv",
  "capturedAt": "2026-09-12T18:03:00Z", "capturedBy": "Ramses"
}
```

**Validaciones** — todas `400 VALIDATION`, nunca un 500 (FR-018):

| Caso | Por qué se rechaza |
|---|---|
| Comisión, bruto, descuentos o retenciones **negativos** | `ValidMoney(v, true)` |
| Cualquier importe fuera de las cotas de dinero (`MaxMoney`, escala absurda) | Un `1e100000000` de 47 bytes quema ~25 s de CPU antes de que nadie lo valide |
| `commissionPct` fuera de 0–100 | Cota de cordura del documento |
| `discountPlatform > discountTotal` | La parte no puede ser mayor que el todo |
| El pedido no es de plataforma | Una liquidación de mostrador no significa nada |
| El pedido no existe en esta empresa | `404` |

**Lo que NO se valida, a propósito**: que `netAmount` sea positivo (escenario 4), que la comisión sea
menor que la venta (una promoción la hace mayor y es real), y que `reportedGross` cuadre con
`orders.total` (cuadrarlos es la feature de conciliación, y rechazar aquí impediría registrar
exactamente la discrepancia que se quiere ver).

## 4. `GET /orders/{id}/settlement`

`200` con el cuerpo de arriba, o **`404 NOT_FOUND`** si no hay liquidación registrada.

**El 404 es la respuesta correcta y es la mitad de FR-015**: "todavía no se sabe" y "la plataforma no
cobró nada" no son lo mismo. Una liquidación de ceros es un `200` con ceros; la ausencia es un `404`.
Devolver ceros en los dos casos borraría la distinción que esta feature viene a crear.

## 5. `GET /sales` y `GET /sales/summary` — un filtro, una búsqueda y una columna

**Filtro nuevo**: `?folioPlataforma=pendiente`

| Valor | Efecto |
|---|---|
| Ausente | No filtra (default del parámetro **ausente**, que es el único caso en que hay default) |
| `pendiente` | Solo pedidos **de plataforma** **sin** folio |
| Cualquier otro | `400 VALIDATION`. Nunca cae en silencio a "todos" (FR-010) |

Aplica a la lista **y** a las tres consultas del resumen, palabra por palabra: el escenario 2 de la
US-2 exige que las dos describan el mismo conjunto **sin excepción**. Es la familia de
`serviceType`, no la de `status` (que a propósito no toca el resumen).

**Búsqueda nueva**: `?folio=<identificador exacto>`

| Valor | Efecto |
|---|---|
| Ausente | No busca |
| Un identificador | Devuelve **el** pedido de plataforma cuyo folio es exactamente ese (igualdad, no parcial) |
| Un identificador que nadie tiene | Lista vacía y resumen en ceros. No es un error: es la respuesta a "este renglón del documento no está capturado" |
| Más de 64 caracteres | `400 VALIDATION` |

Busca **solo** el folio de plataforma, nunca el número ni el nombre interno del pedido — son cosas
distintas que comparten la palabra "folio", y mezclarlas haría que `187` devolviera el pedido #187 y
además cualquier folio que contenga 187.

Se puede combinar con `folioPlataforma=pendiente`, y la combinación devuelve vacío por construcción:
un pendiente no tiene folio que buscar. Es correcto, no un caso que haya que impedir.

**Campo nuevo en cada renglón de `/sales`**: `platformOrderRef: string | null`.

## 6. `GET /platform-settlements/summary` — las tres cifras de SC-006

Mismos parámetros de rango que `/sales` (`preset`, `from`, `to`), con la misma validación: un preset
desconocido o unas fechas que el preset no usa se rechazan.

```jsonc
{
  "range": { "from": "2026-09-01", "to": "2026-09-30" },

  // Cada cifra declara qué incluye y qué excluye. No son renglones hermanos y no se suman.
  "vendido": {
    "amount": "18420.00", "orders": 96,
    "incluye": "El total que cobró el POS en pedidos de plataforma del periodo",
    "excluye": "Canceladas, reembolsadas y pedidos que no son de plataforma"
  },
  "seQuedoLaPlataforma": {
    "amount": "5940.00", "orders": 74,
    "incluye": "Comisión + retenciones de los pedidos CON liquidación capturada",
    "excluye": "Los 22 pedidos de plataforma sin liquidación: de esos todavía no se sabe"
  },
  "llegoAlBanco": {
    "amount": "12480.00", "orders": 74,
    "incluye": "El neto del documento, de los mismos 74 pedidos",
    "excluye": "Depósitos que no se pudieron atribuir a un pedido"
  },

  "sinLiquidar":  { "orders": 22 },
  "sinFolio":     { "orders": 3 }
}
```

**La regla que sostiene este contrato**: `vendido` sale de `orders` y las otras dos de
`platform_settlements`, y **cubren conjuntos distintos** — 96 pedidos contra 74. Presentarlas sin
decirlo invita a restar y a reportar un margen que el negocio no tuvo; es la misma forma del fondo de
caja que dejó un turno con $4,500 de faltante. Por eso cada cifra viaja con `orders`, `incluye` y
`excluye`, y por eso **`llegoAlBanco` no es `vendido − seQuedoLaPlataforma`**.

**Ninguna de estas cifras entra a `/sales/summary`, al corte de caja ni al arqueo** (FR-016). Son
endpoints distintos a propósito.

---

## 7. Lo que NO cambia

- El cálculo de precios, el cobro, el corte de caja, el arqueo y el ticket impreso: **byte por byte
  iguales**. SC-005 se verifica contra una copia de los datos de producción.
- El costo de envío de un pedido de plataforma sigue siendo cero (lo cobra la plataforma).
- Un pedido de plataforma sigue exigiendo turno de caja abierto. Este plan no lo cambia; el spec ya
  declara esa puerta por su nombre.
