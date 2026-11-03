# Feature Specification: Ubicación, autoguardado de la ficha y empaques claros

**Feature Branch**: `043-ubicacion-y-autoguardado`

**Created**: 2026-10-20

**Status**: Decisiones del dueño tomadas (2026-10-20); plan y tareas listos, sin implementar

**Input**: Seguimiento de la spec 042 (gestión de artículos del almacén), con las decisiones del dueño
tras probar la 042 en tableta el 2026-10-20. Cierra #119, depende de #120 y cubre la palabra
«Lugar» y «Se cuenta en g» de #103. Fuera de alcance: #121 y #122.

## Contexto

Verificado en el ambiente de pruebas el 2026-10-20, con la 042 desplegada:

- La ficha del artículo guarda con un botón «Guardar» que aparece solo con cambios. Elegir el
  lugar de un artículo cuesta 4 toques (abrir la ficha, tocar «Lugar», elegir, «Guardar»).
- Al volver a la lista después de guardar, a veces aparece «¿Descartar cambios?» aunque sí se
  guardó (#119). El «Mínimo» cambia de valor al guardar, y vuelve a cambiar si se guarda otra vez
  (#120).
- El campo y la columna dicen «Lugar», y su vacío dice «Sin lugar». La palabra no se entendió en las
  pruebas con personas nuevas (#103).
- La hoja de empaque nuevo ofrece solo los nombres que la empresa ya usa en artículos parecidos y
  un «Otro…»; una empresa nueva ve solo «Otro…». «Se cuenta en g» queda entre el título y los
  controles y no se entendió (#103).
- Una existencia negativa en la ficha se ve igual que una positiva.
- La prueba de pantalla de lugares para contar (spec 040) falla desde la 042, y la prueba de
  pantalla que la 042 prometía (`gestion-de-articulos.spec.ts`) no existe.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Darle ubicación a un artículo (Priority: P1)

Quien administra el almacén abre la ficha de un artículo y en «Ubicación» elige una de las que ya
existen o escribe una nueva y la crea ahí mismo, como hoy se hace con la categoría.

**Why this priority**: es lo que se hace en serie antes de un conteo, y hoy cuesta 4 toques por
artículo y una palabra que no se entiende.

**Independent Test**: en un artículo sin ubicación, abrir la ficha, tocar «Ubicación», escribir
«Bodega» y tocar «Crear "Bodega"»: el artículo queda con «Bodega» sin tocar otro botón.

**Acceptance Scenarios**:

1. **Given** la lista de artículos, **Then** la columna se llama «Ubicación» y un artículo sin ella
   dice «Sin definir». El renglón no lleva un botón de ubicación propio.
2. **Given** la ficha de un artículo y ninguna ubicación creada en la empresa, **When** se abre el
   campo «Ubicación», **Then** el campo dice «Sin definir» y el buscador muestra «Ej. Refri, Bodega,
   Barra».
3. **Given** el campo abierto, **When** se escribe «bod», **Then** quedan las ubicaciones que
   contienen «bod», sin distinguir mayúsculas ni acentos.
4. **Given** que no existe «Bodega», **When** se escribe «Bodega», **Then** se ofrece
   «Crear "Bodega"», y al tocarlo se crea y queda elegida y guardada.
5. **Given** que existe «Bodega», **When** se escribe « BODEGA » o «bódega», **Then** no se ofrece
   crear otra; se ofrece la que existe.
6. **Given** quien puede asignar ubicaciones pero no configurar el almacén, **When** escribe una que
   no existe, **Then** no se ofrece crear y se lee «Pídele a un administrador que la cree».
7. **Given** una ficha con ubicación elegida, **When** se elige otra, **Then** queda guardada en 3
   toques desde la lista (abrir, tocar el campo, elegir).

---

### User Story 2 - La ficha guarda sola (Priority: P1)

Cada cambio en la ficha se guarda solo: las listas e interruptores al elegir; el texto y los números
tras 2 segundos sin escribir y también al salir del campo. No hay botón «Guardar», y al salir solo se pregunta si
algo quedó sin guardar.

**Why this priority**: el botón aparecía y desaparecía, el aviso de salir mentía (#119) y cada
cambio costaba un toque más.

**Independent Test**: cambiar el nombre de un artículo, esperar 2 segundos sin tocar nada y volver
a la lista: el nombre nuevo está en la lista y no hubo pregunta al salir.

**Acceptance Scenarios**:

1. **Given** la ficha, **When** se elige una categoría o una ubicación, **Then** se guarda en ese
   momento.
2. **Given** la ficha, **When** se cambia el nombre o el mínimo y pasan 2 segundos sin escribir, o se
   sale del campo, **Then** se guarda. Enter también guarda, como atajo; nunca es necesario.
3. **Given** un campo con cambios que todavía no se mandan, **Then** junto al título se ve un punto
   discreto; mientras guarda, «Guardando…» con un indicador de espera; al terminar, «Guardado ✓».
4. **Given** que el guardado falla por la red, **Then** se ve «No se pudo guardar · Reintentar» y el
   campo conserva lo escrito, marcado; «Reintentar» manda lo que se ve. Si el servidor lo rechaza
   (otra persona lo cambió o el dato no vale), el campo vuelve a lo guardado y se dice por qué.
5. **Given** dos cambios seguidos en campos distintos, **Then** se guardan uno después del otro,
   en el orden en que se hicieron, y el segundo no pisa al primero.
6. **Given** un guardado en curso, **Then** ningún campo se deshabilita (en la tableta cerraría el
   teclado): se puede seguir escribiendo en ese y en los demás, y la cola manda lo nuevo después. En
   una lista o un interruptor que está guardando, un segundo toque se ignora hasta que termine.
7. **Given** un guardado en curso, **When** se toca «‹ Artículos», **Then** espera a que termine y
   después regresa a la lista, sin preguntar.
8. **Given** algo sin guardar (un guardado fallido o un dato inválido), **When** se sale de la ficha,
   **Then** aparece «¿Descartar cambios?»; con todo guardado, nunca aparece (#119).
9. **Given** un dato inválido (nombre vacío, mínimo negativo), **Then** el campo se marca, se dice
   qué está mal y no se manda.
10. **Given** el mismo mínimo guardado dos veces seguidas, **Then** queda el mismo valor (#120).
11. **Given** «Corregir costo», «Cambiar» la unidad y «Archivar artículo», **Then** conservan su hoja
    y su confirmación; su botón de confirmar muestra el indicador de espera mientras guarda.
12. **Given** «Nuevo artículo», **Then** conserva su botón «Guardar»: nada se guarda hasta tocarlo.

---

### User Story 3 - Elegir el empaque sin adivinar (Priority: P2)

Al agregar un empaque, la hoja ofrece botones con los empaques más comunes para el tipo del
artículo, además de los que la empresa ya usa, y una frase que dice cuánto trae.

**Why this priority**: una empresa nueva solo veía «Otro…»; «Se cuenta en g» no se entendió.

**Independent Test**: en un artículo en peso de una empresa sin empaques, «+ Agregar empaque» ofrece
Bolsa, Caja, Costal, Paquete y «Otro»; al elegir Bolsa y escribir 5 kg se lee «1 bolsa = 5 kg».

**Acceptance Scenarios**:

1. **Given** un artículo en peso, **Then** los botones base son Bolsa, Caja, Costal y Paquete.
2. **Given** un artículo en volumen, **Then** son Botella, Galón, Garrafa y Caja.
3. **Given** un artículo en piezas, **Then** son Caja, Paquete y Bolsa.
4. **Given** una empresa que ya usa otros empaques en artículos del mismo tipo, **Then** se ofrecen
   también, sin repetir uno que ya está entre los base.
5. **Given** la hoja abierta, **Then** ningún botón está elegido; «Otro» se llama «Otro» y al tocarlo
   abre el campo con «Ej. Cubeta, Garrafa».
6. **Given** la hoja, **Then** la pregunta dice «¿En qué empaque lo compras?» y arriba solo queda una
   línea corta de ayuda.
7. **Given** «Bolsa» y 5 kg, **Then** debajo se lee «1 bolsa = 5 kg», y la frase cambia al cambiar el
   nombre, la cantidad o la unidad. «Se cuenta en g» deja de aparecer suelto.
8. **Given** la hoja, **Then** ya no tiene la fila «Ubicación»: la ubicación se elige en la ficha.
9. **Given** un empaque de la lista, **Then** «Quitar este empaque» va aparte de «Editar» y con estilo
   secundario; quitarlo no lo borra (decisión del dueño, 2026-10-20).
10. **Given** la hoja, **Then** el selector de unidad de «Trae» se ve y mide igual que el de «Mínimo»
   en la ficha.

---

### User Story 4 - Ver que una existencia negativa hay que revisarla (Priority: P2)

**Why this priority**: «Hay -1.87 kg» se leía como un dato normal.

**Independent Test**: abrir la ficha de un artículo con existencia negativa: la cifra está en rojo
y dice «Revisar con un conteo».

**Acceptance Scenarios**:

1. **Given** un artículo con existencia menor que cero, **Then** la ficha la muestra en rojo con
   «Revisar con un conteo».
2. **Given** un artículo con existencia cero o positiva, **Then** no hay aviso.
3. **Given** el aviso, **Then** no se recalcula ni se corrige nada del pasado.

---

### User Story 5 - Pruebas de pantalla que sí existen (Priority: P1)

**Why this priority**: la 042 marcó como hecha una prueba de pantalla que nunca se escribió, y la
prueba de la spec 040 quedó rota sin que nadie la corriera antes de mergear.

**Independent Test**: la suite de pantalla corre completa contra el ambiente de pruebas y pasa.

**Acceptance Scenarios**:

1. **Given** la lista de artículos a 1024×600, **Then** se ven 6 renglones completos y todo control
   tocable mide 44 px o más.
2. **Given** la ficha a 1024×600 de lado y de pie, **Then** no hay desplazamiento de lado y todo
   control tocable mide 44 px o más.
3. **Given** la prueba de ubicaciones de la spec 040, **Then** mide el flujo nuevo de la ficha
   (3 toques y sin abrir el teclado al tocar el campo) y pasa.

### Edge Cases

- **Dos ubicaciones que solo difieren en acentos, mayúsculas o espacios**: no se crea la segunda; se
  elige la que existe.
- **Nombre de ubicación demasiado largo**: no se puede escribir más allá del tope.
- **Salir de un campo sin cambiar nada**: no se manda nada ni aparece «Guardando…».
- **Una pausa a media cifra** («1» … «5»): puede guardar 1 y luego 15; la bitácora lo muestra como un
  solo cambio al valor final.
- **Borrar el campo para reescribirlo**: vacío no se manda por la pausa; si se sale del campo vacío,
  el nombre se marca como inválido y el mínimo queda «sin mínimo».
- **Otra persona cambió la ficha mientras se veía**: el guardado se rechaza, se avisa y la ficha se
  recarga con lo que hay; no se pisa.
- **Se cae la red a media captura**: «No se pudo guardar · Reintentar» y el campo conserva lo escrito,
  marcado; nada se queda a medias en el servidor. Salir en ese estado pregunta «¿Descartar cambios?».
- **Recargar la página con un guardado en curso**: lo que no terminó de guardarse se pierde y la
  ficha muestra lo que hay en el servidor.
- **Un campo inválido y otro válido**: el válido se guarda; el inválido se queda marcado.
- **Una ubicación archivada**: no se ofrece; un artículo que ya la tenía se ve «Sin definir»
  (regla de la spec 040).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: La interfaz MUST llamar «Ubicación» al campo de la ficha, a la columna y al filtro de
  la lista de artículos, y «Sin definir» al valor vacío.
- **FR-002**: El campo MUST decir «Sin definir» cuando el artículo no tiene ubicación. Sin ninguna
  ubicación creada en la empresa, el ejemplo «Ej. Refri, Bodega, Barra» MUST ir dentro del buscador,
  nunca en el campo (decisión del dueño, 2026-10-20: en el campo parecía un valor ya puesto).
- **FR-003**: El campo MUST permitir escribir para buscar entre las ubicaciones, sin distinguir
  mayúsculas ni acentos, y ofrecer «Crear "X"» cuando lo escrito no existe, igual que la categoría.
  Lo creado queda elegido y guardado.
- **FR-004**: Dos ubicaciones que solo difieren en mayúsculas, acentos o espacios MUST ser la misma.
  El nombre tiene un tope de largo.
- **FR-005**: Crear una ubicación MUST pedir `inventory.configure` en el servidor. Quien puede
  asignar ubicaciones sin ese permiso solo elige entre las existentes, y el campo dice «Pídele a un
  administrador que la cree».
- **FR-006**: La lista de artículos MUST NOT llevar un botón de ubicación en el renglón.
- **FR-007**: La ficha MUST guardar cada cambio sola: las listas al elegir, el texto y los números al
  dejar de escribir 2 segundos y también al salir del campo; Enter es un atajo, nunca un requisito.
  MUST NOT tener botón «Guardar». Al salir MUST preguntar «¿Descartar
  cambios?» solo si algo quedó sin guardar (un guardado fallido o un dato inválido).
- **FR-008**: Los guardados de la ficha MUST ir en cola, uno a la vez y en el orden en que se
  hicieron. Ningún campo se deshabilita mientras guarda; lo que se escriba después entra a la cola. En
  listas e interruptores, un segundo toque mientras guardan se ignora.
- **FR-009**: Junto al título MUST haber un solo indicador: un punto discreto mientras hay cambios sin
  mandar, «Guardando…» con indicador de espera,
  «Guardado ✓» o «No se pudo guardar · Reintentar». Si falla por la red, el campo MUST conservar lo
  escrito, marcado, y «Reintentar» manda lo que se ve; si el servidor lo rechaza, MUST volver a lo
  guardado y decir por qué.
- **FR-010**: «‹ Artículos» MUST esperar a que termine un guardado en curso antes de regresar.
- **FR-011**: Un dato inválido MUST marcarse en su campo y no mandarse.
- **FR-012**: Guardar el mismo valor dos veces MUST dejar el mismo valor (#120). El autoguardado no
  se activa hasta que #120 esté arreglado con su prueba.
- **FR-013**: «Corregir costo», el cambio de unidad y archivar MUST conservar su hoja y su
  confirmación, con indicador de espera en el botón que confirma. «Nuevo artículo» MUST conservar su
  «Guardar».
- **FR-014**: La hoja de empaque nuevo MUST ofrecer siempre botones base según el tipo del artículo
  (peso: Bolsa, Caja, Costal, Paquete; volumen: Botella, Galón, Garrafa, Caja; piezas: Caja,
  Paquete, Bolsa), más los que la empresa ya usa en artículos del mismo tipo, sin repetir. Los base
  son fijos y genéricos: nada propio de un negocio.
- **FR-015**: Ningún botón de empaque MUST venir elegido. «Otro» se llama «Otro» y abre el campo con
  «Ej. Cubeta, Garrafa».
- **FR-016**: La hoja MUST preguntar «¿En qué empaque lo compras?», dejar arriba una sola línea corta
  de ayuda y mostrar debajo una frase que se actualiza con lo capturado («1 bolsa = 5 kg»). «Se
  cuenta en …» deja de aparecer suelto.
- **FR-017**: El selector de unidad de «Trae» MUST verse y medir igual que el de «Mínimo».
- **FR-018**: Una existencia menor que cero MUST verse en rojo en la ficha con «Revisar con un
  conteo». Nada del pasado se recalcula.
- **FR-019**: Todo control tocable MUST medir 44 px o más, y ningún selector es nativo.
- **FR-020**: Cada guardado de la ficha MUST dejar su evento en la bitácora como hoy
  (`ingredient.updated`), uno por guardado. La bitácora del artículo MUST mostrar como una sola
  entrada los guardados seguidos del mismo campo, por el mismo usuario, separados por menos de un
  minuto: con el valor de antes del primero y el valor final del último. Los eventos no se borran ni
  se reescriben; solo se juntan al mostrarlos.
- **FR-021**: La hoja de empaques MUST NOT tener la fila de ubicación; se elige en la ficha.
- **FR-022**: Lo que se está escribiendo MUST NOT guardarse a medias cuando todavía no es un valor:
  un número que termina en punto o en signo, o un campo vacío, no se manda por la pausa de 2
  segundos; se valida y se manda (o se marca) al salir del campo.
- **FR-023**: «Quitar este empaque» MUST ir aparte de «Editar», con estilo secundario, y archivar el
  empaque sin borrarlo.

### Key Entities

- **Ubicación** (ya existe como «lugar para contar», spec 040): nombre único por empresa, orden en el
  recorrido del conteo, activa o archivada. Cambia cómo se llama en pantalla y quién puede crearla.
- **Empaque base** (nuevo, fijo en el código): nombre genérico por tipo de unidad. No se guarda en la
  base.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Darle ubicación a un artículo desde la lista toma 3 toques si ya existe, y 3 toques más
  lo que se escribe si hay que crearla.
- **SC-002**: Cero preguntas «¿Descartar cambios?» al salir de la ficha con todo guardado.
- **SC-003**: Guardar el mismo mínimo dos veces deja el mismo valor, en cada tipo de unidad.
- **SC-004**: Una empresa sin empaques ve al menos 3 botones base al agregar el primero.
- **SC-005**: La suite de pantalla corre completa contra el ambiente de pruebas sin fallas atribuibles
  a esta feature, con la prueba de la 040 y la de la 042 incluidas.
- **SC-006**: Una persona nueva le da ubicación a un artículo y agrega un empaque sin ayuda, en la
  prueba con usuario sin contexto.

## Assumptions

- **Quién asigna y quién crea** (lectura de la decisión del dueño): asignar una ubicación sigue
  pidiendo lo que pide hoy (`inventory.configure` en la ficha; `inventory.count` desde el conteo).
  Crear una pasa a pedir `inventory.configure` en todos los caminos, también desde el conteo, donde
  hoy basta `inventory.count`. Con solo `inventory.view` la ficha es de lectura y no asigna nada.
- **#120 primero**: se investiga y se arregla con su prueba de regresión antes de que el
  autoguardado se active; si su causa es del servidor, el arreglo vale también para «Nuevo
  artículo».
- **Bitácora**: el autoguardado manda un cambio por guardado; la bitácora deja un evento por cada uno,
  con solo lo que cambió, y la pantalla los junta (FR-020). La bitácora no admite modificar ni borrar
  registros (0089), así que juntarlos al guardar no es posible sin cambiar esa regla.
- **Número a medias**: escribir «15» con una pausa entre el 1 y el 5 puede guardar 1 y después 15.
  Se acepta: el valor final es el correcto y la bitácora los muestra juntos. Lo que no se acepta es
  guardar algo que no es un número todavía («1.», «-», vacío), y eso lo impide FR-022.
- **Ubicación es lo mismo que el «lugar para contar» de la spec 040**: solo cambia la palabra en las
  pantallas del almacén. No es un almacén (#65).
- **Fuera de alcance**: catálogo de empaques editable (#121), mover varios artículos a una ubicación
  (#122) y el resto de #103 («Se compra hecho ›» y «Cambiar el orden ›»).
