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
| V1 | Envío mal escrito (`1,000`) y cobrar desde la barra o la píldora | No cobra. Antes solo el panel se apagaba; las otras dos mandaban el envío **por defecto** | `envio.test.ts` cubre la decisión; el **cableado** no está cubierto extremo a extremo | Razonado |
| V2 | Recargar con envío capturado | Sobrevive: vive en la cuenta, no en la pantalla | `ticket.test.ts › cada cuenta lleva el suyo` | Navegador |
| V3 | Capturar envío y abrir otra cuenta | La nueva **no** lo hereda | idem | Navegador |
| V4 | Barra/píldora y panel del mismo pedido a domicilio | Dicen el mismo total, envío incluido | `envio.test.ts` cubre la decisión; el **cableado** no está cubierto extremo a extremo | Razonado |
| V6 | Envío mal escrito y luego cambiar a mostrador | Deja de bloquear: el campo ya no se pinta, así que no puede trabar sin nada que corregir | `Ticket.test.tsx › deja de bloquear` | Navegador |
| V7 | Envío ausente | El default del negocio, y **no** viaja al servidor | `envio.test.ts` | Navegador |
| V8 | Envío `0` explícito | Envío gratis decidido: viaja como cero | idem | Navegador |
| V10 | Vaciar una cuenta con plataforma activa | Borra también la plataforma y el envío | `ticket.test.ts › vaciar la cuenta` | Navegador |
| V8 | Agregar a un pedido con un producto ya desactivado en el carrito | Se recorta ese renglón, igual que al confirmar; no se tira el carrito entero | `agregarRecorta.test.tsx` | Navegador |
| V7 | Doble tap en COBRAR con red lenta | El botón se apaga mientras el pedido viaja, igual que "Enviar a cocina" | — | **no cubierto** (se ve en red lenta, no en jsdom) |
| V11 | El cableado del envío en las superficies de cobro, a 1024×600 | Las tres cifras coinciden y las tres se apagan igual | — | **no cubierto**: el intento de e2e partió de una suposición falsa (a 1024×600 el POS es modo angosto, no hay píldora) |

## P. El tablero de pedidos

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| P5 | Motivo de cancelación con un solo espacio | Se rechaza. Pasaba los dos lados y el `check` de la base lo daba por bueno: histórico con una cancelación sin motivo | `TestUnMotivoEnBlancoSeRechaza` | Go |
| P5b | Motivo de 10,000 caracteres | Se rechaza; el tope son 200 **caracteres**, no bytes | `TestUnMotivoDesmedidoSeRechaza` | Go |
| P6 | Pedido de total $0 en la barra | **No** cuenta como por cobrar: el badge decía "1 por cobrar · $0" y ninguna tarjeta ofrecía Cobrar | `porCobrar.test.ts › un pedido de $0` | Navegador |
| P7 | Doble tap en "Entregar todo" | El segundo es un no-op, no un error rojo sobre una entrega que sí ocurrió | `TestUnDobleTapEnEntregarTodoNoDaError` | Postgres |
| P9 | Error de red al entregar | Un mensaje accionable, no `TypeError: Failed to fetch` | `mensajes.test.ts` | Navegador |
| P3 | Renglones del menú ⋮ del tablero | 44 px, y "Cancelar pedido" separado de "Reimprimir comanda" | — | **no cubierto** (se mide en el navegador real) |

## A. Sesiones y relevo entre estaciones

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| A1 | Re-login masivo de la migración 0052 | **Caduca** las sesiones, no las revoca: revocar clasifica el siguiente refresh como ROBO | `TestDosEstacionesConLaMismaCuentaNoSeRevocanEntreEllas` | Postgres |
| A2 | Dos estaciones con la misma cuenta tras el re-login masivo | Una no tumba a la otra. Revocando, se tumbaban cada ≤15 min indefinidamente | idem | Postgres |
| A3 | Un reuso de credencial de verdad | **Sigue** revocando: el arreglo no afloja la detección de robo | `TestUnReusoDeVerdadSigueRevocando` | Postgres |
| A4 | Rebote de `/auth/refresh` | Borra la cookie. Sin eso, la credencial muerta se re-presenta en cada recarga hasta 30 días | — | **no cubierto** |

