# Matriz de cobro — todos los caminos por los que se puede perder dinero

Este documento es **ejecutable**: cada renglón nombra el test que lo sostiene. Un renglón sin test no
está cubierto, y se dice. La columna "medido" distingue lo que se comprobó contra Postgres o contra
el navegador de lo que solo se razonó.

**Por qué existe.** Entre el 1 y el 2 de septiembre de 2026 se encontraron catorce formas de cobrar
mal, y ninguna la atrapó una revisión: salieron de medir. Tres las introdujo el propio arreglo de las
otras. Una lista de casos que vive en la cabeza de quien programó se pierde en la siguiente sesión;
esta vive junto a los tests y falla cuando alguien la contradice.

## Regla de oro: el servidor calcula, la pantalla pinta

**Ninguna cifra que se cobre se calcula en el front.** El servidor recalcula todos los precios al
crear el pedido y es el único que sabe cuánto falta. La pantalla puede mostrar un *anticipo* mientras
se arma la cuenta, pero en cuanto el pedido existe, lo que se cobra sale de `outstanding`.

Esa regla no es teórica: el anticipo y el total real ya divergieron. Un pedido marcado a domicilio y
después asignado a una plataforma sumaba $20 de envío que el servidor no cobra, y la pantalla ofrecía
cobrar $115 de un pedido de $95.

---

## A. El servidor — lo que ninguna pantalla puede saltarse

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| A1 | Dos cobros idénticos de media cuenta (doble tap) | El segundo es inocuo; queda **una** mitad cobrada y **una** propina | `TestUnDobleTapNoCobraDosVecesLaMismaMitad` | Postgres |
| A2 | La misma llave sobre **otro** pedido | `ErrConflict`; el segundo pedido no registra nada | `TestLaMismaLlaveEnOtroPedidoNoSeTomaComoReintento` | Postgres |
| A3 | La misma llave con **otro método** | `ErrConflict` | `TestLaMismaLlaveConOtroMetodoNoPasaPorReintento` | Postgres |
| A4 | La misma llave con **otro monto** | `ErrConflict` | idem | Postgres |
| A5 | La misma llave con **otra propina** | `ErrConflict` | idem | Postgres |
| A6 | Reintento de un cobro ya registrado, con la **caja cerrada** | Se reconoce (`yaEstaba`), no registra un pago nuevo | `TestUnCobroYaRegistradoSeReconoceConLaCajaCerrada` | Postgres |
| A7 | Cobro **nuevo** con la caja cerrada | `ErrNoOpenRegister` | `TestSinCajaAbiertaNoSeCobraNadaNuevo` | Postgres |
| A8 | Propina mayor que la cuenta entera | `ErrPropinaExcede` | `TestLaPropinaNoPuedeSuperarLaCuenta` | Postgres |
| A9 | Dos propinas generosas que suman más que la cuenta | **Se aceptan**: el tope es por pago, no acumulado | `TestDosPropinasPlausiblesNoSeBloqueanEntreEllas` | Postgres |
| A10 | Dividir $100 en tres partes de $33.33 | Queda saldado y **sin centavo** de deuda en ninguna vista | `TestUnPedidoCerradoNoDejaCentavosDeDeuda` | Postgres |
| A11 | El detalle del pedido y la respuesta del cobro | Dicen **la misma** cifra de faltante | `TestElDetalleDelPedidoDiceCuantoFalta` | Postgres |
| A12 | Método de pago desactivado | `ErrMetodoInactivo` | `TestUnMetodoDesactivadoNoCobra` | Postgres |
| A13 | Método de una plataforma sobre un pedido de mostrador (y al revés) | `ErrPaymentMethodPlatform` | `metodo_de_plataforma_test.go` | Postgres |
| A14 | Cobrar más de lo que falta | `ErrCobroExcede` | `TestSePuedeAbonarYLuegoCompletar` | Postgres |
| A15 | Cobrar un pedido cancelado o reembolsado | `ErrPedidoNoCobrable` | `TestNoSeCobraUnPedidoCancelado` | Postgres |
| A16 | Crear un pedido ya cobrado | `ErrCobroFueraDeLugar` | `cobrar_exige_confirmar_test.go` | Postgres |
| A17 | Un pedido de plataforma marcado a domicilio | El servidor **fuerza** el envío a 0 | `precios_plataforma_test.go` | Postgres |
| A18 | La barra pide `?porCobrar=true` | Solo lo que debe; el total sigue siendo la suma de lo listado | `TestLaBarraPuedePedirSoloLoQueFaltaPorCobrar` | Postgres |
| A19 | `?porCobrar` con un valor que no se entiende | 400 `ErrValidation`; **nunca** el default en silencio | `TestUnaBanderaMalEscritaNoSeLeeComoFalse` | unitario |
| A20 | Agregar renglones a un pedido ya ENTREGADO | Se acepta y el pedido vuelve a `abierta`, o cocina no prepara lo que ya se cobró | `TestElEntregadoQueRecibeMasVuelveACocina` | Postgres |
| A21 | Agregar renglones a un pedido cancelado o reembolsado | `ErrConflict`: su dinero ya lo contó un arqueo firmado | `TestAgregarAUnPedidoEnCurso` | Postgres |

