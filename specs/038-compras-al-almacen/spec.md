# Feature Specification: Compras al almacén

**Feature Branch**: `038-compras-al-almacen`
**Created**: 2026-10-14
**Status**: Draft
**Input**: Issue #60 y los requerimientos de finanzas del 9-oct (compras e inventario) y del 13-oct
(inventario flexible, punto 8), fuera del repositorio. Decisión del dueño (13-oct): el requerimiento
de inventario se separó en dos. El 037 (Salud del inventario, ya en develop) muestra qué no cuadra;
este 038 corrige las existencias.

## Contexto

Lo guardado en el almacén solo baja: cada venta resta y nada suma. La pieza que suma ya existe
(un gasto con renglones de mercancía, al recibirse, deja una entrada por renglón), pero no se usa.
Medido el 14-oct sobre una copia del respaldo de producción:

- Todos los gastos están marcados como recibidos, y ninguno tiene un renglón ligado a un insumo.
- No hay un solo movimiento de entrada por compra.

El defecto no está en el almacén: está en la captura. Hoy un gasto se registra como importe y
categoría, y decir «esto fueron 3 cajas de leche de 12 litros» cuesta más de lo que vale para quien
captura. Mientras eso no sea fácil, lo guardado no puede subir.

**Criterio del dueño (13-oct): de aquí en adelante, no hacia atrás.** No se recalculan ventas ni se
reconstruyen compras pasadas. Lo heredado (existencias negativas acumuladas) se corrige con un
conteo, y desde ese conteo cada compra suma y cada venta resta.

## Clarifications

### Session 2026-10-14

- Q: Cuando se registra una compra, ¿qué pasa con el costo del insumo? → A: Por omisión toma el costo de la última compra. Cada empresa puede elegir en su configuración el promedio ponderado (lotes comprados a precios distintos), como opción avanzada para ver sus números de otra manera.
- Q (prueba con una ferretería, 14-oct): ¿agregamos una unidad de longitud? → A: Sí, en el 038: longitud (m, cm) como cuarto tipo, junto a peso, líquido y piezas.
- Q (misma prueba): «insumo» no le dice nada a quien revende, ¿qué hacemos? → A: Las pantallas de compras, empaques y conteo dicen «artículo» por omisión y cada empresa puede cambiar la palabra. Un modelo de reventa propio (precio por pieza y por caja, variantes) va en el issue #64.
- Q: Al cerrar un conteo, ¿qué pasa con los insumos que nadie contó? → A: Se quedan como estaban; solo cambia lo contado. Salud los sigue señalando y se pueden contar después.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Registrar una compra que suma al almacén (Priority: P1)

Quien recibe la mercancía registra el gasto como siempre y, en el mismo paso, dice qué insumos
llegaron y cuántos, en la presentación en que llegaron («3 cajas»). Al guardar, la existencia de
cada insumo sube en su unidad base.

**Why this priority**: Es lo único que hace subir lo guardado. Sin esto, el conteo de la Historia 3
se vuelve a desfasar en días.

**Independent Test**: Registrar un gasto con un renglón «3 cajas de Leche» (caja = 12 L) y ver que la
leche sube 36 L, que el movimiento dice de qué gasto viene, y que «Insumos sin compras registradas»
del 037 ya no la lista.

**Acceptance Scenarios**:

1. **Given** un insumo con la presentación «caja = 12 L», **When** se registra un gasto con «3 cajas» de ese insumo que ya llegó, **Then** la existencia sube 36 L con un movimiento de entrada ligado al gasto y al renglón.
2. **Given** un gasto con renglones de mercancía y renglones que no son de almacén (envío, bolsa), **When** se guarda, **Then** solo los renglones de insumo tocan el almacén; los demás suman al importe y nada más.
3. **Given** un gasto sin ningún renglón de insumo (luz, renta), **When** se guarda, **Then** se registra como hoy, sin pedir insumos.
4. **Given** un gasto ya registrado que entró al almacén, **When** se corrige la cantidad de un renglón, **Then** el almacén se ajusta por la diferencia con un movimiento que lo dice, nunca se duplica la entrada.
5. **Given** un gasto que entró al almacén, **When** se borra, **Then** sus entradas se revierten con movimientos que lo dicen; el historial no se borra.
6. **Given** una empresa con el costeo por omisión, **When** se guarda una compra de leche a $25 el litro, **Then** el costo de la leche pasa a $25 el litro.
7. **Given** una empresa que eligió promedio ponderado, con 10 L a $20 y una compra de 10 L a $30, **When** se guarda, **Then** el costo de la leche pasa a $25 el litro.

---

### User Story 2 - Presentaciones de compra por insumo (Priority: P1)

