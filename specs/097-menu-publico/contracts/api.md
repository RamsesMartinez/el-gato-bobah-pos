# Contratos: Menú público v1

## API pública (sin sesión)

Fuera del grupo de tenant, como el webhook. Límite por IP `publicMenuMax = 120` por minuto (ver
research R5 para la llave). Respuestas con `Cache-Control: public, max-age=30`. CORS: ninguno (la
llama la Function, servidor a servidor).

### `GET /api/v1/public/menus/{name}`

`name` se valida por forma (`domain.ValidMenuNameShape`) antes de tocar la base: una forma imposible
responde 404 igual que uno inexistente.

**200 — menú vigente**

```json
{
  "kind": "menu",
  "name": "el-gato-bobah",
  "qrPath": "/q/3f9a0c2b7d1e",
  "business": { "name": "El Gato Bobah" },
  "brand": { "palette": "brand" },
  "template": "classic",
  "categories": [
    {
      "id": 12, "name": "Bubble tea", "family": "tea",
      "imageUrl": "https://media.elgatobobah.com/2/categories/12/ab…-1200.webp",
      "products": [
        { "id": 301, "name": "Taro", "description": "Con perlas de tapioca",
          "price": "65", "imageUrl": null, "soldOut": false }
      ],
      "subcategories": [
        { "id": 31, "name": "Buldak", "products": [ … ] }
      ]
    }
  ]
}
```

- Dos niveles, como el catálogo (spec 085): `categories` son las principales (barra, columna y
  mosaico); `subcategories` las hijas, que se pintan como subtítulos dentro de su principal. Los
  productos de la principal van antes que los de sus hijas.

- `categories` siempre es arreglo (vacío si no hay nada en web); `products` nunca es `null`.
- Solo categorías con al menos un producto (propio o de una hija) y solo hijas con productos. Orden:
  `sort_key`, luego nombre; igual hijas y productos. `subcategories` nunca es `null`.
- `price`: decimal como texto, ya con `Round2`.
- `family`: `null` = genérica. `imageUrl`: la de 1200; la Function deriva la de 400.

**200 — nombre anterior**

```json
{ "kind": "moved", "name": "nombre-vigente" }
```

**404** `{ "code": "NOT_FOUND" }` — nombre inexistente, forma imposible o empresa inactiva (misma
forma y mismo cuerpo en los tres casos).

**429** `{ "code": "RATE_LIMITED" }`.

### `GET /api/v1/public/menus/q/{token}`

`token` validado por forma (`^[0-9a-f]{12}$`). **200** `{ "kind": "moved", "name": "nombre-vigente" }`
o **404** igual que arriba.

### Encabezados de la Function hacia la API

| Encabezado | Valor |
|---|---|
| `X-Menu-Edge-Key` | Secreto compartido `MENU_EDGE_KEY` |
| `X-Menu-Client-IP` | `CF-Connecting-IP` del visitante |

Sin el secreto correcto, `X-Menu-Client-IP` se ignora.

## API de Ajustes (con sesión)

Permiso `business.configure` (`RequirePermission`).

### `GET /api/v1/admin/public-menu`

Crea el menú si falta (empresa nacida durante el traslape).

```json
{
  "businessName": "El Gato Bobah",
  "name": "el-gato-bobah",
  "url": "https://menu.elgatobobah.com/el-gato-bobah",
  "qrUrl": "https://menu.elgatobobah.com/q/3f9a0c2b7d1e",
  "palette": "brand",
  "previousNames": ["gatobobah-viejo"]
}
```

### `PATCH /api/v1/admin/public-menu`

```json
{ "name": "donalupe", "palette": "green" }
```

Los dos campos opcionales; ausente = sin cambio. `name` se normaliza en el servidor con la misma
regla que la pantalla (la pantalla solo la muestra).

- **200** con el mismo cuerpo que el GET.
- **400** `VALIDATION` — forma inválida (longitud, caracteres tras normalizar) o paleta desconocida.
- **409** `MENU_NAME_UNAVAILABLE` — reservado, de otra empresa o ganado por otra en la carrera.
- **409** `MENU_NAME_LIMIT` — la empresa ya tiene 20 nombres anteriores; puede volver a uno de ellos.
- **403** sin el permiso.

## La Function (`menu.elgatobobah.com`)

| Ruta | Respuesta |
|---|---|
| `/<nombre>` vigente | 200 HTML del menú |
| `/<nombre>` anterior | 302 a `/<vigente>`, `Cache-Control: public, max-age=60` (corto: si el negocio vuelve a ese nombre, el navegador no se queda con un salto viejo) |
| `/q/<token>` | 302 a `/<vigente>`, `max-age=60` |
| `/<nombre>/` o con mayúsculas | 302 a `/<nombre en minúsculas, sin diagonal final>` |
| `/` | 200 página breve «Menús en línea» sin lista de negocios |
| inexistente | 404 HTML «Este menú no existe» |
| API caída sin copia | 503 HTML «No pudimos cargar el menú» con `Retry-After: 30` |

Encabezados del HTML: `Content-Security-Policy: default-src 'none'; img-src 'self' data:
https://media.elgatobobah.com; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none';
frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`,
`Cache-Control: public, max-age=60`.

### Variables del proyecto de Pages

| Variable | Producción | Pruebas | Local |
|---|---|---|---|
| `API_ORIGIN` | `https://api.elgatobobah.com` | `https://api-dev.elgatobobah.com` | `http://localhost:80NN` |
| `MEDIA_ORIGIN` | `https://media.elgatobobah.com` | `https://media-dev.elgatobobah.com` | `http://localhost:93NN` |
| `MENU_EDGE_KEY` (secreto) | igual al de la API de producción | igual al de pruebas | vacío |
