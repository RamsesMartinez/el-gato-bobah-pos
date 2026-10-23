# Validación a 1024×600 (T024)

Se corrió el 2026-10-12 contra la API local de esta rama, con la migración 0090, y se probó en Playwright a 1024×600. Lo observado queda en texto: las capturas no se versionan (AGENTS.md).

**Entorno**
- **API**: sirve como `gatobobah_app` y `gatobobah_platform`, nunca como owner (`scripts/dev-api.sh`). La base es un Postgres desechable con dos cajas.
- **Arqueo ciego**: encendido.
- **Fondo fijo**: $1,000 en la caja principal.
- **Usuarios de prueba**: una gerente, la cajera «Ana Cajera» y el cajero «Luis Turno».
- **Datos**: no hay ventas, porque no se creó ningún pedido; solo se abre y se cierra el turno. Los nombres son inventados.

| # | Paso | Resultado |
|---|---|---|
| 1 | Ajustes › Caja como gerente: bloque «Fondo fijo». | Un renglón por caja: «Caja principal $1,000 ›» y «Caja fuerte · Sin configurar ›». |
| 2 | Tocar la caja principal. | Hoja inferior fija al borde de la pantalla, con el campo de solo lectura, el teclado de 52 px, «Quitar fondo fijo» a la izquierda, y «Cancelar» y «Guardar» a la vista. Bajo el campo: «El cierre lo trae escrito y se puede cambiar solo ese día.» **La primera corrida encontró un defecto**, descrito abajo. |
| 3 | Como cajera, contar el cajón (5×$1,000 + $200 + $100 = $5,300) y bajar al cierre. | «¿Se retira dinero del cajón?» abre en «Sí». El campo trae **$1,000**, el fijo, sin teclearlo, y al lado dice «→ Retiro $4,300». «Se lo lleva» muestra una fila de personas con quien cierra marcada. |
| 4 | Tocar «No, todo se queda». | «Se queda de fondo todo lo contado: $5,300. Sin retiro. (el fijo es $1,000)». Sin campo, sin personas y sin borde rojo. |
| 5 | Volver a «Sí», elegir a Luis y tocar «Cerrar caja». | La confirmación dice: «Se retiran $4,300 · se los lleva Luis Turno. No podrás modificarla después.» |
| 6 | Confirmar. | El turno queda cerrado. |
| 7 | Histórico y corte, como gerente. | El corte muestra «Retiro al cierre −$4,300» en egresos y, en el arqueo, contado $5,300. |
| 8 | Histórico y corte, como cajera, con la caja todavía sin reabrir. | En el histórico la diferencia dice «Oculto». El corte dice «Arqueo oculto · Se ve cuando se vuelva a abrir la caja.» No aparece el retiro, ni lo contado, ni el esperado, ni las diferencias. «Efectivo contado» muestra solo la apertura, que capturó ella misma. |

## Defecto encontrado y corregido

- **Qué pasaba**: la hoja del fondo fijo en Ajustes se pintaba a media página y no fija al borde de la pantalla. A 1024×600, el botón «Guardar» quedaba en y=848, fuera de la pantalla.
- **Causa**: `FondoFijo.tsx` armaba la hoja con las piezas crudas de Chakra, sin el `Positioner` que trae `components/ui/drawer`.
- **Arreglo**: se importa de `components/ui/drawer`.
- **Prueba**: `AjustesDeCaja.test.tsx › la hoja va fija al borde inferior de la pantalla`. Falló antes del arreglo con «expected null not to be null». Después del arreglo, «Guardar» queda en y=532.

## Lo que no se verificó en pantalla

- **Pasos 4 y 5 de [quickstart.md](quickstart.md)**: un segundo turno con el fondo cambiado a $800, y un cierre de antes de la subida. No se recorrieron en pantalla. Los cubren las pruebas de integración: cierre con `floatLeft` distinto del fijo, con su snapshot, y migración con cierres previos de dos empresas.
- **Reabrir la caja y comprobar que la cajera vuelve a ver el corte**: lo cubre `TestLastCloseHidesCountedFromCashierUntilReopened`; no se vio en pantalla.
- **Turno con ventas reales**: no hubo pedidos, así que el esperado fue $0. La regla de que el retiro no cambia el arqueo con ventas la cubren las pruebas de integración.
- **Tableta física**: no se probó. Todo se hizo en Chromium a 1024×600.
