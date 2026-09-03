# Fase 1 — Cómo se verifica

Un recorrido por historia, cada uno con **el fallo que se espera si la pieza no está**. Un paso que
solo dice "funciona" no sirve para verificar nada.

Ambiente: `app-dev.elgatobobah.com` contra `api-dev.elgatobobah.com`, en una ventana de **1024×600**
(en el navegador: dispositivo personalizado con esa medida, no la ventana del escritorio a ojo).

## Antes de empezar

1. Entrar con `admin@gatobobah`.
2. En Ajustes → Impresión, encender la comanda de cocina. Sin eso, US3 no imprime nada y el
   recorrido pasa en falso.
3. Anotar cuántos renglones de productos se ven en la lista. Es la línea base de SC-005.

## US1 — El pedido confirmado sigue a la vista

1. Armar una cuenta con dos productos y confirmarla.
   - **Se espera**: aparece un chip con su nombre de animal en la fila de cuentas, y la cuenta local
     queda vacía.
   - **Falla si**: el chip no aparece o la cuenta local sigue con los productos. Lo primero es que
     la barra no está leyendo del servidor; lo segundo es el defecto de tener dos versiones del
     mismo pedido.
2. Tocar el chip y agregar un producto más.
   - **Se espera**: un solo toque para llegar al pedido, y el producto se suma a **ese** pedido.
   - **Falla si**: se crea un pedido nuevo, o si llegar cuesta más de un toque. Lo segundo es SC-001
     en rojo, que es el motivo de la feature.
3. Recargar la página (F5).
   - **Se espera**: el chip sigue ahí, con el mismo folio y el mismo monto.
   - **Falla si**: aparece dos veces. Es el pedido duplicado del carrito local que no se limpió.
4. Abrir la aplicación en una segunda ventana, como si fuera la otra tableta.
   - **Se espera**: el mismo pedido, con el mismo folio, en menos de 30 segundos.
   - **Falla si**: no aparece. Es la barra leyendo del almacén local en vez del servidor, que es la
     decisión B2 sin hacer.
5. Contar los renglones de productos visibles.
   - **Se espera**: los mismos que en la línea base.
   - **Falla si**: hay menos. La barra se está comiendo alto que no le toca.

## US2 — Cobrar exige haber confirmado

1. Armar una cuenta y buscar cobrarla sin confirmar.
   - **Se espera**: el POS ofrece confirmar; cobrar no está disponible.
   - **Falla si**: se puede cobrar. Es el camino corto sin cerrar.
2. Desde la consola del navegador, `POST /api/v1/orders` con `payments` no vacío.
   - **Se espera**: `422`, con un mensaje que dice que se cobra con `/pay`.
   - **Falla si**: devuelve `201`. La barrera está solo en la pantalla, y eso es el principio V en
     rojo: el front es espejo, nunca la barrera.
3. `POST /api/v1/orders` con `lines: []`.
   - **Se espera**: `400`.
   - **Falla si**: crea un pedido. Ocupó un folio y va a sacar una comanda en blanco.
4. Cobrar un pedido en curso desde la barra.
   - **Se espera**: se cobra y el chip desaparece de "en preparación".
   - **Falla si**: pide confirmar otra vez. El pedido ya existe; pedirlo de nuevo es la barrera mal
     puesta.

## US3 — Lo agregado sale solo, marcado

1. Confirmar un pedido de dos productos con la comanda encendida.
   - **Se espera**: sale un papel con los dos, folio grande, sin precios.
2. Agregarle un tercero desde la barra.
   - **Se espera**: sale un segundo papel con **solo el tercero**, el mismo folio y la marca de
     agregado.
   - **Falla si**: salen los tres. Cocina va a preparar dos veces lo primero, que es el defecto que
     C1 vino a evitar.
3. Desde `/pedidos`, pedir la reimpresión de ese pedido.
   - **Se espera**: sale la comanda completa con los tres.
4. Apagar la impresora y agregar otro producto.
   - **Se espera**: el renglón queda agregado y sale un aviso de que no se pudo imprimir.
   - **Falla si**: se pierde el renglón, o si no avisa nada. Lo segundo es el modo de fallo que la
     feature 001 ya quitó del ticket del cliente y que no puede volver por esta puerta.

## US4 — La empresa nueva nace imprimiendo

1. Provisionar una empresa de prueba.
   - **Se espera**: su ajuste de comanda nace encendido.
2. Leer los ajustes de `gatobobah`.
   - **Se espera**: **sin cambios** respecto a antes del despliegue.
   - **Falla si**: se encendió sola. Un despliegue que cambia la configuración de un negocio en
     operación sin que nadie la pida es un defecto, no una mejora.

## Los bordes, a mano

Estos no se ven en el camino feliz y son los que cuestan:

1. **Dos ventanas agregando al mismo pedido.** Agregar en las dos casi a la vez.
   - **Se espera**: el pedido termina con los renglones de las dos.
   - **Falla si**: uno pisa al otro. Se perdió una venta.
2. **Agregar a un pedido entregado.** Entregar el pedido en una ventana y agregarle en la otra.
   - **Se espera**: `409` con el estado en el mensaje.
   - **Falla si**: agrega. Quedó un renglón sobre un pedido terminado que nadie va a preparar.
3. **Agregar a un pedido ya cobrado.**
   - **Se espera**: se agrega y el pedido vuelve a la barra con el saldo nuevo a la vista.
   - **Falla si**: el saldo no se ve. Es deuda invisible, que es exactamente lo que el dueño señaló
     al pedir esta ronda de trabajo.
4. **Confirmar dos veces con la red cortada.** Cortar la red en las herramientas del navegador,
   confirmar, restaurar y reintentar.
   - **Se espera**: **un** pedido.
   - **Falla si**: hay dos con lo mismo. El identificador de la cuenta se está regenerando en cada
     intento y la idempotencia del servidor nunca se dispara.
5. **Un pedido viejo, de antes del despliegue.** Cobrarlo y entregarlo.
   - **Se espera**: funciona igual que siempre.
   - **Falla si**: se bloquea. Los pedidos que ya existen no pueden cambiar de significado.

## Los gates, antes de dar nada por bueno

```bash
cd server && go build ./... && go test ./... && go test -tags=integration ./internal/integration/...
cd ../web && bun run lint && bun run vitest run && bun run build
```

En Windows corren en contenedor; los scripts de `scripts/hooks/` ya traen la salida.
