# Feature Specification: Caducidad por lote, primero en caducar primero en salir, y aviso de próximos a caducar

**Feature Branch**: `098-caducidad-por-lote`

**Created**: 2026-11-23

**Status**: Draft

**Input**: Issue #146 (cuerpo, caso real de nov-2026 y decisiones del dueño del 2026-11-23).
Depende de la spec 094 (varios almacenes, ya en producción) y se alinea con la subida 2 de la spec 096
(rediseño de inventario y conteo, todavía sin implementar).

## Contexto

El inventario no guarda fechas de caducidad. El personal las anota a mano en mensajes al contar y aun
así se tiran productos que caducaron sin que nadie lo viera venir (salsas, perlas, siropes,
congelados, lácteos). El caso que motivó el issue: un insumo de uso bajo comprado en presentación
grande caducó a medio envase, se siguió usando caducado varios meses y al final se tiró casi la
mitad. El conteo de ese mes lo registró «ya caducado», pero el dato se quedó en un mensaje.

Con dos lotes del mismo artículo y fechas distintas, nadie sabe cuál usar primero, y el sistema
descuenta como si todo fuera la misma existencia.

### Lo que decidió el dueño (2026-11-23)

- **Cada lote guarda su almacén**, y un traspaso mueve lotes con su fecha.
- **Al vender o mermar se descuenta primero el lote bueno que caduca antes. Nunca se descuenta de un
  lote caducado.** Si lo bueno se acaba, la venta sigue y la existencia buena queda en negativo, sin
  bloquear nada (puede faltar un conteo o una entrada sin registrar), y **avisa por la campana de
  notificaciones**. El lote caducado espera a que alguien lo mande a merma desde la vista de
  caducidades.
- Por lote, el **consumo promedio diario**, los **días de inventario** contra los días que faltan para
  caducar, y la marca **«no se termina antes de caducar: quedan ~X»**.
- La fecha es opcional: un artículo sin fecha se comporta como hoy. El orden de consumo es automático:
  nadie elige lote al vender.
- Las existencias de hoy **no se reparten en lotes inventados**: quedan como existencia sin fecha y el
  primer lote nace con la siguiente compra o conteo.

## Cómo lo resuelven otros sistemas

Resumen (detalle y fuentes al final, «Investigación: lotes y caducidad en otros sistemas»): los POS
de restaurante casi no llevan caducidad por lote; el FEFO real vive en ERP como Odoo. Lo que se toma
de ellos y lo que se decide distinto:

| Tema | Lo común | Aquí |
| --- | --- | --- |
| Dónde se captura | Al recibir la compra | Al recibir **y** al contar (el caso real se vio al contar) |
| Quién elige el lote | El sistema, por regla | El sistema; nadie elige al vender (decisión del dueño) |
| Lote caducado | Odoo avisa pero lo deja salir; otros lo excluyen | Se excluye siempre; la existencia buena queda en negativo y la campana avisa |
| Sin fecha | Odoo lo saca al final | Sale primero (ver Assumptions: así el aviso de caducidad nunca se queda corto) |
| Alertas | Ventanas fijas 7/30/60/90 días | Caducado, 7, 30 y 60 días |
| Proyección contra consumo | Nadie la documenta | Sí: consumo diario, días de inventario y «no se termina antes de caducar» |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Recibir una compra con su fecha de caducidad (Priority: P1)

Quien recibe la mercancía captura, en cada renglón de la compra, la fecha de caducidad que trae el
empaque. Ese renglón se vuelve un lote del artículo en el almacén donde se recibió.

**Why this priority**: sin fechas no hay nada que avisar ni que ordenar. Es la entrada principal de
lotes y la que el personal ya hace con la mercancía en la mano.

**Independent Test**: recibir 2 kg de perlas con caducidad 15-dic en Bodega; la vista de caducidades
muestra el lote de Bodega con 2 kg y 15-dic.

**Acceptance Scenarios**:

1. **Given** una compra por recibir, **When** se recibe un renglón con fecha, **Then** el artículo
   gana un lote con esa cantidad y fecha en el almacén del renglón.
2. **Given** un renglón recibido sin fecha, **When** se guarda, **Then** se comporta como hoy: suma a
   la existencia sin fecha.
