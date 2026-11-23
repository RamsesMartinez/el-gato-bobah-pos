# Quickstart: validar la 090

## Preparación

```bash
make db-restaurar            # respaldo de producción más reciente, con dueños y GRANT
make start                   # API :8090, web :3090 (worktree 090)
```

Crear en la pantalla (Integraciones) un token con todos los alcances para un usuario admin de la
empresa `gatobobah` y exportarlo como `TOKEN`. Las llamadas van a `POST http://localhost:8090/mcp`
con `Authorization: Bearer $TOKEN` (cliente MCP o `curl` con JSON-RPC `tools/call`).

## Recorrido (cada pedido de prueba se entrega y se cobra)

1. **Defectos**: `modifier_option_update` con solo `name` sobre un extra con precio → el precio
   queda; sobre una opción de un grupo inactivo → la encuentra y avisa del grupo. En la pantalla de
   productos, un producto con grupo inactivo muestra la insignia.
2. **Pedido de plataforma**: `order_draft_create` (vista previa: total y `price_source` por renglón)
   → aplicar → `order_draft_send` → `order_deliver` → `order_discount_set` → `order_pay` por
   monto con lo que falta y el método de la plataforma (`all_remaining` es del mostrador). El total cobrado = total de la vista previa.
3. **Cuenta descartada**: `order_draft_create` + `order_draft_discard`.
4. **Corrección de cobro**: pedido de mostrador cobrado en efectivo → `order_payment_void` →
   `order_pay` con otro método → `order_payment_terminal_set` (si hay terminal) → `order_refund`
   (vista previa con `refund-info`, luego aplicar).
5. **Costo**: `ingredient_cost_set` con motivo → `cost-history` lo muestra con el motivo.
6. **Receta**: `recipe_get` de un extra con homónimos → `recipe_set` sin `also_in_same_name` → solo
   cambia ése.
7. **Modificadores**: `modifier_group_create` → `modifier_option_create` ×2 → `product_group_link`
   con mínimo 1 → la pantalla de venta lo muestra obligatorio → `product_group_unlink` →
   `modifier_group_update active:false`.
8. **Productos**: `product_create` → `product_update` con solo `name` → lo demás igual.
9. `audit_list` muestra cada llamada aplicada.

Esperado: 0 de 9 pasos necesitan la pantalla (solo se abre para verificar 1 y 7). Pruebas
automatizadas: `make test-integration` (incluye `mcp_090_test.go`), `make api-test`, `make web-test`.
