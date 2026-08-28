# Phase 0 — Research: Visualizador e impresión del ticket

**Feature**: 001-ticket-preview-print · **Date**: 2026-08-27

Nueve decisiones. D1–D6 salieron del diseño; D7–D9 salieron de tener el papel en la mano.
Ninguna quedó como NEEDS CLARIFICATION.

## D1 — Transporte de impresión: iframe, no popup

**Decisión**: montar el HTML del ticket en un `<iframe srcdoc>` y llamar
`iframe.contentWindow.print()`.

**Rationale**:

- El `window.open` que usa hoy [printReceipt.ts](../../web/src/utils/printReceipt.ts) muere con
  cualquier bloqueador de popups y regresa `null` — la función ya tiene un `if (!w) return` que
  **falla en silencio**: el operador toca imprimir y no pasa absolutamente nada.
- En tablet, un popup abre una pestaña nueva encima del POS; el operador tiene que regresar a mano.
- El iframe vive dentro del modal, así que el mismo nodo que se ve es el que se imprime (ver D2).
- **`print()` se llama desde el padre**, con `iframe.contentWindow.print()`. Nunca con un `<script>`
  dentro del `srcdoc`: la CSP de producción es `script-src 'self'`
  ([web/public/_headers](../../web/public/_headers)) y bloquearía ese script — pero en local, donde
  Vite no manda CSP, funcionaría. Sería un fallo que solo aparece en el sitio publicado.
- El documento del `srcdoc` es **same-origin**, así que todo dato de una persona que entre al ticket
  —incluidos los campos del negocio— pasa por `esc()`. Un `<img onerror>` en el nombre del negocio
  correría con acceso al token.

**Alternativas descartadas**:

| Alternativa | Por qué no |
| --- | --- |
| `window.open` + `document.write` (lo actual) | Bloqueadores de popups; falla silenciosa; roba el foco en tablet |
| `@media print` sobre la página del POS | Habría que ocultar toda la app con reglas de impresión. Cada pantalla nueva puede romper el ticket sin que nadie se entere, y no da vista previa fiel |
| Generar un PDF y abrirlo | Agrega dependencia, un paso más para el operador y el visor de PDF vuelve a meter su propio diálogo |

## D2 — Fidelidad entre vista previa y papel: un solo documento

**Decisión**: `buildReceiptHtml(order, businessInfo, opts)` sigue siendo la **única** definición del
contenido. La vista previa muestra exactamente ese HTML dentro del iframe; imprimir es llamar
`print()` sobre ese mismo documento ya renderizado.

**Rationale**: FR-002 pide que sea *imposible* que diverjan. Con dos renders (uno React para la
pantalla, otro HTML para el papel) la divergencia es cuestión de tiempo: alguien agrega un campo en
uno y no en el otro. Con un solo documento, no hay dónde equivocarse.

**Consecuencia de diseño**: la vista previa se ve con el ancho real de 80mm (≈302 px a 96 dpi) y se
escala con `transform: scale()` para caber en 7". El `transform` es solo de pantalla: la impresión
usa el layout propio del documento del iframe y su `@page { size: 80mm auto }`, así que escalar la
vista **no** encoge el papel.

**Alternativa descartada**: componente React para la preview + HTML aparte para imprimir. Más
"idiomático", pero es justo el bug que FR-002 prohíbe.

## D3 — Dónde vive el logo: bytea en `business_settings`

**Decisión**: columnas `logo_bytes bytea`, `logo_mime text`, `logo_updated_at` en la fila que
`business_settings` ya tiene por empresa.

**Rationale**:

- El VPS es un e2-micro sin volumen persistente ni bucket. Escribir a disco del contenedor pierde el
  logo en cada `docker compose up -d` — viola FR-020 y SC-005.
- `business_settings` **ya está registrada** en la policy `tenant_isolation` de
  [0024_tenant_rls.sql](../../server/migrations/0024_tenant_rls.sql). Una tabla nueva habría que
  darla de alta a mano en RLS, y una tabla per-tenant sin policy es una fuga de datos entre empresas
  esperando a pasar.
- Postgres TOASTea el bytea fuera de la fila, así que la fila de ajustes no engorda. Las queries de
  sqlc nombran columnas explícitamente, así que `GetBusinessSettings` **no** arrastra la imagen.
- El backup de la base ya se lleva el logo; no hay un segundo artefacto que respaldar.

**Alternativas descartadas**:

| Alternativa | Por qué no |
| --- | --- |
| Disco del contenedor | Se pierde en cada deploy |
| Volumen docker | Un artefacto más que respaldar y montar, para 256 KB |
| S3 / bucket | Dependencia externa, credenciales y costo, para un archivo por negocio |
| Tabla `ticket_logo` aparte | Obliga a registrar RLS a mano; el beneficio (fila ligera) ya lo da nombrar columnas en las queries |

## D4 — Cómo llega el logo al ticket: data URI, resuelto antes de imprimir

**Decisión**: el front pide el logo una vez, lo convierte a data URI y lo incrusta en el HTML del
ticket. El logo por default se importa con `?inline` de Vite, que ya lo entrega como data URI.

**Rationale**: éste es el defecto que muerde en producción y no en la demo. Un
`<img src="/api/...">` dentro del documento que se va a imprimir dispara una carga de red; si
`print()` corre antes de que la imagen termine, **el papel sale con un hueco blanco donde iba el
logo** y nadie se entera hasta que el cliente lo tiene en la mano. Con data URI no hay carrera: el
documento es autocontenido en el momento en que existe.

