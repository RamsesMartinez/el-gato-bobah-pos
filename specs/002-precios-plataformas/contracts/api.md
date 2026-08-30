# Contratos de API: venta por plataformas digitales

Todo bajo `/api/v1`, con `RequireAuth`. Los errores usan el sobre de siempre
(`{error:{code,message,details?}}`) y el mapeo vive solo en `httpapi.Error`.

## GET `/pos/menu` (cambia)

El documento del menú gana lo necesario para que el POS pinte cualquier lista **sin pedir nada
más**: el precio base ya venía; se agregan el margen de cada plataforma y el mapa disperso de
excepciones.

```jsonc
{
  "categories": [ /* sin cambios */ ],
  "products":   [ /* sin cambios: price sigue siendo el BASE */ ],
  "platforms": [
    { "id": 5, "name": "Didi",      "markupPct": 35 },
    { "id": 6, "name": "Uber Eats", "markupPct": 35 },
    { "id": 7, "name": "Rappi",     "markupPct": 35 },
    { "id": 8, "name": "Propio",    "markupPct": 0 }
  ],
  // Solo las EXCEPCIONES. Un producto ausente usa base × (1 + markupPct/100), redondeado a 2dp.
  "platformPrices":   { "5": { "512": 149.00 }, "7": { "512": 155.00 } },
  "platformModPrices": { "5": { "301": 30.00 } }
}
```

**Por qué todo en un documento y no un endpoint por plataforma**: el POS ya carga el menú una vez y
lo cachea; cambiar de lista tiene que ser instantáneo, y una llamada por cambio de plataforma haría
lento justo el momento que esta feature viene a acelerar. El caché de Redis sigue siendo uno por
empresa (`pos:menu:<companyID>`) — la plataforma **no** entra en la llave.

**Autorización**: la de hoy. Cualquier rol que pueda vender.

## PUT `/platform-prices/product`

Captura o corrige el precio de un producto en una plataforma. Idempotente (upsert).

```jsonc
// Request
{ "productId": 512, "platformId": 5, "price": 149.00 }
// 200 → el precio efectivo que queda
{ "productId": 512, "platformId": 5, "price": 149.00, "source": "manual" }
```

| Código | Cuándo |
|---|---|
| `400 VALIDATION` | Falta un campo o el body no es JSON |
| `404 NOT_FOUND` | El producto o la plataforma no son de esta empresa (RLS no los ve) |
| `422 UNPROCESSABLE` | Precio ≤ 0 o fuera de los topes de `ValidMoney` |

**Efectos**: escribe `updated_by` con el usuario del token e invalida `pos:menu:<companyID>`, o la
otra tablet sigue mostrando el precio viejo hasta 24 h.

**Autorización**: cualquier rol que pueda vender, incluido cajero. Es una decisión de agilidad del
dueño; el rastro de quién lo hizo es la mitigación.

## DELETE `/platform-prices/product?productId=&platformId=`

Quita la excepción: el producto vuelve al precio calculado. `204` también si no había fila (borrar
lo que no existe deja el mundo como se pidió).

Existe porque un precio equivocado pero plausible ($14.90 donde iban $149.00) pasa todas las
validaciones, y `price > 0` cierra el idioma "pon 0 para limpiar".

## PUT y DELETE `/platform-prices/modifier-option`

Lo mismo para una opción de modificador. El campo es `priceDelta` y acepta **0** (un extra sin
costo es normal); sigue rechazando negativos.

## POST `/orders` (cambia)

El cuerpo es el de hoy. `deliveryPlatformId` deja de ser una etiqueta y pasa a **elegir la lista de
precios**, así que el servidor lo resuelve en vez de confiarlo:

- Se busca la plataforma **bajo RLS**. Si no aparece → `422`, nunca un margen de 0 por default. Un
  id de otra empresa cobraría a precio de mostrador en Uber, con el ticket bien impreso y el
  descuadre apareciendo semanas después al conciliar el depósito.
- Cada línea se valúa con `precioEfectivo(base, margen, manual)`, redondeado a 2dp **en el
  unitario**, antes de tocar `numeric(10,2)`.
- Los precios que mande el cliente se siguen ignorando, igual que hoy.
- Con plataforma, `deliveryFee` se fuerza a **0**: el reparto lo cobra la plataforma.
- El método de pago debe ser el de esa plataforma → si no, `422`.

Sin `deliveryPlatformId` el comportamiento es idéntico al actual: precio base, mostrador.

## Lo que NO cambia

- `GET /payment-methods` — los tres métodos de plataforma ya existen.
- `POST /cash-sessions/close` — el corte ya agrupa por método activo, así que cada plataforma ya
  sale en su renglón sin contar como efectivo del cajón. Se cubre con un test, no con código.
- El ticket impreso — ya usa el `unitPrice` que viene del pedido, que es el de la lista con la que
  se cobró.
