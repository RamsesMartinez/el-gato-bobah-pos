# Implementation Plan: Ajustes de Almacén tras la prueba de la 044

**Branch**: `045-ajustes-de-almacen` | **Date**: 2026-10-22 | **Spec**: [spec.md](spec.md)

## Summary

Cinco arreglos de front, sin esquema ni servidor. Causas encontradas en el código (detalle en
[research.md](research.md)):

| Historia | Causa | Arreglo |
|---|---|---|
| US1 unidades | Existencias pinta `{s.on_hand} {s.unit_code}` crudo (`StockPage.tsx:92`); Artículos usa `formatBaseQty` | Existencias (y todo lo que pinte una cantidad del almacén) pasa por `formatBaseQty` |
| US2 conteo | «Revisar con un conteo» es un `<Text>` (`ArticleSheet.tsx:275`) | Botón compacto «Revisar con un conteo ›» (44 px de alto, al final del renglón de «Hay …», sin `py` extra ni partir la línea; maqueta A) que espera a que termine el autoguardado de la ficha y navega a `/almacen?tab=conteo&q=<nombre>`; la lista del conteo toma `q` inicial |
| US3 buscador | `normalize(name).includes(needle)` (`StockCountTab.tsx:434`) | Función pura `matchesWords`: cada palabra buscada coincide al inicio de una palabra del nombre o del empaque |
| US4 barra lateral | Por confirmar a 1024×600: la lista de navegación tiene su propio scroll y queda a media altura de un renglón | Reproducir con Playwright primero; arreglo: ajuste por renglón (`scrollSnapType: y mandatory`, `scrollSnapAlign: start` en cada opción; maqueta A). **Descartados** desplazar hasta la opción activa (maqueta B, deja la de arriba cortada hasta el siguiente toque) y regresar la lista arriba al navegar: al tocar una opción bajo el pliegue («Asistencia») la activa quedaría fuera de vista |
| US5 tema | `main.tsx:30` pasa `defaultTheme="light"`: sin elección guardada, nunca mira al sistema | `defaultTheme="system"`; la opción ya existe en Interfaz › Tema como «Sistema», se renombra «Como el sistema» y va primero. Las tabletas sin elección y con el sistema en oscuro pasan a oscuro al recargar: se repasan en oscuro POS, cobro y corte (B11) |

## Technical Context

**Language/Version**: TypeScript, React 19, Chakra UI v3, next-themes
**Primary Dependencies**: las existentes; ninguna nueva
**Storage**: N/A (preferencia de tema en `localStorage` de next-themes, como hoy)
**Testing**: vitest (unidades, búsqueda, tema, botón), Playwright local a 1024×600 (barra lateral y tema oscuro)
**Target Platform**: tabletas 1024×600
**Project Type**: web (solo `web/`)
**Constraints**: controles ≥44 px, sin `<select>` nativo, un solo estilo
**Scale/Scope**: ~6 archivos de `web/src`

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | Solo front; la lógica pura (`matchesWords`, `formatBaseQty`) en módulos sin React |
| IV Test-first y bordes | Cada historia arranca con su test en rojo; bordes en la tabla de abajo |
| V Seguridad | El botón de US2 se muestra según `can('inventory.count')` (o el permiso que hoy abre la pestaña Conteo); el servidor ya protege el conteo |
| VI YAGNI | Sin componente nuevo de búsqueda: una función y su uso |
| VIII Puertas | No toca esquema: ninguna puerta en juego |
| Tableta | El botón nuevo usa `Button` del tema (44 px, `brand`); sin pantallas nuevas |

Pasa. Post-diseño: sin cambios.

## Capas

Todo en `web/src`: `shared/units.ts`, `utils/format.ts` (o junto al conteo) para `matchesWords`,
`features/backoffice/StockPage.tsx`, `StockCountTab.tsx`, `articles/ArticleSheet.tsx`,
`app/AppShell.tsx`, `main.tsx`, `features/admin/AppearancePage.tsx`.

## Maquetas

| Pantalla | Página | Elegida | Por qué |
|---|---|---|---|
| Ficha del artículo: aviso de existencia negativa | https://claude.ai/artifact/4kw6b56kBcviSGYV9tZGWA | A · botón «Revisar con un conteo ›» de 44 px al final del aviso, mismo renglón | El aviso sigue leyéndose como aviso, sin alto extra |
| Barra lateral: desplazamiento | https://claude.ai/artifact/RuTAU63NUhq7avLaA6rx6i | A · se acomoda por renglón (`scroll-snap`) | Ninguna opción queda cortada nunca |

Sin maqueta: tema (renombra y reordena una opción existente) y unidades (cambia texto, no disposición).

## Bordes (se piensan antes del código)

