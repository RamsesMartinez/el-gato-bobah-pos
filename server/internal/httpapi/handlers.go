package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

const refreshCookie = "refresh_token"

// authFailMax/Window: account-targeted lockout. 10 wrong secrets lock that
// username/user for the rest of a short window (self-healing, no permanent lock).
// This is the real brute-force gate for passwords and the 4-digit PIN. The window is
// deliberately short so a mistyped-PIN operator (or someone maliciously locking a
// colleague on shared café WiFi) recovers fast; brute force is still bounded to
// ~2880 tries/day/account, which bcrypt + the weak-PIN blocklist make impractical.
const (
	authFailMax    = 10
	authFailWindow = 5 * time.Minute
	// docExtractMax: extracciones de documento por hora y por usuario. Cada una es una llamada
	// pagada a un modelo, así que el tope protege el presupuesto, no la seguridad: un local
	// captura unas cuantas compras al día, y 60/hora deja margen de sobra para reintentar una
	// foto borrosa sin que un bucle accidental en el front cueste dinero.
	docExtractMax = 60
	// platformPriceMax/Window: escrituras de precio de plataforma por usuario. No es un tope de
	// presupuesto como el de arriba, es de amplificación: cada escritura invalida el menú cacheado
	// y publica un `menu.updated` que hace refetch a TODAS las tablets conectadas, así que un bucle
	// convierte un request en una tormenta en todo el local. 120 en 5 minutos deja capturar una
	// lista completa a mano —un producto cada dos segundos y medio, sostenido— y acota el script.
	platformPriceMax    = 120
	platformPriceWindow = 5 * time.Minute
)

// Deps agrupa las dependencias de los handlers (crece por fase).
type Deps struct {
	Cfg        config.Config
	Version    string // SHA del build (ldflags); "dev" en local
	BuiltAt    string // timestamp del build (ldflags); "" en local
	JWT        *auth.Manager
	Auth       *app.AuthService
	Users      *app.UsersService
	Menu       *app.MenuService
	MenuCache  *cache.MenuCache
	Suggest    *app.SuggestService
	Costing    *app.CostingService
	Orders     *app.OrdersService
	Backoffice *app.BackofficeService
	Admin      *app.AdminService
	Settings   *app.SettingsService
	Company    *app.CompanyService
	Reset      *app.ResetService
	Broker     *realtime.Broker
	// PurchaseDoc puede ser nil: la extracción de tickets es opcional (sin ANTHROPIC_API_KEY el
	// POS opera capturando las líneas a mano).
	PurchaseDoc    *app.PurchaseDocService
	PlatformPrices *app.PlatformPricesService
	Sales          *app.SalesService
}

type Handlers struct {
	cfg            config.Config
	version        string
	builtAt        string
	jwt            *auth.Manager
	auth           *app.AuthService
	users          *app.UsersService
	menu           *app.MenuService
	menuCache      *cache.MenuCache
	suggest        *app.SuggestService
	costing        *app.CostingService
	orders         *app.OrdersService
	backoffice     *app.BackofficeService
	admin          *app.AdminService
	settings       *app.SettingsService
	company        *app.CompanyService
	reset          *app.ResetService
	broker         *realtime.Broker
	purchaseDoc    *app.PurchaseDocService
	platformPrices *app.PlatformPricesService
	sales          *app.SalesService
	// docExtract limita el endpoint de extracción: cada llamada cuesta dinero en la API del
	// modelo, así que un cliente con un bug (o malicioso) no puede vaciar el presupuesto.
	docExtract *rateLimiter
	// platformPrices limita las ESCRITURAS de precio por usuario (ver platformPriceMax).
	platformPriceWrites *rateLimiter
	authFails           *rateLimiter // account-targeted brute-force lockout (per username / user id)
	authIPs             *rateLimiter // per-IP request throttle for the /auth group
}

func NewHandlers(d Deps) *Handlers {
	return &Handlers{
		cfg: d.Cfg, version: d.Version, builtAt: d.BuiltAt, jwt: d.JWT, auth: d.Auth, users: d.Users,
		menu: d.Menu, menuCache: d.MenuCache, suggest: d.Suggest, costing: d.Costing, orders: d.Orders,
		backoffice: d.Backoffice, admin: d.Admin, settings: d.Settings, company: d.Company, reset: d.Reset, broker: d.Broker,
		purchaseDoc:    d.PurchaseDoc,
		platformPrices: d.PlatformPrices,
		sales:          d.Sales,
		docExtract:     newRateLimiter(d.Cfg.RedisURL, "ratelimit:doc-extract:", docExtractMax, time.Hour),
		// Redis-backed cuando REDIS_URL está definido (contadores compartidos entre réplicas y
		// que sobreviven un restart); si no, caen a in-memory (dev). Prefijos separados: los dos
		// limiters comparten la misma instancia de Redis sin pisarse las claves.
		authFails: newRateLimiter(d.Cfg.RedisURL, "ratelimit:auth-fails:", authFailMax, authFailWindow),
		authIPs:   newRateLimiter(d.Cfg.RedisURL, "ratelimit:auth-ips:", 60, time.Minute),
		platformPriceWrites: newRateLimiter(d.Cfg.RedisURL, "ratelimit:platform-price:",
			platformPriceMax, platformPriceWindow),
	}
}

