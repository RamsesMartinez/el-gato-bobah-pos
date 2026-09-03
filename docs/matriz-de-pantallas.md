# Matriz de pantallas — vender, pedidos y ventas

Hermana de [matriz-de-cobro.md](matriz-de-cobro.md). Aquella cubre **por dónde se pierde dinero**;
esta cubre **por dónde una pantalla dice algo que no es cierto**: una cifra sin su periodo, dos
tablas que responden rangos distintos, un filtro que se descarta en silencio, un control al que no
se le atina con el dedo.

Es **ejecutable**: cada renglón nombra el test que lo sostiene. Un renglón sin test no está cubierto,
y se dice. La columna "medido" distingue lo que se comprobó contra Postgres o contra el navegador de
lo que solo se razonó.

**Crece.** Cuando aparezca un caso nuevo se agrega su renglón *antes* de arreglarlo, con el test que
lo atrapa. Un caso que se arregla sin renglón vuelve.

## Regla de oro: toda cifra declara de qué periodo es

Una cifra sin su periodo al lado no se puede auditar. Una con el periodo **equivocado** al lado es
peor: se audita mal y nadie lo nota. Por eso el servidor devuelve el rango que **realmente** consultó
y la pantalla lo imprime, en vez de que la pantalla repita el rango que cree haber pedido.

Corolario que ya costó: **dos tablas de la misma pantalla se derivan del mismo predicado.** Si una
lleva cota superior y la otra no, quien lee no tiene forma de saber cuál de las dos cifras es la del
periodo que pidió.

---

## R. El rango de fechas — servidor

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| R1 | `preset` desconocido (`?preset=el-mes-pasado-pero-solo-martes`) | 400 de validación, **no** cae a "hoy" | `TestUnPresetDesconocidoSeRechaza` | Go |
| R2 | Rango invertido (`from=2026-08-31&to=2026-08-01`) | 400. Devolvería cero filas sin error y el operador creería que no vendió | `TestRangoLibre` | Go |
| R3 | Rango de años (`from=2020-01-01`) | 400: sin cota, el escaneo tumba el gigabyte de RAM del VPS | `TestRangoLibre` | Go |
| R4 | Rango libre con una sola fecha | 400: media fecha no es un rango | `TestRangoLibre` | Go |
| R5 | Fechas mandadas con un preset que no las usa (`?preset=hoy&from=2026-01-01`) | 400. Antes se descartaban en silencio y contestaba HOY con la pantalla viéndose perfecta | `TestUnasFechasQueElPresetNoVaAUsarSeRechazan` | Go |
| R6 | `preset=30d` | Treinta días **contando hoy**, no treinta y uno | `TestElPresetDeTreintaDiasSonTreintaDiasContandoHoy` | Go |
| R7 | "Hoy" a las 19:00 de México | El día del **negocio**, no el de UTC (que ya es mañana) | `TestResolveRangeUsaLaZonaDelNegocio` | Go |
| R8 | Fecha malformada (`from=31/08/2026`) | 400, nunca el default | `TestUnaFechaMalformadaNoCaeAlDefault` | Go |

## Q. Los reportes — un solo periodo por pantalla

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| Q1 | Venta de otro día dentro del alcance de "por medio de pago" | **No** aparece: la tabla lleva cota superior, igual que su hermana | `TestElReporteDeVentasNoMezclaDosPeriodos` | Postgres |
| Q2 | Venta reembolsada | No suma en "por medio de pago", igual que no suma en "venta por día" | `TestUnaVentaReembolsadaNoSumaEnLosMetodosDePago` | Postgres |
| Q3 | Producto vendido en otro día | No entra en "utilidad por producto" del rango pedido | `TestLaUtilidadPorProductoRespetaElRango` | Postgres |
| Q4 | Los tres reportes de la pantalla | Piden el **mismo** periodo | `ReportsPage.test.tsx › los tres reportes piden el mismo periodo` | Navegador |
| Q5 | Encabezado de la pantalla | Dice el periodo que el **servidor** consultó, no una frase fija | `ReportsPage.test.tsx › muestra el periodo que devolvió el servidor` | Navegador |

## F. El control de rango — pantalla

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| F1 | 31 de febrero | Se rechaza. `new Date('2026-02-31')` rueda al 3 de marzo, y consultar otro día se ve idéntico a consultar el pedido | `rangoDeFechas.test.ts › diaValido` | Navegador |
| F2 | Una sola fecha capturada | No se consulta; se dice qué falta y se conserva el periodo anterior | `rangoDeFechas.test.tsx › con una sola fecha no consulta` | Navegador |
| F3 | Rango invertido | No se consulta; se dice por qué, antes de ir al servidor | `rangoDeFechas.test.tsx › un rango invertido no consulta` | Navegador |
| F4 | Volver de "Rango" a un preset | Las fechas dejan de viajar (el servidor las rechaza) | `rangoDeFechas.test.tsx › al volver a Hoy deja de mandar las fechas` | Navegador |
| F5 | Elegir un día que no ha pasado | El campo lo topa con el día del **negocio**, no con el del navegador | `rangoDeFechas.test.tsx › no deja elegir un día que no ha pasado` | Navegador |
| F6 | Cruzar el cambio de horario | La cuenta de días no gana ni pierde uno | `rangoDeFechas.test.ts › cruzar el cambio de horario` | Navegador |
| F7 | Tope de 366 días | El 366 pasa, el 367 no — el mismo número que el servidor | `rangoDeFechas.test.ts › el tope son 366 días` | Navegador |

---

## Pendientes de cubrir

Renglones que este documento reconoce como **no cubiertos**. Están aquí porque un hueco nombrado se
arregla y uno olvidado no.

_(Se llena con el barrido de pantallas; ver la sección de hallazgos abajo.)_
