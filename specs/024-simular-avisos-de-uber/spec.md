# Feature Specification: Simular los avisos de Uber contra nuestro propio ambiente

**Feature Branch**: `024-simular-avisos-de-uber`

**Created**: 2026-09-21

**Status**: Draft

**Input**: User description: "Simular los avisos de Uber Eats contra nuestro propio ambiente, con pedidos armados a partir del menú real, para poder probar la recepción de punta a punta sin depender de la plataforma."

## Por qué existe esta feature

La spec 021 promete, en su FR-031, que recibir pedidos se puede validar **sin depender de la
plataforma**. Esa promesa no es opcional: Uber **no documenta** cómo generar un pedido de prueba —se
investigó y no existe camino público— y el dueño decidió no abrir el ticket para no quedarse
bloqueado. O sea que hoy, si no lo simulamos nosotros, la feature **no se puede probar de ninguna
forma**.

Y hoy esa promesa es teórica. Lo único que existe es el quickstart de la 021, que pide armar el
cuerpo a mano, calcular el HMAC con `openssl` y mandarlo con `curl`. Nadie va a hacer eso cada vez,
y menos con un pedido realista de varios renglones con opciones anidadas. Una verificación que
cuesta quince minutos de teclado no se corre, y una que no se corre no es una verificación.

Lo que falta es poder disparar un pedido **armado con nuestro propio menú** y ver qué pasa.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Un pedido realista entra de un comando (Priority: P1)

Quien desarrolla lanza un escenario y, segundos después, ve en la tableta un pedido con platillos de
su catálogo, con sus opciones, y con el precio que la plataforma habría cobrado. Lo acepta y observa
el ticket, el pedido en cocina y el efecto en el corte.

**Why this priority**: Es la feature. Sin esto, la 021 no se puede probar.

**Independent Test**: Lanzar el escenario simple contra el ambiente de pruebas y ver el pedido en la
pantalla, con sus renglones emparejados contra el catálogo real.

**Acceptance Scenarios**:

1. **Given** una tienda conectada con su llave de firma, **When** se lanza el escenario simple,
   **Then** el pedido aparece en la tableta con platillos que existen en el catálogo, sus opciones y
   su total.
2. **Given** un escenario lanzado, **When** se revisa lo que se mandó, **Then** el aviso venía
   **firmado con la llave real** de esa conexión — no se saltó la verificación, la ejerció.
3. **Given** que el catálogo no tiene ningún platillo emparejado con la plataforma, **When** se
   lanza un escenario, **Then** el simulador lo dice y no inventa productos.

---

### User Story 2 - Los casos difíciles se lanzan igual de fácil que el fácil (Priority: P1)

Los bordes que la 021 nombra como caros —una cancelación que llega antes que su notificación, el
mismo aviso repetido, un platillo sin pareja, un pedido para recoger— se lanzan con un comando
cada uno, no armando JSON a mano.

**Why this priority**: Es donde están los defectos. El camino feliz se prueba solo la primera vez;
los bordes solo se prueban si cuestan poco.

**Independent Test**: Lanzar cada escenario y verificar que el sistema responde como el spec 021
dice que debe responder.

**Acceptance Scenarios**:

1. **Given** el escenario del aviso repetido, **When** se lanza, **Then** el sistema confirma las
   veces que haga falta y queda **un** pedido.
2. **Given** el escenario de la cancelación que llega primero, **When** se lanza, **Then** el pedido
   queda cancelado y nunca aparece como pendiente.
3. **Given** el escenario del platillo sin pareja, **When** se lanza, **Then** el pedido entra igual
   y el renglón queda señalado como sin emparejar.
4. **Given** el escenario del pedido para recoger, **When** se lanza, **Then** entra como tal y no
   como pedido a domicilio.
5. **Given** el escenario del detalle que no se puede traer, **When** se lanza, **Then** el sistema
   **no confirma** el aviso y queda registrada la causa.

---

### User Story 3 - Un pedido simulado no puede tocar producción (Priority: P1)

Quien desarrolla apunta el simulador por error a producción y **no pasa nada**: el disparo se
rechaza antes de mandar un solo byte.