## B. La aritmética del front — un solo lugar, con su prueba

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| B1 | Campo de dinero vacío | Es "pagó justo", **no** cero | `cobro.test.ts` › parseMonto | vitest |
| B2 | `1,000` con coma de millar | **Se rechaza**, no se lee como 1 | idem | vitest |
| B3 | `abc` | Se rechaza, no se disuelve en 0 | idem | vitest |
| B4 | `-50` | Se rechaza, no se clampa por detrás | idem | vitest |
| B5 | Repartir $100 en tres | `[33.33, 33.33, 33.34]`, suma exacta | idem › dividirEnPartes | vitest |
| B6 | Repartir un monto que dejaría una parte en $0 | Se rechaza | idem | vitest |
| B7 | Cobrar exactamente lo que falta | Se puede; un centavo más, no | idem › validarCobro | vitest |
| B8 | Sin método elegido | `sin-metodo`, nunca `methodId: 0` | idem | vitest |
| B9 | Efectivo recibido menor que el monto + propina | `falta-efectivo`, botón apagado | idem | vitest |
| B10 | Billetes ofrecidos | Solo los que alcanzan | idem › billetesUtiles | vitest |
| B11 | Cambio con decimales | Sin arrastre de flotantes | idem › cambioDeEfectivo | vitest |
| B12 | Porcentajes de propina | Sobre lo que se cobra ahora, no sobre otra base | idem › presetsDePropina | vitest |
| B13 | Redondeo en la frontera | `1.005` → `1.01` | idem › round2 | vitest |

## C. La pantalla que cobra un pedido existente

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| C1 | Encabezado | Dice el **total** y lo que **falta**, no una sola cifra | `CobrarSheet.test.tsx` | vitest |
| C2 | El faltante | Sale del pedido vivo, no de la foto que traía la lista | idem | vitest |
| C3 | Método | Ninguno preseleccionado: el tap es la confirmación | idem | vitest |
| C4 | Reparto | Cerrado no ocupa alto; abierto el número de partes es libre y el `+` se topa donde una parte quedaría en $0 | idem | vitest |
| C4b | Repartir de a uno | Cada parte sale del faltante VIVO; la última se lleva el residuo y la suma es exacta | idem · `cobro.test.ts` | vitest |
| C5 | Un cobro | Manda **una** llamada, con su llave | idem | vitest |
| C6 | Reintento tras un fallo | Manda **la misma** llave | idem | vitest |
| C7 | Cada pedazo cobrado | Estrena llave | idem | vitest |
| C8 | Con saldo pendiente | La hoja no se cierra; muestra lo ya cobrado | idem | vitest |
| C9 | Saldado | Se cierra | idem | vitest |
| C10 | Rebote de otra caja | Se traduce a algo accionable | idem | vitest |
| C11 | Sin métodos elegibles | Lo dice con palabras y no deja cobrar | idem | vitest |
| C12 | Pedido ya saldado al abrir | Lo dice; no ofrece cobrar | idem | vitest |
| C13 | "El cambio es propina" | Un toque, sin teclear | idem | vitest |