Quien configura el almacén define, para cada insumo, cómo llega: «caja = 12 L», «bolsa = 1 kg»,
«paquete = 50 piezas». Al capturar una compra se elige la presentación en vez de convertir de
cabeza.

**Why this priority**: Es lo que hace barata la captura de la Historia 1. Convertir «3 cajas» a
litros a mano es justo el paso que hoy nadie hace.

**Independent Test**: Crear la presentación «caja = 12 L» para la leche, capturar «2 cajas» y ver que
sube 24 L; cambiar la caja a 10 L y ver que una compra nueva usa 10 y la anterior conserva 12.

**Acceptance Scenarios**:

1. **Given** un insumo en litros, **When** se le agrega la presentación «caja» con 12, **Then** se ofrece al capturar compras de ese insumo, junto con la unidad base.
2. **Given** una presentación que cambia de 12 a 10, **When** se registra una compra nueva, **Then** usa 10; las compras anteriores conservan la equivalencia con la que se registraron.
3. **Given** una presentación en una unidad de otro tipo (una caja de 12 piezas para un insumo en litros), **When** se intenta guardar, **Then** se pide cuánto trae cada pieza en la unidad base o se rechaza; nunca se convierte en silencio.
4. **Given** un insumo sin presentaciones, **When** se captura una compra, **Then** se puede capturar en su unidad base y, desde ahí mismo, crear la presentación para la próxima vez.

---

### User Story 3 - Conteo inicial que reemplaza lo heredado (Priority: P1)

Quien administra cuenta lo que de verdad hay en el almacén de su sucursal y lo captura. La
existencia de cada insumo contado pasa a ser la cifra contada, con un ajuste que dice cuánto había
según el sistema y cuánto se contó.

**Why this priority**: Sin conteo, sumar compras a una existencia negativa heredada sigue dando un
número falso. Es el punto de partida de «de aquí en adelante».

**Independent Test**: Sobre la copia del respaldo, contar 5 insumos en negativo, capturar sus cifras y
ver que quedan con la cifra contada, que cada uno deja un ajuste con «había −X, se contaron Y», y que
«Lo guardado en negativo» del 037 ya no los lista.

**Acceptance Scenarios**:

1. **Given** un insumo con −200 ml según el sistema, **When** se cuentan 1,500 ml, **Then** queda en 1,500 ml con un ajuste de +1,700 que dice «conteo» y quién lo hizo.
2. **Given** un conteo a medias, **When** quien cuenta sale de la pantalla o se cae la red, **Then** lo capturado no se pierde y el conteo se puede terminar después.
3. **Given** una venta que resta un insumo mientras se cuenta, **When** se cierra el conteo, **Then** la existencia final es la cifra contada menos lo vendido después de capturar esa cifra.
4. **Given** un conteo cerrado, **When** se intenta editar, **Then** no se puede; una corrección es un conteo nuevo.
5. **Given** insumos que no se contaron, **When** se cierra el conteo, **Then** se quedan como estaban, incluso en negativo, y «Lo guardado en negativo» los sigue listando.

---

### Edge Cases

Formas de fallar, pensadas antes del diseño (constitución IV):

- **El valor vacío que significa algo:**
  - Un renglón contado en 0 es «no hay»; un renglón sin capturar es «no se contó». La pantalla no puede confundirlos.
  - Una presentación con equivalencia vacía o en 0 no se guarda.
- **El camino nuevo que se salta el control viejo:**
  - Un gasto que se edita, se borra o cambia de insumo no puede dejar entradas duplicadas ni huérfanas.
  - La lectura de un ticket (la extracción automática que ya existe) propone renglones; ninguno entra al almacén sin que alguien lo confirme.
- **El estado que no sobrevive:** un conteo a medias sobrevive a recargar la página, a que se caiga la red y a cambiar de tableta.
- **Lo que entra en otro orden:**
  - Una compra registrada antes del conteo y recibida después: el conteo manda sobre lo que había hasta capturar la cifra.
  - Un gasto con fecha anterior al último conteo: entra al almacén igual, con su fecha (ver Assumptions).
- **El hermano que no se movió:**
  - «Insumos sin compras registradas» y «Lo guardado en negativo» del 037 leen las mismas entradas: un insumo comprado deja de aparecer en la primera; una empresa con alguna compra deja de ver la segunda resumida.
  - El costo de un insumo alimenta el costo de las recetas: si la compra lo cambia, cambia el costo estimado de los productos.
