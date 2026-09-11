# Research: un solo arqueo de cajón

Seis decisiones que el spec dejó abiertas a propósito, con lo que se midió para tomarlas.

## 1. Dónde vive la diferencia única del cajón

**Decisión**: `session_cash_counts` gana `expected` y una columna **generada** `difference`. Los
renglones por método de `register_session_totals` de los métodos que tocan el cajón quedan con
`declared = expected`, o sea diferencia cero, y el arqueo del cajón es su propio hecho: contado
contra esperado, en una sola fila.

**Por qué**: la fila del conteo ya es *lo que el operador declaró*. Ponerle su esperado al lado la
vuelve el arqueo completo, y el sistema deja de tener que repartir una cifra que nadie repartió.

**Lo que lo hace barato ahora y caro después**: `session_cash_counts` nació en la migración 0066,
que está en el ambiente de pruebas y **no en producción** — `git log main..develop` la trae en tres
commits que main no tiene, y la tabla no existe en producción. Agregarle columnas hoy no toca un solo
dato real. El día que la 0066 llegue a producción, esto ya es una migración sobre datos vivos.

**Alternativas descartadas**:

| Alternativa | Por qué no |
|---|---|
| Concentrar la diferencia en el renglón del método «Efectivo» (sin migración) | Guardaría un `declared` que nadie declaró: `contado − Σ(esperado de los otros métodos de cajón)`. Es el mismo defecto que este spec viene a cerrar, con otro disfraz — un número derivado que se lee como una declaración |
| Volver `register_session_totals.declared` nulable para decir "no se declara por método" | Es el modelo más honesto y el más caro: la columna es `not null` con datos de producción, `difference` se genera de ella, y su tipo cambia en sqlc, en el dominio, en la vista del corte y en el histórico. Mucho radio de cambio para expresar algo que la fila del conteo ya dice |
| Guardar la diferencia calculada en `register_sessions` | Una tercera copia de la misma resta. La columna generada ya garantiza que nadie la escriba a mano |

## 2. Cómo se sabe qué efectivo llega al cajón

**Decisión**: `payment_methods.affects_cash_drawer`, que ya existe y ya tiene la granularidad
correcta — cada plataforma tiene su propio método de efectivo (`Didi efectivo`, `Uber Eats efectivo`,
`Rappi efectivo`), así que "depende de la plataforma" se configura por método sin inventar nada.

`kind = 'efectivo'` sigue significando otra cosa y no se toca: identifica al **único** método dueño
del fondo de apertura y de los movimientos de caja. Esa distinción ya está escrita en el comentario
de `ExpectedByMethodForSession` y nació de un defecto medido: sumar el fondo a todo lo que toca el
cajón lo contaba una vez por método y le inventó $4,500 de faltante a un turno.

**Alternativas descartadas**: un interruptor nuevo por plataforma (redundante con el que ya hay por
método), o deducirlo del nombre del método (el repo ya lo rechazó por escrito: se rompe el día que
alguien renombre «Uber Eats efectivo»).

## 3. Un método desactivado que ya cobró dinero en el turno abierto

**Decisión**: la consulta del esperado incluye los métodos **inactivos que tienen pagos en este
turno**. Hoy filtra solo por `pm.is_active`, así que apagar un método a media jornada haría
desaparecer del arqueo el dinero ya cobrado con él.

**Por qué es un camino real y no hipotético**: es exactamente lo que US2 vuelve posible. Hoy nadie
puede apagar un método desde la aplicación, así que el filtro nunca se ha ejercitado con dinero
adentro; el día que el interruptor exista, la primera vez que alguien lo use a media jornada el
turno pierde ese dinero del esperado y el arqueo cuadra contra una cifra más chica.

**Alternativas descartadas**: dejar el filtro como está (hace desaparecer dinero en silencio, que es
lo peor que puede hacer un sistema de caja), o prohibir desactivar un método con ventas en el turno
(bloquea una operación legítima: dejar de ofrecer un método a media tarde es normal).

## 4. Dónde vive el interruptor del arqueo ciego

**Decisión**: `business_settings`, que es por empresa y donde ya viven la zona horaria, el esquema de
folios y la impresión automática al cerrar. Lo lee la misma pantalla de Ajustes del negocio donde ya
está el interruptor de auto-declarar.

**Alternativas descartadas**: por caja (`cash_registers`) —el control es de política del negocio, no
del mueble— o por usuario (invitaría a exentarse a sí mismo, que es justo lo que el control evita).

## 5. Qué se le oculta al operador con el arqueo ciego

**Decisión**: la columna «Esperado» completa y el bloque de diferencia, para **todos** los métodos y
no solo para el efectivo. Lo que sigue viéndose es el total de lo que él mismo capturó: eso es su
captura, no la respuesta.

**Por qué todos**: ver el esperado de la tarjeta permite el mismo acomodo que ver el del efectivo. Un
control que tapa la mitad de la pantalla no es un control.

**Efecto lateral que conviene**: la tabla del cierre se acorta. A 1024×600 esa pantalla ya mide
1,494 px de alto, así que quitar una columna y tres campos de captura devuelve presupuesto en vez de
gastarlo.

## 6. Migración nueva, no editar la 0066

**Decisión**: una migración `0067` para las columnas nuevas.

**Por qué**: la 0066 ya está aplicada en el ambiente de pruebas y goose la tiene marcada, así que
editarla no la vuelve a correr — el esquema de dev se quedaría sin las columnas y el defecto
aparecería solo ahí. Es la misma razón por la que la FK invertida de la 0066 se corrigió antes de
que llegara a producción y no después.

## Lo que no hizo falta investigar

- **El cálculo del esperado del cajón** ya existe repartido: `sessionWithExpected` suma fondo y neto
  de movimientos al método dueño del fondo. Lo que cambia es a qué se compara, no cómo se calcula.
- **La conciliación con la plataforma** no se toca: lo que la app cobró y depositó vive en
  `platform_settlements` desde la 014, y es el lugar correcto para responder si el efectivo que se
  llevó un repartidor de la app llegó al depósito.
