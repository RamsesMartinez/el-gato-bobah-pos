# Seguridad — Auditoría OWASP Top 10 y endurecimiento para MVP

> Auditoría 2026-07-19. Código 100% generado por IA → tratado como **riesgo alto**:
> todo hallazgo se verificó de forma adversarial contra el código real. Repo público.
> Alcance: backend Go (`server/`) + frontend React (`web/`) + despliegue (`deploy/`).

## Resumen ejecutivo

Base sólida para un POS de un solo local: **SQL 100% parametrizado** (sqlc, sin
inyección), **sin SSRF**, refresh tokens `HttpOnly`+`SameSite=Strict`+rotados, API
distroless/non-root, secretos **no** versionados. La auditoría produjo **50 hallazgos
verificados** (4 altos, 15 medios, 22 bajos, 9 informativos). Se corrigieron los **3
bloqueadores de lanzamiento** y el hardening barato de alta señal. `govulncheck` y
`bun audit` quedan **limpios**.

## Bloqueadores de lanzamiento — CORREGIDOS

| ID | OWASP | Problema | Corrección | Test |
|----|-------|----------|-----------|------|
| B1 | A04/A07 | Cero rate limiting en `/auth/*` (el comentario afirmaba que existía). Fuerza bruta anónima + escalada vía `/pin-switch` (PIN 4 dígitos). | Limiter en memoria: throttle per-IP (proxy-safe) en `/login`+`/refresh` + lockout per-cuenta (usuario/PIN). Mapas con poda. | `ratelimit_test.go` |
| B2 | A03 | XSS almacenado en `printReceipt.ts` (nombres de producto/cliente → `document.write` sin escapar → robo de token). | `buildReceiptHtml()` pura que escapa todo string de usuario. | `printReceipt.test.ts` |
| B3 | A07 | `ADMIN_PIN=1234` por defecto en repo público, sin enforcement. | `checkAdminSecrets` (arranque) rechaza PIN trivial/placeholder; `check-env.sh` y `.env.example` corregidos. | `auth_test.go` (`IsWeakPin`) |

## Hardening adicional aplicado (no bloqueaba, pero recomendado)

| OWASP | Cambio | Archivo |
|-------|--------|---------|
| A02 | `JWT_SECRET` validado en arranque (≥32, no placeholder); la API no arranca si es débil. | `config.go` |
| A05 | `CORS_ORIGIN` fail-closed: `*` prohibido en producción (la API no arranca); default vacío = solo mismo origen. | `config.go`, `router.go` |
| A05 | Headers de seguridad en Caddy: HSTS, `nosniff`, `X-Frame-Options: DENY`, CSP `frame-ancestors 'none'`, `Referrer-Policy: no-referrer`. | `Caddyfile` |
| A04 | Límite de body 1 MiB (`MaxBytesReader`) + `ReadTimeout` 15 s (sin `WriteTimeout`, para no romper SSE). | `router.go`, `main.go` |
| A01 | Role-gates: `/stock`, `/expenses`, `/products/{id}/costing` → admin+gerente; `/cash-sessions` → admin+gerente+cajero. | `router.go` |
| A04 | Máquina de estados de orden (`domain.CanTransition`): sin regresiones, `entregada`/`cancelada` terminales, cancel idempotente (sin doble-restock), doble-tap = no-op. | `domain/order.go`, `orders.go` |
| A07 | `IsActive` re-verificado en `Refresh` (empleado dado de baja pierde acceso al instante). | `app/auth.go` |
| A06 | Bump toolchain Go → 1.25.12 (cierra GO-2026-5856 en crypto/tls). | `go.mod` |
| A05 | Cap de logs de contenedor (json-file 10m×3) → no llena el disco del VPS. | `docker-compose.yml` |

## Post-MVP — CORREGIDO (segunda ronda de endurecimiento)

