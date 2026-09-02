---

description: "Tareas de la feature 006 — la hora del negocio manda"
---

# Tasks: La hora del negocio manda

**Input**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md),
[quickstart.md](./quickstart.md)

**Tests**: sí, y **antes** del código (constitución, principio IV). Una migración sin su test de
integración la rechaza el pre-commit.

## Format: `[ID] [P?] [Story] Descripción con la ruta`

`[P]` = archivos distintos y sin dependencias pendientes.

## Path Conventions

Monorepo: `server/` y `web/`. Migraciones goose embebidas; SQL solo por sqlc.

---

## Phase 1: El defecto que mueve dinero de día

**Va primero y solo**: es un defecto vivo, no depende de nada, y su consecuencia —dinero en el
arqueo equivocado— es peor que todo lo demás junto.

- [X] T001 Test de integración en `server/internal/integration/fecha_de_negocio_sin_zona_test.go`: abrir caja cuando no se puede leer la zona debe fechar el turno con el default del producto, no con UTC. Se provoca borrando la fila de ajustes y se verifica a una hora que en UTC ya sea del día siguiente. Con **dos empresas**. Verlo fallar antes de T002.
- [X] T002 Cambiar el fallback de `BackofficeService.businessDate` en `server/internal/app/backoffice.go`: de `time.UTC` a `domain.LoadBusinessLocation(domain.DefaultTimezone)`. El comentario explica que la intención del fallback era correcta —no tumbar la apertura de caja— pero el valor no: seis horas de corrimiento silencioso metían el turno en el día siguiente.
- [X] T002b Test de integración en `server/internal/integration/el_dia_de_la_venta_no_cambia_test.go`: sembrar pedidos con su fecha de negocio, correr todo lo que esta feature toca —abrir caja, listar en curso, listar entregados, cambiar la zona del negocio— y verificar que **ninguna `business_date` cambió**. Es FR-015, la invariante que hace segura toda la feature, y hasta ahora solo se verificaba a mano.

**Checkpoint**: `go test ./...` en verde. Nada visible cambió; dejó de moverse dinero de día.

---

## Phase 2: Fundacional (bloquea las historias 3 y 4)

- [X] T003 Test unitario en `server/internal/domain/businessdate_test.go` de `DesdeCuandoSeVen(modo, ahora, zona, turno)`: table-driven con los tres modos, **incluyendo el día del cambio de horario en `America/Tijuana`** — ahí la distancia entre dos medianoches es de 23 o 25 horas y un cálculo que resta 24 se desfasa.
- [X] T004 Implementar `domain.DesdeCuandoSeVen` en `server/internal/domain/businessdate.go`: función pura que devuelve desde qué instante se ven los entregados. La medianoche se calcula EN la zona, nunca restando horas.
- [X] T005 Test de integración en `server/internal/integration/corte_de_vista_test.go`: el ajuste nace en `medianoche`, acepta los tres valores y **rechaza** cualquier otro. Con dos empresas, verificando que el de una no cambia el de la otra. Verlo fallar antes de T006.
- [X] T006 Crear `server/migrations/0055_corte_de_vista.sql`: `business_settings.corte_de_vista text not null default 'medianoche'` con su check de los tres valores y su `Down`. El comentario dice que decide solo QUÉ SE MUESTRA, nunca de qué día es una venta. Mismo commit que T005.
- [X] T007 Exponer el ajuste en `server/internal/app/settings.go` y `server/internal/httpapi/handlers_settings.go`: un valor fuera de los tres es `domain.ErrValidation`. El default es para el campo **ausente**, nunca para el presente y malformado.

**Checkpoint**: el ajuste se guarda y se lee; ninguna pantalla lo usa todavía.

---

## Phase 3: User Story 1 — La hora del local en todas las pantallas (P1) 🎯 MVP

**Meta**: dos tabletas con el reloj distinto muestran la misma hora, y el papel también.

**Prueba independiente**: cambiar la zona del sistema operativo y comprobar que nada cambia en
pantalla.

### Tests primero

