# Data Model: Menú lateral en dos niveles

Sin tablas ni columnas nuevas. Las entidades son del front.

## MenuModule
| Campo | Tipo | Nota |
|---|---|---|
| id | `'sell' \| 'orders' \| 'cash' \| 'inventory' \| 'menu' \| 'staff' \| 'reports' \| 'settings'` | estable, en inglés |
| label | string | Vender, Pedidos, Caja, Inventario, Menú, Personal, Reportes, Ajustes |
| icon | componente | |
| items | MenuItem[] | destinos en el orden de la spec; el primero de Caja y Menú es su pantalla propia |

## MenuItem
| Campo | Tipo | Nota |
|---|---|---|
| to | string | ruta que ya existe |
| label | string | el nombre de siempre |
| icon | componente | |
| permission | Permission? | solo los que hoy lo declaran (`/merma`, `/integraciones`) |

## VisibleModule (salida de `visibleModules`)
`{ module, items: MenuItem[] }` con al menos un destino; con uno, tocar navega; con varios, abre el panel.

## Medición
`pantallasMedibles` += `apariencia`, `integraciones`. `permissionByScreen` += `apariencia: ""`, `integraciones: integrations.manage`.
