# Research: Escrituras del servidor MCP

## R1. ¿Despachar a la ruta o llamar al servicio?

- **Decision**: despachar a la ruta, como toda la 072.
- **Rationale**: cancelar avisa a las tabletas desde el handler; abrir exige `cash.adjust_count`
  según el cuerpo dentro del handler; cerrar lee los permisos en el handler. Llamar al servicio
  obligaría a copiar cada uno.
- **Alternatives**: servicio directo — rechazado por la decisión 1 de la 072.

## R2. ¿Cómo sabe la vista previa cuánto cambia el costo vigente?

- **Decision**: ruta nueva `POST /expenses/purchase-preview` con el mismo cuerpo que `POST /expenses`.
  El servicio corre la captura (`createExpenseTx`, extraída de `CreateExpense` sin cambiar su
  comportamiento) en una transacción, lee el costo de cada insumo antes y después **dentro** de ella,
  y la revierte con un error centinela. Devuelve por renglón: insumo, cantidad en unidad base, costo
  unitario de la compra, costo antes y después, y si el costo no cambió porque llegó una compra más
  reciente.
- **Rationale**: es la misma regla (método de costeo, compras tardías, conversión con densidad y
  empaque, regalos) y las mismas validaciones (insumo unido, empaque de otro insumo, pago fuera de
  arqueo). El patrón ya existe dos veces con el mismo centinela `errPreviewRollback`: la vista previa
  de editar un gasto (`app/expense_edit.go`) y la de unir insumos (`app/ingredient_merge.go`).
- **Costo**: la vista previa toma los insumos `nowait` igual que la captura; con una venta tomándolos
  responde «vuelve a intentar». Consume valores de secuencia (ids) que no se usan: inofensivo.
- **Alternatives**: (a) calcular en la capa MCP con `domain.NextIngredientCost` leyendo costo y
  existencia por separado — rechazado: duplica conversión y regla de compras tardías; (b) un
  `?preview=1` en `POST /expenses` — rechazado: mezcla en la ruta de la pantalla un modo que la
  pantalla no usa.

## R3. Candidatos de insumo

- **Decision**: la vista previa despacha `GET /articles/suggest?supplierId&rawName` por cada renglón
  sin insumo (la cascada que ya usa la captura con ticket: lo aprendido por proveedor, luego por
  nombre) y `GET /stock/presentations` para los empaques del insumo propuesto o elegido (si la
  persona no tiene «Recibir compras», se omite con su nota, como en la pantalla).
- **Rationale**: es lo mismo que ve quien captura en la tableta.

## R4. Arqueo ciego

- **Decision**: D4 del spec. El ocultamiento ya vive en `vistaDelTurnoAbierto` (servicio) y se
  hereda por la ruta. La capa MCP agrega solo la exigencia de cifras explícitas.
- **Medido**: el negocio tiene el arqueo ciego encendido (respaldo del 2026-11-10).
- **Alternatives**: (a) ocultar el esperado por MCP siempre, aun con el arqueo ciego apagado —
  rechazado: quitaría al dueño una lectura que la pantalla le da y no protege nada que el negocio
  haya pedido proteger; (b) negar el cierre a un token que leyó el resumen de ventas del día —
  rechazado: no se puede probar que un agente no lo vio por otro lado, y la persona con ese permiso
  lo ve en la pantalla.

## R5. Qué hay que declarar al cerrar

- **Decision**: `domain.MissingCloseDeclarations(req, given)`. `req` sale del turno filtrado: si el
  cajón pide conteo (`drawer.requiresCount`), los métodos que no se autodeclaran y no son del cajón
  (`totals[].autoDeclare == false`, fuera de `drawer.methodIds`). `given`: `counts` o
  `counted_cash`, y las llaves de `declared`. Las terminales las sigue exigiendo el servidor
  (`TERMINAL_COUNT_REQUIRED`), con su corrección en palabras.
- **Rationale**: el servidor cierra un método no declarado con 0 (`ResolveDeclared`), que es
  correcto en la pantalla (la hoja no deja enviar sin capturar) y peligroso con un agente.

## R6. Lote de conteo

- **Decision**: hasta 50 renglones; cada uno `PUT` con `confirmable: true`. Al primer error se
  detiene y responde qué quedó y cuál falló. `NEEDS_CONFIRM` → «confirma la cifra del renglón N con
  `confirmed: true`».
- **Rationale**: ver D6 del spec.

## R7. Alcances

- **Decision**: D1 del spec. Migración 0112 amplía el check (cardinalidad hasta 7).
