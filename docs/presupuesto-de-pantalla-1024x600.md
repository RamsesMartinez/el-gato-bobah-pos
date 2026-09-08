# El presupuesto de pantalla del POS a 1024×600

**Medido el 7 de septiembre de 2026** en chromium headless (Playwright 1.62.1) contra la app local
(Vite en :3000, API en :8080), con el catálogo de 501 productos de la empresa `gatobobah`. Lo que
está aquí **no se vuelve a derivar**: se cita.

Este documento existe porque el alto de esta pantalla es un requisito funcional que se olvida solo,
y porque medirlo no se puede hacer en vitest: las medidas de Chakra son clases CSS y jsdom no las
resuelve, así que un assert de píxeles allá pasa verde con los controles chicos. Hace falta un
navegador de verdad.

## Cómo está armada la pantalla a 1024×600

Contra lo que decían [matriz-de-pantallas.md](matriz-de-pantallas.md) (caso V11) y el comentario de
`web/e2e/cabe-en-la-tableta.spec.ts`, a 1024×600 el POS **no está en modo angosto**:

- `wide` es verdadero a 1024 px de ancho, así que la rama que corre es la del panel lateral.
- `panelHidden` **arranca colapsado** porque `window.matchMedia('(max-height: 720px)')` hace match
  con 600 px de alto (`web/src/features/pos/POSPage.tsx:257`). El panel del ticket no se ve.
- Por eso el control del carrito es la **píldora flotante arrastrable** de la rama
  `wide && panelHidden` (`POSPage.tsx:518`), no una hoja inferior ni una barra.

| Elemento | Caja medida |
|---|---|
| Píldora flotante (`position: absolute`) | x 739–1008, y 524–584 (269×60) |
| Handle de arrastre (`aria-label="Mover"`) | 36×44 |
| Botón `Cobrar` de la píldora | y 532–576, alto 44 (cumple el piso táctil) |
| Barra de 64 px de ancho completo al fondo | **no existe**: los únicos elementos de 64 px de alto y ancho completo están en y 181–245, y son el riel de categorías |
| Catálogo (ancho) | x ≈ 92 a 1008; el riel de navegación ocupa la izquierda |

