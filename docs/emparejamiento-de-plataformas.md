# Emparejamiento y sincronía con las plataformas — decisiones y pendientes

Material previo al spec que rediseñe la pantalla de la tienda de plataforma **cuando ya está
conectada** (emparejar lo publicado en Uber con el catálogo del POS, y mantenerlo en sincronía).
Recoge lo que el dueño decidió y lo que salió de evaluar la pantalla actual. Léelo antes de abrir
ese spec, junto con [catalogo-por-canal.md](catalogo-por-canal.md), cuya decisión 1 —el mapeo
apunta a una configuración vendible, no a un producto— sigue mandando.

## 1. Regla decidida (2026-09-28): con la API conectada, el precio lo pone la plataforma

**Si una plataforma está conectada por API, su precio manda siempre.** El precio por plataforma
del POS de un producto emparejado (`product_platform_prices`) se **sobrescribe** con el de la
plataforma en cada lectura del menú; no hay diferencia de precio que la persona tenga que decidir.
La captura a mano de precios por plataforma queda solo para las plataformas **no** conectadas.

- **Por qué**: la dirección de la verdad ya era de la plataforma hacia el POS
  (`AGENTS.md`, nota de `product_platform_prices`). Sin API, el precio se copiaba a mano para que
  el ticket cuadrara con lo que la app ya cobró; con API, copiarlo a mano es trabajo que solo puede
  salir mal. Quita de la pantalla el peso de «precio distinto».
- **Tiene que ser explícito en el sistema**, no una regla escondida:
  - en la tienda conectada, un aviso fijo: «Los precios de Uber los pone Uber. Para cambiar un
    precio, cámbialo en Uber» y, tras cada lectura, cuántos precios se actualizaron y cuáles;
  - en el catálogo, el precio de una plataforma conectada se ve **bloqueado** con «Lo pone Uber ·
    se actualizó hoy a las 10:42», y el de una no conectada sigue editable con «Lo capturas tú».
- **Nunca se escribe en la plataforma** desde el POS: el transporte de `internal/uber` es solo
  lectura salvo aceptar y rechazar pedidos.

### Las cuatro preguntas de la regla, decididas por el dueño (2026-09-30)

1. **El emparejamiento es flexible, no uno a uno.** Tiene que admitir los tres casos: el mismo
   producto en los dos lados; uno del POS ligado a varios de la plataforma (cada crepa de Uber a
   «Arma tu Crepa»); y uno de la plataforma con otra composición ligado a un producto base del POS.
   El caso de El Gato Bobah se resolverá desactivando en Uber lo que no coincide, pero el modelo no
   se cierra a los otros clientes. **Consecuencia**: el precio de la plataforma se guarda **por
   platillo de la plataforma** (por pareja), no por producto del POS; así uno a varios no obliga a
   elegir un precio.
2. **Sucursales en la base desde ahora, aunque no se vean.** Toda empresa nace con una sucursal y
   puede tener N (otra sucursal, una cocina oculta). Cruza la puerta «Más de una sucursal» del
   principio VIII: va en **su propio spec, antes** que el rediseño de emparejar, porque una tienda de
   plataforma pertenece a una sucursal y su precio también. **Hecho en el spec 025**:
   `platform_connections.branch_id` existe, así que el precio por platillo de la tienda ya cuelga de
   una sucursal.
3. **Extras: hoy su precio también viene de la plataforma.** A futuro se quiere el camino inverso
   —crear un producto en el POS y publicarlo en la plataforma— y ligar bien lo creado allá. Eso
   **reabre** la decisión del §5 (hoy Uber manda y el POS solo lee): cuando llegue, habrá que decidir
   por producto quién manda. Ligado a otro spec pendiente, el de insumos y variantes (ver abajo).
4. **La captura a mano de un pedido de plataforma se queda**, también con la API conectada: sirve al
   cliente que no contrate la integración.

### Spec pendiente: insumos, tamaños y variantes que heredan (deuda reconocida, 2026-09-30)

Hoy las variantes se crean a mano en cada grupo y no se reutilizan: «leche deslactosada» existe
por separado en frappés, bobah teas y licuados. Se quiere:

- **Insumos reutilizables**: la leche deslactosada es un insumo, y cada variante que la usa lo
  referencia.
- **Tamaños por producto terminado**, cada uno con su receta.
- **Variantes con «padre»**: una opción renombrada en un producto hereda del original, y en el
  catálogo se ve de dónde viene.
- **El ticket dice lo que se agregó** («+ leche deslactosada»).

Va en su propio spec. Relacionado con [costeo-de-recetas.md](costeo-de-recetas.md) y la puerta
«Costear con recetas e inventario de insumos» del principio VIII.

## 2. La pantalla de emparejar hoy: defectos encontrados (2026-09-28)

