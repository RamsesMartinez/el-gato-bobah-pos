# El Gato Bobah POS — Go Backend Service Architecture

MVP-pragmatic design for a single-location cat café POS. Optimized for: one developer, one VPS, tablets on counter Wi-Fi, order-taking speed as priority #1. Everything below is deliberately boring where boring wins, and only clever where FUDO's weaknesses (fudo report §8) demand it.

---

## 1. Repo layout

### 1.1 Monorepo shape: `web/` + `server/` (flat, no `apps/`)

The repo is currently frontend-at-root CRA. Since the CRA→Vite migration forces touching every config file anyway, move the frontend into `web/` in the same commit and create `server/`. Skip the `apps/` + `packages/` nesting: there are exactly two apps, no shared JS packages (the API contract is Go-side truth, consumed as generated TS types), and no need for turborepo/nx. Two folders, two toolchains, zero orchestration overhead — best solo-dev ergonomics.

```
el-gato-bobah-pos/
├── web/                        # React 18 + TS + Chakra v2, Vite, bun
│   ├── src/
│   ├── index.html
│   ├── vite.config.ts
│   └── package.json            # bun-enforced (keep preinstall guard)
├── server/                     # Go module: github.com/ramthedev/el-gato-bobah-pos/server
│   ├── cmd/
│   ├── internal/
│   ├── migrations/
│   ├── queries/
│   ├── sqlc.yaml
│   ├── .air.toml
│   └── go.mod
├── deploy/
│   ├── docker-compose.yml
│   ├── docker-compose.dev.yml  # overrides: bind mounts, air, exposed ports
│   ├── Caddyfile
│   └── .env.example
├── references/                 # FUDO exports (keep, gitignored or LFS)
├── Makefile                    # single entrypoint for both apps
└── README.md
```

Root `package.json` goes away (or becomes a stub pointing at `web/`). CI/tooling keys off the two directories.

### 1.2 Go project layout

```
server/
├── cmd/
│   ├── api/main.go             # wire config → pool → stores → services → router; graceful shutdown
│   └── seed/main.go            # import FUDO XLS-derived catalog (one-shot CSV/JSON loader)
├── internal/
│   ├── config/config.go        # env parsing, one struct
│   ├── domain/                 # PURE: no db, no http imports. The testable core.
│   │   ├── money.go            # Cents type (int64), percent math, rounding rules
│   │   ├── order.go            # Order, OrderLine, ModifierSelection, Status + state machine
│   │   ├── catalog.go          # Product, Category, ModifierGroup/Option, Recipe types
│   │   ├── costing.go          # recipe cost roll-up: ingredients + merma% + packaging + nested preps
│   │   ├── stock.go            # movement types, availability derivation, oversell policy
│   │   ├── cashsession.go      # corte de caja: open/close, expected vs counted
│   │   └── errors.go           # sentinel domain errors (ErrInvalidTransition, ErrInsufficientStock…)
│   ├── app/                    # command handlers + query services (CQRS-light seam)
│   │   ├── orders.go           # CreateOrder, AddLine, UpdateStatus, CancelLine, PayOrder…
│   │   ├── catalog.go          # product/modifier/recipe CRUD commands + cost recompute
│   │   ├── stock.go            # RecordMovement, adjustments, purchase receipts
│   │   ├── cash.go             # OpenSession, RecordExpense, CloseSession
│   │   ├── auth.go             # Login, PinSwitch, Refresh
│   │   └── queries/            # read-side: posmenu.go, activeorders.go, reports.go
│   ├── store/                  # persistence
│   │   ├── db/                 # sqlc-GENERATED code (do not edit)
│   │   ├── store.go            # pgxpool wrapper, WithTx helper
│   │   └── migrate.go          # goose embedded migrations, run at startup
│   ├── cache/
│   │   └── menu.go             # Redis menu snapshot cache (get/set/invalidate)
│   ├── realtime/
│   │   └── broker.go           # in-process SSE broker (interface allows Redis pub/sub later)
│   ├── httpapi/
│   │   ├── router.go           # chi router assembly, versioned mount
│   │   ├── middleware.go       # auth, role, request-id, logging, recover
│   │   ├── respond.go          # JSON writer, error envelope mapping
│   │   ├── handlers_orders.go  # thin: decode → validate → app call → respond
│   │   ├── handlers_catalog.go
│   │   ├── handlers_stock.go
│   │   ├── handlers_cash.go
│   │   ├── handlers_auth.go
│   │   └── handlers_sse.go
│   └── auth/
│       ├── jwt.go              # sign/verify access tokens
│       └── password.go         # bcrypt + PIN hashing
├── migrations/                 # goose SQL files (schema agent owns content)
│   └── 00001_init.sql
├── queries/                    # sqlc input .sql files, one per aggregate
│   ├── orders.sql
│   ├── catalog.sql
│   ├── stock.sql
│   ├── cash.sql
│   └── users.sql
└── Dockerfile
```

