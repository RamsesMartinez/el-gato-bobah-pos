# Feature Specification: Leer el menú de las plataformas y decir en qué difiere del nuestro

**Feature Branch**: `020-leer-menus-de-plataforma`

**Created**: 2026-09-14

**Status**: Draft

**Input**: Ver *Origen* al final.

## Contexto

El negocio vende por DiDi Food, Uber Eats y Rappi. El menú que los clientes ven en esas tres apps
**se captura a mano en el portal de cada una** y no tiene ninguna relación con el catálogo del POS.
Nadie sabe, hoy, en qué difieren. Cuando el precio de una crepa cambia en el mostrador, cambiarlo en
las tres apps es trabajo manual que se hace tarde, se hace a medias, o no se hace.

La consecuencia no es cosmética. Un platillo que ya no existe pero sigue publicado se vende y hay
que cancelarlo; un precio viejo se cobra al precio viejo y la comisión se calcula sobre él. Y hay un
costo mayor que este negocio ya conoce de cerca: **la plataforma penaliza al restaurante por el
desabasto que no se sincroniza** — hay un caso documentado de ítems marcados como agotados que no se
sincronizaron durante tres meses y terminaron con el restaurante en lista de monitoreo y riesgo de
expulsión.

[docs/apis-de-plataformas.md](../../docs/apis-de-plataformas.md) resolvió, en su revisión del
2026-09-13, la pregunta de si esto se puede hacer. La respuesta corta: **las tres plataformas
exponen una lectura documentada de su menú publicado, y nadie vende la comparación contra el
catálogo del POS.** Ese documento es la fuente de este spec y no se vuelve a derivar aquí.

## Qué NO es esta feature

Se declara arriba porque es lo que define su forma:

- **No escribe en ninguna plataforma.** Ni un precio, ni un platillo, ni la disponibilidad. El `PUT`
  de menú de Uber es reemplazo total —*«overwrites any existing menus»*— y el primer disparo contra
  la tienda viva puede borrar el menú publicado. Esta feature **lee y compara**; quien actúa es una
  persona, en el portal, con el reporte en la mano.
- **No recibe pedidos.** Eso es otro spec y depende de accesos que hoy no existen.
- **No automatiza el portal de comercios con un navegador.** La prohibición es explícita en los
  términos mexicanos de Uber y la terminación es inmediata y sin causa pactada. El beneficio serían
  minutos de trabajo a la semana; el costo del peor caso es la cuenta de la que vive el negocio.
- **No sincroniza en ninguna dirección.** Comparar no es corregir.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ver en qué difiere el menú de una plataforma (Priority: P1)

Quien administra el catálogo abre una pantalla, elige una plataforma, y ve **la lista de diferencias
entre lo que el POS dice y lo que esa app publica**: lo que está arriba y no en el POS, lo que está
en el POS y no arriba, y lo que está en los dos con distinto precio o distinta disponibilidad.

**Por qué es P1**: es la feature. Todo lo demás existe para que esta lista sea cierta.

**Prueba independiente**: con un menú de plataforma que difiere del catálogo en un platillo de cada
clase, la pantalla nombra los tres y no inventa un cuarto.

**Escenarios de aceptación**:

1. **Dado** un producto que existe en el POS a $95 y en la plataforma a $89, **cuando** se pide la
   comparación, **entonces** aparece como diferencia de precio, con los dos importes y cuál es cuál.
2. **Dado** un producto publicado en la plataforma que no existe en el catálogo del POS, **cuando**
   se pide la comparación, **entonces** aparece como «solo en la plataforma» y **no** se propone
   borrarlo: puede ser deliberado.
3. **Dado** un producto del POS que nunca se publicó, **cuando** se pide la comparación, **entonces**
   aparece como «solo en el POS», que es la lista con la que alguien decide si lo sube.
4. **Dado** un producto marcado como no disponible arriba y activo en el POS, **cuando** se pide la
   comparación, **entonces** la diferencia de disponibilidad se reporta aparte de la de precio —son
   dos problemas distintos y se arreglan en lugares distintos.
5. **Dado** que dos productos coinciden en todo, **cuando** se pide la comparación, **entonces** no
   aparecen. La pantalla lista lo que difiere, no el catálogo entero.

---

### User Story 2 - Emparejar por primera vez lo de arriba con lo de abajo (Priority: P1)

El menú de las plataformas se capturó a mano, así que **no trae ningún identificador del POS**. La
primera comparación no puede empatar nada sola. Quien administra el catálogo ve las dos listas lado
a lado, acepta las coincidencias que el sistema propone por nombre y precio, y resuelve a mano las
que no. Ese emparejamiento **se guarda y no se vuelve a pedir**.

