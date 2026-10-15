# Quickstart: Compras al almacén

Validación de punta a punta. Infraestructura del worktree (`make start`) con la copia del respaldo
de producción restaurada (`make db-restaurar`); contratos en [contracts/api.md](contracts/api.md).

## 1. Un gasto sin mercancía no queda «recibido»

Gastos › Nuevo gasto › «Capturar sin ticket» › «No, solo el gasto»: un gasto de luz. **Esperado:** el gasto no muestra
«recibido»; en la base, `received_at` nulo.

## 2. Compra en presentación suma al almacén y fija el costo

1. Almacén › Insumos, columna «Cómo llega» de la leche › «+ Agregar»: empaque «Caja» que trae 12 L
   (la pantalla muestra «= 12,000 ml» antes de guardar).
2. Gastos › Nuevo gasto › «Capturar sin ticket» › «Sí»: renglón «Leche», 3 «Caja · 12 L», $540, «Llegó».
3. **Esperado:** la leche sube 36 L; el movimiento dice el gasto y el renglón; el costo de la leche
   queda en $15 por litro y el costo de las recetas con leche cambia; Salud deja de listarla en
   «Insumos sin compras registradas».

## 3. Corregir y anular

1. En el gasto del paso 2, «Corregir lo que llegó» a 2 cajas. **Esperado:** un movimiento de −12 L; la leche queda
   en +24 L respecto de antes de la compra.
2. «Anular compra» (la confirmación dice cuánto quedará). **Esperado:** en un solo paso, un
   movimiento de −24 L, el gasto anulado, ningún movimiento borrado.
3. Dos pestañas corrigiendo el mismo renglón: la segunda recibe 409.

## 4. Promedio ponderado

Configuración del negocio › Almacén: «Promedio de lo que tienes». Con 10 L a $20 guardados, comprar 10 L a $30. **Esperado:**
costo $25. Con lo guardado en negativo, comprar a $30: costo $30.

## 5. Conteo

1. Almacén › Conteo › Empezar. Capturar 1,500 ml de un insumo que el sistema tiene en −200 ml.
2. Recargar la página y abrir desde otra tableta: la cifra sigue ahí.
3. Vender algo que use 100 ml de ese insumo; cerrar el conteo.
4. **Esperado:** el insumo queda en 1,400 ml; un ajuste de +1,700 con el conteo; los no contados
   igual que antes y la pantalla dice cuántos quedaron sin contar; Salud ya no lo lista en negativo.

## 6. Por recibir

1. Registrar una compra de 3 cajas de leche con «Llega después». **Esperado:** no suma; aparece en
   Almacén › Por recibir con proveedor, fecha y lo pagado.
2. «Recibir…» 2 cajas. **Esperado:** +24 L con la fecha de hoy; queda 1 caja pendiente.
3. «Ya no va a llegar…» la caja restante. **Esperado:** sale de la lista; el total pagado no cambia;
   la compra deja de salir en Gastos › «Con pendientes de llegar».
4. Otra compra con 3 cajas pendientes: recibir 4, con $180 por la de más. **Esperado:** +48 L y el
   total pagado sube $180.
5. «Llegó algo más…» con 2 cajas de crema pagadas y otra como regalo. **Esperado:** las dos suman; el
   total sube solo por la pagada; el costo de la crema no cambia por el regalo.
6. Dos tabletas recibiendo el mismo renglón: la segunda recibe 409 y la pantalla pide volver a abrir.

## 7. Aislamiento y permisos

Un cajero recibe 403 en recibir, corregir, por recibir, empaques, conteo y configuración del almacén. Nada de otra
empresa aparece ni se modifica (tests bajo `appRoleStore` con `inTheThreeCases`).

## Gates

`make api-build && make api-test`, `make test-integration`, `bun run lint`, `bun run vitest run`,
`bun run build`, `make ci-local`.

## Corrida del 14-oct

Contra la app local del worktree (copia de producción con dos empresas, la API como
`gatobobah_app`), por la API real con login: los pasos 1 a 6 pasan, 24 de 24 comprobaciones sobre lo
guardado en la base. No se corrió en esta vuelta: el paso 7 por la pantalla (lo cubren las pruebas
de integración por el router real) ni lo que Salud muestra en los pasos 2 y 5. La pasada por
pantalla, con capturas a 1024×600 y usuarios sin contexto, es T065 tras el merge.