Dependency rule (enforced by import direction only — no interfaces-for-the-sake-of-it): `httpapi → app → domain` and `app → store/cache/realtime`. `domain` imports nothing internal. This is what makes money math and state machines unit-testable without a database (§8 testing).

---

## 2. Stack choices

| Concern | Pick | Why (and why not the alternatives) |
|---|---|---|
| Router | **chi v5** | Pure `net/http`, stdlib `http.Handler` everywhere — SSE, middleware, and testing (`httptest`) all work with zero adapters. Echo brings its own context type; Fiber is fasthttp (breaks `net/http` ecosystem, SSE is awkward, and its raw-throughput edge is irrelevant at one-café QPS). chi is the boring choice that never fights you. |
| DB access | **pgx/v5 + sqlc** | sqlc gives compile-time-checked, typed queries from real SQL — you write the exact query, you get the exact plan, no N+1 surprises, no reflection. GORM's hooks/magic are a liability for a money system where you want to *see* every statement. pgx v5 auto-prepares/caches statements (see §9). CQRS-light *is* "hand-written SQL per read model" — sqlc is purpose-built for that. |
| Migrations | **goose** | Plain SQL files, supports Go migrations if ever needed, and — decisive for a solo-dev VPS deploy — embeds via `embed.FS` so the API binary migrates itself on boot (`store/migrate.go`). golang-migrate is fine but clunkier CLI; atlas is declarative-diff magic that's overkill and adds a SaaS-flavored tool to learn. Schema agent writes goose-format SQL. |
| Validation | **go-playground/validator v10** | Struct tags on request DTOs in `httpapi`, one thin wrapper that converts field errors into the API error envelope. Domain invariants (state transitions, money ≥ 0) live as code in `domain`, not tags. |
| Config | **caarlos0/env v11** | One annotated struct, env-only (12-factor, Compose-friendly). No viper — no config files, no watch, no YAML for an app with ~12 knobs. |
| Logging | **stdlib `slog`** JSON handler | Request-scoped logger with `request_id`, `user_id`, `route` via middleware. No zap/zerolog dependency; slog is fast enough by orders of magnitude here. |
| Hot reload | **air** | `.air.toml` watching `server/`, rebuilds in ~1s. Runs inside the dev compose override with a bind mount. |
| Money | **int64 centavos** (`domain.Cents`) | MXN, no floats ever. Percent ops (merma, discounts) computed in int math with explicit half-up rounding at defined points. DB column BIGINT. |
| Quantities | `NUMERIC` in PG ↔ **shopspring/decimal** in recipe/stock code | Recipe quantities like 0.398 kg and merma % must not float-drift; decimal only in the costing/stock ledger paths, ints everywhere else. |
| IDs | **UUIDv7** (`google/uuid`) app-generated | Sortable, no round-trip for ID, fixes FUDO's name-based-join disease (fudo §8.2) at the API layer too — every entity referenced by ID, names freely renameable. |
| JWT | golang-jwt/jwt v5 | Boring, maintained. |

Fixing FUDO weaknesses is mostly schema-agent territory, but the service layer commits to: modifier options as a dedicated entity (not pseudo-products — fudo §8.1), order→line→modifier-selection→payment→state-transition all first-class in the API (§8.10), payment methods and providers as ID'd entities (§8.7), margin returned as both $ and % (§8.5), fractional `position` ranking (`NUMERIC` sort key, insert-between = midpoint — §8.9).

---

## 3. CQRS-light, concretely

Not event sourcing. One Postgres, normalized write model, plus (a) purpose-built read queries and (b) one Redis-cached denormalized document. That's the whole pattern.

