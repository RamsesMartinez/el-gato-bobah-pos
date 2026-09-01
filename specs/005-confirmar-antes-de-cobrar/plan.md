# Implementation Plan: Confirmar el pedido antes de cobrar, y verlo en curso

**Branch**: `005-confirmar-antes-de-cobrar` | **Date**: 2026-09-01 | **Spec**: [spec.md](./spec.md)

**Input**: [spec.md](./spec.md)

## Summary

Dos huecos que se sostienen entre sí. Cobrar sin que cocina se entere **se puede** —`POST /orders`
con pagos crea y cobra de un golpe— y es el camino corto. Y cuando sí se manda a cocina, la cuenta
desaparece de la pantalla, así que agregarle algo cuesta cinco toques por un camino enterrado en la
hoja de cobro: en producción se ha usado **cero veces**.

El trabajo es más chico de lo que parece porque casi todo existe:

1. **Cerrar el camino corto.** `POST /orders` deja de aceptar pagos; cobrar es solo
   `POST /orders/{id}/pay`, que ya existe.
2. **Que el pedido no desaparezca.** Los pedidos en curso salen del servidor y se pintan como chips
   en la fila de cuentas que ya existe — misma fila, mismo alto, un toque para volver al pedido.
3. **Que lo agregado llegue a cocina solo.** Una columna en el renglón dice si ya salió en una
   comanda; con eso se imprime exactamente lo nuevo.

Lo nuevo de verdad son **una columna, un cambio de default y una consulta**. El resto es mover
barreras y quitar un camino muerto.

## Technical Context

**Language/Version**: Go 1.27 (backend), TypeScript / React 19 (frontend)

**Primary Dependencies**: chi, pgx + sqlc, goose (backend); Chakra UI v3, TanStack Query, Zustand
(frontend)

**Storage**: PostgreSQL con RLS por empresa. Una columna en `order_lines`, un cambio de default en
`business_settings`. Ninguna tabla nueva.

**Testing**: `go test` (unitarios en `domain`, integración contra Postgres real con **dos
empresas**), vitest

**Target Platform**: tabletas de 7 a 10 pulgadas, presupuesto real ~1024×600, táctil

**Project Type**: aplicación web (backend Go + frontend React), monorepo

**Performance Goals**: agregar a un pedido en curso en **un toque**. La barra se refresca cada 30 s,
igual que la píldora que reemplaza — no hace falta tiempo real.

**Constraints**: controles de ≥44 px; prohibido `<select>` nativo; la barra **no puede consumir alto
nuevo**. El toque extra de confirmar es el único que la feature puede cobrar.

**Scale/Scope**: 4 a 10 pedidos por día, 2.2 renglones de promedio, 2 estaciones, 2 empresas en la
misma base. Ninguna pantalla nueva: cambian tres que ya existen.

## Constitution Check

*GATE: revisado antes de Fase 0 y de nuevo tras el diseño de Fase 1.*

| Principio | Cómo lo cumple este plan |
| --- | --- |
| **I. Layering estricto** | "Qué renglones no han salido a cocina" y "se puede agregar a este pedido" van a `domain` como funciones puras. El servicio orquesta y maneja la transacción; el handler decodifica y mapea. SQL solo por sqlc |
| **II. Errores envueltos** | El rechazo de crear-con-pagos y el de agregar a un pedido terminal son sentinels nuevos envueltos con `%w`; el mapeo a HTTP sigue viviendo solo en `httpapi.Error` |
| **III. Dinero** | La feature **no calcula dinero nuevo**: mueve dónde se cobra. El único cuidado es que el saldo pendiente que la barra muestra salga del mismo predicado que la lista, y no de dos consultas que puedan divergir |
| **IV. Test-first, bordes primero** | Los bordes están enumerados en el spec ANTES del código, agrupados por las cuatro familias. La columna nueva y su backfill son de **integración con dos empresas**; las reglas puras son unitarios en `domain` |
| **V. Seguridad adversarial** | Detalle abajo |
| **VI. YAGNI** | Sin estado nuevo en la orden: se nombró la pregunta que respondería y no existe. Sin tabla nueva, sin tiempo real, sin bloqueo pesimista, sin índice para una columna que se filtra dentro de un pedido de 6 renglones |
| **VII. Comentarios del porqué** | Cada decisión no obvia —por qué la marca va en el renglón y no en el pedido, por qué el backfill deja `NULL`, por qué la barra es una unión de dos conjuntos— va como comentario donde vive |

### Principio V, en detalle

| Pregunta adversarial | Respuesta concreta |
| --- | --- |
| ¿Puedo cobrar saltándome la cocina? | No. `POST /orders` rechaza `payments` en el **servidor**. Esconder el botón no es la barrera, y el quickstart lo prueba con una petición a mano |
| ¿Puedo agregarle renglones al pedido de otra empresa? | No. RLS acota `orders` y `order_lines` por empresa; el test de integración corre bajo el rol de aplicación, no como owner |
| ¿Puedo agregar renglones a un pedido terminado para inflar una venta cerrada? | No: se rechaza con `409`. El estado va en el mensaje para que la tableta suspendida sepa qué pasó |
| ¿Puedo dejar deuda invisible agregando a un pedido ya cobrado? | Se permite agregar, pero el saldo resultante **aparece en la barra**. Es la regla que el dueño puso: si se cobró, no hay deuda escondida |
| ¿Un reintento crea dos pedidos con lo mismo? | No, si el identificador de la cuenta sobrevive al reintento. Hoy **no sobrevive** — es un defecto real que este plan arregla |
| ¿Un error de la impresora pierde el pedido? | No. El pedido queda confirmado y el aviso informa sin bloquear, igual que el ticket del cliente desde la feature 001 |
| ¿El cambio de default enciende algo en un negocio en operación? | No. Cambiar el `DEFAULT` de la columna no toca filas existentes, y el quickstart lo verifica leyendo los ajustes de `gatobobah` antes y después |

