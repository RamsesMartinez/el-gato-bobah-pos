# Data Model: Desactivar y reactivar insumos desde el servidor MCP

Nada nuevo en la base. Lo que tocan las herramientas ya existe:

| Entidad | Qué se lee o escribe | Dueña de las reglas |
| --- | --- | --- |
| Insumo (`ingredients.is_active`) | Se archiva o reactiva por las rutas de la hoja «Archivar» | `BackofficeService.ArchiveArticle`, `ReactivateArticle` (spec 042, #203) |
| Existencia por sucursal (`stock_levels`) | Solo lectura; archivar no la ajusta. La lista de existencias omite insumos archivados | `ListStockLevels` |
| Renglón aprendido del proveedor (`supplier_items`) | Sin cambio de datos; no se propone mientras su insumo esté archivado | `LookupSupplierItem`, `SuggestFromSupplierItems` |
| Llamada MCP (`audit_events`, `mcp.tool_called`) e idempotencia | Una por llamada / por clave | Spec 072 |

Transiciones: activo → archivado (sin receta activa, sin conteo abierto que lo capturó, y sin
existencia o confirmándola); archivado → activo (sin otro activo con su nombre y sin estar unido).