| OWASP | Cambio | Archivo(s) | Test |
|-------|--------|-----------|------|
| A07 | Timing oracle de enumeración: bcrypt de descarte (`CheckDummySecret`) en las ramas "usuario no encontrado" y "sin password" del login. | `auth/password.go`, `app/auth.go` | `auth/timing_test.go` |
| A07 | Reuse-detection de refresh: reusar un token ya revocado revoca **toda la familia** del usuario + evento de seguridad. Clasificador puro `domain.ClassifyRefresh`. | `domain/refresh.go`, `app/auth.go` | `domain/refresh_test.go` |
| A04 | Validación de rangos central (`domain.ValidMoney`/`ValidQty`): rechaza NaN/±Inf y topes bajo el límite de cada columna. Cableada en orders/pagos/gastos/caja/stock. Evita overflow→500 y wrap de int16 en modificadores. | `domain/limits.go`, `domain/order.go`, `app/orders.go`, `app/backoffice.go` | `domain/limits_test.go`, `domain/order_test.go` |
| A09 | Body/PII (customerName, notas) fuera de logs normales: solo en `LOG_LEVEL=debug` o 5xx. Eventos de seguridad distintos (`login_failed`, `pin_failed`, `auth_lockout`, `forbidden`) vía `logging.SecurityEvent`. | `httpapi/logging.go`, `httpapi/handlers.go`, `httpapi/middleware.go`, `logging/security.go` | `httpapi/logging_test.go`, `httpapi/middleware_test.go` |
| A02/A05 | Access token fuera de `localStorage` → slice en memoria de zustand; re-emitido con la cookie de refresh al recargar (`restoreSession` al arrancar). | `web/stores/session.ts`, `web/api/client.ts`, `web/App.tsx`, `web/app/RequireAuth.tsx` | `web/stores/session.test.ts` |
| A07 | **La sesión dura un turno, no 30 días, y el turno no se renueva solo.** El vencimiento del refresh sale de `business_settings.session_hours` (default 8 h) y **la rotación lo CONSERVA**: reponer el plazo en cada rotación lo corría hacia adelante, y como el front rota al volver el foco a la ventana, una tableta que alguien toca cada rato no caducaba nunca. Sin fila de ajustes cae al default del negocio, no a los 30 días. | `app/auth.go`, `migrations/0050_ajustes_identificacion.sql` | `integration/sesion_caduca_test.go`, `integration/el_turno_no_se_renueva_solo_test.go` |
| A07 | **El reloj de la sesión es de la ESTACIÓN, no de la persona.** El relevo hereda el vencimiento del refresh que esa tableta presenta y revoca solo ese, en **una sola sentencia** (`TomarSesionDeEstacion`): leer y revocar por separado dejaba que dos peticiones con la misma cookie acuñaran dos credenciales de una, y buscar por `user_id` hacía que una tableta heredara el reloj más lejano de la otra. El token presentado tiene que ser de quien viene operando. | `app/auth.go`, `queries/users.sql` | `integration/pin_switch_conserva_reloj_test.go`, `integration/relevo_concurrente_test.go` |
| A07 | **Bloqueo de pantalla por inactividad**, configurable por negocio. Va ENCIMA de la aplicación y no la desmonta: si se perdiera lo capturado, el operador aprendería a impedir el bloqueo y la protección se caería sola. | `web/features/auth/`, `migrations/0050_…` | `web/features/auth/noSePierde.test.tsx`, `inactividad.test.ts` |
| A07 | **Desbloqueo por PIN sin enumerar usuarios**: id inexistente y PIN incorrecto dan la misma respuesta y la misma latencia (bcrypt de descarte); el evento de seguridad lleva a quién se intentó y **nunca el PIN**. Sin `userId` y sin modo de solo-PIN, se rechaza en vez de caer al modo permisivo. | `app/auth.go`, `httpapi/handlers.go` | `httpapi/pin_seguridad_test.go`, `integration/pin_seguridad_test.go` |
| A07 | **Encender el modo de solo-PIN es atómico.** Guardar el ajuste y borrar los PINs viejos van en la misma transacción: eran dos autocommits, y el reintento no reparaba porque al segundo intento el ajuste ya decía "encendido" y el borrado no volvía a correr — el negocio quedaba en un modo de seis dígitos con PINs de cuatro. El borrado alcanza también a quien está dado de baja. | `app/settings.go`, `queries/users.sql` | `integration/encender_solo_pin_es_atomico_test.go`, `integration/solo_pin_sin_puertas_traseras_test.go` |
| A07 | **Cada modo de desbloqueo cierra el camino del otro, y falla cerrado.** Con solo-PIN encendido se rechaza elegir persona (dos rutas al mismo desbloqueo con lockouts separados duplican el presupuesto de intentos); con el modo apagado se rechaza el camino que deduce la persona por su PIN. El largo del PIN se exige **al desbloquear**, no solo al capturar. Un error de lectura de los ajustes se propaga en vez de degradar al modo permisivo. | `app/auth.go`, `app/users.go` | `integration/solo_pin_sin_puertas_traseras_test.go` |
| A05 | **`ADMIN_PIN` pasa por el mismo filtro que `ADMIN_PASSWORD`**: valor de ejemplo, no numérico o trivial → la API no arranca. El de `deploy/.env.example` se aceptaba, y `/auth/unlock-options` publica el id del administrador a cualquier autenticado. Los tres caminos de arranque calculan la huella del PIN por el mismo helper. | `cmd/api/main.go` | `cmd/api/admin_pin_de_ejemplo_test.go`, `cmd/api/pin_de_admin_test.go` |
| A02/A07 | **Huella determinista del PIN** (HMAC con `PIN_PEPPER`) junto al bcrypt, solo para comparar por igualdad y deducir de quién es. bcrypt saliniza y no puede hacer ninguna de las dos. Un índice único de la base impide dos PINs iguales por empresa; el rechazo **no dice de quién** es el repetido, o el formulario sería un oráculo. Sin el secreto, el modo de solo-PIN no se enciende (fail-closed). | `domain/pin.go`, `app/users.go`, `app/settings.go`, `migrations/0051_pin_lookup.sql` | `domain/pin_test.go`, `integration/pin_unico_test.go` |
| A05/A10 | CSP completo (probado contra el build real en Chrome headless, 0 violaciones). | `deploy/Caddyfile` | verificación headless |
| A06/A08 | `bun audit` bloqueante (`--audit-level=high`); Actions clavadas por SHA; imágenes base por digest; Dependabot (actions/gomod/npm/docker). | `.github/workflows/ci.yml`, `.github/dependabot.yml`, `server/Dockerfile`, `deploy/docker-compose.yml` | — |
| A01/UX | Gating client-side por rol (nav + guardas de ruta), espejo de los `RequireRole` del backend. | `web/app/roles.ts`, `web/app/AppShell.tsx`, `web/app/RequireAuth.tsx`, `web/App.tsx` | `web/app/roles.test.ts` |

