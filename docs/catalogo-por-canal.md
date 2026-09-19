# Catálogo canónico × catálogo de canal — cómo lo modela quien ya lo resolvió

**Investigado el 2026-09-14.** Referencia previa al plan de la [spec 020](../specs/020-leer-menus-de-plataforma/spec.md).
No es un plan: es lo que se sabe y con qué respaldo.

**Niveles**: **[D]** fuente primaria · **[R]** terceros · **[I]** inferido.

> Los números del negocio no van aquí (`AGENTS.md` §1). Lo que se documenta son hechos que siguen
> siendo ciertos en otro restaurante.

---

## 1. La decisión que ordena todo: el mapeo NO apunta a un producto

Un catálogo donde **el 80% de los productos tiene grupos de modificadores** —y grupos de 74, 71 y 61
opciones— no se puede mapear producto a producto. En la plataforma, un configurable se publica como
N platillos planos; abajo es uno.

El destino del mapeo tiene que ser una **configuración vendible**: un producto **más** una selección
concreta de opciones, posiblemente vacía. Con selección vacía *es* el producto pelón, así que los
platillos que ya existen como producto propio caen ahí sin caso especial.

**Y el caso real es mixto**: de los N ítems publicados arriba, unos empatan 1:1 con un producto que
ya existe y otros son combinaciones del mismo configurable. Ningún esquema público leído resuelve
ese caso; la *configuración vendible* es propuesta, no copia [I].

**No se pre-expande.** El modo que aplica es el `dynamic` de Odoo: la combinación se materializa
**solo cuando alguien la publica**. Un configurable con cuatro grupos puede tener millones de
combinaciones vendibles; ninguna se toca hasta que se nombra [D].

---

## 2. Los dos modelos públicos que vale la pena copiar

### Open Delivery v2 (Abrasel) — [D], `merchant.openapi.yaml`

Su decisión estructural: **`Item` (el producto) e `ItemOffer` (la oferta vendible) son entidades
separadas.** El precio y la disponibilidad viven en la oferta; el nombre, la descripción, las
imágenes y **la unidad de venta** viven en el ítem.

- **Un modificador ES un producto**: *«An option has no name of its own — it inherits it from the
  `Item` referenced via `itemId`»*. Igual que Uber.
- **`Option.defaultQuantity`**: cantidad incluida sin seleccionar; **ponerlo en 0 permite quitar un
  ingrediente que viene por defecto**. Es la primitiva del «sin cebolla».
- **`priceMethod`** por grupo: `SUM` / `MAX` / `AVG`.
- **Identificadores**: *«Opaque strings (no meaning derived from the value) · UUID v4 recommended ·
  **Stable after creation**»*.
- **Precio por canal: no hay overrides.** Se resuelve con **otro menú** por servicio. El contenido
  queda duplicado.
- **Tres modelos de sincronización mutuamente excluyentes** (`PULL`, `PUSH_FULL`, `PUSH_CRUD`), y lo
  dice con todas sus letras: *«combining pull and push-full for the same menu leads to race
  conditions»*. El `GET` del snapshot queda siempre disponible para *«Reconciliation (detect drift
  after extended downtime)»*.
- **El pedido que regresa trae el código del POS del platillo Y de cada opción**, más snapshot de
  nombre y precio.

> **Dos contradicciones dentro de su propia documentación** [D, verificadas en los dos archivos]: la
> guía pone `externalCode` e `imageUrl` en `ItemOffer` y el contrato normativo no los tiene ahí; y
> la guía dice que el precio va en unidades menores mientras el YAML lo declara `double` con ejemplo
> `25.00`. **Quien implemente contra la guía y quien implemente contra el contrato difieren ×100.**

### HubRise — [D], hubrise.com/developers/api/catalogs

Su aporte es el **override escaso con condiciones**:

```
variant                { ref, name }                       ← el canal (o la sucursal)
sku.price_overrides[]  { price, variant_refs[], dow,
                         start_time, end_time,
                         start_date, end_date }
```

Regla de precedencia: *«cuando empatan varias reglas, aplica el precio de la **última**; cuando no
empata ninguna, aplica el precio por defecto»*. Y `restrictions` con la misma forma resuelve la
disponibilidad por canal.

**Sus tres límites, que deciden hasta dónde copiarlo:**

- **No hay override de nombre, descripción ni imagen.** Solo precio y disponibilidad.
- El catálogo **se reemplaza entero** y no hay endpoint por ítem, así que el `ref` es la identidad
  real y debe ser *«unique, auto-generated, and stable»*.
- En el pedido, `sku_ref` es **opcional**: puede llegar un renglón sin llave resoluble.

### Qué copiar de cada uno

De Open Delivery, la partición **producto / oferta vendible** y la disponibilidad como entidad
propia. De HubRise, el **override escaso con condiciones** y *la última regla que empata gana*.

