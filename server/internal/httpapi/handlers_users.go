package httpapi

import (
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GET /users
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": users})
}

// POST /users
func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string  `json:"name"`
		Username *string `json:"username"`
		Role     string  `json:"role"`
		PIN      string  `json:"pin"`
		Password string  `json:"password"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, err := h.users.Create(r.Context(), app.CreateUserInput{
		Name:     body.Name,
		Username: body.Username,
		Role:     domain.Role(body.Role),
		PIN:      body.PIN,
		Password: body.Password,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, u)
}