- **Unidades y empaques** (medido el 14-oct en la copia de producción: la cuenta se lleva solo en g,
  ml o pieza; kg y L son para comprar y mostrar; 142 de 398 insumos activos traen una medida o un
  empaque en el nombre). Reglas aceptadas por el dueño el 14-oct:
  1. Un empaque pide número **y unidad** del mismo tipo (kg o g; L o ml) y muestra la equivalencia
     («= 25,000 g») antes de guardar: «25» a secas en un insumo en gramos sería un error de 1,000 veces
     que nadie ve.
  2. Un insumo que se cuenta en piezas se compra por pieza o por empaque; comprarlo en litros o kilos
     se rechaza diciendo cómo hacerlo.
  3. Comprar por peso algo que se cuenta en volumen (o al revés) se rechaza y se ofrece un empaque:
     no se adivina una densidad.
  4. La unidad de un insumo se elige al crearlo («¿En qué lo mides?»: peso, líquido o piezas) y no
     cambia si ya tiene existencia, recetas o compras.
  5. Dos empaques del mismo insumo se distinguen por nombre y contenido («Bulto 25 kg» y «Bulto 20
     kg» conviven).
  6. Empaque dentro de empaque no existe: una caja de 12 paquetes de 50 vasos es «Caja = 600 piezas».
  7. Lo de peso variable (0.280 kg) se compra directo en kg, sin empaque.
  8. Las cantidades aceptan hasta 4 decimales (1.5 bultos, 0.25 L).
  9. Cucharada, cucharadita y onza no se ofrecen al comprar.
  10. Lo guardado se muestra en kg o L desde 1,000 g o ml, y el costo por kg o por L.
  11. Si el ticket trae un tamaño y el proveedor ya tiene uno aprendido, gana el empaque que se
      elija; lo aprendido solo lo propone.
  12. El conteo acepta empaques más sueltos («3 cajas + 2 L») y los suma.
  13. Un empaque que ya no se usa se desactiva; las compras pasadas lo muestran con su contenido de
      entonces.
  14. Un empaque no puede llamarse como una unidad («Litro», «Kilo», «Metro»): eso es comprar suelto,
      no un empaque.
- **Varias sucursales:** una compra y un conteo son de una sucursal; con dos sucursales y sin elegir, no se adivina (como el resto del sistema).
- **Otra empresa:** ni una compra, ni una presentación, ni un conteo de otra empresa se ven ni se tocan.

## Requirements *(mandatory)*

### Functional Requirements

**Compras (Historia 1)**

- **FR-001**: Al registrar un gasto, el sistema MUST permitir agregar renglones de insumo con cantidad y presentación (o unidad base), además de los renglones que no son de almacén.
- **FR-002**: Cada renglón de insumo de un gasto que ya llegó MUST sumar a la existencia del insumo, en su unidad base, con un movimiento de entrada ligado al gasto y al renglón.
- **FR-003**: Corregir la cantidad, la presentación o el insumo de un renglón ya recibido MUST ajustar el almacén por la diferencia, con un movimiento que lo dice; borrar el gasto MUST revertir sus entradas con movimientos que lo dicen. Nunca se borra un movimiento.
- **FR-004**: Un gasto sin renglones de insumo MUST poder registrarse como hoy.
- **FR-005**: Los renglones que propone la lectura automática de un ticket MUST quedar como propuesta hasta que alguien confirme el insumo y la cantidad; ninguno suma solo.
- **FR-006**: Al guardar una compra, el costo del insumo MUST actualizarse según el método de costeo de la empresa: **última compra** por omisión, o **promedio ponderado** (lo que había por su costo más lo que llegó por el suyo, entre el total) si la empresa lo eligió.
- **FR-006a**: El método de costeo MUST ser una configuración por empresa, cambiable por quien tiene el permiso de configurar el almacén. Cambiarlo afecta el costo desde ese momento; MUST NOT reescribir el costo guardado en ventas anteriores.
- **FR-006b**: Con promedio ponderado, si lo guardado es cero o negativo, el costo MUST tomarse de la compra (no hay nada que promediar); un negativo heredado no puede producir un costo negativo ni absurdo.
- **FR-007**: Un gasto recibido antes de esta feature MUST NOT recalcularse ni reconstruirse.

**Presentaciones (Historia 2)**

- **FR-008**: Cada insumo MUST poder tener cero o más presentaciones de compra, cada una con un nombre y su equivalencia en la unidad base del insumo, editable.
- **FR-009**: Cada renglón de compra MUST guardar la equivalencia con la que se registró; cambiar la presentación después MUST NOT cambiar compras anteriores.
- **FR-010**: Una presentación MUST capturarse con su contenido **y su unidad** del mismo tipo que el insumo (kg o g, L o ml, piezas) y mostrar la equivalencia antes de guardar; una compra en una unidad de otro tipo MUST rechazarse diciendo cómo capturarla. Nunca se convierte en silencio (reglas 1–3 de *Unidades y empaques*).
- **FR-010a**: Al crear un insumo desde la compra MUST preguntarse en qué se mide (peso, líquido, longitud o piezas); la unidad de un insumo con existencia, recetas o compras MUST NOT cambiarse.
- **FR-010b**: El sistema MUST ofrecer longitud (m, cm) como cuarto tipo de unidad, con las mismas reglas de conversión que peso y líquido.
- **FR-011**: Desde la captura de una compra MUST poder crearse la presentación que falta, sin salir de la pantalla.