### Verificación adversarial de la ronda post-MVP

Se revisó cada control de forma adversarial (¿un atacante lo evade?). La revisión halló y se
**corrigieron 11 defectos** sobre los propios cambios (5 commits de fix):

- **A02/A05 (alto, regresión):** "Salir" no llamaba a `/auth/logout`, así que la cookie de
  refresh sobrevivía y el arranque re-autenticaba al operador que acababa de salir en una
  tablet compartida. Ahora `logout` la revoca en el server. Además, el refresh de arranque
  podía pisar un login concurrente → el single-flight ahora solo aplica si nadie autenticó.
- **A07 (medio):** `PinSwitch` tenía el mismo oráculo de temporización que Login (y sin
  throttle per-IP, lockout per-userID) → enumeración de usuarios; ahora corre bcrypt de
  descarte. La rotación de refresh era read-then-revoke no atómico → dos presentaciones
  concurrentes acuñaban dos tokens; ahora `RevokeRefreshTokenIfActive` (UPDATE condicional).
- **A04 (medio/bajo):** qty de línea validado sin redondear (0.001 → 500) y la depleción de
  stock de la venta sin `ValidQty` (overflow del numeric → 500); `RecordMovement` usaba
  Round2 en una columna de 4 decimales. Corregidos con Round2/Round4 + validación.
- **A06/A08 (bajo/medio):** faltaba `permissions: contents: read`; `version:/bun-version:
  latest` no pinados; Dependabot `/web` estaba en ecosistema `npm` (no actualiza `bun.lock`)
  → cambiado a `bun`.
- **A09/A05 (bajo):** PII de cliente (customerName/notes) se registraba en cuerpos de 5xx/
  debug → añadida al redactor; `object-src 'none'` añadido a la CSP.

