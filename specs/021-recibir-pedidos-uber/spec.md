# Feature Specification: Recibir los pedidos de Uber Eats

**Feature Branch**: `021-recibir-pedidos-uber`

**Created**: 2026-09-16

**Status**: Draft

**Input**: User description: "Recibir los pedidos de Uber Eats por webhook, para que lleguen solos al POS y el operador solo tenga que aceptar e imprimir."

## Por qué existe esta feature

Hoy la integración con las plataformas es de **una sola dirección**: el sistema lee el menú publicado
y dice en qué difiere del catálogo (spec 020). Un pedido hecho en Uber Eats **no llega al sistema**.
Alguien lo lee en la tableta de Uber, lo vuelve a capturar en el POS producto por producto, y de ahí
salen las tres cosas que ya cuestan dinero: un renglón que se teclea mal, un pedido que nunca se
captura y no aparece en el corte, y el tiempo de quien estaba atendiendo el mostrador.

Esta feature cierra ese camino. El objetivo declarado por el dueño, textual: *"el operador no
debería de hacer nada más que simplemente aceptar el pedido e imprimir"*.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El pedido llega solo y se acepta de un toque (Priority: P1)

Un cliente pide en Uber Eats. Segundos después el pedido aparece en la pantalla del POS con sus
platillos, sus opciones y lo que el cliente pagó, sin que nadie haya tecleado nada. Quien atiende lo
ve, toca **Aceptar**, y el ticket se imprime para la cocina.

**Why this priority**: Es la feature. Sin esto no hay nada: el resto de las historias son lo que
pasa cuando algo sale mal en este camino.

**Independent Test**: Se dispara un evento de pedido firmado contra el ambiente de pruebas y se
verifica que el pedido aparece completo en la pantalla, que al aceptarlo queda registrado en el POS
con sus renglones, y que se imprime. No necesita que Uber mande nada: el evento se firma con la
misma llave que se configuró.

**Acceptance Scenarios**:

1. **Given** una tienda conectada con su llave de firma configurada, **When** llega un evento de
   pedido nuevo correctamente firmado, **Then** el pedido queda registrado como pendiente de
   aceptar, con sus platillos emparejados contra el catálogo y el total que cobró la plataforma.
2. **Given** un pedido pendiente de aceptar, **When** el operador toca Aceptar, **Then** el pedido
   entra al flujo normal del POS como un pedido ya pagado por la plataforma, se imprime el ticket de
   cocina, y la plataforma recibe la confirmación.
3. **Given** un pedido pendiente, **When** pasan 90 segundos sin que nadie lo toque, **Then** la
   pantalla lo señala de forma que no se pueda ignorar, porque a esa altura la plataforma ya está
   llamando por teléfono al local.
4. **Given** un pedido pendiente, **When** se agota el plazo sin que nadie decida, **Then** el
   sistema NO decide por el operador: la plataforma cancela, y el sistema lo registra y lo muestra.

---

### User Story 2 - Rechazar un pedido que no se puede preparar (Priority: P1)

Se acabó un ingrediente, la cocina está saturada o el pedido trae algo que no se puede hacer. Quien
atiende toca **Rechazar**, elige el motivo de una lista corta, y la plataforma se entera.

**Why this priority**: Es la otra mitad de la misma decisión, y no es opcional: si no se puede
rechazar desde el POS, hay que rechazarlo en la tableta de Uber — y entonces el operador sigue
trabajando en dos pantallas, que es justo lo que esta feature viene a quitar.

**Independent Test**: Sobre un pedido pendiente, tocar Rechazar con un motivo y verificar que la
plataforma recibió el rechazo con ese motivo y que el pedido queda registrado como rechazado, con
quién y por qué.

**Acceptance Scenarios**:

1. **Given** un pedido pendiente, **When** el operador lo rechaza eligiendo un motivo, **Then** la
   plataforma recibe el rechazo con ese motivo y el pedido queda cerrado en el sistema sin contar
   como venta.
2. **Given** un pedido que ya se aceptó, **When** el operador intenta rechazarlo, **Then** el
   sistema no lo permite y explica que ya fue aceptado.

---

### User Story 3 - Un pedido no se pierde aunque algo falle (Priority: P1)

