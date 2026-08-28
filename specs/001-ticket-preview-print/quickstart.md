# Phase 1 — Quickstart: cómo verificar el visualizador de ticket

**Feature**: 001-ticket-preview-print · **Date**: 2026-08-27

Guía de validación, no de implementación. Cada bloque cierra una historia de la
[spec](./spec.md).

## Prerrequisitos

```bash
BACKEND_PORT=8080 FRONTEND_PORT=3000 make start   # postgres+redis+mailpit+API+web
```

Credenciales de dev y puertos: los que dejó `make start`. Las migraciones `0033`, `0034` y `0035` corren solas al arrancar la API (goose embebido); la
`0035` deja el pie del ticket precargado.

Si `make start` muere con *Permission denied* en `server/tmp/api`, es Smart App Control bloqueando
el binario recién compilado: levanta la API en contenedor con el `docker run` de `AGENTS.md` §7 y
deja el front corriendo en el host. No es un problema de esta feature.

**Los tests de integración NO se corren contra la base de dev**: el harness hace
`drop schema public cascade`. Va contra una base aparte:

```bash
cd server && TEST_DATABASE_URL='postgres://gatobobah:gatobobah@localhost:5490/gatobobah_test?sslmode=disable' \
  go test -tags=integration ./internal/integration/
```

**Para las verificaciones en papel** hace falta la impresora térmica instalada como impresora del
sistema. En la caja de dev es `POS-80` sobre `USB001`, y ya está como default. Para que no salga el
diálogo, el navegador se lanza así:

```powershell
& "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" `
  --kiosk-printing --user-data-dir="$env:LOCALAPPDATA\edge-pos" http://localhost:3000