## Refund/void de orden entregada — IMPLEMENTADO

El negocio confirmó que sí hay devoluciones. Flujo dedicado (no reutiliza cancel):

- Estado nuevo `reembolsada` (terminal), solo alcanzable desde `entregada`
  (`domain.CanRefund`, migración 0018). Tratado como **pérdida**: sin restock (la mercancía
  se consumió; el costo ya descontado ES la pérdida de inventario), solo se revierte el
  ingreso. `refund_amount`/motivo/actor persistidos.
- Reportes: `SalesByDay`/`ProductMargins` excluyen `reembolsada`; `RefundsByDay` la reporta
  como pérdida por devolución.
- `POST /orders/{id}/refund` gated admin/gerente + evento de seguridad `order_refund`;
  UI en el tablero (sección "Entregadas hoy", solo admin/gerente).
- Verificado end-to-end con tests de integración contra Postgres real.

## El arqueo ciego — control antifraude (spec 015, 2026-09-10)

El negocio puede encender **«Contar sin ver lo esperado»** (`business_settings.blind_cash_count`):
quien cuenta el cajón no ve la cifra contra la que se va a comparar, y la diferencia aparece al
confirmar el cierre. Protege contra que alguien acomode lo que declara para que cuadre.

**Qué se nulifica, dónde y por qué ahí.** Lo que la pantalla no debe mostrar **no se le manda**:
ocultarlo en el cliente dejaría la cifra en la respuesta, legible con las herramientas del
navegador. Vive en `app.vistaDelTurnoAbierto`, que es el único camino por el que una pantalla
recibe un turno abierto, y no en cada llamador — lo era, y de los cuatro caminos dos se lo saltaban:
registrar un movimiento de caja, abierto a rol cajero, devolvía el esperado completo, así que una
entrada de un centavo alcanzaba para leerlo.

**Nulificar `expected` no basta, y creerlo es lo que convierte el control en un adorno.** La misma
respuesta llevaba el desglose por método y lo cobrado por cada cajero, y la pantalla los pintaba
arriba de la tabla del cierre: fondo 500 + neto 0 + Ventas del mostrador 200 + Ventas de Didi
efectivo 135 = **835**, que era exactamente el esperado oculto, sin abrir nada. Por eso se va todo
lo que descompone la venta del turno (`breakdown.ingresos`, `breakdown.plataformas`,
`cashiers[].cash/other`) y se queda lo que el propio operador registró y ya conoce: el fondo con el
que abrió y sus movimientos.

| Qué | Dónde | Test |
|---|---|---|
| Nulificado en un solo lugar, para los cuatro caminos de lectura | `app.vistaDelTurnoAbierto`, `app.ocultarLoEsperado` | `TestConArqueoCiegoElEsperadoNoViaja`, `TestConArqueoCiegoUnMovimientoNoDevuelveElEsperado` |
| Lo derivado tampoco reconstruye la cifra | idem | `TestConArqueoCiegoLoDerivadoNoReconstruyeElEsperado`, `arqueo-ciego.spec.ts` › B1 |
| Sin esperado, el servidor sigue diciendo qué falta capturar | `MethodTotal.RequiresEntry`, `ArqueoDelCajonView.RequiresCount` | `TestConArqueoCiegoElServidorSigueDiciendoQueFalta`, › B2 |
| El fail-open se registra | `app.arqueoCiego` → `blind_count_unavailable` | — |

**Alcance declarado, porque un control cuyo límite no está escrito se cree más grande de lo que
es**: ata a quien **cuenta** el cajón. Un rol de administración llega a las mismas cifras por
`GET /sales/summary`, que es su trabajo. Si algún día tiene que atar también a administración, eso
es una feature nueva y no un ajuste de ésta.

**Clave de evento renombrada**: `payment_method_auto_declare_changed` → `payment_method_changed`,
porque el endpoint dejó de escribir un solo interruptor. Queda anotado aquí porque la clave estable
es requisito y una alerta sobre la vieja dejaría de coincidir en silencio. El evento lleva ids y
booleanos, cero PII.

## Testing de integración (Postgres efímero)

