---

description: "Tareas de la feature 005 — confirmar antes de cobrar, y el pedido en curso"
---

# Tasks: Confirmar el pedido antes de cobrar, y verlo en curso

**Input**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md),
[quickstart.md](./quickstart.md)

**Tests**: sí, y **antes** del código. Es no negociable en este repo (constitución, principio IV):
primero el test que falla, viéndolo fallar por la razón correcta, luego el código. Una migración sin
su test de integración la rechaza el pre-commit.

## Format: `[ID] [P?] [Story] Descripción con la ruta`

`[P]` = puede correr en paralelo (archivos distintos, sin dependencias pendientes).

## Path Conventions

Monorepo: backend en `server/`, frontend en `web/`. Migraciones goose embebidas en
`server/migrations/`; SQL solo por sqlc (`server/queries/` + `make sqlc`).

---

## Phase 1: Fundacional (bloquea todo lo demás)

**Propósito**: la columna, el default y las reglas puras. Nada de esto cambia comportamiento por sí
solo, y todo lo demás depende de ello.

- [X] T001 Escribir el test de integración de la columna nueva en `server/internal/integration/renglon_enviado_a_cocina_test.go`: con **dos empresas**, verifica que los renglones que ya existían quedan en `NULL` tras la migración y que un renglón nuevo también nace en `NULL`. Verlo fallar antes de T002.
- [X] T002 Crear `server/migrations/0053_renglon_enviado_a_cocina.sql`: `order_lines.enviado_a_cocina_at timestamptz` nullable, con su `Down`. El comentario explica por qué va en el renglón y no en el pedido, y por qué el backfill NO marca nada. Va en el **mismo commit** que T001.
- [X] T003 Escribir el test unitario de `RenglonesSinEnviar` en `server/internal/domain/order_test.go`: table-driven, incluyendo pedido sin renglones, todos enviados, y ninguno enviado.
- [X] T004 Implementar `domain.RenglonesSinEnviar` en `server/internal/domain/order.go`: función pura que devuelve los renglones con `enviado_a_cocina_at` vacío.
- [X] T005 Escribir el test unitario de `PuedeAgregar` en `server/internal/domain/order_test.go`: acepta `abierta` y `lista`, rechaza `entregada`, `cancelada` y `reembolsada`.
- [X] T006 ~~Implementar `domain.PuedeAgregar`~~ — **la regla ya existía** como `domain.PuedeRecibirLineas`, sin test. Se borró la duplicada que había planeado y la que ya estaba se quedó con la cobertura de T005.
- [X] T007 ~~Agregar `domain.ErrPedidoTerminal`~~ — **innecesario**: `AddLines` ya envuelve `ErrConflict` con el número y el estado del pedido, que es lo que FR-010 pide. Un sentinel más no respondía ninguna pregunta nueva.
- [X] T008 Agregar el sentinel `domain.ErrCobroFueraDeLugar` en `server/internal/domain/order.go` (crear un pedido ya cobrado) y su mapeo a `422` en `server/internal/httpapi/respond.go`.
- [X] T009 Escribir en `server/queries/orders.sql` las consultas nuevas —pedidos en curso (la unión de no-terminales y con-saldo), renglones sin enviar de un pedido, y marcar renglones como enviados— y correr `make sqlc`. El `where` de la lista y el del total pendiente viven en el mismo archivo y se editan juntos.

**Checkpoint**: `go build ./...` y `go test ./...` en verde; la migración aplicada contra la base de pruebas.

---

## Phase 2: User Story 1 — El pedido confirmado sigue a la vista (P1) 🎯 MVP

**Meta**: el pedido deja de desaparecer y agregarle cuesta **un toque** en vez de cinco.

**Prueba independiente**: confirmar un pedido, verlo como chip con su folio, tocarlo, agregarle un
producto — todo sin abrir la pantalla de cobro.

### Tests primero

- [X] T010 [P] [US1] Test de integración en `server/internal/integration/pedidos_en_curso_test.go`: la lista de en curso trae un pedido `abierta` sin pagos, trae uno `entregada` con saldo, y **no** trae uno cancelado ni uno entregado y pagado. Con dos empresas: la de una no ve la de la otra.
- [X] T011 [P] [US1] Test de integración en `server/internal/integration/agregar_a_pedido_test.go`: agregar a un pedido `entregada` devuelve `ErrPedidoTerminal`; agregar a uno `abierta` suma los renglones; **dos agregados seguidos suman los dos**, que es la propiedad de la que depende FR-011. La concurrencia real no se prueba aquí —un test de goroutines pasaría por el número de núcleos, no por el código— sino a mano en el quickstart.
- [X] T011b [P] [US1] Test de integración en `server/internal/integration/agregar_a_pedido_test.go`: agregar a un pedido **ya cobrado por completo** lo deja con saldo pendiente, y ese saldo aparece en la lista de pedidos en curso. Es FR-009 y es la regla que el dueño puso — si se cobró, no puede quedar deuda escondida.
- [X] T012 [P] [US1] Test en `web/src/features/pos/useMandarPedido.test.ts`: al confirmar, la cuenta local queda vacía; y **el mismo uuid se manda en el reintento** — es el defecto que hoy produce dos pedidos tras un corte de red.
- [X] T013 [P] [US1] Test en `web/src/features/pos/PedidosEnCurso.test.tsx`: pinta un chip por pedido con su folio y su monto, con altura de 44 px, y no pinta nada cuando no hay ninguno.

