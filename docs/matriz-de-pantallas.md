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
| V11 | El cableado del envío en las superficies de cobro, a 1024×600 | Las tres cifras coinciden y las tres se apagan igual | — | **no cubierto**. La suposición que se anotó aquí como falsa era la correcta: medido el 7 sep 2026, a 1024×600 el POS está en modo **ancho** con el panel colapsado (`panelHidden` arranca en true por `max-height: 720px`) y **sí hay píldora flotante** — ver [presupuesto-de-pantalla-1024x600.md](presupuesto-de-pantalla-1024x600.md) |

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

## Y. El folio de la plataforma y su liquidación (spec 014)

Renglones **abiertos**: se escriben antes que el código, como pide el principio IV, y su columna
*Test* se llena cuando el test existe. Un renglón que llegue al merge sin test es un hueco, y se
declara como tal.

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| Y1 | Folio con espacios en los extremos (pegado del reporte) | Se guardan los extremos recortados y **nada más**: ni mayúsculas, ni guiones, ni longitud | `TestNormalizePlatformRef` · `TestElFolioSeGuardaTalCualConLosExtremosRecortados` | Postgres |
| Y2 | Folio vacío o solo espacios | Se rechaza. `""` no es "sin folio": chocaría contra la unicidad o pasaría | `TestNormalizePlatformRef` · `TestElEsquemaRechazaUnFolioQueNoEsUnFolio` | Postgres |
| Y3 | Folio repetido en la misma empresa y plataforma | Se rechaza **nombrando el pedido que ya lo tiene**, no con un aviso genérico | `TestUnFolioRepetidoDiceCualPedidoLoTiene` · `TestElFolioRepetidoTieneSuPropioCodigo` | Postgres |
| Y4 | El mismo folio en dos empresas | Se acepta. La unicidad es por empresa y plataforma, nunca global | `TestLaUnicidadDelFolioEsPorEmpresaYPorPlataforma` | Postgres (respaldo real) |
| Y5 | Folio en un pedido de mostrador | Imposible por construcción (check en el esquema), no por validación de pantalla | `TestUnPedidoDeMostradorNoPuedeTenerFolioDePlataforma` · `TestUnPedidoDeMostradorNoAceptaFolioDePlataforma` | Postgres |
| Y6 | El campo de folio con la lista en Mostrador | **No existe en el árbol**, no "existe oculto", y no ocupa alto | `PlatformPicker.test.tsx` › *no existe con la lista en Mostrador* | Vitest |
| Y7 | Mandar un pedido de plataforma con el campo vacío | Se pide el dato con **una** salida explícita; tomarla lo manda sin folio y lo deja como pendiente | `folioPlataforma.test.ts` · `FolioPlataformaSheet.test.tsx` | Vitest |
| Y8 | La salida explícita con el teclado abierto | Sigue visible y tappable. Si el teclado la tapa, SC-003 pasa de un toque a dos | `folio-de-plataforma.spec.ts` › Y7+Y8 | Playwright (ventana encogida a 350 px; el teclado real no se simula) |
| Y9 | Renglones del mosaico con plataforma activa a 1024×600 | Medido: 3 en mostrador, 2 con plataforma. El renglón lo cuestan las dos líneas de texto del selector, que ya existían; el campo del folio agrega 0 px | `folio-de-plataforma.spec.ts` › Y6+Y9 | Playwright |
| Y10 | Filtro de pendientes con un valor desconocido | 400 de validación; **nunca** cae en silencio a "todos" | `TestElFiltroDePendientesDeFolio` | Go |
| Y11 | Lista y resumen con el filtro de pendientes | Describen **el mismo conjunto**, sin excepción: el `total` de la lista y el `count` del resumen coinciden | `TestLaListaYElResumenDescribenElMismoConjunto` | Postgres |
| Y12 | Buscar pegando el folio del documento de pago | Devuelve **ese** pedido y solo ese, en un paso | `TestBuscarPegandoElFolioDelDocumentoDevuelveEsePedido` | Postgres |
| Y13 | Buscar el número interno (`187`) o el nombre (`Tigre`) | **No** los encuentra: la búsqueda es del folio de plataforma, que es otra cosa | `TestLaBusquedaNoEncuentraPorNumeroNiPorNombreInterno` | Postgres |
| Y14 | El folio en la celda "Tipo" de la lista | Truncado con elipsis; la página **no** se desborda a lo ancho, que es lo que pasaría con una columna nueva | `folio-de-plataforma.spec.ts` › Y14+F65 | Playwright |
| Y13b | Dos plataformas usan el MISMO identificador | La búsqueda devuelve los dos, y el renglón dice de qué plataforma es cada uno. La unicidad es por plataforma a propósito: rechazarlo tiraría una captura legítima | medido a mano en dev (pedidos 224/226) | Postgres |
| Y15 | El plan de la consulta de pendientes | Con el predicado LITERAL entra por `orders_plataforma_sin_folio`; con el patrón `narg … is null or (…)` no hay índice que pueda usar ni con `enable_seqscan=off` | medido a mano (ver §Y-plan) | Postgres |
| Y16 | Rótulos del folio de plataforma en la interfaz | Nombran la plataforma (*Folio de Uber Eats*); nunca dicen "folio" a secas, que en esta pantalla ya es el número del turno | `PlatformPicker.test.tsx` · `SalesPage.test.tsx` | Vitest |
| Y17 | Pedido sin liquidación contra liquidación en ceros | Se distinguen: ausencia es 404, ceros es 200 con ceros | `TestSinLiquidacionNoEsLoMismoQueUnaLiquidacionEnCeros` | Postgres |
| Y18 | Los nueve campos de la liquidación a 1024×600 | Caben en 600 px, todos ≥44 px, y guardar sigue visible con la ventana encogida a 350 px | `folio-de-plataforma.spec.ts` › Y18 | Playwright |
| Y19 | Los tiles del resumen de plataformas | Cada uno con su conteo de pedidos a la vista: cubren conjuntos distintos y no se restan | `TestLasTresCifrasDelPeriodoCubrenConjuntosDistintos` · `TestLoQueLlegoAlBancoNoEsLaRestaDeLasOtrasDos` | Go |