type sessionResponse struct {
	AccessToken string      `json:"accessToken"`
	User        domain.User `json:"user"`
}

// La cookie de refresh codifica el tenant como "cid.token": el /refresh necesita fijar la
// empresa (para RLS) ANTES de conocer al usuario. cid no es secreto (solo dice qué empresa);
// la autenticación real es el token aleatorio. Ver AuthService.Refresh.
func (h *Handlers) setRefreshCookie(w http.ResponseWriter, companyID int64, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    strconv.FormatInt(companyID, 10) + "." + token,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.cfg.Env == "production",
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(app.RefreshTokenTTL),
	})
}

// parseRefreshCookie separa "cid.token". Devuelve (0,"") si el formato no calza.
func parseRefreshCookie(v string) (int64, string) {
	cid, token, ok := strings.Cut(v, ".")
	if !ok {
		return 0, ""
	}
	id, err := strconv.ParseInt(cid, 10, 64)
	if err != nil {
		return 0, ""
	}
	return id, token
}

func (h *Handlers) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: "", Path: "/api/v1/auth",
		HttpOnly: true, Secure: h.cfg.Env == "production", SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
}

func (h *Handlers) writeSession(w http.ResponseWriter, s *app.Session, status int) {
	h.setRefreshCookie(w, s.CompanyID, s.RefreshToken)
	JSON(w, status, sessionResponse{AccessToken: s.AccessToken, User: s.User})
}

// POST /auth/login  {username, slug, password}. El identificador es username@slug; también se
// acepta que venga junto en `username` ("nick@empresa") y aquí se separa.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Slug     string `json:"slug"`
		Password string `json:"password"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.Slug == "" {
		if nick, slug, ok := strings.Cut(body.Username, "@"); ok {
			body.Username, body.Slug = nick, slug
		}
	}
	// Lockout por cuenta = empresa+usuario: bloquea antes de tocar bcrypt tras demasiados fallos.
	key := "login:" + body.Slug + ":" + body.Username
	if h.authFails.blocked(r.Context(), key) {
		logging.SecurityEvent(r.Context(), "auth_lockout", "kind", "login", "slug", body.Slug, "username", body.Username, "ip", clientIP(r))
		tooManyRequests(w, h.authFails.retryAfter(r.Context(), key))
		return
	}
	s, err := h.auth.Login(r.Context(), body.Username, body.Slug, body.Password)
	if err != nil {
		h.authFails.record(r.Context(), key)
		if errors.Is(err, domain.ErrInvalidCredentials) {
			logging.SecurityEvent(r.Context(), "login_failed", "slug", body.Slug, "username", body.Username, "ip", clientIP(r))
			// Mensaje un poco más orientativo SIN revelar cuál de los tres falló (anti-enumeración):
			// el mismo texto para usuario/empresa/contraseña incorrectos.
			Error(w, fmt.Errorf("%w: revisa el usuario, la empresa y la contraseña", domain.ErrInvalidCredentials))
			return
		}
		Error(w, err)
		return
	}
	h.authFails.reset(r.Context(), key) // success: don't penalize the next legit login
	h.writeSession(w, s, http.StatusOK)
}

// POST /auth/pin-switch (requires a valid device session)
func (h *Handlers) PinSwitch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		// Puntero: con el modo de solo-PIN el cliente NO manda a quién, y el servidor lo deduce.
		// Con el modo apagado, su ausencia se rechaza — no se cae al modo permisivo en silencio,
		// que aquí significaría aceptar cualquier PIN sin saber de quién es.
		UserID *int64 `json:"userId"`
		PIN    string `json:"pin"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	actor, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	// El relevo conserva el reloj de ESTA estación, así que hace falta el refresh que la estación
	// viene presentando. Sin cookie no hay sesión de dispositivo que heredar, y arrancar una nueva
	// sería regalar un turno completo a cambio de un PIN.
	ck, err := r.Cookie(refreshCookie)
	if err != nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	_, refreshActual := parseRefreshCookie(ck.Value)
	if refreshActual == "" {
		Error(w, domain.ErrUnauthorized)
		return
	}
	// Sin userId, el negocio tiene que estar en modo de solo-PIN: ahí el PIN identifica y el
	// servidor deduce de quién es. Con el modo apagado se RECHAZA — caer al modo permisivo aquí
	// significaría aceptar cualquier PIN sin saber de quién, y con eso la atribución del arqueo
	// dejaría de valer.
	if body.UserID == nil {
		// El limitador cuelga de QUIEN PIDE, no de a quién se busca: en este modo no hay a quién
		// buscar, y sin llave la rama nacía sin ninguna protección. Como aquí el PIN IDENTIFICA,
		// cada intento se prueba contra toda la plantilla a la vez —con 8 personas la esperanza
		// baja a ~62,500 intentos— y si cae el del admin, quien ataca recibe rol de admin.
		llave := "pinsolo:" + strconv.FormatInt(actor.ID, 10)
		if h.authFails.blocked(r.Context(), llave) {
			logging.SecurityEvent(r.Context(), "auth_lockout", "kind", "pin_solo", "ip", clientIP(r))
			tooManyRequests(w, h.authFails.retryAfter(r.Context(), llave))
			return
		}
		opciones, err := h.auth.UnlockOptions(r.Context())
		if err != nil || !opciones.PinOnly {
			Error(w, fmt.Errorf("%w: falta indicar quién va a desbloquear", domain.ErrValidation))
			return
		}
		s, err := h.auth.PinSwitchSoloPin(r.Context(), body.PIN, actor.ID, refreshActual)
		if err != nil {
			h.authFails.record(r.Context(), llave)
			// El evento no puede decir a quién se intentó desbloquear: en este modo justamente no
			// se sabe. Lleva solo el origen.
			logging.SecurityEvent(r.Context(), "pin_failed", "modo", "solo_pin", "ip", clientIP(r))
			Error(w, err)
			return
		}
		h.authFails.reset(r.Context(), llave)
		h.writeSession(w, s, http.StatusOK)
		return
	}
	objetivo := *body.UserID
	// PIN keyspace is tiny (4 digits) — lock per target user id, which the caller
	// controls in the body, so guessing any operator's PIN is throttled.
	key := "pin:" + strconv.FormatInt(objetivo, 10)
	if h.authFails.blocked(r.Context(), key) {
		logging.SecurityEvent(r.Context(), "auth_lockout", "kind", "pin", "target_user_id", objetivo, "ip", clientIP(r))
		tooManyRequests(w, h.authFails.retryAfter(r.Context(), key))
		return
	}
	s, err := h.auth.PinSwitchEnEstacion(r.Context(), objetivo, body.PIN, actor.ID, refreshActual)
	if err != nil {
		h.authFails.record(r.Context(), key)
		if errors.Is(err, domain.ErrInvalidCredentials) {
			// El evento lleva a QUIÉN se intentó desbloquear, nunca el PIN: un secreto en un log
			// es peor que no tener el log.
			logging.SecurityEvent(r.Context(), "pin_failed", "target_user_id", objetivo, "ip", clientIP(r))
		}
		Error(w, err)
		return
	}
	h.authFails.reset(r.Context(), key)
	h.writeSession(w, s, http.StatusOK)
}

