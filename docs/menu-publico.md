# Menú público en línea

Runbook del menú que cada negocio publica en `menu.elgatobobah.com/<nombre>` (spec 097, #316). El
porqué de cada decisión está en [specs/097-menu-publico/research.md](../specs/097-menu-publico/research.md).

## Cómo está armado

```text
celular ──► menu.elgatobobah.com (Cloudflare Pages: _worker.js)
              │  caché del borde: 60 s fresca, 24 h de respaldo si la API falla
              ▼
            api.elgatobobah.com/api/v1/public/menus/{nombre}   (sin sesión, límite por IP)
              │  resuelve el nombre con la vista public_menu_routes y lee bajo RLS
              ▼
            fotos: media.elgatobobah.com (R2, ver medios-r2.md)
```

| Ambiente | Proyecto de Pages | Dominio | API |
|---|---|---|---|
| Producción | `el-gato-bobah-pos-menu` (rama de producción `develop`) | `menu.elgatobobah.com` | `api.elgatobobah.com` |
| Pruebas | `el-gato-bobah-pos-menu-dev` (rama de producción `staging`) | `menu-dev.elgatobobah.com` | `api-dev.elgatobobah.com` |
| Local | `bun run dev:menu` (wrangler) | `localhost:87NN` | `localhost:80NN` |

- CI construye `web/dist-menu/` (`bun run build:menu`: lo estático de `web/menu/public` y la
  Function empaquetada como `_worker.js`) y lo sube con `wrangler pages deploy` en los jobs
  `deploy-menu` (main) y `deploy-menu-staging` (develop).
- Sin JavaScript en el navegador. La Cache API **no funciona en `*.pages.dev`**: por eso cada
  ambiente tiene su dominio propio.
- Un cambio del catálogo, de un precio o un «Agotado» se ve en **2 minutos como máximo** (60 s del
  borde + 60 s del navegador). No hay purga manual.

## Variables

En el proyecto de Pages (Settings › Variables and Secrets, entorno Production):

| Variable | Tipo | Valor |
|---|---|---|
| `API_ORIGIN` | texto | `https://api.elgatobobah.com` (pruebas: `https://api-dev.elgatobobah.com`) |
| `MENU_EDGE_KEY` | secreto | El mismo valor que en la API de ese ambiente |
| `MEDIA_ORIGIN` | texto, opcional | Las fotos que trae el menú se permiten solas en la CSP; esto agrega uno más |

En el `deploy/.env` de cada VM:

| Variable | Producción | Pruebas |
|---|---|---|
| `MENU_BASE_URL` | `https://menu.elgatobobah.com` | `https://menu-dev.elgatobobah.com` |
| `MENU_EDGE_KEY` | `openssl rand -hex 32` | otro distinto |

Sin `MENU_BASE_URL` la API arma el enlace y el QR con la de producción. Sin `MENU_EDGE_KEY` arranca
igual, deja `menu_edge_key_missing` en el journal y limita por la IP de Cloudflare en vez de la del
visitante.

## Límite de peticiones

1. **Cloudflare**, antes de invocar la Function: Security › WAF › Rate limiting rules, condición
   `http.host in {"menu.elgatobobah.com" "menu-dev.elgatobobah.com"}`, por IP, 60 peticiones en
   10 s, Block 10 s. Protege también la cuota diaria de invocaciones de Pages Functions.
2. **La API**, en los fallos de caché: 120 por minuto y por visitante. Con `MENU_EDGE_KEY` la
   Function le reenvía la IP del visitante (`X-Menu-Client-IP`); sin el secreto ese encabezado se
   ignora.

## Rotar `MENU_EDGE_KEY`

Las dos mitades tienen que coincidir; durante el cambio, la API limita por la IP de Cloudflare (no
se cae nada):

1. `openssl rand -hex 32`.
2. Ponerlo en el `deploy/.env` de la VM y desplegar (`make deploy-image`); verificar `readyz`.
3. Ponerlo como secreto `MENU_EDGE_KEY` del proyecto de Pages y volver a desplegar el menú (el
   último job `deploy-menu` › Re-run, o `bunx wrangler pages deploy`).

## Nombres y QR

- El nombre lo cambia el negocio en Ajustes › Venta en web. Un nombre usado **nunca** pasa a otro
  negocio; los anteriores redirigen al vigente con **302** (no 301: un 301 se queda en el navegador y
  regresar a un nombre anterior dejaría un ciclo).
- Tope de 20 nombres anteriores por negocio; con el tope lleno solo puede volver a uno de ellos.
  Liberar un nombre es una operación a mano en la base, como owner, y rompe los enlaces viejos de
  quien lo tenía.
- El QR codifica `menu.elgatobobah.com/q/<código>`: el código no cambia nunca. **Cuando el producto
  se mude de dominio (#331), `menu.elgatobobah.com` tiene que seguir redirigiendo para siempre**, o
  los QR impresos dejan de funcionar.

Consultas de solo lectura (producción):

```sql
begin read only;
select name, company_id, is_current, current_name, company_active from public_menu_routes order by company_id, is_current desc;
```

## Sin verificar al publicar la v1

- Que la vista previa de WhatsApp muestre la foto (las fotos son WebP). Si no la muestra, el
  siguiente paso es una transformación de Cloudflare a JPEG sobre el dominio de medios.
- La regla de límite de peticiones en el plan de la cuenta.
