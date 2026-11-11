# Data Model: Crear y editar insumos desde el servidor MCP

Nada nuevo en la base. Las entidades que tocan las herramientas ya existen:

| Entidad | Qué se lee o escribe | Dueña de las reglas |
| --- | --- | --- |
| Insumo (`ingredients`) | Alta: nombre, unidad base, categoría, ubicación, mínimo, existencia y costo iniciales. Edición: nombre, categoría, ubicación, mínimo, otros nombres | `BackofficeService.CreateArticle`, `UpdateArticle`, `SetArticleAltNames` (specs 042, 043, 063) |
| Empaque de compra (`ingredient_presentations`) | Solo lectura, filtrada por insumo | `BackofficeService.Presentations` (spec 038) |
| Llamada MCP (`audit_events`, `mcp.tool_called`) e idempotencia (`api_idempotency_keys`) | Una por llamada / por clave | Spec 072 |