### Y-plan: cómo se midió el plan de la consulta

Contra la base restaurada de producción, con `plan_cache_mode = force_generic_plan` y
`enable_seqscan = off` (la tabla tiene 128 filas; sin desactivar el seq scan el planner lo prefiere
por tamaño, no por incapacidad):

| Forma de la consulta | Plan |
|---|---|
| `delivery_platform_id is not null and platform_order_ref is null` (literal) | `Bitmap Index Scan on orders_plataforma_sin_folio` |
| `($1::boolean is null) or (…mismo predicado…)` | `Seq Scan` — **no hay índice que pueda usar** |
| `platform_order_ref = $1` | `Index Scan using orders_platform_ref_busqueda` |

Es la razón de que `sales.sql` tenga cinco PARES de consulta en vez de cinco con un parámetro.

## Z. Deuda de specs viejos, cerrada el 8 de septiembre de 2026

Dos renglones que llevaban abiertos porque exigían el ambiente desplegado, y cuatro que salieron de
cerrarlos: uno de medición y tres de un defecto que se vio en el ambiente de pruebas.

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| Z1 | El mosaico con pedidos en curso (005 · SC-005) | La barra flota sobre el catálogo en vez de empujarlo: 3 renglones y 396 px de catálogo, con deuda puesta | `deuda-de-especificaciones.spec.ts` › *005/T047* | Playwright |
| Z2 | La vista previa del ticket contra el desplegado (001 · SC-006) | Abre sin una petición que no salga y sin nada bloqueado por CSP; imprimir se alcanza sin desplazarse | `deuda-de-especificaciones.spec.ts` › *001/T037* | Playwright (el papel sigue siendo manual) |
| Z3 | Medir píxeles de un diálogo | Se espera a que la animación de entrada asiente. `boundingBox()` devuelve la caja **transformada**: el mismo botón da 42 px a media animación y 44 asentado, y eso ya produjo un hallazgo falso | el `waitForTimeout(600)` de *001/T037* | Playwright |
| Z4 | Entrar con una cuenta guardada por la versión anterior | El POS renderiza. `platformOrderRef` nació con 014 y no se agregó al `merge` del almacén persistido; `FolioPlataformaSheet` vive siempre montada y arranca con `useState(cuenta.platformOrderRef).trim()`, así que una cuenta ya guardada en la tableta dejaba la pantalla **en blanco al entrar** — no al mandar el pedido | `cuentaGuardadaAntes.test.ts` y `folio-de-plataforma.spec.ts` › *Y20* | vitest + Playwright |
| Z5 | Reproducir el estado de una tableta que ya se usó | Sembrar el carrito viejo NO basta: sin la marca `sesion.ultimaEmpresa`, `hayQueLimpiar` trata el perfil limpio de Playwright como cambio de empresa y el login llama a `descartarTodo()`, que tira lo sembrado antes de que el POS renderice. **Y20 pasó en verde contra el build roto por esto.** Hay que sembrar las dos llaves | el `addInitScript` de *Y20* | Playwright |
| Z6 | Ninguna pantalla se queda en blanco | Se recorren las 14 rutas con una cuenta guardada por la versión ANTERIOR y se exige que cada una pinte algo y que ninguna tire una excepción. Es la guardia de la **clase**, no del campo: sin error boundary, cualquier throw en render deja el `#root` vacío. Verificado en rojo sirviendo el bundle roto por su hash viejo: `#root` en 0 caracteres y el `pageerror` que reportó el operador | `pantalla-en-blanco.spec.ts` › *Z6* | Playwright |