- [X] T008 [P] [US1] Test en `web/src/hooks/useHoraDelNegocio.test.tsx`: con la zona del negocio en `America/Mexico_City` y el entorno en otra, formatea en la del negocio; sin ajustes cargados **no devuelve una hora**; con una zona que el navegador rechaza cae al default y no lanza.
- [X] T009 [P] [US1] Test en `web/src/utils/printReceipt.test.ts`: el ticket lleva la hora de la zona que se le pasa, no la del entorno.
- [X] T010 [P] [US1] Test en `web/src/utils/printKitchen.test.ts`: lo mismo para la comanda.
- [X] T011 [P] [US1] Test de guardia en `web/src/utils/formateoUnico.test.ts`: recorre `web/src` y **falla si algún archivo llama a `toLocaleString`/`toLocaleTimeString`/`toLocaleDateString` fuera del helper**. **El helper se recorta antes de buscar**: ahí llamarlo es su trabajo, y sin recortarlo el test falla contra su propia implementación — el mismo tropiezo que ya costó una corrección en el guardia equivalente de Go. Es lo que impide que el problema vuelva por una pantalla nueva; sin él, la migración de hoy se deshace sola en tres meses.

### Implementación

- [X] T012 [US1] Crear `web/src/hooks/useHoraDelNegocio.ts`: **el único lugar** que convierte. Lee la zona de los ajustes, cae al default del producto si no hay, y devuelve un indicador de "todavía no sé la zona" para que nadie pinte una hora que después se corrija.
- [X] T012b [US1] Dejar constancia cuando la zona guardada no se puede aplicar (FR-006), con su test en `web/src/hooks/useHoraDelNegocio.test.tsx`. Sin esto, un negocio con la zona rota se comporta bien y nadie se entera nunca: la pantalla cae al default en silencio, que es el modo de fallo que esta feature vino a quitar.
- [X] T013 [US1] Mover el formateo de fecha y hora de `web/src/utils/format.ts` a que reciba la zona. Es la pieza de la que cuelgan las demás.
- [X] T014 [US1] Pasar la zona a `web/src/utils/printReceipt.ts` y `web/src/utils/printKitchen.ts`. `toTicketBusinessInfo` ya arma el encabezado desde los ajustes: la zona entra por ahí.
- [X] T015 [P] [US1] Migrar `web/src/features/backoffice/CashPage.tsx` (5 sitios).
- [X] T016 [P] [US1] Migrar `web/src/features/sales/SalesPage.tsx` y `web/src/features/sales/SaleDetailDialog.tsx`.
- [X] T017 [P] [US1] Migrar `web/src/features/backoffice/StockPage.tsx` y `web/src/app/SystemInfo.tsx`.
- [X] T018 [US1] Que ninguna pantalla pinte una hora antes de conocer la zona (SC-003). Una hora que se corrige sola enseña al operador a no confiar en lo que lee.

**Checkpoint**: US1 entregable sola. El ticket deja de mentir sobre cuándo fue la venta.

---

## Phase 4: User Story 2 — Los pedidos activos no desaparecen (P1)

**Meta**: un pedido abierto se ve hasta que alguien lo cierre, sin importar de qué día sea.

**Independiente de US1**: se puede entregar antes o después.

### Tests primero

- [X] T019 [P] [US2] Test de integración en `server/internal/integration/pedidos_viejos_se_ven_test.go`: un pedido abierto de hace dos meses sale en la lista; uno entregado y pagado no; uno entregado sin pagar sí. Con dos empresas: la de una no ve la de la otra.
- [X] T020 [P] [US2] Test en `web/src/features/pos/PedidosEnCurso.test.tsx`: un pedido de un día anterior se distingue a la vista de los de hoy.

### Implementación

- [X] T021 [US2] Quitar el filtro de fecha de `ListOpenOrders` en `server/queries/orders.sql` y devolver la fecha de negocio de cada pedido. **Retira el arreglo parcial de la feature 005**, que ató la lista al turno abierto; no se acumulan los dos. Revisar qué pasa con `TestLaBarraSigueMostrandoElTurnoQueCruzoLaMedianoche`, que fijó ese comportamiento: si sobrevive afirmando lo viejo, es un verde que engaña — o se reescribe contra la regla nueva, o se dice por qué sigue valiendo.
- [X] T022 [US2] Ajustar `OrdersService.Open` en `server/internal/app/orders.go`: sin fecha, y quitar la consulta del turno que ya no hace falta.
- [X] T023 [US2] Marcar en `web/src/features/pos/PedidosEnCurso.tsx` los pedidos de días anteriores, para que el rezago se note en vez de confundirse con el trabajo de hoy.

**Checkpoint**: los once pedidos de julio de la cuenta de pruebas vuelven a ser visibles y cerrables.

---

## Phase 5: User Story 3 — "Entregados hoy" se vacía cuando el negocio dice (P2)

### Tests primero