Suite `internal/integration` (build tag `integration`) contra un Postgres real —cubre lo que
los tests unitarios no alcanzan por el `*db.Queries` concreto: reuso de refresh→revoca
familia, rotación, y el flujo de reembolso. Job de CI `integration` con servicio
postgres:16-alpine (digest-pin). `go test ./...` normal no se ve afectado (skip sin
`TEST_DATABASE_URL`).

## Tercera ronda — lo que encontró la revisión adversarial de la spec 016 (2026-09-11)

Cuatro defectos que **no los trajo esa feature**: los destapó auditarla. Tres viven en el login del
negocio, que lleva meses en producción.

| OWASP | Defecto | Escenario concreto | Arreglo | Test |
|-------|---------|--------------------|---------|------|
| A07 | **El lockout por cuenta se evadía cambiando mayúsculas** | `users.username` y `companies.slug` son **citext**: `admin` y `ADMIN` son el mismo usuario para autenticar, pero la llave del limitador se armaba con el cuerpo crudo. `admin` × `gatobobah` da 2⁵ × 2⁹ = **16,384 contadores** para la misma credencial, así que quien adivina desde varias IPs solo tenía que alternar mayúsculas. El bloqueo B1 dejaba de morder | `domain.NormalizarUsuario` antes de armar la llave, en los dos logins | `TestElLockoutDelLoginNoDistingueMayusculas` |
| A04 | **El usuario del login no tenía cota en la frontera** | Un `username` de 900 KB (cabe en el `maxBody` de 1 MiB) viajaba a la llave del limitador —Redis de 128 MB con `allkeys-lru`: ~145 peticiones lo llenan y empiezan a desalojarse los contadores del POS, con el limitador **fallando abierto**— y al evento de seguridad, que rota a 1 MB con 10 respaldos: **once peticiones se llevan toda la bitácora** de intentos anteriores | `domain.UsuarioValido` (1–64, sin espacios ni `@`) en los dos handlers, rechazando con el **mismo 401 y el bcrypt de descarte** para no volver la forma del usuario en un oráculo | `TestUnUsuarioAbsurdoSeRechazaSinLlegarAlLimitador`, y su gemelo de la consola |
| A09 | **La IP de la bitácora la elegía el cliente** | `clientIP` tomaba la **primera** entrada de `X-Forwarded-For`. `curl -H 'X-Forwarded-For: 8.8.8.8'` dejaba cada `login_failed` con la IP que el atacante quisiera. El throttle nunca se pudo evadir así (`rateKeyIP` ya usaba la última), pero en un subdominio público la bitácora es lo único que queda de un intento | `clientIP` toma la **última**, igual que `rateKeyIP`: Caddy agrega el peer real al final | `TestLaIPDelEventoNoLaEligeElCliente` |
| A05 | **Un `APP_ENV` con typo apagaba las dos mitades del CORS** | `Validate` solo prohíbe `*` cuando el valor es exactamente `production`, y el router refleja cualquier Origin cuando es cualquier cosa **distinta** de `production`. Un `APP_ENV=prod` cae en el peor cuadrante de los dos: arranca con `*` **y** refleja | Lista cerrada: solo `development` o `production`; cualquier otra cosa no arranca | `TestValidate_RechazaUnAmbienteDesconocido` |

Y uno que sí era de la 016: sin `PLATFORM_DB_PASSWORD` el compose arma una URL que *parece* válida
(`postgres://gatobobah_platform:@postgres:…`), el bootstrap no le fija contraseña al rol porque no
hay ninguna, y la API muere al conectar con un error que no nombra la variable. Ahora `Validate` la
exige en producción.

## La consola de plataforma (spec 016) — tres barreras, ninguna es un `if`

Superficie nueva en `staff.elgatobobah.com`, para quien VENDE el sistema. Lo que la separa del
negocio no es una comprobación en el código, y esa es toda la decisión:

