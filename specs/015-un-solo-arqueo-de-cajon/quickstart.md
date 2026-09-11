# Quickstart: verificar el arqueo de un solo cajón

Cómo comprobar que hace lo que dice. Un recorrido por historia, con las cifras puestas para que el
resultado sea verificable y no una impresión.

## Antes de empezar

- Una caja cerrada, para poder abrirla.
- Rol con permiso de caja, y rol de administración para los interruptores.
- Al menos un método de plataforma en efectivo activo (`Didi efectivo`, `Uber Eats efectivo` o
  `Rappi efectivo`).

## US1 — El arqueo del cajón es una sola cifra

1. Abre la caja contando **$500** (un billete de $500).
2. Cobra una venta de **$200 en efectivo** de mostrador.
3. Cobra una venta de **$100** con un método de plataforma **en efectivo**.
4. Entrega los dos pedidos y entra a cerrar.

**Se espera**: un solo renglón de cajón que espera **$800**. Los métodos de efectivo de plataforma se
siguen listando —para saber por dónde entró el dinero— pero **sin campo de captura y sin diferencia
propia**.

**Falla si**: te pide teclear una cifra para el efectivo de la plataforma. Ese es el reparto
inventado que esta feature viene a quitar: nadie puede partir un montón de billetes por canal.

5. Cuenta **$800** y confirma.

**Se espera**: diferencia **$0**, una sola.

6. Repite el recorrido contando **$750**.

**Se espera**: **un** faltante de **$50**. No un faltante de $50 en un método y un sobrante en otro.

**Falla si**: aparecen dos diferencias que se cancelan. Es el defecto medido del turno 3 —«Efectivo»
en $0.00 y «Didi efectivo» en −$64.80— con otro disfraz.

7. Abre ese corte en el histórico.

**Se espera**: la diferencia total del corte dice **−$50**, y se ve cuánto del efectivo entró por
mostrador y cuánto por la plataforma.

**Falla si**: el histórico dice que el corte cuadró. Ahí es donde alguien audita, y un cero de más
esconde justo lo que vino a buscar.

## US2 — Configurar los métodos de pago

1. En **Ajustes del negocio**, apaga un método de pago.

**Se espera**: deja de ofrecerse al cobrar un pedido nuevo.

2. Con el turno abierto, cobra algo con un método y **después** apágalo. Entra a cerrar.

**Se espera**: el arqueo **sigue esperando** ese dinero.

**Falla si**: el esperado baja al apagarlo. Sería dinero que entró y el sistema dejó de pedir: la
falla más grave posible en una caja, porque cuadra por construcción y nadie la ve.

3. Marca que el efectivo de una plataforma **no** llega al cajón, y cierra un turno con una venta de
   esa plataforma en efectivo.

**Se espera**: el esperado del cajón **no** incluye ese dinero, y no se te pide contarlo.

4. Abre un corte cerrado **antes** de cambiar los interruptores.

**Se espera**: sus cifras son exactamente las que tenía.

## US3 — Arqueo ciego

1. Enciende **arqueo ciego** en Ajustes del negocio.
2. Entra a cerrar un turno con ventas en efectivo.

**Se espera**: no aparece la columna de esperado, ni la diferencia, en ningún punto antes de
confirmar. Sí se ve el total de lo que **tú** contaste.

**Falla si**: la diferencia se alcanza a ver un instante mientras carga la pantalla, o queda en la
respuesta del servidor aunque la pantalla la oculte. Un control que se puede leer abriendo las
herramientas del navegador no es un control.

3. **Sin capturar nada**, intenta tocar «Cerrar caja».

**Se espera**: sigue **bloqueado**, y la pantalla dice qué falta por capturar sin decir cuánto se
espera.

**Falla si**: el botón está habilitado. Con el esperado en null, cualquier pantalla que deduzca "no
falta nada" de que la cifra sea cero deja firmar un arqueo vacío — es el faltante inventado de $1,662
por la puerta de atrás.

4. Captura el conteo y los métodos que no son de cajón, y confirma el cierre.

**Se espera**: ahí sí, la diferencia.

5. Apaga el interruptor y repite.

**Se espera**: la diferencia se ve antes de confirmar, como en la 003.

## Lo que se revisa en 1024×600

Con la caja teniendo pedidos pendientes y más de un cajero, que son las dos secciones que ya
compiten por el alto de esa pantalla:

- **Mide el alto de la tabla del cierre, no lo supongas.** Tiene tres campos de captura menos que
  antes, pero también un renglón más (el del cajón), así que el neto se mide con un navegador de
  verdad. Si empeoró, el recorte va antes del merge: esta pantalla ya mide 1,494 px contra 600.
- Con arqueo ciego, la columna de esperado desaparece y la tabla se angosta.
- En **Ajustes del negocio**, la lista de métodos pasa a ser una tabla de tres columnas de
  interruptores. Cuenta cuántos métodos se alcanzan a ver sin desplazarse — y para poder comparar,
  **mide primero cuántos se ven hoy**: ese *antes* no está medido en ningún documento, y sin él la
  comparación no se puede hacer. El ancho útil de esa página es ~520 px, no 1024: usa `<Page
  maxW="560px">` con su padding.
- Los tres interruptores de cada renglón miden al menos 44 px de alto, y «Activo» queda separado de
  los otros dos: es el único con efecto inmediato en el mostrador.

## El caso que hay que verificar con el front viejo en caché

La aplicación es una PWA con service worker, así que una tableta puede quedarse con la pantalla
anterior después del deploy.

1. Con el front viejo, intenta cerrar un turno declarando el efectivo por método.

**Se espera**: un 400 que **nombra el método** y dice que hay que recargar.

**Falla si**: el cierre pasa y la cifra tecleada se descarta en silencio. El operador se queda
creyendo que declaró algo que no se guardó.
