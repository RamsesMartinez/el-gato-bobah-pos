# Research — 089

## R-1 · «Lo que falta» se deriva del estado, no se pide al servidor

- **Decision**: una función pura en `web/src/domain/cuentas.ts` traduce `AccountState` a {texto,
  acciones (cobrar, entregar), monto}. Sin campo nuevo en la respuesta.
- **Rationale**: el estado ya codifica las dos dimensiones (saldado × entregado) decididas por el
  servidor con `PedidoSaldado`; la traducción es nombre. Un campo `missing` duplicaría la regla.
- **Alternatives**: campo `missing: ['charge','deliver']` en `AccountItem` — rechazado por YAGNI:
  mismo dato, dos fuentes.

| Estado | Texto en pantalla | Botón en la columna |
| --- | --- | --- |
| capturing | «Sin enviar · $95» (gris) | — |
| in_kitchen | «Falta cobrar $180 y entregar» (rojo) | — (las dos: se abre el ticket) |
| paid_in_kitchen | «Falta entregar» (azul) | Entregar |
| partly_paid | «Falta cobrar $140 y entregar» (rojo; el monto es lo que resta) | — |
| delivered_owes | «Falta cobrar $60» (rojo) | Cobrar |
| closed_with_new | «Nuevo sin enviar» (gris) | — |

La fila angosta usa la forma corta («Cobrar $180 · entregar», «Cobrar $60»): en 112 px la larga se
corta.

## R-2 · Entregar desde Vender

- **Decision**: `posApi.deliverOrder(id)` (ya existe), con mutación que invalida `['pos','accounts']`
  y `['orders']`; botón deshabilitado mientras va (doble toque); error con `mensajeDeError`.
- **Rationale**: es el mismo camino de «Entregar todo» en Pedidos, de un toque. El servidor rechaza
  entregar un pedido ya cerrado con conflicto; la fila se relee.
- **Confirmación**: no. En Pedidos es un toque; una entrega errónea de una cuenta pagada es lo que
  se quería hacer, y una no pagada sigue visible como «Cobrar». La constitución pide minimizar taps.

## R-3 · Cobrar desde la cuenta

- **Decision**: elegir la cuenta y abrir la hoja de cobro con el pedido leído del servidor
  (`posApi.order(id)` → `aPedidoParaCobrar`). Sin envío previo (es pedido ya enviado).
- **Alternatives**: solo elegir la cuenta y dejar el botón Cobrar del ticket — rechazado: dos toques
  y el ticket puede estar oculto.

## R-4 · Canales sin renglones de texto

- **Decision**: el canal elegido se marca con el color (naranja para plataforma, como hoy) y el folio
  sigue en el mismo renglón. El renglón «Cobrando con precios de X» se elimina: el ticket ya dice el
  canal bajo el nombre. «Mantén presionado un producto…» pasa a una ayuda en el selector.
- **Rationale**: son ~40 px que solo existen con plataforma: justo cuando hay más pedidos.

## R-5 · Franjas del shell

- Entrantes 52 px (`FranjaDeEntrantes`, `minH 52px`), agotados 60 px (`FranjaDeAgotado`, 56 + 4 de
  barra). Juntas 112 px. Se miden en las maquetas y en la e2e simulando su alto.
