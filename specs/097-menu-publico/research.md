# Research: Menú público v1

La revisión de arquitectura se hizo **antes** del plan (la pidió el dueño en el #316). Puntuó cuatro
formas de servir el menú y fijó caché, resolución sin sesión, límite por IP, redirecciones y
presupuesto de carga. Lo que sigue es su resultado, con lo verificado en el código.

## R1. Cómo se sirve el menú

**Decisión**: HTML renderizado en el borde por una **Pages Function** en un **proyecto de Pages
propio** (`el-gato-bobah-pos-menu`, dominio `menu.elgatobobah.com`), que pide un JSON público a la
API. Cero JavaScript en el navegador, CSS en línea, sprite de ilustraciones en línea, fuentes del
sistema.

| Opción | Rendim. | Seguridad | Costo/oper. | Simplicidad | Puertas #328/#330/#331/#333 | Total |
|---|---|---|---|---|---|---|
| **A. HTML en el borde (Function propia)** | 5 | 4 | 4 | 3 | 5 | **21** |
| D. La API Go renderiza y Cloudflare cachea | 4 | 4 | 5 | 4 | 3 | 20 |
| B. SPA React estática | 2 | 4 | 4 | 2 | 3 | 15 |
| C. Dentro del POS | 1 | 2 | 3 | 2 | 2 | 10 |

- **B** no resuelve la vista previa de WhatsApp (su rastreador no ejecuta código): haría falta una
  Function de todos modos, más ~45 KB gz de JS y un fetch antes de pintar; LCP estimado 3–4 s.
- **C** arrastra el service worker del POS (que solo se actualiza cuando alguien acepta), su CSP y
  su paquete; tampoco resuelve la vista previa.
- **D** queda cerca, pero deja al e2-micro como origen del HTML (si se cae, se cae el menú), pide un
  host nuevo en Caddy y complica los subdominios del #331. Queda como plan B.
- **A** sirve una copia vieja si la API falla, separa plantilla/datos/marca en TypeScript probado
  con vitest, y el #331 (subdominios) solo cambia cómo la Function lee el nombre de la URL.

## R2. Caché e invalidación

**Decisión**: TTL corto en el borde, sin purga.

| Qué | Dónde | Vida |
|---|---|---|
| HTML de `/<nombre>` | Cache API de la Function; llave = URL canónica (sin query) + id del build | Fresca 60 s; se guarda 24 h para servirla si la API falla |
| 404 y redirecciones | Cache API | 60 s |
| JSON de la API | No se cachea en la API; `Cache-Control: public, max-age=30` | 30 s |
| Fotos | `media.elgatobobah.com`, ya `immutable` | 1 año |
| Navegador | `Cache-Control: public, max-age=60` | 60 s |

- `catalog_versions` (0093) **no sirve de ETag**: no se mueve con las existencias, así que no vería
  un «Agotado». El TTL cubre las dos cosas con una sola regla: un cambio se ve en ≤ 2 min (60 s del
  borde + 60 s del navegador), dentro de SC-006.
- El id del build va en la llave: la Cache API sobrevive a los despliegues, y sin él una plantilla
  nueva tardaría un TTL en verse.
- La Cache API **no funciona en `*.pages.dev`**: el ambiente de pruebas usa `menu-dev.elgatobobah.com`.
- Costo de un fallo de caché: una petición a la API por negocio, por minuto y por colo.

## R3. Resolver el nombre sin sesión

**Decisión**: una **vista** `public_menu_routes` cuyo dueño es un rol nologin
`gatobobah_public_menu` con política `using (true)` solo para él (patrón de `candidatas_del_aviso`,
0073). Proyecta nombre, empresa, si es el vigente, el nombre vigente y el código del QR. **Nunca**
`companies.slug` (la mitad de la credencial `usuario@slug`). Con la empresa resuelta, el servicio
abre `WithTenant` y todo lo demás corre bajo RLS como cualquier petición.

- Alternativas: `security definer` (no la tipa sqlc si devuelve filas; corre como owner); leer como
  owner con `store.Q` (salta RLS para todo, no solo para esta proyección).

## R4. Nombres, historial y QR

- Un nombre usado por un negocio **no se libera nunca** a otro: `unique` global sobre todos los
  nombres, vigentes y retirados. Si se liberara, un enlace viejo de WhatsApp o un QR impreso podría
  caer en el menú de otro negocio.
- Regresar a un nombre propio anterior reactiva esa misma fila.
- El QR apunta a `/q/<código>`: 12 caracteres hexadecimales aleatorios (48 bits), no el id de la
  empresa, para que no se puedan recorrer.
- **302, no 301**, con `Cache-Control: public, max-age=300`. Un 301 se queda en el navegador sin
  caducidad: si el negocio regresa a un nombre anterior, el navegador seguiría su 301 viejo y
  entraría en un ciclo. `<link rel=canonical>` apunta al nombre vigente.
- El nombre inicial sale del **nombre del negocio**, no del slug de login.
- Reservados: `q`, `api`, `_m`, `assets`, `static`, `admin`, `www`, `menu`, `login`, `ayuda`,
  `soporte`, `robots-txt`… (lista completa en data-model).

## R5. Límite por IP cuando quien llama a la API es la Function

| Variante | ¿Limita al cliente real? | Veredicto |
|---|---|---|
| Solo el límite de la API | No: vería IPs de salida de Cloudflare y bloquearía colos enteros | Descartada sola |
| Regla de rate limiting de Cloudflare en `menu.elgatobobah.com` | Sí, antes de invocar la Function (protege también la cuota de invocaciones) | **Primera barrera** (la configura el orquestador) |
| Secreto Function→API que reenvía la IP del cliente | Sí, en la API, para los fallos de caché | **Segunda barrera** |

- En la API: con `X-Menu-Edge-Key` válido (comparación de tiempo constante), la llave del límite es
  `X-Menu-Client-IP`; sin él o con uno inválido, `rateKeyIP` como cualquier ruta. Un cliente que
  mande la IP falsa sin el secreto se limita por su IP real (test adversarial).
- `MENU_EDGE_KEY` es opcional: sin él la API arranca y limita por `rateKeyIP` (lo dice un log al
  arrancar). Si está, mínimo 32 caracteres (`config.Validate`).
- La Function ignora la query string y valida la forma del nombre antes de llamar a la API: un
  nombre imposible responde 404 sin gastar una petición al origen. `_routes.json` deja fuera de la
  Function los archivos estáticos.

## R6. Vista previa al compartir (OpenGraph)

- `og:title` = nombre del negocio; `og:description` = «Menú de X: N productos. Precios y fotos.»;
  `og:image` = la foto **chica** (400 px, ≤ 100 KB) del primer producto del menú que tenga foto; sin
  ninguna, una imagen PNG por omisión del proyecto del menú; `og:url` canónica; `og:type=website`.
- **No verificado**: que WhatsApp muestre WebP en Android e iOS. Si no, el siguiente paso es una URL
  de transformación de Cloudflare a JPEG sobre el dominio de medios (no requiere tocar #325). Se
  prueba con un enlace real después de subir.

## R7. Qué viaja en el JSON público

| Sí | No |
|---|---|
| Nombre del negocio, nombre vigente del menú, paleta, plantilla; por categoría: id, nombre, familia, foto; por producto: id, nombre, descripción, precio efectivo, foto, agotado | Costos, recetas, existencias, `allow_oversell`, precio de mostrador cuando hay precio web, precios de plataforma, slug, `company_id`, `system_kind`, fechas |

Los ids de producto y categoría ya aparecen en las rutas de las fotos y el #328 los necesitará;
esconderlos no protege nada.

## R8. Presupuesto de carga (Lighthouse móvil: ~150 ms RTT, ~1.6 Mbps, CPU 4× más lenta)

- HTML ≤ 35 KB comprimido con ~180 productos; CSS en línea ≤ 12 KB; 0 KB de JS; sin web font; sprite
  en línea (4.7 KB).
- `preconnect` al dominio de medios; las primeras 4 fotos `fetchpriority=high` sin lazy; el resto
  `loading=lazy` con `width`/`height`.
- El LCP debe ser texto (encabezado con el nombre del negocio) o una foto chica, nunca una portada
  a pantalla completa.
- `content-visibility:auto` por categoría para no maquetar 180 tarjetas con la CPU lenta.
- Estimado 1.8–2.1 s; se **mide** con Lighthouse (perfil móvil por omisión) contra la Function local
  y el catálogo restaurado.

## R9. Generar el QR

**Decisión**: generarlo en el navegador, en Ajustes, con `uqr` (codificador QR pequeño, sin
dependencias, MIT), cargado solo al abrir Venta en web. SVG directo del codificador; PNG dibujando
ese SVG en un canvas; tarjeta imprimible con `window.print()` y CSS de impresión.

- Alternativas: escribir un codificador propio (~400 líneas con Reed-Solomon, sin beneficio frente a
  una librería auditada de 10 KB); generarlo en Go (también exige dependencia o código propio, y
  pondría trabajo de imagen en el e2-micro).

## R10. Color de marca

No existe color de marca por negocio: la paleta de Interfaz es por tableta. Se agrega
`public_menus.brand_palette` con los mismos ids de `PALETTES` (`brand`, `blue`, `teal`, `green`,
`cyan`, `purple`, `pink`, `orange`), por omisión `brand`. La Function traduce cada id a los mismos
tonos que usa el tema del POS (500/600/700/50). El #330 lo amplía a logo, portada y tipografía.

## R11. Clave de la función (#333)

Constante `domain.FeaturePublicMenu = "public_menu"`, sin tabla. Agregar la tabla de funciones por
paquete después cuesta lo mismo que hoy (principio VIII: sí se puede agregar después al mismo
costo).
