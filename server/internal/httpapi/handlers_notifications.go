package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GET /notifications?status=open|closed&limit=50 — la campana. Un status desconocido es 400: caer a
// «abiertas» pintaría avisos cerrados bajo la pestaña equivocada.
func (h *Handlers) ListNotifications(w http.ResponseWriter, r *http.Request) {
	open := true
	switch r.URL.Query().Get("status") {
	case "", "open":
	case "closed":
		open = false
	default:
		Error(w, fmt.Errorf("%w: status es open o closed", domain.ErrValidation))
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			Error(w, fmt.Errorf("%w: limit es un número", domain.ErrValidation))
			return
		}
		limit = n
	}
	u, _ := userFrom(r.Context())
	list, err := h.notifications.List(r.Context(), u.ID, h.permissionsOf(r.Context(), u), open, limit)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": list})
}

// GET /notifications/unread-count — el número de la campana.
func (h *Handlers) UnreadNotifications(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	n, err := h.notifications.Unread(r.Context(), u.ID, h.permissionsOf(r.Context(), u))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]int{"unread": n})
}

// POST /notifications/read — `{ids: [..]}`, leídos para quien los abrió.
func (h *Handlers) ReadNotifications(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := DecodeStrict(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.notifications.MarkRead(r.Context(), u.ID, body.IDs); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /notifications/{id}/banner-seen — la franja de ese aviso ya no vuelve para esta persona.
func (h *Handlers) NotificationBannerSeen(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.notifications.BannerSeen(r.Context(), u.ID, id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /notifications/{id}/resolve — `{action, ingredientId}`. El permiso de cada acción lo decide el
// servicio: «ya llegó» pide recibir mercancía; lo demás, configurar el almacén.
func (h *Handlers) ResolveNotification(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Action       string `json:"action"`
		IngredientID *int64 `json:"ingredientId"`
	}
	if err := DecodeStrict(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.notifications.Resolve(r.Context(), u.ID, h.permissionsOf(r.Context(), u), id,
		app.ResolveCmd{Action: body.Action, IngredientID: body.IngredientID}); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
