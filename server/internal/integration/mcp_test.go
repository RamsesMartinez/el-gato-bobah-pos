//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// mcpWorld es la API completa servida como gatobobah_app, con una empresa A (la de las pruebas) y
// una B ajena.
type mcpWorld struct {
	owner    *store.Store
	app      *store.Store
	router   http.Handler
	jm       *auth.Manager
	admin    int64
	company  int64
	other    int64
	otherAdm int64
}

func mcpRouter(st *store.Store, jm *auth.Manager) http.Handler {
	orders := app.NewOrdersService(st, nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Broker: realtime.NewBroker(), MenuCache: cache.NewMenuCache(""),
		Menu: app.NewMenuService(st, nil), Orders: orders, Backoffice: app.NewBackofficeService(st, nil),
		Admin: app.NewAdminService(st), Sales: app.NewSalesService(st, nil), Audit: app.NewAuditService(st),
		Attendance: app.NewAttendanceService(st, nil), CashConcepts: app.NewCashConceptsService(st),
		PlatformPrices: app.NewPlatformPricesService(st), Users: app.NewUsersService(st, nil, false, ""),
		APITokens: app.NewAPITokensService(st),
	})
	return httpapi.Router(config.Config{}, jm, h, st)
}

func newMCPWorld(t *testing.T) mcpWorld {
	t.Helper()
	st := newTestStore(t)
	w := mcpWorld{owner: st, company: defaultCompanyID, jm: auth.NewManager(secretoDelNegocioEnPruebas, nil)}
	w.admin = makeUser(t, st, "admin-mcp", "admin")
	w.other = makeCompany(t, st, "mcp-ajena")
	w.otherAdm = makeUserIn(t, st, w.other, "admin-mcp-ajeno", "admin")
	h, err := auth.HashSecret(tokenPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(context.Background(), `update users set password_hash = $1 where id in ($2, $3)`, h, w.admin, w.otherAdm); err != nil {
		t.Fatal(err)
	}
	w.app = appRoleStore(t)
	w.router = mcpRouter(w.app, w.jm)
	return w
}

// token crea un token por el servicio, como la pantalla.
func (w mcpWorld) token(t *testing.T, company, user int64, scopes ...string) string {
	t.Helper()
	ctx, release, err := w.app.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	tok, err := app.NewAPITokensService(w.app).Create(app.WithActorUser(ctx, user), user, tokenPassword,
		domain.APITokenInput{Name: "Claude de prueba", Scopes: scopes, ExpiresInDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	return tok.Token
}

type rpcResult struct {
	status  int
	result  json.RawMessage
	rpcErr  json.RawMessage
	isError bool
	text    string
	data    map[string]any
}

func mcpRPC(t *testing.T, r http.Handler, token, method string, params any, hdr ...string) rpcResult {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	out := rpcResult{status: rec.Code}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	out.result, out.rpcErr = env.Result, env.Error
	var res struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Structured map[string]any `json:"structuredContent"`
	}
	_ = json.Unmarshal(env.Result, &res)
	out.isError, out.data = res.IsError, res.Structured
	if len(res.Content) > 0 {
		out.text = res.Content[0].Text
	}
	return out
}

func callTool(t *testing.T, r http.Handler, token, name string, args map[string]any) rpcResult {
	t.Helper()
	return mcpRPC(t, r, token, "tools/call", map[string]any{"name": name, "arguments": args})
}

func listTools(t *testing.T, r http.Handler, token string) []string {
	t.Helper()
	res := mcpRPC(t, r, token, "tools/list", map[string]any{})
	var body struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(res.result, &body); err != nil {
		t.Fatalf("tools/list (%d): %v %s", res.status, err, res.rpcErr)
	}
	var names []string
	for _, x := range body.Tools {
		names = append(names, x.Name)
	}
	return names
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func TestMCPTransportGuards(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin)

	t.Run("sin token, 401", func(t *testing.T) {
		if res := mcpRPC(t, w.router, "", "tools/list", map[string]any{}); res.status != http.StatusUnauthorized {
			t.Fatalf("estado %d", res.status)
		}
	})
	t.Run("un token MCP no abre las rutas de la pantalla", func(t *testing.T) {
		rec := do(t, w.router, http.MethodGet, "/api/v1/auth/me", tok, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("/me con un token MCP respondió %d", rec.Code)
		}
	})
	t.Run("un token MCP tampoco escribe por una ruta de la pantalla", func(t *testing.T) {
		rec := do(t, w.router, http.MethodPost, "/api/v1/expenses/", tok, []byte(`{"categoryId":1,"amount":"1.00","status":"pendiente"}`), "application/json")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("POST /expenses con un token MCP = %d", rec.Code)
		}
	})
	t.Run("GET y DELETE: 405", func(t *testing.T) {
		for _, m := range []string{http.MethodGet, http.MethodDelete} {
			if rec := do(t, w.router, m, "/mcp", tok, nil, ""); rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /mcp = %d", m, rec.Code)
			}
		}
	})
	t.Run("un lote JSON-RPC se rechaza", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`[{"jsonrpc":"2.0","id":1,"method":"tools/list"}]`))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		w.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("lote = %d", rec.Code)
		}
	})
	t.Run("un Origin ajeno se rechaza", func(t *testing.T) {
		if res := mcpRPC(t, w.router, tok, "tools/list", map[string]any{}, "Origin", "https://malo.example"); res.status != http.StatusForbidden {
			t.Fatalf("origen ajeno = %d", res.status)
		}
	})
	t.Run("el token solo por header: ?token= no entra", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp?token="+tok, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		rec := httptest.NewRecorder()
		w.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("?token= = %d", rec.Code)
		}
	})
	t.Run("revocado: 401 en la siguiente llamada", func(t *testing.T) {
		tok2 := w.token(t, w.company, w.admin)
		if res := mcpRPC(t, w.router, tok2, "tools/list", map[string]any{}); res.status != http.StatusOK {
			t.Fatalf("antes de revocar = %d", res.status)
		}
		p, _ := domain.ParseAPIToken(tok2)
		if _, err := w.owner.Pool.Exec(context.Background(), `update api_tokens set revoked_at = now(), revoked_by = user_id where id = $1`, p.TokenID); err != nil {
			t.Fatal(err)
		}
		if res := mcpRPC(t, w.router, tok2, "tools/list", map[string]any{}); res.status != http.StatusUnauthorized {
			t.Fatalf("revocado = %d", res.status)
		}
	})
}

