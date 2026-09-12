# Feature Specification: La consola de plataforma, separada del negocio

**Feature Branch**: `016-consola-de-plataforma`

**Created**: 2026-09-11

**Status**: Draft

**Input**: Ver *Origen* al final.

## Contexto

El sistema tiene hoy **un solo tipo de usuario**: gente que trabaja en un negocio —cajero, gerente,
administrador—, siempre dentro de su empresa y aislada de las demás por RLS. Eso funciona mientras
el único que mira el sistema es quien lo opera.

Pero hay un segundo papel que hasta ahora no existía en el modelo: **quien vende y mantiene el
producto**. Ese necesita cosas que ningún usuario de negocio debe tener: ver qué empresas existen,
si están al día, y —más adelante— cómo usan el sistema y en qué vigencia están. Y necesita hacerlo
**cruzando empresas**, que es exactamente lo que RLS existe para impedir.

Meter eso como una bandera más en la tabla de usuarios del producto sería poner un bypass del
aislamiento al lado del login de cada cliente. Esta feature crea, en cambio, una **superficie
aparte**: otra identidad, otro login, otra aplicación y otro subdominio, sin puntos de contacto con
la del negocio.

**Lo que esta feature NO trae a propósito**: ninguna acción que modifique datos de un cliente. La
primera versión **solo lee**. Es deliberado: sirve para probar el camino completo —identidad,
despliegue y lectura cruzada— sin que un error pueda tocar el negocio de nadie.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Entrar a la consola, y solo a la consola (Priority: P1)

El operador de plataforma abre el subdominio de administración, entra con su credencial y ve las
pantallas que le tocan. Su credencial **no existe** para el sistema del negocio.

**Why this priority**: Es la base. Sin esta separación, todo lo demás es una bandera en un `if`.

**Independent Test**: Con una credencial de plataforma y una de negocio, cada una entra en su
superficie y es rechazada en la otra.

**Acceptance Scenarios**:

1. **Given** una credencial de plataforma, **When** se intenta entrar al POS, **Then** se rechaza
   igual que una credencial inexistente, sin decir que existe en otro lado.
2. **Given** una credencial de negocio, **When** se intenta entrar a la consola, **Then** se
   rechaza, y da lo mismo que sea administrador de su empresa.
3. **Given** una sesión de plataforma, **When** se piden datos de operación de una empresa
   (pedidos, ventas, cortes), **Then** se rechaza.
4. **Given** una sesión de negocio, **When** se piden datos de la consola, **Then** se rechaza.

---

### User Story 2 - Ver qué empresas hay y si están sanas (Priority: P1)

El operador de plataforma ve la lista de empresas: cuáles son y desde cuándo existen, más la versión
de esquema **de la instalación**.

**Lo que esta primera versión NO muestra, y por qué.** «Última actividad» exigiría leer una tabla de
operación, que es exactamente lo que FR-005 prohíbe; llega con la [spec 017](../017-mapa-de-calor-de-uso/spec.md),
que escribe ese dato del lado de plataforma. Y «esquema por empresa» no existe: hoy hay **una sola
base con un solo número de versión**, así que una columna por cliente diría lo mismo en todos los
renglones — una pantalla que aparenta informar.

**Why this priority**: Es la razón de entrar. Y es **solo lectura**, que es lo que hace seguro
probar el camino completo.

**Independent Test**: Con dos empresas en la base, la consola las lista a las dos con su estado, y
la aplicación del negocio sigue sin poder ver la otra.

**Acceptance Scenarios**:

1. **Given** varias empresas, **When** el operador abre la consola, **Then** las ve todas con su
   nombre, desde cuándo existen y su última actividad.
2. **Given** la instalación, **When** se abre la consola, **Then** se ve **una vez** en qué versión
   de esquema está — no como columna por empresa.
3. **Given** la lista, **When** se busca cualquier cifra de dinero, **Then** no hay ninguna: la
   consola no reporta las ventas de los clientes.

---

### User Story 3 - Que el permiso cruzado no alcance al dinero (Priority: P1)

El permiso de leer a través de empresas está acotado a los datos de plataforma. Aunque alguien
entrara a la consola, **no puede llegar a los pedidos, las ventas ni los cortes** de un cliente.

**Why this priority**: Es la mitigación que vuelve aceptable la decisión de cruzar tenants. Sin
ella, la consola es la llave maestra de todos los negocios.

**Independent Test**: Desde la sesión de plataforma, cualquier intento de leer datos de operación de
una empresa se rechaza, comprobado contra la base y no solo contra la aplicación.

