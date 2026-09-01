# Análisis de consistencia — feature 006

**Corrido**: 2026-09-01, antes de `/speckit-implement` (gate obligatorio de la constitución).
**Artefactos**: [spec.md](../spec.md), [plan.md](../plan.md), [tasks.md](../tasks.md).

## Veredicto

**Sin CRITICAL. Un HIGH, tres MEDIO, un BAJO. Todos remediados en esta pasada.**

El HIGH es el mismo patrón que ya apareció en la feature 005 y que conviene nombrar: **el requisito
más importante de la feature era el único sin test automatizado**. Aquí es "ninguna venta cambia de
día" — la invariante de dinero, la que justifica todo el cuidado del plan — y su única verificación
era un paso manual del quickstart. Un paso manual se salta, y este en particular se salta justo
cuando hay prisa por desplegar.

## Hallazgos

| ID | Categoría | Severidad | Dónde | Qué | Remediación |
| --- | --- | --- | --- | --- | --- |
| A1 | Cobertura | **ALTO** | FR-015, FR-016, SC-006 | "El día de una venta no cambia" y "las cifras de un arqueo cerrado quedan idénticas" solo se verificaban a mano (T037). Es la invariante que hace segura toda la feature, y la que más caro sale si falla: dinero movido entre días que nadie nota hasta el corte siguiente | **T002b** y **T028b** nuevos |
| A2 | Cobertura | MEDIO | FR-006 | "Si la zona no se puede aplicar… deja constancia para quien pueda corregirla". Ninguna tarea implementa ni prueba esa constancia. Sin ella, un negocio con la zona rota se comporta bien y nadie se entera nunca | **T012b** nuevo |
| A3 | Cobertura | MEDIO | FR-014 | "Las listas reflejan el corte sin que nadie recargue". T030 lo implementa; nada lo verifica | **T030b** nuevo |
| A4 | Subespecificación | MEDIO | T011 | El test de guardia recorre `web/src` buscando `toLocale*`, pero el helper legítimamente lo llama: tal como está descrito, falla contra su propia implementación. Es el mismo tropiezo que ya costó una corrección en el test equivalente de Go | T011 reescrito para recortar el helper |
| A5 | Inconsistencia | BAJO | T021 | Retira el filtro por turno que introdujo la feature 005, pero no dice qué pasa con el test que lo fijó (`TestLaBarraSigueMostrandoElTurnoQueCruzoLaMedianoche`). Un test que afirma lo viejo y sobrevive al cambio es de donde salen los verdes que engañan | T021 lo nombra |

## Cosas que el análisis verificó y están bien

- **El orden de fases es correcto y por la razón correcta.** La fase 1 —el fallback que fecha en
  UTC— va sola y primero porque es lo único que mueve dinero, y no depende de nada.
- **La terminología es consistente**: `corte de vista` en el spec, `corteDeVista` en el contrato,
  `corte_de_vista` en la columna. Cada capa con su convención, sin drift.
- **Ninguna tarea toca `orders.business_date`.** Se revisó archivo por archivo: la feature cambia
  consultas de lectura y presentación, nada más.
- **El permiso de tocar históricos no se usa, y eso está justificado con medición**, no con
  cautela: 21 de 21 pedidos del negocio real tienen la fecha correcta.
- **El borde del horario de verano tiene su test** (T003) y su razón medida (research, hallazgo 7):
  `America/Tijuana` sigue cambiando de horario y está en la lista que el producto ofrece.

## Cobertura de requisitos

| Requisito | Tareas |
| --- | --- |
| FR-001 · toda hora en la zona del negocio | T012–T018 |
| FR-002 · ticket y comanda | T009, T010, T014 |
| FR-003 · nunca la hora del dispositivo | T011 |
| FR-004 · sin hora que se corrija sola | T008, T018 |
| FR-005 · sin ajustes, el default del producto | T008 |
| FR-006 · zona inválida: default **y constancia** | T008, **T012b** |
| FR-007 · un solo lugar | T011, T012 |
| FR-008 · activos sin filtro de fecha | T019, T021, T022 |
| FR-009 · sale al entregarse o cancelarse | T019 |
| FR-010 · el rezago se distingue | T020, T023 |
| FR-011 · entregados hasta el corte | T024, T027, T028 |
| FR-012 · los tres modos | T025, T029 |
| FR-013 · default medianoche | T005 |
| FR-014 · sin recargar | T030, **T030b** |
| FR-015 · el día de la venta no cambia | **T002b**, T037 |
| FR-016 · arqueos cerrados idénticos | **T028b**, T037 |
| FR-017 · aviso al cambiar de zona | T031, T032 |
| SC-001 · dos dispositivos, misma hora | T036 |
| SC-002 · ticket = pantalla | T009, T010, T036 |
| SC-003 · ninguna hora cambia sola | T008, T018 |
| SC-004 · la lista aguanta hasta el corte | T024 |
| SC-005 · el pedido viejo aparece y se distingue | T019, T020 |
| SC-006 · arqueo idéntico | **T028b**, T037 |
| SC-007 · una pantalla nueva hereda | T011 |

## Alineación con la constitución

| Principio | Veredicto |
| --- | --- |
| I. Layering | OK. El cálculo del corte es puro y vive en `domain`; la conversión de zona es de presentación y vive en la vista |
| II. Errores | OK. Un `corteDeVista` inválido es `ErrValidation`; el default es para el campo ausente, nunca para el malformado |
| III. Dinero | OK **tras A1**. La invariante de que ninguna venta cambia de día ahora tiene test, no solo un paso manual |
| IV. Test-first y bordes primero | OK tras la remediación. Los bordes se enumeraron en el spec antes del plan, y los tres huecos de test se cerraron aquí |
| V. Seguridad | OK. El fallback deja de ser el silencioso; RLS cubre las consultas nuevas y sus tests corren con dos empresas |
| VI. YAGNI | OK. Sin librería de fechas, sin tabla, sin zona por usuario. Los tres modos son a pedido explícito y su costo está anotado |
| VII. Comentarios | OK. El plan nombra qué decisiones necesitan comentario del porqué |

## Métricas

| | |
| --- | --- |
| Requisitos funcionales | 17 |
| Criterios de éxito | 7 |
| Tareas | 37 → **41** tras la remediación |
| Cobertura por tarea | 100% |
| CRITICAL | 0 |
| ALTO | 1 (remediado) |
| MEDIO | 3 (remediados) |
| BAJO | 1 (remediado) |
