# Feature Specification: Devolver el dinero de una venta que no fue

**Feature Branch**: `007-devolver-el-dinero`

**Created**: 2026-09-03

**Status**: Draft — decisiones pendientes del dueño (ver *Preguntas que este spec NO puede contestar*)

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

## Preguntas que este spec NO puede contestar

Son decisiones del dueño y cambian el alcance. **La implementación no empieza sin ellas.**

1. **Cancelar un pedido cobrado: ¿se rechaza o se permite con devolución?** Rechazar es una línea de
   código y obliga a devolver primero; permitir con devolución es una operación nueva. La segunda es
   más trabajo y refleja mejor lo que pasa en la caja.
2. **La devolución, ¿sale del cajón o solo se anota?** Si sale del cajón, es un movimiento de caja y
   el arqueo lo descuenta solo. Si solo se anota, alguien tiene que cuadrarlo a mano.
3. **Reembolso parcial: ¿hace falta?** Hoy el reembolso es de la cuenta entera. Devolver un platillo
   de tres es un caso real en un restaurante; construirlo cuando nadie lo ha pedido es lo que el
   principio VI prohíbe. La pregunta es si ya se pidió.
4. **Cancelar un renglón, ¿repone stock?** Un renglón que nunca se preparó, sí. Uno que ya está en la
   plancha, no — y el sistema no distingue esos dos hoy.

## Assumptions

- `orders.business_date` no se toca. Lo que cambia es qué se registra, no en qué día cae una venta.
- Los arqueos ya cerrados quedan idénticos: cualquier columna nueva nace vacía para lo histórico.
- Se implementa contra Postgres real y con **al menos dos empresas** en la base de prueba: los
  caminos "por cada otra empresa" son no-ops con una sola, y la migración pasaría verde para romper
  en producción.
