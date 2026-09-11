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

## Una trampa al medir un diálogo: la animación de entrada

`boundingBox()` devuelve la caja **transformada**, y Chakra entra los diálogos con un `scale`. El
mismo botón del ticket mide **42 px** medido en cuanto el diálogo es "visible" y **44 px** ya
asentado.

Costó un falso hallazgo: se reportó que el botón de imprimir violaba el piso táctil de 44 px, y no
era cierto — la medición estaba mal, no el botón. **Un assert de píxeles sobre un diálogo espera a
que la animación termine**, no a `toBeVisible()`. Los tests de este repo esperan 600 ms.

# Medición del 9 de septiembre de 2026 — el conteo de efectivo (spec 003)

## `/caja` no tiene dónde poner una rejilla, y por eso el contador es una hoja

Estas cifras se midieron el **8 de septiembre** al planear la feature, y se traen aquí porque es
donde se buscan; lo que se midió el 9 es la hoja de la sección siguiente. Escenario **más vacío
posible** (turno con $0 en todo, sin pedidos pendientes, sin "sin cobrar", un solo cajero) a
1024×600:

| | |
|---|---|
| Alto total de `/caja` | **1,494 px** contra un viewport de 600 |
| Encabezado + tabs + chips + resumen del corte | **572 px** — deja 28 px de los 600 |
| Título "Cierre — declarado por método" | y = **727** |
| El renglón "Efectivo", donde iría la rejilla | y = **796** a 849 |
| Botón "Cerrar caja" | y = **1,426** |

Con cualquier dato real esos números solo empeoran. El punto de inserción ya está **200 px debajo
del fold** antes de dibujar el primer renglón, así que ninguna rejilla, por compacta que sea, cumple
el requisito de caber sin empujar el resumen fuera de vista.

## La hoja del contador: 552 px de 600

Medida con la hoja abierta y el catálogo cargado (once denominaciones en dos columnas, agrupadas en
billetes y monedas):

| | |
|---|---|
| Alto de la hoja | **552 px** de 600 (es el `maxH="92dvh"`) |
| La moneda de 50¢ —último renglón— | **alcanzable sin desplazarse** |
| El total y el botón de confirmar | visibles, incluso con la ventana recortada a 350 px |

El recorte a 350 px es cómo se mide el teclado numérico sin poder abrirlo: se come ~250 px de
ventana visual, y con `vh` en vez de `dvh` el botón se iría debajo de él.

**El margen es de 48 px, y no sobra tanto como parece**: agregar un renglón al encabezado de la
hoja —un subtítulo, un aviso permanente— se lo come. El aviso de "al cambiar se borra lo capturado"
es condicional a propósito.

## El cierre con un solo cajón, y Ajustes del negocio (spec 015)

Medido el **10 de septiembre** con [`presupuesto-del-cierre.spec.ts`](../web/e2e/presupuesto-del-cierre.spec.ts):
el "antes" contra `app-dev` con la spec 003 desplegada y el "después" contra el candidato servido
en local con la API nueva, **con los mismos diez métodos configurados**. El archivo que mide es el
mismo para los dos.

**Se mide el contenedor que se desplaza, no el documento.** El AppShell es `h="100dvh"
overflow="hidden"` y quien hace scroll es el `<Box flex="1" overflowY="auto">` que envuelve al
`<Outlet>`. La primera versión de esta medición leía `document.documentElement.scrollHeight` y
devolvía **600 px en las dos pantallas** —exactamente el viewport—, un número que se lee como si
todo cupiera. Es la misma trampa que la animación de un diálogo: el valor existe, contesta, y no
mide lo que uno cree.

| | Antes (003) | Después (015) |
|---|---|---|
| Tabla del cierre | **575 px** en 10 renglones | **540 px** en 11 |
| Puntos de captura en esa tabla | **7** (6 campos + el botón de contar) | **4** (3 campos + el botón) |
| Renglón que captura | 61 px | 61 px |
| Renglón que dice «Va al cajón» o «Automático» | 37 px | 37 px |