3. **Given** dos compras del mismo artículo con la misma fecha en el mismo almacén, **When** se
   reciben, **Then** quedan como un solo lote con la suma (el lote es artículo + almacén + fecha).
4. **Given** un renglón ya recibido con fecha, **When** se corrige su cantidad o su fecha, **Then** el
   lote cambia en la diferencia (o la cantidad pasa a la fecha nueva), sin duplicar mercancía.
5. **Given** un renglón recibido y su compra anulada, **When** se anula, **Then** su lote baja en lo
   que el renglón había sumado.

---

### User Story 2 - Lo que se vende sale del lote que caduca antes, nunca de uno caducado (Priority: P1)

Al vender, mermar o traspasar, el sistema descuenta solo y en este orden: primero la existencia sin
fecha, después los lotes buenos del que caduca antes al que caduca después. Un lote caducado no se
toca. Si lo bueno no alcanza, la venta sigue y la existencia buena queda en negativo; la campana avisa.

**Why this priority**: es lo que hace que las cantidades de cada lote sean ciertas. Sin esto, los
avisos de caducidad dirían cifras falsas.

**Independent Test**: con 1 kg sin fecha, 2 kg que caducan el 10-dic y 2 kg el 20-dic, vender 2 kg
deja 0 sin fecha, 1 kg del 10-dic y 2 kg del 20-dic.

**Acceptance Scenarios**:

1. **Given** lotes del 10-dic y del 20-dic, **When** se vende, **Then** se descuenta del 10-dic hasta
   agotarlo y luego del 20-dic.
2. **Given** existencia sin fecha y un lote, **When** se vende, **Then** sale primero lo sin fecha.
3. **Given** un lote que caducó ayer y uno bueno, **When** se vende más de lo que tiene el bueno,
   **Then** el bueno queda en 0, el caducado intacto, la existencia buena en negativo, la venta pasa
   sin aviso al cajero, y la campana abre «Se está usando [artículo] y lo que queda está caducado».
4. **Given** el aviso abierto, **When** el lote caducado se manda a merma o llega existencia buena
   que cubre el negativo, **Then** el aviso se cierra solo.
5. **Given** un lote que caduca hoy, **When** se vende, **Then** todavía se descuenta de él (caduca
   al terminar el día, en la hora del negocio).
6. **Given** una venta que ya descontó de dos lotes, **When** se cancela y repone, **Then** lo
   repuesto vuelve a esos mismos lotes (aunque uno haya caducado mientras tanto), no a «sin fecha».
7. **Given** una merma normal (no desde caducidades), **When** se registra, **Then** descuenta con el
   mismo orden que una venta.

---

### User Story 3 - Ver lo que caduca pronto y decidir a tiempo (Priority: P1)

Quien administra el almacén abre «Caducidades» y ve lo ya caducado y lo que caduca en 7, 30 y 60
días, con cantidad y almacén. Cada lote dice cuánto se usa al día, en cuántos días se terminaría y, si
no se termina antes de caducar, cuánto quedaría.

**Why this priority**: es el resultado que el dueño pidió: enterarse con 30 a 60 días de
anticipación y decidir una promoción, un uso alterno o una presentación más chica.

**Independent Test**: un lote de 3 kg que caduca en 20 días con consumo de 100 g/día dice «no se termina
antes de caducar: quedan ~0.9 kg» (se usa hasta el último día).

**Acceptance Scenarios**:

1. **Given** lotes con varias fechas, **When** se abre Caducidades, **Then** se agrupan en Caducado,
   7 días, 30 días y 60 días, del que caduca antes al que caduca después, con almacén y cantidad.
2. **Given** un artículo vendido en los últimos 30 días, **When** se ve su lote, **Then** muestra el
   consumo promedio diario y los días de inventario contando lo que se usará antes (lo sin fecha y
   los lotes que caducan antes).
3. **Given** un lote que no se va a terminar a tiempo, **When** se ve, **Then** lo marca «no se
   termina antes de caducar: quedan ~X».
4. **Given** un artículo sin ventas en 30 días, **When** se ve su lote, **Then** dice «No se ha vendido
   en 30 días» y lo marca como que no se terminará.
5. **Given** varios almacenes, **When** se elige uno, **Then** la vista muestra solo sus lotes; en
   «Todos», los de todos los almacenes de la sucursal.