### 3.1 Write side — command handlers

Every mutation is a method on an `app` service that runs inside a single pgx transaction and returns the updated aggregate. Sketch:

```go
// internal/app/orders.go
type OrdersService struct {
    store  *store.Store
    menu   *cache.MenuCache      // for price/modifier validation snapshot
    broker realtime.Broker
    clock  func() time.Time
}

func (s *OrdersService) CreateOrder(ctx context.Context, cmd CreateOrderCmd) (*domain.Order, error) {
    var ord *domain.Order
    err := s.store.WithTx(ctx, func(q *db.Queries) error {
        // 1. Load priced catalog rows for the requested product/modifier IDs (server-authoritative prices).
        // 2. domain.BuildOrder(cmd, catalog) — computes line totals, validates modifier
        //    group min/max, order total. Pure function, unit-tested.
        // 3. Insert order + lines + modifier selections (sqlc batch).
        // 4. Insert order_events row (status=OPEN, actor, ts)  — the audit trail FUDO lacks.
        // 5. Stock: insert stock_movements rows (type=SALE_RESERVE or direct SALE per policy).
        return nil // (elided)
    })
    if err != nil { return nil, err }
    s.broker.Publish(realtime.Event{Type: "order.created", OrderID: ord.ID}) // AFTER commit
    return ord, nil
}
```