La plataforma reintenta lo que no se le confirma, puede mandar el mismo aviso dos veces y no
garantiza el orden de llegada. Un pedido duplicado en la pantalla hace que la cocina prepare dos
veces; un pedido perdido hace que el cliente espere comida que nadie está haciendo.

**Why this priority**: Es P1 y no P2 porque el fallo es **silencioso**: nada truena, simplemente hay
un pedido de más o de menos, y se descubre cuando el cliente reclama.

**Independent Test**: Disparar el mismo evento tres veces y verificar que hay UN pedido. Disparar un
aviso de cancelación antes que el de creación y verificar que el resultado final es correcto.

**Acceptance Scenarios**:

1. **Given** un aviso de pedido ya procesado, **When** llega otra vez el mismo aviso, **Then** el
   sistema lo reconoce, no crea un segundo pedido y confirma la recepción igual.
2. **Given** un aviso cuyo detalle no se pudo traer de la plataforma, **When** el sistema responde,
   **Then** NO confirma la recepción, para que la plataforma lo reintente, y queda registrado que
   falló y por qué.
3. **Given** un aviso cuya firma no cuadra, **When** llega, **Then** se rechaza sin procesarlo y
   queda un evento de seguridad, sin filtrar en el mensaje qué parte de la firma falló.

---

### User Story 4 - Saber qué pasó sin abrir la consola (Priority: P2)

Quien administra abre una pantalla y ve los pedidos que llegaron de la plataforma, cuáles se
aceptaron, cuáles se rechazaron y con qué motivo, y cuáles fallaron al procesarse.

**Why this priority**: No hace falta para vender, hace falta para confiar. La primera vez que un
cliente diga *"yo sí pedí"* y en el POS no esté, alguien tiene que poder responder si el pedido
nunca llegó o si llegó y falló.

**Independent Test**: Provocar un pedido aceptado, uno rechazado y uno fallido, y verificar que los
tres aparecen en la pantalla con su estado y su hora.

**Acceptance Scenarios**:

1. **Given** pedidos de plataforma con distintos desenlaces, **When** se abre la pantalla, **Then**
   se ven todos con su estado, su hora y el motivo cuando lo hay.

---

### User Story 5 - Enterarse de que la plataforma cerró la tienda (Priority: P2)

Uber cierra la tienda de su lado cuando se dejan pedidos sin responder. Dejan de entrar pedidos, no
falla nada y nadie se entera hasta que alguien nota que hoy no ha vendido en la app.

**Why this priority**: Es el costo real de dejar expirar un pedido, y es invisible. No es P1 porque
no bloquea recibir ni aceptar; es P2 porque sin esto el negocio puede estar cerrado en la app
durante horas creyendo que simplemente no hay clientes.

**Independent Test**: Disparar el aviso de cambio de estado de la tienda y verificar que la pantalla
lo dice de forma visible y que queda registrado cuándo pasó.

**Acceptance Scenarios**:

1. **Given** una tienda recibiendo pedidos, **When** la plataforma la cierra, **Then** quien opera
   lo ve de forma imposible de pasar por alto, con la hora y —si la plataforma la da— la razón.
2. **Given** una tienda cerrada por la plataforma, **When** vuelve a abrirse, **Then** el aviso
   desaparece.

---

### User Story 6 - La tienda se entera de que la conectaron o la desconectaron (Priority: P3)

La plataforma avisa cuando una tienda otorga o retira el acceso a nuestra aplicación. Si se retira y
el sistema no se entera, sigue mostrando una tienda conectada que ya no lo está.

**Why this priority**: Hoy hay una sola tienda y el dueño se entera por otros medios. Importa cuando
haya varias empresas.

**Independent Test**: Disparar el aviso de desconexión y verificar que la conexión queda marcada
como inactiva y que la pantalla lo dice.

**Acceptance Scenarios**:

1. **Given** una tienda conectada, **When** llega el aviso de que se retiró el acceso, **Then** la
   conexión queda marcada como inactiva y la pantalla de plataformas lo refleja.

---

### Edge Cases

