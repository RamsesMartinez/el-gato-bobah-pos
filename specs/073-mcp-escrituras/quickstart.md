# Quickstart: probar las escrituras con un cliente MCP real

Contra la API local del worktree (`make start`, base restaurada con `make db-restaurar`). Nunca contra
producción.

1. En `http://localhost:30NN/integraciones`, crear un token con los alcances Gastos, Caja, Almacén,
   Compras y Pedidos.
2. `export POS_MCP_TOKEN=egbpat_…` y conectar:
   `claude mcp add --transport http pos-local http://localhost:80NN/mcp --header "Authorization: Bearer ${POS_MCP_TOKEN}"`.
3. Recorrido (cada escritura primero en vista previa, luego `dry_run: false` con clave nueva):
   - **Compra**: `purchase_create` con dos renglones solo con nombre → ver candidatos; repetir con
     `ingredient_id` y `presentation_id` → ver `costBefore`/`costAfter`; aplicar; comprobar con
     `stock_levels` y en la pantalla de Gastos.
   - **Conteo**: `stock_count_open`, `stock_count_capture` con tres renglones,
     `stock_count_close` en vista previa (diferencias), aplicar.
   - **Caja**: `cash_session_close` en vista previa (con el arqueo ciego encendido no trae esperado);
     aplicar sin un método → rechazo que lo nombra; aplicar completo; `cash_session_open` de nuevo.
   - **Pedido**: crear uno en la pantalla, `order_cancel` en vista previa y aplicar.
4. Revisar la bitácora (`audit_list`): una entrada `mcp.tool_called` por llamada y los eventos de
   cada operación con `device.agent = "mcp"`.
5. Cerrar lo que la prueba dejó abierto (pedidos cobrados o cancelados, caja con turno si estaba).

Resultado esperado y observado: se anota en `tasks.md` (texto, sin capturas ni cifras del negocio).
