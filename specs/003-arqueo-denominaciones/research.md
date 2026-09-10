# Research: Conteo de efectivo por denominaciones

Todo lo de aquí está **medido sobre el código y sobre producción**. Cada hallazgo dice qué decide.

## 1. Dónde entra y sale el efectivo hoy

| Momento | Qué se captura | Dónde |
|---|---|---|
| Abrir la caja | UN número: `openingCash` | `BackofficeService.OpenSession` |
| Cerrar la caja | UN número por método: `declared map[int]decimal` | `BackofficeService.CloseSession` |

Los dos ya validan con `domain.ValidMoney(..., allowZero=true)`: rechazan negativos y absurdos antes
de que desborden el `numeric(10,2)`.

**Decisión**: el conteo por denominaciones **reemplaza la captura**, no el cálculo. El total sigue
llegando a las mismas columnas y la validación existente sigue aplicando; lo que cambia es de dónde
sale ese número.

**Alternativa descartada**: guardar el total del conteo en una columna nueva y dejar las de hoy como
están. Serían dos cifras del mismo dinero, y el día que difieran nadie sabría cuál manda — es
exactamente el fallo que la constitución señala en su principio de dinero.

## 2. El arqueo ya es por caja, y hay tres

`register_sessions.register_id` es `not null` y el índice `one_open_session_per_register` permite una
sesión abierta **por caja**. En producción hay tres por empresa: principal, clip y externa.

**Decisión**: el conteo cuelga de la SESIÓN, no del negocio ni del día. Cada caja que maneje efectivo
lleva el suyo.

**Lo que esto descarta**: un conteo "del negocio" que sume las tres cajas. Sería una cifra que no
corresponde a ningún cajón físico, y contra la que nadie puede contar.

## 3. Solo el efectivo se cuenta

`payment_methods.kind` distingue `efectivo` de `tarjeta`, `transferencia` y `plataforma`. El corte ya
agrupa por método y `auto_declare` marca los que se declaran solos.

**Decisión**: el conteo aplica **únicamente** a los métodos de tipo efectivo. Los demás siguen siendo
una cifra. Es lo único que está físicamente en el cajón, y por lo tanto lo único de donde puede
salir una diferencia de arqueo.

## 4. La moneda vive en la sesión

`register_sessions.currency char(3) default 'MXN'`. El enum del dominio acepta `MXN` y `USD`; en
producción todo es MXN.

**Decisión**: las denominaciones se siembran **por moneda** en una tabla, no se cablean en el código.
No es especulación: la columna ya existe y el sistema se vende a más negocios. Sembrar solo MXN hoy
es una fila menos, no una tabla menos.

**Alternativa descartada**: una lista de constantes en Go. Agregar USD obligaría a un despliegue, y
peor: no habría dónde apuntar desde el conteo guardado, así que un arqueo viejo no podría decir
contra qué catálogo se contó.

## 5. Lo que ya se aprendió del arqueo en esta misma sesión de trabajo

Dos cosas construidas hace poco condicionan el diseño:

- **El cierre ya bloquea con pedidos sin entregar**, y el arqueo los lista. La pantalla de cierre ya
  tiene una sección que compite por el alto.
- **El arqueo ya desglosa por cajero.** Otra sección más.

**Decisión**: el conteo NO puede ser una lista de once renglones apilados. Con las dos secciones que
ya existen, el resumen del corte —lo que el operador vino a leer— quedaría fuera de pantalla en los
600 px de alto. El diseño tiene que resolverlo en una rejilla compacta.

## 6. Cómo se ve el dinero en esta base

`opening_cash numeric(10,2)`, `declared numeric(10,2)`, `difference` es columna **generada**
(`declared - expected`).

**Decisión**: las piezas son enteros y el valor de cada denominación es `numeric`; el total sale de
multiplicar y sumar **en el servidor**, con `Round2` en la frontera. El cliente manda piezas.

**Por qué importa que `difference` sea generada**: no hay que tocarla. Al escribir `declared` con el
total del conteo, la diferencia se recalcula sola y no puede quedar desincronizada.

## 7. Qué pasa con los arqueos ya cerrados

Producción tiene cortes cerrados de meses. Ninguno tiene desglose y ninguno puede inventárselo.

**Decisión**: el conteo es una tabla aparte con su llave a la sesión. Un corte sin filas es un corte
sin desglose, y la pantalla lo muestra como siempre. No se agrega una columna a
`register_session_totals` que quedaría nula para todo el histórico y obligaría a distinguir "nulo
porque es viejo" de "nulo porque no se contó".

## 8. La captura en una tableta

No hay componente de contador numérico reutilizable. Sí hay `Picker` para elecciones y los botones de
billetes del cobro (`BILLS = [50, 100, 200, 500, 1000]`), que es el gesto más parecido que el
operador ya conoce.

**Decisión**: la captura reusa ese lenguaje —tocar una denominación, ver el total subir— en vez de
once campos de texto. Un campo de texto por denominación son once toques de teclado en pantalla y
once oportunidades de un dedazo, que es justo el error que esta feature viene a quitar.

## 9. El motivo de la captura manual (FR-014)

`register_sessions.notes` existe y es texto libre del cierre.

**Decisión**: el motivo va en su **propia columna** del conteo y no en `notes`. Mezclarlo con las
notas del turno lo volvería imposible de encontrar, y FR-016 exige que todo arqueo esté explicado —
lo que exige poder preguntarle a la base cuáles no lo están.
