# Quickstart: validar folio de plataforma y liquidación

Cómo comprobar que la feature hace lo que dice. **Lo que falla se reporta antes que lo que
funciona**; lo que no se midió se llama no verificado.

## Prerequisitos

```bash
make start                      # Postgres, Redis, API y web (puertos auto-ajustados)
PG_PORT=… make db-migrate       # aplica 0065
```

Para los tests de integración hace falta Postgres real **y, para los de migración y dinero, la base
restaurada de un respaldo anonimizado de producción** — con una base sembrada limpia todo camino "por
cada otra empresa" es un no-op y las formas que muerden (pedidos viejos sin nombre de folio, pedidos
sin sesión de caja) no existen:

```bash
export TEST_DATABASE_URL="postgres://…/gatobobah_test?sslmode=disable"
make respaldo-anonimo          # baja el dump, borra nombres y notas, restaura en TEST_DATABASE_URL
cd server && go test -tags=integration ./internal/integration/...
```

El respaldo se apoya en el acceso que ya existe (`make prod-db-tunnel`, `gcloud compute ssh` a
`pos-vps`). **Anonimizado, no crudo**: los nombres de cliente y las notas de pedido son PII y una base
que corre en CI no es lugar para eso. Importes, fechas, estados, sesiones y plataformas se conservan
intactos — son justo lo que estos tests vienen a probar.

> En Windows nada de esto corre en el host: Smart App Control bloquea binarios recién compilados.
> Usa los caminos en contenedor de [AGENTS.md §7](../../AGENTS.md).

---

## 1. Los gates, primero

```bash
cd server && go build ./... && go test ./...        # = make api-build && make api-test
cd server && go test -tags=integration ./internal/integration/...
cd web && bun run lint && bun run vitest run && bun run build
```

Ninguno se da por bueno sin correrlo. Un hook de lefthook en rojo se arregla, no se salta.

---

## 2. La migración, contra dos empresas

**Con una sola empresa toda la migración pasa verde y rompe en producción**: los caminos "por cada
otra empresa" son no-op. El test parte de un esquema restaurado y siembra una segunda empresa.

| Qué se prueba | Cómo falla si está mal |
|---|---|
| El `grant` a `gatobobah_app` | `42501 permission denied` bajo `appRoleStore`, no bajo el owner |
| La policy de RLS aísla las liquidaciones | La empresa B lee la liquidación de la A |
| El índice único es **por empresa y plataforma** | Dos empresas no pueden usar el mismo folio, o la misma empresa lo repite entre plataformas |
| El check de forma | Un `""` o un `" abc "` entra a la columna |
| El check de plataforma | Un pedido de mostrador acepta folio |
| La FK compuesta rechaza una liquidación cruzada | Una fila con `company_id` de una empresa y `order_id` de otra entra sin protestar y sale del resumen de dinero |
| El check todo-o-nada del rastro del folio | Se puede escribir el folio sin `set_by`/`set_at` |
| El `Down` deja el esquema como estaba | `goose down` truena o deja columnas (incluido `orders_id_company_key`) |

### El planner usa el índice, medido vía pgx

El índice parcial del filtro de pendientes **solo se usa con el predicado literal**, y el defecto no
se ve con psql. Se verifica sobre la ejecución real:

```sql
set plan_cache_mode = force_generic_plan;   -- el modo al que pgx cae solo
explain (analyze, buffers) <la consulta que emite sqlc>;
```

**Esperado**: la consulta de pendientes entra por `orders_plataforma_sin_folio` y la de búsqueda por
`orders_platform_ref_busqueda`. Si en la de pendientes aparece `Bitmap Index Scan on
orders_date_status` con el predicado en `Filter:`, la consulta volvió al patrón
`narg … is null or (…)` y SC-008 no se cumple.

---

## 3. El folio, a mano

```bash
API=http://localhost:8080/api/v1
TOKEN=…   # sesión de cajero

# 3.1 Un pedido de plataforma nace con su folio
curl -s -X POST $API/orders -H "Authorization: Bearer $TOKEN" -d '{
  "clientUuid":"'"$(uuidgen)"'", "serviceType":"domicilio", "deliveryPlatformId":6,
  "platformOrderRef":"  4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44  ",
  "lines":[{"productId":512,"qty":"1"}] }'
# Esperado: 201, platformOrderRef SIN los espacios de los extremos y con las mayúsculas intactas.

# 3.2 El mismo folio en la misma plataforma
# Esperado: 409 PLATFORM_REF_TAKEN, y el mensaje NOMBRA el pedido que ya lo tiene.

# 3.3 Vacío / solo espacios / 65 caracteres
# Esperado: 400 VALIDATION en los tres.

# 3.4 Folio en un pedido de mostrador (sin deliveryPlatformId)
# Esperado: 400 VALIDATION.

# 3.5 Corregirlo después, sobre un pedido ya cobrado
curl -s -X PATCH $API/orders/187/platform-ref -H "Authorization: Bearer $TOKEN" \
  -d '{"platformOrderRef":"4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44"}'
# Esperado: 200, y las tres columnas del rastro escritas juntas.

# 3.6 Intentar VACIAR el folio
curl -s -X PATCH $API/orders/187/platform-ref -H "Authorization: Bearer $TOKEN" \
  -d '{"platformOrderRef":null}'
# Esperado: 400. No hay borrado: corregir es sobrescribir.
```

