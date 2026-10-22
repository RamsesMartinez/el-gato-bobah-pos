# Research: Gestión de artículos del almacén

## R1 — Qué movimientos impiden el cambio directo de unidad

- **Decision**: bloquean `compra`, `venta`, `merma`, `produccion` y `cancelacion`. No bloquea ningún
  `ajuste`: ni el del conteo (`stock_count_id`), ni la existencia inicial (`reason = 'existencia
  inicial'`), ni el del cambio de modo de la 037.
- **Rationale**: el dueño decidió que los ajustes de conteo no cuentan. Los otros ajustes son de la
  misma naturaleza, porque corrigen cuánto hay y no registran un intercambio con nadie. Con la
  existencia inicial como ajuste, un artículo recién creado puede cambiar de unidad.
- **Alternatives**: solo los de conteo. Así, la existencia inicial bloquearía el cambio desde el
  minuto uno.

## R2 — Dónde vive el historial de costos

- **Decision**: es la unión de dos fuentes, paginada por llave `(fecha, id)` descendente, de 20 en
  20:
  - los movimientos `compra` del artículo, con `expense_item` → empaque, proveedor y fecha;
  - `ingredient_cost_corrections`.
- **Rationale**: cada compra ya guarda su costo en el movimiento, así que no hace falta una tabla
  que duplique ese dato.
- **Alternatives**: una tabla `ingredient_cost_history` escrita en cada compra. Duplica el dato y
  pierde lo anterior a la migración.

## R3 — Aviso del conteo sin romper el conteo a ciegas

- **Decision**:
  - El servidor evalúa `domain.CountSanity` al capturar.
  - Si la cifra cae fuera, responde 200 con `needsConfirm: true` y no guarda.
  - El cliente muestra la cifra con su unidad en grande y reenvía con `confirmed: true`.
- **Rationale**: si el cliente recibiera la referencia o los límites, la pantalla podría deducir lo
  esperado. Así, la referencia no sale del servidor.
- **Alternatives**: mandar `[min, max]` con cada artículo. Filtra lo esperado dentro de un factor 10.

## R4 — Artículo nuevo dentro del gasto y de la receta

- **Decision**:
  - Los renglones de `POST/PUT /expenses` y de `PUT …/composition` aceptan `newArticle: {name,
    baseUnitId}` en vez de `ingredientId`.
  - El servicio lo crea en la misma transacción con `createArticleTx`, la misma función que usa la
    lista.
  - Si el nombre ya existe, se usa el que ya existe, como hoy.
- **Rationale**: es la única forma de que cancelar o fallar no deje el artículo creado.
- **Alternatives**: crear el artículo y borrarlo al cancelar. Falla si se cierra la pestaña o se cae
  la red.

## R5 — Gráfica de tendencia

- **Decision**: un SVG propio. Es una línea con puntos del costo por unidad, el eje con la fecha
  inicial y la final, y la cifra del último punto. Usa los mismos 20 renglones que la lista y mide
  120 px de alto.
- **Rationale**: es una sola gráfica. Una librería pesaría más que la pantalla entera y entraría al
  bundle de la tableta.
- **Alternatives**: recharts o visx.

## R6 — Concurrencia de la ficha

- **Decision**: el `updated_at` de `ingredients` funciona como versión. El cliente lo manda y el
  `update` lo exige.
- **Rationale**: la columna ya existe y no hace falta agregar otra.