**Por qué es P1 y no P2**: sin esto, la US1 reporta que el 100% del menú difiere, que es cierto y es
inútil. Es el trabajo que hace posible todo lo demás, y se hace una sola vez por plataforma.

**Prueba independiente**: con un menú de plataforma cuyos nombres coinciden parcialmente con el
catálogo, el sistema propone las coincidencias evidentes, deja las dudosas sin decidir, y lo que la
persona resuelve sobrevive a la siguiente lectura.

**Escenarios de aceptación**:

1. **Dado** un producto de la plataforma cuyo nombre coincide exactamente con uno del POS, **cuando**
   se abre el emparejamiento, **entonces** se propone la pareja **y se marca como propuesta, no como
   hecho** — quien administra la confirma.
2. **Dado** un producto cuyo nombre no coincide con ninguno, **cuando** se abre el emparejamiento,
   **entonces** queda sin pareja y visible, nunca emparejado por aproximación silenciosa.
3. **Dado** un emparejamiento ya confirmado, **cuando** se vuelve a leer el menú, **entonces** se
   conserva, aunque el nombre haya cambiado arriba.
4. **Dado** un producto del POS que en la plataforma es **varios** productos —«Arma tu Crepa» arriba
   es una crepa por sabor—, **cuando** se empareja, **entonces** el sistema acepta esa relación de
   uno a varios y no obliga a elegir una sola.
5. **Dado** que alguien se equivocó al emparejar, **cuando** deshace la pareja, **entonces** vuelve a
   quedar sin emparejar y la comparación lo refleja de inmediato.

---

### User Story 3 - Saber cuándo lo que veo dejó de ser cierto (Priority: P2)

Cada comparación dice **de cuándo es la lectura de cada plataforma**. Una diferencia leída hace tres
semanas no es una diferencia: es una foto vieja. Si la lectura falló, la pantalla lo dice en vez de
mostrar la anterior como si fuera de hoy.

**Por qué es P2**: la US1 sin esto sigue siendo útil el primer día y empieza a mentir al segundo.

**Prueba independiente**: con una lectura vieja y otra reciente, la pantalla distingue las dos y
nombra cuál es cuál; con una lectura fallida, dice que falló.

**Escenarios de aceptación**:

1. **Dado** un menú leído hace diez minutos, **cuando** se abre la comparación, **entonces** dice
   cuándo se leyó.
2. **Dado** que la última lectura falló, **cuando** se abre la comparación, **entonces** se dice que
   falló y desde cuándo no hay dato fresco — **nunca** se presenta la lectura anterior como actual.
3. **Dado** que una plataforma nunca se ha leído, **cuando** se abre la comparación, **entonces** se
   dice eso, y no «no hay diferencias».

---

### User Story 4 - Que nada de esto pueda tocar la tienda (Priority: P1)

Quien opera el negocio necesita poder confiar en que esta feature **no puede** cambiar nada arriba,
ni por error ni por un camino nuevo. Es una garantía estructural, no una promesa de pantalla.

**Por qué es P1**: el riesgo que cierra es catastrófico y silencioso — un menú borrado en la app que
factura el 30%, un sábado.

**Prueba independiente**: no existe en el sistema ningún camino que haga una escritura contra una
plataforma, y un intento de agregarlo rompe una prueba.

**Escenarios de aceptación**:

1. **Dado** el sistema completo, **cuando** se buscan las operaciones que puede hacer contra una
   plataforma, **entonces** todas son de lectura.
2. **Dado** un intento de agregar una operación de escritura, **cuando** corren las pruebas,
   **entonces** fallan nombrando la plataforma y la operación.
3. **Dado** que una credencial tiene permisos de escritura concedidos por la plataforma, **cuando**
   el sistema la usa, **entonces** sigue sin ejercer ninguno.

---

### Edge Cases

- **El menú de arriba está vacío o la lectura vuelve sin productos.** No es «todo el catálogo
  sobra»: es una lectura que no sirve, y se reporta como tal. Tratarla como dato válido propondría
  borrar el menú entero.
- **Un producto del POS cambió de nombre.** El emparejamiento guardado manda sobre el nombre; si no,
  cada edición de catálogo rompería las parejas.
- **Dos productos del POS con el mismo nombre.** La propuesta automática no puede elegir: los deja a
  los dos sin emparejar y lo dice.
- **El mismo producto de la plataforma emparejado con dos del POS.** Se rechaza: es un error de
  captura, y aceptarlo haría que la comparación reportara diferencias contradictorias.
