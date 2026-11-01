# Research: 048
- **Barrido con `update … returning` bajo el candado del turno**, no ventana calculada al vuelo: el
  esperado sale de `register_cash_movements` y así sigue; el pendiente se convierte en una salida normal.
- **Hora de pagos pasados = medianoche de su día**: no existe hora real; así quedan antes del último
  cierre y no pueden caer en un corte equivocado.
- **Mover la hora solo al abrir**: cubre el caso del issue (red de seguridad) sin reabrir ventanas de cortes vivos.