**Cierra lo que crees.** Un pedido de prueba que se queda abierto aparece en la barra del POS, suma a
"por cobrar" y bloquea el cierre de caja: entregar (`POST /orders/:id/deliver`) y cobrar
(`POST /orders/:id/pay`, **no** `/charge`), y un pedido de plataforma solo acepta el método de SU
plataforma.

---

## 4. Que corregir un folio no mueva un peso — el criterio SC-005

Es el escenario que más caro sale si falla, y **no se razona: se mide** contra Postgres.

1. Fotografía: `/sales/summary`, el detalle del arqueo cerrado que contiene al pedido, y el corte del
   turno.
2. `PATCH /orders/{id}/platform-ref` sobre un pedido de ese arqueo.
3. Vuelve a pedir las tres cosas.

**Esperado**: idénticas, importe por importe. El test de integración falla **nombrando la cifra que se
movió**, no con "esperaba X obtuve Y" — la forma de
[`TestElFondoDeCajaSeCuentaUnaSolaVez`](../../server/internal/integration/corte_plataformas_test.go).

---

## 5. El filtro de pendientes: la lista y el resumen dicen lo mismo

```bash
curl -s "$API/sales?preset=mes&folioPlataforma=pendiente" -H "Authorization: Bearer $ADMIN"
curl -s "$API/sales/summary?preset=mes&folioPlataforma=pendiente" -H "Authorization: Bearer $ADMIN"
```

| Comprobación | Esperado |
|---|---|
| La lista trae solo pedidos de plataforma sin folio | Ninguno de mostrador, ninguno con folio |
| `total` de la lista y `count` del resumen | **El mismo número**. Si divergen, uno de los dos miente y quien lo lee no puede saber cuál |
| `?folioPlataforma=pendientes` (con `s`) | `400 VALIDATION`, nunca "todos" |
| Escribirle el folio a uno de ellos | Desaparece del filtro en la siguiente consulta |

---

## 5-bis. Buscar un pedido por su folio

Es el criterio SC-002: con el documento de pago a la vista, cualquier renglón se localiza **en un
solo paso**, sin comparar montos ni fechas.

```bash
curl -s "$API/sales?preset=mes&folio=4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44" -H "Authorization: Bearer $ADMIN"
```

| Comprobación | Esperado |
|---|---|
| Pegar el folio del documento | **Un** pedido: ese |
| Un folio que nadie capturó | Lista vacía y resumen en ceros — no un error. Es la respuesta a "este renglón no está capturado" |
| Buscar el número interno del pedido (`187`) | **No lo encuentra**. La búsqueda es del folio de plataforma, no del folio del turno |
| Buscar el nombre del pedido (`Tigre`) | **No lo encuentra**, por lo mismo |
| El mismo folio existiendo en otra empresa | No aparece |
| Un folio de 65 caracteres | `400 VALIDATION` |
| Combinado con `folioPlataforma=pendiente` | Vacío, por construcción: un pendiente no tiene folio |

---

## 6. La liquidación

```bash
curl -s -X PUT $API/orders/187/settlement -H "Authorization: Bearer $ADMIN" -d '{
  "reportedGross":"220.00","commissionAmount":"33.00","commissionPct":"30.00",
  "discountTotal":"110.00","discountPlatform":"0.00","withholdings":"9.96",
  "netAmount":"51.77","payoutReference":"PAY-2026-09-12-0043",
  "documentRef":"uber-payments-2026-09-08.csv" }'
```

Los números salen del 2x1 real de Uber Eats medido en
[plataformas-digitales.md §4-bis](../../docs/plataformas-digitales.md) — no se re-derivan.