### Implementación

- [X] T014 [US1] Mover el `clientUuid` del intento a la cuenta en `web/src/stores/ticket.ts`: se genera al abrir la cuenta y sobrevive al reintento.
- [X] T015 [US1] Consumir ese uuid en `web/src/features/pos/useMandarPedido.ts` en vez de llamar a `uuid()` dentro de `mutationFn`.
- [X] T016 [US1] Implementar el servicio de pedidos en curso en `server/internal/app/orders.go` sobre la consulta de T009, devolviendo por pedido el grupo al que pertenece y el saldo.
- [X] T017 [US1] Cambiar la ruta `/orders/unpaid` por `/orders/open` en `server/internal/httpapi/router.go` y su handler en `server/internal/httpapi/handlers_orders.go`. El nombre viejo miente —la lista ya no es solo de impagos— y el nuevo va en inglés como todas las demás rutas del router.
- [X] T018 [US1] Aplicar `domain.PuedeAgregar` en `AddOrderLines` (`server/internal/app/orders.go`) y mapear el rechazo con el estado en el mensaje.
- [X] T019 [US1] Crear `web/src/features/pos/PedidosEnCurso.tsx`: chips de 44 px con folio y monto, desplazamiento horizontal, y el total pendiente a la vista. Absorbe lo que hacía `PorCobrarPill`.
- [X] T020 [US1] Montar los chips en la fila que ya existe de `web/src/features/pos/POSPage.tsx`, junto a `TicketTabs`, y quitar `PorCobrarPill`. **No se agrega alto**: es el presupuesto de SC-005.
- [X] T021 [US1] Crear `web/src/features/pos/useAgregarAPedido.ts`: tocar un chip abre el pedido y los productos que se agreguen entran por `POST /orders/{id}/lines`.
- [X] T022 [US1] Quitar el selector "Agregar a un pedido en curso" de `web/src/features/pos/CheckoutSheet.tsx`. Dos caminos para lo mismo, uno escondido, es de donde salen los defectos.

**Checkpoint**: US1 entregable sola. El POS ya no pierde el pedido y agregar cuesta un toque.

---

## Phase 3: User Story 2 — Cobrar exige haber confirmado (P1)

**Meta**: no se puede cobrar un pedido que cocina no vio. La barrera vive en el servidor.

**Depende de US1**: sin ella, confirmar hace desaparecer el pedido y cobrarlo cuesta más toques que
hoy — la feature empeoraría el POS.

### Tests primero

- [X] T023 [P] [US2] Test de integración en `server/internal/integration/cobrar_exige_confirmar_test.go`: `CreateOrder` con pagos devuelve `ErrCobroFueraDeLugar`; sin pagos crea el pedido; y cobrar ese pedido con el camino de `/pay` funciona.
- [X] T024 [P] [US2] Test de integración en el mismo archivo: crear un pedido con `lines` vacío se rechaza — un pedido de cero renglones ocuparía folio y sacaría una comanda en blanco.
- [X] T025 [P] [US2] Test de integración en el mismo archivo: **los pedidos que ya existían** siguen siendo cobrables y entregables. Es FR-020 y protege a producción.
- [X] T026 [P] [US2] Test en `web/src/features/pos/CheckoutSheet.test.tsx`: la hoja de cobro ya no puede crear un pedido; con una cuenta sin confirmar, cobrar no está disponible.

### Implementación

- [X] T027 [US2] Rechazar `Payments` no vacío en `CreateOrder` (`server/internal/app/orders.go`), envuelto con `%w` sobre el sentinel de T008, con un mensaje que nombra el camino correcto.
- [X] T028 [US2] Rechazar `lines` vacío en la misma ruta, con `domain.ErrValidation`.
- [X] T029 [US2] Quitar `payments` del cuerpo aceptado en `server/internal/httpapi/handlers_orders.go` y de `web/src/api/pos.ts`.
- [X] T030 [US2] Dejar `CheckoutSheet` cobrando **solo** pedidos que existen, por `POST /orders/{id}/pay` (`web/src/features/pos/CheckoutSheet.tsx`).
- [X] T031 [US2] Renombrar la acción del panel del pedido a **Confirmar** en `web/src/features/pos/POSPage.tsx`, y dejar cobrar disponible solo desde un pedido en curso.
- [X] T032 [US2] Comprobar que cobrar desde el tablero `/pedidos` sigue funcionando y no quedó pidiendo una confirmación que ahí no aplica (`web/src/features/orders/CobrarSheet.tsx`).

