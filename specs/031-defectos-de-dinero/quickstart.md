# Quickstart: verificar 031

## Prerrequisitos

- Worktree `/home/ramy/git/egb-031-dinero`, rama `031-defectos-de-dinero`.
- Postgres propio: contenedor `egb031-pg` en el puerto 5501 (`gatobobah`/`pw`, base `gatobobah_test`).
  Nunca `deploy-postgres-1` ni los de otras ramas.
- Para el test de la migración 0080, un respaldo real con dos empresas en una base **aparte**,
  `gatobobah_restored`, restaurado con dueños y GRANT (nunca `--no-owner` ni `--no-privileges`):
  `PG_CONTAINER=egb031-pg POSTGRES_DB=gatobobah_restored bash scripts/restaurar-respaldo.sh <dump>`.
  El dump vive fuera del repo (`backups/prod/` del clon principal), se usa el más reciente.

## 1. Backend

```bash
cd server && go build ./... && go test ./...
export TEST_DATABASE_URL="postgres://gatobobah:pw@localhost:5501/gatobobah_test?sslmode=disable"
export TEST_RESTORED_DATABASE_URL="postgres://gatobobah:pw@localhost:5501/gatobobah_restored?sslmode=disable"
go test -tags=integration -count=1 ./internal/integration/...
# El test de la migración tiene que haber corrido, no omitido: debe dar 0.
go test -tags=integration -count=1 -v -run MigrationMoneyByShift ./internal/integration/... | grep -c SKIP
bash ../scripts/hooks/golangci-lint.sh
```

| Escenario | Esperado |
|---|---|
| $40 efectivo + $60 tarjeta; devolver 40 y luego 60 | Segunda: $60 por tarjeta, una sola salida de caja (D1) |
| Dos «devolver todo» simultáneos | Uno pasa, el otro `ErrDevolucionExcede` (D3) |
| Devolver contra un renglón de $60 en un pedido de $500 | Tope $60; repetir da 0 (D4) |
| Devolver $40 y luego devolver el pago de $100 | Rechazado `ErrPaymentHasRefunds` (D2) |
| Tarjeta $300 devuelta en el mismo turno | Esperado tarjeta $0, la devolución listada (D6) |
| Devolver efectivo sin turno | `ErrCashRefundNeedsOpenRegister`, nada registrado (D7) |
| Devolver tarjeta sin turno, abrir turno | La devolución entra a ese turno (D7) |
| Uber aceptado sin turno, abrir turno | Su pago en el esperado del turno (D5) |
| Cobro concurrente al cierre | Dentro del esperado guardado o rebota (D8) |
| $100 + $10 propina, cancelar con devolución | Salida $110; `refund_amount` 100 (D9) |
| Pedido del turno A cobrado en B | A: sin cobrar $554 fijo; B: «Cobros de otros turnos» $554 (D12) |
| Cobro de un día y devolución al siguiente | Ventas por método: cada uno en su día (D12) |
| Utilidad con renglón quitado y descuento | 1 pieza, ingreso neto del descuento (D10) |
| Quitar 3 y cerrar sin productos | «Renglones cancelados» 3 (D14) |
| Cancelar pedido con frappé enviado a cocina | No repone sus insumos (D11) |
| Cobrar $99.99 de $100 | Sigue debiendo $0.01 (D16) |
| Partir $45.55 a la mitad | 22.78 + 22.77 (D17) |
| Pedido «reembolsado» viejo | Devolver rechazado (D18) |
| Consultas nuevas sobre tablas de empresa | Aisladas en los tres casos (`inTheThreeCases`) |

## 2. Front

```bash
cd web && bun install && bun run lint && bun run vitest run && bun run build
```

`CashPage.test.tsx`: «Total Dif.» incluye el cajón (D13) y la lista de devoluciones del turno.
`SalesPage.test.tsx`: con filtro de tipo no se pide el resumen de plataformas (D15).

## 3. En la tableta

El cambio de pantalla es una lista (devoluciones del turno, con el mismo tope en dvh que «Pagos
devueltos») y una cifra («Total Dif.»). Se revisa con el revisor de tableta; no se cambia
disposición.
