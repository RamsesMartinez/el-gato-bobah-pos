# Feature Specification: Renglones excluidos y caja que vende

**Feature Branch**: `049-compras-con-excluidos`

**Created**: 2026-11-01

**Status**: Draft

**Input**: Lo que queda del issue #111 después de las specs 047 y 048: renglones excluidos en una
compra (sección 5), total del gasto distinto del total del ticket, y «habilita ventas» como ajuste de
caja (sección 1). Menos es más.

**Decisiones con puntaje** (evaluador independiente, 2026-11-01): «habilita ventas» como ajuste visible
con UNA caja que vende por sucursal, cambiable con las cajas cerradas (6/2/8) contra varias cajas
vendiendo a la vez (8/8/2, cruza la puerta del principio VIII). La «propuesta por defecto (por
confirmar)» del issue —importe que salió de la caja mayor que el gasto— no se construye: el issue la
marca por confirmar, y el total del ticket que esta spec guarda deja la puerta abierta.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Excluir renglones de una compra (Priority: P1)

Un ticket trae cosas del negocio y cosas que no lo son. Quien captura marca «Excluir» en los renglones
que no son del negocio: siguen visibles, atenuados, con su importe; no entran a existencias, ni al
costo, ni suman al total del gasto.

**Independent Test**: ticket leído con 8 renglones, 2 excluidos: existencias y costo reflejan 6; los 2
se ven atenuados en el detalle; el total del gasto es el que se escribió.

**Acceptance Scenarios**:

1. **Given** un renglón leído o capturado, **When** se marca «Excluir», **Then** se ve atenuado con su
   importe y no cuenta para «la suma no pasa del total».
2. **Given** se guarda, **Then** los excluidos no entran a existencias ni al costo y quedan guardados
   como evidencia.
3. **Given** un excluido antes de guardar, **When** se desmarca, **Then** vuelve a contar.
4. **Given** un gasto guardado, **When** quien puede recibir mercancía des-excluye un renglón, **Then**
   entra a existencias con un movimiento correctivo; **When** excluye uno que ya entró, **Then** sale con
   un movimiento correctivo (como en la spec 038).

---

### User Story 2 - Total del ticket informativo (Priority: P2)

El total del gasto es lo que cuenta para el negocio y se escribe a mano. Si el ticket se leyó por
foto, su total se guarda aparte y se ve en el detalle como «Total del ticket».

**Acceptance Scenarios**:

1. **Given** un ticket leído con total $500 y $120 excluidos, **When** se escribe $380 como total del
   gasto, **Then** se guarda así y el detalle muestra «Total del ticket $500».
2. **Given** un gasto capturado a mano, **Then** no aparece «Total del ticket».

---

### User Story 3 - Elegir qué caja vende (Priority: P3)

En Caja › Administrar, la columna «Vende» muestra cuál caja recibe las ventas. Quien configura cajas
puede pasar las ventas a otra caja, con las dos cerradas.

**Acceptance Scenarios**:

1. **Given** dos cajas cerradas, **When** se marca «Vende» en la otra, **Then** tras confirmar, esa
   pasa a vender, la anterior deja de hacerlo, y la nueva lleva arqueo.
2. **Given** alguna de las dos con turno abierto, **Then** se rechaza: «Cierra las dos cajas primero».
3. **Given** quien no tiene el permiso de configurar caja, **Then** no puede.
4. **Then** nunca quedan dos cajas vendiendo ni ninguna en la sucursal.

### Edge Cases

- Excluir un renglón que ya entró saca sus existencias, pero el costo actual del insumo no se
  revierte (mismo techo que ya existe para correcciones de compra).
- Un renglón excluido que es regalo, o con importe 0: se guarda igual.
- Excluir todos los renglones: el gasto queda solo con su total («otros»).
- Des-excluir un renglón que haría que la suma pase del total: se rechaza con el mensaje de la 038.
- Gastos ya guardados: ninguno tiene excluidos; nada cambia.
- Lo que hoy se marcaba «personal» y se descartaba ahora se guarda excluido.

## Requirements *(mandatory)*

- **FR-001**: Cada renglón de una compra MUST poder marcarse excluido; uno excluido se guarda, no entra
  a existencias ni a costo, y no cuenta para la regla de la spec 038.
- **FR-002**: Excluir o des-excluir después de guardar MUST hacerse con un movimiento correctivo de
  existencias, con el permiso de recibir mercancía.
- **FR-003**: El detalle MUST mostrar los excluidos atenuados con su importe.
- **FR-004**: Si el ticket se leyó por foto y trae total, MUST guardarse y mostrarse como «Total del
  ticket», distinto del total del gasto.
- **FR-005**: Cada caja MUST mostrar si vende; pasar las ventas a otra caja MUST exigir el permiso de
  configurar caja, las dos cajas cerradas y la misma sucursal, y ocurrir de un solo golpe.
- **FR-006**: Los controles nuevos preguntan por permiso.

## Success Criteria *(mandatory)*

- **SC-001**: Un ticket mixto se captura sin borrar evidencia y con existencias exactas.
- **SC-002**: Ninguna sucursal queda con dos cajas que venden o con ninguna.
- **SC-003**: El caso normal (ticket sin excluidos) no agrega pasos.

## Assumptions

- La casilla «Excluir» reemplaza a la marca «personal» que hoy descarta el renglón.

## Fuera de alcance

- «Importe que salió de la caja» (por confirmar en el issue). Varias cajas vendiendo a la vez.
  Recordar exclusiones por proveedor.
