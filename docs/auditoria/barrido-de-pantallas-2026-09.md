# Barrido de pantallas — vender, pedidos, ventas (3 de septiembre de 2026)

Qué se buscó, qué apareció y por qué se resolvió como se resolvió. La matriz de casos vive en
[matriz-de-pantallas.md](../matriz-de-pantallas.md); este documento es el **razonamiento**: causa,
regla violada, opciones consideradas y por qué las descartadas se descartaron.

**Método.** Cuatro barridos en paralelo sobre el código (vender, pedidos, ventas/reportes y las
fronteras del backend), y después Playwright a 1024×600 contra el ambiente de pruebas desplegado —
no contra un backend simulado, porque con el mock la pantalla y el servidor coinciden por
construcción y es justo esa coincidencia la que hay que poner en duda.

**Vara.** Los principios de [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md).
Cada hallazgo cita el que rompe; uno que no rompa ninguno se dice y se deja.

---

## Hallazgos

### H1 · "Por medio de pago" contestaba otro periodo que "Venta por día" — ALTA, corregido

**Contexto.** La pantalla de Reportes pinta cuatro tablas una junto a otra, y la de medios de pago
es la que se usa para *cuadrar* la de ventas: si el total del día no se explica con lo que entró por
caja, algo se registró mal.

**Qué pasaba.** `SalesByDay` filtraba `o.business_date between $1 and $2`. `SalesByMethod` filtraba
`op.created_at >= $1`: sin cota superior y sobre el instante del cobro en vez del día de negocio.
Con la pantalla como estaba —sin filtro, siempre "últimos 30 días"— la diferencia era invisible.
Al poner el filtro de fechas se habría vuelto un error de lectura permanente: elegir julio mostraba
julio arriba y *de julio a hoy* abajo.

Dos agravantes en la misma consulta:
- No excluía canceladas ni reembolsadas, que su hermana sí excluye. El cobro de una venta devuelta
  sumaba abajo mientras el total de arriba no lo contaba.
- `created_at` es un instante en UTC. Un cobro de un turno que cruza la medianoche caía en un día
  distinto que su propia venta.

**Regla violada.** Principio III, corolario: *la lista y el resumen de una misma pantalla se derivan
del mismo predicado — si divergen, uno de los dos miente y quien lo lee no tiene forma de saber
cuál.* Es la misma forma del fondo de caja que se contó una vez por método y dejó un turno con
$4,500 de faltante.

**Opciones.**

| | Opción | Pro | Contra |
|---|---|---|---|
| A | Dejarlo y documentar que la tabla de métodos es "acumulada" | Cero riesgo de cambiar cifras históricas | Es documentar una mentira. Nadie lee una nota al pie mientras cuadra una caja |
| B | Ponerle cota superior sobre `created_at` | Cambio mínimo | Sigue mezclando instante y día de negocio: el turno que cruza medianoche sigue partido |
| C | Mover el predicado a `o.business_date between $1 and $2` y excluir canceladas/reembolsadas | Las dos tablas quedan derivadas del mismo predicado, literalmente | Las cifras históricas de esa tabla cambian |
| D | Borrar la tabla de métodos de esta pantalla | Elimina la contradicción de raíz | Quita la herramienta con la que se cuadra; el corte de caja no cubre rangos largos |

**Refutación.** A queda descartada por el principio, no por conveniencia: una pantalla que necesita
una nota para leerse bien ya falló. B se ve suficiente hasta que se recuerda por qué existe
`business_date`: es la unidad con la que este negocio cuadra su caja, y un turno que abre a las
16:00 y cierra a las 02:00 es un día para el dueño y dos para `created_at`. D confunde el síntoma
con la causa — el problema no es que la tabla exista, es que responde otra pregunta.

**Elegida: C.** Sobre el contra: lo que cambia es **qué muestra** una pantalla de análisis, no en
qué día cae una venta. `orders.business_date` no se toca y ningún arqueo cerrado se mueve — que es
la restricción real. Y la cifra que cambia estaba mal: incluía ventas de fuera del periodo y ventas
reembolsadas.

