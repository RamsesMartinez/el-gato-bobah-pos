package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

const maxPrefBytes = 64 * 1024 // una preferencia no debería pesar más que esto

// GET /me/preferences/{key} — preferencia del usuario autenticado; {value:null} si no existe.
func (h *Handlers) MeGetPreference(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	val, err := h.users.GetPreference(r.Context(), u.ID, chi.URLParam(r, "key"))
	if err != nil {
		Error(w, err)
		return
	}
	var raw json.RawMessage // nil → se serializa como null
	if val != nil {
		raw = json.RawMessage(val)
	}
	JSON(w, http.StatusOK, map[string]any{"value": raw})
}

// PUT /me/preferences/{key} — guarda la preferencia; el body es el valor JSON tal cual.
func (h *Handlers) MeSetPreference(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" || len(key) > 100 {
		Error(w, domain.ErrValidation)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPrefBytes+1))
	if err != nil || len(body) > maxPrefBytes || !json.Valid(body) {
		Error(w, domain.ErrValidation)
		return
	}
	if err := h.users.SetPreference(r.Context(), u.ID, key, body); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
