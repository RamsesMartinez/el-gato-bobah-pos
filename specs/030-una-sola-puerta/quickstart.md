# Quickstart: verificar «una sola puerta»

## Prerrequisitos

- Worktree `/home/ramy/git/egb-030-cuentas`, rama `030-una-sola-puerta`. No se tocan otros
  worktrees, el ambiente de pruebas solo para e2e, nunca producción.
- Postgres propio: contenedor `egb030-pg` en el puerto **5502** (si no está arriba:
  `docker run -d --rm --name egb030-pg -p 5502:5432 -e POSTGRES_USER=gatobobah -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=gatobobah_test postgres:16-alpine`).
- Migraciones de esta rama: solo **0082** en adelante.
- Test de migración: respaldo real con **dos empresas** en una base aparte `gatobobah_restored`
  (`PG_CONTAINER=egb030-pg POSTGRES_DB=gatobobah_restored bash scripts/restaurar-respaldo.sh`;
  nunca `--no-owner` ni `--no-privileges`).

## 1. Backend

```bash
cd server && go build ./... && go test ./...
export TEST_DATABASE_URL="postgres://gatobobah:pw@localhost:5502/gatobobah_test?sslmode=disable"
export TEST_RESTORED_DATABASE_URL="postgres://gatobobah:pw@localhost:5502/gatobobah_restored?sslmode=disable"
go test -tags=integration -count=1 ./internal/integration/...
# El test de la migración tiene que haber corrido, no omitido: esto debe dar 0.
go test -tags=integration -count=1 -v -run MigrationOrderDrafts ./internal/integration/... | grep -c SKIP
bash ../scripts/hooks/golangci-lint.sh
```

| Escenario | Esperado | Test |
|---|---|---|
| Cuenta con 3 productos | No sale en Ventas, corte, reportes, recetas, Top, tablero ni almacén | `TestADraftIsNeverASale` |
| Dos cuentas nacen a la vez | Nombres distintos | `TestTwoDraftsNeverShareAName` |
| Descartar | El nombre vuelve a la bolsa; el folio no se gastó | `TestDiscardReturnsTheName` |
| Enviar dos veces (red caída) | Un solo pedido, una sola comanda | `TestSendIsIdempotent` |
| Dos tabletas: + y + | Se suman | `TestConcurrentAddsSum` |
| Dos tabletas: − sobre lo que la otra cambió | `409`, nada aplicado | `TestStaleChangeIsRejected` |
| Pedido pagado y entregado | No recibe | `TestClosedOrderReceivesNothing` |
| Pedido de plataforma | No recibe | `TestPlatformOrderReceivesNothing` |
| 12 h sin tocar | Descartada, nombre suelto | `TestIdleDraftExpires` |
| Cierre con cuentas en captura y deudas | Cierra; las lista | `TestCloseLiveAccountsDoNotBlock` |
| Lista de cuentas vivas, cinco estados | Estado y grupo correctos | `TestLiveAccountsStates` |
| Aislamiento | Tres casos de RLS | `TestDraftsInTheThreeCases`, `TestEveryCompanyTableIsIsolated` |
| Lista con 30 mil pedidos | ≤ 30 ms | `TestLiveAccountsStaysFast` |

## 2. Front

```bash
cd web && bun run lint && bun run vitest run && bun run build
```

Incluye `sinDialogosDelSistema.test.ts` (cero `confirm(`/`prompt(`/`alert(` en `web/src`).

## 3. En la tableta (ambiente de pruebas, 1024×600)

1. Prender la VM de pruebas (skill `ambiente-dev`) y desplegar la imagen de la rama.
2. Correr la suite en contenedor (AGENTS.md §2):

   ```bash
   docker run --rm --network host -v "/home/ramy/git/egb-030-cuentas/web:/w" \
     -v gatobobah_e2e_modules:/w/node_modules -w /w -e CI=1 \
     mcr.microsoft.com/playwright:v1.63.0-noble \
     sh -c "bun install --frozen-lockfile --silent; npx playwright test"
   ```

3. `web/e2e/una-sola-puerta.spec.ts` recorre los 30 casos del lienzo (mapa caso → test en
   [tasks.md](./tasks.md), Fase 9). `cabe-en-la-tableta.spec.ts` mide la fila, el ticket con sus tres
   secciones, la hoja «+N», la hoja de descartar y el cierre de caja a 1024×600 con el panel abierto.
4. El teardown descarta las cuentas en captura que creó la suite y cobra y entrega sus pedidos
   (`limpiar-lo-que-cree.ts`). Verificar al final que `GET /pos/accounts` no tiene nada de la suite.