## C. La hoja del contador de efectivo (spec 003)

Medido el 9 de septiembre de 2026 contra un navegador real a 1024×600, con el stack completo en
local (API Go + Postgres, sin mocks: un backend simulado estaría de acuerdo con la pantalla por
construcción).

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| C1 | La hoja con las once denominaciones | Cabe en 600 px —**552 px medidos**— y la moneda de 50¢, que es el último renglón, se alcanza **sin desplazarse**. Inline en `/caja` era imposible: esa pantalla mide 1,494 px y el punto de inserción arranca 200 px debajo del fold | `contar-el-cajon.spec.ts` › **C1** | Playwright |
| C2 | Los controles de cada denominación | Campo y ajustes ≥44 px reales, medidos **después** de que la animación asiente (ver Z3). Y el campo es más ancho que una tecla de ajuste: es el control principal y el peso visual tiene que decirlo | `contar-el-cajon.spec.ts` › **C2** | Playwright |
| C3 | El teclado numérico abierto | El total y el botón de confirmar siguen a la vista. Se simula recortando la ventana a 350 px de alto, que es lo único que importa del teclado: se come ~250 px. Es lo que `dvh` + footer fijo existen para resolver | `contar-el-cajon.spec.ts` › **C3** | Playwright |
| C4 | Contar 40 monedas y abrir la caja | Se teclean, no se tapean 40 veces, y **el turno abre con la misma cifra que mostró la hoja**. El servidor recalcula desde las piezas: si las dos sumas no coinciden, el arqueo se compara contra un fondo que nadie contó | `contar-el-cajon.spec.ts` › **C4** | Playwright + servidor |
| C5 | El desglose después del cierre | Piezas por denominación y subtotal del servidor; un arqueo capturado a mano muestra su motivo; **un corte anterior a la feature no pinta nada**, sin avisos que hablen del sistema | `CashPage.test.tsx` › *el desglose de lo contado* | Vitest |

