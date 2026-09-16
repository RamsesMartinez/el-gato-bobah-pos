# Phase 1 — Contrato de la API

**Feature** [020](../spec.md) · Prefijo `/api/v1` · Todo bajo
`RequireRole(domain.RoleAdmin, domain.RoleGerente)`, como el resto de `/admin/*`.

El cajero y el mesero **no** entran: esto administra el catálogo, no cobra. Es la misma vara que ya
usan `/admin/products` y `/admin/groups`.

Todas las rutas cuelgan de `/admin/platform-menus`.

---

## Conexiones

### `GET /admin/platform-menus/connections`

Las tiendas dadas de alta, con el estado de su última lectura. Es lo que pinta la pantalla al
abrir.

```json
{
  "connections": [
    {
      "id": 1,
      "platformId": 2,
      "platformName": "Uber Eats",
      "label": "Sucursal Centro",
      "externalStoreId": "3806dacf-965d-4b09-91b6-6d40ad5e15db",
      "active": true,
      "credentialsConfigured": true,
      "lastRead": {
        "id": 88,
        "status": "ok",
        "startedAt": "2026-09-14T14:02:11Z",
        "finishedAt": "2026-09-14T14:02:14Z",
        "itemCount": 222
      }
    }
  ]
}
```

- **`lastRead: null`** significa **nunca se ha leído**, y la pantalla lo dice así. No es «sin
  diferencias» (FR-003).
- **`credentialsConfigured`** es un booleano derivado del entorno. **Nunca** viaja el secreto ni
  una parte de él (FR-022). `false` → «esta tienda no está conectada», no una comparación vacía.
- `status` ∈ `en_curso` · `ok` · `fallida`.

### `POST /admin/platform-menus/connections`

Da de alta una tienda. **El `store_id` se captura aquí, no en el entorno** — una empresa tendrá
varias sucursales.

```json
{ "platformId": 2, "externalStoreId": "3806dacf-…", "label": "Sucursal Centro" }
```

`201` con la conexión. `409` si ya existe esa tienda para esa plataforma y empresa (la llave única
de `platform_connections`). `422` con `domain.ErrValidation` si el `externalStoreId` viene vacío o
pasa de 200 caracteres.

**En pantalla ese campo NO se llama `externalStoreId` ni «External Store ID».** Es «ID de tienda en
Uber Eats», y el «de dónde lo saco» va detrás de un icono de ayuda, como el interruptor de impresión
automática en `PrintSettingsPage`. Poner el nombre del campo de la API delata el internal y la
constitución lo prohíbe.

### `DELETE /admin/platform-menus/connections/{id}`

Da de baja la tienda. **Se lleva sus lecturas y su emparejamiento** (`on delete cascade`), o sea
hasta 65 decisiones manuales de una sesión completa.

Por eso **no vive en el encabezado de uso diario**, junto al botón de leer: vive en la
configuración de la conexión, y su diálogo dice **cuántas parejas se pierden** antes de confirmar.
Es la regla de separar las acciones destructivas de las frecuentes.

---

## Lectura

### `POST /admin/platform-menus/connections/{id}/read`

Dispara una lectura. **Responde de inmediato**, no espera al menú (SC-006).

`202 Accepted`:

```json
{ "readId": 89, "status": "en_curso", "startedAt": "2026-09-14T15:10:02Z" }
```

- La lectura corre en una goroutine con **su propio `context.WithTimeout`**, no el del request, que
  muere al responder.
- `409` si ya hay una lectura `en_curso` para esa conexión: dos lecturas simultáneas de la misma
  tienda gastan dos tokens para escribir la misma foto.
- `412` si la plataforma no tiene credenciales configuradas (`ErrPlataformaSinCredenciales`).
- Limitado por usuario con `rateLimitUser`, como ya se hace con los precios por plataforma: cada
  lectura es una llamada a un tercero con cuota, y **Uber invalida el token más viejo a partir del
  101 en una hora**.

### `GET /admin/platform-menus/connections/{id}/reads`

Las últimas lecturas, para que la pantalla vea cuándo se leyó y qué pasó.

```json
{
  "reads": [
    { "id": 89, "status": "fallida", "startedAt": "…", "finishedAt": "…",
      "failureKind": "tiempo_agotado" }
  ]
}
```

