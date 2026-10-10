# Quickstart: verificar que dividir la cuenta funciona

## Prerrequisitos

- Worktree `/home/ramy/git/egb-027-dividir-cuenta`. **No** se usan los contenedores
  `deploy-*` de la otra sesión: los tests de integración corren contra un Postgres propio.
- Postgres para integración: `docker run -d --rm --name egb027-pg -p 5499:5432 -e POSTGRES_USER=gatobobah -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=gatobobah_test postgres:16-alpine`.
- **La rama de Uber (021) ya está en `develop` y 027 está rebasada encima** (D-11): sin ella no
  existen `inTheThreeCases`, `TestEveryCompanyTableIsIsolated` ni las políticas con `nullif`, y lo que
  toca tablas nuevas no se puede probar. Antes del rebase solo se verifica lo de permisos, dominio
  puro, US7 y US8 (sin su aislamiento). Ningún test se marca `skip`. Al rebasar, el conflicto de la
  constitución se resuelve reaplicando la enmienda de roles sobre la 1.13.0 de 021 (queda 1.14.0).
- **Para el test de la migración**, `egb027-pg` lleva un respaldo real con dos empresas en una
  base **aparte**, `gatobobah_restored`: el test usa `restoredStore`, que lee
  `TEST_RESTORED_DATABASE_URL` y se omite (`t.Skip`) si falta, y no puede compartir la base de
  `TEST_DATABASE_URL` porque `newTestStore` la borra. Se restaura con dueños y GRANT, con el mismo
  script de `make db-restaurar` apuntado al contenedor propio:
  `PG_CONTAINER=egb027-pg POSTGRES_DB=gatobobah_restored bash scripts/restaurar-respaldo.sh
  [backups/prod/x.dump]`. No se usa `make db-restaurar` directo: su `deps-up` levanta los
  contenedores `deploy-*` de la otra sesión. El script llega con `develop` (tras el rebase) y crea
  los roles antes de restaurar. **Nunca** `--no-owner` ni `--no-privileges` (AGENTS.md §2). Con una sola empresa, todo camino «por cada otra empresa» es un no-op y la migración pasa
  verde para romper en producción.

## 1. Backend

```bash
cd server && go build ./... && go test ./...
export TEST_DATABASE_URL="postgres://gatobobah:pw@localhost:5499/gatobobah_test?sslmode=disable"
export TEST_RESTORED_DATABASE_URL="postgres://gatobobah:pw@localhost:5499/gatobobah_restored?sslmode=disable"
go test -tags=integration -count=1 ./internal/integration/...
# El test de la migración tiene que haber corrido, no omitido: esto debe dar 0.
go test -tags=integration -count=1 -v -run MigrationSplitBill ./internal/integration/... | grep -c SKIP
```

Qué tiene que pasar, con el nombre del test que lo dice:

| Escenario | Esperado |
|---|---|
| La mesa del incidente: 11 renglones, pagos de 1, 4 y el resto | 3 pagos que suman el total al centavo, 0 cancelaciones, sin cambio neto en existencias |
| Cotizar una selección y luego cobrarla | `quote` no escribe nada y da el monto que `/pay` cobra |
| Dos cobros simultáneos de la misma pieza | Uno pasa, el otro `ErrConflict` |
| Descuento de $50 sobre $907, tres pagos por productos | Suma = total; el último absorbe el centavo |
| Pago por monto y luego «Todo lo que falta» | La cobertura suma exactamente el monto del pago (prorrateo) |
| Devolver el pago 2 y cobrarlo a otra persona | El corte espera por método lo correcto; `voidedPayments` lo lista; el `client_uuid` original no lo revive |
| Pasar 2 productos ya enviados a cocina a un pedido nuevo | Sin comanda nueva, existencias iguales, origen y destino con totales correctos |
| Pasar todos los productos a un pedido nuevo | Rechazado («Ya es su propio pedido…») |
| Pasar todos los productos a un pedido existente | El origen queda juntado; ni Ventas ni las ventas del turno lo cuentan como cancelación; lo quitado antes sí |
| Quitar 1 de 2 piezas y luego la otra | Renglón partido; cada mitad repone solo la suya |
| Quitar un refresco sin preparación | Su existencia vuelve |
| Quitar un renglón y luego cancelar el pedido | Sin doble reposición |
| Quitar un renglón de un pedido ya cobrado | Rechazado («Ya se cobró…») |
| Entregar todo sin renglones vivos | Rechazado |
| Pedido sin productos vivos y sin pagos, con cualquier rol | «Cerrar pedido» lo cancela («Sin productos») y cuenta como cancelación; con pagos, rechazado |
| Pasar parte de los productos | Origen y destino se cierran solos si ya no les falta nada |
| Cobrar, quitar, pasar y devolver en varios órdenes | Ningún pedido abierto sin una acción que lo cierre |
| Devolver, pasar, quitar lo que falta o cancelar sin el permiso | 403 con «Tu usuario no puede …» |
| Toda tabla nueva y todo servicio nuevo | Aislado en los tres casos (`inTheThreeCases`); `TestEveryCompanyTableIsIsolated` en verde |

## 2. Front

```bash
cd web && bun run lint && bun run vitest run && bun run build
```

`CobrarSheet.test.tsx` (matriz C), `OrdersBoardPage.test.tsx` (nuevo, matriz P),
`CancelPendingSheet.test.tsx` y `CancelarRenglonDialog.test.tsx` cubren: modos del selector, lo
pagado en gris tras recargar, fichas de pagos, detalle con «Devolver este pago» deshabilitado sin
permiso y con su texto, Cerrar pedido, Quitar los que faltan, quitar 1 de 2 y motivos sin
preselección.

## 3. En la tableta (ambiente de pruebas, 1024×600)

1. Prender la VM de pruebas (skill `ambiente-dev`) y desplegar la imagen de esta rama.
2. `web/e2e/split-bill-incident.spec.ts` (reescrito desde `dividir-cuenta-incidente.spec.ts`): la
   misma mesa se resuelve con «Dividir → Por productos» y ningún pedido queda abierto.
3. E7 bis (`cabe-en-la-tableta.spec.ts`): la hoja con Efectivo elegido, propina y la lista de
   productos cabe en 600 px con el pie visible.
4. E7 bis también mide a 600 px la hoja de quitar lo que falta y la vista de pasar a otro pedido.
5. Contar toques del recorrido: ≤ 20 (SC-001).
6. Los pedidos que la prueba crea quedan entregados y cobrados (teardown de la suite).