**Sin violaciones que justificar.** La sección de complejidad va vacía.

## Project Structure

### Documentation (this feature)

```text
specs/005-confirmar-antes-de-cobrar/
├── plan.md              # Este archivo
├── research.md          # Fase 0 — diez hallazgos, todos medidos
├── data-model.md        # Fase 1 — una columna, un default, y qué NO cambia
├── quickstart.md        # Fase 1 — un recorrido por historia, con su fallo esperado
├── contracts/api.md     # Fase 1 — uno se cierra, dos cambian, cero nuevos
├── checklists/
└── tasks.md             # Lo crea /speckit-tasks
```

### Source Code

```text
server/
├── migrations/
│   ├── 00NN_renglon_enviado_a_cocina.sql     # columna en order_lines
│   └── 00NN_comanda_por_default.sql          # default de print_kitchen_ticket
├── queries/
│   └── orders.sql                             # en-curso (unión); renglones sin enviar; marcar enviados
├── internal/
│   ├── domain/
│   │   ├── order.go                           # PuedeAgregar; RenglonesSinEnviar
│   │   └── order_test.go
│   ├── app/
│   │   └── orders.go                          # Create rechaza pagos; AddLines marca y devuelve lo agregado
│   ├── httpapi/
│   │   ├── handlers_orders.go                 # 422 al crear con pagos; 409 al agregar a terminal
│   │   └── router.go                          # /orders/unpaid → /orders/en-curso
│   └── integration/
│       ├── cobrar_exige_confirmar_test.go
│       ├── comanda_del_agregado_test.go
│       └── comanda_por_default_test.go        # con DOS empresas

web/src/
├── features/pos/
│   ├── POSPage.tsx                            # la fila ya existe; entra la barra
│   ├── TicketTabs.tsx                         # chips locales + chips del servidor
│   ├── PedidosEnCurso.tsx                     # los chips del servidor (absorbe PorCobrarPill)
│   ├── CheckoutSheet.tsx                      # deja de crear pedidos; se va el selector viejo
│   ├── useMandarPedido.ts                     # el uuid deja de regenerarse por intento
│   └── useAgregarAPedido.ts                   # agregar + comanda del agregado
├── stores/ticket.ts                            # el uuid vive en la cuenta
└── utils/printKitchen.ts                       # variante "agregado" del mismo documento
```

**Structure Decision**: monorepo existente. Una sola carpeta gana archivos (`features/pos/`) y una
desaparece (`PorCobrarPill`, absorbida). Ninguna pantalla nueva.

## Fases

### Fase 0 — Investigación ✅

Ver [research.md](./research.md). Diez hallazgos. Los tres que cambian el diseño:

- **La barrera es una condición, no un rediseño**: cobrar ya tiene su endpoint propio.
- **El pedido no necesita estado nuevo; el renglón sí necesita una marca.** Se nombró la pregunta y
  no existe para el pedido; sí existe para el renglón.
- **La barra no cuesta alto**: la fila de cuentas ya está y las cuentas ya scrollean solas.

### Fase 1 — Diseño ✅

- [data-model.md](./data-model.md) — una columna, un default, y por qué el backfill deja `NULL`
- [contracts/api.md](./contracts/api.md) — el `422` al crear con pagos y el `409` al agregar a un
  terminal, más el defecto de idempotencia del front
- [quickstart.md](./quickstart.md) — un recorrido por historia con el fallo esperado de cada paso

### Fase 2 — Tareas

La genera `/speckit-tasks`. El orden que sugiere el diseño:

1. **La columna y el default** (migraciones + consultas). No cambian comportamiento solos.
2. **US1, el pedido en curso.** Es el MVP y se puede entregar sola: la tableta deja de perder el
   pedido y agregar baja de cinco toques a uno.
3. **US2, cobrar exige confirmar.** Va después de la 1 a propósito: entregarla antes dejaría el
   flujo peor que hoy.
4. **US3, la comanda del agregado.** Necesita la columna y la 1.
5. **US4, el default encendido.** P3, independiente de todo lo demás.

## Riesgos, y qué hace el plan con cada uno

| Riesgo | Qué haría | Cómo se cierra |
| --- | --- | --- |
| Entregar US2 antes que US1 | El pedido se confirma, desaparece, y cobrarlo cuesta **más** toques que hoy: la feature empeora el POS | El orden de fases lo prohíbe y las prioridades del spec lo dicen |
| El uuid que se regenera por intento | Un corte de red produce dos pedidos con lo mismo, y el operador cobra uno y deja el otro abierto | Es un defecto **actual**, no un riesgo nuevo. El uuid pasa a la cuenta y deja su test |
| Fundir la píldora perdiendo el entregado-sin-cobrar | Desaparece del encabezado el pendiente más caro: el cliente ya se fue | La barra es la **unión** de dos conjuntos, no el reemplazo de uno por otro. Está en el modelo de datos |
| `SeedBusinessSettings` nombrando la columna | El cambio de default queda decorativo y las empresas nuevas siguen naciendo apagadas | Se verifica el sembrado, no se asume; el test provisiona una empresa y lee el ajuste |
| Marcar como enviados los renglones viejos | Se afirma que salieron papeles que nadie vio | El backfill deja `NULL` = "no se sabe", que es la verdad |
| La barra comiéndose la lista de productos | El operador ve menos productos por pantalla en 7" | Los chips van en la fila que ya existe, y el quickstart cuenta los renglones antes y después |

## Complexity Tracking

Sin violaciones a la constitución. Nada que justificar.