**Cubierto por.** `TestElReporteDeVentasNoMezclaDosPeriodos`,
`TestUnaVentaReembolsadaNoSumaEnLosMetodosDePago` (integración, contra Postgres real; vistos en rojo
con el predicado viejo, fallando con el nombre del concepto duplicado).

---

### H2 · "Utilidad por producto" no tenía cota superior ni miraba el día de negocio — ALTA, corregido

**Qué pasaba.** `ProductMargins` filtraba `o.opened_at >= $1`. Sin el filtro de fechas era "los
últimos 30 días y lo que venga"; con el filtro habría seguido contestando *de esa fecha a hoy*
mientras el resto de la pantalla contestaba el rango elegido. Y `opened_at` es un instante en UTC:
un pedido abierto a las 19:00 de México ya es del día siguiente para esa comparación.

**Regla violada.** Principio III, mismo corolario que H1.

**Opciones y refutación.** Las mismas que H1 y con el mismo desenlace. Se añadió una consideración
propia: cambiar `opened_at` por `business_date` mueve al periodo correcto los pedidos abiertos
después de las 18:00 locales, que en un restaurante de cena **son la mayoría**. No es un ajuste de
borde: es el grueso del turno.

**Cubierto por.** `TestLaUtilidadPorProductoRespetaElRango`.

---

### H3 · Las fechas que el preset no iba a usar se descartaban en silencio — MEDIA, corregido

**Qué pasaba.** `?preset=hoy&from=2026-01-01&to=2026-01-31` contestaba **hoy**. Los parámetros
llegaban, se parseaban y se tiraban.

**Regla violada.** Principio V: *un parámetro de frontera inválido se rechaza; nunca cae a un default
en silencio. El default es para el parámetro ausente, nunca para el presente y malformado.* Un
parámetro presente que no se puede atender es el mismo caso.

**Opciones.**

| | Opción | Pro | Contra |
|---|---|---|---|
| A | Dejarlo: el front nunca manda esa combinación | Cero trabajo | "El front nunca" dura hasta el siguiente cambio del front. Y es exactamente cómo nació el defecto de `parseDate` |
| B | Cambiar a `rango` automáticamente si vienen fechas | Nada se rechaza | Adivina la intención. `?preset=mes&from=…` ¿es el mes o el rango? Elegir por el usuario es lo que se está tratando de evitar |
| C | Rechazar con 400 | Nada se descarta callado | Un cliente que hoy manda basura empieza a ver errores |

**Refutación.** B es la peor de las tres y se ve como la más amable: convierte una petición ambigua
en una respuesta concreta sin decirlo, que es la definición del defecto. El contra de C es real pero
el único cliente es el propio front, y se ajustó en el mismo cambio.

**Elegida: C**, en `domain.ResolveRange` y no en cada handler: escrita dos veces divergiría, y las
dos pantallas tienen que rechazar lo mismo.

**Cubierto por.** `TestUnasFechasQueElPresetNoVaAUsarSeRechazan` (los seis presets, y con una sola
de las dos fechas también: media fecha ignorada engaña igual que dos).

---

### H4 · "Últimos 30 días" eran treinta y uno — BAJA, corregido

**Qué pasaba.** El handler armaba la ventana con `to.AddDate(0, 0, -30)`, que con `between`
inclusivo son 31 días. El encabezado decía 30.

**Regla violada.** Ninguna de las no negociables. Va aquí porque es el tipo de defecto que **nadie
ve** —un día sobre treinta— y que muerde exactamente cuando alguien compara dos periodos "de 30
días" y encuentra una diferencia que no se explica.

**Elegida.** `preset=30d` en el dominio, treinta días contando hoy, con la cuenta asertada en el
test. Se descartó redactar el encabezado como "últimos 31 días": el número redondo es el que el
dueño pide, y el código tiene que decir lo que la pantalla dice.

**Cubierto por.** `TestElPresetDeTreintaDiasSonTreintaDiasContandoHoy` y, extremo a extremo,
`pantallas.spec.ts › Q6`.

---

### H5 · El encabezado de Reportes decía un periodo que no era el consultado — MEDIA, corregido

**Qué pasaba.** `Reportes (últimos 30 días)`, escrito a mano en el JSX. Con cualquier otro rango
seguiría diciéndolo.