| # | Borde | Cómo se cierra | Test |
|---|---|---|---|
| B1 | Cantidad 0, -1, 1, 1.5, -3000 ml, 999 g, 0.3333 L | Una sola función; mismo texto en Existencias, Artículos y ficha | vitest de `formatBaseQty` + de Existencias |
| B2 | `-1 pieza` contra `-1 piezas` | Singular por valor absoluto 1 (regla actual) | vitest |
| B3 | Unidad desconocida | Se muestra el código tal cual | vitest |
| B4 | «apio» contra «Tapioca», «almen», «AZÚ», «leche alm», espacios dobles, una letra | `matchesWords`: palabras por `split(/\s+/)`, cada una `startsWith` de alguna palabra normalizada | vitest tabla |
| B5 | Empaque «Bolsa de hielo» al buscar «bols» | Mismo criterio sobre nombre de empaque | vitest |
| B6 | `?q=` con un artículo que no está en el conteo abierto, o sin conteo abierto | Lista vacía con la búsqueda visible; sin conteo, se ve «Empezar» y la búsqueda se aplica al empezar | vitest |
| B7 | Usuario sin permiso de contar | El renglón sigue como aviso de texto, sin botón | vitest |
| B8 | Elección de tema guardada antes (light/dark) | next-themes la lee de `localStorage` y gana sobre el default | vitest |
| B11 | Tableta sin elección de tema y sistema en oscuro: todo el POS cambia al recargar | Repaso en oscuro de POS, hoja de cobro y corte de caja; colores fijos (`gray.900`, `white`, hex) que se pierdan se pasan a tokens; contraste 4.5:1 texto, 3:1 controles | Playwright local con `colorScheme: 'dark'` y capturas |
| B12 | Tocar «Revisar con un conteo» con el autoguardado de la ficha a medias (se acaba de escribir el mínimo) | El botón espera a que termine el guardado (o lo confirma) antes de navegar; si el guardado falla, no navega y se ve el error | vitest |
| B9 | Sistema cambia con la app abierta y tema = sistema | next-themes escucha `prefers-color-scheme` | Playwright `emulateMedia` |
| B10 | Barra lateral más alta que 600 px, página o lista desplazada | Primer renglón completo y tocable; al navegar a una opción bajo el pliegue, la activa sigue visible | Playwright con medición de `getBoundingClientRect` |

## Al construir

- **US1, barrido de cantidades (T005)**: además de Existencias (existencia y mínimo) pasaron por
  `formatBaseQty` el reporte de faltante del conteo (`CountLossReport`) y los renglones de merma
  (`WasteTab`, `WastePage`, `PendingWasteBar`). La merma de un producto llega con unidad `pza` (sin
  unidad base): `formatBaseQty` la dice como pieza. **No se tocó** Por recibir: sus cantidades van en
  la unidad de compra (bolsa, caja), no en la base.
- **US2**: la ficha recibe `onCount`; `StockPage` lo pasa solo con `inventory.count`. El botón saca el
  foco del campo (eso lo manda a guardar) y espera la cola del autoguardado; si el guardado falla se
  queda en la ficha con «No se pudo guardar» a la vista, sin la pregunta de descartar. Con `?q=` el
  conteo abre en Lista aunque la vista recordada sea «Uno por uno» (que no tiene buscador), sin
  cambiar la recordada.
- **US3, barrido de buscadores (T012)**: `matchesWords` en el conteo (nombre y empaque) y en Por recibir
  (artículo y proveedor). **No se tocaron**: la búsqueda de Artículos, que filtra en el servidor con
  `ilike '%q%'` (cambiarla es servidor, fuera del alcance de esta feature: issue aparte); el `Picker` y
  `SearchSheet`, compartidos con todo el POS, no solo con el almacén.
- **US4, causa medida (T013)**: la página nunca se desplaza (`docH` = 600). Lo que partía la opción era
  la propia barra: al abrir una pantalla con su opción bajo el pliegue (Asistencia) se llevaba a la
  vista con `scrollIntoView({ block: 'nearest' })`, que la desplazaba 111 px y dejaba la de arriba
  partida. Con solo `scroll-snap` la activa quedaba partida abajo; se calcula el primer inicio de
  opción que la deja entera (`railScrollFor`).
- **US5**: el tema por omisión pasó de `main.tsx` al `Provider` (`defaultTheme="system"`), para que la
  prueba monte lo mismo que la app. Repaso en oscuro (B11) a 1024×600 con contraste medido: Caja
  (cerrada, contador, abierta y corte), POS, ticket y hoja de cobro, **0 textos perdidos por el
  tema**. Para el POS se abrió una caja en la base local del worktree; el pedido de prueba lo cobró
  el `globalTeardown`. **Pendientes, no de esta feature** (fijos, igual en claro que en oscuro): la
  categoría activa del POS («Top», blanco sobre `brand.500`, 4.29:1) y el verde de «Cobrar» /
  «Enviar y cobrar» (blanco sobre `green.600`, 3.30:1; pasa solo como texto grande en negritas).
- **Prueba con usuario sin contexto (2026-10-22), arreglado con su prueba**: llegando desde la ficha
  sin conteo abierto, la pestaña no decía para qué artículo se llegó; ahora dice «Al empezar, la lista
  ya estará buscando «…»». Lograda sin tropiezos: unidades iguales en las tres pantallas (leche en L),
  «apio» sin «Tapioca», barra lateral sin opciones partidas, tema «Como el sistema». **Pendientes, fuera
  de alcance**: Existencias no tiene buscador (~250 renglones); el selector de lugar del conteo sale
  cortado («Almacén prin…»); el mínimo de la ficha se captura en g mientras la existencia se dice en
  kg; lugares llamados «Almacén …» se leen como almacenes (es dato de la empresa, no texto del
  sistema). La prueba dejó un conteo abierto en la base local del worktree.
- **SC-001 medido (T022)**: `web/e2e/existencias-iguales.spec.ts` contra el `make start` del worktree con
  el respaldo de producción del 2026-10-22: 400 artículos, 400 fichas abiertas, **0 diferencias** entre
  Existencias, Artículos y la ficha (11.2 min). Abre cada ficha tocando su renglón: recargar la página
  por ficha dejaba la pantalla en blanco a media corrida.
- **Vuelta final contra el build (T021)**: `bun run build && bun run preview` (puerto 4146: Chromium
  bloquea el 4045 como inseguro) con la misma API local: las cuatro e2e en verde (10 pruebas, 9.6 min):
  400 artículos con 0 diferencias, barra lateral, tema del sistema y el repaso en oscuro con los mismos
  dos contrastes fijos de arriba y nada más.