## C2. La hoja de modificadores — lo que el operador le dice al cliente

Es la cifra que se lee ANTES de agregar, y por eso importa aunque el carrito guarde bien: el
operador la canta en voz alta y el ticket sale después.

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| C14 | Producto con precio capturado a mano para una plataforma | El botón suma el precio DE LA LISTA, no el de mostrador | `ModifierSheet.test.tsx` › el total en la lista de una plataforma | vitest |
| C15 | Encabezado y botón de la misma hoja | Salen del mismo precio; con dos fuentes el botón decía $125 sobre un encabezado de $100 | idem | vitest |

## D. La pantalla que cobraba el carrito — **ya no existe**

Diez defectos vivían aquí. Siete se cerraron **borrando la pantalla**: el POS ya no tiene su propia
hoja de dinero, sino que crea el pedido y abre la misma hoja de cobro del botón naranja y del
tablero. No hay guardia más confiable que el archivo que no existe.

| # | Caso | Cómo quedó | Test |
|---|---|---|---|
| D1 | Borrar una línea del pago dividido y reintentar heredaba la llave de la que se fue | **borrado con la pantalla** | — |
| D2 | Corregir los montos tras un fallo dejaba la cuenta atorada para siempre | **borrado con la pantalla** | — |
| D3 | Domicilio y **después** plataforma cobraba un envío fantasma | `cobraEnvio`, un solo lugar | `pedido.test.ts`, `Ticket.test.tsx` |
| D4 | Propina sin tope dejaba el pedido creado y sin cobrar | **borrado**; la hoja que queda la topa antes de mandar | `cobro.test.ts` |
| D5 | Sin método elegible mandaba `methodId: 0` | **borrado**; la hoja que queda apaga el botón | `CobrarSheet.test.tsx` |
| D6 | Envío mal escrito = envío gratis | El panel lo rechaza y apaga los botones | `Ticket.test.tsx` |
| D7 | Propina mal escrita caía a $0 | **borrado**; `parseMonto` distingue inválido de cero | `numeros.test.ts` |
| D8 | Encabezado y botón decían cifras distintas | **borrado**; la hoja que queda pinta una sola | `CobrarSheet.test.tsx` |
| D9 | El diálogo decía el total en vez del saldo | Usa `outstanding` | — (verificado a mano) |
| D10 | Rebotes del servidor crudos | **borrado**; la hoja que queda los traduce | `CobrarSheet.test.tsx` |

## E. Extremo a extremo, contra el ambiente desplegado

Playwright, a 1024×600, contra `app-dev` y `api-dev` de verdad. Lo que estas atrapan y las de vitest
no es el **desacuerdo entre la pantalla y el servidor**: con el backend simulado los dos están de
acuerdo por construcción, y de esa forma fueron todos los defectos caros de este sistema.

Se corren con `bun run e2e` (en contenedor, como el resto de los gates).