func TestMCPLogsNeverContainTheToken(t *testing.T) {
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin, "write:expenses")
	var buf bytes.Buffer
	var mu sync.Mutex
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&lockedWriter{w: &buf, mu: &mu}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	p, _ := domain.ParseAPIToken(tok)
	bad := domain.FormatAPIToken(w.company, p.TokenID, strings.Repeat("0", 64))
	mcpRPC(t, w.router, bad, "tools/list", map[string]any{})
	callTool(t, w.router, tok, "expenses_list", map[string]any{})
	callTool(t, w.router, tok, "expense_create", map[string]any{"category_id": 999999, "amount": "10.00",
		"dry_run": false, "idempotency_key": "log-check-001"})
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(buf.String(), "egbpat_") || strings.Contains(buf.String(), p.Secret) {
		t.Fatal("el log trae el token")
	}
	if !strings.Contains(buf.String(), "api_token_rejected") {
		t.Fatal("el 401 del token no dejó su evento de seguridad")
	}
}

type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func TestMCPToolsListFollowsScopeAndPermission(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	read := w.token(t, w.company, w.admin)
	names := listTools(t, w.router, read)
	if !contains(names, "expenses_list") || contains(names, "expense_create") {
		t.Fatalf("token de lectura ve: %v", names)
	}
	writer := w.token(t, w.company, w.admin, "write:expenses")
	if names := listTools(t, w.router, writer); !contains(names, "expense_create") || contains(names, "product_update") {
		t.Fatalf("token de gastos ve: %v", names)
	}
	// Un mesero con un token de catálogo: el alcance no le da el permiso del rol.
	mesero := makeUser(t, w.owner, "mesero-mcp", "mesero")
	h, _ := auth.HashSecret(tokenPassword)
	_, _ = w.owner.Pool.Exec(context.Background(), `update users set password_hash = $1 where id = $2`, h, mesero)
	tm := w.token(t, w.company, mesero, "write:catalog")
	names = listTools(t, w.router, tm)
	if contains(names, "product_update") || contains(names, "sales_summary") {
		t.Fatalf("un mesero ve herramientas de su alcance sin el permiso: %v", names)
	}
	if res := callTool(t, w.router, tm, "product_update", map[string]any{"product_id": 1, "price": "1.00", "dry_run": false,
		"idempotency_key": "mesero-0001"}); !res.isError && len(res.rpcErr) == 0 {
		t.Fatalf("un mesero cambió un precio: %s", res.result)
	}

	// Lo que la 073 agregó a los alcances que ya existían (decisión D1): almacén abarca los conteos y
	// caja abrir y cerrar. Un token emitido antes de la subida gana esto sin que nadie lo vuelva a crear;
	// la prueba lo deja escrito para que nadie lo descubra en producción.
	inv := listTools(t, w.router, w.token(t, w.company, w.admin, "write:inventory"))
	for _, tool := range []string{"stock_count_open", "stock_count_capture", "stock_count_close", "stock_count_discard"} {
		if !contains(inv, tool) {
			t.Errorf("un token de almacén no ve %s", tool)
		}
	}
	if contains(inv, "purchase_create") || contains(inv, "order_cancel") {
		t.Errorf("un token de almacén ve compras o pedidos: %v", inv)
	}
	cash := listTools(t, w.router, w.token(t, w.company, w.admin, "write:cash"))
	if !contains(cash, "cash_session_open") || !contains(cash, "cash_session_close") {
		t.Errorf("un token de caja no ve abrir y cerrar: %v", cash)
	}
	// Compras y pedidos solo con su alcance.
	if names := listTools(t, w.router, w.token(t, w.company, w.admin, "write:purchases", "write:orders")); !contains(names, "purchase_create") ||
		!contains(names, "order_cancel") || contains(names, "cash_session_close") {
		t.Errorf("compras y pedidos: %v", names)
	}
}