**Regla violada.** Principio III en su forma general: *toda cifra agregada declara qué incluye.* Una
cifra sin su periodo no se puede auditar; con el periodo equivocado al lado, se audita mal.

**Opciones.** (A) que la pantalla imprima el rango que *ella* pidió; (B) que el servidor devuelva el
rango que *consultó* y la pantalla lo imprima.

**Refutación de A.** Es la opción obvia y es la que reintroduce el defecto: la pantalla imprimiría lo
que *cree* haber pedido, y el desacuerdo entre lo pedido y lo consultado es precisamente lo que hay
que poder ver. Si el servidor recorta, tope o normaliza un rango, A lo esconde.

**Elegida: B.** Es lo que ya hacía la pantalla de Ventas; ahora lo hacen las dos.

**Cubierto por.** `ReportsPage.test.tsx › muestra el periodo que devolvió el servidor` y
`pantallas.spec.ts › F10`.

---

### H6 · El mismo defecto seguía vivo en la pantalla de Ventas — ALTA, corregido

**Contexto.** H1 corrigió `SalesByMethod` en `reports.sql`. La pantalla de Ventas tiene su propia
copia, `SalesTotalsByMethod` en `sales.sql`, y se quedó como estaba.

**Qué pasaba.** Una venta de $500 cobrada con tarjeta y luego reembolsada salía en TRES tiles
vecinos: `Total = $0` (la excluye), `Reembolsadas = $500` y `Tarjeta = $500`. El mismo dinero
clasificado de tres maneras a la vista, en la misma fila de tarjetas.

**Regla violada.** IV-d, *el hermano que no se movió*: la constitución nombra este error exacto —al
cambiar dónde vive una validación hay que buscar a todos los que llamaban a lo viejo—. Y III.

**Por qué no lo atrapó el test que existía.** `TestElResumenDeVentasClasificaCadaPesoUnaVez` cancela
una orden **sin pagos** (omite `Payments`), así que su aserción sobre `ByMethod` pasaba sin tocar el
caso. Un test que nunca ejercita el borde es documentación optimista.

**Opciones.** (A) incluir canceladas y reembolsadas —conciliar contra la terminal bancaria necesita
el flujo bruto—; (B) excluirlas, como el total de al lado.

**Refutación de A.** El argumento del flujo bruto es real pero es de OTRA pantalla: conciliar contra
la terminal es el corte de caja, que es por turno y sí mira todo lo que pasó por el cajón. La
pantalla de Ventas responde *qué vendió el negocio*, y ahí una venta devuelta no es venta. Con A, un
día de una sola venta devuelta se lee "no vendimos nada pero entraron $500 por tarjeta".

**Elegida: B**, y las dos consultas quedan anotadas como hermanas, con la instrucción de editarse
juntas.

**Cubierto por.** `TestElDesgloseDeMetodosNoCuentaLoReembolsado` (visto en rojo: *los métodos suman
600 y el total de la pantalla es 100*).

---

### H7 · Los números de la frontera caían al default, y uno daba 500 — ALTA, corregido

**Qué pasaba.** Las FECHAS ya cumplían el principio V —`parseDate` rechaza lo presente y
malformado—; los ENTEROS no.

- `?limit=abc` y `?limit=0` contestaban las 50 filas del default.
- `?limit=3000000000` truncaba a un `int32` **negativo**, y un `LIMIT` negativo es un error de
  Postgres: un **500** por una petición que nunca fue válida, cuando la constitución pide 400.
- `?page=214748365` con páginas de 20: el producto es 4,294,967,300, que envuelve a 4 en `int32`.
  El servidor contestaba la quinta página con un 200 limpio. El `nolint` de esa línea decía
  "ambos ya acotados" y en ese punto no lo estaban.
- Cerrar caja con una llave de `declared` que no es un id la descartaba en silencio: el corte
  comparaba lo esperado contra cero y le inventaba al cajero un faltante por todo lo que sí contó.

**Regla violada.** V, que nombra los tamaños de página explícitamente, y *"rechaza entradas absurdas
como 400, no 500"*.

**Opciones.** (A) acotar con mínimo y máximo en silencio (lo que ya hacía el histórico de cortes:
`?limit=99999` devolvía 50 filas); (B) rechazar.

