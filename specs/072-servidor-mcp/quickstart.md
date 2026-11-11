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

   **No pongas las herramientas que escriben en `permissions.allow` de Claude Code**: el agente lee
   texto que escribe gente de fuera (proveedores, plataformas) y cada escritura debe pasar por tu
   aprobación. Para solo consultar, usa un token sin alcances de escritura.
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

## Resultado de la corrida del 2026-11-10 (local)

- Cliente MCP del SDK oficial contra `http://localhost:8072/mcp`: lista 30 herramientas para un
  token con gastos y catálogo (sin las de caja ni almacén); gasto con tarjeta, gasto en efectivo
  (con caja cerrada: rechazo con la corrección `include_in_count: false`; con ella, aplicado) y cambio
  de precio, cada uno con su `mcp.tool_called` y el evento del negocio con `device.agent = "mcp"` y la
  IP del cliente. Token revocado → `initialize` responde 401.
- Claude Code: `claude mcp add --transport http … --header 'Authorization: Bearer ${POS_MCP_TOKEN}'`
  guarda la referencia y no el valor; con la variable, `claude mcp list` → Connected; sin ella, 401 y
  aviso de variable faltante. Probado con `--scope local` en una carpeta de prueba (mismo archivo que
  `--scope user`); `--scope user` no se corrió para no tocar la configuración del dueño.
- Pantalla a 1024×600: 4 de 4 alcances, contraseña y «Crear token» a la vista sin desplazar. Con
  teclado simulado (alto 340 px) «Crear token» sigue a la vista; la contraseña queda bajo el teclado
  y la hoja la desplaza al enfocarla.