| OWASP | Barrera | Qué la impone | Test |
|-------|---------|---------------|------|
| A01 | Las credenciales no se cruzan | `platform_operators` es otra tabla, **sin `company_id`**: el login del negocio consulta `users` y ahí no está. | `consola_separada_test.go` |
| A01/A02 | Los tokens no se cruzan | **Dos secretos de firma**. Un token de la consola no valida en el negocio porque la firma no coincide, no porque alguien lo revise. `config.Validate` no arranca si `PLATFORM_JWT_SECRET` falta, es débil o **es igual a `JWT_SECRET`**. | `auth/plataforma_test.go`, `config_test.go` |
| A01 | La consola no alcanza la operación | Rol `gatobobah_platform` con `select` **solo** sobre `companies` y `platform_operators`. Un endpoint que por descuido consultara `orders` falla con `42501`. **Nunca `BYPASSRLS`**; la consola ve todas las empresas por una política de RLS acotada a `select`. | `consola_sin_permisos_test.go`, `migracion_consola_test.go` |
| A05 | El rol correcto, comprobado al arrancar | `store.AssertPlatformGrants`: aborta si el rol es superusuario o si un `select` canario sobre `orders` **no** falla con `42501`. Cierra el `PLATFORM_DATABASE_URL` copiado de `DATABASE_URL`. | `migracion_consola_test.go` |
| A07 | No se puede enumerar operadores | Usuario inexistente, contraseña equivocada y operador desactivado: misma respuesta y **misma latencia** (bcrypt de descarte, y el `is_active` se revisa DESPUÉS del hash). | `consola_separada_test.go` |
| A01 | Desactivar corta el acceso ya | El middleware relee al operador en cada request; no se espera a que caduque el token. | `consola_separada_test.go` |
| A05 | `CORS_ORIGIN` admite varios orígenes | Dos frentes (POS y consola) contra la misma API. Coincidencia **exacta** por origen, nunca por prefijo, y `*` sigue prohibido en producción **en cualquier entrada de la lista**. | `cors_test.go` |

Lo que la consola **no** puede hacer hoy, y es deliberado: **escribir**. No tiene un solo `insert`,
`update` ni `delete`. Las acciones de soporte (spec 018) exigen cambiar permisos a propósito.

## La medición de uso (spec 017) — lo que NO se guarda

| OWASP | Decisión | Por qué |
|---|---|---|
| A01 | **No se guarda un renglón por evento**, solo un conteo por día | La primera versión sí lo guardaba, con marca de microsegundos. Una auditoría adversarial mostró que eso deshace el anonimato entero: ese instante se cruza con `register_sessions.closed_by` o `orders.opened_by` y etiqueta al empleado — y como los eventos de un turno son un flujo contiguo, una sola coincidencia etiqueta todo lo que cayó en medio. **El identificador no era el rol: era el reloj** |
| A01 | El conteo **no tiene columna de persona** ni FK a `users` | No es que la aplicación no la escriba: no hay dónde |
| A01 | Un rol con **menos de dos** usuarios activos se guarda como «sin corte» | Decir «el gerente hizo 40 acciones» en una empresa con un gerente es decir su nombre. Se decide al ESCRIBIR: leyendo no se podría —la consola no tiene permiso sobre `users`— y escrito ya no se deshace |
| A03 | Lista blanca de pantallas y acciones en `domain` | Sin ella, cuántos valores distintos hay en la base lo decide el cliente |
| A04 | Tope de 50 eventos por lote y limitador por usuario | Un bucle en el front no puede costar una escritura por vuelta. El limitador lleva test propio: como el endpoint responde 204 pase lo que pase, roto es indistinguible de ausente |
| A04 | Ninguna columna libre que el cliente pueda llenar | Al quitar el grano fino se fue también el `jsonb` donde un cuerpo malicioso podía escribir. Las coordenadas nacieron después en su propia tabla y con su propia decisión: ver la spec 019, abajo |
| A04 | El tope se cuenta con el **valor de retorno del `INCR`**, no con un `GET` previo | Los dos pasos dejaban una carrera: 300 peticiones simultáneas leían el contador antes del primer incremento y pasaban todas |
| A04 | Y con Redis caído, la ingesta **falla cerrada** | Al revés que el login, donde fallar abierto existe para no dejar a nadie fuera. Aquí no hay a quién dejar fuera: perder mediciones cuesta cero, quedarse sin tope cuesta el disco del VPS |
| A03 | Un rol no puede reportar pantallas que su rol no abre | La lista blanca acota el conjunto de valores, no su coherencia: sin esto un mesero llena el mapa de aperturas de la pantalla de administración |
| A09 | Un `usage_descartado` en el log con el primer nombre desconocido | Es el único testigo de que una versión del front dejó de medir: el mapa mostraría menos, indistinguible de «se usó menos» |
| A01 | La consola lee **conteos, nunca hechos** | Su rol tiene `select` sobre `usage_daily` y `usage_touches_daily` y sobre nada más. La tabla de grano fino no existe: no es que no la alcance, es que no hay qué alcanzar |