## E. Devolver dinero (spec 007)

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| E1 | Devolver más de lo cobrado | Se rechaza. `Refund` anotaba como pérdida el TOTAL del pedido sin mirar un cobro | `TestSeDevuelveLoCobradoNoElTotalDelPedido` | Postgres |
| E2 | Devolver un pedido sin cobrar | Se rechaza con su propio error, y no anota pérdida | `TestUnPedidoSinCobrarNoSeDevuelve` | Postgres |
| E3 | Devolver lo cobrado con tarjeta | **No** toca el cajón: ese dinero nunca estuvo ahí | `TestSoloLaDevolucionEnEfectivoTocaElCajon` | Postgres |
| E4 | Devolver lo cobrado en efectivo | Sale del cajón como movimiento, y el arqueo lo descuenta solo | idem | Postgres |
| E5 | Cancelar un pedido ya cobrado | Se rechaza sin devolución; con ella, el cajón queda cuadrado | `TestCancelarUnPedidoCobradoExigeLaDevolucion` | Postgres |
| E6 | Devolver por un método desactivado | Se permite: el dinero entró por ahí y por ahí sale | `TestSeDevuelvePorUnMetodoDesactivado` | Go |
| E7 | Devolver dos veces el mismo renglón | El tope es lo cobrado de ESE renglón | `TestNoSeDevuelveMasDeLoQueEntro` | Go |
| E8 | Cancelar un renglón NO enviado a cocina | Repone el insumo y baja el total | `TestCancelarUnRenglonReponeSoloSiNoSalioACocina` | Postgres |
| E9 | Cancelar un renglón YA enviado a cocina | Baja el total y **no** repone: el insumo se consumió | `TestUnRenglonQueYaSalioACocinaNoRepone` | Postgres |
| E10 | Doble tap al cancelar un renglón | No repone dos veces ni da error | `TestCancelarDosVecesElMismoRenglonNoReponeDosVeces` | Postgres |
| E11 | La tarjeta de un entregado sin cobrar | **No** ofrece "Devolver": el servidor lo rechazaría | `OrdersBoardPage` (SC-003) | Navegador |
| E12 | La hoja de devolución | Propone lo que queda, descuenta lo ya devuelto, y dice por qué se apaga | `DevolucionSheet.test.tsx` | Navegador |
| E15 | Quitar un renglón ya enviado a cocina | **Avisa** que el ingrediente no vuelve, antes de confirmar | `CancelarRenglonDialog.test.tsx` | Navegador |
| E16 | Quitar un renglón | Pide confirmar: no borra al tocar | idem | Navegador |
| E17 | Reporte de devoluciones vs salidas del cajón | Cuadran en la parte en efectivo | `TestElReporteDeDevolucionesCuadraConLoQueSalioDelCajon` | Postgres |
| E18 | El error de entrega parcial | Lo que dice ("cancela los que falten") ahora **se puede hacer** | `TestLoQueElErrorDeEntregaParcialDiceSePuedeHacer` | Postgres |
| E13 | El grant de la tabla nueva | El rol de app puede leer e insertar; sin grant es 42501 en producción | `TestElLibroDeDevolucionesEsUsablePorElRolDeApp` | Postgres |
| E14 | Un arqueo ya cerrado tras la migración | Mismas cifras | `TestUnArqueoCerradoNoCambiaConLaMigracion` | Postgres |


---

## T. La fecha la da el reloj, el folio lo da el turno (spec 008)

El defecto que lo abrió: `orders.business_date` se HEREDABA del turno de caja, sin techo. Medido el
2026-09-04 en el ambiente de pruebas — el turno abrió el 31 de agosto y nadie lo cerró, así que 158
pedidos y $6,664 quedaron archivados como 31 de agosto y la pantalla de Ventas del día salía vacía
con el negocio vendiendo.

