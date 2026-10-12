# Quickstart — validar la 033

Prerrequisitos: `make start` (API como `gatobobah_app`), dos empresas en la base, una con dos
sucursales.

1. **Gates**: `cd server && go build ./... && go test ./...`; `cd web && bun run lint && bun run vitest run && bun run build`.
2. **Activar**: como admin, Negocio › Asistencia → activar. Sin activar, el riel y la pantalla de
   bloqueo no muestran Asistencia.
3. **Checar** (US1): riel › Asistencia → persona → PIN → «Entrada». Volver: el botón ya dice
   «Salida». Desde la pantalla de bloqueo, lo mismo sin desbloquear. El usuario de la barra no
   cambió. Tres PIN malos → bloqueo, y el desbloqueo del POS de esa persona también queda bloqueado.
4. **Origen** (US2): checar desde otro navegador; en el reporte, los dos registros tienen IP,
   descripción y clave distintas. Como cajero, ninguna respuesta trae esos campos.
5. **Historial** (US3): crear pedido, agregar renglón, cobrar en dos pagos, devolver uno; como
   gerente, Historial → 5 eventos con autor, aparato e IP. Como cajero, el botón no aparece.
6. **Bitácora** (US4): cambiar un precio y registrar una salida; Reportes › Bitácora, filtrar por
   persona; ver «Precio: $X → $Y».
7. **Horas** (US5): con medición apagada, solo horas. Encenderla con 10 min, versión de horario L–S
   9–17; otra versión L–V desde hoy: el sábado ya no cuenta retardo.
8. **Corrección** (US6): gerente corrige la salida de otro (ok) y la suya (rechazado); captura a
   mano un día sin registro.
9. **Riel a 600 px**: captura real como admin; ningún ítem queda oculto sin indicio.
10. **e2e**: `bun run e2e` con los specs nuevos contra el ambiente de pruebas; los pedidos creados
    se cobran en el teardown.