`failureKind` es **una clase cerrada**, nunca el mensaje de la plataforma: `sin_credenciales` ·
`auth_rechazada` · `tiempo_agotado` · `respuesta_invalida` · `menu_vacio`. El front tiene el
diccionario a español; el servidor no manda prosa.

---

## Emparejamiento

### `GET /admin/platform-menus/connections/{id}/pairing`

Las dos listas lado a lado, con las propuestas ya calculadas.

```json
{
  "readAt": "2026-09-14T14:02:14Z",
  "items": [
    { "externalId": "Chamoyada_de_Mango", "name": "Chamoyada de Mango 🥭",
      "priceCents": 9900, "available": true,
      "link": { "productId": 161, "productName": "Chamoyada", "confirmed": false } },
    { "externalId": "Dedos_de_queso", "name": "Dedos de queso",
      "priceCents": 7500, "available": true, "link": null }
  ],
  "unlinkedProducts": [ { "id": 487, "name": "Papas Fritas - Corte Gajo 270g" } ]
}
```

- **`confirmed: false` es una propuesta y se pinta distinta** (FR-011). Aceptada en silencio
  produciría comparaciones falsas que nadie puede auditar — y medido, el nombre exacto acierta
  **6 de 65**.
- `link: null` cuando no hay ni propuesta. **Nunca se empareja por aproximación** (FR-011, US2 ac.2).
- Si dos productos del POS se llaman igual, **ninguno se propone** y se dice por qué: la propuesta
  automática no puede elegir (edge case del spec).
- `410` si no hay ninguna lectura `ok`: no se empareja contra una foto que no existe.

**El parámetro `kind` decide qué se está emparejando** (FR-014a): `platillo` por omisión, `opcion`
para los ingredientes. Son dos pasadas de la misma pantalla y **el orden importa**: primero los
platillos, después los ingredientes que cuelgan de los ya emparejados — emparejar un ingrediente de
un platillo que nadie ha resuelto es trabajo que puede sobrar.

Con `kind=opcion`, `unlinkedProducts` trae **opciones de modificador del POS**, no productos. El
nivel `grupo` existe en el modelo y **esta feature no lo expone**.

### `PUT /admin/platform-menus/connections/{id}/links/{externalId}`

Confirma o cambia una pareja.

```json
{ "localId": 161, "localKind": "producto", "kind": "platillo" }
```

- **`kind`** es qué es del lado de **la plataforma**: `platillo` u `opcion`.
- **`localKind`** es qué es del lado de **acá**, y hoy solo acepta `producto` cuando `kind` es
  `platillo`, y `opcion_de_modificador` cuando `kind` es `opcion`. Existe por FR-014b: el día que
  haya inventario de insumos, un platillo podrá corresponder a una receta. Un `localKind`
  desconocido se rechaza con `ErrValidation`, nunca cae a un default.
- Un `kind` que no case con el nivel del `externalId` en la última lectura se rechaza: guardar una
  opción como si fuera platillo produce un mapeo que nunca empata y **nadie ve un error**.

`200`. `409` si ese `externalId` ya está emparejado con otro producto **y** no se mandó
`replace: true` — la PK lo impide de todos modos (FR-012), y el 409 es para que la pantalla lo
explique en vez de fallar con un error de base.

**`404` si el `externalId` no existe en la última lectura `ok` de esa conexión.** No es una
validación de cortesía: como `platform_item_links` no tiene FK hacia `platform_menu_items` —a
propósito, para que podar lecturas no borre parejas— **nada en la base impide guardar una pareja
contra un id que no existe arriba**. El `{externalId}` viaja url-encoded y los ids de Uber traen
acentos y emoji (`Chamoyada_de_Mojit🌿`, 59 de ellos truncados a 20 caracteres): un desajuste de
codificación escribiría una fila que jamás empata, el `PUT` respondería `200`, y la pantalla de
emparejamiento seguiría diciendo «sin pareja» **sin que nadie vea un error**. La validación en
`app` es lo que sustituye a la FK que deliberadamente no existe.