- [X] T024 [P] [US3] Test de integración en `server/internal/integration/corte_de_vista_test.go`: con el corte en `medianoche`, un pedido entregado a las 23:00 locales sigue en la lista y a las 00:01 ya no — **con el reloj del servidor en UTC**, que es lo que hoy la vacía a las 18:00.
- [X] T025 [P] [US3] Test en el mismo archivo para `turno` y `cierre_de_caja`.
- [X] T026 [P] [US3] Test en `web/src/features/admin/BusinessSettingsPage.test.tsx`: el selector de corte usa `Picker`, no `<select>` nativo, y sus opciones miden al menos 44 px.

### Implementación

- [X] T027 [US3] Cambiar `ListDeliveredToday` en `server/queries/orders.sql` para filtrar desde un instante en vez de por fecha del servidor.
- [X] T028 [US3] Usar `domain.DesdeCuandoSeVen` en `OrdersService.DeliveredToday` (`server/internal/app/orders.go`).
- [X] T028b [US3] Test de integración: las cifras de un arqueo **ya cerrado** son idénticas antes y después de cambiar el `corte_de_vista` y la zona del negocio (FR-016, SC-006). El corte de vista es de pantalla; si toca una cifra de dinero, está mal construido.
- [X] T029 [US3] Agregar el selector de corte a `web/src/features/admin/BusinessSettingsPage.tsx` con `Picker`. El texto dice qué hace en un renglón; el porqué de cada modo va en el diálogo de ayuda, no en la pantalla.
- [X] T030 [US3] Que la lista refleje el corte sin recargar (FR-014).
- [X] T030b [P] [US3] Test de que la lista se vacía al cruzar el corte **sin recargar**: se avanza el reloj y se comprueba que la lista cambia sola. Sin él, FR-014 queda implementado y sin nadie que lo mire.

---

## Phase 6: User Story 4 — Cambiar la zona no asusta (P3)

- [X] T031 [P] [US4] Test en `web/src/features/admin/BusinessSettingsPage.test.tsx`: al elegir otra zona aparece el aviso, y dice **las dos cosas** — las horas mostradas cambian, las ventas ya registradas no se mueven de día.
- [X] T032 [US4] Implementar ese aviso en `web/src/features/admin/BusinessSettingsPage.tsx`. Informativo, no una confirmación de doble paso: cambiar la zona no destruye nada.

---

## Phase 7: Cierre

- [X] T033 Correr el `db-architect` sobre la migración 0055 y las dos consultas cambiadas de `server/queries/orders.sql`, antes de aplicarlas.
- [X] T034 Correr el `tablet-ui-reviewer` sobre `web/src/features/admin/BusinessSettingsPage.tsx` y `web/src/features/pos/PedidosEnCurso.tsx`: un ajuste nuevo y una marca nueva en la pantalla más usada.
- [X] T035 Correr el `go-backend-reviewer` sobre `backoffice.go`, `orders.go` y `businessdate.go`.
- [ ] T036 Recorrer [quickstart.md](./quickstart.md) completo, **empezando por cambiar la zona horaria del sistema operativo** — sin eso el recorrido pasa en falso.
- [X] T037 ~~Comparar las cifras de un arqueo cerrado antes y después del despliegue~~ — **no se puede correr en el ambiente de pruebas**: tiene CERO turnos cerrados, así que no hay arqueo que comparar. La invariante queda cubierta por dos tests automatizados, que son más fuertes que la comparación manual porque corren en cada cambio: `TestNingunaVentaCambiaDeDia` (ninguna `business_date` se mueve) y `TestElCorteDeVistaNoCambiaUnArqueoCerrado` (ninguna cifra del arqueo cambia al mover el modo de corte y la zona). En producción, cuando toque, las cifras a comparar son las de los turnos 4, 5, 7 y 10.

---

## Dependencias

```text
Fase 1 (T001-T002)  ← independiente, va primero por gravedad
Fase 2 (T003-T007)  ← bloquea las fases 5 y 6
      ↓
Fase 3 · US1 (T008-T018)  🎯 MVP        Fase 4 · US2 (T019-T023)  ← independientes entre sí
      ↓
Fase 5 · US3 (T024-T030)
Fase 6 · US4 (T031-T032)  ← encima de US1
      ↓
Fase 7 (T033-T037)
```

## Paralelismo

- Fase 3: T008-T011 juntos; T015-T017 juntos una vez que T012-T014 estén.
- Fase 4 corre en paralelo a la 3 completa.
- Fase 5: T024-T026 juntos.

## Alcance mínimo

**US1 sola ya entrega valor**: el ticket que se lleva el cliente deja de decir la hora de la tableta,
y las dos estaciones dejan de contradecirse.