| # | Caso | Qué debe pasar | Test | Medido |
| --- | --- | --- | --- | --- |
| T1 | Turno abierto hace cuatro días, venta de hoy | Se archiva con la fecha de HOY, y sigue perteneciendo a ese turno | `TestLaVentaSeArchivaEnElDiaEnQueOcurrioYNoEnElDelTurno` | Go |
| T2 | Turno que cruza la medianoche | La fecha cambia de día, el folio NO se reinicia | `TestElFolioSigueAlTurnoAunqueCruceLaMedianoche` | Go |
| T3 | Ocho cobros simultáneos del mismo turno | Folios distintos y consecutivos: el candado de fila sigue serializando | `TestDosCobrosSimultaneosNoCompartenFolio` | Go |
| T4 | Turno que ya traía folios repartidos al migrar | Continúa en N+1; sin la semilla pediría el 1 y chocaría con un 23505 | `TestUnTurnoConFoliosRepartidosContinuaLaNumeracion` | Go |
| T5 | Cerrar y reabrir la caja el mismo día | Renumera desde 1 y no colisiona, porque cerrar exige que no queden pedidos vivos | `TestReabrirLaCajaElMismoDiaRenumeraSinColisionar` | Go |
| T6 | `folio_counters` bajo el rol de la app | Cobra sin 42501: el grant de 0024 fue puntual y no hay default privileges | `TestElFolioSeReparteBajoElRolDeLaAplicacion` | Go |
| T7 | Contador colgado del turno de otra empresa | Lo rechaza el ESQUEMA (23503), no un servicio: los chequeos de FK saltan RLS | `TestElEsquemaRechazaUnContadorDeFolioQueCruzaEmpresas` | Go |
| T8 | La corrección histórica (0062) con dos empresas | Corrige la fecha; deja intactos folio, turno y toda cifra de arqueo; y se revierte | `TestLaMigracionCorrigeElDiaSinMoverDineroDeArqueo` | Go |
| T9 | Zona con horario de verano el día del cambio | El día se resuelve preguntándole a la zona, nunca restando 24 h | `TestTurnoDeOtroDia` | Go |

## U. Las ventas de un corte y el aviso de turno viejo (spec 008)

| # | Caso | Qué debe pasar | Test | Medido |
| --- | --- | --- | --- | --- |
| U1 | Corte con una venta cancelada | Se LISTA (pasó en el turno) pero NO suma al total: su dinero no entró | `TestElDetalleDelCorteListaSusVentasYSoloSumaElIngreso` | Go |
| U2 | Dos turnos el mismo día | Cada detalle trae solo lo suyo. El filtro es por turno, no por ventana de tiempo | `TestElDetalleDeUnCorteNoTraeVentasDeOtro` | Go |
| U3 | Corte con más ventas que el tope de 200 | La pantalla dice cuántas hay EN TOTAL: un recorte silencioso se lee como "esto es todo" | `el detalle dice cuántas ventas hay cuando muestra solo una parte` | Vitest |
| U4 | El total del corte | Declara en pantalla que deja fuera canceladas, reembolsadas y propinas | `el total de las ventas del corte declara qué deja fuera` | Vitest |
| U5 | Corte sin ninguna venta | Lo dice con una frase; una tabla vacía parece un error de carga | `un corte sin ventas lo dice en vez de pintar una tabla vacía` | Vitest |
| U6 | Turno abierto ayer a las 23:00, lleva una hora | Avisa: compara DÍAS, no horas transcurridas | `TestElEstadoDeCajaAvisaCuandoElTurnoEsDeOtroDia` | Go |
| U7 | Con el aviso visible | La pantalla de venta sigue completa: el aviso nunca bloquea el cobro | `el aviso de turno viejo se ve y NO bloquea la pantalla de venta` | Vitest |
| U8 | Turno abierto hoy | Sin aviso. Un aviso permanente se vuelve ruido y se aprende a ignorar | `un turno abierto hoy no muestra el aviso` | Vitest |
| U9 | Backend viejo, sin el campo `deOtroDia` | Sin aviso, nunca uno inventado: el front se despliega ~7 min antes que el backend | `sin el campo del backend no se inventa un aviso` | Vitest |

