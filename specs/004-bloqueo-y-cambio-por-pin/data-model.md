# Data Model: Bloqueo por inactividad y cambio de operador por PIN

## Lo que cambia en la base

### `business_settings` — ajustes de identificación

Tres columnas nuevas en la tabla que ya lleva los ajustes por negocio (ahí viven la comanda de
cocina y el cobro desde Pedidos).

| Columna | Tipo | Default | Por qué |
|---|---|---|---|
| `pin_only_unlock` | `boolean not null` | `false` | Si desbloquear pide solo el PIN o pide nombre y PIN. Nace apagado: el modo seguro es el default y el otro se enciende a propósito (FR-004, FR-005) |
| `lock_after_seconds` | `int not null` | `180` | Cada cuánto se bloquea la pantalla sin actividad (FR-001, FR-016) |
| `session_hours` | `int not null` | `8` | Cuánto dura una sesión antes de exigir credenciales completas (FR-009, FR-016) |

**Por qué en `business_settings` y no constantes**: la constitución prohíbe config para un valor que
nunca cambia, pero estos sí cambian por negocio — un local con mostrador a la vista quiere tres
minutos y uno con la caja en una oficina cerrada quiere quince. Es la misma razón por la que ahí
viven `print_kitchen_ticket` y `kitchen_can_charge`.

**Por qué segundos y horas en columnas distintas**: son magnitudes distintas y ponerlas en la misma
unidad obligaría a `session_hours` a viajar como 28800, un número que nadie lee bien en una pantalla
de ajustes.

### `refresh_tokens` — nada estructural

`expires_at` ya existe. Lo que cambia es **quién lo calcula**: hoy es una constante de 30 días y pasa
a salir de `session_hours` del negocio.

**Riesgo que esto abre y cómo se cierra**: los tokens ya emitidos traen 30 días por delante. Bajar la
constante NO los acorta. La migración tiene que decidir explícitamente qué hace con ellos (ver
`quickstart.md`); dejarlos vivos significaría que la protección no aplica hasta que cada tableta
vuelva a entrar, y eso puede ser en semanas.

### `users.pin_hash` — nada estructural

Gana **reglas**, no columnas: largo mínimo y unicidad, pero **solo cuando el negocio tiene el modo de
solo-PIN encendido**. No hay índice único sobre el hash — bcrypt saliniza, así que dos PINs iguales
producen hashes distintos y un índice no los detectaría. La unicidad se verifica comparando el PIN
candidato contra los hashes de las personas activas, en el servidor.

## Lo que NO cambia, y por qué importa decirlo

- **`order_payments.received_by`** ya guarda quién cobró y ya lo usa el arqueo. Esta feature hace que
  ese dato sea confiable; no lo toca.
- **Las cuentas abiertas** (`localStorage`, clave `egb:ticket:v2`) siguen siendo del DISPOSITIVO.
  Cambiar de operador no las limpia — son las cuentas del mostrador, no de la persona (FR-013).
- **Los roles** siguen decidiendo qué puede hacer cada quien. El operador activo cambia; los permisos
  se recalculan con el token de esa persona.

## Entidades del dominio

### Operador activo

No es una tabla. Es **quién viene en el token que la estación está usando**. Cambia al desbloquear.

Su valor está en que el servidor lo lee del token y no de un campo que mande la pantalla: nadie
puede cobrar a nombre de otro sin tener su PIN.

### Sesión del dispositivo

La fila de `refresh_tokens` que la estación viene rotando. Su `expires_at` es el reloj del turno.

**La regla que sale del hallazgo 2 de la investigación**: cambiar de operador **conserva ese reloj**.
Si cada desbloqueo emitiera un token nuevo con el plazo completo, una tableta que se usa cada veinte
minutos no caducaría nunca y `session_hours` sería decorativo.

### Política de identificación del negocio

Los tres ajustes de arriba, leídos juntos. La pantalla de bloqueo los necesita antes de saber qué
pedir, así que viajan con los ajustes que el POS ya carga al arrancar.

## Reglas que el dominio debe hacer cumplir

| Regla | Dónde vive | Qué rompe si falta |
|---|---|---|
| Un PIN tiene al menos 4 caracteres, sin secuencias | ya existe (`auth.IsWeakPin`) | — |
| Con solo-PIN activo, al menos 6 dígitos | dominio, función pura con test | Un dedazo cae en el PIN de otro 100 veces más seguido |
| Con solo-PIN activo, PIN único entre personas activas | servicio, comparando contra los hashes vigentes | Dos personas se desbloquean la una a la otra y la atribución del arqueo miente |
| No se puede encender solo-PIN si algún PIN no cumple | servicio, verificación previa que NOMBRA a quiénes | El negocio queda en el estado roto de arriba sin enterarse |
| El cambio de operador conserva el vencimiento de la sesión | servicio de autenticación | `session_hours` no aplica nunca |
| Un desbloqueo fallido se frena y se registra | ya existe el lockout; falta el evento | Fuerza bruta sobre un PIN de 4 dígitos, sin rastro |
