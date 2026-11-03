# Research: Nombres alternativos (063)

## R1. Dónde se guardan: columna `text[]` en la fila, no tabla aparte

- **Decisión**: `products.alt_names text[] not null default '{}'` e `ingredients.alt_names text[] not
  null default '{}'`, con `check (cardinality(alt_names) <= 10)`.
- **Por qué**:
  - El choque que hay que impedir cruza el nombre principal y los alternativos. Ningún índice único
    lo cubre: el nombre principal de un producto solo es único exacto (`citext`), sin acentos ni
    espacios. Con tabla aparte o sin ella, la validación necesita el candado de la empresa (R3).
    La tabla no compra una garantía que la columna no tenga.
  - La columna hereda lo que la fila ya tiene: RLS, `GRANT` a `gatobobah_app`, el trigger de
    `catalog_versions` en `products` (el menú del POS se invalida solo al guardar alternativos), y
    queda fuera de `app.MergeTables`/`UnitChangeTables`, que clasifican **tablas** que apuntan a
    `ingredients`.
  - Una tabla nueva exigía política RLS, grants, trigger de versión de catálogo, clasificación en la
    unión de insumos y en el cambio de unidad, y su test de aislamiento propio. Es más superficie para
    10 textos por fila.
- **Alternativas**: tabla `product_alt_names`/`ingredient_alt_names` con índice único por llave
  normalizada (descartada por lo de arriba); `jsonb` (sin ganancia sobre `text[]`, y sqlc lo tipa
  como bytes).

## R2. La llave de comparación: una función en la base y su gemela en Go

- **Decisión**: la 0107 crea `catalog_name_key(text) returns text immutable` =
  `regexp_replace(translate(lower(btrim(x)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')`, la misma
  expresión que `IngredientNameTaken`, `FindArticleByName` y el índice de empaques. En Go,
  `domain.CatalogNameKey` = minúsculas, quitar los mismos acentos y juntar espacios con
  `strings.Fields` (como `NormalizeReasonName`). Una prueba de paridad compara las dos sobre los
  casos que ya mordieron: acentos, mayúsculas, tabulador, espacio duro, espacios en las orillas.
- **Por qué**: las consultas nuevas la citan una vez en vez de copiar la expresión seis veces. Las
  consultas que ya existen no se reescriben (no es su feature).
- **Lo que se guarda va limpio**: `domain.CleanAltNames` recorta y junta espacios con
  `strings.Fields`, así que un alternativo guardado nunca trae espacio duro ni dobles, y la
  diferencia `\s` de Go contra la de Postgres no llega a la comparación.

## R3. Candado: el mismo de las altas en artículos, uno nuevo en productos

- **Artículos**: `LockIngredientNames` (28037) ya serializa las altas y los renombres. Guardar
  alternativos, crear, renombrar y el reemplazo por cambio de unidad lo toman antes de leer los
  nombres. Un solo candado para nombre y alternativos: comparten el mismo espacio de nombres.
- **Productos**: `LockProductNames` = `pg_advisory_xact_lock(28063, empresa)`. Lo toman guardar
  alternativos, `CreateProduct`, `UpdateProduct` (solo si el nombre cambió) y `DuplicateProduct`.
  Sin candado, dos tabletas que guardan «café frío» en dos productos a la vez leen las dos «no
  existe» y quedan las dos.
- **Orden**: el candado de nombres va primero en la transacción, antes de cualquier fila tomada
  (como `LockCompositionEdits`): así no se cruza con quien ya tiene la fila del producto.

## R4. Qué choca con qué

- Un alternativo de X contra: el nombre de cualquier producto (X incluido) y los alternativos de
  cualquier otro producto, activos o no. Dentro de la lista, dos iguales. Igual en artículos.
- El nombre principal al crear o renombrar contra los alternativos de los demás (FR-005).
- **Inactivos y archivados cuentan**: un producto inactivo o un artículo archivado se reactiva con
  su nombre; si sus alternativos se liberaran al archivar, reactivarlo crearía un choque que nadie
  decidió. El aviso dice «archivado» para que se sepa dónde ir a quitarlo; la ficha de un archivado
  deja editar sus alternativos.
- **Una sola consulta** por catálogo devuelve el primer choque: `unnest` de los nombres propuestos
  contra `catalog_name_key(name)` y `unnest(alt_names)` de los demás. Productos e ingredientes son
  catálogos distintos: un artículo «Leche» y un producto «Leche» no chocan.
- `products.system_kind` (el genérico de plataforma) queda fuera de la búsqueda y del choque: no es
  catálogo y su ruta de alternativos responde 404, igual que su edición.

## R5. Escribir los alternativos: ruta propia, lista completa

- **Decisión**: `PUT /admin/products/{id}/alt-names` (permiso `menu.configure`) y
  `PUT /stock/articles/{id}/alt-names` (permiso `inventory.configure`), con la lista completa.