El `{externalId}` **se guarda sin normalizar** (FR-020): recortarlo o normalizarlo rompe el
emparejamiento contra la siguiente lectura.

### `DELETE /admin/platform-menus/connections/{id}/links/{externalId}`

Deshace la pareja (FR-013). `204`. La comparación lo refleja en la siguiente petición.

---

## Comparación

### `GET /admin/platform-menus/connections/{id}/differences`

La feature.

```json
{
  "readAt": "2026-09-14T14:02:14Z",
  "stale": false,
  "differences": [
    { "kind": "precio", "externalId": "Bobah_Tea", "platformName": "Bobah Tea",
      "productId": 140, "productName": "Bobah Tea",
      "platformPrice": 110.00, "catalogPrice": 94.50 },
    { "kind": "disponibilidad", "externalId": "Crepa_Ferrero", "platformName": "Crepa Ferrero",
      "productId": 331, "productName": "Ferrero",
      "platformAvailable": false, "catalogActive": true },
    { "kind": "solo_en_plataforma", "externalId": "Dedos_de_queso",
      "platformName": "Dedos de queso" },
    { "kind": "solo_en_catalogo", "productId": 487,
      "productName": "Papas Fritas - Corte Gajo 270g" }
  ],
  "unpaired": 109
}
```

- **`readAt` va siempre** (FR-002, SC-005). Sin él la pantalla no puede decir de cuándo es.
- **`stale`** lo calcula el servidor contra la retención. Una diferencia de hace tres semanas no es
  una diferencia: es una foto vieja.
- **Los que coinciden NO aparecen** (FR-018). La pantalla lista lo que difiere, no el catálogo.
- **`precio` y `disponibilidad` son clases distintas** (FR-016): se arreglan en lugares distintos.
- **`unpaired`** es cuántos items todavía no tienen pareja.
- **`kind` se pide por parámetro y la pantalla NO abre con las cuatro clases juntas.** Por omisión
  pide `precio` y `disponibilidad`, que es lo accionable; `solo_en_plataforma` y `solo_en_catalogo`
  viven en otra pestaña con el conteo en la etiqueta. Con 174 productos activos abajo y 65
  platillos arriba, la mayoría de los 109 «solo en un lado» **nunca se van a emparejar a
  propósito** —muchos nunca se publicaron— así que mezclarlos ahoga todos los días lo que sí hay
  que corregir. Un `kind` desconocido se rechaza con `ErrValidation`, nunca cae a un default en
  silencio.
- **Ninguna respuesta propone una acción destructiva** (FR-019). No hay `suggestedAction`, no hay
  «borrar». Un platillo que solo existe arriba puede ser deliberado.
- `catalogPrice` es el precio **para esa plataforma** (FR-017): la excepción de
  `product_platform_prices` si existe, y si no `base × (1 + markup)`. Hoy en producción hay **cero
  excepciones**, así que es siempre la fórmula.
- **Los precios viajan en pesos** con dos decimales, ya convertidos desde los centavos de la
  plataforma por la única función que hace esa conversión.
- `409` si la última lectura fue `fallida`: se dice que falló y desde cuándo no hay dato fresco.
  **Nunca** se devuelve la lectura anterior como si fuera de hoy (FR-004).

---

## Lo que NO existe en este contrato

No hay ninguna ruta que publique, actualice, suspenda o borre nada **en la plataforma**. Los
`DELETE` de arriba borran filas nuestras.

La garantía no vive en este documento: vive en el `http.RoundTripper` de `internal/uber`, que
rechaza todo verbo distinto de `GET` antes de abrir el socket, y en la prueba que parsea el AST del
paquete y falla nombrando la operación si alguien agrega una (FR-006, FR-007, US4).

## Errores, mapeados en un solo lugar

Sentinels de `domain`, traducidos a HTTP **solo** en `httpapi.Error` vía `errors.Is`, como manda el
principio II:

| Sentinel | HTTP |
| --- | --- |
| `ErrValidation` | 422 |
| `ErrPlataformaSinCredenciales` | 412 |
| `ErrLecturaEnCurso` | 409 |
| `ErrSinLecturaValida` | 410 |
| `ErrParejaOcupada` | 409 |
| `ErrConexionDuplicada` | 409 |
