# Feature Specification: Devolver el dinero de una venta que no fue

**Feature Branch**: `007-devolver-el-dinero`

**Created**: 2026-09-03

**Status**: Decidido — las cuatro preguntas las resolvió el dueño el 2026-09-03 (ver *Decisiones tomadas*)

**Input**: Hallazgos P1, P2 y P4 del [barrido de pantallas](../../docs/auditoria/barrido-de-pantallas-2026-09.md)
del 3 de septiembre de 2026.

## Contexto medido

El sistema **no tiene ninguna operación que devuelva dinero**. Tiene dos que dicen que una venta no
ocurrió —cancelar y reembolsar— y ninguna de las dos mira lo que el cliente ya pagó.

| Qué | Hoy |
| --- | --- |
| `Cancel` consulta los pagos del pedido | **No** |
| `Refund` consulta los pagos del pedido | **No**: anota como pérdida `o.Total` |
| Estados que acepta `Refund` | Solo `entregada` |
| El corte de caja filtra por estado del pedido | **No**: `ExpectedByMethodForSession` suma todo pago del turno |
| Forma de cancelar un renglón suelto | **No existe**, y un mensaje de error la nombra |

### Los tres agujeros, con su recorrido

**A. Cancelar un pedido ya cobrado.** Un pedido de $275 que se cobró en el mostrador queda
`abierta` con saldo cero mientras cocina prepara. Cancelarlo desde el tablero responde 204: la venta
sale de los reportes, los renglones de `order_payments` se quedan intactos y el arqueo **sigue
esperando** esos $275 en el cajón. Si se le devuelve el dinero al cliente, el corte cierra con $275
de faltante y ningún renglón lo explica. Si no se le devuelve, el negocio se quedó con dinero que no
aparece en ninguna venta.

**B. Reembolsar un entregado sin cobrar.** Un pedido entregado con $220 pendientes pinta en la
misma tarjeta **Cobrar $220** y **Reembolsar**, pegados. Tocar el segundo registra
`refund_amount = 220`: el reporte de devoluciones anota una pérdida de $220 que nunca fue ingreso, y
la cuenta por cobrar desaparece del badge sin haberse cobrado.

**C. Entrega parcial sin salida.** Un pedido con un renglón entregado y otro en la freidora no se
puede cancelar (repondría stock de comida que ya salió) ni reembolsar (`Refund` exige `entregada`).
El error dice *"cancela los que falten o haz un reembolso"* y **cancelar un renglón no existe**: no
hay query que ponga `order_lines.cancelled_at`. La única salida practicable es marcar como entregado
lo que sigue en la plancha.

### Qué principio rompe cada uno

- **A y B**: principio III — *cada peso se clasifica una sola vez, y lo que no es ingreso no entra
  al total*. En A el mismo dinero es "esperado en caja" y "venta que no ocurrió" a la vez; en B se
  clasifica como pérdida un dinero que jamás fue ingreso.
- **A**, además: `/orders/{id}/cancel` no lleva `RequireRole` mientras el reembolso sí lo lleva
  **por ser salida de dinero**. El camino que mueve el mismo dinero quedó sin la misma barrera —
  principio IV(c), *el camino nuevo que se salta el control viejo*.
- **C**: restricción de producto — *prohibido advertir de algo que el usuario no puede accionar
  desde ahí*.

## User Scenarios & Testing *(mandatory)*

### US1 — Cancelar una venta que el cliente ya pagó (P1)

**Como** cajero, **quiero** que el sistema no me deje borrar en silencio una venta ya cobrada,
**para** que el cajón y el reporte digan lo mismo al cerrar el turno.

- **Given** un pedido cobrado en efectivo por $275, todavía en cocina,
  **when** intento cancelarlo,
  **then** el sistema no lo cancela sin resolver el dinero.
- **Given** ese mismo pedido, **when** registro la devolución del dinero,
  **then** el arqueo deja de esperar esos $275 y queda constancia de quién devolvió, cuánto y por qué.

### US2 — Devolver una venta entregada (P2)

- **Given** un pedido entregado y cobrado por $500,
  **when** lo reembolso,
  **then** la pérdida registrada es **$500** —lo que efectivamente entró—, no el total nominal.
- **Given** un pedido entregado y **sin cobrar** por $220,
  **when** intento reembolsarlo,
  **then** el sistema no registra una pérdida de $220: no hubo ingreso que devolver.

### US3 — Cancelar un renglón que no se va a preparar (P4)

- **Given** un pedido con un renglón entregado y otro pendiente,
  **when** cancelo el renglón pendiente,
  **then** el total del pedido baja, el stock de ese renglón se repone, y el pedido sigue vivo para
  cobrarse o entregarse.

