# Implementation Plan: Menú público v1 con enlace y QR

**Branch**: `097-menu-publico` | **Date**: 2026-11-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/097-menu-publico/spec.md`

## Summary

Cada negocio publica su menú en `menu.elgatobobah.com/<nombre>`. Un proyecto de Pages propio
(`el-gato-bobah-pos-menu`, como la consola) corre una Function que pide a la API un JSON público,
renderiza HTML sin JavaScript con una plantilla separada en plantilla + datos + marca, y lo guarda
60 s en el borde (24 h como respaldo si la API falla). La API resuelve el nombre sin sesión con una
vista de rol nologin (patrón 0073) y luego lee bajo RLS con la regla única de venta en web (#326),
ampliada con nombre, descripción, foto y categoría. Dos tablas nuevas guardan el nombre con su
historial (un nombre nunca pasa a otra empresa; los viejos redirigen con 302) y el código estable
del QR con la paleta de marca. En Ajustes › Venta en web, la dueña edita nombre y color, copia y
comparte el enlace y descarga el QR (PNG, SVG, tarjeta imprimible). Detalle de las decisiones en
[research.md](research.md) (revisión de arquitectura previa al plan).

## Technical Context

**Language/Version**: Go 1.27.4 (server); TypeScript + React 19 (POS); TypeScript sin framework en la
Function del menú (runtime de Cloudflare Workers)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query; **una nueva**: `uqr`
(codificador QR, MIT, sin dependencias, cargado solo en Venta en web). La Function no usa librerías.

**Storage**: PostgreSQL con RLS (2 tablas, 1 vista, 2 funciones); Cache API de Cloudflare en el borde

**Testing**: `go test` (tabla en `domain`), integración contra Postgres real bajo `appRoleStore` con
`inTheThreeCases` y migración sobre respaldo con dos empresas; vitest para plantilla, marca, datos y
la Function (con `fetch` y `caches` falsos); Playwright contra el POS y contra `wrangler pages dev`;
Lighthouse (perfil móvil por omisión) contra la Function local con el catálogo restaurado

**Target Platform**: API en el VPS (azul/verde); Function en Cloudflare Pages; POS en el equipo de
mostrador (1280×854); el menú en celulares (360–430 px), tabletas (768) y computadoras (1280+)

**Project Type**: web (server + web + un proyecto de Pages nuevo)

**Performance Goals**: LCP < 2.5 s y CLS < 0.1 en Lighthouse móvil; HTML ≤ 35 KB comprimido con
~180 productos; 0 KB de JS en el menú; un fallo de caché cuesta una petición a la API (3 consultas)

**Constraints**: migración aditiva (traslape azul/verde); nada del POS dentro del menú y viceversa
(barrera de eslint como la consola); el endpoint público no expone datos internos (FR-019); controles
de 44 px y sin `<select>` en Ajustes

**Scale/Scope**: 2 empresas, ~180 productos en web y ~25 categorías por empresa

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I. Layering | `domain/public_menu.go`: `SlugifyMenuName`, `ValidMenuNameShape`, `ReservedMenuNames`, `PlanMenuRename`, `ValidPalette`, `FeaturePublicMenu`, `ErrMenuNameUnavailable`. `app/public_menu.go`: `PublicMenuService` (resolver sin tenant → `WithTenant` → oferta + categorías; ajustes con tx). `app/web_sales.go`: `offer(ctx, q)` compartido por `Offer` y el menú. `httpapi`: handlers finos `PublicMenu`, `PublicMenuByToken`, `GetPublicMenuSettings`, `UpdatePublicMenuSettings`; mapeo en `respond.go`. `store/db`: sqlc. Function: `web/menu/functions/` (I/O) sobre `web/src/menu-publico/` (puro). |
| II. Errores | `ErrMenuNameUnavailable` y `ErrValidation` de `domain`, mapeados solo en `httpapi.Error`; 23505 del `unique` traducido a `ErrMenuNameUnavailable` en `store.DomainError`. |
| III. Dinero | El precio sale de `domain.ChannelPrice` con `Round2` (la regla de #326); el menú no recalcula ni guarda precios. |
| IV. Test-first | Tabla en `domain` primero (slugify, forma, reservados, transición de nombre). Integración: migración 0121 con dos empresas; aislamiento `inTheThreeCases` de las dos tablas y del servicio público; `TestThePublicMenuExposesOnlyThePublicFields` sobre el JSON crudo (lista blanca de llaves); SQL y Go normalizan igual. Vitest: plantilla escapa HTML, marca, Function (caché, copia vieja, 302, 404). |
| IV. Migración tolerante | Solo agrega (tablas, vista, funciones, rol, relleno). La versión anterior no lee nada nuevo. `CreateCompany` viejo crea empresas sin menú → `EnsurePublicMenu` lo repara al abrir Ajustes. |
| IV. Tres casos de RLS | `public_menus` y `public_menu_names` entran solos a `TestEveryCompanyTableIsIsolated`; el servicio público se prueba con otra empresa, conexión reciclada y sin empresa. |
| V. Seguridad | Endpoint público con lista blanca de campos, límite por IP con llave real (secreto Function→API, comparación de tiempo constante, test adversarial con IP falsa), 404 indistinguible, token del QR aleatorio, vista sin slug. `MENU_EDGE_KEY` y `MENU_BASE_URL` en `config.Validate` (https en producción; secreto ≥ 32). Ajustes detrás de `RequirePermission(business.configure)`. HTML con escape obligatorio y CSP sin scripts. `/security-review` obligatorio. |
| VI. YAGNI | Sin Redis para el menú, sin purga de caché, sin tabla de funciones por paquete, sin interruptor de publicar, sin PDF en servidor. Una sola dependencia (QR) que la plataforma no ofrece. |
| VII. Comentarios / idioma | Identificadores en inglés (`public_menus`, `PlanMenuRename`); texto y comentarios en español; el porqué de 302, del historial único y de la vista va en la migración y en el servicio. |
| VIII. Puertas | Respondido en data-model («Puertas»): todo se puede agregar después al mismo costo; el historial de nombres se guarda hoy porque es el hecho irrecuperable. «Más de una sucursal»: el menú es por empresa; precio o disponibilidad por sucursal sigue abierto (columna nullable). |
| Restricciones UX (POS) | Venta en web con controles ≥ 44 px, paleta con botones (no `<select>`), componentes comunes, texto para quien opera (sin «Function», «caché», «302»). |
| Texto para quien opera | El menú público no dice nada técnico; los errores son «Este menú no existe» / «No pudimos cargar el menú». |

Sin violaciones. *Complexity Tracking* abajo: un proyecto de Pages más.

## Capas y piezas

| Pieza | Capa | Archivo |
| --- | --- | --- |
| Migración 0121 + su test | migración / integración | `server/migrations/0121_public_menu.sql`, `server/internal/integration/migracion_0121_test.go` |
| Menú al crear empresa | query | `server/queries/companies.sql` (`CreateCompany`) |
| Reglas de nombre, paleta, clave | domain | `server/internal/domain/public_menu.go` (+ `_test.go`) |
| Oferta web ampliada | query + app | `server/queries/web_sales.sql`, `server/internal/app/web_sales.go` |
| Menú público y ajustes | query + app | `server/queries/public_menu.sql`, `server/internal/app/public_menu.go` |
| Rutas públicas y de ajustes, límite por IP | httpapi | `server/internal/httpapi/handlers_public_menu.go`, `router.go`, `ratelimit.go` (`publicMenuKey`), `respond.go` |
| Configuración | config | `server/internal/config/config.go` (`MENU_BASE_URL`, `MENU_EDGE_KEY`) |
| Plantilla, datos, marca | web (puro) | `web/src/menu-publico/{plantillas/classic.ts,datos.ts,marca.ts,html.ts,formato.ts}` |
| Function | web (Pages) | `web/menu/functions/[[path]].ts`, `web/menu/public/{_routes.json,_headers,robots.txt,og-default.png}`, `web/tsconfig.menu.json`, scripts `build:menu`/`dev:menu`/`typecheck:menu` |
| Barrera POS ↔ menú | lint | `web/eslint.config.js` |
| Venta en web (Ajustes) | web | `web/src/features/admin/settings/VentaEnWebSection.tsx` + `web/src/features/admin/settings/menuPublico/*` (enlace, QR, tarjeta, color), `web/src/api/admin.ts` |
| Despliegue | CI | `.github/workflows/ci.yml` (`deploy-menu`, `deploy-menu-staging`, typecheck y tests del menú en `frontend`) |
| Infra local | scripts | `scripts/worktree-env.sh` (`MENU_PORT` = 8700+NN), `scripts/dev-api.sh` (`MENU_BASE_URL`) |
| Documentación | docs | `docs/menu-publico.md` (runbook: proyecto de Pages, variables, regla de Cloudflare, rotar el secreto) + renglón en `docs/README.md` y en la tabla de notas por área de `AGENTS.md` |

## Pantallas y maquetas

## Maquetas

| Pantalla | Dónde vive | Maquetas | Elegida | Por qué |
|----------|-----------|---------|---------|---------|
| Menú público (celular 360, tableta 768, computadora 1280, 2K) | `menu.elgatobobah.com/<nombre>` | [maquetas](https://claude.ai/artifact/DL3qd2Fi6TwRjxe9WpnmiP) | **A + mosaico** (ajuste del dueño sobre A) | La más rápida (LCP 2.0 s contra 2.2 de B y 3.5 de C), la que más enseña (4 productos en celular, 9 en computadora) y aguanta un catálogo con pocas fotos; el dueño pidió además el mosaico de categorías para navegar, que no cuesta carga |
| Ajustes › Venta en web (nombre, enlace, QR, color) | `web/src/features/admin/settings/VentaEnWebSection.tsx` | [maquetas](https://claude.ai/artifact/VWsPvQhF9T3STF1iUY5pni) | **B** | Cabe sin desplazar a 1280×854 (793 de 793 px); lo diario (copiar, descargar) arriba a un toque; nombre en 2 toques y color en 1, sin ventana extra |

### Lo elegido, en detalle

- **Menú público (A + mosaico)**:
  - Encabezado de ancho completo con el color de marca (700, por contraste): nombre del negocio (26 px) y «Menú · N
    productos». Es el LCP de texto si la primera foto tarda.
  - **Celular y tableta (< 1024 px)**: barra de categorías fija arriba (`position: sticky`), botones
    de 44 px que se desplazan a lo ancho; el primero, «Categorías» con ícono de cuadrícula y fondo de
    marca, abre el mosaico. Lista en una columna (dos desde 700 px): renglón con nombre, descripción a
    2 líneas, precio en color de marca y, a la derecha, foto o ilustración de 96 px.
  - **Computadora (≥ 1024 px)**: la barra se oculta y aparece una columna fija de 220 px a la
    izquierda con «Categorías» (abre el mosaico) y el nombre de cada categoría; lista en dos columnas,
    contenido a 1200 px como máximo.
  - **Mosaico**: hoja `#categorias` que se abre con `:target` (sin JavaScript), fondo oscurecido,
    encabezado fijo «Categorías» con «Cerrar» (44 px); cuadrícula de 3 columnas en celular, 6 en
    tableta y computadora, cada mosaico con la foto de la categoría o la ilustración de su familia y
    su nombre. Sus imágenes son `loading="lazy"` dentro de una hoja oculta: no se descargan hasta
    abrirla. Tocar un mosaico lleva a `#c<id>` (y cierra la hoja). En computadora la hoja es una
    ventana centrada de 900 px.
  - **Producto**: `<details>` en su lugar; abierto muestra la foto grande (1200, diferida) y la
    descripción completa. «Agotado» es una etiqueta junto al precio y la foto atenuada; nunca una
    cantidad.
  - Primeras 4 fotos `fetchpriority="high"`; el resto `loading="lazy"` con `width`/`height`.
    `content-visibility: auto` por categoría. Tema claro y oscuro por `prefers-color-scheme`.
- **Ajustes › Venta en web (B)**, dentro del `Page` de 640 px de Ajustes:
  1. Tarjeta «Comparte tu menú»: QR de 144 px a la izquierda; a la derecha la dirección vigente
     (texto grande, se parte si no cabe), fila Copiar enlace (sólido; cambia a «Copiado» con palomita)
     · Compartir · Abrir, y fila de descargas PNG · SVG · Imprimir tarjeta.
  2. Tarjeta de edición: «Nombre en la dirección» (campo + Guardar en la misma fila; debajo «Quedará:
     menu.elgatobobah.com/<normalizado>» en vivo y la frase de que lo anterior y el QR siguen
     funcionando) y «Color del menú» (8 círculos de 44 px, `radiogroup`, palomita en el elegido; el
     color se guarda al tocarlo).
  3. Resumen de productos en web en un renglón con «Ir a los productos».
  - La dirección mostrada es la guardada, no la que se está escribiendo.
  - Errores del nombre bajo el campo: «Ese nombre no está disponible» / «Usa al menos 3 letras o
    números».

### Ajustes de la revisión de arquitectura (después de elegir)

**Pantallas (tablet-ui-reviewer)**

- **Contraste de marca**: el texto blanco sobre el tono 600 queda bajo 4.5:1 en verde (3.30), naranja
  (3.56), cielo (3.68) y menta (3.74). El encabezado y el botón «Categorías» usan el **700** de fondo
  (6.8 a 9.4:1 en las 8 paletas) y el subtítulo sin opacidad. Un test recorre las 8 paletas y falla
  nombrando la que quede bajo 4.5:1 (blanco sobre fondo y 700 sobre blanco).
- **Modo oscuro del menú**: se redefinen `--m100` (el toque en un chip ya no queda rosa con texto
  claro) y el trazo de las ilustraciones al tono 100.
- **Mosaico**: `overscroll-behavior: contain` en la hoja y `html:has(.hoja:target){overflow:hidden}`
  para que el cuerpo no se desplace detrás. Sin JavaScript no hay trampa de foco ni Esc: aceptado y
  anotado (la hoja es una lista de enlaces, no un formulario). «Cerrar» deja un fragmento en el
  historial: aceptado.
- **Pie del menú** a 14 px.
- **Color en Ajustes**: anillo de foco visible en los 8 círculos (`_focusVisible`), el nombre de la
  paleta elegida escrito junto al grupo y un aviso «Color guardado» al tocar (se guarda al tocarlo,
  como hoy Interfaz).
- **Alto de la tarjeta de compartir**: con el `Page` de 640 px los 6 botones ocupaban 4 renglones y
  la sección quedaba 4 px por encima del alto útil (797 de 793). El QR baja a **144 px** y las
  descargas van en una fila: «PNG», «SVG», «Imprimir tarjeta». La implementación se mide a 1280×854
  **con el mensaje de error del nombre visible** y debe caber sin desplazar.
- **Validación en vivo** del nombre: «Usa al menos 3 letras o números» bajo el campo mientras se
  escribe; sin «…» vacío en «Quedará».

**Base de datos (db-architect)** — aplicado en [data-model.md](data-model.md):

- Slugify con truncado y recorte de guiones en orden; `free_name` que nunca viola el check (base
  vacía → `menu-<id>`, sufijo sin desbordar 40).
- Renombrar con `for update` sobre el menú de la empresa; revivir exige 1 fila; el 23505 se
  distingue por restricción.
- Tope de 20 nombres retirados por empresa (`ErrMenuNameLimit`): un nombre global nunca se libera y
  sin tope una empresa podría acapararlos.
- Relleno en un loop ordenado; token sin pgcrypto; `grant select (id, is_active)` sobre `companies`;
  vista `security_barrier`; sin `force row level security`; el Down no borra el rol.
- `public_menus` con `id` propio y FK compuesta del historial: el menú por sucursal se agrega después
  sin reconstruir la FK.
- La Function pasa el nombre a minúsculas y redirige con 302 a la forma canónica antes de validar
  (`/El-Gato-Bobah` → `/el-gato-bobah`).

Medido en las maquetas: a 1280×854 la sección B ocupa 793 de 793 px de alto útil; ningún control
de menos de 44 px; en 2K sobra espacio.

## Project Structure

### Documentation (this feature)

```text
specs/097-menu-publico/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code

```text
server/
├── migrations/0121_public_menu.sql
├── queries/{public_menu,web_sales,companies}.sql
└── internal/
    ├── domain/public_menu.go
    ├── app/{public_menu,web_sales}.go
    ├── config/config.go
    ├── httpapi/{handlers_public_menu,router,ratelimit,respond}.go
    └── integration/{migracion_0121,public_menu}_test.go
web/
├── menu/                       ← proyecto de Pages nuevo (sin Vite)
│   ├── functions/[[path]].ts
│   └── public/{_routes.json,_headers,robots.txt,og-default.png}
├── src/menu-publico/           ← plantilla + datos + marca (puro, probado con vitest)
├── src/features/admin/settings/{VentaEnWebSection.tsx,menuPublico/*}
└── tsconfig.menu.json
```

**Structure Decision**: monorepo existente. El menú es un tercer producto de `web/` con su propio
`tsconfig`, sus propios scripts y su propio proyecto de Pages, aislado del POS por eslint en las dos
direcciones (como la consola).

## Lo que necesita el orquestador en Cloudflare (una sola vez)

Se entrega al dueño en un solo mensaje cuando el plan quede aprobado; el detalle vive en
`docs/menu-publico.md`:

1. Dos proyectos de Pages de subida directa: `el-gato-bobah-pos-menu` y `el-gato-bobah-pos-menu-dev`.
2. Dominios propios: `menu.elgatobobah.com` → el primero; `menu-dev.elgatobobah.com` → el segundo.
3. Variables de cada proyecto (Production): `API_ORIGIN`, `MEDIA_ORIGIN` y el secreto `MENU_EDGE_KEY`.
4. El mismo `MENU_EDGE_KEY` y `MENU_BASE_URL` en el `deploy/.env` de `pos-vps` y `pos-vps-dev`.
5. Una regla de rate limiting en la zona: host `menu.elgatobobah.com` o `menu-dev.elgatobobah.com`,
   por IP, 60 peticiones en 10 s, bloquear 10 s.
6. El token de CI (`CLOUDFLARE_API_TOKEN`) con permiso de Pages sobre los proyectos nuevos.

## Coordinación con otras ramas

- **#65 / spec 094** (varios almacenes, ya en develop): «Agotado» llega de `salesStock`, que ya mide
  el almacén de venta. El menú no lo toca.
- **#328** (pedidos desde el menú): los ids de producto ya viajan; la Function tendrá que agregar JS
  solo en esa etapa, como isla, y revalidar existencia en la API al pedir (el menú puede tener hasta
  60 s de atraso).
- **#331** (dominio genérico): `MENU_BASE_URL` es la única base; el QR ya impreso con
  `menu.elgatobobah.com/q/…` exige que ese host siga redirigiendo para siempre (anotado en el runbook).

## Complexity Tracking

| Qué | Por qué | Lo que cuesta |
| --- | --- | --- |
| Un proyecto de Pages más con una Function | El rastreador de WhatsApp no ejecuta JS y el presupuesto de carga no admite una SPA (research R1) | Dos jobs de CI, variables por ambiente y una barrera de eslint más |
| Secreto Function→API | Sin él, el límite por IP de la API vería IPs de Cloudflare (research R5) | Una variable en cada lado y su rotación en el runbook |