**Refutación de A.** Acotar en silencio es la misma familia que el default silencioso: quien pidió
200 recibe 50 y la respuesta no dice que se le recortó. Es peor en una lista, porque el recorte se
ve idéntico a "no hay más".

**Elegida: B**, con un helper único (`enteroDeQuery`) del que salen `limiteDeQuery` y
`paginaDeQuery`, y con el tope de página **derivado** del tamaño para que el offset quepa siempre.

**Cubierto por.** `TestUnEnteroDeLaFronteraNoCaeAlDefault`, `TestUnaPaginaQueDesbordaNoContestaOtra`,
`TestElOffsetNuncaDesbordaInt32`.

---

### H8 · El filtro nuevo dejaba Reportes en ceros — ALTA, corregido

**Es una regresión que introdujo este mismo trabajo**, y por eso va aquí y no en una nota al pie.

**Qué pasaba.** Al tocar "Rango", las tres consultas quedan deshabilitadas hasta que estén las dos
fechas. Sin `placeholderData`, los datos se iban a `undefined`, los tres tiles caían a cero y el
renglón del periodo desaparecía. Y no salía spinner: con la consulta deshabilitada `isLoading` es
falso. La pantalla mostraba `Ventas $0.00 · Pedidos 0 · Propinas $0.00` con las tablas vacías.

**Regla violada.** V — tres ceros con aspecto de cifra se leen como un día sin ventas, y nadie audita
una pantalla que se ve bien.

**Agravante propio.** El comentario del test que escribí afirmaba *"la pantalla conserva el periodo
anterior"* mientras el test solo comprobaba que no se llamara a la API. Un comentario que describe un
comportamiento sin test es un comentario que va a mentir (principio VII); aquí ya mentía al nacer.

**Elegida.** `placeholderData` en las tres consultas, como ya hacía Ventas, y el test reescrito para
mirar el número en pantalla y el renglón del periodo, no la ausencia de llamada.

---

### H9 · Un rango en el futuro se aceptaba en las dos capas — MEDIA, corregido

**Qué pasaba.** El componente topa los dos campos con el día de hoy, y el comentario daba el caso por
resuelto. Pero ese tope no impide **teclear** la fecha: el navegador marca el campo como inválido y
ya, y aquí no hay validación de formulario. Ni `validarRango` ni `rangoLibre` comparaban contra hoy.

**Regla violada.** V, y el principio IV en su forma más incómoda: *el borde estaba escrito en un
comentario y no en un test*.

**Opciones.** (A) dejarlo: el encabezado imprime el periodo futuro, así que es auditable;
(B) recortar el rango hasta hoy; (C) rechazarlo.

**Refutación.** B es un default silencioso con otro nombre: el operador pide un periodo y recibe
otro. A es defendible —la pantalla no miente— pero deja en pie el modo de falla que este repo ya
trata como grave: una pantalla vacía que se lee como *"no vendimos nada"*.

**Elegida: C**, en las dos capas y contra el día del **negocio**, no el del navegador ni el del
servidor: a las 19:00 de México ya es mañana en UTC, y con el reloj del servidor un rango que termina
mañana pasaría por bueno justo en la hora de más venta.

---

### H10 · Las propinas se agrupaban por nombre — MEDIA, corregido

**Qué pasaba.** El reporte agrupaba por el nombre del empleado. Dos empleados llamados "Ana" salían
en un solo renglón con la suma de los dos. Es el único reporte que existe **para entregarle dinero a
una persona**, y un renglón así no se puede repartir.

**Regla violada.** III: dinero atribuido a quien no lo cobró.

**Elegida.** Agrupar por el id del empleado. Se descartó desambiguar con el nombre de usuario en la
etiqueta: el reporte lo lee quien reparte, y "ana_turno_b" no le dice nada; dos renglones "Ana" con
sus montos sí, porque quien conoce el turno los distingue.

---

### H11 · El piso de 44 px no aplicaba al `Picker` chico — MEDIA, corregido

**Qué pasaba.** La receta del tema sube el alto mínimo solo en el tamaño `md`. Los Pickers de las
barras de filtros usan el tamaño `sm` y quedaban en 32 px; el paginador de Ventas traía un
`minH="40px"` explícito que **bajaba** el control por debajo del piso en vez de subirlo.