## Dónde cae el dedo (spec 019) — la coordenada que no existe

La 017 dejó esta mitad aplazada y con la puerta abierta. Al cruzarla, la pregunta adversarial fue la
misma —¿qué se puede cruzar con qué?— y la respuesta cambió el diseño entero.

| OWASP | Decisión | Por qué |
|---|---|---|
| A01 | **La celda se calcula en la tableta**; al servidor nunca llega un `(x, y)` | Un punto con precisión de píxel que viaja EXISTE: en el cuerpo del request, en el log de acceso de un proxy y en la memoria del servidor, aunque después se redondee. «Se borra luego» es una intención; redondear en el origen es una garantía. Y el tipo del evento no tiene campo para un punto, así que la promesa no depende de que nadie lo llene |
| A01 | **Tampoco hay instante**, ni siquiera un `updated_at` | Esa fila se toca con cada lote: su hora de última escritura diría a qué hora estuvo activa esa zona, que es medio camino de vuelta al cruce con `register_sessions` |
| A01 | **Ninguna imagen y ningún texto de la pantalla** | La tentación es pintar las manchas encima de una captura, que se entiende mejor. Una captura del POS de un cliente lleva nombres, el contenido de sus pedidos e importes, y eso no puede salir del local. Lo vigila un guardia que **lee el código** de los archivos que arman el envío: `toDataURL`, canvas, `innerText`, `document.title` y la dirección de la página |
| A01 | La rejilla es **gruesa a propósito**: 12×7, zonas del tamaño de un botón | Más fina sería un mapa de puntos con otro nombre. Y la dirección importa: hacia una rejilla más gruesa se recalcula fusionando celdas, **hacia una más fina no se puede** — el toque fino no se guarda en ningún lado |
| A01 | Mismo corte de rol que la 017: menos de dos personas activas, «sin corte» | Y suprimir el rol no es tirar la medición: la fila existe sin él |
| A03 | Lista blanca de pantallas instrumentadas, **subconjunto** de la de la 017 | Una pantalla que no esté allá tendría todos sus toques descartados en silencio y su rejilla saldría vacía. Lo vigila un test: el modo de falla de esta familia no es fallar, es medir menos — y «menos» se lee como «nadie lo usa» |
| A03 | La misma lista corre **también en el cliente** | El escuchador vive en la raíz y ve toda la aplicación; sin filtrar allá, encolaría toques de pantallas no medidas todo el turno para que el servidor los tire |
| A03 | `check` de orientación en la columna | Sin él, una versión vieja de la tableta que mande `'landscape'` crea un **balde invisible**: la fila entra, pasa el rango de celda —0..83 vale en las dos formas— y la consola nunca la muestra. Los toques de esa zona desaparecen sin un solo error |
| A04 | Mil toques en la misma zona **suben un contador** | Es lo que hace que quepa, y por construcción: las filas las fija la rejilla (84 × 2 orientaciones × 5 cortes de rol × pantallas instrumentadas), no los dedos. Medido: 16.5 MB por trimestre y por pantalla, con el churn al tope del limitador |
| A04 | Mismo endpoint, mismo limitador y misma cola que la 017 | Un camino nuevo obligaría a volver a demostrar todo lo que hace que la medición no estorbe, a cambio de nada |
| A09 | Un `toques_descartados` en el log | El mismo testigo que en la 017, con su propio nombre |
| A01 | La consola pide **una orientación** y recibe esa | Sumarlas pintaría una rejilla que nadie tocó nunca, y el error sería invisible: se vería normal describiendo un lugar que no existe |
| — | Retención **más corta** que la del agregado: 92 días contra 396 | Una rejilla de hace un año describe un layout que ya no existe. Conservarla es conservar una referencia que miente |
| A01 | El corte de rol mira la plantilla **entera**, no solo ese rol | La regla obvia deja un agujero: todo lo suprimido cae en el mismo balde `role is null`, así que cuando **un solo rol** queda bajo el umbral, ese balde ES esa persona —con 1 admin, 2 gerentes, 3 cajeros y 2 meseros, `null` es el dueño— y la consola lo pinta como «sin corte». Cuando el balde no alcanza a tapar a nadie, la medición **no se escribe**. Aplica también a la 017, que tenía el mismo agujero |