## V2. Los huecos declarados que se cerraron (barrido de 2026-09-04)

Siete renglones que este documento reconocía como no cubiertos. Cada uno cita qué defecto atrapa su
prueba; el que no atrapa ninguno no está aquí.

| # | Caso | Qué debe pasar | Test | Medido |
| --- | --- | --- | --- | --- |
| X6a | El mismo lote de renglones se agrega dos veces | Ni se cobra de más ni se descuenta el inventario dos veces; cocina no lo reimprime | `TestAgregarElMismoLoteDosVecesNoDuplicaNiCobraDeMas` | Go |
| X6b | Un lote llega dirigido a otro pedido | Rebota diciendo a cuál se aplicó. La comida no se le carga a una cuenta ajena | `TestUnLoteDeRenglonesNoSeAplicaAOtroPedido` | Go |
| X6c | Un cliente viejo agrega sin llave | Sigue funcionando. Es un techo consciente, no un descuido | `TestAgregarSinLlaveSigueFuncionando` | Go |
| X7 | Los tres controles del renglón del ticket | Miden ≥44 px reales y la papelera va al extremo opuesto de los de cantidad | `X7 · los controles del renglón del ticket miden 44 px` | Playwright |
| X9 | Entregar un pedido | Publica evento: la otra tableta deja de ofrecer comida que ya salió | `TestEntregarPublicaEventoParaLaOtraTableta` | Go |
| X10 | El rótulo de "Entregadas" con los tres modos de corte | Nombra la ventana que el negocio configuró, y el vacío usa la misma | `el título dice la ventana que el negocio configuró` | Vitest |
| X11a | Reuso de credencial con dos estaciones en la misma cuenta | Corta solo la cadena comprometida; la otra tableta sigue trabajando | `TestElReusoRevocaSoloLaFamiliaComprometida` | Go |
| X11b | Reuso de una credencial sin familia (anterior a 0064) | Cae al castigo viejo —revocar por usuario— en vez de no cortar nada | `TestUnaCredencialSinFamiliaSigueRevocandoPorUsuario` | Go |
| X15 | Un corte con más ventas que el tope de página | Se pueden traer las siguientes sin salir del corte | `SessionSalesPage` + "Ver más" | Go |

**X12** no lleva test: es orden de despliegue, y se verifica leyendo el grafo de `ci.yml` —
`deploy-frontend` ahora depende de `deploy-backend`.

## W. Cuándo nace el pedido, y el papel de la cuenta

Tocar COBRAR creaba el pedido y lo mandaba a cocina, así que un toque por equivocación —el botón
vive junto al total, en la barra que se toca todo el día— dejaba comida preparándose. Ahora el
pedido nace al tocar el botón final. Y como en la hoja de cobro puede no haber todavía ningún
pedido, imprimir la cuenta pasa por un papel propio.

**La barrera de la 005 no se toca**: "no se cobra un pedido que cocina no ha visto" vive en el
servidor, y el pedido se sigue creando ANTES de cobrarse.