**Regla violada.** Restricciones del producto: *todo control tappable mide al menos 44 px*, sin
excepción por tamaño.

**Elegida.** El piso va **dentro del `Picker`**, no en cada pantalla que lo usa: es el mismo dedo en
todas, y un piso repartido por las pantallas se pierde en la siguiente que se escriba.

---

### H12 · El envío del POS vivía fuera de la cuenta — ALTA, corregido

**Una causa, cinco síntomas.** El costo de envío era un `useState` de la pantalla de venta, no un
campo del pedido. De ahí salían:

1. **No sobrevivía a un F5** mientras el carrito sí (el carrito está en `persist`). El operador
   volvía a un ticket completo con el campo de envío vacío, y el pedido se cobraba con el default
   del negocio: $80 capturados se cobraban como $20, sin aviso.
2. **Se heredaba entre pestañas** y sobrevivía al cierre de la cuenta que lo capturó.
3. **La píldora y la barra angosta no lo checaban.** El panel apagaba sus botones con un envío mal
   escrito; esos dos caminos —los que se usan cuando el panel está oculto, y en 1024×600 el panel
   arranca oculto— mandaban el pedido con el default.
4. **Pintaban otro total.** El panel sumaba el envío y la píldora no, del mismo pedido.
5. **Trababa el POS entero.** Un valor mal escrito capturado en domicilio seguía apagando los
   botones tras cambiar a mostrador, donde el campo ni se pinta: botones muertos, sin razón visible
   ni campo que corregir. Y como el envío era global, ninguna cuenta podía vender.

**Reglas violadas.** IV-b (*el estado que no sobrevive a un F5* — la constitución lo nombra tal
cual), III y su corolario (dos superficies de la misma pantalla con predicados distintos), y V (el
default es para el campo ausente, no para el ilegible).

**Opciones.** (A) parchar cada síntoma donde aparece: guard en la píldora, sumar el envío en la
barra, limpiar el estado al cerrar la cuenta; (B) mover el envío a la cuenta y extraer la decisión
a una función pura que consuman las tres superficies.

**Refutación de A.** Es más rápida y deja el defecto vivo: la sexta superficie que se escriba nace
sin el guard, exactamente como nacieron la píldora y la barra. Y no arregla el (1), que es el que
cuesta dinero en silencio.

**Elegida: B.** El envío es parte de `orders.total`; su lugar es la cuenta. Se guarda el TEXTO y no
el número porque un valor mal escrito tiene que poder bloquear el cobro, y un número ya parseado no
distingue "vacío" de "ilegible".

**Cubierto por.** `envio.test.ts` (12 casos), `ticket.test.ts › el envío pertenece a la cuenta`,
`Ticket.test.tsx › deja de bloquear`, y lo que jsdom no puede ver —la píldora a 1024×600— en
`pantallas.spec.ts › V1, V4`.

**De paso:** vaciar una cuenta borraba el tipo de servicio y dejaba puesta la plataforma, así que lo
capturado después salía con precio de Uber en una cuenta que decía mostrador.

---

### H13 · El badge de "por cobrar" usaba el predicado que el servidor descartó — MEDIA, corregido

**Qué pasaba.** La pantalla filtraba por `!paid`. El servidor filtra por `outstanding > 0`, y su
comentario dice por qué: *"`paid` exige un total positivo: un pedido de $0 no está saldado pero
tampoco hay nada que cobrarle"*. Un pedido de $0 aparecía en el badge como "1 por cobrar · $0" y
ninguna tarjeta ofrecía Cobrar, porque ese botón sí se pinta con `outstanding > 0`. Un renglón que
no se puede atender y que no se va solo.

**Regla violada.** III, corolario. La razón por la que el servidor lo cambió estaba escrita en el
código, y la pantalla se quedó con la versión vieja.

---

### H14 · Un espacio era un motivo de cancelación válido — MEDIA, corregido

**Qué pasaba.** La pantalla de reembolso recortaba el texto; la de cancelación solo miraba que la
cadena no fuera vacía. Un espacio pasaba los dos lados y llegaba a la base, donde el `check` de la
migración 0007 lo da por bueno. El histórico se quedaba con una cancelación sin motivo, que es
exactamente lo que ese campo existe para impedir. Y ningún camino acotaba el largo: el único tope
era el megabyte del cuerpo entero.

