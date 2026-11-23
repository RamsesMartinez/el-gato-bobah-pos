# Quickstart: validar el menú público en local

## Requisitos

- `make start` del worktree arriba, con un respaldo restaurado (`make db-restaurar`).
- Puertos del worktree en `.env.worktree.local` (API `80NN`, POS `30NN`, menú `87NN`, medios `93NN`).

## 1. API

```bash
cd server && go build ./... && go test ./...
make test-integration   # migración 0123, aislamiento, campos públicos, slugify SQL = Go
curl -s localhost:80NN/api/v1/public/menus/el-gato-bobah | jq '.kind, (.categories|length)'
curl -s -o /dev/null -w '%{http_code}\n' localhost:80NN/api/v1/public/menus/no-existe   # 404
```

Esperado: `"menu"` y > 0 categorías; ninguna llave fuera de [contracts/api.md](contracts/api.md).

## 2. Menú en el borde (local)

```bash
cd web && bun run dev:menu   # wrangler pages dev en :87NN con API_ORIGIN y MEDIA_ORIGIN locales
```

- `http://localhost:87NN/el-gato-bobah` → menú; `…/q/<token>` → 302; nombre anterior → 302.
- `curl -s localhost:87NN/el-gato-bobah | grep -c '<script'` → 0.
- `curl -s localhost:87NN/el-gato-bobah | grep 'og:'` → título, descripción, imagen, url.

## 3. Rendimiento

```bash
bunx lighthouse http://localhost:87NN/el-gato-bobah --only-categories=performance \
  --output=json --output-path=test-results/lh-menu.json --chrome-flags="--headless"
jq '.audits["largest-contentful-paint"].numericValue, .audits["cumulative-layout-shift"].numericValue' test-results/lh-menu.json
```

Esperado: LCP < 2500 ms y CLS < 0.1 con el perfil móvil por omisión (4G lenta, CPU 4×). Medir con
fotos: subir al menos 20 fotos de productos en local antes de medir.

**Medido el 2026-11-23** (Lighthouse 12, perfil móvil por omisión, contra `dev:menu` local con el
respaldo de producción restaurado: 183 productos, 8 categorías principales, 24 fotos de relleno con
el peso de una foto real, ~42 KB la de 400 px):

| Corrida | LCP | CLS | FCP | TBT | Bytes | Puntaje |
|---|---|---|---|---|---|---|
| 1 | 1,957 ms | 0 | 652 ms | 0 ms | 1,016 KB | 0.99 |
| 2 | 1,955 ms | 0 | 618 ms | 7 ms | 1,016 KB | 0.99 |
| 3 | 1,956 ms | 0 | 618 ms | 0 ms | 1,016 KB | 0.99 |

El elemento LCP es la primera foto (400 px). No medido: la red real de Cloudflare ni el tiempo de un
fallo de caché del borde contra la VM (el local no tiene ese salto).

## 4. Ajustes › Venta en web

En `http://localhost:30NN/negocio?seccion=web` a 1280×854: ver dirección, copiar, abrir,
descargar PNG/SVG, imprimir tarjeta, cambiar el nombre (ver la vista previa normalizada), cambiar el
color y comprobar en el menú (≤ 2 min, o al momento con `dev:menu` sin caché).

## 5. Historial

Cambiar `a → b → c → a`: `/b` y `/c` llevan a `/a` en un salto; `/q/<token>` también.

## 6. e2e

```bash
cd web && E2E_BASE_URL=http://localhost:30NN E2E_API_URL=http://localhost:80NN/api/v1 \
  E2E_MENU_URL=http://localhost:87NN … bun run e2e -- menu-publico
```

## 7. Prueba con alguien nuevo (2026-11-23, local)

| Persona | Resultado | Lo que se corrigió |
|---|---|---|
| Dueña de una fonda (1280×854 y 2K), comparte por WhatsApp | 4/5. Copió el enlace, descargó el QR, imprimió la tarjeta, cambió nombre y color y lo regresó. Buscó primero en Menú y en Plataformas | «Copiado» movía los botones (ancho fijo); «0 con precio web» no se entendía (ahora «con un precio distinto al de mostrador», y solo si hay); el resumen de la sección dice «Menú en línea: enlace, código QR…»; la dirección escrita a mano («/Doña Lupe») lleva a `/dona-lupe` |
| Cliente en celular (360×780) | 3/5. Encontró un Buldak en 2.5 s y lo de la última categoría en 1.4 s | La foto «grande» salía de 96 px; el salto a una categoría tardaba ~2 s (ahora inmediato); un renglón sin foto ni descripción parecía tocable |

Verificado decodificando el PNG descargado: el QR lleva a `/q/<código>` antes y después de cambiar el
nombre, y ese código redirige al nombre vigente.

Fuera de la v1, anotado: buscador de productos, marcar en la barra la categoría donde se está (pide
JavaScript), y abrir WhatsApp directo desde «Compartir» en la computadora (ahí copia). Un producto del
catálogo real, «Pedido de plataforma (genérico)», sale en el menú porque está activo con «Web»
encendido: se apaga en su pestaña «Venta en web» si el negocio no lo quiere publicar.
