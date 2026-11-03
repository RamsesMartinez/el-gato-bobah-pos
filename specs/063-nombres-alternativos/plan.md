# Implementation Plan: Nombres alternativos en productos y artículos

**Branch**: `063-nombres-alternativos` | **Date**: 2026-11-02 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/063-nombres-alternativos/spec.md` (issue #221)

## Summary

Productos y artículos del almacén ganan una lista de hasta 10 nombres alternativos
(`alt_names text[]` en su fila, migración 0107). Se escriben por una ruta propia que reemplaza la
lista completa, bajo el mismo permiso que edita la ficha, con la regla de choque (nombre principal y
alternativos de los demás, sin mayúsculas, acentos ni espacios de más) decidida bajo un candado por
empresa. Los seis buscadores comparan también contra los alternativos y dicen cuál coincidió; lo que
se imprime y se reporta sigue leyendo solo el nombre principal. Detalle en [research.md](research.md).

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query

**Storage**: PostgreSQL (dos columnas `text[]` y una función SQL nuevas)

**Testing**: `go test` (dominio table-driven), integración contra Postgres real bajo `appRoleStore`
con `inTheThreeCases`, vitest en web, Playwright a 1024×600 contra el `make start` local

**Target Platform**: tabletas de 7–10" (~1024×600), API en VM Linux

**Project Type**: web (monorepo `server/` + `web/`)

**Performance Goals**: buscar sigue siendo instantáneo: el filtro del cliente suma ≤ 10 textos por
producto; las consultas del servidor recorren el catálogo de una empresa (cientos de filas)

**Constraints**: la 0107 convive con la API anterior (R8); nada de `<select>` nativo; 44 px por
control; menú del POS invalidado por la versión de catálogo existente

**Scale/Scope**: ~200 productos y ~300 artículos por empresa; 6 buscadores, 2 fichas

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I. Layering | `domain/alt_names.go` (limpiar, validar, llave, error); `app` (candado + choque + escritura en una tx); `httpapi` (dos handlers finos); SQL solo vía sqlc |
| II. Errores | `domain.AltNameTaken` envuelve `ErrConflict`; el código HTTP se decide solo en `httpapi.Error` |
| III. Dinero | No toca dinero |
| IV. Test-first | Dominio table-driven primero; integración de la 0107 sobre base restaurada con dos empresas; aislamiento con `inTheThreeCases`; concurrencia; paridad Go/SQL; regresión «el renglón vendido lleva el nombre principal». Migración tolera la API anterior (R8) |
| V. Seguridad | Rutas con `RequirePermission` existente; los nombres de otra empresa nunca aparecen en un aviso (RLS + prueba); entradas absurdas (11 nombres, 61 letras) → 400 |
| VI. YAGNI | Columna en vez de tabla; sin búsqueda difusa; sin pasar alternativos en uniones de insumos |
| VII. Comentarios / idioma | Identificadores en inglés (`alt_names`, `AltNameTaken`, `catalog_name_key`); textos en español |
| VIII. Puertas | ¿Se puede agregar después al mismo costo? Sí (R10). No toca ninguna puerta |
| Restricciones de producto | Componentes comunes (`Picker`, `SearchSheet`, `ListRow`, tokens); controles de 44 px; sin `<select>`; ninguna pregunta a media operación |

Sin violaciones; Complexity Tracking vacío.

## Capas

| Pieza | Capa | Archivo |
| --- | --- | --- |
| Migración 0107 | migración | `server/migrations/0107_alt_names.sql` |
| `CatalogNameKey`, `CleanAltNames`, `AltNameTaken`, `MaxAltNames`, `MaxAltName` | domain | `server/internal/domain/alt_names.go` |
| `ArticleNameTaken.ViaAlt` | domain | `server/internal/domain/ingredients.go` |
| Consultas de candado, choque y escritura | store (sqlc) | `server/queries/alt_names.sql` |
| Búsquedas y lecturas con alternativos | store (sqlc) | `admin.sql`, `ingredients_admin.sql`, `ingredients.sql`, `menu.sql`, `stock_counts.sql` |
| `SetProductAltNames`, choques en crear/renombrar/duplicar | app | `server/internal/app/alt_names.go`, `admin.go` |
| `SetArticleAltNames`, `createArticleTx`, `UpdateArticle`, `replaceArticle` | app | `alt_names.go`, `ingredients_admin.go`, `unit_change.go` |
| Menú y conteo con `altNames` | app | `menu.go` (o donde se arma el menú), conteo |
| Handlers y rutas (incluye `…/alt-names/similar`) | httpapi | `handlers_alt_names.go`, `router.go`, `respond.go` |
| Respaldo anónimo | integración | `noPersonalData`: `products.alt_names`, `ingredients.alt_names` («catálogo») |
| `matchedAlias` | web/utils | `web/src/utils/format.ts` |
| `aliases` en opciones | web/components | `Picker.tsx`, `shared/SearchSheet.tsx` |
| Editor de alternativos, chip y etiqueta | web/components | `AltNamesField.tsx`, `AltNameChip.tsx`, `AltMatchTag.tsx`, `ListRow.tsx` (`match`) |
| Fichas | web | `shared/ProductEditDialog.tsx`, `features/backoffice/articles/ArticleSheet.tsx` |
| Buscadores | web | `features/pos/buscarProducto.ts` + `ProductTile.tsx`, `ProductsAdminPage.tsx`, `ArticlesTab.tsx`, `StockCountTab.tsx`, `ExpenseDialog.tsx`, `AddLineSheet.tsx`, `ArrivedItemSheet.tsx`, `shared/CompositionSheet.tsx` |

## Pantallas y maquetas

Dos piezas de interfaz, las dos dentro de pantallas que ya existen:

1. **Editor «Otros nombres»** (opción A) en la ficha del producto (`ProductEditDialog`, guarda con
   su botón) y en la ficha del artículo (`ArticleSheet`, se guarda al agregar o quitar, como el
   resto de la ficha). Un componente, `AltNamesField`: título «Otros nombres (n de 10)», chips de
   44 px con «×» de 40 px para quitar, y debajo un campo de 44 px con «Agregar» (Enter también
   agrega). Va después de Categoría; los chips se acomodan en renglones (en una sola línea con
   scroll, el 4.º quedaba fuera de la hoja en la prueba con usuario nuevo); «×» de 44×44. Un nombre
   igual al de otro no se agrega y dice de quién es; uno parecido avisa. Con 10 el campo se reemplaza por «Ya tiene 10. Quita uno para agregar otro.». Un choque
   se muestra en rojo bajo el campo con el texto del servidor. **Aviso de parecidos** (pedido del
   dueño al elegir): mientras se escribe (300 ms de pausa) se consultan parecidos; «Agregar» espera
   esa consulta, y si hay parecidos el primer toque muestra «Se parece a: «X», «Y»» en color de
   advertencia y el botón pasa a «Agregar de todos modos». No impide agregar. La nota «Sirven para
   encontrarlo al buscar. No salen en el ticket ni en la comanda.» va en el `InfoButton` del título.
   Texto escrito sin «Agregar» se agrega al guardar o salir (R14).
   - En el producto, la lista se guarda con «Guardar» de la ficha: primero `PUT alt-names`
     (si cambió) y luego el `PATCH` de siempre; un choque deja la ficha abierta sin cerrar.
   - En el artículo, agregar y quitar pasan por el autoguardado de la ficha y su `SaveIndicator`; si
     falla, la lista vuelve a la guardada.
2. **Otro nombre que coincidió** (opción B): una etiqueta común `AltMatchTag` («también: café
   frío», `Badge` outline gris) junto al nombre en la fila de Productos, la de Artículos, la de
   Conteo y las filas de `Picker` (`ListRow`, prop `match`) y `SearchSheet`, truncada a `maxW` 45%
   con el nombre con prioridad. En el tile de Vender ocupa la línea de la descripción mientras el
   resultado salió por alternativo, para no hacer crecer el tile (R14). En Conteo va en su propio
   renglón bajo el nombre (revisión de código: junto a un nombre largo se quedaba sin ancho).

Presupuesto de alto: la ficha del producto ya hace scroll dentro de su diálogo; el editor ocupa una
fila de chips + un campo de 44 px + una línea de aviso. La etiqueta va en la misma línea del nombre:
en el tile puede empujar el nombre a una línea más (tiene `lineClamp` 3), en las filas no agrega
alto.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Editor «Otros nombres» | https://claude.ai/artifact/AHVPSACV77sgAyZZFdpvpD | A, con aviso de parecidos | Se ven y se quitan sin abrir nada; el dueño pidió además avisar (sin bloquear) si hay otro con nombre parecido |
| «Coincidió con» en resultados | https://claude.ai/artifact/WMYpvgWr5UxDYJEDsGHDYz | B | El otro nombre se ve de un vistazo como etiqueta junto al nombre |

## Project Structure

### Documentation (this feature)

```text
specs/063-nombres-alternativos/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/alt-names.md
└── tasks.md
```

### Source Code

```text
server/
├── migrations/0107_alt_names.sql
├── queries/alt_names.sql (+ admin, ingredients_admin, ingredients, menu, stock_counts)
├── internal/domain/alt_names.go (+ _test)
├── internal/app/alt_names.go (+ admin.go, ingredients_admin.go, unit_change.go, menú, conteo)
├── internal/httpapi/handlers_alt_names.go (+ router.go, respond.go)
└── internal/integration/alt_names_{products,articles,menu,http}_test.go, migration_0107_test.go,
    catalog_name_key_test.go (+ respaldo_anonimo_test.go)