- **Por qué**:
  - **FR-009**: una tableta con la pantalla vieja manda el `PATCH` de siempre sin el campo; con una
    ruta aparte, el `PATCH` no puede borrar lo que no conoce.
  - **Permiso, no rol** (constitución, roles híbridos): `menu.configure` lo tienen los roles que hoy
    editan un producto (admin y gerente); `inventory.configure` es el que ya protege el `PATCH` del
    artículo. Ningún permiso nuevo (decisión del dueño).
  - La ficha del artículo se autoguarda campo por campo; la del producto guarda con un botón. Una
    ruta por lista sirve a las dos.
- **Bitácora**: en artículos, el cambio queda en el evento `ingredient.updated` que ya produce la
  edición de la ficha (`before/after.altNames`), en la misma transacción. En productos no hay
  evento de nombre hoy y no se agrega.

## R6. Buscar: servidor donde ya se busca en el servidor; cliente donde ya se filtra en el cliente

| Buscador | Hoy | Cambio |
| --- | --- | --- |
| Vender | `buscarProductos` sobre `GET /pos/menu` | El menú trae `altNames`; tercer balde entre nombre y código |
| Productos | `AdminListProducts`, `name ilike` | `strpos(catalog_name_key)` sobre nombre y alternativos; devuelve `matchedAlt` |
| Artículos | `ListArticles`, `strpos(translate(lower))` | Suma alternativos; devuelve `matchedAlt` |
| Conteo | `matchesWords` sobre `GET /stock/counts/current` | Los renglones traen `altNames`; mismo `matchesWords` |
| Gastos | `Picker` sobre `GET /articles` (`SearchArticles`) | Trae `altNames`; `PickerOption.aliases` |
| Recetas | `SearchSheet` sobre `GET /ingredients` y `/admin/products` | Traen `altNames`; `aliases` en la opción |

- El `Picker` y el `SearchSheet` ganan `aliases?: string[]` en la opción: filtran por etiqueta o
  alias y, si coincidió solo un alias, lo muestran debajo de la etiqueta. Así cualquier otra hoja
  que use el componente (merma, «llegó algo más») puede sumarse pasando los alias.
- Una sola función decide el alias que coincidió: `matchedAlias(alts, query, match)` en
  `utils/format.ts`, que recibe la regla de coincidencia de cada buscador.
- `matched_alt` en SQL va con `coalesce(…, '')`: la subconsulta sobre un arreglo vacío devuelve
  NULL, y un cast a `text` haría que sqlc la tipe no nulable (AGENTS §1, el 500 de
  `/catalogo/opciones`).

## R7. Ticket, comanda, reportes y plataformas

- El renglón del pedido copia `products.name` al venderse (`order_lines.name`); la comanda y el
  ticket leen esa copia. Reportes y plataformas leen `name`. Ninguna de esas lecturas cambia; una
  prueba de integración vende un producto con alternativos y verifica que el renglón lleva el nombre
  principal, y otra que el menú de plataforma no los manda.

## R8. Convivir con la API anterior durante la subida

- La 0107 solo agrega: dos columnas con default constante (en Postgres ≥ 11 no reescribe la tabla),
  dos `check` que cualquier insert viejo cumple (no manda la columna → `{}`) y una función.
- La API vieja crea, edita y duplica productos y artículos sin tocar `alt_names`.
- Lo que la API vieja no valida durante esos segundos: crear o renombrar un producto o artículo con
  un nombre que ya es alternativo de otro. Se acepta: no rompe nada, solo deja dos que se encuentran
  con la misma búsqueda, y el siguiente guardado de alternativos lo detecta.
- Down: quita columnas y función. Pierde los alternativos; el Up los vuelve a aceptar, así que no
  necesita guarda.

## R9. Duplicar, unir y cambiar unidad

- `CloneProductRow` nombra sus columnas: la copia nace con `{}` sin cambiar nada (FR-016).
- Unir insumos no mueve alternativos (fuera de alcance; el origen archivado los conserva).
- Cambio de unidad por reemplazo (`replaceArticle`): lee los alternativos del viejo, los vacía y se
  los pone al nuevo, en la misma transacción y bajo el candado 28037 que ya toma (FR-019).

## R10. Puerta VIII

¿Se puede agregar después al mismo costo? Sí: los alternativos no registran un hecho que se pierda
si no se guarda hoy. Ninguna puerta de la tabla se cierra.

La puerta «una lista de productos por plataforma, con nombres propios» se roza y queda abierta: los
alternativos son **locales** (para buscar en el POS), no nombres de plataforma. Un nombre propio en
Uber será otra tabla por `(producto, plataforma)` y no choca con éstos.

## R11. Aviso de parecidos (pedido del dueño al elegir la maqueta)

- **Decisión**: `GET /admin/products/{id}/alt-names/similar?name=` y
  `GET /stock/articles/{id}/alt-names/similar?name=`, con el permiso de editar. Devuelven hasta 5
  `{id, name, viaAlt?}` con `domain.SimilarNames` (la regla de «parecidos» de unir insumos, spec
  056) contra el nombre principal y cada alternativo de los demás del catálogo, sin el propio. Un
  idéntico (llave igual) se deja fuera: ése lo responde el bloqueo al guardar.
