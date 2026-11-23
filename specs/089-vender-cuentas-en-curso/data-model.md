# Data model — 089

Sin cambios de esquema ni de contrato del servidor.

- **AccountItem** (`GET /pos/accounts`, sin cambios): `state`, `total`, `paid`, `outstanding`,
  `orderId`, `kind`.
- **LoQueFalta** (solo front, derivado): `{ texto: string; cobrar: boolean; entregar: boolean;
  monto: number | null }`, función pura de `AccountItem` (tabla en research R-1). No se guarda.
- Transiciones: entregar lleva el pedido a `entregada` (`domain.CanTransition` ya lo permite desde
  `abierta` y `lista`); cobrar no cambia el estado de cocina.