web/src/
├── utils/format.ts
├── components/AltNamesField.tsx, Picker.tsx
├── shared/SearchSheet.tsx, ProductEditDialog.tsx, CompositionSheet.tsx
├── api/admin.ts, api/backoffice.ts, types/pos.ts
└── features/pos, features/admin, features/backoffice
```

**Structure Decision**: monorepo existente; ningún paquete nuevo.

## Revisión de arquitectura (2026-11-03)

`db-architect` y `tablet-ui-reviewer` sobre este plan: cambios requeridos, todos aplicados aquí y en
[data-model.md](data-model.md) / [research.md](research.md) (R12–R14): `lock_timeout` y una sentencia
por tabla en la 0107; `btrim` después de juntar espacios y espacio duro explícito; check contra
elementos nulos o vacíos; listas vacías nunca nil; `matched_alt` con orden capturado; escritores
exentos del candado enumerados; orígenes de unión fuera del choque; texto pendiente al guardar,
aviso de parecidos antes de agregar, alto acotado, «×» de 44 px, etiqueta y chip comunes, tile sin
crecer. Ninguno cruzaba una puerta del principio VIII.

## Prueba con usuario nuevo (2026-11-03)

Ticket y comanda salen con el nombre principal (HTML del iframe de impresión). Corregido de lo que
encontró: el choque con el nombre de otro producto se ve al agregar y no al guardar, con un mensaje
que dice «de otro producto»; los chips en renglones; la lista de Menú › Recetas busca por otros
nombres del producto y de sus insumos; textos del campo por catálogo (en un artículo, «otro nombre o
marca»). Queda como está: el editor abajo de Categoría en la ficha del producto (decisión de la
revisión de tableta), y el aviso de parecidos con la regla de unir insumos (no tolera dos dedazos).

## Vuelta final contra el build servido (2026-11-03)

Usuario nuevo con Playwright a 1024×600 (`hasTouch`) contra `bun run build && bun run preview`, API
local como `gatobobah_app` sobre un respaldo de producción restaurado (0107 aplicada al arrancar).

- Pasan los seis buscadores (Vender, Productos, Artículos, Conteo en vista «Lista», renglón de gasto y
  receta, insumo y producto del combo): encuentran sin acentos ni mayúsculas y la etiqueta dice
  «también: …» con el otro nombre que coincidió.
- Captura en la ficha del producto (con «Guardar») y del artículo (autoguardado), las dos sobreviven a
  recargar; el nombre de otro producto se rechaza nombrándolo (también por API: 409 `ALT_NAME_TAKEN`).
- Comanda (reimpresa desde Pedidos), ticket de «Ver ticket» y ticket del cobro: «1x» con el nombre
  principal, ninguno con el otro nombre. El pedido se cobró y entregó.
- Ningún 5xx ni error de consola.
- No verificado: la búsqueda del conteo en «Uno por uno» y la comanda impresa sola (la impresión
  automática está apagada en el respaldo).
- Observado, sin cambio: en gasto y receta, junto a un resultado que coincidió por otro nombre sigue
  la fila «Crear «…»» con lo escrito. Escribir el otro nombre completo y crear ofrece el artículo que
  ya existe (FR-014); lo que crea uno nuevo es un texto parcial, que es otro nombre.

## Riesgos

- **El borde de unicidad en concurrencia**: lo cubre el candado (R3) y una prueba con dos
  guardados simultáneos.
- **sqlc y el NULL de `matched_alt`**: `coalesce(…, '')` (R6).
- **Picker compartido**: agregar `aliases` es opcional; las hojas que no lo pasan no cambian. Lo
  vigilan los tests existentes del `Picker`.
- **Productos buscaba con `ilike`**: pasar a `strpos(catalog_name_key)` cambia que «%» y «_» ya no
  sean comodines (mejora) y que los acentos dejen de importar (pedido en el issue).

## Complexity Tracking

Sin violaciones.
