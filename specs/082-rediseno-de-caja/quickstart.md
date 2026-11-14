# Quickstart: validar el rediseño de Caja

Todo en local, contra el `make start` de este worktree (API :8082, web :3082), con datos reales
restaurados (`make db-restaurar` con la API detenida) o un admin nuevo. Capturas solo en
`test-results/`.

## Prerrequisitos

```bash
make start                       # o ya arriba
curl -s localhost:8082/readyz    # {"status":"ready"}
```

## Fase 1 — una sola diferencia

1. Abrir y cerrar una caja dejando el cajón $120 corto y la tarjeta cuadrada.
2. Ver el diálogo «Caja cerrada», la fila del Histórico y el detalle: los tres dicen «Faltante
   −$120.00» en Cajón y Total, «Cuadra» en otros métodos, y el efectivo de la conciliación dice
   «ver arqueo».
3. Abrir el turno siguiente contando distinto, con motivo «El cierre se contó mal» y nota. El detalle
   del corte anterior dice «Diferencia explicada al abrir el turno #N» con la nota; el nuevo muestra
   el motivo.
4. `cash_sessions_history` por MCP trae `openingReason` y `openingReasonNote`.
5. Con un cajero (sin `cash.configure`) y arqueo ciego: ninguna de las tres pantallas muestra
   esperado ni diferencia.
6. Botón sol/luna abajo a la izquierda: un toque cambia de tema; en otra pestaña con otro
   `localStorage` no cambia.

Pruebas: `cd server && go test ./internal/domain/ -run CutDifference` y
`make test-integration` (casos de corte: posterior a 0067, entre 0066 y 0067, anterior a 0066).

## Fase 2 — partir sin cambiar comportamiento

```bash
make api-test && make test-integration
cd web && bun run vitest run src/features/caja src/features/backoffice && bun run e2e
```

Mismos resultados antes y después (se guardan los conteos de casos de cada corrida). Abrir
`/caja/historico/<id>` en otra pestaña muestra ese corte; recargar en Histórico sigue en Histórico.

## Fase 3 — correcciones con rastro

1. Como gerente, en un corte cerrado: Corregir conteo → $1,120 con motivo y nota. El detalle muestra
   el original y el corregido; la lista usa la diferencia corregida.
2. Revertir la corrección: vuelve al original; las dos filas quedan a la vista.
3. Revertir otra vez → «ya se revirtió».
4. Como cajero: sin la acción; `POST /cash-sessions/{id}/adjustments` directo → 403.
5. `make test-integration` con `inTheThreeCases` sobre la tabla nueva y la prueba de la migración
   0113 contra el respaldo restaurado con dos empresas.

## Fase 4 — asistente, X/Z y anchos

1. Con una cuenta abierta y sin contar el cajón: Cerrar → el paso 1 nombra la cuenta con su acción;
   el botón final dice qué falta.
2. Contar a medias y recargar: el conteo sigue.
3. Corte X: muestra el turno y no cambia nada (el turno sigue abierto, sin conteos guardados).
4. Tolerancia $50, cierre viendo el esperado con −$80: se ofrece la explicación; sin dejarla, el corte
   sale «por explicar» en el Histórico para el gerente, y el cajero no lo ve.
5. Capturas Playwright a 768×1024, 1024×600, 1366×768, 1920×1080 y 2560×1440 del turno abierto, el
   asistente y el Histórico con un corte abierto (`test-results/`).
6. Prueba con usuario nuevo a 1024×600 y en 1920/2560.