### Hasta dónde llega el anonimato de la medición, dicho por escrito

Lo de arriba impide guardar a una persona. **No impide cruzar lo guardado con otra cosa**, y eso hay
que decirlo en vez de dejar que se lea como una garantía que no es:

- El periodo de la consulta es libre dentro de la retención, así que se puede pedir **un solo día**.
- En un local donde ese día trabajó una sola persona de ese rol —turnos que no se solapan, que es lo
  normal en un negocio chico— la rejilla de ese día es la de esa persona, y `orders.opened_by` o
  `register_sessions.closed_by` dicen quién fue.
- **Un mínimo de ventana no lo arregla**: restar «7 días hasta hoy» menos «7 días hasta ayer»
  recupera el día. Lo mismo deshace cualquier agregación temporal que se ponga encima.

Quien puede hacer ese cruce es quien ya tiene acceso a la base del cliente, no un operador de la
consola —su rol solo alcanza `companies`, `platform_operators` y los dos agregados de medición—. Se
documenta, como se documentó el alcance del arqueo ciego, porque la promesa correcta es «no se
guarda quién», no «es imposible saber quién».

## Checklist de lanzamiento en el VPS (operador)

**Secretos y config (antes del primer arranque):**
- [ ] `JWT_SECRET`: `openssl rand -base64 48` (≥32; la API rechaza débiles/placeholder).
- [ ] `POSTGRES_PASSWORD`: `openssl rand -hex 24`.
- [ ] `ADMIN_PASSWORD` y `ADMIN_PIN` reales (no `cambia-esto`, no `1234` — la API los rechaza).
- [ ] **`CORS_ORIGIN=https://tu-dominio,https://staff.tu-dominio`** (exactos, separados por coma).
  Con `*` en producción —aunque sea una entrada más de la lista— la API NO arranca.
- [ ] `PLATFORM_JWT_SECRET`: `openssl rand -base64 48`, **distinto de `JWT_SECRET`** (si son iguales
  la API no arranca).
- [ ] `PLATFORM_DB_PASSWORD`: `openssl rand -hex 24`. Sin él en producción la consola no tiene su rol
  y la API no arranca.
- [ ] `scripts/check-env.sh` pasa. `deploy/.env` no versionado (ya lo está) y `chmod 600`.

**Hardening del host:**
- [ ] `ufw`: permitir 22/80/443, denegar el resto. Postgres/Redis **no** publican puertos (ya es así en compose — mantenerlo).
- [ ] SSH: solo con llave; deshabilitar root y password auth.
- [ ] Rotar `ADMIN_PASSWORD`/`ADMIN_PIN` tras el primer login (`make reset-admin`).

**Durabilidad (día uno):**
- [ ] Backup nocturno: `pg_dump | gzip`, retención 7–14 días, **copiado fuera del VPS**
  (un backup en el mismo disco no sobrevive a un fallo de disco). Redis no necesita backup (cache).

**Smoke post-deploy:**
- [ ] `/auth/login` responde 429 tras repetidos fallos (B1 vivo).
- [ ] Imprimir un ticket de una orden con nombre `<b>x</b>` → sale como texto, no markup (B2 vivo).
- [ ] Confirmar headers de seguridad: `curl -I https://tu-dominio` muestra HSTS + `X-Frame-Options`.

## Cómo re-verificar

```bash
cd server && go test ./... && govulncheck ./...    # backend + CVEs
cd web    && bun run typecheck && bunx vitest run   # frontend + XSS test
```
