# Research: 074 — empaques y edición de compras por MCP

## R1. La vista previa de `expense_edit` es la de la ruta, no la del marco de escrituras

- **Decision**: con `dry_run` ausente o `true`, la función `preview` de la herramienta despacha
  `POST /expenses/{id}/edit?preview=1` con el mismo cuerpo que se aplicaría y devuelve su
  `preview` completo (encabezado, renglones, existencias, costos, dinero, decisiones y huella). Al
  aplicar, `apply` despacha la misma ruta sin la consulta, con `fingerprint` y `decisions`.
- **Rationale**: la ruta ya corre la edición completa en una transacción revertida (spec 053, R1):
  lo que se previsualiza es por construcción lo que se confirma. Calcularlo en la capa MCP sería la
  segunda copia de la regla.
- **Alternatives**: una vista previa propia leyendo el gasto y estimando — descartada por lo mismo.

## R2. Huella e idempotencia

- **Decision**: aplicar exige `fingerprint` no vacío (error de argumento antes de despachar: «pide
  primero la vista previa»). La huella vencida (409 `EXPENSE_EDIT_STALE`) y la decisión faltante
  (400 `DECISION_REQUIRED`) caen en la rama 4xx de `apply`, que libera la clave. Las dos entran a
  `toolFixByCode` con la corrección en palabras.
- **Rationale**: la huella cubre «el estado cambió»; la clave, «el reintento duplicaría». Liberar la
  clave tras un 4xx deja que el agente repita con la huella nueva y la misma clave. La huella entra
  a la huella de argumentos de la clave: la misma clave con otra huella es «otra escritura» → 409
  de idempotencia, que es correcto (pide clave nueva).
- **Alternatives**: que la herramienta pida la vista previa sola y aplique con esa huella — descartada:
  aplicaría sin que la persona viera los efectos, que es justo lo que el protocolo existe para evitar.

## R3. «Cambiar un renglón» = quitar y agregar en la misma llamada

- **Decision**: argumentos `remove` (ids de renglón) y `add` (renglones nuevos, con la forma de
  `purchase_create.lines` más `arrived_on`). Sin verbo propio.
- **Rationale**: la ruta y la pantalla no tienen otro. Una sola transacción: la vista previa muestra el
  efecto neto. Quitar un renglón recibido revierte sus entregas con su fecha; agregar uno con
  `arrived_on` del mismo día lo vuelve a sumar, así que existencia y costo quedan como si se hubiera
  capturado bien (lo verifica el caso de aceptación contra una compra de referencia).
- **Riesgo**: un renglón agregado sin `arrived_on` queda por recibir (así es la ruta). La vista previa
  lo dice en una nota por renglón.

## R4. `presentation_update` parte del empaque actual

- **Decision**: la herramienta lee `GET /stock/presentations` (el empaque por id) y arma el cuerpo
  completo de la pantalla: nombre, contenido, unidad, aproximado y activo del registro, reemplazando
  solo lo que el agente manda. La marca se manda solo si el agente la manda (la ruta conserva la que
  hay si no viene; vacía la quita). `qty` y `unit_id` van juntos o ninguno.
- **Contenido sin cambiar**: un empaque normal guarda su contenido en la unidad base (`qtyInBase`), así
  que se manda `qty = qtyInBase` con la unidad base del insumo; su id sale de `GET /units` buscando el
  código de la unidad base con equivalencia 1. Un aproximado guarda su contenido tal cual y su
  unidad de compra (`purchaseUnitId`): se mandan esos dos.
- **Rationale**: es el patrón de `product_update` (072). Mandar campos vacíos por omisión borraría
  datos que nadie pidió cambiar.
- **Alternatives**: exigir todos los campos — descartada: obliga al agente a releer y reescribir,
  y un error de transcripción cambia el contenido, que mueve el almacén de las compras futuras.

## R5. Sugerencia de empaque en la vista previa de compra

- **Decision**: en cada renglón con insumo y sin empaque, `presentations` lista los activos del insumo
  **con su marca** y `create_presentation` trae `{tool: "presentation_create", args: {ingredient_id,
  name: <texto del ticket>, unit_id: <unidad base del insumo>}, ask: ["brand", "qty"]}`. Si el renglón
  trae `unit_id`, una nota dice que entrará en esa unidad sin marca ni empaque.
- **Unidad base**: de `GET /stock/articles/{id}` (`unitCode`) y `GET /units`; si alguna no se puede
  leer, la sugerencia va sin `unit_id` y lo pide.
- **Rationale**: lo que el servidor sabe va lleno; lo que solo sabe la persona (marca, contenido) se
  pregunta (D4). Parsear «375G» del ticket sería una regla nueva y podría equivocarse.

## R6. Alcances y el token existente

- **Decision**: `write:inventory` (empaques) y `write:expenses` (edición); ninguno nuevo.
- **Medido** en el respaldo del 2026-11-11 restaurado en local: el único token vivo («Token Gestion
  Claude», del administrador de la empresa del negocio) tiene los siete alcances. `whoami` contra la
  API local lo confirma después de implementar (quickstart, paso 1).
