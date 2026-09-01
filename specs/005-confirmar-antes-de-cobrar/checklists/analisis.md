# Análisis de consistencia — feature 005

**Corrido**: 2026-09-01, antes de `/speckit-implement` (gate obligatorio de la constitución).
**Artefactos**: [spec.md](../spec.md), [plan.md](../plan.md), [tasks.md](../tasks.md).

## Veredicto

**Sin hallazgos CRITICAL ni HIGH. La implementación no está bloqueada.** Siete hallazgos MEDIO/BAJO,
todos remediados en esta misma pasada. El patrón que los agrupa: **la cobertura por tarea es del
100%, pero la cobertura por TEST tiene huecos** — hay requisitos que tienen quién los implemente y
nadie que los verifique, y en este repo eso es documentación optimista.

## Hallazgos

| ID | Categoría | Severidad | Dónde | Qué | Remediación |
| --- | --- | --- | --- | --- | --- |
| C1 | Inconsistencia | MEDIO | contracts/api.md, T017 | `/orders/en-curso` rompe la convención: **todas** las rutas del router son en inglés (`/unpaid`, `/delivered`, `/current`, `/close`, `/levels`). Un endpoint en español obliga a recordar cuál es la excepción | Renombrado a `/orders/open` |
| C2 | Cobertura | MEDIO | FR-009 | Agregar a un pedido **ya cobrado** y que el saldo resultante se vea solo está en el recorrido manual. La constitución es explícita: un defecto que el ensayo manual encuentra y el test no, significa que falta el test | T011b nuevo |
| C3 | Cobertura | MEDIO | SC-003 | "Confirmar y cobrar toma como máximo un toque más" no lo verifica ninguna tarea. Es el criterio que protege la vara de UX del producto y estaba sin dueño | T047b nuevo |
| C4 | Cobertura | MEDIO | FR-016 | La reimpresión de la comanda completa (T040) no tiene test. Es el camino de recuperación cuando la impresora falló: el que más importa que funcione y el que nadie ejercita a diario | T040b nuevo |
| C5 | Subespecificación | MEDIO | FR-011 vs T011 | FR-011 dice **simultáneos**; T011 prueba dos agregados **seguidos**. Un test secuencial no prueba concurrencia, y llamarle así es un verde que engaña | T011 reescrito para nombrar lo que prueba: que agregar es append. La concurrencia real se verifica a mano en el quickstart |
| C6 | Cobertura | BAJO | FR-023 | "Usable con más pedidos de los que caben a lo ancho" no tiene test | Cubierto por T047, que ya trabaja con seis pedidos |
| C7 | Contradicción | BAJO | T003, T005 | Los dos van marcados `[P]` y editan **el mismo archivo** (`domain/order_test.go`). La propia sección de paralelismo lo admite con un "cuidado al editar", que es la señal de que no son paralelos | Se les quitó el `[P]` |

## Cosa que el análisis encontró y no es un defecto

**FR-004 y FR-005 ya están implementados para el camino de confirmar.**
[`KitchenTicket`](../../../web/src/features/tickets/AutoPrintTicket.tsx) ya saca la comanda del
pedido recién mandado, ya recuerda cuál imprimió para no duplicar en cada re-render, y ya avisa
cuando no sale — con el mismo criterio de la feature 001: *"El pedido ya está registrado y se ve en
Pedidos"*.

Las tareas no lo decían, y un implementador podía rehacerlo. Se anotó en `tasks.md`: lo único nuevo
en ese frente es la variante de **agregado**.

## Cobertura de requisitos

| Requisito | ¿Tiene tarea? | Tareas |
| --- | --- | --- |
| FR-001 · confirmar obligatorio | Sí | T023, T027, T030, T031 |
| FR-002 · barrera en el servidor | Sí | T023, T027 |
| FR-003 · sin renglones se rechaza | Sí | T024, T028 |
| FR-004 · comanda al confirmar | **Ya existe** | — (verificado en T046) |
| FR-005 · aviso si no sale | **Ya existe** para confirmar; T035 para el agregado | T035 |
| FR-006 · barra con folio y monto | Sí | T013, T019, T020 |
| FR-007 · del servidor, dos estaciones | Sí | T010, T016, T017 |
| FR-008 · un toque | Sí | T021, T046 |
| FR-009 · agregar a pedido cobrado | Sí | **T011b** |
| FR-010 · rechazar terminal | Sí | T011, T018 |
| FR-011 · agregados se suman | Sí | T011, T046 |
| FR-012 · doble confirmación | Sí | T012 |
| FR-013 · cuenta local vacía | Sí | T012 |
| FR-014 · comanda solo lo agregado | Sí | T033, T034, T038 |
| FR-015 · mismo folio + marca | Sí | T034, T038 |
| FR-016 · reimpresión completa | Sí | **T040b** |
| FR-017 · qué renglones no salieron | Sí | T003, T004, T009 |
| FR-018 · empresa nueva encendida | Sí | T041, T042 |
| FR-019 · existentes no cambian | Sí | T041 |
| FR-020 · pedidos viejos funcionan | Sí | T025 |
| FR-021 · sin alto adicional | Sí | T020, T047 |
| FR-022 · 44 px | Sí | T013 |
| FR-023 · muchos pedidos | Sí | T047 |
| SC-001 · un toque | Sí | T046 |
| SC-002 · rechazo con petición a mano | Sí | T023, T046 |
| SC-003 · un toque más máximo | Sí | **T047b** |
| SC-004 · 30 s entre estaciones | Sí | T046 |
| SC-005 · renglones visibles | Sí | T047 |
| SC-006 · nunca dos veces | Sí | T033 |

## Alineación con la constitución

| Principio | Veredicto |
| --- | --- |
| I. Layering | OK. Las dos reglas puras van a `domain`; nada de SQL fuera de sqlc |
| II. Errores | OK. Dos sentinels nuevos, envueltos con `%w`, mapeados solo en `httpapi.Error` |
| III. Dinero | OK. La feature no calcula dinero nuevo. El único riesgo —que la lista y el total diverjan— está nombrado en el plan y las dos consultas viven en el mismo archivo |
| IV. Test-first y bordes primero | OK tras la remediación. Los bordes se enumeraron en el spec **antes** del plan, y los tres huecos de test (C2, C3, C4) se cerraron aquí |
| V. Seguridad | OK. La barrera está en el servidor y el quickstart la prueba con una petición construida a mano, no confiando en la pantalla |
| VI. YAGNI | OK. Se nombró la pregunta que un estado nuevo respondería, no existe, y no se agrega. Sin índice, sin tabla, sin tiempo real |
| VII. Comentarios | OK. El plan nombra qué decisiones necesitan comentario del porqué |

## Métricas

| | |
| --- | --- |
| Requisitos funcionales | 23 |
| Criterios de éxito | 6 |
| Tareas | 47 → **50** tras la remediación |
| Cobertura por tarea | 100% |
| Hallazgos CRITICAL | 0 |
| Hallazgos HIGH | 0 |
| Hallazgos MEDIO | 5 (remediados) |
| Hallazgos BAJO | 2 (remediados) |
