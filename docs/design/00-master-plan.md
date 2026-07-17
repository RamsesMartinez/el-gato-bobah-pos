# El Gato Bobah POS — Sistema propio (plan maestro)

## Context

El repo era un frontend React que reemplazaba la UI de FUDO (SaaS restaurantero). Decisión del dueño: **sistema 100% propio**. Se analizaron los exports reales de FUDO en `references/` (752 productos, 318 ingredientes, 620 líneas de receta, 53+ grupos de modificadores con 485 opciones, stock, ventas ~2,600 tickets/11 meses, gastos) y se hizo ingeniería inversa del modelo de dominio, incluyendo sus 10 debilidades a corregir (opciones de modificador como pseudo-productos, joins por nombre, sin sistema de unidades, stock negativo caótico, combos inútiles, strings libres para proveedores/medios de pago, etc.).

**Decisiones ya tomadas por el usuario:**
- Backend **Go** dockerizado, **PostgreSQL + Redis**, CQRS-light, deploy en **VPS**.
- Frontend: React 18 + Chakra v2 se queda; **migrar CRA → Vite**; bun + Node 24 (ya hecho).
- **Monorepo** en este repo: `web/` + `server/` + `deploy/`.
- Alcance MVP completo: POS + catálogo/modificadores + cobro/cortes + ingredientes/recetas/stock + gastos/reportes + **importador FUDO** + **impresión térmica**.

**Planes detallados** (diseños completos con DDL, API y UX — leerlos al ejecutar cada fase):
- `/private/tmp/claude-501/-Users-ramses-Documents-git-ramthedev-el-gato-bobah-pos/1b12f2b6-15a6-4ffc-897b-96ce20013959/scratchpad/plan-domain.md` (schema SQL completo)
- `.../scratchpad/plan-backend.md` (arquitectura Go completa)
- `.../scratchpad/plan-frontend.md` (UX POS tablet completo)
- Reportes de exploración: `.../scratchpad/explore-reports.json`

⚠️ Al iniciar la ejecución, copiar estos 4 archivos a `docs/design/` en el repo para que sobrevivan a la sesión.

## Arquitectura (reconciliada entre los 3 diseños)

```
el-gato-bobah-pos/
├── web/          # React 18 + TS 5 + Chakra v2 + Vite + bun
├── server/       # Go: cmd/{api,fudo-import,print-agent}, internal/{domain,app,store,cache,realtime,httpapi,auth}, migrations/, queries/
├── deploy/       # docker-compose.yml, docker-compose.dev.yml, Caddyfile, .env.example
├── docs/design/  # los planes detallados
├── references/   # exports FUDO (fuente del importador)
└── Makefile      # dev, build, migrate, sqlc, seed, test, deploy
```

**Stack backend:** chi v5 · pgx/v5 + sqlc · goose (migraciones embebidas, self-migrate al boot) · validator v10 · caarlos0/env · slog · air (dev) · shopspring/decimal solo en costeo/stock. Dockerfile multi-stage → distroless. Caddy sirve `web/dist` + reverse proxy `/api` (flush_interval -1 para SSE).

**Decisiones reconciliadas:**
- **Dinero:** Postgres `numeric(10,2)` (costos unitarios `numeric(12,6)`); la API expone **centavos enteros** (`totalCents`) — sin floats en JS ni en Go (int64/decimal).
- **IDs:** `bigint identity` PKs + `client_uuid` en orders para idempotencia de tablets (header `Idempotency-Key` en POST /orders y /payments).
- **Estados de orden: 4** (vs 7 de FUDO): `abierta → lista → entregada` + `cancelada` (razón + usuario obligatorios). **Pagado es un campo derivado** (Σ payments ≥ total), no un estado. Mostrador rápido puede saltar abierta→entregada.
- **Redis (roles mínimos y honestos):** (1) `pos:menu` — documento JSON denormalizado del catálogo completo (~750 productos, un solo GET para el POS; invalidación por DEL + singleflight; versión embebida + evento SSE `menu.updated`), (2) `avail:*` — disponibilidad derivada por producto, invalidada por movimiento de stock vía índice reverso ingrediente→productos. El board de órdenes activas y reportes salen de Postgres directo (≤30 órdenes abiertas; índice parcial, <2ms).
- **Real-time:** SSE con broker in-process (`/api/v1/events`, ring buffer + Last-Event-ID); fallback polling 10s en el cliente. Interface `Broker` como costura para Redis pub/sub si algún día hay 2 réplicas.
- **Migraciones:** goose (no golang-migrate).

## Schema (resumen — DDL completo en plan-domain.md)