La píldora **se traslapa con el mosaico** en vez de quitarle alto: el contenedor del catálogo mide
lo mismo con carrito y sin carrito. El propio código ya lo sabe (`POSPage.tsx:261`: "a veces tapa
las cards de abajo-derecha; el operador la mueve a discreción").

## El mosaico

| Medida | Valor |
|---|---|
| Alto de ficha | 104 px — es el `minH`; una ficha cuyo nombre envuelve a dos líneas crece y su renglón entero crece con ella |
| Gap | 10 px |
| **Paso de renglón** | **114 px** |
| Columnas | 5 |

## Cuántos renglones se ven, y cuánto sobra

Renglones **completos** dentro de la ventana de 600 px. El "sobrante" es lo que queda después del
último renglón completo: es el presupuesto disponible para meter un control nuevo arriba del mosaico
sin costar un renglón.

| Estado | Tope del catálogo | Alto visible | Renglones | Sobrante |
|---|---|---|---|---|
| Mostrador, con el aviso de caja | y=253 | 347 px | 3 | 15 px |
| Plataforma activa, con el aviso | y=296 | 304 px | 2 | 86 px |
| Mostrador, sin el aviso | y=200 | 400 px | 3 | 68 px |
| Plataforma activa, sin el aviso | y=243 | 357 px | 3 | 25 px |

El bloque del selector de plataforma (`PlatformPicker`) mide **40 px** sin plataforma y **83 px**
con una plataforma elegida: **+43 px**, que es lo que un pedido de plataforma ya cuesta hoy de alto.

**El presupuesto que manda es 25 px** — el del estado más apretado en renglones (plataforma activa,
sin aviso). El piso de altura táctil de la constitución es 44 px, así que **ningún campo de texto
cabe en 25 px**: un control nuevo en ese bloque cuesta exactamente un renglón, de 3 a 2.

## El aviso de caja abierta tira 53 px

El aviso de turno viejo (`AvisoDeTurnoViejo`, el de "La caja lleva abierta desde…") **desplaza el
shell 53 px hacia abajo sin restárselos de su alto**:

| Con el aviso | Sin el aviso ("Ahora no") |
|---|---|
| Contenedor del catálogo: y 253 → 653 | y 200 → 600 |
| Alto del contenedor: 400 px | 400 px — **el mismo** |
| Fuera de la ventana: **53 px** | 0 px |

No es scroll de página: `document.documentElement.scrollHeight` es 600, igual que su
`clientHeight`. Son 53 px del área de producto pintados fuera de la pantalla, casi medio renglón.
Mientras el aviso está puesto, elegir una plataforma baja el mosaico de 3 renglones a 2.

Es la palanca más barata que tiene esta pantalla: recuperar esos 53 px vale más que cualquier
control que se le quite.

## Cómo reproducirlo

Ventana de 1024×600, sesión `admin@gatobobah`. Dentro del navegador:

1. El mosaico es el único `div` con `display: grid`, más de una columna y más de cuatro hijos.
2. Su contenedor de scroll es el primer ancestro con `overflow-y: auto`.
3. El piso real es `min(contenedor.bottom, 600)` — lo que queda debajo de 600 no lo ve el operador,
   tenga o no scroll el contenedor.
4. Renglones que caben: `floor((alto_visible + gap) / paso)`, porque el último renglón no paga gap
   de cierre.

Ojo con dos trampas al medir:

- **Contar los renglones pintados no mide la pantalla**: la pestaña "Top" está topada a `topCount`
  fichas, que es una preferencia del usuario. Para medir la pantalla hay que elegir una
  subcategoría, que devuelve el scope completo sin recorte (`POSPage.tsx:322`).
- **Un `element.click()` sintético no agrega el producto**: el mosaico usa `useLongPress`, que
  escucha eventos de puntero. Hace falta un clic real (el de un locator de Playwright).

Esta medición todavía **no tiene un caso de e2e que la sostenga**. Debería tenerlo, junto con el
V11 de [matriz-de-pantallas.md](matriz-de-pantallas.md).

---

# Medición del 8 de septiembre de 2026 — el folio de plataforma (spec 014)

Mismo método y mismo navegador, pero contra el **ambiente desplegado**
(`app-dev.elgatobobah.com`) y no contra la app local, porque es lo que la suite de Playwright ya
usa. Los casos que la producen están en
[web/e2e/folio-de-plataforma.spec.ts](../web/e2e/folio-de-plataforma.spec.ts) y vuelven a medir en
cada corrida: este documento cita, no re-deriva.

## El bloque del selector de plataforma

| | Antes (7-sep) | Ahora | Qué cambió |
|---|---|---|---|
| Sin plataforma | 40 px | **44 px** | Los botones subieron al piso táctil. Estaban por debajo desde que se escribieron |
| Con plataforma elegida | 83 px | **87 px** | +43 px sobre el estado sin plataforma: **exactamente el mismo delta que antes** |

**El campo del folio NO agregó alto al estado con plataforma.** Entra en el renglón que los botones
ya ocupaban, que es para lo que se puso en línea y no apilado. Los 4 px de diferencia son el piso
táctil, no el campo.

## El mosaico

| Estado | Renglones | Sobrante |
|---|---|---|
| Mostrador | 3 | 6 px |
| Plataforma activa, con el campo de folio | 2 | 77 px |

**El renglón que se pierde al elegir plataforma YA se perdía antes de esta feature**: lo cuestan las
dos líneas de texto que el selector muestra desde la 002 ("Cobrando con precios de…" y la
instrucción de la pulsación larga), no el campo nuevo. Con 6 px de sobra en mostrador, el
presupuesto de esta pantalla sigue siendo el más apretado del sistema.

## La pantalla de Ventas — medida por primera vez

Este documento solo cubría el POS. Con un mes de datos y filas de 54 px:

| Estado | Tope de la tabla | Alto útil | Renglones |
|---|---|---|---|
| Sin cifras de plataformas | y = 354 | 167 px | **3** |
| Con las cifras en su PROPIA fila | y = 455 | **66 px** | **1** |
| Con las cifras dentro de la fila de tiles que ya existía | y = 354 | 167 px | **3** |

**Una fila propia para las tres cifras de plataformas cuesta 101 px y deja la tabla en un renglón.**
Por eso viven en la primera fila de tiles, que ya scrollea en horizontal: ahí cuestan cero alto. Lo
que impide que se resten con el total de ventas no es estar en otra fila — es que cada una lleva
sobre cuántos pedidos habla.

## La hoja de la liquidación

Cabe en 600 px con sus nueve campos, todos de al menos 44 px, gracias a las dos columnas: apilados
serían ~630 px. Con la ventana encogida a 350 px —lo que se lleva el teclado numérico— el botón de
guardar sigue dentro de la pantalla, que es lo que el footer fijo en `dvh` compra.

## Lo que sigue sin medirse

- **El teclado del sistema de verdad.** Chromium headless no lo abre; lo que el e2e mide es la
  ventana encogida a 350 px, que reproduce el efecto pero no el teclado. En una tableta real esto
  se verifica a mano.
- **El alto de Ventas con el aviso de turno viejo puesto.** Los números de arriba son sin él; el
  aviso desplaza el shell 53 px como en el POS.