Evaluada con Playwright a 1024×600 sobre un menú de 65 platillos y 25 opciones contra 174
productos, por tres evaluadores independientes (conciliación de datos, tableta, flujo del
operador). Defectos de código, con consenso:

- **No deja ligar un producto a varios platillos** (uno a varios): `Emparejar` quita del buscador
  todo producto que ya tiene pareja (`unlinkedLocal`), aunque la base lo permite. Cierra en la
  pantalla una puerta del principio VIII.
- **La fila de decisión se sale de la pantalla** a 1024 px: «Más tarde» solo se alcanza arrastrando
  de lado, y en vertical no se ve.
- **El nivel «opción» es inalcanzable** desde la interfaz: `EmparejarConexionPage` nunca pasa
  `nivel`.
- **El precio de la plataforma sale sin formato** en las diferencias (`$82.8`).

## 3. Rediseño: lo que tuvo consenso (al menos 2 de 3 evaluadores)

- Ver el universo completo: una lista de todos los platillos, filtrada por estado con contador.
- Corregir cualquier pareja desde la lista, no solo la última.
- Emparejar platillos y opciones en la misma vista.
- Confirmar en lote las propuestas **con revisión** (casillas marcadas, los dos nombres a la vista;
  nada se confirma sin que se vea — FR-011).
- Candidatos ordenados por parecido al buscar (ordenar, no proponer).
- Decisiones que se guardan: «solo existe en la plataforma» y «no se vende en la plataforma». Sin
  ellas, «Solo en un lado» nunca baja a cero (hoy mezcla lo pendiente con lo deliberado).
- La tienda conectada en su propia vista; la conexión y la llave dejan de ocupar el trabajo diario.
- Con la regla del §1, la pestaña «Precio distinto» deja de ser tarea: es un aviso informativo.

Lo que los tres rechazaron: confirmar por parecido sin revisión, arrastrar entre dos listas y
escribir en la plataforma desde esta pantalla.

**Forma de la vista**: la propuesta recomendada es pestañas por estado con una tabla
(«platillo de la plataforma → producto del POS»). Las alternativas (lista y panel lado a lado; un
platillo arriba y la lista abajo) y las pantallas dibujadas viven en el lienzo de diseño del dueño.
**2026-09-29**: el dueño descartó la A y pidió explorar B y C, porque aprovechan toda la pantalla.
Las dos quedaron dibujadas en cuatro estados cada una (sin pareja, lote, corregir, y vertical en B;
con propuesta, sin propuesta, lote y corregir en C). En B, «Por revisar» tiene los dos modos con un
selector: **uno por uno** (una propuesta en grande, avanza sola al confirmar) y **en lote**
(casillas y un solo «Confirmar»); el dueño pidió conservar los dos. **Decidido (2026-09-30): la B**, lista y panel lado a lado,
con los dos modos de «Por revisar». La C queda descartada.

## 4. Pendiente para después: el producto «OTRO» (2026-09-28)

Cuando llegue por webhook un pedido con un platillo que no está emparejado, que se ligue a un
producto por omisión «OTRO» con el detalle del platillo (nombre, precio, opciones), para que al
menos salga en el ticket de impresión y herede lo que un producto necesita para mostrarse ahí.

Hoy la 021 ya acepta el pedido y deja el renglón con `product_id` NULL y la copia del nombre y el
precio (0073). **No está medido** qué le falta a ese renglón para imprimirse igual que uno normal
(categoría, estación de cocina, reglas del ticket): medirlo antes de diseñarlo. Probablemente va
en su propio spec, cuando se prueben los webhooks de la 021.

## 5. ¿Administrar el menú de Uber desde el POS? Lo verificado (2026-09-29)

El dueño preguntó si, con la API conectada, el POS podría administrar el menú de Uber en vez de
solo leerlo. Verificado contra developer.uber.com (Eats Marketplace):

