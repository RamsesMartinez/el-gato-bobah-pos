# Quickstart: validar venta por plataformas digitales

Cómo comprobar que la feature funciona de punta a punta. Los detalles del esquema están en
[data-model.md](data-model.md) y los de la API en [contracts/api.md](contracts/api.md).

## Prerrequisitos

```bash
make start                 # Postgres, Redis, API y web
```

La migración 0037 se aplica sola al arrancar el binario (goose embebido). Verifica que entró:

```bash
docker exec deploy-postgres-1 psql -U gatobobah -d gatobobah -c \
  "select name, price_markup_pct from delivery_platforms order by id"
```

Esperado: 35.00 en Didi, Uber Eats y Rappi; **0 en Propio**.

## Verificación 1 — los grants (la que solo falla en producción)

**Córrela siempre.** En dev la API sirve como owner, así que un grant faltante no se nota; en
producción sirve como `gatobobah_app` y el primer request devuelve `42501: permission denied`.

```bash
docker exec deploy-postgres-1 psql -U gatobobah_app -h 127.0.0.1 -d gatobobah -c \
  "select count(*) from product_platform_prices"
```

Esperado: `0` filas, **sin error de permisos**. Si sale `permission denied`, falta el `grant` de la
migración y el deploy rompería producción entera.

El equivalente automatizado —el que de verdad protege— es el test de integración que toca las dos
tablas por `appRoleStore`:

```bash
cd server && TEST_DATABASE_URL="postgres://gatobobah:gatobobah@localhost:5490/gatobobah_test?sslmode=disable" \
  go test -tags=integration ./internal/integration/ -run TestPlatformPrices
```

## Verificación 2 — el precio sale solo, sin capturar nada

1. Abre la caja principal en `/caja` (sin turno no se puede cobrar).
2. En `/pos`, cambia el selector a **Uber Eats**.
3. Agrega un producto cuyo precio base conozcas.

Esperado: el precio mostrado es `base × 1.35` redondeado a 2 decimales, y el indicador dice con qué
lista se está cobrando.

**Caso obligatorio del redondeo**: usa *BONELESS J - 1 Kg* (base 434.98). Debe mostrar **587.22**,
no 587.223. Agrega 3 y verifica que el total sea **1,761.66** — con el redondeo mal puesto sale
1,761.67 y el ticket que se pega a la bolsa no cuadra por un centavo.

## Verificación 3 — el precio manual persiste y no contamina

1. Con Uber Eats activo, corrige ese producto a **$599.00** desde la pantalla de venta.
2. Cierra la venta y empieza otra en Uber Eats → el producto entra en $599.00.
3. Cambia a **mostrador** → el producto vuelve a $434.98.
4. Cambia a **Rappi** → sale el calculado (587.22), no el $599 de Uber.

## Verificación 4 — quitar la excepción

Quita el precio manual del producto y vuelve a agregarlo en Uber Eats: debe regresar a 587.22.

Es la salida para un precio equivocado pero plausible ($14.90 donde iban $149.00), que pasa todas
las validaciones y no se puede limpiar poniendo 0.

## Verificación 5 — cambiar de lista con el ticket armado

Arma un ticket de 3 productos en mostrador y cambia a Uber Eats **sin vaciarlo**.

Esperado: las 3 líneas ya en el ticket cambian de precio y el total de pantalla coincide con el que
devuelve el servidor al cobrar. Es el riesgo principal del diseño: si no coinciden, el operador
entrega un ticket con un total que no es el cobrado.

## Verificación 6 — cobro y corte

1. Cobra el ticket de Uber Eats. Solo debe ofrecerse el método **Uber Eats**.
2. Confirma que **no** se cobró envío (lo reparte la plataforma).
3. Imprime el ticket: los precios son los de la lista.
4. Cierra la caja en `/caja`.

Esperado en el corte: Uber Eats aparece en su propia línea con el total, y **no** suma al efectivo a
contar en el cajón.

## Verificación 7 — el inventario no cambia

Vende el mismo producto con receta en mostrador y en las 3 plataformas, y compara los movimientos:

```bash
docker exec deploy-postgres-1 psql -U gatobobah -d gatobobah -c \
  "select o.delivery_platform_id, sm.ingredient_id, sm.quantity
     from stock_movements sm join orders o on o.id = sm.order_id
    order by sm.id desc limit 20"
```

Esperado: la cantidad descontada es idéntica en los 4 casos. El margen es de precio de venta, no de
costo.

## Verificación 8 — una plataforma ajena no cobra a precio de mostrador

Manda un `POST /orders` con un `deliveryPlatformId` que no exista en la empresa.

Esperado: **422**. Si respondiera 201, la venta se habría cobrado a precio de mostrador con el
ticket bien impreso, y el descuadre aparecería al conciliar el depósito semanas después.

## Gates antes de dar por buena la feature

```bash
make api-build && make api-test    # backend
cd web && bun run test && bun run typecheck && bun run build
```

Y, por ser endpoint de escritura nuevo accesible a cajero, pasar el subagente `security-auditor`
(principio V) y el `db-architect` sobre la migración final.