- **El precio de la plataforma incluye el sobreprecio.** El POS ya guarda un precio por plataforma
  ([0037](../../server/migrations/0037_platform_prices.sql)); la comparación es contra **ese**
  precio, no contra el del mostrador. Comparar contra el de mostrador reportaría una diferencia en
  cada renglón, todos los días.
- **Una plataforma no tiene credenciales configuradas.** Se dice que no está conectada. No se
  inventa una comparación ni se deja la pantalla en blanco.
- **La lectura tarda o la plataforma no responde.** No puede dejar esperando a quien la pidió ni
  puede quedarse a medias y reportar diferencias falsas por los productos que no alcanzó a leer.
- **El menú de arriba trae más productos de los que caben.** Las plataformas publican topes (una de
  ellas, 4,000 ítems). Una lectura truncada que se compare completa reporta como «falta arriba» lo
  que sí está.

## Requirements *(mandatory)*

### Functional Requirements

**Leer**

- **FR-001**: El sistema MUST poder leer el menú publicado de una plataforma cuando tenga
  credenciales para ella, sin intervención humana en el momento de la lectura.
- **FR-002**: El sistema MUST guardar cada lectura con **el momento en que se hizo** y con la
  plataforma de la que vino.
- **FR-003**: El sistema MUST distinguir «lectura fallida» de «lectura sin diferencias» y de «nunca
  se ha leído». Los tres se ven igual en una pantalla mal hecha y significan cosas opuestas.
- **FR-004**: El sistema MUST NOT presentar una lectura vieja como si fuera reciente.
- **FR-005**: El sistema MUST tratar una lectura vacía o incompleta como fallida, no como un menú
  sin productos.

**No escribir**

- **FR-006**: El sistema MUST NOT ejecutar ninguna operación que cree, modifique, borre, publique o
  suspenda nada en una plataforma.
- **FR-007**: La prohibición de FR-006 MUST ser verificable por una prueba que falle si alguien
  agrega esa operación, y no solo por revisión humana.
- **FR-008**: El sistema MUST NOT automatizar el portal de comercios de ninguna plataforma.

**Emparejar**

- **FR-009**: El sistema MUST permitir emparejar un producto de la plataforma con un producto del
  catálogo, y conservar ese emparejamiento entre lecturas.
- **FR-010**: El sistema MUST permitir que **un** producto del catálogo corresponda a **varios**
  productos de una plataforma. Es el caso real del negocio, no una generalización.
- **FR-011**: El sistema MUST proponer emparejamientos automáticos **marcados como propuestas**, que
  una persona confirma. Un emparejamiento automático aceptado en silencio produce comparaciones
  falsas que nadie puede auditar.
- **FR-012**: El sistema MUST NOT emparejar dos productos del catálogo con el mismo producto de la
  plataforma.
- **FR-013**: El sistema MUST permitir deshacer un emparejamiento.
- **FR-014**: El emparejamiento MUST sobrevivir a que cambie el nombre del producto en cualquiera de
  los dos lados.

**Comparar**

- **FR-015**: El sistema MUST reportar, por plataforma: los productos solo en la plataforma, los
  productos solo en el catálogo, y los emparejados que difieren.
- **FR-016**: El sistema MUST reportar la diferencia de **precio** y la de **disponibilidad** como
  dos clases distintas de diferencia.
- **FR-017**: La comparación de precio MUST hacerse contra el precio que el catálogo tiene **para esa
  plataforma**, no contra el precio de mostrador.
- **FR-018**: El sistema MUST NOT listar los productos que coinciden.
- **FR-019**: El sistema MUST NOT proponer ninguna acción destructiva a partir de una diferencia. Un
  producto que solo existe arriba puede ser deliberado.

**Lo que se guarda**

- **FR-020**: El sistema MUST guardar el identificador que la plataforma usa para cada producto, tal
  como lo entrega, sin recortarlo ni normalizarlo.
- **FR-021**: El sistema MUST NOT guardar credenciales de plataforma en el repositorio ni en la base
  de datos en claro.
- **FR-022**: El sistema MUST NOT registrar credenciales ni secretos en la bitácora, incluido el
  caso en que la plataforma los transmita como parte de una dirección.

### Key Entities

- **Conexión con una plataforma** — qué plataforma, de qué empresa, si está configurada, y el
  identificador de la tienda de ese lado. Una empresa puede tener una conexión por plataforma.
- **Lectura del menú** — una foto del menú publicado de una plataforma en un momento dado, con su
  resultado (sirvió o falló). Es lo que permite decir «de cuándo es esto».
