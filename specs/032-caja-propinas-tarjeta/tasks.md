# Tasks: Caja, propinas y cobro con tarjeta

**Input**: [spec.md](spec.md), [plan.md](plan.md), [data-model.md](data-model.md), [contracts/api.md](contracts/api.md)

Formato: `[ID] [P?] [Historia] Descripción`. Todo test se escribe y se ve en rojo antes de su tarea de implementación (constitución IV). EB-xx = borde del spec que el test atrapa.

## Estado (2026-10-09)

Hecho: entregar propinas (US1, opción C, pesos enteros) y la decisión de propinas al cerrar
(US2), con la migración `0087_tip_payouts.sql`. El resto de las historias sigue pendiente; la
migración de terminales y conceptos tomará el siguiente número libre. T003 se cubrió con
`tip_payouts_test.go` para lo que trae `0087`; el Down no tiene test propio.

## Fase 1: Preparación

- [x] T001 Confirmar que `0087`/`0088` siguen libres en develop y en ramas vivas; renumerar si no.
- [ ] T002 Respaldo de producción restaurado en local con `make db-restaurar` (dos empresas) para los tests de migración.

## Fase 2: Fundación (bloquea todas las historias)

- [ ] T003 [P] Test de integración de `0087` (conceptos, `kind` ampliado, check de forma, reverso único, `tip_payout_sources`, `tips_carried_over`, motivo de apertura) bajo `appRoleStore` con `inTheThreeCases`, dos empresas, Down que falla con filas `propina` (D1–D7, D9) en `server/internal/integration/cash_tips_migration_test.go`.
- [ ] T004 [P] Test de integración de `0088` (terminales, modo por sucursal, `order_payments.card_*`, folio en `order_refunds`, `session_terminal_counts`, `daily_summary_sends`, siembra por slug) con históricos intactos (D6, EB-35) en `server/internal/integration/card_terminals_migration_test.go`.
- [x] T005 Migración `server/migrations/0087_cash_concepts_and_tips.sql`.
- [ ] T006 Migración `server/migrations/0088_card_terminals.sql`.
- [x] T007 Queries sqlc en `server/queries/{tips,cash_concepts,terminals,cash}.sql` y `make sqlc`.
- [x] T008 Verificar que `TestEveryCompanyTableIsIsolated` cubre las tablas nuevas.

## Fase 3: US1 Entregar propinas (P1) 🎯 MVP

- [x] T009 [P] [US1] Tests table-driven de `domain.TipLedger`: pendiente por método, FIFO heredado→efectivo→resto, entrega > pendiente, devolución tras entrega, herencia; `SplitEven` en pesos enteros con sobrante pendiente, centavos rechazados, persona repetida (EB-03, EB-06, EB-07, EB-08, EB-12, EB-43, EB-44, EB-46; D-A/D-B 2026-10-09) en `server/internal/domain/tips_test.go`.
- [x] T010 [US1] Implementar `server/internal/domain/tips.go`.
- [x] T011 [P] [US1] Test de integración: pendiente pre-agregado con 2 pagos × 2 devoluciones × 2 entregas (D5); otra caja abierta no se mezcla (EB-14); receptor de otra empresa o inactivo rechazado (EB-41).
- [x] T012 [P] [US1] Test `TestTipIsNeitherSaleNorExpense` y `TestCardTipPaidInCashLeavesDrawerOnce`, que fallan nombrando el concepto duplicado (EB-01, EB-02, EB-04, SC-001).
- [x] T013 [US1] `app.TipsService` (pendiente, entrega en tx) y handlers + rutas con rol en `httpapi`.
- [x] T014 [P] [US1] Tests vitest de la hoja «Repartir propinas» (opción C, decidida 2026-10-09): parejo en pesos enteros, sobrante visible, ajustado sin centavos, 3 toques a una persona, Cancelar no guarda.
- [x] T015 [US1] Pantalla en `web/src/features/caja/`.

## Fase 4: US5 Terminal y tipo de tarjeta (P1)

- [ ] T016 [P] [US5] Tests de dominio: terminal por omisión archivada u otra sucursal se ignora; tarjeta sin terminal rechazada (EB-28, EB-29, EB-31).
- [ ] T017 [P] [US5] Test de integración: todos los caminos de cobro (`pay`, cobro dividido, reintento idempotente) exigen terminal; plataforma no (EB-31); negocio sin terminales activas (EB-30).
- [ ] T018 [US5] Servicio, handlers, catálogo de terminales y preferencia de usuario.
- [ ] T019 [P] [US5] Tests vitest de la hoja de cobro: fila Débito/Crédito reemplaza «Recibido», un toque con terminal por omisión (U1, SC-002).
- [ ] T020 [US5] Pantallas de cobro y Configuración → Terminales.

## Fase 5: US2 Cierre que separa negocio y propinas (P1)