**Why this priority**: Es lo que más importa de toda la feature, y es P1 aunque no aporte una sola
funcionalidad. Un pedido simulado **entra al POS como una venta real**: crea un pedido, registra un
pago y suma al corte de caja. Si alguien simula contra producción, el corte del día queda mal y
**nadie lo nota hasta que no cuadra** — y para entonces nadie puede distinguir qué venta fue de
verdad.

Un aviso que se rechaza es una molestia de diez segundos. Un corte contaminado es un turno completo
tratando de entender dónde se perdió el dinero, sin poder.

**Independent Test**: Apuntar el simulador a producción y ver que se niega, sin haber mandado nada.

**Acceptance Scenarios**:

1. **Given** el simulador apuntado a la dirección de producción, **When** se lanza cualquier
   escenario, **Then** se niega **antes de mandar la petición** y dice por qué.
2. **Given** un ambiente que se identifica como productivo, **When** llega un aviso simulado,
   **Then** el sistema lo rechaza, aunque venga bien firmado.
3. **Given** un pedido simulado ya registrado, **When** alguien lo ve en cualquier pantalla,
   **Then** se distingue a simple vista de un pedido real.

---

### User Story 4 - Lo que se crea probando no le estorba a nadie (Priority: P2)

Quien termina de probar deja el ambiente como lo encontró: sin pedidos abiertos colgando ni dinero
fantasma en la barra del POS.

**Why this priority**: El ambiente de pruebas se comparte con una persona. Un pedido de prueba
abierto aparece en la barra, suma a «por cobrar» y **bloquea el cierre de caja**: le hace creer a
quien opera que hay dinero pendiente.

**Independent Test**: Lanzar varios escenarios, pedir la limpieza, y verificar que el ambiente queda
sin pedidos simulados abiertos y que **los pedidos que ya estaban** siguen intactos.

**Acceptance Scenarios**:

1. **Given** varios pedidos simulados abiertos, **When** se pide limpiar, **Then** se cierran solo
   los que el simulador creó.
2. **Given** un pedido de una persona abierto desde antes, **When** se pide limpiar, **Then** ese
   pedido **no se toca**.

---

### Edge Cases

- **La tienda no tiene llave de firma capturada.** El simulador no puede firmar: lo dice y explica
  dónde se captura, en vez de mandar algo que va a rebotar con «no autenticado».
- **El catálogo cambió entre un lanzamiento y otro.** Los escenarios se arman leyendo el catálogo en
  el momento, no de un archivo congelado: un platillo que ya no existe no se puede pedir.
- **Dos personas lanzan el mismo escenario a la vez.** Cada pedido simulado tiene identidad propia;
  el segundo no se confunde con un aviso repetido del primero.
- **Se lanza un escenario y el sistema no responde.** El simulador dice qué contestó, sin
  interpretarlo: quien prueba necesita el código y el cuerpo, no un resumen.
- **La empresa tiene varias tiendas conectadas.** Hay que poder elegir a cuál llega el pedido.

## Requirements *(mandatory)*

### Functional Requirements

**Armar pedidos que valgan la pena**

- **FR-001**: Los pedidos simulados MUST armarse con platillos del **catálogo real** de la empresa y
  su emparejamiento vigente, leídos en el momento del lanzamiento.
- **FR-002**: Los pedidos simulados MUST incluir opciones anidadas cuando el catálogo las tenga: un
  pedido sin modificadores no ejercita el camino que más cuesta.
- **FR-003**: Los precios de un pedido simulado MUST ser los que la plataforma cobraría, no los del
  catálogo, porque la dirección de la verdad es de arriba hacia abajo.
- **FR-004**: El simulador MUST negarse a armar un pedido cuando no haya nada emparejado, diciendo
  qué falta, en vez de inventar platillos.

**Firmar de verdad**

- **FR-005**: Cada aviso simulado MUST ir **firmado con la llave real** de esa conexión, para que
  ejercite la verificación y no la salte.
- **FR-006**: El simulador MUST negarse cuando la tienda no tenga llave capturada, y decir dónde se
  captura.
- **FR-007**: La llave MUST tomarse del ambiente de quien lanza, y NUNCA escribirse en un archivo
  del repositorio, que es público.

**Los escenarios**

