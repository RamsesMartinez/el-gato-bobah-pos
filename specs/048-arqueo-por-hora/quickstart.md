# Quickstart: 048 (API :8048, web :3048, 1024×600)
- Q1 casilla: efectivo + caja con arqueo → casilla marcada; tarjeta o caja sin arqueo → no aparece.
- Q2 pendiente: caja cerrada, gasto $200 → hoja de aviso → «Registrar».
- Q3 apertura: abrir la caja → lista «Gastos sin corte…» con el de $200 → encender «Incluirlos (desde las HH:MM)» → «Abrir caja» → esperado $200 menor.
- Q4 antes del cierre: gasto con fecha de hace una semana → hoja sin «Registrar».
- Q5 permiso: un cajero no ve «Incluirlos (desde…)»; el servidor rechaza `openedAt` con 403.
- Q6 casilla después: desmarcar un pago del corte abierto → el esperado sube; en corte cerrado está deshabilitada.