**Acceptance Scenarios**:

1. **Given** una sesión de plataforma, **When** se consulta cualquier tabla de operación,
   **Then** se rechaza, incluso construyendo la petición a mano.
2. **Given** un defecto que dejara pasar una petición indebida, **When** llega a la base,
   **Then** la base la rechaza igual — la barrera no vive solo en el código.

---

### User Story 4 - Que la tableta no cargue nada de esto (Priority: P1)

La aplicación del negocio no incluye una sola línea de la consola. Quien usa el POS en una tableta
descarga lo mismo que antes.

**Why this priority**: Lo pidió el dueño explícitamente, y es además una regla de seguridad: código
que no viaja, no se puede inspeccionar ni alcanzar desde la tableta de un cliente.

**Independent Test**: El paquete que sirve el POS no crece, y no contiene ninguna cadena ni ruta de
la consola.

**Acceptance Scenarios**:

1. **Given** el paquete del POS, **When** se busca cualquier rastro de la consola, **Then** no hay
   ninguno.
2. **Given** el POS desplegado, **When** se navega a una ruta de la consola, **Then** no existe.

---

### Edge Cases

- **La primera vez, sin ningún operador de plataforma creado.** Tiene que haber una forma de crear
  el primero que no sea una pantalla pública ni una contraseña en el código.
- **Un operador de plataforma que se va.** Se desactiva, y su sesión deja de servir de inmediato.
- **Una empresa recién creada, sin actividad.** Aparece en la lista y se distingue de una que dejó
  de usarse.
- **El subdominio de la consola apuntando al ambiente equivocado.** Dev y producción son
  superficies distintas y no se deben poder confundir desde la pantalla.
- **Alguien intenta usar el token de la consola contra la API del negocio, y al revés.** Es el
  escenario que US1 y US3 tienen que cerrar con prueba, no con confianza.
- **Cero empresas.** La consola lo dice en vez de pintar una tabla vacía.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El operador de plataforma MUST vivir separado de los usuarios de negocio, de forma que
  el inicio de sesión del negocio no pueda encontrarlo **ni existiendo el mismo nombre**.
- **FR-002**: El operador de plataforma MUST NOT pertenecer a ninguna empresa.
- **FR-003**: El inicio de sesión del negocio MUST rechazar una credencial de plataforma con la
  misma respuesta y la misma latencia que una credencial inexistente.

  La latencia importa: una respuesta más rápida o más lenta delata que la cuenta existe, que es la
  fuga por temporización que el principio V ya cierra en el resto del login.

- **FR-004**: El inicio de sesión de la consola MUST rechazar cualquier credencial de negocio.
- **FR-005**: Una sesión de plataforma MUST NOT poder leer datos de operación de ninguna empresa
  (pedidos, líneas, pagos, ventas, cortes, gastos, inventario, clientes).
- **FR-006**: La restricción de FR-005 MUST estar impuesta **también en la base de datos**, no solo
  en la aplicación.

  Es la lección que el propio repo ya aprendió: los chequeos que viven solo en el código se saltan
  por el camino nuevo que nadie revisó.

- **FR-007**: La consola MUST poder listar todas las empresas con su nombre y su antigüedad, y MUST
  mostrar la versión de esquema **de la instalación** una sola vez.

  *Corregido el 2026-09-11, tras la revisión de arquitectura.* La versión original pedía «el estado
  de su esquema y su última actividad» por empresa. Ninguna de las dos se puede cumplir hoy: el
  esquema es uno solo para toda la base —verificado, las dos empresas marcan la misma versión— y la
  última actividad exigiría leer tablas de operación que FR-005 prohíbe. Un requisito que no se
  puede satisfacer no se implementa «como se pueda»: se corrige.

- **FR-007b**: La «última actividad» por empresa MUST quedar fuera de esta feature y llegar con la
  spec 017, que ya va a escribir ese dato del lado de plataforma.
- **FR-008**: La consola MUST NOT mostrar cifras de dinero de ningún cliente.
- **FR-009**: La primera versión de la consola MUST ser de **solo lectura**: ninguna acción que
  modifique datos de una empresa.
- **FR-010**: La consola MUST servirse desde un subdominio propio, con uno para el ambiente de
  pruebas y otro para producción.
- **FR-011**: El paquete que descarga la aplicación del negocio MUST NOT contener código de la
  consola.
- **FR-012**: Todo inicio de sesión en la consola —exitoso o fallido— MUST quedar registrado como
  evento de seguridad, sin credenciales.
