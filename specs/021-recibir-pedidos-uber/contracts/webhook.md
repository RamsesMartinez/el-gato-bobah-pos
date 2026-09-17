# Contratos — 021 · Recibir los pedidos de Uber Eats

Dos superficies: la que **entra** (Uber nos llama) y la que usa el POS.

---

## A · La puerta pública: `POST /api/v1/webhooks/uber-eats`

Va bajo `/api/v1` porque es lo que Caddy ya enruta al backend, y **fuera** de los dos grupos de
middleware: sin `RequireAuth` y sin `WithTenant`. Quien llama es Uber, no una persona, y la empresa
es el resultado de resolver la tienda, no un dato de entrada.

### Cabeceras

| Cabecera | Obligatoria | Qué se hace con ella |
|---|---|---|
| `X-Uber-Signature` | Sí | HMAC-SHA256 del **cuerpo crudo**, hexadecimal minúsculas. Se compara en tiempo constante contra la llave primaria y, si hay, la secundaria |
| `X-Environment` | Sí | `production` o `sandbox`. Debe corresponder al ambiente del sistema; si no, se rechaza |
| `Content-Type` | Sí | `application/json` |

### Cuerpo

Lo que importa del aviso; el resto se conserva crudo sin interpretarse.

```json
{
  "event_type": "orders.notification",
  "event_id": "a3f1...",
  "event_time": 1758000000,
  "meta": { "user_id": "<id de la tienda>", "resource_id": "<id del pedido>", "status": "pos" },
  "resource_href": "https://test-api.uber.com/v1/eats/orders/<id>"
}
```

**El aviso no trae el pedido.** El detalle se pide con un GET a `resource_href`, y **ese detalle se
guarda crudo** (`raw_detail`): es lo único desde lo cual se puede corregir un mapeo equivocado días
después, cuando Uber ya no entregue ese pedido.

**Si `(plataforma, id de tienda)` resuelve a más de una empresa** —pasa en el ambiente de pruebas,
donde se reparten tiendas de demostración compartidas— se verifica la firma contra cada candidata.
La que valide es la dueña. En el caso normal son cero o una.

### Respuestas

| Código | Cuándo | Cuerpo |
|---|---|---|
| **200** | Procesado, o repetido, o de un tipo que no manejamos (se confirma a propósito para que no se reintente eternamente) | Vacío |
| **400** | Cabecera ausente, cuerpo mal formado, ambiente equivocado | Genérico |
| **401** | La firma no valida, **o la tienda no es de ninguna empresa nuestra** | **El mismo mensaje** en los dos casos: no se dice qué falló, ni si la tienda existe. El aviso de una tienda desconocida no se guarda — no hay empresa a la cual guardarlo |
| **413** | El cuerpo excede el tope | Genérico |
| **429** | Demasiados avisos del mismo origen | Genérico |
| **5xx** | No se pudo traer el detalle, o falló al guardar | Genérico. **Es a propósito**: es lo que hace que Uber reintente |

**Lo que nunca sale en una respuesta**: si la tienda existe, de qué empresa es, qué parte de la
firma falló, ni nada del pedido.

### Tipos de evento

Llegan **todos** a esta misma URL; no hay filtro del lado de Uber.

| `event_type` | Qué hace el sistema |
|---|---|
| `orders.notification` | Trae el detalle, lo registra como pendiente, avisa a la tableta |
| `orders.cancel` / `orders.failure` | Marca cancelado. Si ya estaba aceptado, cancela el pedido del POS |
| `store.provisioned` | Marca la conexión activa |
| `store.deprovisioned` | Marca la conexión inactiva |
| `store.status.changed` | La plataforma abrió o cerró la tienda: se muestra (FR-028) |
| cualquier otro | Se registra y se confirma con 200, **sin procesar** |

### Presupuesto de tiempo

El request completo —verificar, resolver, traer el detalle, guardar— tiene un presupuesto duro. Si
se agota, se contesta 5xx y Uber reintenta. Nunca se confirma un pedido que no se pudo leer.

---

## B · Lo que consume el POS

