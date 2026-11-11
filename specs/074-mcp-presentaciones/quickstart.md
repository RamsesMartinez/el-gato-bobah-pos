# Quickstart: 074

Prerrequisitos: `make start` del worktree arriba, respaldo restaurado (`make db-restaurar`), un
cliente MCP contra `http://localhost:80NN/mcp`.

1. **El token de «Gestión Claude» cubre las tres** (sin regenerarlo): con su fila local y un secreto de
   prueba puesto solo en la base local, `whoami` lista `presentation_create`, `presentation_update` y
   `expense_edit`.
2. **Caso de aceptación** (insumo de harina del negocio; filtra por slug, nunca por id):
   1. `purchase_create` aplicado con un renglón de harina en `unit_id` = gramos (375), sin empaque.
      Anota existencia y costo vigente.
   2. `purchase_create` en vista previa con el mismo renglón: trae `create_presentation`.
   3. `presentation_create` (vista previa y aplicar): «Bolsa», 375 g, marca «Betty Crocker».
   4. `expense_edit` vista previa: `remove` el renglón en gramos, `add` el mismo con el empaque nuevo,
      cantidad 1, mismo importe, `arrived_on` = el día de la compra. Leer existencias, costo e importe
      de antes y después.
   5. Aplicar con `fingerprint` y una clave nueva. Comprobar: existencia y costo vigente iguales a los
      del paso 1; importe igual; el renglón vivo con empaque y marca.
3. **Rechazos**: marca repetida (409), marca de 41 letras (400), huella vencida, decisión faltante,
   token sin alcance, rol sin permiso.
4. **Limpieza**: anular la compra de prueba (`expense_cancel`) y desactivar el empaque de prueba.

Resultado esperado de cada paso: el del spec (SC-001 a SC-004).
