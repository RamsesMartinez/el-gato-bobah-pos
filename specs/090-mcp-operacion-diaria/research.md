# Research: MCP para la operación diaria (paquete 1 del #289)

Decisiones técnicas. Las de producto están en [spec.md](spec.md) (Assumptions).

## R1 — Causa del «No existe el extra»

- **Hallazgo**: `modifier_option_update` busca la opción en `GET /admin/modifier-options/?limit=0`, y
  `AdminListModifierOptions` filtra `where mg.is_active`. Una opción activa de un grupo inactivo no
  sale, y la herramienta concluye que no existe. Es el mismo estado del defecto 3 (grupo apagado
  ligado): las dos quejas del 15-nov eran la misma opción.
- **Decisión**: lectura por id, `GET /admin/modifier-options/{id}`, que devuelve la opción con su
  `maxPerLine` y `groupActive` sin filtrar el grupo. La lista no cambia: la pantalla de extras sigue
  mostrando solo los de grupos activos, que es lo que se le pidió.
- **Alternativas**: quitar el filtro de la lista (cambia lo que la pantalla muestra y sus conteos);
  leer de `GET /admin/groups/{id}/options` (necesita el grupo, que es justo lo que no se sabe).

## R2 — Causa del `PATCH` parcial que borra el precio

- **Hallazgo**: `AdminUpdateOption` trata «viene `name`» como «vienen los tres» y rellena los
  ausentes con `0` y `1` (`pd, mpl := decimal.Zero, 1`). El formulario siempre manda los tres, por eso
  no se vio en la pantalla; cualquier otro cliente sí lo pisa.
- **Decisión**: el servicio recibe un parche (`*string`, `*decimal`, `*int`), lee la opción con
  candado de fila y escribe lo que resulta, en una transacción. Un `priceDelta` o `maxPerLine` sin
  `name` también se acepta (hoy se ignoran en silencio: «parámetro presente ignorado»).

## R3 — Vista previa del total de una cuenta

- **Decisión**: `DraftsService.Quote(header, lines)` arma renglones en memoria con la misma forma que
  los guardados y los pasa por `pricedCatalog` + `draftLineView` + la resolución de descuento y envío
  que hoy vive en `buildDraftView`, extraída a `priceDraft`. La cuenta guardada y la cotización usan
  la misma función.
- **Origen del precio**: `domain.PlatformPriceSource(platform bool, manual *decimal) string` →
  `"counter"` (sin plataforma), `"platform"` (precio propio) o `"markup"` (base × (1 + margen)).
  `Quote` lo pone en cada renglón y extra, y devuelve el margen de la plataforma.
- **Ruta**: `POST /pos/drafts/quote`, mismo gate que crear (ninguno). Corre la validación de
  renglones de crear (`validLineCmd`, tope de renglones, `checkLineSells`) para que la vista previa
  rechace lo mismo que crear.
- **Alternativa descartada**: crear la cuenta y descartarla (deja rastro, ocupa un nombre de folio y
  avisa a las tabletas); calcular en la capa MCP con `/pos/menu` (segunda regla de precio).

## R4 — Ids de cuenta en el despachador

- **Hallazgo**: el despachador solo acepta parámetros enteros. La cuenta se identifica con UUID.
- **Decisión**: `dispatch` acepta además parámetros `uuid.UUID` (tipo, no texto): se escriben con
  `String()`, así que el camino solo puede llevar hex y guiones. La herramienta recibe `draft_id`
  como texto y lo lee con `uuid.Parse`; lo que no se lee es error de argumentos.

## R5 — Reenvío de una cuenta o un cobro

- **Decisión**: el `id` de la cuenta, el `opId` de cada renglón y el `clientUuid` del cobro se derivan
  de (empresa, token, clave de idempotencia, herramienta[, renglón]) con SHA-256 y bits de versión 8
  (`domain.DerivedUUID`). Dos llamadas con la misma clave mandan el mismo id: aunque el registro de
  idempotencia fallara, la ruta reconoce el reintento (crear con el mismo id no reaplica; cobrar con
  el mismo `clientUuid` no cobra dos veces).

## R6 — Cobro: vista previa y método

- **Vista previa**: `POST /orders/{id}/quote` con la misma selección (`all_remaining` o `amount`),
  más el pedido y el método resuelto por nombre. No cobra.
- **Cambiar de método**: anular el pago (`order_payment_void`) y cobrar de nuevo (`order_pay`), como
  la pantalla. Sin herramienta propia: no sería atómica y repetiría la regla del cobro.

## R7 — Motivo del costo

- **Decisión**: columna `reason text null` con `check (reason is null or (reason = btrim(reason) and
  char_length(reason) between 1 and 200))` (el servicio recorta antes de insertar) en `ingredient_cost_corrections` (0116). La ruta acepta `reason` opcional; la
  herramienta lo exige. El historial (`cost-history`) lo devuelve y la bitácora lo pone en el después.
- **Respaldo anónimo**: es texto libre que escribe el personal → el paso 5 de
  `respaldo-anonimo.sh` lo reemplaza por `'motivo anonimizado'` donde no es null (vacío violaría el
  check) y `FUGAS` lo cuenta.
- **Down**: quita la columna (no hay dato que la versión anterior vuelva a aceptar ni guarda que se
  niegue).

## R8 — Recetas

- `recipe_get` pasa la vista de la pantalla (`CompositionView`).
- `recipe_set` arma `CompositionRequest`: `items` (insumo, cantidad, unidad), `linked_product_id`,
  `components`, `mode`. `based_on` es el `stamp` que dio `recipe_get` o la vista previa, obligatorio al
  aplicar si la receta ya existía. `also_in_same_name: true` pone `alsoOptionIds` y `alsoBasedOn` con
  los `sameName` leídos en la misma llamada.
- **Existencia propia**: la vista previa trae `ownStock`, `productOnHand` y, por insumo, su
  existencia, y explica qué hay que mandar (`actual_stock`, y de vuelta las dos cifras vistas). El
  rechazo de la ruta (códigos de la 037) llega con su pista en `toolFixByCode`.

## R9 — Ediciones de registro completo

`PATCH /admin/products/{id}` y `PATCH /admin/groups/{id}` reciben el registro completo. La herramienta
lee el actual en la misma llamada y solo cambia lo mandado (como `product_update` hoy). El costo del
producto viaja solo si se pide (`cost` ausente = no tocar).

## R10 — Número de migración

0116 libre al 2026-11-22: revisado con `git ls-tree` en todas las ramas remotas y en `../egb-*/server/migrations`; ninguna la trae.