- **Por qué una ruta nueva y no `/stock/articles/similar`**: aquélla compara solo nombres
  principales y alimenta la unión de duplicados; sumarle alternativos cambiaría qué grupos ve la
  pantalla de duplicados, que no es de esta feature.
- **Por qué no bloquea**: «parecido» es heurístico; «Café frío» y «Café frío de olla» pueden ser
  dos productos de verdad (decisión del dueño, 2026-11-03).

## R12. Unir insumos: el origen no reserva nombres

- **Escenario** (revisión de arquitectura): se une «Leche Lala» en «Leche» y luego se quiere que
  «Leche Lala» sea alternativo de «Leche» para que se siga encontrando. Con «los archivados reservan»,
  el origen archivado bloquea justo el caso de uso natural de unir.
- **Decisión**: un artículo que es **origen de una unión** (`ingredient_replacements.kind = 'merge'`)
  no cuenta en el choque de alternativos, ni con su nombre ni con sus alternativos. No puede volver:
  nada nuevo cuelga de él (spec 056). Un archivado normal sí reserva, porque se reactiva.
- El alta de artículos (`FindArticleByName`) no cambia en esto: es de la 056.

## R13. Quién escribe nombres sin pasar por el candado

| Camino | Toma el candado | Por qué |
| --- | --- | --- |
| `CreateProduct`, `UpdateProduct` (nombre cambiado), `DuplicateProduct`, `SetProductAltNames` | 28063 | Los caminos de pantalla |
| `createArticleTx` (lista, gasto, receta), `UpdateArticle` (nombre cambiado), `replaceArticle`, `SetArticleAltNames` | 28037 | Ya lo tomaban, salvo el último |
| `cmd/fudo-import`, scripts de `docs/`/`scripts/`, `ensure_platform_unpaired_product`, `CreateCompany` | No | Cargan catálogos sin alternativos; el genérico de plataforma no lleva |

Si uno de los exentos deja un nombre igual a un alternativo, el siguiente guardado de alternativos de
**ese** producto lo encuentra y el aviso nombra con quién: sigue siendo accionable.

## R14. Revisión de pantalla, decisiones

- **Texto escrito sin «Agregar»**: «Guardar» de la ficha del producto y «Volver» de la del artículo
  lo agregan primero; si choca, la ficha no se cierra y el choque se ve bajo el campo.
- **El aviso de parecidos va antes de agregar**: «Agregar» espera la consulta de parecidos de ese
  texto; si hay, el primer toque muestra el aviso (color de advertencia, con texto) y el botón pasa a
  «Agregar de todos modos». Un toque más solo cuando hay parecidos; no bloquea.
- **Alto acotado**: el editor va después de Categoría; los chips en una sola línea con scroll
  horizontal; la nota «no salen en el ticket ni en la comanda» va en el `InfoButton` del título; el
  campo hace `scrollIntoView` al enfocarse.
- **«×» de 44×44** con `aria-label`, 8 px entre chips.
- **En el artículo, agregar y quitar pasan por el mismo autoguardado y su `SaveIndicator`**; si el
  guardado falla, la lista vuelve a la guardada. Sin «Deshacer»: volver a agregar cuesta lo mismo que
  hoy corregir cualquier campo de la ficha.
- **Etiqueta común** `AltMatchTag` (`Badge` outline gris, con «también:» delante) y chip común
  `AltNameChip`, en `components/`, con tokens del tema.
- **Tile de Vender**: la etiqueta ocupa la línea de la descripción mientras el resultado salió por
  alternativo; el tile no crece. Junto al nombre en las filas (Productos, Artículos, Conteo, `Picker`,
  `SearchSheet`), truncada (`maxW` 45%) y con el nombre con prioridad; en Conteo, en la línea de
  empaques si no cabe.
- El choque se muestra con el mensaje del servidor bajo el campo, nunca en un toast con `String(e)`.

## R15. Revisión de código (2026-11-03), lo que cambió del diseño

- **Esquema del menú `v4`** (`internal/cache/menu.go`): la 0107 no sube la versión del catálogo, y con
  `v3` el menú guardado por la API anterior se servía hasta 24 h sin los otros nombres.
- **Candado de nombres antes que la fila también en cambiar unidad y reactivar**: guardar alternativos
  toma el candado y luego la fila; esos dos lo hacían al revés y se podían esperar uno al otro.
- **`for no key update`** al leer el producto o artículo: no frena la llave foránea de una venta.
- **El buscador del gasto filtra por otro nombre también en el servidor** (`SearchArticles`).
- **`catalog_name_key` junta todos los espacios Unicode que junta `strings.Fields`**, y un otro
  nombre con caracteres que no se ven (NUL, controles, ancho cero) es 400.
- **El PUT del artículo exige la versión de la ficha** (STALE si cambió).
- **Front**: el producto que se renombra y conserva el nombre viejo como otro nombre guarda la lista
  después del PATCH; crear un artículo con un otro nombre existente usa ése; lo escrito sin «Agregar»
  se guarda también al ir al conteo.
- **Residual aceptado**: un acento escrito como letra más acento combinado (dos caracteres) no se
  iguala con el acento de un solo carácter; los teclados de tableta escriben el de un carácter.