| # | Caso | Qué debe pasar | Test |
|---|---|---|---|
| E0 | La barra y su lista | La cifra del encabezado es la suma de la lista | `dinero.spec.ts` |
| E1 | COBRAR desde el POS | Crea el pedido y abre **la** hoja de cobro, sin método preseleccionado | `cobro-en-pantalla.spec.ts` |
| E1b | Cobrar en efectivo | El pedido queda saldado y la confirmación dice Cobrado, no "falta" | idem |
| E2 | Repartir entre tres | Queda saldado, sin centavos colgando, y cada cobro devuelve el faltante correcto | `dinero.spec.ts` |
| E3 | Doble tap sobre cobrar | Un solo pago; con otro método se rechaza | idem |
| E5 | Pedido de plataforma | El servidor no cobra el envío del negocio y el detalle dice con qué lista se armó | idem |
| E5b | Domicilio y **después** plataforma, en la pantalla | El campo de envío desaparece | `cobro-en-pantalla.spec.ts` |
| E6 | Envío con coma de millar | Lo dice y apaga COBRAR | idem |
| E7 | El alto de la hoja de cobro | Cabe en 600 px con y sin repartir, y el repartidor solo cuesta alto cuando se usa | `cabe-en-la-tableta.spec.ts` |

### Lo que la matriz E encontró y ninguna otra prueba veía

**El POS dejaba de poder vender.** `POST /orders` respondía 500 con un choque del índice único de
nombres del día. Cuando la pantalla empezó a proponer el nombre, el camino donde el servidor lo
asigna se quedó sin verificar si ya estaba usado — y en cuanto la lista de animales se consume a
medias, el que le toca por folio numérico ya está tomado. Medido: el pedido **24** del día. La única
salida del operador era esperar al día siguiente. Cubierto por
`folio_no_tumba_la_venta_test.go`.

## F. La liquidación de plataforma NO es dinero de la caja (spec 014)

Renglones **abiertos**, escritos antes que el código. La comisión de una plataforma es dinero que
el negocio no tuvo, y el modo de falla que esta sección vigila es que alguien la reste de una venta.

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| F1 | Registrar una liquidación | El resumen de Ventas, el corte de caja y el arqueo devuelven **exactamente lo mismo** que antes | `TestRegistrarUnaLiquidacionNoMueveNingunaVenta` | Postgres |
| F2 | Escribirle el folio a un pedido de un arqueo **ya cerrado** | Ninguna cifra se mueve. El test falla nombrando la que se movió | `TestCorregirElFolioNoMueveNingunaCifra` | Postgres (respaldo real) |
| F3 | Las tres cifras del periodo | `vendido`, `se quedó la plataforma` y `llegó al banco` cubren conjuntos distintos; `llegó al banco` **no** es la resta de las otras dos | `TestLoQueLlegoAlBancoNoEsLaRestaDeLasOtrasDos` · `TestElResumenDePlataformasDelPeriodo` | Go + Postgres |
| F4 | Neto negativo (promoción que financió el restaurante) | Se acepta y se muestra negativo: es lo que de verdad pasó | `TestElNetoNegativoSeAceptaPorqueEsLoQueDeVerdadPaso` | Postgres |
| F5 | Comisión negativa, tasa fuera de rango, parte del descuento mayor que el total | 400 de validación, nunca 500 | `TestLoQueUnaLiquidacionRechaza` · `TestLaLiquidacionRechazaLoQueUnDocumentoNoPuedeDecir` | Go + Postgres |
| F6 | Recapturar desde un documento corregido | Reemplaza, no duplica; queda una sola liquidación | `TestUnDocumentoCorregidoReemplazaLaLiquidacionYNoLaDuplica` | Postgres |
| F8 | Rechazar un importe con exponente absurdo (`1e100000000`) | Rechazo en milisegundos. El mensaje de error **no** expande el número: hacerlo tardaba 77 s y comía memoria | `TestRechazarUnImporteAbsurdoEsBarato` | Go |
| F7 | Liquidación con sesión de cajero | 403: es dinero que no pasó por la caja | `TestLaLiquidacionExigeRolDeAdministracion` | Postgres |

## G. Un pedido entregado y nunca cobrado, nombrado en el arqueo