- [x] T021 [P] [US2] Tests de dominio del cierre: decisión obligatoria con pendiente, efectivo del negocio = contado − propinas en cajón (EB-05, EB-11, EB-13).
- [x] T022 [P] [US2] Test de integración: «se queda en caja» → siguiente turno de la misma caja la hereda, otra caja no (EB-12, EB-14).
- [x] T023 [US2] Ampliar cierre y corte en `app` y queries; corte con propinas por método (FR-022).
- [x] T024 [US2] Paso de cierre con los dos botones (U3).

## Fase 6: US3 Salidas con concepto y corrección (P2)

- [ ] T025 [P] [US3] Tests de dominio: `CanReverse` (doble reverso, reverso de reverso), `ExpenseDay` pasada medianoche, fecha de documento no mueve el día (EB-15, EB-16, EB-21, EB-22).
- [ ] T026 [P] [US3] Tests de integración: salida sin concepto rechazada por todo camino (EB-18); concepto duplicado por mayúsculas/espacios (EB-20); fusión mueve salidas (EB-19); reverso de turno cerrado cae en el abierto (EB-17).
- [ ] T027 [US3] Servicios y handlers de conceptos y corrección.
- [ ] T028 [US3] `Picker` de concepto con alta en línea; hoja de detalle con «Corregir»; Configuración → Conceptos (U5, U9).

## Fase 7: US4 Apertura a ciegas (P2)

- [ ] T029 [P] [US4] Tests: respuesta de apertura no contiene el cierre anterior (EB-23); vacío ≠ cero (EB-24); motivo validado en servidor (EB-25); primer turno sin motivo (EB-26); redondeo (EB-27).
- [ ] T030 [US4] Dominio, servicio y handler de apertura.
- [ ] T031 [US4] Hoja de conteo por denominación (U4).

## Fase 8: US6 Arqueo por terminal (P2)

- [ ] T032 [P] [US6] Tests: modo copiado al abrir (EB-33); terminal archivada con cobros se pide, sin cobros no (EB-32).
- [ ] T033 [US6] Implementación y paso «Tarjeta» del cierre (U6); control segmentado en Configuración.

## Fase 9: US7 Devolución con folio (P2)

- [ ] T034 [P] [US7] Tests: folio vacío o solo espacios rechazado; cobro histórico sin terminal pide folio sin terminal; quién lo capturó (EB-34–EB-36).
- [ ] T035 [US7] Implementación en devolución y `DevolucionSheet` (U7).

## Fase 10: US8 Avisos, reportes y correo (P3)

- [ ] T036 [P] [US8] Test: reportes de gastos excluyen propinas y traspasos y los muestran aparte (EB-09, EB-10).
- [ ] T036a [P] [US8] Tests de `domain.ValidSummaryEmails` y del campo en Configuración (EB-47; D-C 2026-10-09).
- [ ] T037 [P] [US8] Tests del correo: una vez por empresa y día con dos cajas y reintento (EB-37); RLS por empresa (EB-38); SMTP caído no bloquea cierre (EB-39); sin destinatario deja evento (EB-40).
- [ ] T038 [US8] Avisos en cierre y Ventas del día (U8); trabajo programado del correo.
- [ ] T039 [US8] Corrección de datos de propinas históricas: SQL numerado + rollback gemelo, lista aprobada por el dueño, respaldo previo (FR-008). Fuera de la migración de esquema.

## Fase 11: US9 Cobrar como acción principal (P3)

- [ ] T040 [P] [US9] Test vitest: en mostrador, «Cobrar» es la acción principal al enviar a cocina.
- [ ] T041 [US9] Implementación.

## Fase 12: Cierre

- [ ] T042 Validación de montos absurdos en todas las fronteras nuevas (EB-42).
- [ ] T043 Instrumentación de pantallas nuevas en `uso.go` (`pantallasMedibles`, `rolesPorPantalla`) y `rutas-medidas.ts`.
- [ ] T044 Actualizar `docs/matriz-de-cobro.md` y e2e de propina y tarjeta; cerrar pedidos de prueba.
- [ ] T045 `make api-build && make api-test`, `make web-lint`, `make ci-local`; `/revision-de-codigo`.

## Dependencias

Fase 2 bloquea todo. US1 y US5 son independientes; US2 depende de US1 (y de US6 para el paso «Tarjeta»). US3, US4, US7 independientes tras Fase 2. US8 al final.

## Resultado de /speckit-analyze (2026-10-09)

- Cobertura: FR-001 a FR-027 tienen tarea; EB-01 a EB-42 tienen test antes de su implementación.
- Sin hallazgos CRITICAL ni HIGH abiertos: los HIGH de la revisión after_plan quedaron como D1–D6 y U1–U3 en el plan.
- Resuelto el 2026-10-09: opción C, pesos enteros y correos en Configuración (D-A, D-B, D-C).