**Y además la CSP lo obliga, no solo la carrera de carga.** La política de producción del front es
`img-src 'self' data:` ([web/public/_headers](../../web/public/_headers)), y en producción la API
vive en **otro dominio** (`api.elgatobobah.com`; el front está en Cloudflare Pages). Un
`<img src="https://api.elgatobobah.com/…">` dentro del ticket queda **bloqueado por CSP en el sitio
publicado y funciona perfecto en local**, donde Vite no manda CSP alguna. El data URI entra por
`data:` y no depende del origen. Quien "optimice" esto de vuelta a un `<img src>` remoto rompe
producción sin romper un solo test.

**Alternativas descartadas**: `<img src>` remoto esperando `img.decode()` — funciona, pero deja el
resultado a merced de un timeout de red justo en el flujo de cobro; y el mismo HTML deja de ser
autocontenido, que es lo que lo hace reutilizable para la fase 2.

## D5 — Validar la imagen por contenido, no por lo que diga el cliente

**Decisión**: `domain.ValidateLogo(data []byte) (mime string, err error)`, función pura. El tipo que
declara quien sube ni se recibe como parámetro: no hay forma de "olvidarse" de ignorarlo.

1. tamaño ≤ 256 KB,
2. tipo real detectado con `http.DetectContentType` sobre los primeros 512 bytes,
3. debe estar en la lista blanca **PNG / JPEG** —y solo esos dos: son los que la stdlib sabe
   decodificar. Aceptar WebP obligaría a sumar `golang.org/x/image` **solo para leer un header**, o
   a aceptar un archivo cuyas dimensiones no podemos acotar, que es justo lo que este punto cierra,
4. dimensiones ≤ 1024 px por lado, leídas con `image.DecodeConfig` (no decodifica la imagen
   completa: no hay bomba de descompresión que reventar 1 GB de RAM),

**Rationale**: principio V. El header del multipart lo escribe el atacante; el contenido no miente.
**SVG queda fuera a propósito**: es XML con `<script>` adentro, y un SVG servido desde nuestro origen
es XSS con acceso al `localStorage` donde vive el token. Al servir el binario va
`X-Content-Type-Options: nosniff` para que el navegador no lo reinterprete.

**Alternativa descartada**: confiar en la extensión o en el `Content-Type` del multipart. Es
exactamente el agujero que el principio V manda cerrar con un caso concreto, no con teoría.

## D6 — Forma de la subida: multipart, copiando el patrón que ya existe

**Decisión**: `POST` multipart con campo `file`, reusando la secuencia de
[`ExtractPurchaseDoc`](../../server/internal/httpapi/handlers_purchasedoc.go): `http.MaxBytesReader`
sobre el `Body` **antes** de `ParseMultipartForm`, y `RemoveAll()` diferido.

**Rationale**: ya está resuelto en el repo con el comentario que explica por qué el
`MaxBytesReader` es la cota real y el argumento de `ParseMultipartForm` solo dice cuánto se queda en
RAM. Copiar ese patrón es lo que pide el principio VI; inventar otro camino de subida sería un
segundo lugar donde equivocarse con los límites.

**Alternativa descartada**: JSON con la imagen en base64. Infla 33% el cuerpo y obliga a leerlo
entero antes de poder rechazarlo por tamaño.

## D7 — Legibilidad en térmica: negro puro y negritas, nunca grises

**Decisión**: el documento del ticket no declara ningún color que no sea `#000`, el cuerpo va en
negritas y la jerarquía se hace con tamaño y espaciado.

**Rationale**: se descubrió con el papel en la mano. El primer ticket impreso salió tan tenue que
no se distinguía. La causa no era la fuente: era que el CSS tenía `.muted { color: #333 }`, muy
razonable en pantalla. **La impresora es de 1 bit**: no imprime gris, lo aproxima con un patrón de
puntos salteados, y un trazo delgado se pierde entre esos puntos. Hay un test que falla si vuelve a
colarse un color distinto de negro.

**Consecuencia operativa aparte**: el driver de la POS-80 venía en `zjSoftFontMode`, que intenta
mapear el texto a fuentes internas de la impresora; lo que manda el navegador es una página
rasterizada, y en ese modo salía **papel en blanco**. Va en `zjGraphMode`. Eso es ajuste de máquina,
no de código, y por eso vive en el runbook y no aquí.

## D8 — La vista previa se ESCALA, no se encoge

**Decisión**: el iframe conserva sus 302 px (80mm) y el contenedor aplica `transform: scale()` para
caber en el hueco disponible.

**Rationale**: el primer intento le puso `width: 100%` al iframe, que en un diálogo más angosto
comprime el marco por debajo del ancho del documento y le mete **scroll horizontal**. En tablet eso
es doblemente malo: arrastrar esa barra es un toque perdido, y el arrastre lo interpreta el diálogo
como una interacción *fuera* y se cierra a media revisión. Por eso además el diálogo del ticket no
cierra al tocar fuera: tiene su botón.

El `transform` es solo de pantalla — la impresión usa el layout del documento del iframe y su
`@page`, así que escalar la vista no encoge el papel.

## D9 — El ticket de prueba sale marcado

**Decisión**: la pantalla de configuración imprime un pedido de muestra fijo, y el papel sale con
`** TICKET DE PRUEBA **`.

**Rationale**: sin la marca, un ticket de prueba que acaba en manos de un cliente parece una venta.
Es el mismo argumento que la marca de reimpresión. El pedido de muestra lleva **fecha fija** a
propósito: al ajustar la impresora se compara un papel contra el anterior, y una hora que cambia en
cada impresión estorba esa comparación.

**Alternativa descartada**: cobrar una venta de mentiras para ver el ticket. Ensucia los reportes y
el corte de caja con dinero que no existió.