## Requirements *(mandatory)*

- **FR-001** Cancelar un pedido con pagos registrados no puede dejar el dinero sin clasificar.
- **FR-002** El monto de un reembolso es **lo efectivamente pagado**, no el total del pedido.
- **FR-003** Un pedido entregado sin pagos no admite reembolso; el rechazo dice por qué.
- **FR-004** Toda salida de dinero exige el mismo rol que hoy exige el reembolso.
- **FR-005** Toda salida de dinero deja constancia: quién, cuánto, por qué y contra qué método.
- **FR-006** El corte de caja no espera en el cajón el dinero de una venta cuyo dinero se devolvió.
- **FR-007** Existe una forma de cancelar un renglón pendiente sin tocar los ya entregados.
- **FR-008** Ningún mensaje de error nombra una acción que no se puede hacer desde donde aparece.
- **FR-009** Las cifras históricas de arqueos ya cerrados no cambian.

## Success Criteria *(mandatory)*

- **SC-001** Un turno con una venta cobrada y luego cancelada cierra **sin diferencia** cuando el
  dinero se devolvió, y con la diferencia exacta cuando no.
- **SC-002** El reporte de devoluciones y la suma de salidas de dinero del periodo coinciden.
- **SC-003** Ningún camino de la interfaz ofrece reembolsar un pedido sin cobrar.
- **SC-004** Un pedido con entrega parcial se puede llevar a un estado terminal sin marcar como
  entregado nada que no haya salido.

## Decisiones tomadas

Las cuatro las resolvió el dueño el 2026-09-03. El spec las implementa; no se reabren.

### D1 — Cancelar un pedido cobrado PIDE registrar la devolución, no lo rechaza

Rechazar era una línea, pero deja al cajero sin salida: no existiría el "devuélvelo primero" al que
lo manda el error. Cancelar un pedido con cobros abre el mismo paso que devuelve el dinero, y las
dos cosas van en **una transacción**: un pedido cancelado sin su devolución registrada es
exactamente el agujero que esto viene a cerrar.

### D2 — La devolución SALE DEL CAJÓN cuando el cobro fue en efectivo

`register_cash_movements` ya existe con `kind in ('entrada','salida')` y el corte ya la lee, así que
no hay maquinaria nueva: la devolución en efectivo entra como **salida** y el arqueo la descuenta
sola. Nadie cuadra a mano, que es de donde salió el turno con $4,500 de faltante.

**Lo que NO sale del cajón**: tarjeta y plataformas. Ese dinero nunca estuvo en la caja, así que
descontarlo del cajón inventaría un faltante. Se registra contra su método y se concilia con la
terminal. La devolución sabe por qué método entró cada peso porque los cobros están en
`order_payments` con su `payment_method_id`.

### D3 — El reembolso puede ser POR RENGLÓN, no solo de la cuenta entera

Devolver un platillo de tres es un caso real de este negocio y ya ocurre. Hoy el reembolso es de la
cuenta completa, así que la única salida es devolver de más o arreglarlo fuera del sistema —donde no
lo ve ningún reporte—.

El monto de un reembolso es **lo efectivamente cobrado** de lo que se devuelve, nunca el precio de
lista: un renglón de un pedido que se cobró a medias no puede devolver más de lo que entró.

### D4 — Cancelar un renglón repone inventario SOLO si no salió a cocina, y lo decide el sistema

No se le pregunta al cajero: `order_lines.enviado_a_cocina_at` ya responde. `NULL` = no salió, la
comida no se hizo, el insumo vuelve. Con fecha = ya está en la plancha, y reponer inventariaría algo
que se consumió.

**Consecuencia que hay que decir en pantalla**: cancelar un renglón que ya salió a cocina baja el
total del pedido pero NO devuelve el insumo, porque se gastó. La pantalla lo dice antes de
confirmar; callarlo hace que el almacén cuadre mal y nadie sepa por qué.

**Limitación asumida**: `stock_movements` no sabe de qué renglón salió cada descuento, así que
reponer con precisión exige agregarle esa referencia. Los movimientos históricos quedan sin ella y
un renglón de un pedido viejo no se puede reponer de forma exacta — se dice, no se adivina.

## Assumptions

- `orders.business_date` no se toca. Lo que cambia es qué se registra, no en qué día cae una venta.
- Los arqueos ya cerrados quedan idénticos: cualquier columna nueva nace vacía para lo histórico.
- Se implementa contra Postgres real y con **al menos dos empresas** en la base de prueba: los
  caminos "por cada otra empresa" son no-ops con una sola, y la migración pasaría verde para romper
  en producción.