Núcleo que corrige las 10 debilidades FUDO:
- `units` (kind masa/volumen/pieza + `to_base`) + `ingredient_purchase_formats` — mata los hacks de conversión ("Mayonesa Cda").
- `suppliers`, `payment_methods` (con `affects_cash_drawer`), `delivery_platforms`, `expense_categories` — entidades reales, no strings.
- `ingredients` (merma %, `is_prep` + receta + yield para subingredientes preparados, `is_packaging`), `recipes`/`recipe_items` — **una sola entidad receta** compartida por productos, opciones de modificador y preps.
- `categories` (2 niveles enforced, `sort_key numeric` fraccional, color estable), `channels` + visibilidad tri-estado heredable.
- `products` (simple|combo, cost_source manual|receta, `margin_amount` generado, `allow_oversell`, favoritos) + `combo_slots`/`combo_slot_products` — combos de primera clase.
- `modifier_groups`/`modifier_options` (**con `recipe_id` propio** → los toppings siguen descontando stock, sin pseudo-productos) + `product_modifier_groups` (grupos compartidos, min/max/título por attachment).
- `stock_movements` — **ledger firmado como fuente de verdad** (venta/compra/ajuste/merma/produccion/cancelacion) + `stock_levels` cache por trigger. Negativos permitidos (verdad contable); Disponibilidad = read model clamp ≥0 con recursión por recetas; gate `allow_oversell` en la tx de venta.
- `orders`/`order_lines`/`order_line_modifiers`/`order_payments` — **snapshots** de nombre/precio/costo en cada línea (la utilidad histórica sobrevive a cambios de costo); folio diario (`order_counters`); cancelación por línea con razón+usuario; pagos divididos = múltiples filas.
- `register_sessions` + `register_session_totals` (declarado vs esperado por método) + `register_cash_movements` (entradas/salidas, propinas) — cortes de caja.
- `expenses` (taxonomía 2 niveles, payee = supplier, liga a movimientos `compra`).
- `users` (roles admin/cajero/mesero, `pin_hash` para quick-switch en POS, `password_hash` admin). Sin usuario compartido "Mostrador" (anti-patrón FUDO).
- `fudo_import_map` — trazabilidad del importador.

**Motor de costeo** (Go, `internal/costing`, no PL/pgSQL): roll-up recursivo ingrediente→prep→receta→producto/opción/combo con merma y empaque, guard de ciclos, memoización; recomputo en cascada síncrono (<50ms para ~750 productos) al cambiar costos/recetas + job nocturno idempotente. Reportes de utilidad leen SOLO snapshots.

## API (resumen — detalle en plan-backend.md)

`/api/v1`, errores `{error:{code, message(es), details}}`, keyset pagination, RFC3339 UTC.
- Auth: login (email+password, JWT 15min + refresh httpOnly rotado en Postgres), `pin-switch` (atribución real por operador), roles middleware.
- `GET /pos/menu` (ETag/304, Redis) · catálogo CRUD (productos, categorías, grupos, recetas, ingredientes, proveedores, medios de pago) · `GET /products/{id}/costing` (desglose).
- Orders: POST (líneas + modificadores + notas; **precios siempre server-side**), status transitions, cancelación por línea/orden, payments (split, cambio calculado).
- Cash sessions (open/current/close con declarado vs esperado) · stock (levels, movements ledger) · expenses · users · reports (sales-summary, margins) · `/healthz`, `/readyz`, `/events` (SSE).

## Frontend POS (resumen — detalle en plan-frontend.md)

- **Vite 6 + TS 5 + vitest** (env `VITE_API_URL`; muere el token FUDO en el cliente). Borrar ~20 archivos muertos verificados (2 routers, 2 ProductCards, doble sistema de temas, deps @mui/styled-components).
- **TanStack Query** (menú completo en 1 payload cacheado → cero fetches por tap, mata el N+1 de breadcrumbs) + **Zustand persist** para el ticket (un refresh jamás pierde el pedido) + `types/{wire,domain,mappers}.ts` como capa de aislamiento.
- **Pantalla POS, 2 modos por ancho de contenedor** (ResizeObserver, no viewport):
  - ≥900px: catálogo + ticket lateral `clamp(300px, 32%, 380px)` (mata el panel fijo de 500px).
  - <900px: catálogo full-width + `TicketBottomBar` 64px siempre visible + ticket como Drawer bottom-sheet 92%.
