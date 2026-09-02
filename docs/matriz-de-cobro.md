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
| C4 | Reparto | Todo / entre 2 / 3 / 4, sin abrir el teclado | idem | vitest |
| C5 | Un cobro | Manda **una** llamada, con su llave | idem | vitest |
| C6 | Reintento tras un fallo | Manda **la misma** llave | idem | vitest |
| C7 | Cada pedazo cobrado | Estrena llave | idem | vitest |
| C8 | Con saldo pendiente | La hoja no se cierra; muestra lo ya cobrado | idem | vitest |
| C9 | Saldado | Se cierra | idem | vitest |
| C10 | Rebote de otra caja | Se traduce a algo accionable | idem | vitest |
| C11 | Sin métodos elegibles | Lo dice con palabras y no deja cobrar | idem | vitest |
| C12 | Pedido ya saldado al abrir | Lo dice; no ofrece cobrar | idem | vitest |
| C13 | "El cambio es propina" | Un toque, sin teclear | idem | vitest |

## D. La pantalla que cobra el carrito — **lo que falta**

Ninguno de estos tenía test cuando se escribió esta matriz, y cada uno es un defecto vivo.

| # | Caso | Qué debe pasar | Test | Estado |
|---|---|---|---|---|
| D1 | Borrar una línea del pago dividido y reintentar | La línea que queda **no** hereda la llave de la que se fue | — | **descubierto, sin arreglar** |
| D2 | Corregir los montos tras un fallo y reintentar | No queda atorado para siempre | — | **descubierto, sin arreglar** |
| D3 | Domicilio y **después** plataforma | No se cobra el envío que el servidor no va a cobrar | — | **descubierto, sin arreglar** |
| D4 | Propina mayor que la cuenta | No llega a crear el pedido | — | **descubierto, sin arreglar** |
| D5 | Sin método elegible | El botón no cobra ni manda `methodId: 0` | — | **descubierto, sin arreglar** |
| D6 | Envío mal escrito | No se convierte en envío gratis | — | **descubierto, sin arreglar** |
| D7 | Propina mal escrita | No cae a $0 en silencio | — | **descubierto, sin arreglar** |
| D8 | Encabezado y botón | Dicen la misma cifra | — | **descubierto, sin arreglar** |
| D9 | El diálogo de "pedido registrado" | Dice lo que **falta**, no el total | — | **descubierto, sin arreglar** |
| D10 | Rebote del servidor | Se traduce, no sale crudo | — | **descubierto, sin arreglar** |

## E. Extremo a extremo, en el navegador

| # | Caso | Qué debe pasar | Test | Estado |
|---|---|---|---|---|
| E1 | Cobrar un pedido completo | Sale de la barra y el corte lo cuenta una vez | — | pendiente |
| E2 | Repartir entre tres, con propina en dos | El corte separa las propinas por método | — | pendiente |
| E3 | Doble tap sobre el botón de cobrar | Un solo pago registrado | — | pendiente |
| E4 | Agregarle a un pedido ya pagado que sigue en cocina | Se puede | — | pendiente |
| E5 | Domicilio + plataforma | La pantalla no ofrece cobrar un envío fantasma | — | pendiente |

## Lo que esta matriz **no** cubre, y hay que decirlo

- **La terminal bancaria.** El sistema no se entera de que una tarjeta se declinó después del acuse.
  Por eso el cobro se registra de a un pedazo, en el instante en que el dinero está en la mano.
- **El conteo físico del cajón.** El arqueo compara lo esperado contra lo declarado; que el declarado
  sea cierto depende de quien cuenta.
- **La pantalla en una tableta real.** Las medidas se calculan contra el presupuesto de 1024×600; lo
  que se ve en la Surface se verifica a mano.