**Lo que no se midió en tableta real:** el teclado del sistema operativo. C3 recorta la ventana, que
es el efecto que importa, pero el teclado real de una Surface puede tapar de otra forma.

## J. El cierre con un solo cajón y los métodos configurables (spec 015)

Medido el 10 de septiembre de 2026 a 1024×600: el "antes" contra `app-dev` con la spec 003
desplegada, el "después" contra el candidato servido en local con la API nueva, **con los mismos
diez métodos configurados** (tres «en línea» en automático, cuatro que caen en el cajón). El
archivo que mide es el mismo para los dos, así que las cifras son comparables.

Lo que se mide es el **contenedor que se desplaza**, no el documento: el AppShell es
`h="100dvh" overflow="hidden"` y quien hace scroll es el `<Box flex="1" overflowY="auto">` que
envuelve al `<Outlet>`. La primera versión de la medición leía `document.documentElement.scrollHeight`
y reportaba 600 px —exactamente el viewport— en las dos pantallas: el número se lee como si todo
cupiera.

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| J1 | El alto de la tabla del cierre | **575 px → 540 px** con un renglón MÁS (10 → 11): el renglón del cajón cuesta 61 px y los cuatro métodos que ya no capturan bajan de 61 a 37 | `presupuesto-del-cierre.spec.ts` › **P1** | Playwright |
| J2 | Los toques que cuesta cerrar (SC-006) | **7 capturas → 4**: seis campos y el botón de contar pasan a tres campos y el botón. El arqueo único no se pagó con más pantallas | `presupuesto-del-cierre.spec.ts` › **P1** | Playwright |
| J3 | El tercer estado de la columna «Declarado» | Dice **«Va al cajón»** y no reusa «Automático»: uno lo resuelve el servidor, el otro se cuenta físicamente, y confundirlos es la ambigüedad que ya costó $4,500 | `CashPage.test.tsx` | Vitest |
| J4 | La columna «Esperado» con el arqueo ciego | Desaparece entera, no se queda con rayas: a 1024×600 el ancho que libera se lo devuelve a lo que el operador vino a leer | `arqueo-ciego.spec.ts` › **B1**, `CashPage.test.tsx` | Playwright + vitest |
| J5 | El botón de cerrar con el arqueo ciego | Bloqueado mientras no se cuente el cajón. Verificado en rojo devolviendo la regla vieja (deducirlo de que el esperado sea cero), que con el esperado en `null` lo habilitaba | `arqueo-ciego.spec.ts` › **B2** | Playwright |
| J6 | Los métodos en Ajustes, a lo ancho | La tabla de cuatro columnas **no se desborda**: el ancho útil de esa página son ~520 px (`<Page maxW="560px">` con su padding), no los 1024 de la tableta — el plan afirmó lo contrario | `presupuesto-del-cierre.spec.ts` › **P2** | Playwright |
| J7 | Los métodos en Ajustes, a lo alto | **0 de 10 renglones se ven sin desplazarse**, antes y después: la sección arranca en y≈1,210 (ahora 1,313, por el interruptor del arqueo ciego que se le puso encima). No empeoró, pero tampoco es una pantalla que se lea de un vistazo | `presupuesto-del-cierre.spec.ts` › **P2** | Playwright |
| J8 | El área tappable de los interruptores | ≥44 px. **Defecto encontrado al medir**: la tabla nueva puso tres interruptores de **24 px** en un renglón de 41, y un dedo que falla por milímetros cae en el de al lado — que aquí significa apagar «Activo» y sacar un método del cobro a media jornada. La lista vieja medía 20 px con uno solo por renglón; no se heredó | `presupuesto-del-cierre.spec.ts` › **P2** | Playwright |

