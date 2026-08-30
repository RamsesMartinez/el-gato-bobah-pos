# Auditoría del endpoint de precios por plataforma

Hallazgos de la auditoría adversarial (`security-auditor`) sobre las rutas `/platform-prices`,
agosto 2026, y los diagramas que explican el más grave.

## El agujero que ya estaba vivo en producción

Un cuerpo JSON de **47 bytes** quemaba ~25 segundos de CPU y 279 MiB de memoria:

```
PUT /api/v1/platform-prices/product
{"productId":2,"platformId":2,"price":1e100000000}
```

Con `1e2000000000` pedía ~830 MB y el sistema mataba el contenedor. **No lo trajo esta feature**:
la misma trampa entraba por `POST /orders` (la cantidad de una línea, sin exigir rol) y por el alta
de productos.

### Por qué

`shopspring/decimal` guarda un número en dos piezas —`value *big.Int` y `exp int32`—, así que
`1e100000000` ocupa ~16 bytes en memoria y decodificarlo es instantáneo. Redondear a 2 decimales
obliga a llevarlo a exponente `-2`, y eso significa **escribir el número completo**: un `big.Int` de
100,000,001 dígitos.

Es una bomba zip: 11 caracteres que denotan cien millones de dígitos. El costo de `big.Int` no
depende del texto que mandaste sino de los dígitos del resultado, así que "Go es rápido" no aplica.

### El error de orden

```go
p := domain.Round2(price)          // revienta AQUÍ
if !domain.ValidMoney(p, false) {  // nunca alcanza a correr
```

La validación de cotas existía y llegaba tarde por una línea. Y moverla dentro de `ValidMoney`
tampoco bastaba: `v.GreaterThan(MaxMoney)` compara, y comparar dos decimales **también** los pone a
la misma escala. Por eso la guarda solo puede mirar el exponente, que es leer un `int32`.

### El arreglo

[`domain.escalaSana`](../../server/internal/domain/limits.go), llamada como **primera** operación en
`Round2`, `Round4`, `ValidMoney` y `ValidQty`. Va en las cuatro porque todas las fronteras del repo
redondean antes de validar, y una guarda puesta solo en la validación llega tarde en todas.

Medido: el test de regresión pasaba de colgarse 45 s y morir por timeout, a **0.00 s**. Contra el
ambiente de pruebas real, el request de arriba responde **400 en 0 segundos**.

## Diagramas

| Archivo | Qué muestra |
|---|---|
| [01-antes.png](diagramas/01-antes.png) | Secuencia del fallo: dónde explota y por qué la validación nunca corre |
| [02-despues.png](diagramas/02-despues.png) | La misma secuencia con la guarda: rechazo en 0 s |
| [03-por-que-explota.png](diagramas/03-por-que-explota.png) | Por qué el número es chico de escribir y enorme de materializar, y por qué ninguna de las defensas habituales ayuda |

Las fuentes `.mmd` están junto a los PNG. Para regenerarlos:

```bash
MSYS_NO_PATHCONV=1 docker run --rm -v "d:/git/el-gato-bobah-pos/docs/auditoria/diagramas:/data" \
  minlag/mermaid-cli -i /data/01-antes.mmd -o /data/01-antes.png -w 1400 -b white
```

## El otro bloqueador: escritura entre empresas

Un cajero de la empresa A podía escribir un precio sobre el producto de la empresa B. Los chequeos
de llave foránea de Postgres **saltan RLS por diseño**, así que la fila entraba con el `company_id`
de A ocupando la llave primaria global `(product_id, platform_id)`. A partir de ahí B no podía
capturar su propio precio —su upsert chocaba con la política y salía como 500— ni borrar la fila
intrusa, porque bajo RLS no la ve.

Se cerró validando la pertenencia bajo RLS antes de escribir, devolviendo **el mismo error** para
"no existe" y "no es tuyo": distinguirlos convertía el endpoint en un censo de los catálogos ajenos.

## La mitigación que no existía

Tres comentarios del código declaraban que el riesgo de dejar al cajero editar precios estaba
mitigado por `updated_by`. El auditor mostró el ataque: poner un producto en $1 para una plataforma,
cobrar en efectivo lo que la plataforma facturó, y **borrar la excepción** — el DELETE se lleva
precio, quién y cuándo. El corte cuadra y no queda rastro.

Las cuatro rutas emiten ahora `logging.SecurityEvent`, que vive fuera de la tabla.

## Sobre el 204 del DELETE

Borrar un precio devuelve 204 aunque no hubiera nada que borrar: el operador pidió que el producto
vuelva a su precio calculado, y si no había excepción ya estaba así.

Ese razonamiento **era falso mientras existía el agujero anterior**: el dueño legítimo pedía borrar,
su borrado no veía la fila intrusa, y el sistema respondía "listo" con el precio todavía puesto.
Cerrado el agujero, el único caso de cero filas es que la excepción no existiera, y la respuesta
vuelve a coincidir con la realidad.