Rules:
- **Prices are always server-side.** Client sends product/modifier IDs + quantities; server prices from the catalog. A stale tablet can never charge a stale price.
- **State transitions go through `domain.Order.TransitionTo(status, actor)`** which enforces the machine (see §4) and appends an `order_events` row — first-class transitions per fudo §8.10.
- **Stock is a ledger, never an UPDATE of a quantity column.** `stock_movements(item_type, item_id, qty_delta, reason, order_id?, user_id, cost_snapshot)`; on-hand = SUM, availability = derived view (coordinate with schema agent on a `stock_levels` materialized rollup if SUM gets slow — it won't at this volume). Oversell allowed per-product flag (FUDO behavior kept, but visible and reportable instead of silently negative).
- **Cost roll-up is a command side-effect:** any recipe/ingredient-cost mutation recomputes affected products' costs via `domain/costing.go` (ingredient cost × qty × (1 + merma%) summed, packaging lines included, nested preps resolved 1 level with cycle guard) and persists snapshot `cost` on the product. Reads never compute costs.

### 3.2 Read side

| Read | Backing | Why |
|---|---|---|
| **POS menu** (categories → products → modifier groups → options, prices, availability flags, sort order) | **One denormalized JSON document**, built by SQL in `app/queries/posmenu.go`, **cached in Redis** | This is THE hot read: every tablet loads it on boot and after every catalog change. Building it is a ~6-join query (~10–30ms); serving it must be instant. Frontend gets its entire catalog in one request — kills the current per-tap category API-walk latency (ux report §1). |
| **Active orders board** | **Plain indexed Postgres query** (`WHERE status IN (...open...)` partial index), no cache | Honest assessment: one location has maybe 5–30 open orders. Postgres answers in <2ms. Caching this in Redis buys nothing and creates invalidation bugs. Don't. |
| Sales history, cortes, reports | Plain SQL (keyset-paginated), plus a couple of SQL views for daily rollups | Volume is tiny (~2,600 sales in 11 months per FUDO export). Materialized views are unnecessary for MVP; revisit if a report exceeds ~200ms. |
| Stock levels / low-stock | SQL view over ledger SUM + min-stock threshold | Same reasoning. |

### 3.3 Redis — minimal, honest roles

**MVP roles: (1) menu snapshot cache, (2) nothing else.** Real-time fan-out is in-process (§6). Refresh tokens live in Postgres. Sessions are JWTs. If Redis died, the system would work with menu reads ~25ms slower.

Keep it anyway because: it's already decided infra, it's one compose service, and it gives a ready lane for post-MVP (rate limiting, pub/sub when a second API replica or a kitchen-display service appears).

Cache design:

```
Key:            pos:menu                (single location → single key)
Value:          gzipped JSON, the full POS menu document
TTL:            24h (safety net only; explicit invalidation is the mechanism)
Invalidation:   DEL pos:menu after commit of ANY catalog-mutating command
                (product/category/modifier/recipe/price/availability change).
                Next reader rebuilds and SETs (singleflight guard in-process
                to avoid a stampede — golang.org/x/sync/singleflight).
Versioning:     the document embeds "menuVersion": <unix ms of build>.
                SSE broadcasts {type:"menu.updated", version} so tablets refetch.
```

That's the entire Redis surface for MVP. Explicitly deferred: caching active orders, caching sessions, Redis Streams, distributed locks.

---

## 4. API surface (REST, `/api/v1`)

### Conventions
- **Versioning:** URL prefix `/api/v1`. One version until there's a reason not to be.
- **Envelope:** success returns the resource bare (no wrapper); errors return:
  ```json
  { "error": { "code": "ORDER_INVALID_TRANSITION", "message": "No se puede pasar de CLOSED a IN_PROGRESS", "details": [{"field": "status", "reason": "..."}] } }
  ```
  Stable machine `code` slugs; `message` in Spanish for direct UI display. HTTP status carries the class (400 validation, 401/403 auth, 404, 409 conflict/transition/version, 422 domain rule, 500).
- **Pagination:** keyset for time-ordered lists — `?limit=50&cursor=<opaque(created_at,id)>` returning `{ "items": [...], "nextCursor": "..." }`. Offset paging nowhere (histories grow forever).
- **Timestamps** RFC3339 UTC; money fields are integer centavos with `Cents` suffix (`totalCents`), display formatting is frontend's job.
- **Idempotency:** `POST /orders` and `POST /orders/{id}/payments` accept an `Idempotency-Key` header (unique index on it) — tablets on café Wi-Fi retry; double-charging must be structurally impossible.

### Endpoints by module

**Auth**
```
POST   /auth/login              {email,password} → {accessToken, user}; sets httpOnly refresh cookie
POST   /auth/refresh            cookie → new access token (rotates refresh)
POST   /auth/pin-switch         {userId, pin} → {accessToken, user}   (requires valid device session)
POST   /auth/logout
GET    /auth/me
```

**Catalog (admin CRUD; role: manager+)**
```
GET/POST        /categories                        PATCH/DELETE /categories/{id}
GET/POST        /products                          GET/PATCH/DELETE /products/{id}
GET/POST        /modifier-groups                   PATCH/DELETE /modifier-groups/{id}
PUT             /products/{id}/modifier-groups     attach groups w/ min/max/título (ordered)
PUT             /products/{id}/recipe              replace recipe lines (BOM)
GET/POST/PATCH  /ingredients, /ingredients/{id}    incl. merma%, unit, cost
PUT             /ingredients/{id}/recipe           sub-ingredients (preps)
GET/POST/PATCH  /providers                         first-class entity (fixes fudo §8.7)
GET/POST/PATCH  /payment-methods                   first-class entity, orderable, activatable
GET             /products/{id}/costing             cost breakdown: lines, merma, packaging, margin $ and %
```

**POS reads (role: any authenticated)**
```
GET    /pos/menu                the cached denormalized document (§3.2); ETag = menuVersion
```

**Orders lifecycle**
```
POST   /orders                  {type: MOSTRADOR|MESA|DELIVERY, tableRef?, customerName?, platform?, lines:[{productId, qty, notes?, modifiers:[{optionId, qty}]}]}
GET    /orders?status=open|…&type=…&cursor=…
GET    /orders/{id}             full detail: lines, selections, events, payments
POST   /orders/{id}/lines       add line to open order
PATCH  /orders/{id}/lines/{lineId}    qty/notes; qty=0 ⇒ cancel line (recorded, per-item cancellation like FUDO's report needs)
POST   /orders/{id}/status      {to: IN_PROGRESS|READY|DELIVERED|CLOSED|CANCELED, reason?}
```
State machine (simplified from FUDO's 7): `OPEN → IN_PROGRESS → READY → DELIVERED? → CLOSED`, with `CANCELED` reachable from any non-closed state (reason required). `CLOSED` requires payments covering total. Every transition appends to `order_events`.

**Payments**
```
POST   /orders/{id}/payments    {methodId, amountCents, receivedCents?} → change computed; supports split payments (multiple posts)
DELETE /orders/{id}/payments/{paymentId}    void before close (audited)
```

**Cash sessions (cortes de caja)**
```
POST   /cash-sessions                       open {openingFloatCents}
GET    /cash-sessions/current
POST   /cash-sessions/current/close         {countedCents per method} → expected vs counted vs diff report
GET    /cash-sessions?cursor=…              history
```

**Stock**
```
GET    /stock/levels?type=product|ingredient&lowOnly=1
POST   /stock/movements          {itemType, itemId, qtyDelta, reason: PURCHASE|ADJUSTMENT|WASTE|COUNT, comment?, unitCostCents?}
GET    /stock/movements?itemId=…&cursor=…   the ledger, filterable (matches FUDO's movement report shape)
```

**Expenses**
```
GET/POST /expenses               {date, providerId?, financialCategory, category, amountCents, notes}; two-level taxonomy kept
```

**Users**
```
GET/POST/PATCH /users            role admin; {name, email?, role, pin, active}
```

**Reports (MVP-thin)**
```
GET /reports/sales-summary?from=&to=&groupBy=day|hour|method|user
GET /reports/margins?from=&to=            per-product qty, revenue, cost, margin $ and %
```

**Ops:** `GET /healthz` (liveness), `GET /readyz` (PG ping), `GET /api/v1/events` (SSE, §6).

---

## 5. Auth

Single location, ≤10 staff, tablets behind counter. Simple and safe:

- **Two credential layers.** (1) *Device/account login:* email + password (bcrypt cost 12) — done once per tablet, establishes a long-lived refresh token. (2) *Operator PIN quick-switch:* 4–6 digit PIN (also bcrypt'd; per-user salt makes small keyspace acceptable given app-level rate limiting: 5 attempts → 30s lockout per user, tracked in memory). PIN switch requires an already-valid session on the device and just re-mints the access token for the new operator — matches how Carlos/Kate/Brenda share a counter tablet under the generic "Mostrador" pattern FUDO shows, but with real attribution.
- **Tokens.** Access = JWT HS256, 15 min, claims `{sub, role, name}`. Refresh = opaque 256-bit random, stored hashed in `refresh_tokens` table (Postgres — not Redis; survives restarts, trivially revocable), 30 days, rotated on use, delivered as `httpOnly; Secure; SameSite=Strict; Path=/api/v1/auth` cookie. Frontend keeps access token in memory only.
- **Roles:** `admin` (todo), `manager` (catalog, stock, reports, cortes), `cashier` (POS, own cash ops). Middleware:

```go
func RequireRole(roles ...domain.Role) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            u := UserFrom(r.Context())               // set by RequireAuth (JWT verify)
            if !u.Role.In(roles...) { respond.Error(w, ErrForbidden); return }
            next.ServeHTTP(w, r)
        })
    }
}
// router: r.With(RequireAuth, RequireRole(RoleManager, RoleAdmin)).Mount("/products", …)
```

Deferred beyond MVP: OAuth, device registration/allow-listing, per-terminal audit beyond `user_id` on every mutation.

---

## 6. Real-time: SSE, in-process broker

**Recommendation: Server-Sent Events.** Reasons over WebSocket: traffic is strictly server→client (order board, menu-version pings); native `EventSource` gives free auto-reconnect with `Last-Event-ID` replay; plain HTTP through Caddy with zero upgrade config; testable with curl. Over polling: instant board updates matter on a busy counter (ux report gap #9), and SSE is barely more code than a polling endpoint.

```go
// internal/realtime/broker.go — in-process; Redis pub/sub NOT needed at 1 API replica
type Event struct { ID uint64; Type string; Data any }
type Broker interface {
    Publish(Event)
    Subscribe(ctx context.Context) (<-chan Event, func())   // returns unsubscribe
    ReplaySince(id uint64) []Event                          // small ring buffer (~256) for Last-Event-ID
}
```

Handler: `GET /api/v1/events` sets `text/event-stream`, flushes a `: ping` comment every 25s (keeps Caddy/proxies from idling out), replays missed events on reconnect, then streams. Event types: `order.created`, `order.updated` (payload = the order summary row the board renders — no refetch needed), `menu.updated` (payload = version; client refetches `/pos/menu`). Frontend fallback: if `EventSource` errors persistently, degrade to 10s polling of `/orders?status=open` — trivial since the query exists anyway.

The `Broker` interface is the seam: if a second API replica or a separate kitchen-display service ever appears, swap the in-process implementation for Redis pub/sub without touching handlers.

---

## 7. Docker & ops

### 7.1 Dockerfile (multi-stage → distroless)

```dockerfile
# server/Dockerfile
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot     # distroless over scratch: ca-certs + tzdata + nonroot for free
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
```
Migrations are `embed.FS` inside the binary (goose), so the image is one file; no separate migrate container.

### 7.2 docker-compose.yml (prod shape on the VPS)

```yaml
services:
  api:
    build: ../server
    env_file: .env
    depends_on:
      postgres: { condition: service_healthy }
      redis:    { condition: service_healthy }
    healthcheck:
      test: ["CMD", "/api", "-healthcheck"]        # binary self-check flag hitting /healthz
      interval: 15s
      timeout: 3s
      retries: 3
    restart: unless-stopped

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: gatobobah
      POSTGRES_USER: gatobobah
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes: [pgdata:/var/lib/postgresql/data]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U gatobobah -d gatobobah"]
      interval: 10s
      timeout: 3s
      retries: 5
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    command: ["redis-server", "--appendonly", "no", "--maxmemory", "128mb", "--maxmemory-policy", "allkeys-lru"]
    healthcheck: { test: ["CMD", "redis-cli", "ping"], interval: 10s, timeout: 3s, retries: 5 }
    restart: unless-stopped     # pure cache: no volume, no AOF — losing it costs one menu rebuild

  caddy:
    image: caddy:2-alpine
    ports: ["80:80", "443:443"]
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - ../web/dist:/srv/web:ro          # built by `make web-build` before deploy
      - caddy_data:/data
    depends_on: [api]
    restart: unless-stopped

volumes: { pgdata: {}, caddy_data: {} }
```

`Caddyfile` — automatic HTTPS, SPA fallback, SSE-safe proxying:

```
pos.elgatobobah.mx {
    handle /api/* {
        reverse_proxy api:8080 { flush_interval -1 }   # -1 = stream immediately (SSE)
    }
    handle {
        root * /srv/web
        try_files {path} /index.html
        file_server
    }
    encode gzip
}
```

**Env handling:** `deploy/.env` (gitignored) + committed `.env.example` documenting every var: `POSTGRES_PASSWORD`, `DATABASE_URL`, `REDIS_URL`, `JWT_SECRET`, `PORT`, `LOG_LEVEL`, `CORS_ORIGIN`. Dev override `docker-compose.dev.yml` runs only postgres+redis with published ports; the Go API runs on the host under air, the web app under `bun dev` (Vite proxy `/api` → `localhost:8080`) — fastest inner loop, containers only for stateful deps.

### 7.3 Makefile (root)

```make
dev:            ## postgres+redis up, air, vite — the one command to start hacking
	docker compose -f deploy/docker-compose.dev.yml up -d
	(cd server && air) & (cd web && bun dev)
build:          ## prod images + web bundle
	cd web && bun install && bun run build
	docker compose -f deploy/docker-compose.yml build
migrate:        cd server && goose -dir migrations postgres "$$DATABASE_URL" up
migrate-new:    cd server && goose -dir migrations create $(name) sql
sqlc:           cd server && sqlc generate
seed:           cd server && go run ./cmd/seed --from ../references
test:           cd server && go test ./...
test-db:        cd server && go test -tags=integration ./...
deploy:         make build && docker compose -f deploy/docker-compose.yml up -d
lint:           cd server && golangci-lint run
```

---

## 8. Testing approach (what earns tests in an MVP)

Test the money and the invariants; skip CRUD choreography.

1. **`domain/costing_test.go` — highest value.** Table-driven against real FUDO data shapes: recipe roll-up with merma % (ingredient at $47/kg, 5% merma, 0.398 kg → exact centavos), packaging lines included, nested prep (Mezcla Crepas) resolution, cycle detection, rounding policy (round half-up once per line, sum lines — document it in the test). Margin $ and % derivation.
2. **`domain/order_test.go`.** Order pricing: line totals with modifier price deltas, group min/max enforcement (min 1 max 2 perlas + $0 "sin perlas" case), split payments summing to total, change calculation, CLOSED-requires-full-payment. Full state-machine matrix: every (from, to) pair asserted allowed/rejected.
3. **`domain/stock_test.go`.** Ledger application: sale of recipe product emits correct per-ingredient deltas (qty × recipe × through preps); availability derivation (negative on-hand clamps to 0 sellable, FUDO's Disponibilidad semantics); oversell flag honored; cancellation reverses movements.
4. **`domain/cashsession_test.go`.** Expected-cash math: opening float + cash payments − cash expenses/payouts; per-method expected vs counted diff.
5. **Integration (build-tagged, against dockerized PG):** one happy-path flow test — login → PIN switch → create order with modifiers → pay split → close → assert stock movements + cash session totals; plus idempotency-key replay on POST /orders and /payments (must not double-insert). Use the dev compose Postgres with per-test schema (cheap, no testcontainers dependency for MVP).
6. **Handler-level:** just the error envelope contract and auth/role middleware (chi + `httptest`, in-memory stores not required — hit the integration DB).

Explicitly not tested in MVP: report SQL exactness (eyeball against FUDO exports via seed data), SSE plumbing beyond one broker unit test, catalog CRUD.

---

## 9. Performance

Single location: the perf risk isn't throughput, it's *tail latency on tablet taps*. Budget-driven targets (server-side, p95, VPS with 2 vCPU):

| Operation | p95 target | How it's met |
|---|---|---|
| `GET /pos/menu` (Redis hit) | < 10ms | pre-serialized gzipped blob, no PG touch |
| `GET /pos/menu` (rebuild) | < 80ms | single multi-join query, singleflight |
| `POST /orders` (5 lines + modifiers) | < 50ms | one tx, sqlc batch inserts, app-generated UUIDv7 (no ID round-trips) |
| `POST /orders/{id}/status`, payments | < 30ms | single-row updates + event insert |
| `GET /orders?status=open` | < 20ms | partial index, ≤ ~30 rows |
| SSE publish → client receive | < 100ms | in-process channel fan-out |

Mechanics:
- **pgxpool:** `MaxConns=10` (plenty; PG default 100 slots untouched), `MinConns=2` to avoid cold-connect latency spikes, `MaxConnLifetime=1h`, `HealthCheckPeriod=1m`.
- **Prepared statements:** pgx v5 automatic statement cache (`QueryExecModeCacheStatement`, per-conn) — every sqlc query is effectively prepared after first use; nothing to hand-tune.
- **Index coordination with the schema agent** (the service depends on these existing):
  - `orders`: partial index `(created_at DESC) WHERE status IN ('OPEN','IN_PROGRESS','READY')` — the board query; plus `(status, created_at)` for history keyset.
  - `order_lines(order_id)`, `order_modifier_selections(order_line_id)`, `payments(order_id)`, `order_events(order_id, created_at)`.
  - `products(category_id) WHERE active`, `modifier_group_attachments(product_id)`, `modifier_options(group_id)` — the menu rebuild.
  - `stock_movements(item_type, item_id, created_at DESC)` — ledger reads and SUM rollups.
  - Unique index on `idempotency_key` (orders, payments).
  - `refresh_tokens(token_hash)`.
- **HTTP:** timeouts on the server (`ReadHeaderTimeout 5s`, `IdleTimeout 120s`; no `WriteTimeout` on the SSE route — chi per-route middleware clears it), gzip via Caddy, `Cache-Control`/ETag on `/pos/menu` so tablets can 304.
- **Observability (MVP-sized):** slog per-request line with duration; a `X-Request-Id`; log any query > 100ms via pgx tracer. No Prometheus/OTel until it hurts.

---

## Build order (suggested milestones)

1. **Skeleton:** repo restructure (`web/`, `server/`), compose dev, chi + healthz + slog + config, goose + sqlc wiring, CI (`make test lint`).
2. **Auth + users** (login, refresh, PIN switch, roles).
3. **Catalog write + `GET /pos/menu`** (with Redis cache + seed importer from `references/` XLS data) — unblocks frontend migration off FUDO reads.
4. **Orders lifecycle + payments + SSE board** — the MVP heart; frontend NewOrder finally gets a working "Pagar".
5. **Stock ledger + costing roll-up** (recipes already imported in 3).
6. **Cash sessions + expenses + thin reports.**
7. Deploy: VPS, Caddy TLS, backups (`pg_dump` cron to object storage).