**Regla violada.** IV-d (*el hermano que no se movió*: el recorte se puso en un camino y no en el
otro) y V.

**Elegida.** `domain.MotivoValido` recorta, rechaza lo vacío y acota a 200 **caracteres**, no bytes:
con acentos, un tope en bytes rechaza un motivo que en pantalla cabe de sobra.

---

### H15 · Un doble tap en "Entregar todo" daba error sobre una entrega que sí ocurrió — MEDIA, corregido

**Qué pasaba.** `SetStatus` ya tenía el no-op idempotente con su comentario —*"un doble-tap en el
tablero no debe dar error"*—. `DeliverAll` nació después y usa `CanTransition` a pelo, y
`entregada → entregada` es `false`: el segundo tap pintaba un toast rojo sobre un pedido
correctamente entregado. El botón además no se apaga mientras la petición viaja.

**Regla violada.** IV-c y la vara de UX del POS: *nunca obligar al operador a deshacer para rehacer*.

---

### H16 · El tablero mostraba el objeto de error crudo — MEDIA, corregido

**Qué pasaba.** `description: String(e)`. Una caída de red pintaba `TypeError: Failed to fetch`; un
409 pintaba `Error: ` pegado delante del mensaje. Es el mismo defecto que la hoja de cobro documenta
como corregido, vivo todavía en entregar, cancelar y reembolsar.

**Regla violada.** Restricciones del producto: *prohibido nombrar internals en la UI*, y *si el
renglón solo tiene sentido para alguien que leyó el código, no va*.

**Elegida.** Un helper con el mensaje del servidor cuando existe, y una instrucción accionable
cuando no: un fallo de red no tiene mensaje que le sirva a quien opera.

---

### H17 · El renglón que cancela un pedido medía 32 px — ALTA, corregido

**Qué pasaba.** Los tres renglones del menú ⋮ del tablero eran los únicos controles tappables de esa
pantalla sin el piso de 44 px: la receta `md` de Chakra da 12 px de padding sobre 20 px de texto. El
tercero cancela el pedido y repone inventario, y estaba pegado al segundo, que solo reimprime un
papel.

**Regla violada.** Restricciones del producto, dos veces: *todo control tappable mide al menos 44
px* y *la separación de las acciones destructivas es requisito funcional, no preferencia estética*.

**Elegida.** 44 px en los tres, separador y aire antes del destructivo. Se descartó pedir
confirmación: la cancelación ya pide motivo, y una confirmación encima de un diálogo de motivo son
dos toques para lo que se hace todos los días. Lo que faltaba no era una barrera, era distancia.

---

---

### H18 · El re-login masivo se tumbaba a sí mismo cada quince minutos — BLOQUEANTE, corregido

**Lo encontró la auditoría del salto a producción, con el deploy ya empujado y a media corrida.** Se
canceló el job antes de que la migración tocara la base. Va aquí, y no en una nota al pie, porque es
el defecto más caro de todo el barrido y ninguna prueba lo veía.

**Qué pasaba.** Hacen falta TRES piezas a la vez:

1. La migración `0052` marcaba **revocadas** todas las sesiones vivas, para forzar un re-login que
   la feature del turno necesitaba.
2. `domain.ClassifyRefresh` mira `revoked` **antes** que `expiresAt`: una credencial revocada que
   reaparece es, por definición, un robo.
3. Y el castigo del robo es `RevokeUserRefreshTokens`, que revoca **por `user_id`** — todas las
   sesiones de esa persona, en todas las tabletas, no solo la cadena comprometida.

En este negocio dos estaciones comparten cuenta (lo dice el comentario de `0050`). Con las tres
piezas juntas: la tableta A entra con contraseña, la B despierta con su cookie vieja y le revoca la
sesión a A, A refresca y le revoca la sesión a B. **Un ping-pong de quince minutos que no converge
mientras las dos se usen** — contraseña a media operación, para siempre. Y el `Down` de `0052`
declara por escrito que no hay vuelta atrás, así que el rollback no lo deshacía.

