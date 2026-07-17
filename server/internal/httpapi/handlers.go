package httpapi

import (
	"net/http"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

const refreshCookie = "refresh_token"

// Deps agrupa las dependencias de los handlers (crece por fase).
type Deps struct {
	Cfg        config.Config
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
	Broker     *realtime.Broker
}

type Handlers struct {
	cfg        config.Config
	jwt        *auth.Manager
	auth       *app.AuthService
	users      *app.UsersService
	menu       *app.MenuService
	menuCache  *cache.MenuCache
	suggest    *app.SuggestService
	costing    *app.CostingService
	orders     *app.OrdersService
	backoffice *app.BackofficeService
	admin      *app.AdminService
	broker     *realtime.Broker
}

func NewHandlers(d Deps) *Handlers {
	return &Handlers{
		cfg: d.Cfg, jwt: d.JWT, auth: d.Auth, users: d.Users,
		menu: d.Menu, menuCache: d.MenuCache, suggest: d.Suggest, costing: d.Costing, orders: d.Orders,
		backoffice: d.Backoffice, admin: d.Admin, broker: d.Broker,
	}
}

type sessionResponse struct {
	AccessToken string      `json:"accessToken"`
	User        domain.User `json:"user"`
}

func (h *Handlers) setRefreshCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    token,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.cfg.Env == "production",
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(app.RefreshTokenTTL),
	})
}

func (h *Handlers) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: "", Path: "/api/v1/auth",
		HttpOnly: true, Secure: h.cfg.Env == "production", SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
}

func (h *Handlers) writeSession(w http.ResponseWriter, s *app.Session, status int) {
	h.setRefreshCookie(w, s.RefreshToken)
	JSON(w, status, sessionResponse{AccessToken: s.AccessToken, User: s.User})
}

// POST /auth/login
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	s, err := h.auth.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		Error(w, err)
		return
	}
	h.writeSession(w, s, http.StatusOK)
}

// POST /auth/pin-switch (requires a valid device session)
func (h *Handlers) PinSwitch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID int64  `json:"userId"`
		PIN    string `json:"pin"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	s, err := h.auth.PinSwitch(r.Context(), body.UserID, body.PIN)
	if err != nil {
		Error(w, err)
		return
	}
	h.writeSession(w, s, http.StatusOK)
}

// POST /auth/refresh (reads refresh cookie)
func (h *Handlers) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		Error(w, domain.ErrUnauthorized)
		return
	}
	s, err := h.auth.Refresh(r.Context(), c.Value)
	if err != nil {
		Error(w, err)
		return
	}
	h.writeSession(w, s, http.StatusOK)
}

// POST /auth/logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil {
		_ = h.auth.Logout(r.Context(), c.Value)
	}
	h.clearRefreshCookie(w)
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
		"id": u.ID, "name": u.Name, "role": u.Role,
	})
}