func TestMCPDryRunIsTheDefault(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin, "write:expenses")
	for _, args := range []map[string]any{
		{"name": "Proveedor de prueba MCP"},
		{"name": "Proveedor de prueba MCP", "dry_run": nil},
		{"name": "Proveedor de prueba MCP", "dry_run": true},
	} {
		res := callTool(t, w.router, tok, "supplier_create", args)
		if res.isError || res.data["preview"] != true {
			t.Fatalf("%v: no fue vista previa: %s", args, res.text)
		}
	}
	var n int
	_ = w.owner.Pool.QueryRow(context.Background(), `select count(*) from suppliers where name = 'Proveedor de prueba MCP'`).Scan(&n)
	if n != 0 {
		t.Fatal("una vista previa creó el proveedor")
	}
	res := callTool(t, w.router, tok, "supplier_create", map[string]any{"name": "Proveedor de prueba MCP", "dry_run": false})
	if !res.isError || !strings.Contains(res.text, "idempotency_key") {
		t.Fatalf("aplicar sin clave: %s", res.text)
	}
	res = callTool(t, w.router, tok, "supplier_create", map[string]any{"name": "Proveedor de prueba MCP", "dry_run": false,
		"idempotency_key": "proveedor-0001"})
	if res.isError || res.data["applied"] != true {
		t.Fatalf("aplicar: %s", res.text)
	}
	_ = w.owner.Pool.QueryRow(context.Background(), `select count(*) from suppliers where name = 'Proveedor de prueba MCP'`).Scan(&n)
	if n != 1 {
		t.Fatalf("hay %d proveedores", n)
	}
}

func TestMCPMoneyIsText(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin, "write:expenses")
	res := callTool(t, w.router, tok, "expense_create", map[string]any{"category_id": 1, "amount": 10.5})
	if !res.isError {
		t.Fatalf("un importe numérico pasó: %s", res.text)
	}
	res = callTool(t, w.router, tok, "expense_create", map[string]any{"category_id": 1, "amount": "10.5", "expense_date": "10/11/2026"})
	if !res.isError {
		t.Fatalf("una fecha mal escrita pasó: %s", res.text)
	}
	// Los de la 073: el cajón, el fondo, los montos por método y por terminal y lo contado del almacén.
	// Un número o un texto que no es cantidad no llega a la herramienta (ni a la vista previa).
	cash := w.token(t, w.company, w.admin, "write:cash", "write:inventory")
	for name, c := range map[string]struct {
		tool string
		args map[string]any
	}{
		"cajón como número":      {"cash_session_close", map[string]any{"register_id": 1, "counted_cash": 1040}},
		"cajón en letra":         {"cash_session_close", map[string]any{"register_id": 1, "counted_cash": "mil"}},
		"fondo dejado en letra":  {"cash_session_close", map[string]any{"register_id": 1, "float_left": "quinientos"}},
		"monto de un método":     {"cash_session_close", map[string]any{"register_id": 1, "declared": map[string]any{"2": "abc"}}},
		"monto de una terminal":  {"cash_session_close", map[string]any{"register_id": 1, "terminal_counts": map[string]any{"1": 300}}},
		"fondo de apertura":      {"cash_session_open", map[string]any{"register_id": 1, "opening_cash": "1,040"}},
		"lo contado del almacén": {"stock_count_capture", map[string]any{"count_id": 1, "lines": []any{map[string]any{"ingredient_id": 1, "counted_qty": "seis"}}}},
	} {
		if res := callTool(t, w.router, cash, c.tool, c.args); !res.isError || strings.Contains(res.text, "preview") {
			t.Errorf("%s pasó: %s", name, res.text)
		}
	}
}

