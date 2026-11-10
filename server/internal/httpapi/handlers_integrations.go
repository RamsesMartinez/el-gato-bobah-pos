package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// Integraciones (spec 072): los tokens con los que un agente opera el negocio por /mcp. El permiso
// lo pone la ruta (integrations.manage). Ninguna de estas rutas es una herramienta MCP: un token no
// puede crear ni revocar tokens.

// GET /integrations/tokens
func (h *Handlers) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	items, err := h.apiTokens.List(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// POST /integrations/tokens {name, scopes, expiresInDays, password}
//
// Pide la contraseña y no basta la sesión: la tableta se comparte, y un PIN visto por encima del
// hombro no debe alcanzar para llevarse una llave que opera el negocio desde fuera durante meses.
// Los intentos fallidos cuentan en el mismo bloqueo que el login. El dueño del token es siempre
// quien llama; un userId en el cuerpo ni se lee.
func (h *Handlers) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var body struct {
		Name          string   `json:"name"`
		Scopes        []string `json:"scopes"`
		ExpiresInDays int      `json:"expiresInDays"`
		Password      string   `json:"password"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	key := "apitoken:" + strconv.FormatInt(u.ID, 10)
	if h.authFails.blocked(r.Context(), key) {
		logging.SecurityEvent(r.Context(), "auth_lockout", "kind", "api_token_create", "user_id", u.ID, "ip", clientIP(r))
		tooManyRequests(w, h.authFails.retryAfter(r.Context(), key))
		return
	}
	created, err := h.apiTokens.Create(r.Context(), u.ID, body.Password, domain.APITokenInput{
		Name: body.Name, Scopes: body.Scopes, ExpiresInDays: body.ExpiresInDays,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			h.authFails.record(r.Context(), key)
			logging.SecurityEvent(r.Context(), "api_token_password_failed", "user_id", u.ID, "ip", clientIP(r))
		}
		Error(w, err)
		return
	}
	h.authFails.reset(r.Context(), key)
	logging.SecurityEvent(r.Context(), "api_token_created", "user_id", u.ID, "token_id", created.ID,
		"scopes", created.Scopes, "expires_at", created.ExpiresAt, "ip", clientIP(r))
	JSON(w, http.StatusCreated, created)
}

// POST /integrations/tokens/{id}/revoke
func (h *Handlers) RevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		Error(w, domain.ErrValidation)
		return
	}
	if err := h.apiTokens.Revoke(r.Context(), u.ID, id); err != nil {
		Error(w, err)
		return
	}
	logging.SecurityEvent(r.Context(), "api_token_revoked", "user_id", u.ID, "token_id", id, "ip", clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}