**Checkpoint**: ningún pedido llega al cobro sin haber pasado por cocina, y el servidor lo sostiene.

---

## Phase 4: User Story 3 — Lo agregado sale a cocina solo, marcado (P2)

**Meta**: cocina recibe únicamente lo nuevo, con el mismo folio.

> **Lo que YA existe y no se rehace**: `KitchenTicket` ya saca la comanda del pedido recién mandado,
> ya recuerda cuál imprimió para no duplicarla en cada re-render, y ya avisa cuando no sale
> ([AutoPrintTicket.tsx](../../web/src/features/tickets/AutoPrintTicket.tsx)). FR-004 y FR-005
> quedan cubiertos para el camino de confirmar. Lo nuevo es **solo la variante de agregado**.

### Tests primero

- [X] T033 [P] [US3] Test de integración en `server/internal/integration/comanda_del_agregado_test.go`: tras agregar, **solo** los renglones agregados quedan marcados como enviados, y la respuesta los identifica.
- [X] T034 [P] [US3] Test en `web/src/utils/printKitchen.test.ts`: la comanda de agregado lleva solo los renglones nuevos, el mismo folio, la marca de agregado, y **sin precios**.
- [X] T035 [P] [US3] Test en `web/src/features/pos/useAgregarAPedido.test.ts`: si la impresión falla, el renglón queda agregado igual y sale un aviso. Es el modo de fallo que la feature 001 ya quitó del ticket del cliente.

### Implementación

- [X] T036 [US3] Marcar los renglones agregados como enviados dentro de la misma transacción de `AddOrderLines` (`server/internal/app/orders.go`), y devolver cuáles son.
- [X] T037 [US3] Marcar como enviados los renglones del pedido al crearlo, cuando la comanda está encendida (`server/internal/app/orders.go`).
- [X] T038 [US3] Agregar la variante de agregado a `web/src/utils/printKitchen.ts`: mismo documento, encabezado marcando **AGREGADO**, solo los renglones nuevos.
- [X] T039 [US3] Disparar esa comanda desde `useAgregarAPedido`, con el aviso no bloqueante si no sale (`web/src/features/pos/useAgregarAPedido.ts`).
- [X] T040b [P] [US3] Test en `web/src/features/orders/` de la reimpresión completa: sale la comanda con **todos** los renglones del pedido, incluidos los que ya habían salido. Es el camino de recuperación cuando la impresora falló, y el que nadie ejercita a diario.
- [X] T040 [US3] Dejar la reimpresión de la comanda **completa** como acción explícita en el tablero de pedidos (`web/src/features/orders/`).

**Checkpoint**: cocina nunca prepara dos veces el mismo renglón.

---

## Phase 5: User Story 4 — La empresa nueva nace imprimiendo (P3)

- [X] T041 [P] [US4] Test de integración en `server/internal/integration/comanda_por_default_test.go`: una empresa provisionada después de la migración nace con la comanda encendida, y una que ya existía con el ajuste apagado **no cambia**. Con dos empresas. Verlo fallar antes de T042.
- [X] T042 [US4] Crear `server/migrations/0054_comanda_por_default.sql`: cambia el `DEFAULT` de `business_settings.print_kitchen_ticket` a `true`, con su `Down`. El comentario explica que no toca ninguna fila existente. Mismo commit que T041.

---

## Phase 6: Cierre

- [X] T043 Correr el `tablet-ui-reviewer` sobre `web/src/features/pos/`: la barra cambia la pantalla más usada del sistema y el presupuesto de alto es lo primero que se pierde.
- [X] T044 Correr el `db-architect` sobre las migraciones 0053 y 0054 y las consultas nuevas de `server/queries/orders.sql`, antes de aplicarlas a producción.
- [X] T045 Correr el `go-backend-reviewer` sobre los cambios de `server/internal/app/orders.go` y `server/internal/httpapi/handlers_orders.go`.
- [X] T046 Recorrer [quickstart.md](./quickstart.md) completo contra el ambiente de pruebas, en una ventana de **1024×600**, incluyendo los cinco bordes a mano.
- [X] T047b Contar los toques de punta a punta de una venta de dos renglones, antes y después de la feature. **Máximo uno más** (SC-003). Si son dos o más, el diseño de la barra o el de confirmar está cobrando toques que no le tocan.
- [ ] T047 Contar los renglones de productos visibles antes y después con seis pedidos en curso (SC-005). Si bajaron, la barra se está comiendo alto que no le toca.

