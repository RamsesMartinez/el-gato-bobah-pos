# Contrato: ruta nueva

## `POST /api/v1/expenses/purchase-preview`

- **Permiso**: `expenses.capture` (el mismo que `POST /expenses`).
- **Cuerpo**: el mismo de `POST /expenses` (`categoryId`, `amount`, `status`, `items`, `payments`,
  `receivedAt`, …).
- **Efecto**: ninguno. Corre la captura en una transacción que se revierte.
- **200**: `{ "costingMethod": "last" | "weighted", "lines": [ { "position", "ingredientId",
  "productId", "name", "baseQty", "baseUnitCode", "unitCost" } ], "costs": [ { "ingredientId",
  "name", "costBefore", "costAfter", "keptByNewerPurchase" } ] }` — solo renglones de almacén; el
  costo es **por insumo** (antes de la compra y después de la compra completa); los dos arreglos
  siempre presentes, aunque vacíos.
- **400** además: un renglón con `newArticle` (la vista previa no da de alta artículos).
- La transacción corre con `lock_timeout` de 2 s y fuerza los constraints diferidos antes de
  revertirse, para que un insumo unido falle aquí igual que al guardar.
- **Errores**: los mismos que `POST /expenses` con el mismo cuerpo (400 validación, 404 artículo de
  otra empresa, 409 insumo ocupado o unido, …).
