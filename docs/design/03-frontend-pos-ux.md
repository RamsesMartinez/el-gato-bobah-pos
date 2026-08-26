# El Gato Bobah POS — Frontend Rebuild + Tablet POS UX Plan

> **HISTÓRICO — plan de la construcción inicial, ejecutado. No es el estado actual.**
> Se conserva para entender *por qué* el diseño es como es. Para saber cómo está el sistema hoy:
> el código, [`AGENTS.md`](../../AGENTS.md) y
> [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md).
> Índice de `docs/`: [../README.md](../README.md).

Repo: `/Users/ramses/Documents/git/ramthedev/el-gato-bobah-pos` · React 18 + TS + Chakra UI v2 kept · CRA→Vite · bun · Spanish UI · Target device: 8–10" tablets, portrait AND landscape.

---

## 1. Vite migration (CRA/react-scripts 5 + TS 4.9 → Vite 6 + TS 5.x)

### 1.1 Dependency changes

```bash
bun remove react-scripts web-vitals ajv ajv-keywords @types/jest @types/styled-components
bun add -d vite @vitejs/plugin-react-swc typescript@^5 vitest jsdom @testing-library/jest-dom @testing-library/react @testing-library/user-event @types/node
```

- `ajv`/`ajv-keywords` exist only as a CRA transitive-dep workaround — gone with react-scripts.
- `web-vitals` + `src/reportWebVitals.ts` + `src/App.test.tsx` (tests CRA's "learn react" link) + `src/logo.svg` + `src/App.css`: delete.
- `@types/jest` → vitest ships its own types.

### 1.2 File moves / rewrites

| CRA | Vite |
|---|---|
| `public/index.html` with `%PUBLIC_URL%` | move to repo root `index.html`; replace `%PUBLIC_URL%/favicon.ico` → `/favicon.ico`; add `<script type="module" src="/src/main.tsx"></script>` before `</body>` |
| `src/index.tsx` | rename `src/main.tsx` (Vite convention), drop `reportWebVitals()`, keep single `ChakraProvider` here (remove the duplicate one in `App.tsx`) |
| `src/react-app-env.d.ts` | delete; add `src/vite-env.d.ts` with `/// <reference types="vite/client" />` + an `ImportMetaEnv` interface typing our vars |
| — | new `vite.config.ts`: `plugins: [react()]`, `server: { port: 3000, host: true }` (host:true so the tablet on LAN can hit the dev server), `test` block for vitest |

### 1.3 Env vars

`process.env.REACT_APP_*` → `import.meta.env.VITE_*`. Since FUDO is being dropped anyway, don't rename 1:1 — replace:

- `REACT_APP_FUDO_API_URL`, `REACT_APP_FUDO_API_TOKEN` → **deleted** (token-in-frontend goes away; auth becomes a login session against the Go backend)
- New: `VITE_API_URL` (e.g. `http://localhost:8080/api/v1`; empty in prod = same-origin behind reverse proxy)
- `REACT_APP_DEFAULT_CATEGORY_IMAGE` → delete (categories use deterministic color tiles, §4.6)
- Rewrite `src/config/env.ts` to read `import.meta.env`, keep the fail-fast `MissingEnvironmentError` pattern. Update `.env.example`.

### 1.4 tsconfig

TS 5.x, `"target": "ES2022"`, `"module": "ESNext"`, `"moduleResolution": "bundler"`, `"types": ["vite/client", "vitest/globals"]`, `"noEmit": true`, `"isolatedModules": true`, `"verbatimModuleSyntax": true` (forces `import type` — will surface ~a dozen mechanical fixes), path alias `"@/*": ["./src/*"]` mirrored in `vite.config.ts` `resolve.alias`.

### 1.5 Jest → Vitest

- `vite.config.ts` → `test: { environment: 'jsdom', globals: true, setupFiles: './src/setupTests.ts' }`
- `src/setupTests.ts`: `import '@testing-library/jest-dom/vitest'`
- Any `jest.fn/mock` → `vi.fn/mock` (currently near-zero test surface, so cheap now — the reason to do this migration first).

### 1.6 What breaks / gotchas checklist

- `%PUBLIC_URL%` references (index.html, manifest.json) — root-relative paths.
- CRA's `eslintConfig: { extends: ["react-app"] }` in package.json dies with react-scripts → add minimal flat `eslint.config.js` (typescript-eslint + react-hooks) or defer; remove the package.json block either way.
- `browserslist` block → irrelevant (esbuild target); remove.
- SVG-as-component imports (`ReactComponent`) — only `logo.svg` used it; deleted.
- Scripts: `"dev": "vite"`, `"build": "tsc -b && vite build"`, `"preview": "vite preview"`, `"test": "vitest"`, keep `"preinstall": "npx only-allow bun"`. Update `Makefile` targets to match.
- TS 4.9→5.x on Chakra v2 + framer-motion 10: expect a few `as const` / generic inference errors, all mechanical.

**Definition of done:** `bun run dev` serves the app, `bun run build` passes `tsc -b`, `bun run test` runs one smoke test rendering `<App/>`.

---

## 2. Cleanup — the single stack, and the deletion list

**Stack kept:** `App.tsx`-descended router (rebuilt), Chakra theme in `src/theme/chakraTheme.ts` as the ONLY theme source, `src/components/ProductCard/ProductCard.tsx` as the ONLY product card, `src/types/sales.ts` as the seed for domain types.

**Delete (all verified dead by the codebase report):**

```
src/routes/index.tsx                       # dead second router (its /sales/active/* ideas are absorbed into the new router)
src/pages/SalesPage.tsx
src/pages/CategoryProducts/               # duplicate order flow (incl. the .slice(1) bug)
src/components/ProductCard.tsx             # root duplicate; keep ProductCard/ProductCard.tsx
src/components/layout/DashboardLayout.tsx
src/components/layout/MainTabs.tsx
src/components/layout/Navbar.tsx           # would crash if mounted (theme.spacing refs)
src/components/products/ProductsPanel.tsx  # (salvage its search-input idea first)
src/components/ticket/TicketPanel.tsx      # (salvage its modifiers-on-line type idea first)
src/services/api/orders.ts                 # dead + hardcoded api.fu.do
src/services/api/products.ts               # only used by dead DashboardLayout
src/services/categoryImages.ts             # mock w/ Unsplash URLs (offline tablet hazard)
src/adapters/fudo.ts
src/context/ThemeContext.tsx               # dual theme system — Chakra theme wins
src/hooks/useTheme.ts                      #   (first migrate MainNav/ActiveSales to Chakra tokens)
src/types/orders.ts  src/types/api.ts  src/types/category.ts
src/App.css  src/logo.svg  src/App.test.tsx  src/reportWebVitals.ts
```

**Delete on backend swap (FUDO layer):** `src/types/fudo.ts`, `src/services/api/{axios,categories,sales}.ts`, `src/services/api/rateLimit.ts` (no rate limits against our own backend), `src/services/filters/saleFilters.ts`, `src/types/filters.ts`, `src/constants/saleStates.ts` (replaced by new state union).

**Deps to remove:** `@mui/material`, `styled-components`, `@types/styled-components` (never imported), plus §1.1 list. **Also fix:** double `ChakraProvider` (index.tsx + App.tsx → one), `getRandomStartHue` in `src/services/images.ts` (§4.6), `console.log` noise in `categories.ts`/`CategoryCard.tsx`.

---

## 3. Architecture

### 3.1 Folder layout (feature-first, shallow)

```
src/
  app/            main.tsx, App.tsx, router.tsx, providers.tsx, AppShell.tsx
  api/            client.ts (fetch wrapper), menu.ts, orders.ts, payments.ts, sse.ts
  types/          domain.ts (UI model), wire.ts (Go API DTOs), mappers.ts (wire→domain)
  stores/         ticketStore.ts (Zustand), sessionStore.ts (logged-in employee)
  features/
    pos/          POSPage, CatalogPane, CategoryRail, ProductGrid, SearchBar,
                  TicketPanel, TicketLine, TicketBottomBar, ModifierSheet, useMenu.ts
    checkout/     CheckoutSheet, OrderTypeStep, PaymentStep, CashTender, OrderConfirmation
    orders/       OrdersBoardPage, OrderCard, OrderColumn, CancelDialog, useLiveOrders.ts
    history/      SalesHistoryPage (MVP: simple list + date filter)
  components/     Breadcrumb/, CategoryGrid/, CategoryCard/, ProductCard/ (retyped survivors)
  theme/          chakraTheme.ts
  utils/          money.ts, normalize.ts (diacritics), dateUtils.ts
```

### 3.2 Server state — TanStack Query

`bun add @tanstack/react-query` (+ devtools in dev).

- **`useMenu()`** — `queryKey: ['menu']`, `GET /menu`: ONE payload with categories (2-level tree), all active+sellable products, modifier groups + options with price deltas, favorites flags. ~750 products ≈ 150–300 KB JSON — trivially cacheable. `staleTime: 5 min`, `gcTime: Infinity`, `refetchOnWindowFocus: true`. Backend sends `menuVersion`; the SSE stream (§3.5) emits `menu.updated` → `invalidateQueries(['menu'])`. **This kills the N+1 breadcrumb walk and CategoryCard's per-tap fetch**: category tree, subcategories, breadcrumbs, and search all resolve in memory.
- **`useActiveOrders()`** — `queryKey: ['orders','active']`, `GET /orders?status=active`, `refetchInterval: 10_000` as SSE fallback.
- **Mutations** — `useCreateOrder`, `useTransitionOrder`, `useCancelOrder`, `useRegisterPayment`; each does optimistic update on `['orders','active']` with rollback on error (the board must feel instant on a busy counter).

### 3.3 Client state — Zustand ticket store (refresh-proof)

`bun add zustand` — with `persist` middleware → `localStorage['egb:ticket:v1']`.

```ts
interface TicketLine {
  lineId: string;               // crypto.randomUUID() — NOT productId; two boba w/ different toppings = two lines
  productId: string;
  nameSnapshot: string; unitPriceBase: number;   // snapshots survive menu edits mid-order
  qty: number;
  modifiers: { groupId: string; optionId: string; nameSnapshot: string; priceDelta: number; qty: number }[];
  notes?: string;
}
interface TicketState {
  lines: TicketLine[];
  orderType: 'MOSTRADOR' | 'LLEVAR' | 'DOMICILIO' | null;
  platform?: 'DIDI' | 'UBER_EATS' | 'RAPPI' | 'PROPIO';
  customerName?: string;
  addLine; incrementLine; decrementLine; removeLine; updateLineModifiers; setLineNotes;
  setOrderType; setCustomerName; clear;
}
// derived, via selectors: lineTotal = qty * (unitPriceBase + Σ delta*qty); ticketTotal; itemCount
```

Rules: a refresh, tab close, or accidental navigation NEVER loses the ticket; `clear()` only on confirmed order POST or explicit "Vaciar pedido" (with confirm dialog). `sessionStore` (also persisted) holds `{ employee, token }` from PIN login.

### 3.4 Domain types decoupled from wire types

- `types/wire.ts` = exact Go JSON DTOs (snake_case per `types/sales.ts` precedent). `types/domain.ts` = what components consume (camelCase, `Money` as integer **centavos** — no float math on prices). `types/mappers.ts` = the only file that knows both. Components import from `domain.ts` ONLY — this is the insulation layer that FUDO-era code lacked (components bound straight to `FudoProduct.attributes.*`).
- Core domain types: `Category {id, name, parentId, position, colorHue}` · `Product {id, name, price, categoryId, imageUrl?, favorite, hasModifiers, available}` · `ModifierGroup {id, title, min, max, options: ModifierOption[]}` · `ModifierOption {id, name, priceDelta, maxQty}` (first-class entity — fixes FUDO weakness #1; no more $0 pseudo-products or "SIN X" sentinels in the UI) · `Order {id, number, type, platform?, status, customerName?, lines, payments, total, createdAt}` · `PaymentMethod {id, name, kind: 'CASH'|'CARD'|'TRANSFER'|'PLATFORM'}` (first-class, from backend — not free strings).

### 3.5 API client + live updates

- `api/client.ts`: ~40-line `fetch` wrapper (baseURL, JSON, `Authorization: Bearer <session token>`, typed `ApiError` with backend error codes, single 401 handler → login screen). **Drop axios** — one less dep, and the interceptor/rate-limit machinery existed only for FUDO.
- `api/sse.ts` + `useLiveOrders()`: `EventSource('/api/v1/events?channel=orders')`; events `order.created|order.updated|order.cancelled|menu.updated` → surgical `setQueryData`/invalidate. On `EventSource.onerror`, mark disconnected (small "Sin conexión en vivo" pill) and let the 10s `refetchInterval` carry the board. SSE over WebSocket: one-directional is all we need, auto-reconnect is free, plays nice with Go + reverse proxies.

---

## 4. THE CORE CHALLENGE — tablet POS order screen (`/pos`)

### 4.1 Layout strategy: two modes, driven by container width

Kill the fixed-500px ticket. `POSPage` measures itself (one `ResizeObserver` hook, `useContainerWidth`) and picks:

**A. Landscape / wide (container ≥ 900px):** side-by-side.
```
┌──────────────────────────────────────┬───────────────┐
│ [🔍 Buscar…            ] [★Favoritos]│ PEDIDO (n)    │
│ [★][Bebidas Frías][Boneless][Ramen]… │ line items…   │  ticket width:
│ ┌────┐┌────┐┌────┐┌────┐┌────┐       │ (scroll)      │  clamp(300px, 32%, 380px)
│ │prod││prod││prod││prod││prod│       ├───────────────┤
│ └────┘└────┘└────┘└────┘└────┘  ...  │ Total  $185   │
│ (product grid, own scroll)           │ [ COBRAR ]    │
└──────────────────────────────────────┴───────────────┘
```
At 1024px the catalog gets ~660px → 4–5 comfortable columns (vs today's ~520px squeeze).

**B. Portrait / narrow (< 900px):** full-width catalog + persistent bottom bar + ticket as bottom sheet.
```
┌────────────────────────────┐
│ [🔍 Buscar…]               │
│ [★][Bebidas][Boneless]…    │  ← category rail, horizontal scroll
│ ┌────┐┌────┐┌────┐┌────┐   │
│ │prod││prod││prod││prod│   │  ← grid fills whole width
│ └────┘└────┘└────┘└────┘   │
├────────────────────────────┤
│ 🛒 3 arts · $185   [Ver pedido ▲] │ ← TicketBottomBar, h=64px, always visible
└────────────────────────────┘
```
"Ver pedido" (or swipe up / tap anywhere on the bar) opens the ticket as a Chakra `Drawer placement="bottom"` at ~92% height with line editing + COBRAR. Adding a product while closed pulses the bar total (150ms scale) as feedback. **The ticket is never an obstacle between the cashier and the products** — the report's biggest complaint (ticket ordered *above* products on narrow screens) is structurally impossible now.

### 4.2 Product grid — container-sized, not viewport-sized

Replace breakpoint-keyed `SimpleGrid columns={{base:2,…}}` with intrinsic CSS grid:

```tsx
<Grid templateColumns="repeat(auto-fill, minmax(118px, 1fr))" gap={2}>
```

The grid answers to its *pane*, so it's automatically right in both layout modes and when the ticket panel exists — no JS, no container-query polyfill needed (though `containerType: inline-size` on the pane is fine as progressive enhancement for tile-internal font sizing). Tile: square-ish (4:5), name (2-line clamp), price bold bottom, category-hue left border 4px, image only if `imageUrl` set (most products have none — color tile + name is the fast path). If the product is already in the ticket, a count badge (`2×`) top-right. Tap target = whole tile ≥ 44px trivially; press feedback via `transform: scale(0.97)` on `:active` (not `:hover` — touch).

### 4.3 Category navigation — zero fetches per tap

All from the cached `useMenu()` tree:

- **Category rail** (horizontal-scroll chip row, 48px tall, chips ≥44px): `★ Favoritos` first, then root categories ordered by `position`, colored by stable hue. Tap → if subcategories exist, a **second chip row** appears below (subcategory chips + "Todos"); products render immediately (parent tap = union of its subcategory products). No breadcrumb walking, no `CategoryGrid` full-screen drill for the default flow — drill-down cost 2–3 taps and multiple fetches; chips cost 1–2 taps and zero fetches. (Keep `CategoryGrid`/`CategoryCard` retyped for a "Ver categorías" overview button — nice for trainees — but the rail is the default.)
- Selected category persists in the URL (`/pos?cat=…`) so refresh restores context.

### 4.4 Search + favorites

- `SearchBar` top of catalog pane: client-side over cached menu, debounced 150ms, **diacritics-insensitive** (`normalize('NFD')` strip combining marks — "FRAPPE" matches "FRAPPÉ") on name + `code`. Typing switches grid to results; ✕ clears back to category. No network.
- Favoritos: FUDO already flags 29 products — the backend keeps `favorite`; the ★ chip is the default landing category (fastest path for the ~30 items that are 80% of tickets).

### 4.5 Ticket panel / sheet (shared component both modes)

- Line: name + modifier summary (`Grande · 50% azúcar · +Perlas ×2`) + notes preview, `[−] qty [+]` steppers (44px), line total; qty `−` at 1 → confirm-less remove with 5s "Deshacer" toast. Tap line body → reopen `ModifierSheet` prefilled for edit (§5).
- Header: order-type quick display (if chosen), "Vaciar" (confirm dialog).
- Footer: big total + `COBRAR` (h=56px, full width) → checkout (§6). Secondary: `Enviar a cocina` (create order unpaid, §6.4).
- Kill hardcoded "COMENSAL 1"/"Ticket #1", no-op "Escanear"/"Promociones" buttons.

### 4.6 Stable category colors

Fix `src/services/images.ts`: hue = deterministic hash of category **id** (`hue = fnv1a(id) % 360`, fixed s/l for light/dark), or better: `colorHue` column on the backend category so the owner can pick. Either way: same category = same color every session, restoring operator color memory.

### 4.7 Touch rules (global)

Everything interactive ≥ 44×44px; `size="lg"` Chakra defaults on POS screens; no hover-dependent affordances; `touch-action: manipulation` + `user-select: none` on tiles/steppers (kills 300ms delays and text-selection on fast repeat taps); grid/pane scrolling with `overscroll-behavior: contain` so the sheet doesn't rubber-band the page.

---

## 5. Modifier selection UX (boba: size, sweetness, ice, toppings min/max, price deltas)

### 5.1 Surface: bottom sheet, both orientations

`ModifierSheet` = Chakra `Drawer placement="bottom"`, height `min(88%, content)`, `maxW=640px` centered in landscape (a modal-width sheet — one component, two looks). Bottom sheet > centered modal on tablets: thumbs live at the bottom, and it survives portrait/landscape without relayout. Opens ONLY when `product.hasModifiers` (or notes requested); plain products add to ticket instantly on tile tap — no friction for a Ramune.

### 5.2 Sheet anatomy

```
━━━ FRAPPÉ GATUNO ─ $45 base ──────────── ✕
 Tamaño (elige 1)                 ← min=1 max=1 → segmented chips, exactly-one
   [ Chico ] [●Mediano +$10] [ Grande +$18 ]
 Azúcar (elige 1)                 [100%] [●75%] [50%] [25%] [Sin azúcar]
 Hielo (elige 1)                  [●Normal] [Poco] [Sin hielo]
 Perlas y toppings (0–3)          ← multi chips; counter "1 de 3"
   [●Perlas +$20] [Litchi +$20] [Coco +$20] …
   per-option stepper appears on selected chip when option.maxQty > 1: [Perlas −1+ ]
 Nota de cocina  [___________]
─────────────────────────────────────────
 [−] 1 [+]            [ Agregar 1 · $75 ]   ← sticky footer, running total incl. deltas
```

- **Single-select groups (max=1):** chip row behaving as radio; if `min=1` a default is preselected (backend `default_option`) so the confirm button is enabled immediately — the common path is *tap product → tap Agregar*.
- **Multi-select (min..max):** chips toggle; live `x de max` counter; at max, unselected chips disable (not hide). Confirm disabled until every `min` satisfied, with the first unsatisfied group flashed + scrolled-to on attempted confirm.
- **Price deltas** always visible on the chip (`+$20`); footer total updates live. This replaces FUDO's $0-product / penny-price hacks — deltas are just numbers on `ModifierOption`.
- **No "SIN PERLAS" sentinel options**: absence = not selecting; sweetness/ice are proper single-select groups. (The backend migration maps FUDO's sentinel products away.)

### 5.3 Fast-repeat patterns

- **Repeat last config:** tapping a modifier product that already has a line in the ticket shows a 2-option action strip instead of the full sheet: `[ +1 igual (Mediano · 75% · Perlas) ]  [ Personalizar ]`. One tap covers "same drink again" — the highest-frequency boba flow.
- **Line edit:** tap any ticket line → sheet reopens prefilled (`updateLineModifiers` on confirm, button reads "Guardar cambios").
- Later (post-MVP): "Lo de siempre" per customer, saved combos.

---

## 6. Order completion flow

`CheckoutSheet` — full-height bottom sheet over the POS, 2 steps, launched by COBRAR.

**Step 1 — Tipo de pedido + cliente** (skipped if already set in ticket header):
- Segmented cards ≥64px: `🏪 Mostrador` · `🥡 Para llevar` · `🛵 Domicilio` (→ reveals platform chips `Didi / Uber Eats / Rappi / Propio`). Maps to backend `order.type` + `platform` — mirrors FUDO's real origin data (Mostrador 2275 vs Delivery 371).
- `Nombre del cliente` optional text field (autofocus for Llevar — it's the "shout the name" identifier; prefilled "Mostrador" placeholder otherwise). No table management in MVP (counter-service café; `table_id` stays nullable in domain types for later).

**Step 2 — Pago:**
- Method tiles from `GET /payment-methods` (first-class entities, admin-manageable — fixes FUDO free-string weakness #7): `💵 Efectivo · 💳 Tarjeta · 🏦 Transferencia · 📱 Plataforma` (Plataforma preselected+locked when Domicilio+platform, amount informational since the platform collects).
- **Efectivo:** quick-tender row `[Exacto] [$100] [$200] [$500]` + numpad; **CAMBIO: $35** rendered huge (48px+) — the number the cashier actually needs. Tender < total blocks confirm.
- Tarjeta/Transferencia: confirm-only (terminal/SPEI happens outside the system; record method + amount).
- MVP: single payment per order, no split, no tips (schema leaves `payments[]` plural for later).
- `[ CONFIRMAR — $185 ]` → `POST /orders` `{type, platform?, customerName?, lines[{productId, qty, modifiers[{optionId, qty}], notes}], payment{methodId, amount, tendered?}}`. Server computes prices authoritatively; client total shown as check.

**Confirmation + landing:** success screen ~2s or tap-through: giant order number (`#42`), name, change reminder if cash, `[ Nuevo pedido ]`. Then `ticket.clear()` → back to `/pos` with ★Favoritos selected (next customer immediately). **Printing: skip for MVP** — orders board + shouted names cover a single-location café; ESC/POS or `window.print` ticket is Phase 2 (leave a `printTicket(order)` stub).

**"Enviar a cocina" (pay later):** creates the order with `paid=false`, no Step 2 → lands on the board with a `POR COBRAR` badge; cobrar later from the order detail (§7). Payment status is a **field, not a state** — deliberately kills FUDO's `PAYMENT-PROCESS` state.

---

## 7. Active orders board (`/pedidos`)

### 7.1 States — 4 instead of FUDO's 7

```
EN_PREPARACION ──► LISTO ──► ENTREGADO (terminal → history)
      │              │
      └──────────────┴─────► CANCELADO (terminal, requires reason)
```
Dropped: `PENDING` (order creation is atomic — POST lands in EN_PREPARACION), `PAYMENT-PROCESS` (→ `paid` boolean badge), `DELIVERY-SENT` (LISTO covers "waiting for rider"; a platform tag on the card gives context). Each card shows exactly **one primary advance button** — no state-picker dropdowns.

### 7.2 Layout — kanban cards, not `size="sm"` tables

- **Landscape:** 3 columns `Preparando / Listo / Entregado (últimos 60 min)`, each independently scrollable, count badges in headers.
- **Portrait:** segmented control (`Preparando (4) · Listo (2) · Entregado`) switching one full-width column — no cramped side-by-side.
- **OrderCard** (≥88px tall): `#42 · 🥡 Llevar · "Karla"` + elapsed timer (amber >10 min, red >20) + first 2–3 line summary + total + `POR COBRAR` badge if unpaid. Primary button full-width 48px: `MARCAR LISTO` → `ENTREGAR` (if unpaid, `ENTREGAR` routes through the payment step first). Overflow `⋯` menu: Ver detalle · Cobrar · Cancelar.
- **Cancellation:** confirm dialog with required reason — radio `Cliente canceló / Error de captura / Sin insumos / Otro` + optional text → `POST /orders/:id/cancel {reason, comment}`. Feeds the cancellations report (FUDO shows 638 cancelled items — this needs to be auditable) and triggers backend stock reversal.
- **Filter chips** above columns: `Todos · Mostrador · Llevar · Domicilio` — replacing the broken tab navigation (`/sales/active/*` routes that 404-bounce today). Filter state in URL query, not path.
- **Live:** SSE-invalidated (§3.5) + 10s polling fallback; new-order cards animate in.
- **Order detail** (`/pedidos/:id`, sheet or route): full lines w/ modifiers+notes, payment info, transition + cancel + cobrar actions — fixes today's dead `/sales/:id` navigation.

---

## 8. Navigation shell for future modules

`AppShell` wraps everything post-login:

- **Landscape (≥900px):** left **icon rail, 72px** wide (icon + 10px label). MVP items: `🛒 Vender` (/pos), `📋 Pedidos` (/pedidos, live count badge), `🕐 Historial` (/historial); Phase-2 items ship greyed-behind-role: `📦 Productos, 🥬 Ingredientes, 🏬 Almacén, 💸 Gastos, 💰 Cortes, 📊 Reportes, 👥 Empleados, ⚙️ Ajustes`. Bottom of rail: employee avatar/initials → menu (Cambiar usuario / Cerrar sesión). 72px is tap-safe and burns far less width than the old 500px habit.
- **Portrait (<900px):** rail collapses to a hamburger in a slim 48px top bar (POS screen real estate is sacred); `Vender`/`Pedidos` also reachable via the bottom bar region. (No persistent bottom tab bar in portrait POS — it would collide with `TicketBottomBar`.)
- **Role gating:** `session.employee.role ∈ {cajero, gerente, admin}`; rail renders only permitted routes and a `<RequireRole>` route guard backs it. Cajero: Vender/Pedidos/Historial. Gerente: +Almacén/Gastos/Cortes. Admin: all. MVP auth: employee PIN pad on launch (fast user switching at a shared counter tablet), JWT session from Go backend.
- Replaces `MainNav.tsx` (hardcoded "Ramses", ThemeContext dependency).

---

## 9. Screen inventory (MVP) + component survival

### Screens

| Route | Screen | Components |
|---|---|---|
| `/login` | PIN pad | `PinPad`, `EmployeePicker` |
| `/pos` | **POS order screen** | `POSPage` (layout switch) → `SearchBar`, `CategoryRail`, `SubcategoryRail`, `ProductGrid`→`ProductCard`, `TicketPanel`/`TicketBottomBar`+`TicketSheet` → `TicketLine`, `QtyStepper`, `ModifierSheet`, `RepeatLastStrip` |
| `/pos` overlay | Checkout | `CheckoutSheet` → `OrderTypeStep`, `PaymentStep` (`PaymentMethodTiles`, `CashTender`+`Numpad`), `OrderConfirmation` |
| `/pedidos` | Active orders board | `OrdersBoardPage` → `OrderTypeFilterChips`, `OrderColumn`×3 / segmented, `OrderCard`, `CancelDialog`, `LiveStatusPill` |
| `/pedidos/:id` | Order detail | `OrderDetailSheet` |
| `/historial` | Sales history (thin MVP) | date filter + paginated closed/cancelled list reusing `OrderCard` compact |
| shell | — | `AppShell` (`NavRail`, `TopBar`), `RequireRole` |

### Existing component survival matrix

| File | Fate | Retyping / changes |
|---|---|---|
| `src/theme/chakraTheme.ts` | **KEEP** — sole theme | Add semantic tokens for the 4 order states + category-hue helpers; keep light-mode-only for MVP (delete `useColorMode` reads in NewOrder; dark mode = post-MVP with real token audit) |
| `src/components/ProductCard/ProductCard.tsx` | **KEEP** | Props `FudoProduct` → `domain.Product`; delete `attributes.*` access, `https://dev.fu.do` base URL, placehold.co fallback (color tile fallback); add ticket-count badge + `:active` press state |
| `src/components/CategoryGrid` + `CategoryCard` | **KEEP** (secondary "Ver categorías" view) | Props `FudoCategory` → `domain.Category`; delete CategoryCard's `getSubCategories` console.log fetch on tap; color from stable hue |
| `src/components/Breadcrumb` + `types/breadcrumb.ts` | KEEP (used by catalog overview view only) | No changes; rail flow doesn't need it |
| `src/services/images.ts` | KEEP | Replace `getRandomStartHue` with id-hash hue (§4.6) |
| `src/utils/dateUtils.ts`, `src/constants/routes.ts` | KEEP | routes.ts rewritten to new route map |
| `src/types/sales.ts` | SEED → `types/wire.ts`/`domain.ts` then delete original |  |
| `src/pages/NewOrder/NewOrder.tsx` | **REBUILD as `features/pos/POSPage`** | Visual structure informs layout; logic (local ticket state, breadcrumb walking, 500px panel) all replaced |
| `src/pages/ActiveSales/ActiveSales.tsx` | **REBUILD as `OrdersBoardPage`** | Tables → cards; ThemeContext → Chakra tokens; `#40CFA3` literal → theme token |
| `src/components/layout/MainNav.tsx` | REPLACE with `AppShell` | — |
| Everything in §2 delete list | DELETE | — |

---

## 10. Build order (suggested milestones)

1. **M0 — Vite migration + cleanup** (§1, §2): pure mechanics, app still runs against FUDO. One PR, easy review.
2. **M1 — Data layer**: TanStack Query + Zustand ticket store + `domain.ts`/`wire.ts`/`client.ts` against the Go backend's `/menu` (mock JSON fixture from the FUDO exports until backend lands — the `references/` XLS gives real data for a `menu.fixture.json`).
3. **M2 — POS screen** (§4): layout modes, grid, rail, search, ticket + persistence. Testable with fixture data, no backend needed.
4. **M3 — Modifier sheet** (§5) + checkout (§6) against real `POST /orders`.
5. **M4 — Orders board** (§7) + SSE + AppShell/login (§8).
6. **M5 — Historial thin slice**; polish pass (timers, undo toasts, offline pill).

**Key risks:** Chakra v2 `Drawer` bottom-sheet drag ergonomics (no native drag-to-dismiss — acceptable MVP: tap-to-close + close button; add `framer-motion` drag later, it's already a dep) · menu payload growth (mitigate: gzip + ETag, it's fine at 750 products) · tablet browser matrix (test on the actual cheap Android tablet early — `dvh` units, `EventSource` reconnect behavior).