---

### User Story 4 - Mandar lo caducado a merma desde la vista (Priority: P2)

Desde un lote de Caducidades, quien tiene permiso de registrar merma lo manda a merma con su motivo.
La merma sale de ese lote, no del bueno.

**Why this priority**: cierra el ciclo; sin esto el lote caducado se queda contando como existencia.

**Independent Test**: mandar a merma un lote caducado de 500 g con motivo «Caducado»; el lote
desaparece, la existencia total baja 500 g y la merma aparece en el registro con ese motivo.

**Acceptance Scenarios**:

1. **Given** un lote caducado, **When** se manda a merma completo con motivo, **Then** la merma se
   registra contra ese lote y el lote queda en 0.
2. **Given** un lote, **When** se manda a merma solo una parte, **Then** el lote baja esa parte.
3. **Given** una empresa donde la merma requiere aprobación, **When** se manda a merma, **Then** el
   lote baja al aprobarse, igual que cualquier merma.
4. **Given** quien no tiene permiso de registrar merma, **When** ve Caducidades, **Then** no ve la
   acción, y el servidor la niega.

---

### User Story 5 - Contar con fechas corrige los lotes (Priority: P2)

Al contar, quien cuenta puede decir de lo contado cuánto caduca en qué fecha (incluso una fecha ya
pasada: «esto ya caducó»). Al cerrar el conteo, los lotes del artículo en ese almacén quedan como se
contaron; lo contado sin fecha queda como existencia sin fecha.

**Why this priority**: es como nacen los lotes de la existencia que ya está en el estante, y como se
corrige un lote equivocado.

**Independent Test**: contar 5 kg de un artículo con 3 kg al 10-dic y 1 kg ya caducado el 20-nov; al
cerrar, el artículo tiene lote del 10-dic con 3 kg, lote caducado del 20-nov con 1 kg y 1 kg sin
fecha.

**Acceptance Scenarios**:

1. **Given** un artículo contado con desglose de fechas, **When** se cierra el conteo, **Then** sus
   lotes en ese almacén son los contados, y los lotes que no se mencionaron quedan en 0.
2. **Given** un artículo contado sin desglose, **When** se cierra, **Then** sus lotes no se reescriben:
   la diferencia del conteo se aplica con el mismo orden que una venta (si falta) o suma a sin fecha
   (si sobra).
3. **Given** lo vendido entre la captura y el cierre del conteo, **When** se cierra, **Then** se
   resta de los lotes contados en el orden de consumo, igual que el conteo ya lo hace con la existencia.
4. **Given** un desglose cuyas fechas suman más que lo contado, **When** se captura, **Then** se
   rechaza con un mensaje claro.

---

### User Story 6 - Un traspaso mueve lotes con su fecha (Priority: P2)

Al traspasar de Bodega a Local, lo que sale de Bodega sale en el orden de consumo y llega a Local con
las mismas fechas.

**Why this priority**: decisión del dueño; sin esto un traspaso borraría la fecha de lo que más se
mueve.

**Independent Test**: Bodega con 1 kg sin fecha y 4 kg al 10-dic; traspasar 3 kg a Local deja Bodega
con 2 kg al 10-dic y Local con 1 kg sin fecha y 2 kg al 10-dic.

**Acceptance Scenarios**:

1. **Given** un lote más grande que lo traspasado, **When** se traspasa, **Then** el lote se parte:
   una parte queda en el origen y otra llega al destino con la misma fecha.
2. **Given** lotes caducados en el origen, **When** se traspasa, **Then** no se mueven.
3. **Given** un traspaso que pide más de lo bueno del origen, **When** se guarda, **Then** se comporta
   como hoy (el origen puede quedar en negativo), sin mover lotes caducados.

---

### User Story 7 - El inventario avisa de lo que caduca (Priority: P3)

En la pantalla de existencias, un aviso de una línea dice cuántos lotes caducaron y cuántos caducan
en 7 días, y lleva a Caducidades.

**Why this priority**: es por donde ya pasa quien administra; sin esto la vista nueva depende de
acordarse de abrirla.

**Independent Test**: con 1 lote caducado y 2 que caducan esta semana, Existencias muestra
«1 caducado · 2 caducan en 7 días ›» y al tocarlo abre Caducidades.

