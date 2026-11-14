# Quickstart: validar notificaciones de inventario (084)

Contra el `make start` del worktree (API :8084, web :3084, mailpit :8284) con una base restaurada
(`make db-restaurar`, dos empresas). Ventana del navegador 1280×854 con `hasTouch`; repetir en
2560×1440. Capturas solo en `test-results/`.

## Pruebas automáticas

```bash
cd server && go build ./... && go test ./...           # domain y servicios
make test-integration                                   # trigger, RLS, concurrencia, correo con reloj falso
cd ../web && bun run vitest run && bun run lint && bun run build
bun run compilar-correos && git diff --exit-code ../server/internal/mailtpl
E2E_BASE_URL=http://localhost:3084 E2E_API_URL=http://localhost:8084/api/v1 E2E_SLUG=gatobobah \
  E2E_USER=… E2E_PASSWORD=… E2E_PIN=… bun run e2e -- notificaciones
```

## Recorrido manual (lo que la e2e también cubre)

1. Como **cajero** (rol con `inventory.report_out`, sin `inventory.count`): menú › «Se acabó», escribir
   «quesos», tocar «Queso crema», confirmar. Ver «Deshacer» y la existencia en 0 (como gerente, en
   Almacén). Intentar `POST /stock/movements` con el token del cajero → 403.
2. Repetir el toque: «También lo confirmaste» y ningún correo extra en mailpit.
3. Escribir «servitoallas» → «No lo encuentro». El cajero no ve ese aviso en su campana; el gerente sí.
4. Como **gerente**: campana al pie del riel con el contador; abrir el panel; el contador baja solo
   para él. «No lo encuentro» › «Es otro nombre de…» › un insumo; buscar «servitoallas» otra vez y
   encontrarlo.
5. Tarjeta de un insumo con existencia 2: «Alertas» › encender, mínimo 5 › Guardar. La confirmación
   dice que se avisará al guardar. Ver el aviso abierto. Subir la existencia con una compra recibida:
   el aviso se resuelve con «llegó compra».
6. Esperar el cierre del bloque de 5 min: un solo correo en mailpit con todo; < 102 KB; con logo.
7. Destinatarios: quitar al gerente, agregar un correo extra; repetir un «Se acabó» en otro insumo y
   ver que solo el extra recibe.
8. Lista de compra: cada renglón con su porqué; uno sin consumo dice «sin consumo medido».
9. Con una hoja de captura abierta en Vender, otro reporta «Se acabó» de un insumo con franja: la
   franja espera; al cerrar la hoja aparece y se va sola a los 90 s.

## Medición

`TestLowStockTriggerLatency` crea un pedido de 30 renglones con receta sobre insumos con alerta y
reporta el tiempo de guardado con y sin el trigger; el resultado va al reporte final.