- **FR-008**: MUST existir un escenario por cada caso que la spec 021 nombra como difícil: pedido
  simple, con opciones anidadas, con un platillo sin pareja, para recoger en tienda, cancelación que
  llega antes que su notificación, aviso repetido, y detalle que no se puede traer.
- **FR-009**: Cada escenario MUST decir **qué se espera que pase**, para que quien lo lanza sepa si
  lo que vio está bien.
- **FR-010**: Los escenarios MUST poder lanzarse de uno en uno y también todos de corrido.
- **FR-011**: Quien lanza MUST poder elegir a qué tienda conectada llega el pedido.

**Que no toque producción**

- **FR-012**: El simulador MUST **negarse a mandar** cuando el destino sea producción, **antes de
  abrir la conexión**.
- **FR-013**: El sistema MUST rechazar un aviso marcado como simulado cuando corre en producción,
  aunque venga correctamente firmado. La defensa va en los dos lados: quien dispara y quien recibe.
- **FR-014**: Un pedido que nació de un aviso simulado MUST quedar **marcado como simulado** de
  forma que sobreviva en la base y se distinga en cualquier pantalla donde se vea.
- **FR-015**: El sistema MUST registrar como evento de seguridad todo aviso simulado que llegue a un
  ambiente donde no debía.

**Dejar el ambiente limpio**

- **FR-016**: MUST existir una forma de cerrar los pedidos que el simulador creó.
- **FR-017**: La limpieza MUST tocar **solamente** lo que el simulador creó, y nunca un pedido que
  ya estaba abierto.

**Que se pueda leer lo que pasó**

- **FR-018**: El simulador MUST mostrar qué mandó y qué le contestaron, sin interpretarlo.
- **FR-019**: El simulador MUST poder correr contra el ambiente local y contra el de pruebas.

### Key Entities

- **Escenario**: un caso con nombre, lo que manda y lo que se espera que pase. Es lo que se lanza.
- **Pedido simulado**: un pedido que llegó de un aviso simulado. Igual a uno real en todo salvo en
  que está **marcado**, y esa marca es lo que permite limpiarlo y no confundirlo en un corte.
- **Destino**: contra qué ambiente se lanza. Producción no es un destino válido.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Lanzar un pedido realista cuesta **un comando**, contra los quince minutos de armar
  JSON y calcular el HMAC a mano que cuesta hoy.
- **SC-002**: Los **siete escenarios** difíciles de la spec 021 se pueden lanzar, cada uno por su
  cuenta.
- **SC-003**: **Cero** pedidos simulados pueden llegar a producción, y el intento se rechaza antes
  de mandar nada.
- **SC-004**: Un pedido simulado se distingue de uno real **sin abrir la base de datos**.
- **SC-005**: Después de limpiar, **cero** pedidos simulados quedan abiertos y **cero** pedidos
  ajenos fueron tocados.
- **SC-006**: La feature 021 se puede demostrar de punta a punta **sin intervención de la
  plataforma**, que es lo que su FR-031 prometía y hoy no se puede cumplir.

## Assumptions

- **Los escenarios se arman leyendo el catálogo, no de archivos congelados.** Un archivo con
  platillos deja de ser cierto en cuanto alguien edita el menú, y el pedido simulado dejaría de
  emparejar sin que nadie se entere.
- **El simulador es una herramienta de quien desarrolla, no una pantalla del producto.** Quien
  opera el restaurante no lo ve ni lo necesita.
- **Se apoya en lo que la 021 ya construyó**: la conexión de la tienda, su llave de firma, el
  emparejamiento y la puerta pública. Esta feature no vuelve a construir nada de eso.
- **La marca de simulado viaja en el aviso**, no se deduce: deducirla por la dirección de origen o
  por el ambiente sería adivinar, y lo que está en juego es la integridad de un corte de caja.
- **Una sola plataforma.** Solo Uber Eats, que es la única con acceso concedido.

## Fuera de alcance

- Simular DiDi o Rappi.
- Cualquier cosa que escriba hacia la plataforma.
- Una pantalla dentro del producto para lanzar escenarios.
- Simular el flujo de aceptación del lado de la plataforma: lo que se simula es lo que **llega**,
  no lo que Uber hace después con nuestra respuesta.