| J9 | La separación entre los tres interruptores de un renglón | **46 px y 63 px medidos**, contra un piso de 22. Una revisión los estimó en 16 px calculando el padding de la celda; la medición dice otra cosa, porque el ancho lo dan los encabezados «Va al cajón» y «Automático», más anchos que el interruptor. Se afirma en el test porque acortar un encabezado cerraría el hueco sin que nadie lo note | `presupuesto-del-cierre.spec.ts` › **P2** | Playwright |
| J10 | El mensaje del cierre rechazado | Dice qué hacer primero y nombra el método, sin explicar el mecanismo: *«recarga la pantalla para cerrar: «Efectivo» ahora se cuenta con el cajón»* | `TestUnMetodoDeCajonEnDeclaradoSeRechaza` | Postgres |

**Lo que J no cubre:** la tableta real. J1–J8 miden un Chromium a 1024×600, que es el presupuesto,
no una Surface con su teclado y su densidad de píxeles.

## K. Que ninguna pantalla esté rota sin que nadie se entere (2026-09-11)

Un defecto vivió **horas en producción** sin que nada lo detectara: `/catalogo/opciones` respondía
500 y el catálogo de modificadores no se podía abrir. La causa fue de base —`string_agg` sobre un
conjunto vacío devuelve NULL, y el `::text` no lo cambia pero sí hace que sqlc tipe la columna como
`string` no nulable—, pero lo que importa aquí es **por qué la suite no lo vio**.

`pantalla-en-blanco.spec.ts` › Z6 ya recorría esa ruta. Solo que Z6 exige que la pantalla PINTE algo
y que no tire una excepción, y **un 500 de la API no deja la pantalla en blanco**: TanStack Query lo
atrapa y queda el encabezado sin datos. Z6 pasaba en verde con el catálogo caído — un caso que se ve
cubierto y no cubre, que es peor que no tenerlo.

| # | Caso | Qué debe pasar | Test | Medido |
|---|---|---|---|---|
| K1 | Cualquiera de las 14 rutas recibe un 5xx | Falla nombrando la ruta y el endpoint. Es la guardia de la CLASE: cubre el próximo 500, no el que ya pasó | `nada-responde-500.spec.ts` › **N1** | Playwright |
| K2 | Las tres pestañas del catálogo de modificadores | Las tres responden. Recorrer la ruta NO bastaba: «Activos» daba 200 —sus grupos sí tenían opciones activas— y solo «Inactivos» y «Todos» daban 500 | `nada-responde-500.spec.ts` › **N2** | Playwright |
| K3 | Un grupo sin opciones activas, en la tarjeta | Ni renglón de vista previa vacío ni «y N más» colgando de una lista que no existe | `ModifierOptionsPage.test.tsx` | Vitest |
| K4 | El resumen «y N más» | Resta las cuatro que ya mostró: con 6 opciones dice «y 2 más», con 4 no dice nada | idem | Vitest |

**Verificado en rojo**: N2 corrido contra el ambiente de pruebas *antes* del arreglo falla nombrando
los dos filtros exactos que rompían. Y las tres unitarias se vieron fallar mutando el componente.

**Lo que K no cubre:** que la pantalla muestre lo CORRECTO — eso sigue siendo de cada spec. K1 solo
exige que el servidor no se rompa, que es justo lo que faltaba.

## L. La consola de plataforma no se cruza con el negocio (2026-09-11)

Spec 016. Lo que se cubre aquí no es una pantalla sino una **separación**, y la vara es distinta:
cada renglón tiene que poder verse fallar quitando la barrera que lo sostiene, porque una barrera
que nadie vio caer no se sabe si está.

