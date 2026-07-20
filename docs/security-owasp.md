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
| A07 | `IsActive` re-verificado en `Refresh` (empleado dado de baja pierde acceso al instante, antes seguía 30 días). | `app/auth.go` |
| A06 | Bump toolchain Go → 1.25.12 (cierra GO-2026-5856 en crypto/tls). | `go.mod` |
| A05 | Cap de logs de contenedor (json-file 10m×3) → no llena el disco del VPS. | `docker-compose.yml` |

## Pendiente post-MVP (no bloquea, priorizado)

- **A05/A10 — CSP completo** (`script-src 'self'`, `style-src 'self' 'unsafe-inline'`
  para Chakra/Emotion, `connect-src 'self'` para SSE) tras probar contra el build.
- **A02/A05 — access token en `localStorage`**: moverlo a memoria (re-mint por refresh
  cookie al recargar). Defensa en profundidad; B2 + CSP son la mitigación real.
- **A07 — timing oracle de enumeración de usuarios** en login (bcrypt dummy en la rama
  "no encontrado").
- **A07 — reuse-detection de refresh** (revocar la familia si se reusa un token revocado).
- **A04 — validación de rangos** (montos/cantidades máximos) para evitar overflow → 500.
- **A09 — logging**: bajar el volcado de body/PII a `LOG_LEVEL=debug`; eventos de
  seguridad distintos (login fallido, 403) para detección.
- **A06/A08 — Dependabot/Renovate** + pin por digest de imágenes base; SHA-pin de
  GitHub Actions; `bun audit` bloqueante en CI (quitar `|| true`).
- **UX — gating client-side** de `/caja`,`/gastos`,`/almacen` por rol (el backend ya lo
  aplica; hoy un mesero llega a un 403 en vez de no ver la opción).
- **Refund/void de orden entregada**: hoy `entregada` es terminal (no se puede cancelar
  una orden ya entregada). Si el negocio lo requiere, añadir flujo de devolución dedicado.

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
