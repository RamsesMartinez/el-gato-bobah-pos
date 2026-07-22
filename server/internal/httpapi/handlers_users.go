package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func userIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// GET /users
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": users})
}

// POST /users  {name, username, role, pin?, password, recoveryEmail?}
func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string  `json:"name"`
		Username      *string `json:"username"`
		Role          string  `json:"role"`
		PIN           string  `json:"pin"`
		Password      string  `json:"password"`
		RecoveryEmail *string `json:"recoveryEmail"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, err := h.users.Create(r.Context(), app.CreateUserInput{
		Name:          body.Name,
		Username:      body.Username,
		Role:          domain.Role(body.Role),
		PIN:           body.PIN,
		Password:      body.Password,
		RecoveryEmail: body.RecoveryEmail,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, u)
}

// PATCH /users/{id}  {name, role, isActive, recoveryEmail?}
func (h *Handlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := userIDParam(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		Name          string  `json:"name"`
		Role          string  `json:"role"`
		IsActive      bool    `json:"isActive"`
		RecoveryEmail *string `json:"recoveryEmail"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, err := h.users.Update(r.Context(), app.UpdateUserInput{
		ID: id, Name: body.Name, Role: domain.Role(body.Role), IsActive: body.IsActive, RecoveryEmail: body.RecoveryEmail,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, u)
}

// POST /users/{id}/password  {password} — reset por admin (fuerza cambio en el próximo login).
func (h *Handlers) AdminSetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := userIDParam(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.users.AdminSetPassword(r.Context(), id, body.Password); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /users/{id}/pin  {pin} — set/reset del PIN por admin.
func (h *Handlers) AdminSetPIN(w http.ResponseWriter, r *http.Request) {
	id, err := userIDParam(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body struct {
		PIN string `json:"pin"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.users.SetPIN(r.Context(), id, body.PIN); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Self-service (el empleado sobre su propia cuenta) ---

// POST /me/password  {currentPassword, newPassword}
func (h *Handlers) ChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.users.ChangeOwnPassword(r.Context(), u.ID, body.CurrentPassword, body.NewPassword); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /me/pin  {pin} — el empleado fija/cambia su propio PIN.
func (h *Handlers) SetOwnPIN(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var body struct {
		PIN string `json:"pin"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := h.users.SetPIN(r.Context(), u.ID, body.PIN); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
