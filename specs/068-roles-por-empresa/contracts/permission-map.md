# Contrato — de rol a permiso

La tabla de equivalencia ruta por ruta vive en [data-model.md](../data-model.md) («Permisos nuevos y
su siembra»). Front:

| Antes | Después |
| --- | --- |
| `ROUTE_ROLES` / `canAccess(role, path)` | `ROUTE_PERMISSIONS` / `canOpen(path, user)` con `can()` |
| `POSPage canEdit` (admin/gerente) | `can('menu.configure')` |
| `POSPage canOpenCash` | `can('cash.operate')` |
| `ModifierSheet canManage` | `can('menu.configure')` |
| `CashPage canManage` | `can('cash.configure')` |
| `PlataformasPage canEdit` (admin) | `can('platforms.credentials')` |
| `OrdersBoardPage puedeAbrirCuenta` (`/pos`, abierto) | sin gate (Vender sigue abierto a toda sesión) |
| `EmployeesPage ROLES` fijo | `GET /roles` activos en un `Picker` |