**Cómo se encontró.** El 8 de septiembre de 2026, corte 5 del ambiente de pruebas: cinco pedidos en
estado `entregada` por **$554.00** sin un solo renglón en `order_payments`. El turno cerró con los
diez métodos en **diferencia $0.00** —cuadró perfecto— mientras la lista de ventas de ese mismo
corte decía **$1,410.50** contra **$856.50** esperados. El arqueo cuadraba por construcción: solo
compara pagos contra declarado, y una venta sin pago no aparece por ningún lado. Para ver el hueco
había que restar dos cifras de dos pantallas distintas.

**Lo que NO se cambió, a propósito.** La guardia del cierre sigue bloqueando por *comida sin
entregar* y no por dinero (`pedidosSinEntregar`). Entregar sin cobrar es una decisión legítima del
negocio —se fio, se cobró por fuera— y convertirla en un bloqueo detendría la operación en hora
pico. El hueco nunca fue la guardia: era que ninguna cifra lo **declaraba**.

**Lo que se agregó.** `UncollectedInSession` — la venta del turno que ningún pago cubre, y en
cuántos pedidos está. Sale del **mismo** `register_session_id` que la lista de ventas y que el
esperado por método, así que las tres cifras de la pantalla hablan del mismo conjunto. Viaja en las
dos vistas: la del turno abierto (`SessionView`, junto a `Pending` — aquélla dice qué comida no ha
salido, ésta qué dinero no entró) y la del corte cerrado (`SessionDetailView`), que es la que
alguien audita cuando ya nadie se acuerda del turno.

En pantalla, el arqueo muestra *"Sin cobrar: $X en N pedidos"* antes del botón de cerrar, para que
se vea mientras se cuenta el efectivo y no después.

| Qué lo cubre | Dónde |
|---|---|
| La cifra existe, no cuenta lo ya cobrado, sobrevive al cierre y la resta cierra en el detalle | `TestElArqueoDiceLoQueSeEntregoSinCobrar` |
| Contra el servidor real: lo vendido = lo cobrado + lo declarado sin cobrar | `fecha-y-folio.spec.ts` › **U2** |

**U2 cambió con esto y no se aflojó.** Antes comparaba lo vendido contra el esperado a secas —así
encontró el corte 5—; ahora exige que lo vendido quede explicado por *esperado + sin cobrar*. Sigue
fallando si aparece un peso que no está ni cobrado ni nombrado; lo que ya no hace es fallar porque
la pantalla calle un hecho que ahora declara.

Lo que sigue sin cubrir: **los cinco pedidos del corte 5 no se tocaron.** Cobrarlos ahora los
metería en el turno siguiente y reescribiría un arqueo firmado, por la misma razón que X18 de
[matriz-de-pantallas.md](matriz-de-pantallas.md) sigue abierta. El corte 5 queda como está, ahora
con su $554.00 declarado.

## H. El arqueo de efectivo por denominaciones (spec 003)

