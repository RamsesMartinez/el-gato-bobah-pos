# Analysis Report: Bloqueo por inactividad y cambio de operador por PIN

**Corrido**: 2026-09-01, tras `/speckit-tasks` · **Re-corrido**: tras aplicar la remediación

## Primera pasada — NO pasó el gate

16 FR · 6 SC · 36 tareas. **0 CRITICAL, 3 HIGH, 3 MEDIUM, 1 LOW.**

Los tres HIGH eran la misma falla repetida: **controles de seguridad con tarea de implementación y
sin tarea de test**. En este repo no es estilo — el principio V lo prohíbe explícitamente y el IV
exige el test antes.

| ID | Severidad | Qué faltaba |
| --- | --- | --- |
| D1 | HIGH | El evento de seguridad del desbloqueo fallido no tenía test |
| D2 | HIGH | "No revelar si falló la persona o el PIN" solo se cubría en el modo solo-PIN (P3), no en el que sí se va a usar |
| D3 | HIGH | Rechazar `userId` ausente es un control de frontera y no tenía test |
| D4 | MEDIUM | La salida "olvidé mi PIN" solo existía como parte de pintar la pantalla |
| D5 | MEDIUM | La migración reservaba el número 0050, que choca con el spec 003 |
| D6 | MEDIUM | Nadie verificaba que quien no tiene PIN **sí puede** entrar |
| D7 | LOW | SC-001 ("menos de 5 segundos") no lo medía ninguna tarea |

La culpa fue del desglose, no del spec: el spec pedía las tres propiedades con claridad y al pasar a
tareas se fueron sus tests.

## Remediación aplicada

De 36 a **41 tareas**. Cinco nuevas, todas de test y todas **antes** de su implementación:

| Antes | Ahora |
| --- | --- |
| D1 — evento sin test | T022 (test) antes de T023 (implementación) |
| D2 — no-revelar sin test | T019, en el modo por default; T036 exige que siga pasando con solo-PIN |
| D3 — frontera sin test | T020 antes de T021 |
| D4 — salida sin test | T009, antes de pintar la pantalla |
| D6 — sin verificar | T015 |
| D5 — número de migración | T001 ya no reserva número; se asigna al implementar |
| D7 — SC-001 sin medir | T038 dice cronometrarlo |

## Segunda pasada — pasa

| | Primera | Ahora |
| --- | --- | --- |
| FR con al menos una tarea | 16/16 | **16/16** |
| FR con tarea de **test** | 12/16 | **16/16** |
| SC verificables | 5/6 | **6/6** |
| Tareas sin requisito mapeado | 0 | 0 |
| CRITICAL / HIGH | 0 / 3 | **0 / 0** |

Ambigüedades: 0. Duplicaciones: 0. Conflictos con la constitución: 0.

**Listo para `/speckit-implement`.**

## Lo que queda anotado, y no bloquea

- **Los refresh tokens ya emitidos traen 30 días** y bajar el ajuste no los acorta. Es decisión de
  despliegue, no de código: T030 la deja escrita antes de desplegar.
- **La numeración de migraciones** entre este spec y el 003 se resuelve al implementar el segundo.
