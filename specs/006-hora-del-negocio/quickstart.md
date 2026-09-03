# Fase 1 — Cómo se verifica

Un recorrido por historia, cada uno con **el fallo que se espera si la pieza no está**.

Ambiente: `app-dev.elgatobobah.com`, en una ventana de 1024×600.

## Antes de empezar

1. Entrar con `admin@gatobobah`.
2. Anotar la zona configurada en Ajustes → Negocio.
3. **Cambiar la zona horaria del sistema operativo** de la máquina a una distinta (p. ej. UTC o
   Tokio). Sin esto, el recorrido pasa en falso: la zona del navegador y la del negocio coinciden y
   no hay nada que distinguir.

## US1 — Todas las pantallas dicen la hora del local

1. Abrir el detalle de una venta y anotar la hora.
   - **Se espera**: la hora del local, no la del sistema.
   - **Falla si**: coincide con el reloj de la máquina. La zona no se está aplicando.
2. Recorrer arqueo, ventas, inventario y la información del sistema.
   - **Se espera**: todas coherentes entre sí.
   - **Falla si**: alguna difiere. Quedó un formateo suelto — el defecto que esta feature viene a
     hacer imposible.
3. Imprimir un ticket y una comanda.
   - **Se espera**: la hora del local en el papel.
   - **Falla si**: la de la máquina. Es el papel que se lleva el cliente.
4. Recargar con la red lenta (limitar a 3G en las herramientas del navegador).
   - **Se espera**: la hora aparece ya correcta.
   - **Falla si**: aparece una y cambia sola. El operador deja de confiar en lo que lee.

## US2 — Los pedidos activos no desaparecen

1. Dejar un pedido abierto y adelantar el reloj del sistema un día.
   - **Se espera**: sigue en la barra.
   - **Falla si**: desaparece. Es el defecto de la medianoche por otra puerta.
2. Mirar la cuenta de pruebas, que tiene once pedidos abiertos desde julio.
   - **Se espera**: se ven todos, y los de días anteriores se distinguen de los de hoy.
   - **Falla si**: no se distinguen. El rezago se confunde con el trabajo del día.
3. Entregar uno de los viejos.
   - **Se espera**: sale de la barra y aparece en entregados.

## US3 — "Entregados hoy" se vacía cuando el negocio dice

1. Con el corte en `medianoche`, entregar un pedido y adelantar el reloj a las 23:00 locales.
   - **Se espera**: sigue en la lista.
2. Pasar la medianoche local.
   - **Se espera**: ya no está, **sin recargar**.
   - **Falla si**: hay que recargar. La pantalla no está mirando el corte.
3. Cambiar el corte a `cierre_de_caja`, entregar un pedido y cerrar la caja.
   - **Se espera**: la lista se vacía aunque no haya cambiado el día.

## US4 — Cambiar la zona no asusta

1. Cambiar la zona a `America/Tijuana` y guardar.
   - **Se espera**: antes de guardar se explica que las horas mostradas cambian y que las ventas ya
     registradas no se mueven de día.
2. Abrir el arqueo de un día anterior.
   - **Se espera**: **las mismas cifras que antes del cambio**.
   - **Falla si**: cambiaron. Se movió dinero entre días, que es justo lo que no puede pasar.

## Los bordes, a mano

1. **Una zona que el navegador no reconoce.** Ponerla directo por la API.
   - **Se espera**: la pantalla funciona con el default y queda constancia.
   - **Falla si**: se cae o muestra UTC en silencio.
2. **Abrir caja sin poder leer la zona.** Es el defecto que dejó dos pedidos mal fechados en la
   cuenta de pruebas.
   - **Se espera**: la fecha de negocio sale del default del producto, no de UTC.
3. **El horario de verano.** Con la zona en `America/Tijuana`, comprobar que la medianoche del corte
   cae donde debe el día del cambio.
   - **Falla si**: se corre una hora. El corte está sumando 24 horas fijas en vez de calcular la
     medianoche de esa fecha.
4. **Dos empresas.** Configurar zonas distintas en `gatobobah` y `bobah-pruebas`.
   - **Se espera**: cada una ve la suya.

## Los gates

```bash
cd server && go build ./... && go test ./... && go test -tags=integration ./internal/integration/...
cd ../web && bun run lint && bun run vitest run && bun run build
```

En Windows corren en contenedor; los scripts de `scripts/hooks/` ya traen la salida.
