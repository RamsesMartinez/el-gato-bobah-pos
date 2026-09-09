# Quickstart: verificar el conteo por denominaciones

Cómo comprobar que la feature hace lo que dice, en el ambiente de pruebas. Un recorrido por historia.

## Antes de empezar

- Una caja cerrada, para poder abrirla.
- Rol con permiso de caja.

## US1 — Contar el fondo al abrir

1. **Caja → Abrir**. Captura 6 monedas de $10 y 3 billetes de $50.

**Se espera**: el total muestra **$210** mientras capturas, y la caja abre con esa cifra.

**Falla si**: hay que escribir el total, o el total no se actualiza al corregir una cantidad. Lo
segundo es peor que lo primero: el operador desconfía y saca la calculadora, que es justo lo que
esto viene a quitar.

2. Abre otra caja sin capturar nada.

**Se espera**: abre con **$0**. Una caja puede arrancar vacía y eso no es un error.

## US2 — Contar al cerrar

1. Haz un par de ventas en efectivo de importe conocido.
2. **Caja → Cerrar**. Captura piezas que sumen exactamente lo esperado.

**Se espera**: la diferencia es **$0**, y se ve **antes** de confirmar el cierre.

**Falla si**: la diferencia solo aparece después de cerrar. Enterarse de un faltante cuando el corte
ya está firmado es exactamente el problema que dejó el turno con $1,662 sin explicación.

3. Repite quitando un billete de $50 del conteo.

**Se espera**: un faltante de **$50**, visible antes de confirmar, y el cierre **deja continuar** —
un faltante no bloquea, o el operador se queda con la caja abierta toda la noche.

4. Fíjate en los métodos que no son efectivo.

**Se espera**: siguen pidiendo una sola cifra. Solo el efectivo se cuenta por piezas.

## US3 — Explicar una diferencia después

1. Abre el corte que cerraste con faltante.

**Se espera**: muestra cuántas piezas de cada denominación se declararon.

**Falla si**: solo muestra el total. Sin el desglose, un corte con diferencia vuelve a ser un número
sin historia y no se puede distinguir un error de conteo de dinero que no está.

2. Abre un corte **anterior** a esta funcionalidad.

**Se espera**: muestra su total como siempre, **sin desglose y sin inventar uno**.

## Los dos caminos, y que no coexistan

1. En el cierre, elige capturar el total a mano.

**Se espera**: pide un **motivo** y no deja continuar sin él.

2. Captura piezas y después cambia al total a mano.

**Se espera**: avisa que se va a descartar lo capturado antes de hacerlo.

**Falla si**: quedan las dos cifras. Serían dos verdades del mismo dinero.

## Lo que se revisa en 1024×600

Con la tableta en su resolución real, y con la caja teniendo pedidos pendientes y más de un cajero
—las dos secciones que ya compiten por el alto en esa pantalla:

- El conteo cabe **sin empujar fuera de vista** el resumen del corte, que es lo que el operador vino
  a leer.
- Cada control de captura mide al menos 44 px.
- El total acumulado se ve **sin desplazarse** mientras se captura. Si hay que hacer scroll para
  verlo, el operador vuelve a la calculadora.

## Los tres casos que la revisión de pantalla encontró sin cubrir (2026-09-08)

Los ejemplos de arriba cuentan 6 y 3 piezas. Ninguno ejercita lo que de verdad pone en riesgo los
criterios, así que se agregan estos:

### La captura no vive en el scroll de la caja

1. Con el turno abierto, entra a **Caja** y toca **Contar efectivo**.
2. Se abre una hoja que ocupa la pantalla completa. **Cuenta sin desplazarte**: las denominaciones,
   el total y el botón de confirmar se ven a la vez a 1024×600.
3. Confirma. Vuelves al cierre con la cifra puesta.

Lo que verifica: FR-013. Medido el 2026-09-08, el punto donde iría una rejilla inline está en
y=796 de una página de 1,494 px contra un viewport de 600 — inline no cabe, y este paso lo prueba.

### Contar 40 monedas no cuesta 40 taps

1. En la hoja, **toca el número** de las monedas de $1 (no el botón de +).
2. Escribe `40`. El total sube $40 de una vez.
3. Ajusta con **+** y **−** si te sobró o faltó una.

Lo que verifica: SC-003 en el caso de bulto. Con tap = +1 puro, 40 piezas son 40 taps y la vara de
UX del POS —minimizar taps— se rompe dentro del propio tope de 60 piezas del criterio.

### La diferencia se ve ANTES de cerrar

1. Cuenta piezas que sumen **menos** de lo que el turno espera en efectivo.
2. Confirma la hoja y vuelve al cierre.
3. **Sin tocar Cerrar caja**, el faltante ya está a la vista junto al botón.

Lo que verifica: FR-005. Hoy la única tabla con columna de diferencia se pinta en el diálogo
POSTERIOR al cierre, cuando el servidor ya cerró la sesión — o sea, hoy este paso falla.