Por dónde se pierde dinero **al contar el cajón**. El agujero que esto cierra no está en el cobro
sino en el arqueo: el operador sumaba a mano en una libreta y de ahí nacían faltantes que después
nadie podía explicar — un turno cerró con $1,662 de descuadre que era una suma mal hecha, no dinero
perdido.

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| H1 | Piezas contadas y total escrito, los dos para el mismo arqueo | `ErrConteoAmbiguo`: son dos cifras del mismo dinero y no hay forma de saber cuál manda | `TestComoSeDeclaraElEfectivo`, `TestMandarConteoYDeclaradoDelEfectivoSeRechaza` | Postgres |
| H2 | El cliente manda piezas **y** un total propio | El servidor **recalcula** desde las piezas y descarta el total del cliente | `TestElConteoDeAperturaAlimentaElFondo` | Postgres |
| H3 | Total escrito a mano sin motivo (al abrir **y** al cerrar) | `ErrConteoSinExplicar`: un arqueo sin desglose y sin explicación no se puede auditar | `TestDeclararElEfectivoDelCierreAManoExigeMotivo` | Postgres |
| H4 | Motivo de puros espacios, o de caracteres invisibles (`U+200B`) | Se rechaza: en pantalla se ve vacío igual | `TestComoSeDeclaraElEfectivo` | Unitario |
| H5 | Motivo más largo que el tope del esquema | 400 y no un `23514` que sube como 500 — el cuerpo de un 500 se registra en el log con el texto del operador dentro | `TestComoSeDeclaraElEfectivo` | Unitario |
| H6 | La misma denominación dos veces en el conteo | 400 nombrando la denominación, **antes** de escribir nada. El dominio suma los dos renglones sin poder saber que son el mismo billete | `TestUnRenglonRepetidoSeRechazaComoCapturaInvalida` | Postgres |
| H7 | Un conteo que desborda el tope de dinero (mil billetes de $1000) | 400 accionable, y el tope se evalúa sobre el TOTAL y no pieza por pieza | `TestTotalDelConteo` | Unitario |
| H8 | Piezas fraccionarias (`1.5`) o con coma de millar (`1,000`) en el campo | Se rechazan visiblemente; **no** caen a cero en silencio | `conteo.test.ts` | Navegador (vitest) |
| H9 | El conteo del cierre alimenta el declarado del efectivo | Y de **ningún otro método**: es el mismo error que sumó el fondo cuatro veces y reportó $4,500 de faltante | `TestElConteoDeCierreAlimentaSoloElDeclaradoDelEfectivo` | Postgres |
| H10 | El faltante del cierre | Sale de la columna **generada** de la base, no de una resta escrita a mano | `TestElFaltanteDelCierreSaleDeLaColumnaGenerada` | Postgres |
| H11 | Una apertura que falla a medias | Ni sesión huérfana ni caja bloqueada: las tres escrituras van en una transacción | `TestUnaAperturaQueFallaNoDejaLaCajaBloqueada` | Postgres |
| H12 | Dos tabletas abren la misma caja al mismo tiempo | La segunda ve un conflicto accionable, no un 500 | `TestDosAperturasSimultaneasDejanUnConflictoYNoUn500` | Postgres |
| H13 | Un conteo de cierre que ya existe | Conflicto, y el cierre **no queda a medias**: turno abierto y cero totales escritos | `TestUnConteoDeCierreQueYaExisteNoSePisaYElCierreNoQuedaAMedias` | Postgres |
| H14 | Cerrar sin declarar efectivo | No se inventa un conteo en cero: "conté y estaba vacío" es un hecho distinto de "nadie contó" | `TestCerrarSinDeclararEfectivoNoInventaUnConteo` | Postgres |
| H15 | Lo que la pantalla suma y lo que el servidor guarda | La misma cifra: se cuentan 40 monedas de $10 y 3 de $50 y el turno abre con $550 | `contar-el-cajon.spec.ts` › **C4** | Navegador + servidor |
| H16 | La diferencia antes de confirmar el cierre | Visible **antes** de tocar el botón, y con lo no capturado en `—` y no en cero | `cierreDeCaja.test.ts`, `CashPage.test.tsx` | Navegador (vitest) |

**Lo que H no cubre, y es la mitad que importa:** que las piezas que el operador teclea sean las que
de verdad hay en el cajón. El sistema quitó la suma manual —que es de donde venían los faltantes
inventados— pero **el conteo físico sigue dependiendo de quien cuenta**. Un cajón mal contado ahora
produce un arqueo consistente con un conteo equivocado, y la única señal es la diferencia contra lo
esperado.

## Lo que esta matriz **no** cubre, y hay que decirlo

- **La terminal bancaria.** El sistema no se entera de que una tarjeta se declinó después del acuse.
  Por eso el cobro se registra de a un pedazo, en el instante en que el dinero está en la mano.
- **El conteo físico del cajón.** El arqueo compara lo esperado contra lo declarado; que el declarado
  sea cierto depende de quien cuenta.
- **La pantalla en una tableta real.** Las medidas se calculan contra el presupuesto de 1024×600; lo
  que se ve en la Surface se verifica a mano.