- **Un platillo del pedido no tiene pareja en el catálogo.** El pedido se acepta igual: el cliente
  ya pagó y rechazarlo por un hueco de nuestra contabilidad interna sería absurdo. El renglón se
  registra con el nombre y el precio que mandó la plataforma, y queda marcado como sin emparejar
  para que alguien lo resuelva después.
- **Llega un pedido y no hay turno de caja abierto.** Recibir y aceptar no dependen del cajón: el
  pedido se registra y se acepta igual, y entra al corte del turno que se abra después. Hoy un
  pedido de plataforma exige turno abierto — esta feature levanta esa restricción para el camino
  automático, y lo paga mostrándolo en la apertura (FR-023).
- **La plataforma cierra la tienda por pedidos sin responder.** No es hipotético: es lo que hacen
  Uber y DiDi. Deja de entrar dinero sin que nada falle. El sistema tiene que decirlo (FR-028).
- **El aviso llega con la cabecera de ambiente equivocada.** Un aviso marcado como de producción
  contra el ambiente de pruebas, o al revés, se rechaza: procesarlo mezclaría un pedido real con
  datos de prueba.
- **La plataforma cancela un pedido que ya está en la cocina.** El pedido se marca cancelado y se
  avisa de forma visible; no se borra ni se esconde, porque la comida ya se hizo y alguien tiene que
  decidir qué hacer con ella.
- **El detalle del pedido tarda o falla.** El sistema no confirma la recepción, la plataforma
  reintenta, y el intento fallido queda registrado. Nunca se confirma un pedido que no se pudo leer.
- **Un aviso de un tipo que no manejamos.** Se confirma la recepción y se registra, sin procesarlo:
  no confirmar haría que la plataforma reintente para siempre algo que nunca vamos a usar.
- **Un cuerpo enorme o mal formado.** Se rechaza por tamaño antes de intentar interpretarlo.
- **Llega un aviso de una tienda que no es de ninguna empresa nuestra.** Se rechaza y queda evento
  de seguridad: es el caso de una llave filtrada o de una configuración equivocada.

## Requirements *(mandatory)*

### Functional Requirements

**Recibir**

- **FR-001**: El sistema MUST exponer un punto de entrada público donde la plataforma entrega sus
  avisos, sin sesión de usuario, porque quien lo llama es la plataforma y no una persona.
- **FR-002**: El sistema MUST verificar la firma de cada aviso contra la llave configurada para esa
  tienda, y MUST rechazar sin procesar todo aviso cuya firma no cuadre.
- **FR-003**: El sistema MUST admitir **dos llaves de firma válidas a la vez** para una misma
  tienda, de modo que se pueda cambiar la llave sin dejar de recibir pedidos mientras dura el
  cambio.
- **FR-004**: El sistema MUST rechazar todo aviso cuyo ambiente declarado no corresponda al del
  sistema que lo recibe.
- **FR-005**: El sistema MUST responder la confirmación que la plataforma espera **solo cuando el
  aviso quedó procesado o descartado a propósito**, y MUST no confirmarlo cuando falló, para que la
  plataforma lo reintente.
- **FR-006**: El sistema MUST responder en menos de lo que la plataforma tolera antes de considerar
  la entrega fallida, incluso cuando traer el detalle del pedido sea lento.
- **FR-007**: El sistema MUST reconocer un aviso ya procesado por su identificador y MUST no
  producir un segundo pedido, confirmando la recepción igual.
- **FR-008**: El sistema MUST tolerar que los avisos lleguen en desorden, y MUST no depender de que
  el aviso de creación llegue antes que el de cancelación.
- **FR-009**: El sistema MUST rechazar un cuerpo que exceda un tamaño máximo, antes de interpretarlo.
- **FR-010**: El sistema MUST limitar la frecuencia con la que acepta avisos por origen, sin que ese
  límite pueda tumbar la recepción de pedidos legítimos.

**Registrar**

- **FR-011**: El sistema MUST traer el detalle completo del pedido siguiendo la referencia que el
  aviso entrega, porque el aviso no trae el pedido.
- **FR-012**: El sistema MUST registrar el pedido con el folio de la plataforma, la tienda a la que
  llegó, sus renglones, sus opciones y el total que la plataforma cobró.