- **Producto de la plataforma** — un platillo tal como está publicado: su identificador de allá, su
  nombre, su precio y si está disponible. **No es un producto del catálogo** y no se mezcla con él.
- **Emparejamiento** — la relación entre un producto del catálogo y uno o varios productos de una
  plataforma, y si la confirmó una persona o solo se propuso.
- **Diferencia** — lo que la comparación produce: de qué clase es, a qué producto se refiere de cada
  lado, y qué valores difieren.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Quien administra el catálogo puede ver, en menos de un minuto y sin salir del sistema,
  en qué difiere el menú de una plataforma del catálogo del negocio.
- **SC-002**: El emparejamiento inicial de una plataforma se completa en **una sola sesión de
  trabajo** y no vuelve a pedirse.
- **SC-003**: Después del emparejamiento inicial, el número de diferencias reportadas corresponde a
  diferencias reales: al revisarlas a mano contra la app, **ninguna** resulta ser un falso positivo
  de emparejamiento.
- **SC-004**: Ninguna operación del sistema modifica nada en ninguna plataforma. Verificable
  revisando el conjunto completo de operaciones posibles, no por muestreo.
- **SC-005**: Toda comparación que se muestre dice de cuándo es su dato, y una lectura fallida nunca
  se presenta como una comparación válida.
- **SC-006**: Una lectura de menú no afecta el tiempo de respuesta de la captura de un pedido en el
  mostrador.
- **SC-007**: El sistema queda listo para leer una plataforma nueva sin cambiar lo ya construido
  para las otras dos.

## Assumptions

- **El acceso a las tres plataformas no está concedido hoy**, y el camino para obtenerlo no depende
  de este equipo: las tres exigen NDA y una aprobación humana. Esta feature se construye para estar
  lista cuando alguno llegue, y **la primera plataforma que se conecte de verdad será la primera que
  conteste**, no la que se elija por diseño.
- **La lectura se puede ejercer antes de tener acceso de producción.** Al menos una de las tres
  ofrece un ambiente de pruebas de alta inmediata, así que el código de lectura se puede probar
  contra una plataforma real sin firmar nada. No se construye a ciegas.
- **El menú de las plataformas se capturó a mano**, así que no trae identificadores del POS y el
  primer emparejamiento es manual. Se asume que es trabajo de una sola sesión por plataforma.
- **Las diferencias las resuelve una persona en el portal.** Esta feature le dice qué mover; no lo
  mueve.
- **El catálogo del POS es la fuente de verdad de lo que el negocio vende**, pero **no** de lo que
  se publica: que un platillo exista arriba y no abajo puede ser deliberado, y la feature no asume
  que sea un error.
- **El volumen es chico**: un menú de restaurante, no un catálogo de retail. Las decisiones se toman
  para decenas o cientos de productos, no para miles.
- **Cada plataforma llama distinto a las mismas cosas** y ninguna coincide con el vocabulario del
  POS. La feature asume que hay que traducir, y que la traducción es parte del trabajo.

## Dependencias

- [docs/apis-de-plataformas.md](../../docs/apis-de-plataformas.md) — qué se puede leer en cada
  plataforma, qué exige el acceso, y la revisión del 2026-09-13 que movió el veredicto técnico. **Su
  §7.2 lista las decisiones de esquema que cierran puertas y no se recuperan**; se lee antes del
  plan.
- [docs/plataformas-digitales.md](../../docs/plataformas-digitales.md) — las comisiones medidas y por
  qué el precio por plataforma existe.
- [0037](../../server/migrations/0037_platform_prices.sql) — el precio por producto y plataforma que
  ya existe, y contra el que se compara.
- La tabla de puertas abiertas de [la constitución](../../.specify/memory/constitution.md), principio
  VIII: esta feature cruza la de **«una lista de productos por plataforma»** y tiene que hacerlo sin
  cerrar la de **«promociones de plataforma»**.

## Origen

Pedido el 2026-09-13: preparar un módulo de conexión con Uber Eats, Rappi y DiDi Food, empezando por
entender cómo está el menú en las cuentas del negocio, para después detectar diferencias y
recomendar qué actualizar.

La investigación previa a este spec —cuatro frentes: las tres plataformas y el mercado de
middleware— está consolidada en [docs/apis-de-plataformas.md](../../docs/apis-de-plataformas.md) §0.
Lo que ese trabajo cambió respecto de lo pedido: **la escritura queda fuera**, porque el `PUT` de
menú de una de las plataformas es reemplazo total y puede borrar el menú publicado; y **la
comparación resultó ser la parte valiosa**, porque las tres exponen la lectura y ningún producto del
mercado vende la comparación contra el catálogo del POS.
