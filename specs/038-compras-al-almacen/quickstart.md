# Quickstart: Compras al almacén

Validación de punta a punta. Infraestructura del worktree (`make start`) con la copia del respaldo
de producción restaurada (`make db-restaurar`); contratos en [contracts/api.md](contracts/api.md).

## 1. Un gasto sin mercancía no queda «recibido»

Registrar un gasto de luz sin renglones desde la pantalla. **Esperado:** el gasto no muestra
«recibido»; en la base, `received_at` nulo.

## 2. Compra en presentación suma al almacén y fija el costo

1. Almacén › Insumos › Leche › Presentaciones: agregar «Caja» = 12 (L).
2. Gastos › nuevo: renglón «Leche», 3 «Caja», $540, «Ya llegó».
3. **Esperado:** la leche sube 36 L; el movimiento dice el gasto y el renglón; el costo de la leche
   queda en $15 por litro y el costo de las recetas con leche cambia; Salud deja de listarla en
   «Insumos sin compras registradas».

## 3. Corregir y anular

1. En el gasto del paso 2, corregir a 2 cajas. **Esperado:** un movimiento de −12 L; la leche queda
   en +24 L respecto de antes de la compra.
2. Anular el gasto. **Esperado:** un movimiento de −24 L, el gasto anulado, ningún movimiento
   borrado.
3. Dos pestañas corrigiendo el mismo renglón: la segunda recibe 409.

## 4. Promedio ponderado

Configuración › Costeo: «Promedio». Con 10 L a $20 guardados, comprar 10 L a $30. **Esperado:**
costo $25. Con lo guardado en negativo, comprar a $30: costo $30.

## 5. Conteo

1. Almacén › Conteo › Empezar. Capturar 1,500 ml de un insumo que el sistema tiene en −200 ml.
2. Recargar la página y abrir desde otra tableta: la cifra sigue ahí.
3. Vender algo que use 100 ml de ese insumo; cerrar el conteo.
4. **Esperado:** el insumo queda en 1,400 ml; un ajuste de +1,700 con el conteo; los no contados
   igual que antes y la pantalla dice cuántos quedaron sin contar; Salud ya no lo lista en negativo.

## 6. Aislamiento y permisos

Un cajero recibe 403 en recibir, corregir, presentaciones, conteo y método de costeo. Nada de otra
empresa aparece ni se modifica (tests bajo `appRoleStore` con `inTheThreeCases`).

## Gates

`make api-build && make api-test`, `make test-integration`, `bun run lint`, `bun run vitest run`,
`bun run build`, `make ci-local`.