**Conteo (Historia 3)**

- **FR-012**: Quien tiene el permiso MUST poder abrir un conteo de la sucursal, capturar la cifra contada de cada insumo y cerrarlo.
- **FR-013**: Al cerrar, cada insumo contado MUST quedar con la cifra contada, con un ajuste que guarda lo que había según el sistema, lo contado, quién y cuándo.
- **FR-014**: Un conteo a medias MUST conservarse en el servidor hasta cerrarse o descartarse.
- **FR-015**: Un conteo cerrado MUST NOT editarse; una corrección es un conteo nuevo.
- **FR-016**: Al cerrar, los insumos no contados MUST quedarse como estaban; el conteo MUST NOT afirmar una cifra que nadie capturó. Al cerrar, la pantalla dice cuántos insumos activos quedaron sin contar.

**Permisos y producto**

- **FR-017**: Registrar compras con insumos, configurar presentaciones y contar MUST pedir permisos verificados en el servidor (no nombres de rol; constitución, «Roles: modelo híbrido»), con administrador y gerente por omisión.
- **FR-018**: Ningún nombre, marca, producto o proveedor de un negocio en particular MUST aparecer en el código ni en las migraciones de esquema. Ninguna pantalla trae listas fijas de un giro: categorías y proveedores salen de los de la empresa (y se crean desde la captura); los nombres de empaque que se sugieren son los que la empresa ya usa.
- **FR-018a**: La palabra con que las pantallas de compras, empaques y conteo llaman a lo que se guarda en el almacén MUST ser configurable por empresa, con «artículo» por omisión.
- **FR-019**: Los textos de la pantalla MUST usar palabras de quien opera, validadas con usuarios sin contexto el 14-oct: «lo que tienes guardado» o «tienes», «llegó», «contaste», «restar» y «sumar», y «empaque» (no «presentación», que se leyó como exposición); no «existencia», «movimiento» ni «unidad base».

### Key Entities

- **Renglón de compra**: un insumo, la cantidad y la presentación con que llegó, y la equivalencia copiada al registrarse. Pertenece a un gasto.
- **Presentación de compra**: cómo llega un insumo («caja = 12 L»). Pertenece al insumo y a la empresa.
- **Conteo**: lo que se contó en una sucursal, abierto o cerrado, con su cifra por insumo y lo que el sistema tenía al capturarla.
- **Entrada y ajuste de almacén**: lo que suma una compra o corrige un conteo; dice de dónde viene.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Quien recibe la mercancía registra una compra de 5 insumos con presentación en menos de 2 minutos, y cada insumo sube lo que llegó.
- **SC-002**: Después de un conteo y una semana de compras registradas, «Lo guardado en negativo» del 037 no lista insumos contados (salvo los que de verdad se acabaron y se siguieron vendiendo).
- **SC-003**: Cada cambio de existencia por compra o conteo se puede rastrear hasta su gasto o su conteo, con quién y cuándo.
- **SC-004**: Corregir o borrar un gasto nunca deja el almacén con una entrada duplicada ni huérfana.
- **SC-005**: Un conteo a medias sobrevive a recargar la página y a cambiar de tableta sin perder una sola cifra.
- **SC-006**: Ninguna referencia a marcas, productos ni proveedores de un negocio aparece en el código ni en las migraciones de esta feature.

## Assumptions

- **Fuera de alcance:** multi-almacén y traspasos, mermas y cortesías, conteos parciales periódicos, kardex y valor del inventario, pago a proveedores, órdenes de compra.
- **Un almacén por sucursal**, que es lo que existe hoy.
- **El paso «recibir» separado del gasto se conserva** para el pedido que se paga antes de llegar; el caso normal es «ya llegó» y entra al guardar.
- **La venta durante el conteo:** la cifra contada se toma como la existencia en el momento de capturarla; lo que se venda después de capturar ese renglón sigue restando sobre la cifra contada.
- **Un gasto con fecha anterior al último conteo** entra al almacén igual: el conteo no sabe qué compras llegaron antes de contar. Es un riesgo aceptado; quien registra tarde ve un aviso.
- **El costo de las ventas ya registradas no cambia** con ningún método: cada renglón de venta guarda su costo del momento (constitución VIII, snapshot).
- **Los formatos aprendidos por proveedor** que ya existen siguen funcionando; las presentaciones por insumo son el camino principal.
- **Permisos sugeridos:** `inventory.receive` (compras con insumos), `inventory.configure` (presentaciones) e `inventory.count` (conteo); nombres finales en el plan.
