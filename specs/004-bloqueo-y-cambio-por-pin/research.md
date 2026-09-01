# Research: Bloqueo por inactividad y cambio de operador por PIN

Todo lo de aquí está **medido sobre el código y sobre producción**, no supuesto. Cada hallazgo dice
qué decide.

## 1. Cómo vive hoy una sesión

| Pieza | Valor | Dónde |
|---|---|---|
| Access token (JWT) | **15 minutos**, en memoria | `auth.AccessTokenTTL`, store `session` del front |
| Refresh token | **30 días**, cookie HttpOnly, rotado en cada uso | `app.RefreshTokenTTL` |
| Ante un 401 | el cliente canjea el refresh y reintenta, sin expulsar | `web/src/api/client.ts` |

**Decisión**: el límite de la historia 3 (la sesión no dura más que un turno) se aplica al **refresh
token**, no al access. Bajar el access no sirve: se renueva solo.

**Alternativa descartada**: acortar el access token a 8 horas. No hace nada — el refresh lo repone.

## 2. El hallazgo que cambia el diseño: `PinSwitch` emite una sesión NUEVA

`app.AuthService.PinSwitch` valida el PIN y llama a `issue()`, que **crea un refresh token nuevo con
30 días por delante**. Si la pantalla de bloqueo usara pin-switch tal cual:

- **Cada desbloqueo reiniciaría el reloj del turno.** Una tableta desbloqueada cada 20 minutos nunca
  caducaría, y la historia 3 quedaría en nada.
- Cada desbloqueo dejaría **un refresh token más** vivo, porque `issue()` no revoca los anteriores.

**Esto ya está pasando y se ve en producción**: un usuario tiene **4 refresh tokens vivos**, el más
viejo del 29 de agosto. No es un bug nuevo; es la consecuencia de que nada revoque al emitir.

**Decisión**: el cambio de operador NO puede emitir una sesión nueva. Tiene que cambiar **quién está
activo** dentro de la sesión del dispositivo, conservando el reloj de caducidad original.

**Alternativa descartada**: dejar que emita sesión nueva y guardar aparte "cuándo empezó el turno de
este dispositivo". Son dos relojes para un solo concepto, y el día que se separen nadie sabrá cuál
manda.

## 3. El PIN hoy: mínimo 4, sin unicidad

`auth.IsWeakPin` exige **4 o más caracteres** y rechaza todo-iguales y secuencias. **No hay ninguna
comprobación de que dos personas no tengan el mismo PIN.**

En producción: **6 de 8 usuarios activos tienen PIN**, todos de la época en que el mínimo era 4.

**Decisión**: el modo por default (elegir persona, luego PIN) se queda con el mínimo de 4 — el
nombre identifica y el PIN solo prueba. La unicidad y los 6 dígitos son requisito **solo** del modo
de solo-PIN, y por eso ese modo necesita una compuerta que verifique antes de dejarse encender.

## 4. La protección contra fuerza bruta ya existe

`pin-switch` está **exento del throttle per-IP** a propósito —es frecuente en el POS— pero sí tiene
**lockout per-usuario**, y la rama de "usuario no existe" corre `auth.CheckDummySecret` para igualar
la latencia y no filtrar qué ids existen.

**Decisión**: FR-010 no construye nada nuevo; reusa ese control. Lo que sí falta es que el evento de
desbloqueo fallido se registre con `logging.SecurityEvent`, como ya se hace con `pin_failed`.

## 5. El front ya tiene el cliente, y ninguna pantalla lo usa

`posApi.pinSwitch(userId, pin)` existe en `web/src/api/pos.ts`. **Ningún componente lo llama.**

**Decisión**: esta feature conecta lo que ya está, no lo inventa. El trabajo grueso es de pantalla y
de política de sesión, no de autenticación.

## 6. Dónde vive el operador activo

El store `session` guarda `{ token, user }`. Cambiar de operador es cambiar `user` — y el `user` del
JWT es lo que el servidor usa para `received_by` en cada pago.

**Decisión**: la atribución sale del token, no de un campo que mande la pantalla. Un cliente no
puede cobrar a nombre de otro sin tener su PIN.

**Riesgo detectado**: las cuentas abiertas viven en `localStorage` bajo `egb:ticket:v2`, que es del
DISPOSITIVO. Al cambiar de operador **no se limpian**, y eso es lo correcto (FR-013: las cuentas son
del mostrador), pero hay que dejarlo escrito para que nadie lo "arregle" después.

## 7. Detectar inactividad

No hay nada hoy. La opción estándar es escuchar eventos de interacción en la raíz de la aplicación y
reiniciar un temporizador.

**Decisión**: el temporizador vive en el cliente y el bloqueo es **de pantalla**, no de sesión — el
servidor no tiene por qué saber que la tableta está bloqueada. Lo que el servidor sí aplica es la
caducidad del refresh, que es la barrera real.

**Alternativa descartada**: bloquear también del lado del servidor invalidando el access token por
inactividad. Sería una barrera de verdad, pero exige que el servidor sepa de cada toque en la
pantalla — tráfico constante desde cada tableta para un riesgo que el bloqueo de pantalla ya cubre
en el escenario real (un mostrador, no un atacante con la tableta en la mano).

## 8. Qué pasa con lo capturado al bloquear

El carrito vive en `localStorage`, no en memoria de React. **Sobrevive incluso a una recarga**, así
que sobrevive a un bloqueo por definición.

**Decisión**: FR-002 no necesita trabajo especial; necesita un test que lo fije, porque es
exactamente la clase de garantía que alguien rompe sin querer al mover el estado a memoria.

## 9. La salida cuando alguien olvida su PIN (FR-011)

Hoy: `POST /me/pin` cambia el PIN propio **estando autenticado**, y un admin puede fijarlo desde la
pantalla de empleados.

**Decisión**: la salida es entrar con usuario y contraseña, que siempre funciona y no depende de
nadie más. La pantalla de bloqueo debe ofrecer ese camino de forma visible, no escondida.

**Alternativa descartada**: un PIN maestro del negocio. Un secreto compartido que abre cualquier
estación destruye la atribución que toda esta feature viene a construir.