---

## Dependencias

```text
Fase 1 (T001–T009)  ← bloquea todo
      ↓
Fase 2 · US1 (T010–T022)  🎯 MVP, entregable sola
      ↓
Fase 3 · US2 (T023–T032)  ← DEBE ir después de US1
      ↓
Fase 4 · US3 (T033–T040)  ← necesita la columna y US1
Fase 5 · US4 (T041–T042)  ← independiente; puede ir en cualquier momento tras la Fase 1
      ↓
Fase 6 (T043–T047)
```

**La única dependencia que no se puede romper**: US2 después de US1. Entregar "cobrar exige
confirmar" con el pedido desapareciendo deja el POS peor de lo que está.

## Paralelismo

- Fase 1: T003 y T005 NO son paralelas — editan el mismo archivo. T001 va antes que T002 por el
  hook de migración.
- Fase 2: T010–T013 los cuatro juntos, son archivos distintos.
- Fase 3: T023–T026 juntos.
- Fase 4: T033–T035 juntos.
- Fase 5 corre en paralelo a las fases 2–4 en cuanto la Fase 1 esté.

## Alcance mínimo

**US1 sola ya entrega valor**: el POS deja de perder el pedido y agregarle baja de cinco toques a
uno, que es el número que hoy está en cero usos.

## Fase 6 — US5: cobrar un pedido en curso, entero o por pedazos

Nació de un reporte del dueño desde la tableta: *"le faltan varios elementos al cobro, no veo los
demás elementos de dividir la cuenta o la propina"*. Es un hueco que abrió esta misma feature al
mover el cobro a un pedido que ya existe.

Los cuatro primeros son de servidor y van antes: sin ellos la división en la pantalla no se puede
hacer segura, y los cuatro se midieron contra Postgres real antes de escribir nada.

- [X] T051 [US5] Test de integración: dos llamadas idénticas de media cuenta no cobran dos veces la
      misma mitad, en `server/internal/integration/dividir_la_cuenta_test.go`
- [X] T052 [US5] Llave de idempotencia del cobro: `order_payments.client_uuid` con índice único por
      `(company_id, client_uuid)`, en `server/migrations/0057_cobro_idempotente.sql`
- [X] T053 [US5] La llave se sella contra la CARGA del pago (método, monto, propina): un reintento
      con otro método se rechaza, en `server/internal/app/orders.go`
- [X] T054 [US5] Tope de propina contra el total de la cuenta, por pago, en
      `server/internal/domain/cobro.go`
- [X] T055 [US5] Un solo predicado de "pedido saldado": `domain.PedidoSaldado`, y `PorCobrar` y
      `ListOpenOrders` responden a él (el centavo del residuo dejaba deuda fantasma)
- [X] T056 [US5] La caja abierta se lee y se bloquea DENTRO de la transacción del cobro, y la llave
      se consulta antes que ella, en `server/internal/app/orders.go`
- [X] T057 [US5] El cobro devuelve lo que falta en vez de `{"ok":true}`; `OrderView` trae
      `outstanding`, en `server/internal/httpapi/handlers_orders.go`
- [X] T058 [US5] Cobrar emite su evento SSE — era la única mutación de pedido que no avisaba
- [X] T059 [US5] Módulo puro de aritmética del cobro con su test ANTES: `web/src/features/pos/cobro.ts`
      y `cobro.test.ts` (parser que distingue ausente de inválido, reparto con residuo en la última
      parte, billetes filtrados, cambio, presets de propina)
- [X] T060 [US5] `CobrarSheet` cobra un pedazo a la vez, con propina, atajos de reparto, "el cambio
      es propina", errores traducidos y el faltante del pedido vivo, en
      `web/src/features/orders/CobrarSheet.tsx` y su `.test.tsx`
- [X] T061 [US5] `CheckoutSheet` migrado al módulo compartido, en el MISMO cambio: dejarlo con su
      copia repetiría el precedente de `build()` contra `armarPedido.ts`
- [X] T062 [US5] La barra del POS pierde los chips y la lista pasa a una hoja con "Agregar" y
      "Cobrar" por renglón, en `web/src/features/pos/PedidosEnCurso.tsx`
- [X] T063 [US5] `invalidateAll` y el SSE invalidan el prefijo `['orders']` completo, no solo
      `active`
- [ ] T064 [US5] Ensayo en dev: cobrar una cuenta repartida entre tres y comparar el corte contra el
      efectivo, con las propinas separadas por método (SC-009)