**Acceptance Scenarios**:

1. **Given** lotes caducados o por caducar en 7 días en el almacén elegido, **When** se abre
   Existencias, **Then** se ve el aviso con las dos cifras.
2. **Given** ninguno, **When** se abre Existencias, **Then** no ocupa espacio.

### Edge Cases

Enumerados antes de diseñar (constitución IV):

- **Lote sin fecha**: un renglón de compra o un conteo sin fecha nunca crea un lote; suma a la
  existencia sin fecha. Un artículo que nunca recibe fecha se comporta exactamente como hoy.
- **Dos lotes con la misma fecha**: en el mismo almacén son uno solo (se suman). En almacenes
  distintos son dos.
- **Venta que agota lo bueno**: la existencia buena queda en negativo, los caducados intactos, la
  venta no se detiene ni pregunta nada, y la campana avisa una vez (no un aviso por venta).
- **Traspaso parcial de un lote**: el lote se parte entre origen y destino con la misma fecha.
- **Conteo que corrige un lote**: el desglose del conteo manda; lo vendido entre captura y cierre se
  resta de lo contado.
- **Cancelación de una venta que ya descontó lotes**: repone a los mismos lotes, aunque alguno ya haya
  caducado; lo que salió de «sin fecha» vuelve a «sin fecha». Una cancelación parcial repone primero
  lo último que se descontó.
- **Corrección de una compra cuyo lote ya se consumió en parte**: el lote baja hasta 0 y lo que no
  alcanza sale de la existencia sin fecha. Ningún lote queda negativo.
- **Lote que caduca hoy**: bueno hasta que termina el día en la zona horaria del negocio.
- **Existencia sin fecha negativa y llega un lote**: el lote entra con su cantidad completa; el
  negativo sin fecha se queda hasta que un conteo lo corrija (no se inventa de qué fecha era).
- **Unir dos artículos**: los lotes del origen pasan al destino con la equivalencia de la unión, en
  su almacén y fecha; si ya hay un lote con esa fecha, se suman.
- **Cambiar la unidad del artículo**: los lotes se convierten igual que la existencia.
- **Archivar un almacén**: ya exige existencias en 0; con lotes en 0 no hay nada que mover.
- **Fecha absurda**: una fecha más de 10 años en el futuro o anterior al 2000 se rechaza al
  capturarla. Una fecha pasada sí se acepta en el conteo (lo que ya caducó); al recibir, la pantalla
  no la ofrece.
- **Versión anterior de la API durante la subida**: sigue moviendo existencias sin fechas; sus
  ventas, mermas, traspasos y conteos descuentan con el mismo orden porque la regla vive en la base,
  no en cada pantalla.
- **Producto con existencia propia** (una lata de reventa): fuera de esta entrega; sigue como hoy.

## Requirements *(mandatory)*

### Functional Requirements

**Lotes**

- **FR-001**: Un lote es un artículo del almacén, en un almacén, con una fecha de caducidad y una
  cantidad mayor que cero. Dos entradas con el mismo artículo, almacén y fecha son el mismo lote.
- **FR-002**: La existencia sin fecha de un artículo en un almacén es su existencia total menos la
  suma de sus lotes ahí. Puede ser negativa.
- **FR-003**: La existencia total de un artículo no cambia de significado: lotes y existencia sin
  fecha son un desglose de ella.
- **FR-004**: Un lote está caducado cuando su fecha es anterior al día de hoy en la zona horaria del
  negocio. La existencia buena es la total menos los lotes caducados.
- **FR-005**: Al subir esta entrega no se crea ningún lote: toda la existencia de hoy queda sin fecha.

**Entradas**

- **FR-010**: Al recibir un renglón de compra se puede capturar una fecha de caducidad opcional; con
  ella, lo recibido suma a ese lote en el almacén del renglón.
- **FR-011**: Corregir lo recibido, quitar el renglón o anular la compra mueve el lote en la diferencia,
  sin dejarlo negativo; cambiar la fecha de un renglón recibido pasa su cantidad al lote de la fecha
  nueva.
- **FR-012**: Al recibir no se ofrecen fechas anteriores a hoy (recibir algo ya caducado no es una
  entrada normal; si pasa, se registra al contar). El servidor acepta cualquier fecha creíble.

