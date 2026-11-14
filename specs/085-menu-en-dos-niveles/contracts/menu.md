# Contrato del menú (UI)

- `visibleModules(user, { attendanceEnabled }) → VisibleModule[]`: puro; mismo orden que la spec; nunca devuelve un módulo sin destino abrible.
- `moduleOf(pathname) → { moduleId, itemTo } | null`: por prefijo; `/catalogo/*` → Menú › Menú, `/plataformas/*` → Menú › Plataformas, `/caja/*` → Caja › Caja, `/apariencia` → Ajustes › Interfaz.
- Cada botón de módulo: `role=button` o enlace con nombre accesible igual a su rótulo; `<button aria-expanded aria-controls>` cuando abre panel; el panel es `<nav>` con lista de enlaces (`aria-current="page"` en el abierto), recorrible con Tab; al abrir, foco al primer enlace; Escape o el velo cierran y el foco vuelve al botón; cambiar de ruta lo cierra.
- Rutas: ninguna nueva, ninguna renombrada. `POST /api/v1/usage` acepta `apariencia` e `integraciones`.
