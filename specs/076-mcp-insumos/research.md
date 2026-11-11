# Research: Crear y editar insumos desde el servidor MCP

Sin incógnitas abiertas: las rutas existen y su comportamiento se leyó en el código el 2026-11-11.

## R1 — Qué búsqueda de parecidos usa la vista previa del alta

- **Decisión**: dos lecturas, en este orden: `GET /stock/articles/similar?name=` (la que usa la hoja
  «Nuevo artículo» al tocar Guardar: nombres parecidos por distancia, hasta 5, activos) y
  `GET /stock/articles?q=` (la búsqueda de la lista de Almacén: contiene el texto en el nombre **o en
  un otro nombre**, sin acentos). Se juntan sin repetir el mismo id; el orden es el de la primera y
  después lo que solo halló la segunda.
- **Por qué**: la de parecidos no lee otros nombres (`MergeCandidates` solo trae `name`), y el caso
  «ya existe con otro nombre» es justo un otro nombre. La pantalla cubre ese caso con el 409 al
  guardar; el agente lo vería hasta aplicar.
- **Alternativas**: `GET /articles?q=` de la captura de gastos (pide `expenses.capture`, permiso
  ajeno a la herramienta); comparar nombres en la capa MCP (regla copiada, prohibido).

## R2 — La edición son dos peticiones

- **Decisión**: `run` de `writeSpec` (ya existe para el lote del conteo): lee la ficha, despacha el
  PATCH si hay algún campo de la ficha, y con la versión que devuelve el PATCH despacha el PUT de
  otros nombres si se mandaron. Si el PUT falla con 4xx, el mensaje se prefija con «El nombre y la
  ficha ya quedaron cambiados» y se conservan el código y los detalles de la ruta.
- **Por qué**: la ficha guarda en dos rutas distintas a propósito (063: la lista viaja aparte para que
  una tableta vieja no la borre). Una ruta nueva que hiciera las dos en una transacción sería API
  nueva para un caso que la pantalla resuelve igual en dos pasos.
- **Alternativas**: dos herramientas (`ingredient_update` y `ingredient_alt_names_set`): el caso del
  issue exige las dos en orden y el agente podría invertirlo.

## R3 — Versión de la ficha (`expectedUpdatedAt`)

- **Decisión**: la herramienta no la pide al agente: la lee de `GET /stock/articles/{id}` en la misma
  llamada, justo antes de escribir.
- **Por qué**: protege contra la tableta que guardó en medio de la llamada (409 `STALE`), que es lo
  único que la versión puede detectar aquí; pedirla al agente agregaría un paso sin proteger nada
  más, porque la vista previa no se guarda.
- **Alternativa**: huella como la de `expense_edit`: esa ruta la exige; ésta no.

## R4 — La pista de un nombre repetido pierde el id

- **Decisión**: `toolHint` lee también `details.existingId` y `details.archived` del sobre de error y
  los agrega a la pista («ya existe: id 749, archivado»), con corrección en palabras para
  `NAME_TAKEN`, `ALT_NAME_TAKEN` y `STALE`.
- **Por qué**: la pantalla ofrece «Abrir ese» o «Reactivar» con ese id; el agente solo ve el mensaje,
  que hoy dice «ya existe» sin cuál.

## R5 — `presentations_list` con filtro

- **Decisión**: `readTool` propio en `mcp_tools_076.go`: con `ingredient_id > 0` lee
  `GET /stock/ingredients/{id}/presentations`; sin él, `GET /stock/presentations` como hoy. Un
  `ingredient_id` presente y no positivo se rechaza (constitución V: no cae al default).
- **Por qué**: es la ruta que usa la ficha y ya tiene el mismo permiso (`inventory.receive`).

## R6 — Alcance y tokens emitidos

- **Decisión**: `write:inventory`, sin migración.
- **Medido**: respaldo de producción del 2026-11-11 03:20: un solo token en `gatobobah`, «Token
  Gestion Claude», de `admin`, con `read, write:expenses, write:cash, write:inventory, write:catalog,
  write:purchases, write:orders`, vigente hasta 2027-02-09. Cubre las dos escrituras sin regenerarlo.