---

## 3. Las llaves externas: dos familias y **tres niveles**

| Familia | Quién la asigna | Ejemplos | Si cambia |
|---|---|---|---|
| **Llave del comercio** | El POS; la plataforma hace upsert por ella | DoorDash `merchant_supplied_id`, HubRise `ref`, Uber `id` + `external_data`, DiDi `app_item_id`, Deliverect `plu` | **Duplica, no renombra** |
| **Id de la plataforma** | La plataforma, opaco | Open Delivery `id`, Square `id` + `version` | Se pierde el ancla del histórico |

**Y no es una llave por producto: son tres.** En Uber, `external_data` se llena en el **ítem**, en el
**grupo de modificadores** y en la **opción** [R, integrador en producción]. Con grupos de decenas de
opciones, eso son cientos de entidades que hoy se identifican por su `id` de secuencia y por su
nombre — y ninguna de las dos es una llave pública legítima.

**Ningún canal expone una tabla de mapeo 1:N en su API.** Todos exponen la llave y esperan que el
integrador mantenga la tabla. Esto se construye; no se compra [D, por ausencia en ocho proveedores].

**Nombre distinto por canal**: solo tres lo modelan como primitiva —BigCommerce (`listing.name`),
Akeneo (atributo `scopable`) y Deliverect (`overloads`)—. Los e-commerce modernos tratan el canal
como interruptor de disponibilidad y precio, nunca como dimensión de contenido.

---

## 4. Drift: el mismo esqueleto con tres nombres

| Fuente | Deseado | Observado |
|---|---|---|
| **Google Merchant** | recurso `products` | **recurso REST aparte** `productstatuses`, con `itemLevelIssues[]` y `lastUpdateDate` |
| **Kubernetes** | `spec`, lo escribe el usuario | `status`, *«supplied and updated by the system»*, en un subrecurso |
| **Terraform** | configuración | `state`; `plan` muestra el diff, `-refresh-only` acepta lo remoto, `import` adopta lo no gestionado. **No hay fusión automática de conflicto** |

Los tres casos se mapean directo: **solo cambió lo local** → publicar; **solo cambió lo remoto** →
adoptar; **cambiaron los dos** → diff por atributo y **decisión humana**.

**Y el acuse de recibo no es el estado publicado**, con tres fuentes independientes: Amazon responde
`ACCEPTED` que *no* significa publicado; su `processingStatus: DONE` *«no significa sin errores»*; y
Rappi devuelve `200` que significa «en cola de aprobación», con un `MENU_REJECTED` que llega sin
motivo.

**Consecuencia de modelado**: el estado publicado va en **filas propias, pobladas solo por el
sincronizador**, nunca en columnas de la fila del catálogo, y **nunca como un solo hash**. Un hash
contesta «difieren»; no contesta «en qué».

---

## 5. Los seis modos de falla del modelo de datos

| Falla | Qué la cierra |
|---|---|
| **Alguien editó el menú en el portal** | Nunca fusionar solo. Mostrar el diff y exigir decisión |
| **Se borra un producto de un lado** | Borrado suave y snapshot en la línea de venta. El mapeo huérfano se marca, no se borra. **Qué pasa cuando el canal borra uno de los N no lo documenta nadie** — hueco de la industria |
| **Dos productos se fusionan, o uno se parte** | Tabla de alias y cierre de la fila vieja. Los ids externos ya repartidos **no se desdoblan**: uno se queda con el histórico |
| **Cambia la unidad de venta o el empaque** | Es **otro producto**. GS1 lo decide formalmente y prohíbe acumular cambios chicos para evadir el umbral; Open Delivery coincide poniendo `unit` en el ítem |
| **Llega un pedido con un ítem no mapeado** | **Uber no valida el menú antes de crear el pedido**: tiene `resolveOrderFulfillmentIssue` para reportar después lo que no se reconoce. Hay que poder aterrizar un renglón **sin producto resuelto** |
| **Se publica plano lo que abajo es una configuración** | Sin la selección de opciones guardada en el mapeo, no se puede reconstruir qué se cocina ni calcular el precio esperado |

---

## 6. El modelo propuesto: cinco entidades

Ninguna toca `products` ni `modifier_options`.

1. **Configuración vendible** — producto + selección de opciones (posiblemente vacía). Se
   materializa solo al publicarse. **Es el destino del mapeo, nunca el producto.** Lleva su llave
   estable hacia afuera: opaca, asignada por el sistema, jamás editable.
2. **Canal de publicación** — el eje único. **Hoy el esquema tiene dos** (`channels` y
   `delivery_platforms`) y ningún documento dice cuál manda. Decidirlo antes de colgarle nada.
