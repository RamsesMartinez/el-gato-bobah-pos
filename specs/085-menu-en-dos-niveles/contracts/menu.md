# Contrato del menú (UI)

- `visibleModules(user, { attendanceEnabled }) → VisibleModule[]`: puro; mismo orden que la spec; nunca devuelve un módulo sin destino abrible.
- `moduleOf(pathname) → { moduleId, itemTo } | null`: por prefijo; `/catalogo/*` → Menú › Menú, `/plataformas/*` → Menú › Plataformas, `/caja/*` → Caja › Caja, `/apariencia` → Ajustes › Interfaz.
- Cada botón de módulo: `role=button` o enlace con nombre accesible igual a su rótulo; `aria-haspopup`/`aria-expanded` cuando abre panel; el panel es `role=menu` con renglones `menuitem`; Escape cierra y devuelve el foco al botón.
- Rutas: ninguna nueva, ninguna renombrada. `POST /api/v1/usage` acepta `apariencia` e `integraciones`.