- **FR-013**: El sistema MUST emparejar cada renglón contra el catálogo usando el emparejamiento que
  ya existe, y MUST registrar como *sin emparejar* el renglón que no tenga pareja, sin impedir que
  el pedido se procese.
- **FR-014**: El sistema MUST tomar como verdadero el precio que cobró la plataforma, no el del
  catálogo. La plataforma es la fuente de la verdad en precio.
- **FR-015**: El sistema MUST guardar el aviso crudo tal como llegó, para poder reconstruir qué pasó
  cuando un pedido salga mal.
- **FR-016**: El sistema MUST conservar los datos del cliente solo en la medida en que hacen falta
  para preparar y entregar el pedido, y MUST mantenerlos fuera de los registros de operación.

**Decidir**

- **FR-017**: Quien atiende MUST poder aceptar un pedido pendiente con un solo toque, y el sistema
  MUST informar la aceptación a la plataforma.
- **FR-018**: Quien atiende MUST poder rechazar un pedido pendiente eligiendo un motivo de la lista
  que la plataforma admite, y el sistema MUST informar el rechazo con ese motivo.
- **FR-019**: El sistema MUST registrar quién aceptó o rechazó cada pedido y cuándo.
- **FR-020**: El sistema MUST impedir aceptar o rechazar un pedido que ya fue decidido, y MUST
  decirlo en vez de fallar en silencio.
- **FR-021**: Un pedido aceptado MUST entrar al flujo normal del POS **ya pagado por la plataforma**,
  y MUST no aparecer como dinero por cobrar en el turno de caja.
- **FR-022**: Aceptar un pedido MUST funcionar **aunque no haya turno de caja abierto**: la cocina
  no puede quedarse esperando a que alguien abra caja. El pedido entra al corte del siguiente turno.
- **FR-023**: Al abrir un turno de caja, quien lo abre MUST ver **de un vistazo** qué pedidos de
  plataforma ya aceptados quedan considerados en esa apertura. Sin eso, aparece dinero que nadie
  recuerda haber cobrado y el arqueo deja de cuadrar sin explicación.
- **FR-024**: El sistema MUST imprimir el ticket de cocina al aceptar, respetando la configuración de
  impresión que ya existe.

**Avisar**

- **FR-025**: La pantalla donde se atiende MUST mostrar un pedido pendiente de forma que no se pueda
  pasar por alto, y MUST hacerlo más insistente conforme se acerca el plazo.
- **FR-026**: La pantalla MUST mostrar cuánto tiempo queda para decidir.
- **FR-027**: El sistema MUST dejar que la plataforma cancele el pedido cuando se agota el plazo, y
  MUST no decidir por el operador. Lo único que hace mientras tanto es avisar cada vez más fuerte
  (FR-025). Rechazar solo sería el sistema decidiendo no vender, y esa no es una decisión de
  software.
- **FR-028**: El sistema MUST enterarse cuando la plataforma **cierra la tienda** por dejar pedidos
  sin responder, y MUST decírselo a quien opera de forma imposible de ignorar. Es el castigo real de
  dejar expirar un pedido: la tienda deja de recibir, no hay error en ningún lado y el negocio se
  entera cuando alguien nota que hoy no ha entrado nada.
- **FR-029**: El sistema MUST reflejar la cancelación hecha por la plataforma sobre un pedido ya
  aceptado, de forma visible para quien atiende.
- **FR-030**: El sistema MUST marcar una conexión como inactiva cuando la plataforma avise que se
  retiró el acceso.

**Poder probarlo**

- **FR-031**: El sistema MUST poder validarse de extremo a extremo disparando avisos firmados contra
  el ambiente de pruebas, sin depender de que la plataforma genere un pedido.

### Key Entities

- **Aviso recibido**: cada entrega de la plataforma. Su identificador, su tipo, la tienda, cuándo
  llegó, si se pudo procesar y qué falló si no. Es lo que permite deduplicar y lo que permite
  reconstruir un pedido que salió mal.
- **Pedido de plataforma pendiente**: un pedido que llegó y todavía no se decide. Vive **aparte** del
  pedido del POS hasta que alguien lo acepta — ver *Assumptions*. Guarda el folio de la plataforma,
  sus renglones con su emparejamiento, el total cobrado y el plazo para decidir.