3. **Publicación** — la fila por (canal, configuración vendible): el id de allá completo y sin
   normalizar, overrides escasos de nombre/descripción/imagen, procedencia (propuesta automática o
   confirmada por una persona) y ciclo de vida `propuesta → confirmada → publicada → rechazada →
   retirada`. **A lo más una publicación viva por (canal, id de allá).**
4. **Lectura del menú publicado** — una por (canal, momento), con su resultado. Una lectura vacía es
   **fallida**, no un menú sin productos.
5. **Ítem observado** — lo que la lectura encontró, tal cual. **Vive aparte del catálogo.** Es el
   `status` de Kubernetes: lo escribe el sincronizador, nadie más.

**La diferencia se deriva** cruzando publicación × ítem observado, y **no se guarda**: guardarla la
deja mintiendo a la siguiente lectura.

**El costo NO va en la publicación.** El precio es por canal; el costo del insumo es canónico y no
depende de quién venda. Saleor pone `costPrice` dentro del listing por canal y para un restaurante
eso es un error.

---

## 7. Las nueve decisiones irreversibles

Las que cuestan una migración sobre datos vivos.

| # | Decisión | Por qué no se recupera |
|---|---|---|
| 1 | **El mapeo apunta a una configuración vendible, no a un producto** | Al desdoblar después no hay de dónde sacar qué opciones representaba cada ítem de allá. Con un catálogo mayoritariamente configurable, falla en casi todo |
| 2 | **La llave estable hacia afuera es opaca, no editable, y existe en tres niveles** | Si cambia, la plataforma **duplica**. Y Uber la exige en ítem, grupo y opción |
| 3 | **El id de la plataforma se guarda completo y textual** | Del código corto no se reconstruye el largo. Mismo razonamiento que el folio en la 0065 |
| 4 | **Un eje de canal, no dos** | Reunificarlos después es reescribir cada fila de publicación y de visibilidad |
| 5 | **El estado publicado vive en filas propias** | Convertir columnas en filas obliga a inventar el pasado que las columnas nunca tuvieron |
| 6 | **Una publicación retirada se marca, no se borra** | Se pierde el registro de que ese ítem existió allá |
| 7 | **La lectura se guarda con su momento y su resultado** | Sin la serie no se distingue lectura fallida de menú vacío |
| 8 | **La unidad de venta pertenece al producto** | Si la publicación la puede mover, el histórico deja de ser comparable |
| 9 | **El costo es canónico y se snapshotea en la venta; el precio es lo único por canal** | Lo que se escribió en cero ya no se corrige: no existe el dato de cuánto costaba el insumo ese día |

**Y una ya tomada que conviene no romper**: los overrides son **escasos, con derivación del
canónico** — está implementado en `domain.PlatformPrice` y razonado en la
[0037](../server/migrations/0037_platform_prices.sql). Extenderlo a nombre, descripción e imagen es
gratis; abandonarlo por copia completa por canal no se deshace.

---

## 8. Qué NO construir todavía

Todo esto son filas nuevas sobre llaves que ya existirían, así que después sale igual de barato:
escritura contra cualquier plataforma; ventanas de hora y fecha en el override; `priceMethod`
MAX/AVG; grupos anidados (solo Uber los soporta); aplicar la diferencia automáticamente; la
diferencia como tabla; variante de tamaño; porción por (opción, producto); descuentos y promociones
—**siempre que la publicación no asuma que el precio publicado es el precio cobrado**—.

---

## 9. Los huecos, con nombre

1. **`order_lines.product_id` es `not null`.** Un pedido con un ítem no mapeado no tiene dónde
   aterrizar. **Decidirlo antes de cualquier spec de pedidos entrantes.**
2. **`order_lines` no snapshotea qué publicación originó el renglón.** Si el mapeo cambia, el pasado
   deja de poder explicarse.
3. **Dos ejes de canal en el esquema** y ningún documento dice cuál manda.
4. **No existe ninguna llave externa de producto en el esquema**: cero coincidencias de
   `external_id`, `external_code` o equivalentes.
5. **`products.sku` es el candidato obvio y es el equivocado**: editable, nullable, unicidad por
   empresa.
6. **Nombre por canal en el dominio restaurantero solo lo modela Deliverect**, y esa fuente es [R].
7. **Ningún esquema leído resuelve el caso mixto** (unos ítems 1:1, otros combinaciones).
8. **Qué pasa cuando el canal borra uno de los N** no lo documenta nadie.
9. **El SC-002 de la spec 020 —«el emparejamiento inicial se completa en una sola sesión»— no se
   midió contra este catálogo.** Con grupos de decenas de opciones, un emparejamiento 1:N por opción
   puede ser un orden de magnitud mayor de lo que ese criterio supone. **Hay que volver a medirlo
   antes de comprometerlo.**

## Mantenimiento

Referencia viva, indexada en [docs/README.md](README.md). Se actualiza cuando una de sus
suposiciones se verifique o se caiga.
