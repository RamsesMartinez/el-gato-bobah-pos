# Quickstart: validar densidad y tara

Prerrequisitos: `make start` del worktree (API :8058, web :3058), base restaurada
(`make db-restaurar`), un usuario con `inventory.configure`. Pruebas a 1024×600 con toque.

## Automáticas

```bash
cd server && go test ./internal/domain/ -run 'Density|BaseQty|NetFromGross|UnitChange'
make test-integration   # densidad_test.go, migration_0105_test.go, los de unit_change y stock_count
cd web && bun run vitest run src/features/backoffice
```

## Recorrido (US1 → US4)

1. Almacén › Artículos › un artículo en ml → «Densidad» → pesada 1,250 / 150 / 1,000 → ve
   «1.10 g por ml» → Guardar. La ficha dice «1.10 g/ml · por pesada · <usuario>, hoy».
2. Gastos › Nuevo → renglón de ese artículo, 4.3 kg, $430 → el renglón dice
   «4,300 g ÷ 1.10 = 3,909 ml». Guardar como recibido. La existencia sube 3,909.0909 ml.
3. Rehacer la pesada con 1,230 / 150 / 1,000 (1.08). Abrir el gasto y corregir lo recibido a 4 kg:
   la existencia baja exactamente 272.7273 ml (300 g ÷ 1.10), no con 1.08.
4. Otro artículo en g sin compras, con densidad 1.30 y 1,300 g → «Cambiar en qué se mide» → ml: la
   equivalencia viene puesta; la vista previa dice 1,000 ml; confirmar.
5. Un empaque «Botella 1 L» del artículo de (1) con tara 120 g → abrir conteo → «Pesar con empaque»:
   850 g, 2 empaques, más 3 botellas cerradas → la hoja muestra la cuenta y guarda 3,554.5455 ml.
6. Un artículo en ml sin densidad: el renglón en kg pinta «Falta la densidad · Capturar la pesada»;
   el atajo abre la hoja y al guardar el renglón se convierte.

Al terminar: cerrar o descartar el conteo abierto y anular el gasto de prueba.