| # | Caso | Qué debe pasar | Test | Visto en rojo |
|---|---|---|---|---|
| L1 | Credencial de plataforma en el login del POS | 401, con la misma respuesta **y la misma latencia** que un usuario inexistente | `consola_separada_test.go` | Sí — medido: 259 µs contra 38 ms al revisar `is_active` antes de bcrypt |
| L2 | Credencial del negocio en la consola, **siendo admin** | 401. El permiso más alto del producto no alcanza | idem | — |
| L3 | Token de una superficie contra rutas de la otra | 401 en las dos direcciones, por la firma y no por una comprobación | `auth/plataforma_test.go`, `consola_separada_test.go` | Sí — con un solo secreto, el token cruzado valida |
| L4 | Operador desactivado con una sesión abierta | El **siguiente** request da 401, sin esperar a que caduque el token | `consola_separada_test.go` | Sí — sin releer al operador devolvía 200 |
| L5 | La consola contra `orders`, `order_payments`, `register_sessions`, `expenses`, `users` | `42501` en las cinco | `consola_sin_permisos_test.go` | Sí — con `grant select on orders` el caso pasa a verde indebidamente |
| L6 | La consola escribiendo en `companies` | `42501` en `insert`, `update` y `delete`, y la fila intacta | idem | Sí — con los grants de escritura, dos de los tres pasan |
| L7 | La lista de clientes | Con dos empresas salen las dos; la versión de esquema viaja una vez | `consola_empresas_test.go` | Sí — sin la política de RLS ve **una de dos** |
| L8 | Lo que la respuesta NO trae | Ni dinero, ni empleados, ni «última actividad», buscado en el JSON crudo | idem | — |
| L9 | Arrancar con la conexión equivocada | La API no sirve si el rol de la consola es superusuario o puede leer `orders` | `migracion_consola_test.go` | — |
| L10 | Revertir la migración | Corta el acceso sin borrar el rol, y volver a aplicarla deja la consola viva | idem | Sí — el `create role if not exists` dejaba el rol sin login en la segunda vuelta |
| L11 | El POS importando código de la consola | `bun run lint` en rojo, en las dos direcciones y también con `import()` dinámico | `eslint.config.js` (FRONTERAS) | Sí — con tres sondas: estática, dinámica POS→consola y dinámica consola→POS |
| L11b | La consola importando cualquier carpeta del POS | En rojo para las diez (`features`, `shared`, `stores`, `domain`, `api`, `components`, `hooks`, `app`, `utils`, `types`) | idem | Sí |
| L12 | El peso del paquete del POS | No crece: 1,133.50 kB antes y después | `bun run build` | Medido |
| L13 | La pantalla de la consola | Arranca pidiendo entrar; lista las empresas; con cero lo dice en vez de pintar una tabla vacía; un 401 posterior devuelve al login y un 500 se dice sin expulsar | `src/consola/Consola.test.tsx` | Sí — quitando el caso de lista vacía y el manejo del 401, dos casos caen |
| L14 | Una caída de red (wifi, DNS, CORS) | Dice "No se pudo conectar…", nunca el `Failed to fetch` que escribe el navegador | idem | — |
| L15 | El mensaje de error se anuncia | Lleva `role="alert"`: un lector de pantalla dice que algo pasó tras tocar Entrar | idem | — |

**Lo que encontró la revisión adversarial y no el trabajo** (queda escrito porque la lección es de
método, no de código): la primera versión de L11 se dio por buena midiendo **solo el import
estático**. `no-restricted-imports` no escucha `ImportExpression`, así que `import('../consola/api')`
cruzaba la frontera con `tsc`, `eslint` y `bun run build` los tres en verde, y con un chunk de la
consola dentro del `dist/` del POS. La barrera existía a medias y el renglón de esta matriz decía
que estaba entera.

**Lo que L no cubre, y hay que decirlo:**

- **Ningún test de navegador AUTOMÁTICO.** La consola no entra a la suite de Playwright: ésa corre a
  1024×600 contra el POS desplegado, y esta pantalla vive en una computadora. Lo que hay es jsdom
  (L13) y las barreras del backend.

  Sí se abrió a mano en Chromium contra los dos ambientes desplegados el 2026-09-12 —entrar, la
  tabla con las dos empresas, cero errores en la consola del navegador—, pero eso fue un ensayo, no
  un check runnable: **si la pantalla se rompe mañana, nada lo va a decir**.
