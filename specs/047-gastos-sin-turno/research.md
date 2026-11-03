# Research: 047

- **Decisión**: el pago guarda `register_id` además de `register_session_id`.
  **Porqué**: el hecho «de qué caja salió» es irrecuperable si no se guarda; la sesión es nula fuera
  de corte. **Alternativa**: abrir y cerrar un turno fantasma — descuadra cortes y es lo que hoy
  obliga a hacer la operación.
- **Decisión**: entra al corte si `paid_on` = día de negocio del turno abierto (no «hoy» del reloj).
  **Porqué**: un turno que cruza la medianoche sigue siendo de su día, igual que `openShiftDay`.
- **Decisión**: la confirmación la exige el servidor (`outsideCount`). **Porqué**: principio V; la
  pantalla no puede saber con certeza si el turno sigue abierto.
- **Decisión**: alcance A (puntaje 8.5 vs 5.5 y 4, evaluado por un revisor). El resto de #111 sigue abierto.