**Salidas**

- **FR-020**: Toda salida sin lote indicado (venta, merma normal, ajuste de conteo hacia abajo,
  traspaso de salida, cualquier otra) descuenta primero de la existencia sin fecha mientras sea
  positiva y después de los lotes buenos, del que caduca antes al que caduca después.
- **FR-021**: Ninguna salida automática descuenta de un lote caducado. Lo que no alcance deja la
  existencia buena en negativo; la salida nunca se bloquea ni pregunta.
- **FR-022**: Cuando una salida deja la existencia buena en negativo y el artículo tiene lotes
  caducados en ese almacén, se abre un aviso en la campana («Se está usando [artículo] y lo que queda
  está caducado»), uno solo abierto por artículo y almacén. Se cierra solo cuando la existencia buena
  vuelve a cero o más o cuando ya no hay lotes caducados.
- **FR-023**: Una cancelación o reposición de venta devuelve la cantidad a los mismos lotes de los que
  salió (lo último que salió, primero), aunque ya hayan caducado; el resto vuelve a sin fecha.
- **FR-024**: El orden de descuento lo aplica el sistema en un solo lugar para todos los caminos que
  mueven existencias, incluida la versión anterior durante la subida.

**Traspasos**

- **FR-030**: Un traspaso saca del origen con el orden de FR-020 y crea en el destino lotes con las
  mismas fechas y cantidades; lo que salió sin fecha llega sin fecha.

**Conteo**

- **FR-040**: Al capturar un artículo en el conteo se puede desglosar lo contado en fechas
  (cantidad + fecha, varias), incluidas fechas pasadas. La suma del desglose no puede pasar de lo
  contado.
- **FR-041**: Al cerrar, un artículo con desglose deja sus lotes en ese almacén igual al desglose,
  menos lo que se descontó de esos lotes entre la captura y el cierre; sin desglose, sus lotes no se
  reescriben y la diferencia se aplica como en FR-020.
- **FR-042**: La pantalla de conteo se alinea con la maqueta elegida en la spec 096 (lista del lugar y
  teclado); esta entrega agrega el desglose de fechas sin rediseñar el conteo.

**Caducidades**

- **FR-050**: Una vista «Caducidades» en Almacén lista los lotes con cantidad mayor que cero agrupados
  en «Ya caducó», «Esta semana», «8 a 30 días», «31 a 60 días» y «Más adelante» (cerrado), con
  artículo, almacén, fecha y cantidad, filtrable por almacén. Sin lotes, dice cómo registrarlos.
- **FR-051**: Cada lote muestra el consumo promedio diario del artículo (ventas netas de cancelaciones
  de los últimos 30 días en la sucursal), los días que faltan para caducar, en cuántos días se
  terminaría contando lo que se usará antes (sin fecha y lotes que caducan antes), y si no se termina
  antes de caducar, cuánto quedaría.
- **FR-052**: Desde un lote, quien tiene permiso de registrar merma lo manda a merma (total o parcial)
  con un motivo; esa merma descuenta de ese lote. Mientras espera aprobación, el lote lo dice y no
  ofrece mandarla otra vez.
- **FR-053**: Ver Caducidades exige el permiso de ver inventario; mandar a merma, el de registrar
  merma. El servidor lo impone.
- **FR-054**: Existencias muestra un aviso de una línea con lotes caducados y por caducar en 7 días del
  almacén elegido, que lleva a Caducidades; sin ninguno, no se muestra.

**Integridad**

- **FR-060**: Los lotes respetan el aislamiento entre empresas como toda tabla de empresa.
- **FR-061**: Unir artículos pasa los lotes al destino con su equivalencia; cambiar la unidad los
  convierte.
- **FR-062**: Ningún lote queda con cantidad negativa ni en un almacén de otra sucursal.

### Key Entities

- **Lote**: artículo, almacén (y su sucursal), fecha de caducidad, cantidad que queda.
- **Descuento de un lote**: qué movimiento de existencia tomó o devolvió cuánto de qué lote. Permite
  reponer a los mismos lotes y llevar las fechas en un traspaso.
