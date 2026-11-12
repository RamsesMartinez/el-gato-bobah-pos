# Research: Desactivar y reactivar insumos desde el servidor MCP

Sin incógnitas abiertas: las rutas existen y su comportamiento se leyó en el código el 2026-11-12.

## R1 — Qué rutas envuelve `active`

- **Decisión**: `active: false` → `POST /stock/articles/{id}/archive` con `{confirmOnHand}`;
  `active: true` → `POST /stock/articles/{id}/reactivate`. Las dos responden 204. La vista previa de
  desactivar lee `GET /stock/articles/{id}/archive-check` (`onHand` por sucursal, `activeRecipes`
  con `kind/id/name`, `openCount`, `pendingReceipts`), la misma que pinta la hoja «Archivar».
- **Por qué**: son las acciones de la hoja; `BackofficeService.ArchiveArticle` ya rechaza receta
  activa y conteo abierto (409 `IN_USE`) y existencia sin confirmar (409 `ARCHIVE_HAS_STOCK`), y
  `ReactivateArticle` el nombre tomado (`NAME_TAKEN`) y el insumo unido (`INGREDIENT_MERGED`).
- **Alternativas**: escribir `is_active` por el PATCH de la ficha (se saltaría las reglas: prohibido
  por el issue).

## R2 — Dónde vive la acción en la herramienta

- **Decisión**: `route` devuelve el POST según `active` y su cuerpo; no hace falta `run` (es un solo
  despacho). `state` lee la ficha (con `isActive`) para el antes y el después. `preview` lee la
  ficha y, para desactivar, la revisión.
- **Por qué**: `active` va solo (spec D3), así que no hay dos pasos que encadenar.

## R3 — La ficha de un archivado se lee

- **Decisión**: `GET /stock/articles/{id}` devuelve la ficha también de un archivado (la pantalla la
  abre para reactivar), con `isActive`. Se agrega `isActive` a lo que la vista previa y la bitácora
  muestran de la ficha.

## R4 — `articles_search` con inactivos

- **Decisión**: `readTool` propio: lee `GET /articles?q=` como hoy y marca cada renglón `active:
  true`. Con `include_inactive: true` lee además `GET /stock/articles?status=archived&q=` (la lista
  Almacén › Archivados, que busca por nombre, otro nombre y marca de empaque) y agrega cada uno como
  `{item_type: "ingrediente", id, name, unit_code, alt_names, on_hand, active: false}` sin repetir
  id. Si esa lectura se niega (rol sin ver el almacén), lo dice en `notes` y devuelve los activos.
- **Por qué**: el buscador de compras no ofrece archivados a propósito; cambiar su consulta los
  sacaría en la captura de compras de la pantalla.
- **Alternativa**: parámetro `status` en `/articles` (cambia la ruta de la captura para nada).

## R5 — Los dos huecos del archivado

- **Sugerencias aprendidas**: `LookupSupplierItem` agrega `and (si.ingredient_id is null or
  i.is_active)`; un aprendido que apunta a un archivado deja de encontrarse y el renglón cae a las
  sugerencias por parecido, como uno sin aprender. `SuggestFromSupplierItems`, igual. Los productos
  no se filtran (desactivar productos no es de esta feature). El renglón aprendido no se borra:
  reactivar el insumo lo vuelve a proponer.
- **Existencias**: `ListStockLevels` agrega `and (sl.ingredient_id is null or i.is_active)` y la
  columna `coalesce(i.is_active, p.is_active, true) as is_active`.
- **Alternativa**: filtrar en Go en `SuggestForLine` (dos lugares en vez de la consulta).

## R6 — Alcance y tokens emitidos

- **Decisión**: `write:inventory`, sin migración; ensancha los tokens con ese alcance (spec D5). Se
  mide en el respaldo restaurado con `whoami` (quickstart).
