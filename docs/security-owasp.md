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
| A07 | **La sesión dura un turno, no 30 días.** El vencimiento del refresh sale de `business_settings.session_hours` (default 8 h). Antes era una constante de 30 días, así que una tableta olvidada era una credencial abierta durante un mes. Sin fila de ajustes cae a los 30 días — el modo de fallo deja entrar, no deja fuera. | `app/auth.go`, `migrations/0050_ajustes_identificacion.sql` | `integration/sesion_caduca_test.go` |
| A07 | **El cambio de operador no repone el reloj de la sesión, y revoca la anterior.** `PinSwitch` emitía un plazo completo nuevo en cada relevo: una tableta usada cada veinte minutos no caducaba nunca, y como nada revocaba al emitir, cada relevo dejaba una credencial suelta (en producción se encontró un usuario con 4 vivas). | `app/auth.go`, `queries/users.sql` | `integration/pin_switch_conserva_reloj_test.go` |
| A07 | **Bloqueo de pantalla por inactividad**, configurable por negocio. Va ENCIMA de la aplicación y no la desmonta: si se perdiera lo capturado, el operador aprendería a impedir el bloqueo y la protección se caería sola. | `web/features/auth/`, `migrations/0050_…` | `web/features/auth/noSePierde.test.tsx`, `inactividad.test.ts` |
| A07 | **Desbloqueo por PIN sin enumerar usuarios**: id inexistente y PIN incorrecto dan la misma respuesta y la misma latencia (bcrypt de descarte); el evento de seguridad lleva a quién se intentó y **nunca el PIN**. Sin `userId` y sin modo de solo-PIN, se rechaza en vez de caer al modo permisivo. | `app/auth.go`, `httpapi/handlers.go` | `httpapi/pin_seguridad_test.go`, `integration/pin_seguridad_test.go` |
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

## Testing de integración (Postgres efímero)

Suite `internal/integration` (build tag `integration`) contra un Postgres real —cubre lo que
los tests unitarios no alcanzan por el `*db.Queries` concreto: reuso de refresh→revoca
familia, rotación, y el flujo de reembolso. Job de CI `integration` con servicio
postgres:16-alpine (digest-pin). `go test ./...` normal no se ve afectado (skip sin
`TEST_DATABASE_URL`).

## Checklist de lanzamiento en el VPS (operador)

**Secretos y config (antes del primer arranque):**
- [ ] `JWT_SECRET`: `openssl rand -base64 48` (≥32; la API rechaza débiles/placeholder).
- [ ] `POSTGRES_PASSWORD`: `openssl rand -hex 24`.
- [ ] `ADMIN_PASSWORD` y `ADMIN_PIN` reales (no `cambia-esto`, no `1234` — la API los rechaza).
- [ ] **`CORS_ORIGIN=https://tu-dominio`** (exacto). Con `*` en producción la API NO arranca.
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