| # | Caso | Qué debe pasar | Test | Medido |
| --- | --- | --- | --- | --- |
| W1 | Tocar COBRAR | Abre la hoja y **no** crea el pedido. Medido contra el servidor: los pedidos en curso no aumentan | `E1 · COBRAR abre la hoja y NO manda el pedido a cocina` | Playwright |
| W2 | El botón final del cobro | Crea el pedido y luego lo cobra, contra ese mismo id | `el botón final crea el pedido y luego lo cobra` | Vitest |
| W3 | Dividir cruzando el momento en que el pedido nace | El pedido se crea UNA vez y los dos pedazos van contra él. Sin esto, cada comensal creaba su propia cuenta y cocina recibía la misma comanda tres veces | `al dividir, el segundo pedazo cobra el pedido que creó el primero` | Vitest |
| W4 | Cerrar la hoja sin cobrar | No hay aviso de "ya está en cocina": no pasó nada y decirlo sería mentir | `abrir la hoja sobre una cuenta sin confirmar no crea el pedido` | Vitest |
| W5 | Una cuenta sin confirmar | No finge tener folio del servidor: dice "Sin confirmar" | `una cuenta sin confirmar no finge tener folio del servidor` | Vitest |
| W6 | El papel de una cuenta sin confirmar | Lleva `** PRE-CUENTA **` | `lleva la marca de pre-cuenta` | Vitest |
| W7 | Ese papel y el número de pedido | No lo lleva: no existe, y uno inventado no coincidiría con el ticket | `no lleva número de pedido` | Vitest |
| W8 | Ese papel y el estado del cobro | No lo lleva. "POR COBRAR" también sale en el ticket de un pedido REAL sin cobrar, y los dos papeles se parecerían justo donde deben distinguirse | `no lleva el estado del cobro` | Vitest |
| W9 | Ese papel y el mensaje del negocio | Sí lo lleva: es identidad, no estado | `conserva el mensaje del negocio` | Vitest |
| W10 | El total del papel | Incluye el envío y coincide al centavo con lo que se va a cobrar | `el total incluye el envío y coincide` | Vitest |
| W11 | El ticket de una venta real | Sigue con su número y su estado de cobro, y sin la marca de pre-cuenta | `el ticket normal conserva su número y su estado de cobro` | Vitest |
| W12 | El papel a 1024×600 | El botón mide ≥44 px, la hoja cabe en 600 px, y el papel sale marcado y sin número | `T-cuenta · el papel de la cuenta sale marcado` | Playwright |
| W13 | Ver el ticket desde la lista del botón naranja | Trae el pedido completo y la lista se queda abierta detrás | `desde la lista se puede ver el ticket de un pedido sin cobrarlo` | Vitest |
| W14 | El papel de un pedido pagado | Sale marcado `** REIMPRESIÓN **`: el original ya circuló | `marca el papel como reimpresión cuando el pedido YA se pagó` | Vitest |
| W15 | El papel de un pedido en curso que SÍ existe | Sale con `POR COBRAR` y sin marca de reimpresión | `un pedido sin cobrar NO se marca como reimpresión` | Vitest |

## Pendientes de cubrir

Renglones que este documento reconoce como **no cubiertos**. Están aquí porque un hueco nombrado se
arregla y uno olvidado no. Cada uno cita el hallazgo del
[barrido](auditoria/barrido-de-pantallas-2026-09.md) que lo describe.

| # | Caso | Por qué todavía no | Hallazgo |
|---|---|---|---|
| X13 | El folio puede repetirse entre dos turnos del **mismo día** | Consecuencia aceptada de numerar por turno (spec 008). Es inofensiva porque cerrar un turno exige que no queden pedidos vivos, así que dos folios iguales nunca coexisten vivos — pero no hay nada que lo impida si esa regla se afloja. Lo vigila `TestReabrirLaCajaElMismoDiaRenumeraSinColisionar` | T5 |
| X14 | El `Down` de 0061 falla si ya se vendió con dos turnos el mismo día | Volver a estrechar la unicidad al día es imposible con dos #1 de la misma fecha. Es inherente a revertir una restricción que se ensanchó; queda escrito en la propia migración en vez de descubrirse al revertir | — |
| X16 | `order_counters` quedó muerta tras 0061 | Se jubila en una migración propia cuando 008 lleve un ciclo en producción, no antes: mientras tanto es lo que permite volver atrás por imagen sin restaurar la base | — |
| X17 | El test viejo `TestRefreshReuseRevokesFamily` tenía UNA sola sesión | Con una sola, revocar por usuario y revocar por familia son indistinguibles: pasaba en verde con el comportamiento equivocado. Se conserva (cubre el rechazo) y la distinción la mide ahora `TestElReusoRevocaSoloLaFamiliaComprometida` | — |
| X18 | Cuatro pedidos de producción cancelados y cobrados sin devolución ($729, 29-ago) | Son datos anteriores a la feature de devoluciones; corregirlos reescribiría un arqueo firmado. Es una decisión del dueño, no un cambio de código. Lo nuevo ya impide que se repita | — |