- **Nadie prueba el despliegue a Pages.** Que `staff-dev` sirva la consola y no el POS se ve
  desplegando, igual que el resto.
- **La sesión de la consola no sobrevive a una recarga** (no hay refresh, a propósito). No es un
  defecto: es la decisión de no dejar una credencial de plataforma durmiendo en el navegador.

## Pendientes de cubrir

Renglones que este documento reconoce como **no cubiertos**. Están aquí porque un hueco nombrado se
arregla y uno olvidado no. Cada uno cita el hallazgo del
[barrido](auditoria/barrido-de-pantallas-2026-09.md) que lo describe.

| # | Caso | Por qué todavía no | Hallazgo |
|---|---|---|---|
| X20 | `GET /expenses?page=abc` cae a la página 0 en silencio | `handlers_backoffice.go:496` hace `page, _ := strconv.Atoi(...)` e ignora el error, así que un `page` malformado no se rechaza: devuelve la primera página como si nada. Es el principio V —"un parámetro de frontera inválido se RECHAZA; nunca cae a un default en silencio"— y lo encontró el barrido del 500 de modificadores (2026-09-11), en otra familia. **No se arregló con ese despliegue a propósito**: rechazar un parámetro que hoy se acepta es un cambio de comportamiento, y no se mete de polizón en un deploy a producción que ya lleva dos migraciones. Va solo, con su test | — |
| X13 | El folio puede repetirse entre dos turnos del **mismo día** | Consecuencia aceptada de numerar por turno (spec 008). Es inofensiva porque cerrar un turno exige que no queden pedidos vivos, así que dos folios iguales nunca coexisten vivos — pero no hay nada que lo impida si esa regla se afloja. Lo vigila `TestReabrirLaCajaElMismoDiaRenumeraSinColisionar` | T5 |
| X14 | El `Down` de 0061 falla si ya se vendió con dos turnos el mismo día | Volver a estrechar la unicidad al día es imposible con dos #1 de la misma fecha. Es inherente a revertir una restricción que se ensanchó; queda escrito en la propia migración en vez de descubrirse al revertir | — |
| X16 | `order_counters` quedó muerta tras 0061 | Se jubila en una migración propia cuando 008 lleve un ciclo en producción, no antes: mientras tanto es lo que permite volver atrás por imagen sin restaurar la base | — |
| X17 | El test viejo `TestRefreshReuseRevokesFamily` tenía UNA sola sesión | Con una sola, revocar por usuario y revocar por familia son indistinguibles: pasaba en verde con el comportamiento equivocado. Se conserva (cubre el rechazo) y la distinción la mide ahora `TestElReusoRevocaSoloLaFamiliaComprometida` | — |
| X18 | Cuatro pedidos de producción cancelados y cobrados sin devolución ($729, 29-ago) | Son datos anteriores a la feature de devoluciones; corregirlos reescribiría un arqueo firmado. Es una decisión del dueño, no un cambio de código. Lo nuevo ya impide que se repita | — |
| ~~X19~~ | ~~Una tableta con la pantalla en blanco no puede aplicar la versión que la arregla~~ **CUBIERTA (2026-09-08)** | `PantallaQueNoSeCae` envuelve a `<App/>` dentro del Provider: un throw en render deja de vaciar el `#root` y pinta una salida con un botón de 56 px que fuerza una navegación nueva (`?r=<ts>`) en vez de un `reload`, que podría volver a servirse del service worker viejo. El detalle del error va a la consola y no a la tableta. Lo que sigue fuera: no reporta a ningún servicio — el techo está escrito en el `// ponytail:` del componente | `PantallaQueNoSeCae.test.tsx` (4 casos) |