**Regla violada.** IV, en su forma más incómoda: *el camino nuevo que se salta el control viejo* y
*los casos de borde se piensan ANTES*. Cada pieza es correcta por separado y tiene su test; lo que
nadie probó es las tres juntas sobre una cuenta con dos estaciones.

**Por qué mi pre-vuelo no lo vio.** Medí lo que se puede medir en una máquina: las migraciones sobre
los datos reales, el arranque de la API nueva y de la vieja, los grants, las variables de entorno,
el rollback por imagen. Todo salió verde — y todo eso sigue siendo cierto. Lo que no se ve así es un
comportamiento que necesita **dos clientes y el paso del tiempo** para manifestarse.

**Opciones.**

| | Opción | Pro | Contra |
|---|---|---|---|
| A | Desplegar y avisar que habrá que reentrar | Cero código | No es "reentrar una vez": es cada quince minutos, sin fin |
| B | Quitar el re-login masivo de `0052` | Simple | Deja el mes de sesión viejo vivo, que es lo que `0052` vino a cerrar |
| C | Que `0052` **caduque** en vez de revocar | Mismo efecto buscado, sin respuesta de robo | Hay que editar una migración ya aplicada en dev |
| D | Que el reuso revoque por **familia** y no por usuario | Arregla la causa de raíz | No hay columna de familia: es cambio de esquema, con producción a medio desplegar |

**Refutación.** A es la que se elige sola por inercia y es la peor: convierte un defecto en una
instrucción de operación imposible de cumplir. B tira la feature. **D es la correcta a largo plazo**
—la constitución dice "revoca toda la **familia**" y el código revoca por usuario, así que ahí hay
un defecto de verdad— pero exige migración de esquema, y hacerlo con el front nuevo ya publicado y
el backend viejo abajo es cambiar dos cosas a la vez en producción.

**Elegida: C**, con D anotado. Caducar dice lo que de verdad pasó —el turno terminó— y sale por
`RefreshExpired`: un 401 limpio, sin respuesta de robo y sin tocar la otra estación. Editar `0052`
es legítimo aquí porque **producción nunca la corrió** (estaba en la 42) y su efecto es un cambio de
datos de una vez, no de esquema: dev ya vivió el logout y el esquema es idéntico en los dos lados.

**Y la cookie se borra en cada rebote de `/auth/refresh`**, no solo al salir. Sin eso, una credencial
que el servidor ya no acepta sobrevive en la tableta hasta su `Max-Age` —treinta días en las
emitidas antes de `0050`— y se vuelve a presentar en cada recarga.

**Cubierto por.** `TestDosEstacionesConLaMismaCuentaNoSeRevocanEntreEllas`, que corre la sentencia
**del archivo** de la migración —no una copia— así que editar `0052` mueve el test. Visto en rojo
con la versión que revocaba: *"el rebote de una sesión CADUCADA revocó 2 credenciales"*. Y
`TestUnReusoDeVerdadSigueRevocando`, porque aflojar la detección de robo abriría la puerta que el
principio V cierra.

**Queda abierto.** `RevokeUserRefreshTokens` sigue revocando por usuario donde la constitución dice
familia. Necesita columna de linaje en `refresh_tokens` y su migración.

---

## Lo que queda abierto, y por qué

Los renglones que quedan abiertos no se cerraron en esta tanda. No es una lista de deuda vaga: cada uno está
en la tabla *Pendientes de cubrir* de la matriz, con el hallazgo que lo describe.

Los que importan, y por qué no se arreglaron de una:

- **Cancelar un pedido ya cobrado (P1)** y **reembolsar uno entregado sin cobrar (P2)** no son
  defectos de una línea: el sistema no tiene ninguna operación que devuelva dinero. `Refund` marca la
  orden y anota como pérdida el total del pedido sin mirar lo efectivamente pagado, y `Cancel` ni
  siquiera consulta los pagos. Arreglarlos exige decidir qué pasa con el dinero —rechazar la
  cancelación, registrar una devolución, o las dos cosas— y esa decisión cambia lo que el arqueo
  espera del cajón. Va por spec, no por parche.
- **Agregar renglones sin llave de idempotencia (V5)** es el mismo control que ya tienen crear y
  cobrar, y el camino nació sin él (IV-c). Necesita columna, migración y su test.