| Comprobación | Esperado |
|---|---|
| `discountRestaurant` en la respuesta | `110.00` (derivado: total − plataforma) |
| Registrar otra vez con un documento corregido | **Reemplaza**; sigue habiendo una sola liquidación y `capturedAt` se mueve |
| `GET` de un pedido **sin** liquidación | `404`. Nunca ceros: "no se sabe" ≠ "cobró cero" |
| `netAmount: "-31.20"` | **200**. Es lo que de verdad pasa con una promoción que financió el restaurante |
| `commissionAmount: "-1.00"` | `400`, no 500 |
| `commissionPct: "180"` | `400` |
| `discountPlatform: "150"` con `discountTotal: "110"` | `400` |
| `reportedGross: "1e100000000"` | `400` en milisegundos, sin quemar CPU |
| Con sesión de **cajero** | `403`. Es dinero que no pasó por la caja |
| `/sales/summary` antes y después de registrarla | **Idéntico**: la comisión no se resta de ninguna venta (FR-016) |

---

## 7. Las tres cifras que hoy no existen — SC-006

```bash
curl -s "$API/platform-settlements/summary?preset=mes" -H "Authorization: Bearer $ADMIN"
```

**Esperado**: `vendido`, `seQuedoLaPlataforma` y `llegoAlBanco`, cada una con su conteo de pedidos y
su `incluye`/`excluye`. La comprobación que importa: **`vendido` cubre más pedidos que las otras dos**
cuando hay liquidaciones pendientes, y `llegoAlBanco` **no** es la resta de las otras dos. Un test de
regresión falla nombrando el concepto que se duplicó si alguien las hace cuadrar a la fuerza.

---

## 8. La pantalla, medida a 1024×600

No basta con vitest: las medidas de Chakra son clases CSS y jsdom no las resuelve, así que un assert
de píxeles allá pasa verde con los controles chicos. Hace falta un navegador.

```bash
cd web && bun run e2e     # Playwright a 1024×600, contra el ambiente de pruebas desplegado
```

| Comprobación | Esperado |
|---|---|
| Renglones del mosaico con plataforma activa y sin el aviso de caja | **3**, los mismos que hoy. Si bajan a 2, se anota como el renglón que SC-007 permite y se dice |
| El campo de folio con la lista en Mostrador | **No existe en el árbol**, no "existe oculto" |
| Alto del campo | ≥ 44 px |
| Mandar un pedido de plataforma con el campo vacío | Aparece la hoja que pide el dato, con **una** salida explícita |
| Tomar la salida y mandar | El pedido entra sin folio y sale en el filtro de pendientes |
| Con el campo lleno | Mandar cuesta **los mismos toques que hoy** |
| **Con el teclado abierto**, la salida *Mandar sin folio* | Sigue visible y tappable. Si el teclado la tapa, SC-003 pasa de un toque a dos |
| Botones de plataforma en el estado "Mostrador" | ≥ 44 px (hoy miden 40) |
| El folio en la celda "Tipo" de la lista de Ventas | Truncado con elipsis, **una sola línea**; el completo en el detalle |
| El filtro de pendientes | Se enciende y se apaga con **un tap cada uno** |
| Los rótulos del folio de plataforma | Nombran la plataforma (*Folio de Uber Eats*), nunca dicen "folio" a secas — esa palabra ya es el número del turno en esta misma pantalla |
| Registrar una liquidación de punta a punta | Cronometrado y **declarado** contra los 30 segundos de SC-004 |
| `LiquidacionSheet` con el teclado abierto | Los 7 campos de dinero en dos columnas y el botón de guardar visible |
| Tiles de plataforma | Cada uno con su conteo de pedidos a la vista, no solo en el JSON |

**El resultado se anota en [presupuesto-de-pantalla-1024x600.md](../../docs/presupuesto-de-pantalla-1024x600.md)**,
que es donde vive la medición y donde hoy dice que el bloque del selector mide 40/83 px. Un número
nuevo sin su renglón ahí se pierde en la siguiente sesión.

**Y la pantalla de Ventas se mide por primera vez**, con el mismo método: hoy ese documento solo
cubre el POS, y esta feature le agrega a Ventas un toggle, una fila de tiles y texto en una celda.
El conteo de renglones de tabla antes y después va en el mismo documento. Estimado sin medir: el
chrome ronda 340–350 px de los ~552 px netos, o sea 3–4 renglones — y **estimado se llama estimado**
hasta que haya un navegador de por medio.

Un fallo en el primer `goto` casi siempre es la VM spot apagada, no el código: revísalo antes de
buscar el defecto.

---

## 9. Las matrices se editan junto con los tests

- [docs/matriz-de-pantallas.md](../../docs/matriz-de-pantallas.md): el filtro que se rechaza, la
  lista y el resumen del mismo predicado, el campo que no existe en mostrador, el alto medido.
- [docs/matriz-de-cobro.md](../../docs/matriz-de-cobro.md): que registrar una liquidación **no** mueva
  el cobro, el corte ni el arqueo.

Un renglón sin test no está cubierto y se dice. Un caso que se arregla sin renglón vuelve.