| Qué | Estado | Fuente |
|---|---|---|
| Existe actualización de **un solo ítem** sin reenviar el menú: `POST /v2/eats/stores/{store_id}/menus/items/{item_id}`, parcial (precio en centavos, suspensión con fecha y razón, entre otros). Responde 204, se aplica al instante, sin aprobación | VERIFICADO | [post-eats-stores-storeid-menus-items-itemid](https://developer.uber.com/docs/eats/references/api/v2/post-eats-stores-storeid-menus-items-itemid) |
| **El scope `eats.store` alcanza para escribir**: es el mismo con el que la 020 ya lee. Con las credenciales de hoy, lo único que impide escribir es la guarda de solo lectura del transporte (`internal/uber/solo_lectura.go`), y por eso no se afloja | VERIFICADO | misma página + `put-eats-stores-storeid-menu` |
| **Precondición**: *«If you have not used upload menu endpoint before, you might receive a 404 error while using update item endpoint.»* El `PUT` completo tiene que haberse usado antes. El menú de la tienda se capturó a mano en Uber Eats Manager (`external_data` vacío), y **no está documentado** si eso cuenta | VERIFICADO / HUECO | misma página |
| El `PUT` completo **reemplaza todo el menú al instante**, sin aprobación ni deshacer («overwrites any existing menus») | VERIFICADO | [menu-integration](https://developer.uber.com/docs/eats/guides/menu-integration) |
| Tamaño máximo, límite de ítems y rate limit del `PUT`/`POST` | HUECO | — |
| **Las pruebas de escritura exigen una tienda de prueba que se pide a Integration Tech Support.** Por eso `test-api.uber.com` devolvió la tienda real en la 020: nunca se pidió una | VERIFICADO | [sandbox](https://developer.uber.com/docs/eats/guides/sandbox) |
| Lo que se edite a mano en Uber Eats Manager se pierde en el siguiente `PUT` del POS | INDICIO (guía de Toast, no de Uber) | support.toasttab.com |
| `store.menu_refresh_request` solo exige un 200 vacío; no está documentado qué lo dispara | VERIFICADO / HUECO | Menu-refresh-request-webhook |
| **Uber sí modela «un producto, varios sabores»**: un ítem con un grupo de modificadores, anidable hasta 6 niveles. Hoy el menú está publicado plano (cada crepa es un ítem) porque así se capturó, no porque Uber no pueda | VERIFICADO | example-menu-payloads |

**Consecuencias:**

- **Si el POS administra el menú, la dirección de la verdad se invierte**: el precio lo pone el POS
  y se publica en Uber. Contradice la regla del §1. No pueden mandar los dos, porque Uber Eats
  Manager sigue permitiendo editar.
- **El primer `PUT` es el momento de mayor riesgo del proyecto**: probablemente será el primero de
  verdad contra la tienda real, y solo se puede ensayar en una tienda de prueba pedida aparte.
- **Remodelar las crepas como un ítem con grupo de sabores** cambiaría los ids de Uber (derivados
  del nombre), que hoy son la llave del emparejamiento. Es una decisión irreversible del principio
  VIII: se decide por escrito antes.

**Decidido (2026-09-29): camino 1, Uber manda y el POS solo lee.** Precios y menú se editan en Uber
Eats Manager; la regla del §1 queda igual y la guarda de solo lectura del transporte no se afloja.
Descartados por ahora: (2) el POS manda precio y disponibilidad por ítem, y (3) el POS administra el
menú completo. Reabrirlos exige antes una tienda de prueba pedida a Integration Tech Support y el
permiso de escritura confirmado por Uber.

## 6. Combos y almacén: lo verificado (2026-10-03)

El dueño aclaró que un platillo de Uber se liga a **un solo** producto del POS, y que la duda real
eran los combos: venderlos en Uber y que el POS descuente del almacén cada componente.

**Cómo los modela Uber** (developer.uber.com; la página se renderiza con JS, así que es alta
confianza, no lectura del código fuente):

- **Combo fijo**: `bundled_items` en el ítem, la lista de lo que va incluido siempre y el cliente no
  elige (las papas de la hamburguesa). Reusa ítems del menú.
- **Combo con elección** («crepa + bebida a escoger»): un ítem con un grupo de modificadores cuyas
  opciones **son ítems del menú**. Es el mismo patrón que la tienda real ya usa para los tamaños
  (`server/internal/uber/testdata/menu.json`).
- **En el pedido** llega el renglón padre y, anidado, lo que se eligió, cada cosa con su propio id y
  precio. No hay un tipo «combo» aparte.
- **Promociones** (2x1, producto gratis) no son combos: viven en `payment.promotions` del pedido,
  separadas del carrito.

**Consecuencia para el diseño**: no hace falta una tabla de combos para emparejar. Cada componente
se liga por su id a lo suyo en el POS (el platillo a un producto, la opción a una opción) y el
almacén se descuenta componente por componente.

**Lo que hoy impide que funcione** (medido en el código):

1. **Los pedidos de plataforma aceptados no descuentan nada del almacén.** Aceptar crea el pedido,
   los renglones y el pago, pero nunca llama a la depleción. Y los renglones hijos (extras,
   componentes) se descartan: solo viajan dentro del nombre.
2. **En el mostrador tampoco se descuentan los extras.** La depleción mira solo el producto del
   renglón; ignora la receta o el producto ligado de cada opción elegida. Un combo del POS no
   descuenta nada (no tiene receta), y la leche deslactosada elegida como extra tampoco.
3. **Una opción de Uber no tiene dónde ligarse**: el emparejamiento solo guarda `product_id`, no hay
   columna para una opción de modificador. Por eso el defecto 3 del §2 no es solo de pantalla.
4. **No está confirmado cómo llega el detalle de un pedido**: el lector espera
   `selected_modifier_groups_items`, la documentación dice `selected_modifier_groups`. Si es lo
   segundo, hoy se pierden los extras de todo pedido de Uber. Se zanja con un pedido real de prueba
   con un extra.

**Combos del POS hoy**: las tablas `combo_slots` existen y están vacías; los combos del menú real
son productos simples con sus componentes como modificadores.

### Decidido por el dueño (2026-10-03)

1. **Un platillo u opción de Uber se liga a una sola cosa del POS.** Varios de Uber pueden ir al
   mismo producto; entonces quien configura elige cuál da el precio de la captura a mano.
2. **Nadie decide nada al operar** (regla en la constitución): un pedido de Uber se acepta y sale a
   cocina. Toda ambigüedad se resuelve al configurar.
3. **Los extras que no descuentan son un defecto que se corrige** en su propio spec, sin perder cómo
   está hoy el negocio. Al llegar ahí: respaldar producción, ensayar en local la migración que
   corrige y vuelve a ligar las variantes (hay grupos y opciones repetidos, ver `docs/reorg/16_*`), y
   solo entonces aplicarla.
4. **Paquetes y promociones**: además del combo, cubrir el 2x1 y el paquete con nombre propio que
   adentro trae varios productos (crepa + papas, frappé + crepa), con conteo e histórico por
   producto, ingrediente, insumo y sus sub-recetas, no solo por insumo.
5. **Aproximados de recetas y almacén** se sacan de los históricos de FUDO
   (`~/gatobobah-datos/references/`, fuera del repo; ver [respaldo-fudo.md](respaldo-fudo.md)).

**Orden de los specs**: emparejar (026, con el destino «opción») → almacén de extras, paquetes y
pedidos de plataforma → insumos, tamaños y variantes que heredan.

**Cómo confirmar el formato del pedido (punto 4 de arriba)**: el probador de webhooks del panel de
desarrollador de Uber manda un aviso de prueba y sirve para comprobar que llega y que la firma
cuadra. El contenido del pedido no viene en el aviso: se pide aparte al enlace que el aviso trae, y
solo existe si el pedido existe. Si el panel no crea pedidos de prueba con contenido, hace falta la
tienda de prueba de Uber o un pedido real chico. Sin verificar todavía qué ofrece el panel.

## 7. Construido (spec 026, 2026-10-03)

En la rama de la integración con Uber, sin desplegar todavía:

- Pantalla con el diseño B y sus dos modos de revisión; elección del precio de captura (tablero B5).
- Parejas a opciones del POS; decisiones «solo existe en la plataforma» y «no se vende ahí».
- Precio que pone la plataforma: copiado en cada lectura buena, bloqueado en el POS y avisado a las
  tabletas. Con dos tiendas de la misma plataforma no se copia (precio por sucursal pendiente).
- Producto genérico para renglones de pedido sin pareja, con las opciones en la nota.

Pendiente: confirmar con un pedido real el nombre del campo de las opciones en el detalle del
pedido. El almacén quedó en el §8.

## 8. Construido (spec 028, 2026-10-07): el almacén descuenta extras, paquetes y plataforma

Resuelve los puntos 1 y 2 del §6 y el 4 de lo decidido:

- El mostrador descuenta cada extra (su receta o el producto que es) y lo que lleva un paquete.
- Aceptar un pedido de plataforma descuenta el platillo y las opciones emparejadas; el producto
  genérico no descuenta.
- «Qué lleva» en el producto y en el extra: insumos, el producto que es (un extra) o los productos
  que lleva con sus piezas (un paquete); estimado o confirmado, y «No lleva nada» para lo que no
  descuenta (un «Sin hielo»). Un producto se vuelve paquete al capturarle productos; el POS lo vende
  igual que antes.
- Reportes cuenta las unidades por producto, sueltas y dentro de paquetes.
- La carga de FUDO llena lo que falta como estimado y no toca lo ya capturado. Rechaza insumos
  circulares y no arma paquetes: los reporta para armarlos en «Qué lleva».

Sigue abierto:

- **Las promociones de plataforma** (2x1, regalo): sin un pedido real no hay formato que leer.
- **La cantidad de una opción de plataforma** se toma por unidad del platillo, como en mostrador. No
  está comprobado con un pedido real de cantidad mayor a uno.
- **Cancelar un pedido completo repone también lo ya enviado a cocina**, como antes. Cambiarlo es
  una decisión del dueño, no un arreglo.
- **Algunos insumos compuestos de FUDO parecen invertidos** (un frasco «hecho de» cucharadas): se
  cargan como estimados y se revisan en Almacén → Insumos, donde cada uno dice si se compra o se
  prepara aquí y abre su «Qué lleva» con lo que rinde.