```

El `--user-data-dir` aparte no es opcional: con Edge ya abierto, la instancia nueva se une a la
existente y el flag se ignora en silencio.

## Gates automáticos

```bash
cd server && go build ./... && go test ./...   # = make api-build && make api-test
make lint                                       # golangci-lint + gosec
cd web && bun run lint && bun run typecheck && bun run test
```

Los tests que esta feature agrega y que deben aparecer en verde:

| Test | Qué prueba |
| --- | --- |
| `domain/logo_test.go` | tamaño, tipo por contenido, dimensiones, y que un JPEG renombrado a `.png` se clasifique por lo que es |
| `domain/logo_test.go` (caso SVG) | un SVG con `<script>` se rechaza |
| `httpapi/handlers_settings_test.go` | un cajero recibe 403 al subir logo, y queda el evento de seguridad |
| `utils/printReceipt.test.ts` | encabezado con y sin cada campo opcional; marca de reimpresión; el escape de XSS que ya existía sigue pasando |
| `features/tickets/TicketPreview.test.tsx` | el modal monta el documento del ticket; el botón dispara una sola impresión aunque se toque dos veces |

## US1 — Ver el ticket y mandarlo a imprimir (P1)

1. Entra al POS, arma un pedido con **dos líneas y al menos un modificador**, y ciérralo.
2. En el modal de confirmación, abre el ticket.
3. **Verifica en pantalla**: encabezado con logo y datos del negocio, las dos líneas con su
   modificador, el total y el estado de pago. Nada recortado.
4. Toca imprimir.
5. **Verifica en papel**: cada campo coincide con lo que estaba en pantalla (SC-002). El papel corta
   al final — eso lo hace el driver (`zjPaperCutting`), no el POS.
6. Repite con un pedido **a domicilio** con costo de envío: el ticket debe mostrar subtotal y envío
   desglosados antes del total.
7. Cierra la vista previa sin imprimir en un pedido nuevo: no sale papel y el pedido queda intacto.
8. Toca imprimir dos veces rápido: **sale un solo ticket**.

## US2 — Reimprimir desde el tablero (P2)

1. Con el pedido del paso anterior ya cerrado, ve al tablero de órdenes.
2. Abre el ticket de ese pedido.
3. **Verifica**: es la misma vista previa, con los mismos datos.
4. Imprime. **El papel sale marcado como reimpresión** y el original no lo estaba.
5. Cancela un pedido y abre su ticket: el estado se ve y no se presenta como venta cobrada.

## US3 — Datos del negocio y logo (P3)

1. Como **admin**, entra a ajustes del negocio. Captura nombre, dirección, teléfono y leyenda.
2. Cierra un pedido nuevo y abre su ticket: los datos nuevos ya están, **sin reiniciar nada**
   (SC-004).
3. Sube un PNG como logo. Ticket nuevo → sale ese logo.
4. Quita el logo. Ticket nuevo → vuelve el ícono del Gato Bobah.
5. **Rechazo por tipo**: renombra un `.txt` a `.png` e intenta subirlo. Debe rechazarse por
   contenido, decir qué se aceptaba, y el logo anterior debe seguir ahí.
6. **Rechazo por tamaño**: sube una imagen de más de 256 KB. Debe cortarse sin leerse completa.
7. **Autorización**: con un usuario **cajero**, llama al endpoint directo (no por la UI, que ni
   siquiera muestra la opción):

   ```bash
   curl -i -X PUT http://localhost:8080/api/v1/business-settings/ticket-logo \
     -H "Authorization: Bearer $TOKEN_CAJERO" -F file=@logo.png
   ```

   Debe responder **403** y dejar el evento `forbidden` en el log. Si responde 200, el principio V
   está roto y la feature no se mergea.

## Ticket de prueba (US3)

1. En **Impresión**, con el logo y los textos ya guardados, toca **Ticket de prueba**.
2. **Verifica en pantalla**: el ticket se ve completo y **sin barra de desplazamiento horizontal**.
   Si aparece esa barra, la vista se está comprimiendo en vez de escalarse (ver D8 en
   [research.md](./research.md)).
3. Toca fuera del diálogo: **no debe cerrarse**. Se cierra con su botón.
4. Imprime. **El papel sale marcado como `TICKET DE PRUEBA`** y no se registró ninguna venta:
   revisa el tablero y el corte de caja para confirmarlo.

## US4 — Imprimir solo al cerrar la venta (P2)

1. Como **admin**, enciende la impresión automática en la configuración del ticket.
2. Lanza el POS con `--kiosk-printing` (arriba) y cierra un pedido.
3. **Verifica**: el papel sale **sin tocar nada** (SC-009), y el botón para ver el ticket sigue ahí
   para reimprimirlo (FR-025).
4. Apaga la opción y cierra otro pedido: **no** debe salir papel hasta que lo pidas.
5. **Falla de impresión**: apaga la impresora y cierra un pedido con la opción encendida. El pedido
   tiene que quedar registrado igual y poder reimprimirse desde el tablero. Si la venta se pierde o
   la pantalla se queda trabada, es un bloqueante.
6. **Sin kiosk-printing**: lanza el POS con Edge normal y cierra un pedido con la opción encendida.
   Va a salir el diálogo del navegador en cada venta — es el comportamiento esperado y la razón de
   que la pantalla de configuración avise que el interruptor depende de cómo se lanzó el navegador.

## Persistencia a través del deploy (SC-005)

```bash
make stop && BACKEND_PORT=8080 FRONTEND_PORT=3000 make start
```

Abre un ticket: el logo y los datos siguen ahí. Si desaparecieron, alguien mandó el logo a disco del
contenedor en vez de a la base — es exactamente el defecto que D3 de [research.md](./research.md)
cierra.

## Paridad local ↔ desplegado (SC-006)

El mismo recorrido de US1 en la instancia desplegada, con el navegador abierto contra el dominio de
producción. **No debe hacer falta instalar nada** en el equipo: ni extensión, ni agente, ni
descarga. Si algo solo funciona en `localhost`, la feature no cumple FR-009.

## Legibilidad en 7" (SC-007)

Abre la vista previa en la tablet, en su resolución real. El ticket debe leerse y el botón de
imprimir debe estar visible **sin desplazarse**. Si hay que hacer zoom, no cumple.