Todo bajo `/api/v1`, con `RequireAuth` y tenant, como el resto del negocio.

### `GET /orders/platform/pending`

Los pedidos que esperan decisión. Es lo que pinta el aviso de la tableta.

```json
{
  "orders": [{
    "id": 12,
    "platformName": "Uber Eats",
    "displayId": "ABC12",
    "placedAt": "2026-09-17T18:04:00Z",
    "decideBefore": "2026-09-17T18:15:30Z",
    "serviceType": "domicilio",
    "customerName": "Ana",
    "total": "342.00",
    "lines": [{
      "externalName": "Crepa de Nutella",
      "quantity": "2",
      "unitPrice": "129.00",
      "productId": 88,
      "matched": true,
      "options": [{ "externalName": "Extra fresa", "quantity": "1", "unitPrice": "20.00", "matched": false }]
    }]
  }]
}
```

- `lines` y `options` **siempre vienen como arreglo**, nunca `null`, aunque estén vacíos. El test va
  sobre el JSON crudo: deserializar a una estructura de Go borra justo esa diferencia, y esto ya
  tumbó la pantalla de pedidos de producción una vez.
- `matched: false` es lo que la pantalla usa para señalar un platillo sin pareja sin impedir aceptar.

### `POST /orders/platform/{id}/accept`

Sin cuerpo. Crea el pedido en el POS con quien acepta como `opened_by`, ya pagado por la plataforma.

| Respuesta | Cuándo |
|---|---|
| **200** con el pedido creado | Aceptado. La tableta imprime con esto |
| **409** | Ya estaba decidido, o ya expiró |
| **502** | La plataforma rechazó la aceptación. **El pedido NO queda aceptado de nuestro lado**: aceptar aquí y no allá es la peor combinación posible |

### `POST /orders/platform/{id}/deny`

```json
{ "reason": "ITEM_AVAILABILITY", "note": "se acabó la nutella" }
```

`reason` va contra una **whitelist del dominio** con los códigos que la plataforma admite. En la
pantalla **nunca se ve ese código**: va su frase en español (`ITEM_AVAILABILITY` → «Se acabó un
ingrediente»), elegida con `Picker` y nunca con un desplegable del sistema. Un código
desconocido se rechaza con 422 — no cae a `OTHER` en silencio: un parámetro presente y malformado
que cae a un default devuelve una pantalla que se ve bien y reporta algo que nadie pidió.

### `GET /admin/platform-orders`

La historia 4: qué llegó y qué pasó. Lista filtrada, ordenable y paginada **copiando el patrón que
ya existe** (`ListExpenses`/`CountExpenses`), con su `Count…` gemela usando el mismo `where`, y la
whitelist de orden en el dominio con su espejo en `SortHead`.

### Evento en vivo

Por el `realtime.Broker` que ya existe, a `GET /api/v1/events`:

```json
{ "type": "platform.order.received", "data": { "id": 12, "decideBefore": "..." } }
```

Y `platform.order.decided` cuando otra tableta ya lo atendió — sin eso, dos tabletas muestran el
mismo aviso y la segunda toca un botón que ya no hace nada.

---

## C · Lo que se le pide a Uber

| Para qué | Método y ruta | Nota |
|---|---|---|
| El detalle del pedido | `GET` al `resource_href` **del aviso** | No se arma la ruta a mano: se sigue la liga. Armarla a mano es lo que nos dio 404 en siete formas |
| Aceptar | `POST /v1/eats/orders/{id}/accept_pos_order` | Exige ser la app gestora de pedidos de esa tienda — permiso en el ticket abierto |
| Rechazar | `POST /v1/eats/orders/{id}/deny_pos_order` | Con `reason.code` de la lista cerrada |
| Estado de la tienda | `GET /v1/eats/stores/{id}/status` | Respaldo de FR-028 si no llega el permiso del aviso |

Las dos escrituras entran a la **lista blanca** del transporte de `internal/uber/`. Todo lo demás
—el `PUT` de menú sobre todo— sigue siendo imposible de disparar, y el test que parsea el AST lo
verifica.
