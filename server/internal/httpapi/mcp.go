package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// LA PUERTA PARA AGENTES (spec 072).
//
// `/mcp` habla el protocolo MCP por HTTP, sin sesión: cada POST trae el token y se resuelve solo.
// Cada herramienta NO llama a un servicio: arma una petición a una ruta de esta misma API y la
// despacha en proceso con la identidad del token. Así hereda todo lo que vive en el handler y no en
// el servicio —el RequirePermission de la ruta (leído de la base en cada llamada), el límite por
// usuario, la invalidación del menú, los eventos de seguridad— y una herramienta no puede quedarse
// atrás cuando la pantalla cambia.
//
// Lo que esta capa agrega encima es lo que la pantalla no tiene: el alcance del token, la vista
// previa por omisión, la clave de idempotencia, el límite por token y un evento de bitácora por
// llamada.

// mcpPrincipal es la identidad que el despachador pone en la petición interna. La llave del
// contexto no se exporta: una petición de afuera no puede fabricar un valor de contexto, así que
// solo el despachador produce peticiones que RequireAuth acepte sin JWT.
type mcpPrincipal struct {
	user   AuthUser
	token  app.APIPrincipal
	tool   string
	origin domain.RequestOrigin
}

type mcpPrincipalKey struct{}

func mcpPrincipalFrom(ctx context.Context) (*mcpPrincipal, bool) {
	p, ok := ctx.Value(mcpPrincipalKey{}).(*mcpPrincipal)
	return p, ok && p != nil
}

// mcpRoute es una ruta que una herramienta puede despachar: método y patrón de chi completos.
type mcpRoute struct {
	method, pattern string
}

// mcpServerKey lleva el servidor MCP armado para esta petición hasta el handler del SDK.
type mcpServerKey struct{}

// mcpMaxTokenCalls es el tope de llamadas de herramienta por token y por minuto. Un agente que
// trabaja hace pocas por minuto; uno en bucle no debe poder vaciar la base a lecturas.
const mcpMaxTokenCalls = 120

// mcpMaxIPRequests acota a quien ni siquiera trae un token válido: cada intento cuesta una
// conexión a la base.
const mcpMaxIPRequests = 300

// MCPProtocolDescription es lo que el servidor dice de sí al conectarse.
const mcpInstructions = "Herramientas del punto de venta. Las de escritura muestran una vista previa " +
	"si no mandas dry_run: false; para aplicar, manda dry_run: false y una idempotency_key nueva. " +
	"Dinero como texto decimal (\"845.50\"); fechas AAAA-MM-DD en la zona del negocio."

// newMCPHandler arma el handler de /mcp sobre `mux`, el router completo de la API.
func (h *Handlers) newMCPHandler(mux *chi.Mux) http.Handler {
	sdk := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		s, _ := r.Context().Value(mcpServerKey{}).(*mcp.Server)
		return s
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20})
	allowed := allowedOrigins(h.cfg.CORSOrigin)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			// Sin sesión no hay flujo de eventos (GET) ni sesión que cerrar (DELETE).
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Un Origin presente que no es de la pantalla es un navegador ajeno (la spec de MCP lo pide).
		if o := r.Header.Get("Origin"); o != "" && !slices.Contains(allowed, o) {
			Error(w, fmt.Errorf("%w: origen no permitido", domain.ErrForbidden))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			Error(w, fmt.Errorf("%w: cuerpo ilegible", domain.ErrValidation))
			return
		}
		// Un lote JSON-RPC metería cientos de llamadas en una petición y el límite contaría una.
		if t := bytes.TrimLeft(body, " \t\r\n"); len(t) > 0 && t[0] == '[' {
			Error(w, fmt.Errorf("%w: no se aceptan lotes; manda una llamada por petición", domain.ErrValidation))
			return
		}
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || raw == "" {
			Error(w, domain.ErrUnauthorized)
			return
		}
		principal, reason, err := h.apiTokens.AuthenticateWithReason(r.Context(), raw)
		if err != nil {
			if errors.Is(err, domain.ErrUnauthorized) {
				var tokenID int64
				if p, perr := domain.ParseAPIToken(raw); perr == nil {
					tokenID = p.TokenID
				}
				logging.SecurityEvent(r.Context(), "api_token_rejected",
					"token_id", tokenID, "reason", reason, "ip", clientIP(r))
			}
			Error(w, err)
			return
		}
		u := AuthUser{ID: principal.UserID, CompanyID: principal.CompanyID, Name: principal.UserName,
			Role: domain.Role(principal.RoleCode)}
		if ti := traceFrom(r.Context()); ti != nil {
			ti.userID, ti.role, ti.userName, ti.companyID = u.ID, string(u.Role), u.Name, u.CompanyID
		}
		perms, err := h.permissionsWithoutHolding(r.Context(), u)
		if err != nil {
			Error(w, err)
			return
		}
		sess := &mcpSession{h: h, mux: mux, outer: r, principal: principal, user: u, perms: perms,
			origin: app.OriginFrom(r.Context())}
		srv := mcp.NewServer(&mcp.Implementation{Name: "pos-" + h.cfg.Env, Version: h.version},
			&mcp.ServerOptions{Instructions: mcpInstructions})
		sess.register(srv)
		r2 := r.Clone(context.WithValue(r.Context(), mcpServerKey{}, srv))
		r2.Body = io.NopCloser(bytes.NewReader(body))
		sdk.ServeHTTP(w, r2)
	})
}

