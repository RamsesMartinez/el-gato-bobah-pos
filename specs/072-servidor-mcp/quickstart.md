# Quickstart — validación de punta a punta (local)

Prerrequisitos: `make start` del worktree arriba (API en `:8072`, web en `:3072`), base restaurada
(`make db-restaurar`), un usuario admin.

1. **Crear el token**: en `http://localhost:3072/integraciones`, «Crear token», alcances
   gastos y catálogo, 30 días. Copiar el token que se muestra una vez.
2. **Conectar Claude Code**:

   ```bash
   export POS_MCP_TOKEN='egbpat_…'
   claude mcp add --transport http --scope user pos-local http://localhost:8072/mcp \
     --header 'Authorization: Bearer ${POS_MCP_TOKEN}'
   claude mcp list    # pos-local: ✓ Connected
   ```

3. **Criterio de aceptación (SC-001)**, desde la sesión de Claude o con un cliente MCP de prueba:
   - `expense_create` con tarjeta: vista previa, después `dry_run: false` con una clave.
   - `expense_create` en efectivo de una caja con turno abierto: el turno muestra la salida.
   - `product_update` con un precio nuevo: la pantalla de productos y el menú del POS lo muestran.
   - `audit_list` (o la pantalla Bitácora): un `mcp.tool_called` por llamada y los eventos de
     gasto y precio con el agente en el aparato.
4. **Negativos (SC-002)**: token solo `read` → `expense_create` con `dry_run: false` niega por
   alcance; revocar en la pantalla → la siguiente llamada da 401; token de la empresa 1 → ve solo
   lo suyo.
5. **Idempotencia (SC-003)**: repetir 10 veces la misma aplicación con la misma clave → un gasto.
6. Limpiar: anular los gastos de prueba y regresar el precio.

Resultado esperado: todos los pasos pasan sin tocar la base a mano.