- **FR-013**: El acceso a la consola MUST poder retirarse desactivando al operador, y esa
  desactivación MUST surtir efecto sin esperar a que caduque su sesión.
- **FR-014**: MUST existir una forma de crear al primer operador de plataforma sin exponer una
  pantalla pública y sin credenciales en el código.
- **FR-015**: Con cero empresas, la consola MUST decirlo en vez de pintar una tabla vacía.
- **FR-016**: La consola MUST NOT registrar ni mostrar datos personales de los empleados de un
  cliente.

### Key Entities

- **Operador de plataforma**: quien vende y mantiene el producto. No pertenece a una empresa, vive
  separado de los usuarios de negocio y se puede desactivar.
- **Estado de una empresa**: lo que la consola lee de cada cliente para saber si está sana — no lo
  que el cliente vende.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Una credencial de plataforma **no sirve** en la aplicación del negocio, y una de
  negocio **no sirve** en la consola. Comprobado en las dos direcciones.
- **SC-002**: Desde una sesión de plataforma **no existe forma** de leer un pedido, una venta ni un
  corte de ninguna empresa, ni construyendo la petición a mano.
- **SC-003**: El paquete que descarga la tableta **no crece** respecto de hoy.
- **SC-004**: El operador de plataforma ve **todas** las empresas de la instalación en una sola
  pantalla, sin que ninguna quede fuera.
- **SC-005**: Desactivar a un operador le corta el acceso **sin esperar** a que caduque su sesión.
- **SC-006**: La consola no muestra **ninguna** cifra de dinero de ningún cliente.

## Assumptions

- **Hay un solo operador de plataforma al principio** —el dueño del producto—, pero el modelo admite
  varios sin rehacerse. Y acotar a un operador a un subconjunto de empresas —un encargado de cuenta
  por cliente— es una puerta **distinta** de esa, también abierta al mismo costo: hoy el único
  operador ve todo, así que una tabla puente se agrega después sin tocar datos.
- **La consola se usa desde una computadora, no desde una tableta.** El dueño lo dijo explícitamente:
  el presupuesto de 1024×600 no aplica aquí, y por eso no se diseña contra él.
- **Comparte repositorio con el resto del producto**, para que CI, tipos y despliegue sean los
  mismos, pero **se construye y se despliega por separado**.
- **Se construye con tecnología propia, no con dependencias nuevas.** No es por licencias: es que
  cada dependencia trae su calendario de versiones, sus conflictos y su superficie de CVE, y en este
  repo un CVE bloquea el merge. Ya es principio de la constitución —*«Sin dependencia nueva para lo
  que hace la stdlib»*— y el producto lo vive: el limitador de tasa, el redondeo y el mapeo de
  errores son código propio de unas decenas de líneas.

## Out of Scope

- **Cualquier acción que modifique datos de un cliente** —resetear contraseñas, desactivar usuarios,
  tocar catálogos—. Va en la spec 018, aparte y con su propia revisión adversarial: tomar control de
  la cuenta de otro negocio no es leer de más, es suplantar a un dueño.
- **El mapa de calor de uso**: es la [spec 017](../017-mapa-de-calor-de-uso/spec.md), y se monta
  dentro de esta consola.
- **Suscripciones, vigencias y cobranza.** Después, con la consola ya probada.
- **Microfrontends.** Se evaluaron y se descartaron: resuelven que varios equipos desplieguen sin
  coordinarse, y aquí hay una persona. Traen peso en tiempo de ejecución y superficie de supply
  chain a cambio de nada. Una aplicación aparte con su propio build da la separación que se buscaba.

## Origen

Pedido por el dueño el 2026-09-11, al revisar quién debería ver el mapa de calor: *«solo yo como
super admin… no quiero que se vaya a cruzar con los otros usuarios que son específicamente de
negocios»*, y después *«vamos por el primer camino con un rol dedicado que asumo que tendrá un
subdominio reservado de administración»*.

Decisiones tomadas por él antes de escribir este spec:

| Pregunta | Respuesta |
|---|---|
| ¿Dónde vive la identidad? | **Sin empresa, en tabla aparte** |
| ¿Qué pesa? | Que **la tableta no cargue** la consola. En su PC el peso da igual |
| ¿Microfrontends? | Descartado tras evaluarlos |

Y una reserva que quedó dicha y que él decidió aceptar priorizando velocidad: un acceso que cruza
empresas es un bypass del aislamiento que sostiene todo el producto. La mitigación acordada, y que
por eso es FR-005 y FR-006, es que ese permiso **no alcance al dinero ni a la operación** de ningún
cliente.