// permissionsWithoutHolding lee los permisos del usuario con una conexión que suelta de inmediato:
// el despacho de la herramienta toma la suya, y retener ésta a la vez agota el pool.
func (h *Handlers) permissionsWithoutHolding(ctx context.Context, u AuthUser) ([]domain.Permission, error) {
	tctx, release, err := h.store.AcquireTenant(ctx, u.CompanyID)
	if err != nil {
		return nil, err
	}
	defer release()
	return h.roles.EffectivePermissions(tctx, u.ID)
}

func allowedOrigins(cfg string) []string {
	var out []string
	for o := range strings.SplitSeq(cfg, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// mcpSession es lo que una petición a /mcp sabe de quién llama.
type mcpSession struct {
	h         *Handlers
	mux       *chi.Mux
	outer     *http.Request
	principal app.APIPrincipal
	user      AuthUser
	perms     []domain.Permission
	origin    domain.RequestOrigin
}

// allows dice si la herramienta se ofrece: el token tiene su alcance y el rol tiene su permiso hoy.
// No es la barrera del permiso —ésa es la ruta— sino lo que decide qué ve el agente.
func (s *mcpSession) allows(t domain.MCPTool) bool {
	if !domain.ScopeAllows(s.principal.Scopes, t.Scope) {
		return false
	}
	return t.Permission == "" || slices.Contains(s.perms, t.Permission)
}

// errDispatchDenied: una herramienta pidió una ruta que no es suya. Es un defecto del servidor, no
// del agente, y no se despacha.
var errDispatchDenied = errors.New("ruta fuera del catálogo de la herramienta")

// dispatch sirve en proceso una ruta de la lista blanca de la herramienta. Los parámetros de ruta
// solo pueden ser enteros: ningún texto del agente forma parte del camino.
func (s *mcpSession) dispatch(ctx context.Context, tool string, method, pattern string, params map[string]int64,
	query url.Values, body any) (int, []byte, error) {
	if !slices.Contains(mcpToolRoutes[tool], mcpRoute{method, pattern}) {
		logging.SecurityEvent(ctx, "mcp_dispatch_denied", "tool", tool, "method", method, "pattern", pattern)
		return 0, nil, errDispatchDenied
	}
	path := pattern
	for k, v := range params {
		path = strings.Replace(path, "{"+k+"}", strconv.FormatInt(v, 10), 1)
	}
	rctx := chi.NewRouteContext()
	if strings.Contains(path, "{") || s.mux.Find(rctx, method, path) != pattern {
		logging.SecurityEvent(ctx, "mcp_dispatch_denied", "tool", tool, "method", method, "pattern", pattern)
		return 0, nil, errDispatchDenied
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	origin := s.origin
	device, _ := json.Marshal(map[string]string{"agent": "mcp",
		"token": fmt.Sprintf("%s (#%d)", s.principal.TokenName, s.principal.TokenID), "tool": tool})
	origin.Device = device
	ictx := context.WithValue(ctx, mcpPrincipalKey{},
		&mcpPrincipal{user: s.user, token: s.principal, tool: tool, origin: origin})
	// El contexto de afuera trae el RouteContext de chi del POST /mcp; con él, chi trata la petición
	// interna como un subrouter que ya se resolvió y la enruta con el método de afuera (un GET
	// despachado respondía 405). La petición interna empieza su ruteo de cero.
	ictx = context.WithValue(ictx, chi.RouteCtxKey, nil)
	req, err := http.NewRequestWithContext(ictx, method, path, rd)
	if err != nil {
		return 0, nil, err
	}
	req.RemoteAddr = s.outer.RemoteAddr
	for _, hname := range []string{"X-Forwarded-For", "X-Real-Ip", RequestIDHeader} {
		if v := s.outer.Header.Get(hname); v != "" {
			req.Header.Set(hname, v)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
	s.mux.ServeHTTP(rec, req)
	return rec.status, rec.body.Bytes(), nil
}

// bufferedResponse guarda la respuesta del handler despachado.
type bufferedResponse struct {
	header http.Header
	status int
	wrote  bool
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }
func (b *bufferedResponse) WriteHeader(code int) {
	if !b.wrote {
		b.status, b.wrote = code, true
	}
}
func (b *bufferedResponse) Write(p []byte) (int, error) {
	b.wrote = true
	return b.body.Write(p)
}

// serverMessage saca el mensaje de un error de la API ({"error":{"message":…}}).
func serverMessage(body []byte) string {
	var env errorEnvelope
	if json.Unmarshal(body, &env) == nil {
		return env.Error.Message
	}
	return ""
}
