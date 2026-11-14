# Contratos: Rediseño del módulo Caja

Todo bajo `/api/v1`. Dinero en pesos como número (`float64` redondeado con `Round2`, constitución III). Los errores siguen `httpapi.Error`.

## Fase 1 — campos nuevos en lecturas existentes

`GET /cash-sessions/` (histórico, también `cash_sessions_history` por MCP) — cada fila agrega:

```json
{
  "difference": { "drawer": -120.00, "otherMethods": 0.00, "total": -120.00 },
  "openingReason": "last_count_wrong",
  "openingReasonNote": "El gasto salió de la caja B",
  "explainedByNext": { "sessionId": 412, "reason": "last_count_wrong", "note": "…" },
  "openedByName": "…", "closedByName": "…"
}
```

- `difference.*` es `null` donde el arqueo ciego lo oculta (mismo criterio que hoy
  `totalDifference`). `drawer` es `null` también si el corte no tiene conteo de cierre con esperado.
- `explainedByNext` es `null` si la apertura siguiente no trajo motivo, o si el arqueo del corte está
  oculto.
- `totalDifference` se conserva igual a `difference.total` (compatibilidad).

`GET /cash-sessions/{id}` agrega lo mismo más `previousSessionId`. `POST /cash-sessions/close`
devuelve `difference` en la vista del turno cerrado, calculada con la misma función.

Ningún campo nuevo en las escrituras de la fase 1.

## Fase 2 — sin cambios de contrato

Mismas rutas, mismos cuerpos, mismas respuestas. Los tests de contrato existentes son la red.

## Fase 3 — correcciones de un corte

`GET /cash-sessions/{id}` agrega:

```json
"adjustments": [
  { "id": 7, "kind": "count_correction", "before": 1000.00, "after": 1120.00,
    "reason": "miscount", "note": "…", "createdBy": "…", "createdAt": "…",
    "reversedBy": null }
],
"effective": { "drawerCounted": 1120.00, "floatLeft": 500.00 }
```

`difference` usa el conteo efectivo. El histórico agrega `adjusted: true` en la fila con alguna
corrección vigente.

`POST /cash-sessions/{id}/adjustments` — permiso `cash.adjust_count`.

```json
{ "kind": "count_correction", "after": 1120.00, "reason": "miscount", "note": "…" }
```

| Respuesta | Cuándo |
| --- | --- |
| 201 + la corrección | |
| 400 `VALIDATION` | `kind`/`reason` fuera de la lista, `after` negativo o no válido (`ValidMoney`), `note` vacía o > 200, `after` en una nota, `after` igual al valor efectivo |
| 404 | corte inexistente o de otra empresa |
| 409 `SESSION_NOT_CLOSED` | el corte sigue abierto |

`POST /cash-sessions/{id}/adjustments/{adjId}/reverse` — mismo permiso. Cuerpo `{ "note": "…" }`.

| Respuesta | Cuándo |
| --- | --- |
| 201 + el reverso | |
| 404 | la corrección no es de ese corte o de esta empresa |
| 409 `ALREADY_REVERSED` | ya revertida (también si dos tabletas lo intentan a la vez) |
| 400 `VALIDATION` | se intenta revertir un reverso |

Bitácora: `cash.session_adjusted` y `cash.session_adjustment_reversed` (grupo caja) con
antes/después, en la misma transacción.

## Fase 4 — tolerancia y explicación al cerrar

`GET /settings/cash-close` (`cash.operate`) → `{ "tolerance": 0.00 }`.
`PUT /settings/cash-close` (`cash.configure`) → mismo cuerpo; 400 si falta, vacío, negativo o no
válido (un campo vacío no es $0).

`POST /cash-sessions/close` acepta además, opcional:

```json
"explanation": { "reason": "miscount", "note": "…" }
```

Solo si quien cierra tiene `cash.configure`; con arqueo ciego activo y sin ese permiso responde 403
en lugar de aceptarla (no es una sonda para saber si hubo diferencia). Se guarda como una corrección
`note` en la misma transacción del cierre. Sin el campo, el cierre se comporta igual que hoy.

Histórico y detalle agregan `needsExplanation: bool`, solo para quien tiene `cash.configure`
(ausente para los demás).
