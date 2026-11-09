# Research: Aviso por correo de cada error interno

## R1. Dónde se detecta el 500

- **Decision**: un middleware propio (`reportServerErrors`) en lugar de `middleware.Recoverer`, puesto
  justo después de `RequestID`. Envuelve el `ResponseWriter` para ver el estado final y hace el
  `recover`. `httpapi.Error()` le entrega el error y su traza buscando, por la cadena de `Unwrap()`
  del writer, al que sabe registrarlo.
- **Rationale**: todos los 500 de la API salen de `Error()` (verificado: ningún otro
  `StatusInternalServerError` en `httpapi`) o de un panic. `Error()` solo recibe `w`, no el request,
  así que el writer es el único canal sin cambiar ~600 llamadas.
- **Alternatives**: leer el log (`slog`) con un handler que mande correo — mezcla bitácora con
  transporte y no ve el request; cambiar la firma de `Error` — toca cientos de llamadas.

## R2. Patrón de ruta

- **Decision**: `chi.RouteContext(ctx).RoutePattern()` leído después de `next`. chi crea el contexto
  de ruta antes de correr los middlewares, así que el middleware externo lo ve lleno al regresar.
- **Rationale**: no lleva query (`?token=` de los avisos en vivo) y agrupa ids.

## R3. Tipo de error

- **Decision**: desenvolver hasta el error de fondo (en un `Join`, el primero); `%T` de ése; si en la
  cadena hay un `*pgconn.PgError`, `pg:<SQLSTATE>`. Panic: `panic:<%T del valor>`.
- **Alternatives**: el texto (trae ids → un correo por pedido); el marco del código donde se llamó
  `Error()` (siempre es el handler, no distingue nada que la ruta no distinga ya).

## R4. Agrupamiento entre procesos

- **Decision**: Redis. `SET alert:win:<k> <proceso> NX PX 15m30s` decide quién gana la ventana. El
  que gana avisa ya y programa el cierre a los 15 min; los demás hacen `INCR alert:count:<k>`
  (vence en 1 h). Al cierre, el dueño hace `GETDEL` del conteo: si > 0 manda el resumen y renueva la
  ventana (`SET … XX`); si es 0, borra la ventana solo si sigue siendo suya (script Lua).
  Al ganar una ventana nueva también hace `GETDEL` del conteo: lo que quede es de un proceso que
  murió sin cerrar (FR-013).
- **Rationale**: el TTL de la ventana (15 min 30 s) es mayor que el timer (15 min) para que nadie
  más la gane entre que el timer dispara y el dueño decide.
- **Fallback**: si Redis no está o una operación falla, la misma lógica sobre un mapa en memoria del
  goroutine del notifier (sin mutex: solo él lo toca).
- **Alternatives**: solo memoria — duplica durante azul/verde y pierde el conteo en cada subida.

## R5. No bloquear

- **Decision**: canal con capacidad 64; el request hace `select { case ch <- a: default: dropped++ }`.
  Todo lo lento (Redis, base, SMTP) ocurre en el goroutine del notifier. El `mailer.Send` ya está
  acotado (10 s de conexión, 30 s de sesión).
- **Consecuencia**: con SMTP colgado, el goroutine tarda hasta 40 s por correo; mientras, la cola
  se llena y descarta. Se acepta: el siguiente correo dice cuántos se descartaron.

## R6. Enmascarado

- **Decision**: en `domain`, regex sobre el texto del error/panic: JWT (`eyJ…\.…\.…`), `Bearer …`,
  `clave=valor`/`"clave":"valor"` para password, pin, token, secret, authorization, api_key,
  card/tarjeta, cvv; secuencias de 13–19 dígitos (con espacios o guiones). Recorte a 1000 caracteres.
  El correo va en HTML con todo escapado (`html.EscapeString`).

## R7. Destinatario y ambiente

- **Decision**: `ALERT_EMAIL_TO` (vacío = apagado). El ambiente se rotula con el host de
  `APP_BASE_URL` (`app.elgatobobah.com` vs `app-dev…`), porque `APP_ENV` vale `production` también
  en la VM de pruebas.

## R8. Nombre de la empresa

- **Decision**: `CompanyService.Name(ctx, id)` con `store.WithTenant(id)` y `GetCompany` (RLS «solo
  mi fila»), plazo de 2 s. Si falla, el correo lleva solo el id.

## R9. Lo que agregó la revisión de código

- **`X-Request-Id`** del cliente solo se acepta con forma `[A-Za-z0-9._-]{1,64}` (el front manda un
  UUID); otro valor se reemplaza por un UUID nuevo. Entraba tal cual al correo y a la bitácora.
- **Cuerpo del correo en base64** (líneas de 76) en `mailer.buildMessage`, para todos los correos:
  RFC 5322 limita la línea a 998 octetos y el HTML del aviso no tiene saltos.
- **Enmascarado ampliado**: claves con prefijo (`client_secret`, `X-Api-Key`), `cookie`, `otp`, el
  eco de valores de Postgres (22P02/22007) y corridas opacas de 24+ caracteres con letras y dígitos.
- **`ALERT_EMAIL_TO`** inválido detiene el arranque (`config.Validate`).
- Si se gana la ventana en Redis pero falla leer el conteo huérfano, la ventana sigue en Redis.