// POST /auth/refresh (reads refresh cookie "cid.token")
func (h *Handlers) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	companyID, token := parseRefreshCookie(c.Value)
	if companyID == 0 {
		Error(w, domain.ErrUnauthorized)
		return
	}
	s, err := h.auth.Refresh(r.Context(), companyID, token)
	if err != nil {
		Error(w, err)
		return
	}
	h.writeSession(w, s, http.StatusOK)
}

// POST /auth/logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil {
		if companyID, token := parseRefreshCookie(c.Value); companyID != 0 {
			_ = h.auth.Logout(r.Context(), companyID, token)
		}
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// POST /auth/forgot  {slug, username | username="nick@slug"} — solicita recuperación.
// Anti-enumeración: SIEMPRE responde 204 (no revela si la cuenta o su email existen).
func (h *Handlers) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Slug     string `json:"slug"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.Slug == "" {
		if nick, slug, ok := strings.Cut(body.Username, "@"); ok {
			body.Username, body.Slug = nick, slug
		}
	}
	// best-effort: ignoramos el error interno para no filtrar existencia por diferencias de
	// respuesta; los fallos reales quedan en el log/eventos de seguridad del servicio.
	_ = h.reset.Request(r.Context(), body.Slug, body.Username)
	w.WriteHeader(http.StatusNoContent)
}

// POST /auth/reset  {token, password} — confirma con el token del email (cid.token).
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	companyID, token := parseRefreshCookie(body.Token) // mismo formato cid.token
	if companyID == 0 {
		Error(w, domain.ErrResetInvalid) // token malformado → mismo mensaje accionable
		return
	}
	if err := h.reset.Confirm(r.Context(), companyID, token, body.Password); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /auth/me
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	JSON(w, http.StatusOK, map[string]any{
		"id": u.ID, "companyId": u.CompanyID, "name": u.Name, "role": u.Role,
	})
}

// GET /auth/unlock-options
//
// Qué debe pedir la pantalla de bloqueo, y a quiénes puede ofrecer. Solo id y nombre: la rejilla se
// pinta en una tableta a la vista del público.
func (h *Handlers) UnlockOptions(w http.ResponseWriter, r *http.Request) {
	opciones, err := h.auth.UnlockOptions(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, opciones)
}
