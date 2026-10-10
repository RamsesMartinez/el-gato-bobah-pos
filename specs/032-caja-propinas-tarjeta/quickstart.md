# Quickstart: validar caja, propinas y tarjeta

1. `make start` (API como `gatobobah_app`), y `cd server && go build ./... && go test ./...`.
2. Abrir caja contando por denominación con una diferencia: debe pedir motivo; sin ver el cierre.
3. Cobrar un pedido en efectivo con propina y otro con tarjeta (débito, terminal por omisión) con propina.
4. «Entregar propina» a un usuario por el total: ventas y gastos sin cambio; efectivo esperado baja solo por la de tarjeta.
5. Capturar una salida con concepto nuevo; «Corregir»: reverso + nueva, corte cuadra.
6. Devolver el cobro con tarjeta: pide folio; sin folio no termina.
7. Cerrar con propina pendiente eligiendo «se queda en caja»; arqueo por terminal; el siguiente turno de esa caja la hereda.
8. Revisar en mailpit (:8095) un solo correo de resumen del día.
9. Entregar y cobrar todo pedido de prueba creado.
