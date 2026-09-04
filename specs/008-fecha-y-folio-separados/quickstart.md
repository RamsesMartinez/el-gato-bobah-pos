# Quickstart — 008 · Cómo comprobar que quedó bien

Escenarios ejecutables. Cada uno dice qué defecto atrapa; si no atrapa ninguno, sobra.

## Gates del repo

```bash
cd server && go build ./... && go test ./...
cd ../web && bun run lint && bun run vitest run && bun run build
```

En Windows el binario recién compilado lo puede bloquear Smart App Control: los gates corren en
contenedor con los scripts de `scripts/hooks/`. No se afloja el gate, se corre en otro lado.

---

## 1 · La venta cae en el día en que ocurrió (US1, FR-001)

**Atrapa**: el defecto reportado — un turno viejo archivando ventas de hoy con su fecha.

Integración, contra Postgres real:

1. Abrir un turno con `business_date` de hace cuatro días.
2. Crear una venta con el reloj fijado en hoy.
3. `business_date` de la venta = hoy en la zona del negocio, no la del turno.
4. `register_session_id` = el turno abierto. La venta sigue perteneciendo a su corte.

**Mutación que lo verifica**: devolver `bizDate := sess.BusinessDate`. El test debe ponerse rojo
diciendo qué fecha esperaba.

## 2 · La medianoche mueve la fecha pero no el folio (US1 + US2, FR-003)

**Atrapa**: el folio partido a mitad de un turno nocturno — el defecto que la herencia tapaba.

1. Turno abierto a las 23:00.
2. Venta a las 23:50 → `business_date` = día D, folio 1.
3. Venta a las 00:10 → `business_date` = día D+1, folio **2**.

Que las dos aserciones vivan en el mismo test es el punto: prueban que los dos caminos son
independientes.

## 3 · Dos ventas simultáneas no comparten folio (FR-005)

**Atrapa**: perder el candado de fila al cambiar de tabla contadora.

Dos transacciones concurrentes creando venta en el mismo turno. Números distintos y consecutivos,
sin repetir ni saltar. Sin este test, el cambio de `order_counters` a `folio_counters` se puede
hacer mal y nadie lo nota hasta el primer día ocupado.

## 4 · El turno que ya venía abierto continúa su numeración (FR-006)

**Atrapa**: el turno de 158 pedidos repartiendo el número 1 después de migrar.

1. Base con un turno abierto y pedidos numerados hasta N.
2. Correr la migración.
3. Crear una venta en ese turno → recibe N+1.

## 5 · Cerrar y reabrir el mismo día renumera sin colisionar (US2 escenario 4)

**Atrapa**: que alguien afloje la regla de "no se cierra con pedidos vivos" y convierta el reinicio
en una colisión real.

1. Turno con un pedido `abierta` → cerrar **falla**.
2. Terminar el pedido → cerrar funciona.
3. Abrir turno nuevo el mismo día → primera venta recibe folio 1, y no hay ningún pedido vivo con
   folio 1.

El paso 1 es el que sostiene la decisión de diseño: es la prueba de la premisa, no del código
nuevo.

## 6 · La corrección histórica no mueve dinero (FR-007, FR-008, SC-004)

**Atrapa**: una corrección de datos que reescribe más de lo que dice.

Contra una base restaurada de un respaldo real y **con al menos dos empresas** —con una sola, todo
camino "por cada otra empresa" es un no-op y la migración pasa verde para romper en producción:

1. Guardar las cifras de cada arqueo cerrado antes de migrar.
2. Correr la migración.
3. Cada cifra de arqueo idéntica, fila por fila.
4. `daily_number`, `folio_name` y `register_session_id` sin un solo cambio.
5. Cero ventas cuyo `business_date` difiera del día de `opened_at` en la zona del negocio.

## 7 · La migración se puede revertir (constitución: migración reversible)

`Up → Down → Up` deja la base igual. Después del `Down`, las fechas vuelven a ser las de antes.

## 8 · La tabla nueva funciona bajo el rol de la aplicación

**Atrapa**: el grant que falta — invisible en local, `42501` en el primer request de producción.

El mismo escenario 1, pero con el store conectado como `gatobobah_app` y no como owner. Cubre
también que la política de RLS no le esconda a la empresa su propio contador.

## 9 · Aislamiento entre empresas del contador (RLS)

Dos empresas con turnos abiertos. La numeración de una no ve ni afecta la de la otra.

## 10 · Las ventas del corte cuadran con el corte (US3)

**Atrapa**: una lista y un resumen de la misma pantalla derivados de predicados distintos — si
divergen, uno de los dos miente y quien lo lee no tiene forma de saber cuál.

1. Corte con ventas entregadas, una cancelada y una reembolsada.
2. `salesCount` = todas, incluidas las canceladas.
3. `salesTotal` = solo las que dejaron ingreso, sin propinas.
4. Ninguna venta de otro corte aparece.

## 11 · El aviso de turno viejo (US4)

- Turno abierto ayer → `deOtroDia: true`, la pantalla lo muestra.
- Turno abierto hoy → `deOtroDia: false`, sin aviso.
- Sin turno abierto → `deOtroDia: false`, y el aviso de "caja cerrada" que ya existe.
- **Con el aviso visible, cobrar sigue funcionando.** Es el que importa: un aviso que bloquea el
  cobro convierte un descuido administrativo en una caja parada.

## 12 · Pantalla, a 1024×600

- El detalle del corte sigue dejando ver el resumen, los gastos y lo declarado por método sin que
  la lista de ventas los empuje fuera.
- Todo control tappable ≥ 44 px. Ningún `<select>` nativo.
- El aviso de turno viejo no le come renglones a la barra del POS, que ya no tiene ancho libre.