El renglón del cajón cuesta 61 px y los cuatro métodos que dejaron de capturar bajan de 61 a 37:
**−35 px netos con un renglón más**. El plan había afirmado que la tabla "se acorta" contando
campos quitados en vez de renglones, que es la afirmación que este archivo existe para no volver a
creer.

El **alto total de `/caja` no se compara** entre ambientes: depende de cuántas cajas, movimientos y
pedidos pendientes tenga el turno (1,867 px en el turno con ventas de `app-dev`, 1,467 px en el
turno casi vacío de local, contra los 1,494 px del escenario más vacío del 8 de septiembre). Lo
que sí es comparable —porque lo determina la configuración de los métodos y no el turno— es la
tabla.

### Ajustes del negocio: los métodos no se ven de un vistazo, ni antes ni después

| | Antes (003) | Después (015) |
|---|---|---|
| Alto del contenido de `/negocio` | **1,959 px** | **2,342 px** |
| Renglones de método visibles sin desplazarse | **0 de 10** | **0 de 10** |
| Dónde arranca la sección | y = **1,231** | y = **1,313** |
| Ancho útil de la página | 520 px (`<Page maxW="560px">` con su padding) | igual |
| Área tappable de un interruptor | **20 px** | **44 px** |
| Hueco entre dos interruptores del mismo renglón | no aplica (uno por renglón) | **46 px** (y 63 el otro) |

Los 82 px que baja la sección son el interruptor del arqueo ciego, que se le puso encima. No
cambia lo que se ve sin desplazarse porque la sección ya estaba 600 px por debajo del fold: esta
pantalla se lee desplazándose y así estaba antes.

**El hallazgo de medirla**: la tabla nueva puso tres interruptores de **24 px** de alto en un
renglón de 41, cuando la constitución pide 44 para cualquier control que se toque con el dedo. Tres
objetivos chicos y pegados en una página de 520 px es el caso que la regla nombra, y aquí un dedo
que falla por milímetros apaga «Activo» y saca un método del cobro a media jornada. Se arregló con
relleno vertical en el `<label>` del interruptor —el objetivo crece, el control no— y quedó su
aserción en P2.

**Y lo que la medición desmintió**: una revisión calculó el hueco horizontal entre los tres
interruptores en 16 px, sumando el padding de la celda de una `Table.Root size="sm"`. Medido son
**46 px**, porque el ancho de esas columnas no lo pone el padding sino los encabezados «Va al
cajón» y «Automático», más anchos que el interruptor de 48 px que contienen. La conclusión que
queda no es "no había problema" sino **dónde vive el riesgo**: el hueco depende del largo de un
encabezado, así que acortar uno lo cierra sin que nadie lo note. Por eso también se afirma en P2.

## Lo que sigue sin medirse

- **El teclado del sistema de verdad.** Chromium headless no lo abre; lo que el e2e mide es la
  ventana encogida a 350 px, que reproduce el efecto pero no el teclado. En una tableta real esto
  se verifica a mano.
- **El alto de Ventas con el aviso de turno viejo puesto.** Los números de arriba son sin él; el
  aviso desplaza el shell 53 px como en el POS.
- **El alto de una fila de Ventas CON folio capturado.** La celda "Tipo" pasa de una a dos líneas.
  El e2e lo mide cuando el periodo trae alguna, y lo DECLARA cuando no — que es lo que pasó en la
  corrida del 8 de septiembre, porque los folios de prueba se habían limpiado del ambiente.

## Y lo que estos casos cerraron de specs viejos

Dos tareas llevaban abiertas desde su spec porque exigían el ambiente desplegado y nadie las medía
a mano:

| Spec | Qué se midió |
|---|---|
| 005 · SC-005 | Con pedidos en curso, el mosaico conserva **3 renglones** y el catálogo sus 396 px: la barra flota (empieza en y=76, el catálogo llega a y=600) en vez de empujarlo |
| 001 · SC-006 | La vista previa del ticket abre contra el desplegado sin una sola petición que no salga y sin nada bloqueado por CSP. El papel sigue siendo manual |
