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

## N. Números de la frontera — servidor

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| N1 | `?limit=abc`, `?limit=0`, `?limit=-5` | 400. Antes contestaban las 50 filas del default | `TestUnEnteroDeLaFronteraNoCaeAlDefault` | Go |
| N2 | `?limit=3000000000` | 400. Truncado a int32 daba un `LIMIT` **negativo** → 500 por una petición que nunca fue válida | idem | Go |
| N3 | `?page=214748365` | 400. `int32(n) * limit` envolvía a 4 y contestaba la quinta página con un 200 limpio | `TestUnaPaginaQueDesbordaNoContestaOtra` | Go |
| N4 | `?pageSize=4294967297` | 400. Truncaba a 1 y pasaba la validación | idem | Go |
| N5 | El offset de la última página que cabe | Pasa; la siguiente se rechaza. El tope se **deriva** del tamaño | `TestElOffsetNuncaDesbordaInt32` | Go |
| N6 | `PATCH /payment-methods/abc` | 400, no 500 con un `slog.Error` de por medio | — | **no cubierto** |
| N7 | Cerrar caja con una llave de `declared` que no es un id | 400. Se descartaba en silencio y el corte comparaba contra cero: faltante inventado | — | **no cubierto** |

## D. Dinero clasificado una sola vez — Ventas

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| D1 | Venta cobrada y luego reembolsada | **No** suma en el desglose por medio de pago; ya la cuenta el tile de reembolsos | `TestElDesgloseDeMetodosNoCuentaLoReembolsado` | Postgres |
| D2 | Dos empleados con el mismo nombre | Dos renglones en propinas por empleado: uno solo no se puede repartir | `TestLasPropinasNoSeFusionanPorHomonimia` | Postgres |

## G. El rango en la pantalla — regresiones del propio filtro

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| G1 | Rango a medias en Reportes | Conserva las cifras y el periodo anteriores. Caía a `$0.00`, sin periodo y sin spinner | `ReportsPage.test.tsx › con media fecha conserva las cifras` | Navegador |
| G2 | Rango a medias en Ventas | El paginador se apaga: `paginas` es del periodo anterior | `rangoDeFechas.test.tsx › el paginador queda apagado` | Navegador |
| G3 | Fecha futura **tecleada** (no elegida en el calendario) | Se rechaza en los dos lados. El `max` del campo no impide teclear | `TestUnRangoQueTerminaEnElFuturoSeRechaza` + `rangoDeFechas.test.tsx` | Go y navegador |
| G4 | El tope del futuro a las 19:00 de México | Usa el día del **negocio**; con el reloj del servidor mañana pasaría por bueno | `TestElTopeDelFuturoUsaLaZonaDelNegocio` | Go |
| G5 | `Picker` con `size="sm"` | 44 px de alto. La receta del tema solo subía el piso en `md` | `Picker.test.tsx › el disparador mide al menos 44 px` | Navegador |

## V. El envío del POS — una sola decisión para tres superficies

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| V1 | Envío mal escrito (`1,000`) y cobrar desde la píldora | No cobra. Antes solo el panel se apagaba; la píldora mandaba el envío **por defecto** | `pantallas.spec.ts › V1` | Navegador |
| V2 | Recargar con envío capturado | Sobrevive: vive en la cuenta, no en la pantalla | `ticket.test.ts › cada cuenta lleva el suyo` | Navegador |
| V3 | Capturar envío y abrir otra cuenta | La nueva **no** lo hereda | idem | Navegador |
| V4 | Píldora y panel del mismo pedido a domicilio | Dicen el mismo total, envío incluido | `pantallas.spec.ts › V4` | Navegador |
| V6 | Envío mal escrito y luego cambiar a mostrador | Deja de bloquear: el campo ya no se pinta, así que no puede trabar sin nada que corregir | `Ticket.test.tsx › deja de bloquear` | Navegador |
| V7 | Envío ausente | El default del negocio, y **no** viaja al servidor | `envio.test.ts` | Navegador |
| V8 | Envío `0` explícito | Envío gratis decidido: viaja como cero | idem | Navegador |
| V10 | Vaciar una cuenta con plataforma activa | Borra también la plataforma y el envío | `ticket.test.ts › vaciar la cuenta` | Navegador |


---

## Pendientes de cubrir

Renglones que este documento reconoce como **no cubiertos**. Están aquí porque un hueco nombrado se
arregla y uno olvidado no. Cada uno cita el hallazgo del
[barrido](auditoria/barrido-de-pantallas-2026-09.md) que lo describe.

| # | Caso | Por qué todavía no | Hallazgo |
|---|---|---|---|
| X1 | Cancelar un pedido **ya cobrado** | Spec [007](../specs/007-devolver-el-dinero/spec.md), a la espera de cuatro decisiones del dueño | P1 |
| X2 | Reembolsar un entregado **sin cobrar** | Spec [007](../specs/007-devolver-el-dinero/spec.md) | P2 |
| X3 | Cancelar un renglón suelto | Spec [007](../specs/007-devolver-el-dinero/spec.md) | P4 |
| X6 | `POST /orders/:id/lines` sin llave de idempotencia | Un reintento duplica renglones y stock | V5 |
| X7 | Controles por debajo de 44 px en el ticket y el tablero | ~24 px en −/+/papelera; 32 px en el menú que cancela | V9, P3 |
| X8 | `window.prompt` para el motivo de cancelación | El diálogo lo pinta el sistema; Chrome lo puede suprimir y la acción deja de hacer nada en silencio | P10 |
