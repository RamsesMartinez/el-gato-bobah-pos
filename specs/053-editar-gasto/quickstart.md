# Quickstart: validar la edición de gastos en local

## Preparación

```bash
cd ~/git/egb-053-editar-gasto
make db-restaurar dump=backups/prod/pre-053-20261101-0545.dump   # trae el gasto 83 (1-nov)
make start                                                       # API :8053, web :3053
```

Entrar como admin a `http://localhost:3053`, empresa `gatobobah`. Las e2e corren con
`E2E_BASE_URL=http://localhost:3053 E2E_API_URL=http://localhost:8053/api/v1 … bun run e2e`.

## Escenarios

1. **Caso del 1-nov (SC-001, criterio 1)**: Gastos → gasto de $67.50 «Tapas Traveler…» → «Agregar
   renglón» ×2 (TAPA TRAVELER 16 INIX 50 piezas $50.00; FAJILLA DE CARTON INIX 10 piezas $17.50) →
   vista previa: importe $67.50 → $67.50, sin aviso de dinero → Confirmar → «Recibir» todo hoy →
   vista previa: tapas −5 → 45, fajillas 0 → 10 → Confirmar. Esperado: existencias 45 y 10, un solo
   pago de $67.50, gasto pagado y recibido hoy.
2. **Borrar sin recibir (criterio 2)**: agregar un renglón sin «Ya llegó» y borrarlo: la vista previa
   no tiene grupo Inventario; `select count(*) from stock_movements where expense_item_id = <id>` = 0.
3. **Borrar recibido con conteo posterior (criterio 3)**: un gasto con un insumo recibido antes del
   último conteo cerrado → borrar el renglón → la vista previa pide «Ajustar el inventario / No mover»
   en la línea del insumo y «Confirmar» está deshabilitado. Con «No mover»: existencias iguales, dos
   movimientos nuevos (compra −q con la fecha original y ajuste +q con el conteo).
4. **Entregas parciales (criterio 4)**: un renglón de 5 piezas → «Recibir» 3 con fecha de ayer →
   «Sigue pendiente» preseleccionado → Confirmar → el renglón dice «Faltan 2» → «Recibir» 2 hoy →
   historial «ayer 3 · hoy 2», gasto recibido hoy.
5. **Importe con pagos (criterio 5)**: en un gasto pagado de $100 agregar $20 de más → aviso «Falta
   pagar $20» con «Registrar pago» / «Dejar pendiente» → «Dejar pendiente» → gasto pendiente con
   saldo $20; el pago de $100 sigue igual.
6. **Bitácora (criterio 6)**: Caja → Bitácora: cada confirmación de arriba aparece como «Gasto
   editado» o «Llegó mercancía» con quién, cuándo y el detalle.
7. **Encabezado (criterio 7)**: cambiar proveedor, categoría, comentario y fecha → vista previa
   compacta → Confirmar → bitácora con antes/después.
8. **Huella vencida (SC-002)**: abrir la vista previa en dos pestañas, confirmar en una; confirmar en
   la otra responde que cambió y muestra la vista previa nueva.

## Gates

```bash
cd server && go build ./... && go test ./...
make test-integration
cd ../web && bun run lint && bun run vitest run && bun run build
make ci-local
```
