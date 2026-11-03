# Quickstart: 047

Contra `make start` del worktree (API :8047, web :3047), a 1024×600.

- **Q1 fecha**: con el reloj del navegador en 31-oct 23:30 America/Mexico_City, «Nuevo gasto»
  propone 31-oct en ticket, día del gasto y pago.
- **Q2 caja sin arqueo**: Caja → Cajas, apagar «Lleva arqueo» a una caja secundaria; con todas las
  cajas cerradas, gasto en efectivo en esa caja → se guarda sin aviso; ningún corte cambia.
- **Q3 caja con arqueo cerrada**: gasto en efectivo en la principal cerrada → hoja «Caja principal no tiene
  turno abierto · Este gasto no entrará a ningún corte»; «Cancelar» no guarda; «Registrar sin incluir en el arqueo» guarda.
- **Q4 fecha pasada con turno abierto**: pago con fecha de ayer → misma hoja; el esperado del corte
  abierto no cambia.
- **Q5 caso normal**: turno abierto, pago con la fecha del día del turno → sin hoja, entra al corte como antes.
- **Q6 guardas**: abrir turno en caja sin arqueo → rechazo; quitar arqueo a la principal → rechazo.