- Grid `repeat(auto-fill, minmax(118px, 1fr))` (container-sized); rail de chips de categorías (2 niveles, colores estables por hash de id); búsqueda client-side sin diacríticos; fila ★Favoritos como landing.
- **ModifierSheet** (bottom sheet): grupos single-select como radio-chips con default preseleccionado, multi-select con contador min/max, deltas visibles (+$20), steppers por opción, nota de cocina, total vivo. Patrón "+1 igual / Personalizar" para repetir bebidas. Touch ≥44px, `:active` no `:hover`, `touch-action: manipulation`.
- **CheckoutSheet** 2 pasos: tipo (Mostrador/Llevar/Domicilio+plataforma) + nombre cliente → pago (tiles de métodos del backend; efectivo con quick-tender y **CAMBIO gigante**). "Enviar a cocina" = orden sin pagar con badge POR COBRAR.
- **Board de pedidos** `/pedidos`: kanban 3 columnas (landscape) / segmented (portrait), OrderCard con timer de antigüedad y UN botón primario de avance, cancelación con razón obligatoria, SSE + fallback polling, filtros por tipo en query (arregla los tabs rotos actuales). Detalle `/pedidos/:id`.
- **AppShell**: rail de iconos 72px (landscape) / hamburger (portrait), gateo por rol, PIN pad de login.

## Importador FUDO (`cmd/fudo-import`)

Staged e idempotente (detalle §12 de plan-domain.md): seeds → suppliers dedupe → ingredientes (+triage de los 25 subingredientes: preps reales vs hacks de conversión→purchase formats) → categorías (excluyendo "Otro"/modificadores) → productos (regla: migra como producto si NO es opción de modificador, o si es vendible solo) → recetas → grupos/opciones de modificadores (el precio del sheet gana; pseudo-productos se colapsan a opciones) → combos (manual, 4 casos) → stock inicial como movimientos `ajuste` → **validación: recomputar costos y diff vs columna Costo de FUDO (331 productos), reportar deltas > $0.50**.

## Impresión térmica (requerida en MVP)

El VPS no alcanza la impresora del local. Dos entregas:
1. **MVP inicial: impresión del navegador** — CSS receipt 80mm + `window.print()` desde la tablet (driver Android/iOS de la impresora). Cero piezas nuevas.
2. **MVP final: `cmd/print-agent`** — binario Go pequeño corriendo en cualquier dispositivo del local; se suscribe por SSE a print-jobs del VPS y manda ESC/POS a la impresora por TCP 9100. Tabla `print_jobs` + endpoint. (Es la solución robusta dada la decisión VPS.)

## Fases de ejecución

| Fase | Entregable | Verificación |
|---|---|---|
| **F0** | Reestructura monorepo (`web/`, `server/`, `deploy/`, `docs/design/`) + migración Vite + purga de código muerto + Makefile nuevo | `make dev` levanta Vite; `bun run build` pasa `tsc -b`; app renderiza |
| **F1** | Skeleton Go: chi+slog+config+healthz, goose+sqlc, compose dev (pg+redis), auth completo (login/refresh/PIN/roles) | `make test`; login+pin-switch vía curl |
| **F2** | Migraciones de schema completas + CRUD catálogo + motor de costeo + `GET /pos/menu` con Redis + **importador FUDO** | Import corre contra `references/`; diff de costos vs FUDO < $0.50; menú servido <10ms cache-hit |
| **F3** | Frontend data layer + pantalla POS + ModifierSheet + Checkout → `POST /orders` + payments | Crear pedido real con modificadores y cobro efectivo con cambio, en viewport 800×1280 y 1280×800 |
| **F4** | Board de pedidos + SSE + detalle + cancelaciones + **depleción de stock en venta** + disponibilidad | Venta descuenta ingredientes vía receta en el ledger; board se actualiza en vivo; cancelar repone stock |
| **F5** | Cortes de caja + gastos + UI de almacén (movimientos/compras/ajustes) + reportes (ventas, márgenes) + historial | Corte: esperado vs declarado cuadra con pagos de la sesión; `go test ./...` (costeo, órdenes, stock, corte) |
| **F6** | UIs admin (productos/recetas/ingredientes/modificadores/empleados) + deploy VPS (Caddy TLS, backups pg_dump cron) + impresión (browser → agent) | Deploy real; ticket impreso; smoke E2E en tablet física |

**Tests que sí valen en el MVP** (backend): `costing_test` (roll-up con merma/empaque/preps/redondeo, table-driven con datos FUDO reales), `order_test` (matriz completa de transiciones, min/max de modificadores, pagos divididos, cambio), `stock_test` (explosión de recetas, disponibilidad clamped, oversell, reversa en cancelación), `cashsession_test` (aritmética del corte) + 1 integration flow (login→orden→pago→corte) + replay de Idempotency-Key. Frontend: vitest smoke + tests del ticketStore.

## Riesgos señalados

- Chakra v2 Drawer sin drag-to-dismiss nativo (aceptable: tap-close; framer-motion ya es dep).
- Probar temprano en la tablet Android real (`dvh`, reconexión EventSource).
- VPS = sin internet en el local no hay POS (decisión consciente del usuario; el print-agent y un plan híbrido quedan como evolución).
- Los 3 planes detallados en scratchpad son de esta sesión — copiarlos a `docs/design/` es el primer paso de F0.
