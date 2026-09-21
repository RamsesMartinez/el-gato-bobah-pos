---

description: "Tareas de la feature 023 — el panel del pedido se reacomoda"
---

# Tasks: El panel del pedido se reacomoda

**Input**: [plan.md](./plan.md), [spec.md](./spec.md) y el canvas de diseño (tablero **V3**).

**Estado**: implementado, con los gates en verde el 2026-09-20 (736 tests, lint, build). Los tres
hallazgos de la revisión de código quedaron arreglados con su test, y el más grave no era de
layout: en la hoja de cobro, un descuento mal tecleado **borraba el que ya estaba**, porque `{}` no
significa «sin cambios» para el servidor sino «quítalo».

**Tests**: primero, como siempre. Aquí el riesgo no es de dinero sino de **alto y de toques**, así
que cada test mide una de esas dos cosas.

**Dónde se trabaja**: worktree `/home/ramy/git/egb-022-descuentos`, rama
`023-el-panel-se-reacomoda`. Solo `web/`.

---

## Phase 1: Setup

- [x] T001 Confirmar el punto de partida en verde: `cd web && bun run lint && bun run vitest run`

---

## Phase 2: Foundational

- [x] T002 Agregar `setOrderDiscount(id, cuerpo)` a web/src/api/pos.ts, contra `PUT /orders/{id}/discount` — el endpoint ya existe desde la 022

---

## Phase 3: User Stories 1 y 2 — el panel (P1) 🎯 MVP

Van juntas: mover los controles sin el menú sería quitarlos.

### Los tests, primero

- [x] T003 [US1] Test en web/src/features/pos/Ticket.test.tsx: en un pedido de mostrador la zona de totales **no** trae botones de tipo ni campo de cliente, y el encabezado sí trae el tipo
- [x] T004 [US1] Test: en un pedido de **plataforma** el botón de tipo no se pinta — ese pedido es a domicilio por definición
- [x] T005 [US2] Test: el menú ofrece las tres acciones y cada renglón mide ≥48 px
- [x] T006 [US2] Test: «Vaciar el pedido» desde el menú sigue pidiendo confirmación, y **no se ofrece con el carrito vacío**
- [x] T007 [US2] Test: con un descuento aplicado, el renglón se ve en la zona de totales **con el menú cerrado**
- [x] T008 [US1] Test del nombre largo: con un folio del esquema `razas` (20 caracteres, «Colorpoint Shorthair») el nombre trunca y **ningún control del encabezado baja de 44 px**
- [x] T009 [US2] Test: con el campo inline abierto, los botones de acción **no se mueven** — lo que se encoge es la lista

### El código

- [x] T010 [US1] Reescribir el encabezado de web/src/features/pos/Ticket.tsx: `[ocultar] <nombre que trunca> … [tipo] [⋮]`, sin la palabra «Pedido» y sin el botón «Vaciar»
- [x] T011 [US1] Quitar de la zona de totales la fila de tipo + cliente, y el acceso al descuento que la 022 dejó en la fila del Total
- [x] T012 [US2] El menú con las tres acciones, usando web/src/components/ui/menu.tsx (el mismo de OrdersBoardPage), renglones de 48 px y «Vaciar» separado y en rojo
- [x] T013 [US2] El campo inline temporal **entre el encabezado y la lista**, fuera de la caja `maxH="60dvh"` — el porqué está en el plan y en el comentario del aviso de no disponibles
- [x] T014 [US1] Verificar en web/src/features/pos/POSPage.test.tsx que las tres superficies siguen diciendo la misma cifra (anchos 500 y 1024)
- [x] T015 [US2] Test en POSPage.test.tsx: el panel se colapsa con el menú abierto y el menú se va con él
- [x] T023 [US2] **(hueco que encontró analyze)** Test en web/src/domain/pedido.test.ts de FR-006: un nombre de cliente capturado desde el menú sigue viajando en el cuerpo del pedido. Mover dónde se escribe no puede cambiar a dónde llega — es lo único que el dueño pidió conservar para el negocio que sí lo use

---

## Phase 4: User Story 3 — corregir el descuento ya creado (P1)

- [x] T016 [US3] Test en web/src/shared/CobrarSheet.test.tsx: con un pedido con descuento se puede corregir, y lo que se pinta después es **lo que devolvió el servidor**, no una cuenta local
- [x] T017 [US3] Test: un pedido ya cobrado rechaza el cambio y **ninguna cifra se mueve**
- [x] T018 [US3] Test: un pedido sin descuento no muestra un renglón en $0.00, pero sí deja agregar uno
- [x] T019 [US3] Implementar el renglón de descuento en web/src/shared/CobrarSheet.tsx, visible solo cuando el pedido ya existe

---

## Phase 5: Pulido

- [x] T020 Gates: `bun run lint`, `bun run vitest run`, `bun run build`
- [x] T021 Correr `/revision-de-codigo` sobre el diff — es el hook `after_implement` y no es opcional
- [x] T022 Actualizar docs/matriz-de-pantallas.md: por dónde una pantalla podría decir algo que no es cierto con este reacomodo

---

## Dependencias

```text
T001 → T002 → US1+US2 (T003–T015) → US3 (T016–T019) → Pulido (T020–T022)
```

US3 solo necesita T002; puede ir en paralelo con el panel si hiciera falta.

## Lo que NO deja test, y se dice

- **SC-001 (un renglón más de producto)**: jsdom no hace layout, así que ningún test puede contar
  renglones. Lo que sí queda probado es la causa —la fila desapareció (T003)— y la medida está
  calculada contra el código: 52 px, la altura exacta de la fila retirada.
- **SC-002 (ni un toque más en el caso común)**: es una propiedad del flujo, no un valor que se
  pueda aserrar. Verificado contando toques: el tipo nace en «mostrador» y el diseño solo lo muestra,
  así que siguen siendo cero toques antes y después.

## Alcance mínimo entregable

Las historias 1 y 2 juntas. La 3 es la que cierra lo que la 022 dejó abierto y va en la misma
entrega porque es el mismo control, en otra pantalla.