- **Renglón del pedido**: qué se pidió, cuántos, a qué precio lo cobró la plataforma, con qué
  producto del catálogo quedó emparejado, y sus opciones.
- **Llave de firma de la conexión**: el secreto con el que se verifica cada aviso de esa tienda.
  Dos válidas a la vez para poder rotarlas.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Un pedido hecho en la plataforma aparece en la pantalla del POS en **menos de 10
  segundos**, sin que nadie teclee nada.
- **SC-002**: Aceptar un pedido cuesta **un solo toque**, y el ticket sale sin pasos adicionales.
- **SC-003**: **Cero pedidos duplicados** al reenviar el mismo aviso cinco veces seguidas.
- **SC-004**: **Cero pedidos perdidos**: todo aviso que el sistema confirmó tiene su pedido o su
  descarte explicado, y todo aviso que falló quedó registrado con su causa.
- **SC-005**: Un aviso con firma inválida **nunca** produce un pedido, y queda registrado como
  evento de seguridad.
- **SC-006**: La captura manual de un pedido de plataforma **deja de ser necesaria**: el operador no
  vuelve a abrir la tableta de la plataforma para leer un pedido.
- **SC-007**: El total registrado en el POS **coincide al centavo** con el que cobró la plataforma,
  en el 100% de los pedidos recibidos.
- **SC-008**: La feature se puede demostrar de principio a fin en el ambiente de pruebas **sin
  intervención de la plataforma**.

## Assumptions

- **El pedido pendiente NO es todavía un pedido del POS.** Vive aparte hasta que alguien lo acepta.
  Se decide así porque un pedido que aún no se acepta no es una venta, y meterlo en el flujo normal
  lo haría aparecer en el tablero de cocina, en la deuda por cobrar y en el corte de caja antes de
  que exista. El principio III manda: cada peso se clasifica una sola vez, y lo que no es ingreso no
  entra al total.
- **La plataforma manda en el precio.** Lo que cobró es lo que se registra, aunque el catálogo diga
  otra cosa. Ya está documentado en `AGENTS.md` y aquí se aplica.
- **Se reusa lo que ya existe** y no se vuelve a construir: la conexión de la tienda con su
  `external_store_id`, el emparejamiento platillo-producto, el folio de plataforma en el pedido, y
  la configuración de impresión.
- **La llave de firma es un secreto por tienda**, no una constante del sistema: cada empresa tiene su
  aplicación en la plataforma y por lo tanto su propia llave.
- **Con qué secreto firma la plataforma está sin resolver.** Su documentación dice una cosa y su
  tablero muestra otra, y hay un ticket abierto. El diseño no puede depender de cuál sea: guarda la
  llave como un valor configurable por tienda.
- **El local tiene conexión** (decisión del 2026-09-08). Recibir pedidos exige red y eso está
  aceptado; lo que hoy funciona sin red no se vuelve dependiente de ella.
- **Enterarse de que la plataforma cerró la tienda depende de un permiso que hoy NO tenemos.** Uber
  manda ese aviso solo a las aplicaciones autorizadas explícitamente para recibirlo. Está en el
  ticket abierto con la plataforma. Si no se concede a tiempo, FR-028 se cumple de la forma que el
  plan decida, pero la necesidad no se negocia.
- **DiDi y Rappi harán lo mismo, y no está investigado.** Se sabe que DiDi también cierra la tienda;
  de Rappi se desconoce. Es investigación de cuando se integre cada una, no de ahora.
- **No se construye aceptación automática.** El operador decide. Aceptar solo lo que el sistema
  puede preparar es una decisión de negocio, no de software, y automatizarla hoy no tiene cómo
  saber si hay ingredientes.
- **Una sola plataforma en esta iteración.** Se diseña para que quepan las demás, pero solo se
  construye Uber Eats — es la única con acceso concedido.

## Fuera de alcance

- Publicar o modificar el menú hacia la plataforma.
- Promociones y su contabilidad.
- Conciliar el depósito de la plataforma contra los pedidos que lo formaron (spec 014 ya cerró la
  parte que se podía cerrar).
- Otras plataformas de reparto.
- Aceptación automática.
- Pedidos programados para más tarde.