func TestMCPRateLimitPerToken(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin)
	var limited bool
	for i := 0; i < 125; i++ {
		res := callTool(t, w.router, tok, "whoami", map[string]any{})
		if res.isError && strings.Contains(res.text, "Demasiadas llamadas") {
			limited = true
			if i < 120 {
				t.Fatalf("se limitó en la llamada %d, antes del tope", i+1)
			}
			break
		}
	}
	if !limited {
		t.Fatal("125 llamadas seguidas y ningún límite")
	}
}

func TestMCPPoolIsNeverHeldAcrossTheDispatch(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin)
	small := appRoleStoreWithOneConnN(t, 2)
	router := mcpRouter(small, w.jm)
	var wg sync.WaitGroup
	done := make(chan struct{})
	errs := make(chan string, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
				"params": map[string]any{"name": "expenses_list", "arguments": map[string]any{}}})
			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+tok)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				errs <- rec.Body.String()
			}
		}()
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("tres llamadas con un pool de dos conexiones se quedaron esperando")
	}
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// Cada herramienta pide en el catálogo el mismo permiso que exige su ruta: un rol con todo menos
// ese permiso rebota en la ruta, y uno con solo ese permiso pasa la compuerta.
func TestEveryToolMatchesItsRouteGate(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	mux := w.router.(interface {
		Find(*chi.Context, string, string) string
	})
	allPerms := domain.AllPermissions()
	roleWith := func(name string, perms []domain.Permission) string {
		var role int64
		if err := w.owner.Pool.QueryRow(ctx, `insert into roles (company_id, name) values ($1, $2) returning id`, w.company, name).Scan(&role); err != nil {
			t.Fatal(err)
		}
		for _, p := range perms {
			if _, err := w.owner.Pool.Exec(ctx, `insert into role_permission_overrides (company_id, role_id, permission_id, granted)
				select $1, $2, id, true from permissions where code = $3`, w.company, role, string(p)); err != nil {
				t.Fatal(err)
			}
		}
		var uid int64
		if err := w.owner.Pool.QueryRow(ctx, `insert into users (company_id, name, username, role_id) values ($1, $2::text, $2::text, $3) returning id`,
			w.company, name, role).Scan(&uid); err != nil {
			t.Fatal(err)
		}
		tok, err := w.jm.Issue(domain.User{ID: uid, CompanyID: w.company, Name: name, Role: domain.RoleCajero})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	for i, tool := range domain.MCPTools() {
		method, pattern, ok := httpapi.MCPToolRoute(tool.Name)
		if !ok {
			if tool.Name != "whoami" {
				t.Errorf("%s sin ruta", tool.Name)
			}
			continue
		}
		path := strings.NewReplacer("{id}", "1", "{ingredientId}", "1", "{productId}", "1").Replace(pattern)
		if got := mux.Find(chi.NewRouteContext(), method, path); got != pattern {
			t.Errorf("%s: la ruta %s %s no existe en el router (encontró %q)", tool.Name, method, pattern, got)
			continue
		}
		if tool.Permission == "" {
			continue
		}
		var without []domain.Permission
		for _, p := range allPerms {
			if p != tool.Permission {
				without = append(without, p)
			}
		}
		sin := roleWith("sin-"+itoa(i), without)
		con := roleWith("con-"+itoa(i), []domain.Permission{tool.Permission})
		if !forbiddenByPermission(t, w.router, method, path, sin) {
			t.Errorf("%s: un rol sin %s pasa la compuerta de %s %s", tool.Name, tool.Permission, method, pattern)
		}
		if forbiddenByPermission(t, w.router, method, path, con) {
			t.Errorf("%s: un rol con %s rebota en %s %s: el catálogo declara otro permiso", tool.Name, tool.Permission, method, pattern)
		}
	}
}