- **Desglose de fechas del conteo**: por artículo contado, cantidades por fecha.
- **Fecha del renglón de compra**: la caducidad que se capturó al recibir.
- **Aviso «lo que queda está caducado»**: aviso de la campana por artículo y almacén.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Registrar la fecha de un renglón al recibir cuesta como máximo 3 toques adicionales, para
  cualquier fecha de los próximos dos años.
- **SC-002**: En las pruebas de los bordes enumerados, la suma de lotes más la existencia sin fecha es
  igual a la existencia total en el 100 % de los casos.
- **SC-003**: Ninguna venta, merma o traspaso automático descuenta de un lote caducado (0 casos en las
  pruebas).
- **SC-004**: Una persona nueva de una fonda encuentra lo que caduca esta semana y manda a merma un
  lote caducado sin ayuda, a 1024×600.
- **SC-005**: Un pedido de 90 movimientos de artículos con lotes tarda como máximo el doble que el
  mismo pedido sin lotes, y uno sin lotes no tarda más que hoy (medido en el Postgres de pruebas).
- **SC-006**: Al subir, ninguna existencia cambia y ningún lote aparece inventado.

## Assumptions

- El consumo promedio usa una ventana fija de 30 días (el issue pide 30–60); no es configurable.
- La existencia sin fecha se consume primero: es la existencia anterior a esta entrega (la más vieja)
  o la que alguien decidió no fechar. Consumir primero lo fechado haría que un lote pareciera más
  chico de lo que es y el aviso de caducidad se quedaría corto, que es justo el error que el issue
  quiere evitar.
- Un renglón de compra lleva una sola fecha; dos fechas en una entrega son dos renglones.
- El traspaso elige los lotes solo; no se pide elegir lote al traspasar.
- Solo los artículos del almacén (insumos) llevan lotes; los productos con existencia propia quedan
  fuera de esta entrega.
- No hay aviso por correo ni recordatorio programado de «próximos a caducar»: el aviso vive en
  Existencias y en Caducidades (el issue deja fuera las notificaciones externas).

## Fuera de alcance

- Número de lote del fabricante y trazabilidad por proveedor.
- Elegir lote a mano al vender o traspasar.
- Lotes de productos con existencia propia.
- Notificaciones fuera del sistema.
- Rediseño de las pantallas de conteo y existencias (es la subida 2 de la spec 096).

## Investigación: lotes y caducidad en otros sistemas

Investigado el 2026-11-23. **Solo Odoo tiene documentación pública verificable**; en MarketMan,
xtraCHEF (Toast), Square, Lightspeed, Restaurant365, Apicbase, Loyverse y Soft Restaurant no se
encontró página del fabricante que describa lotes con caducidad.

- **Odoo**: lotes con cuatro fechas (caducidad, consumo preferente, retiro, alerta), capturadas al
  recibir y calculadas con días por producto
  ([doc 17.0](https://www.odoo.com/documentation/17.0/applications/inventory_and_mrp/inventory/product_management/product_tracking/expiration_dates.html)).
  FEFO es una estrategia de salida que toma el lote con la fecha de retiro más cercana y reparte un
  pedido entre varios lotes
  ([FEFO](https://www.odoo.com/documentation/17.0/applications/inventory_and_mrp/inventory/shipping_receiving/removal_strategies/fefo.html)).
  Lo sin fecha sale al final (doc 13.0). Un lote caducado se sigue tomando; solo avisa. Su POS
  estándar no elige lote: lo hacen módulos de terceros.
- **Square**: solo cantidad; la caducidad es petición de usuarios en su foro
  ([foro](https://community.squareup.com/t5/Feature-Requests/Square-Feature-Request-Expiration-Date-Tracking/idi-p/810906)).
- **Toast/xtraCHEF, Lightspeed, Restaurant365, MarketMan**: merma, varianza y recepción digital;
  lotes con caducidad no verificados en fuentes del fabricante.
- **Fuera de restaurantes**: Paradigm asigna el lote más próximo a caducar que no esté caducado; Zoho
  filtra lo que caduca en 7/30/60/90 días; Anchanto no deja corregir la fecha de un lote recibido.
- **No verificado**: cómo mueven lotes en traspasos los sistemas de restaurante, y si alguno proyecta
  el consumo contra la caducidad (no se encontró ninguno).
