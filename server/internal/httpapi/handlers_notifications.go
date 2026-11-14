package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

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

// lowStockBody es la sección «Alertas» como la manda la pantalla. branchMinStock se distingue
// ausente (no se toca) de null (se quita el de la sucursal).
type lowStockBody struct {
	Enabled           bool             `json:"enabled"`
	OnMin             bool             `json:"onMin"`
	OnOut             bool             `json:"onOut"`
	Banner            bool             `json:"banner"`
	MinStock          *decimal.Decimal `json:"minStock"`
	CoverageDays      *int             `json:"coverageDays"`
	BranchMinStock    *json.RawMessage `json:"branchMinStock"`
	ExpectedUpdatedAt *time.Time       `json:"expectedUpdatedAt"`
}

func (b lowStockBody) input() (app.LowStockInput, error) {
	in := app.LowStockInput{Enabled: b.Enabled, OnMin: b.OnMin, OnOut: b.OnOut, Banner: b.Banner, MinStock: b.MinStock,
		CoverageDays: b.CoverageDays, ExpectedUpdatedAt: b.ExpectedUpdatedAt}
	if b.BranchMinStock != nil {
		in.BranchMinStock.Set = true
		if string(*b.BranchMinStock) != "null" {
			var v decimal.Decimal
			if err := json.Unmarshal(*b.BranchMinStock, &v); err != nil {
				return in, fmt.Errorf("%w: el mínimo de la sucursal es un número", domain.ErrValidation)
			}
			in.BranchMinStock.Value = &v
		}
	}
	return in, nil
}

// GET /stock/{articles|products}/{id}/low-stock — la sección «Alertas».
func (h *Handlers) GetLowStock(itemType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idParam(r, "id")
		if err != nil {
			Error(w, err)
			return
		}
		v, err := h.notifications.LowStockConfig(r.Context(), itemType, id)
		if err != nil {
			Error(w, err)
			return
		}
		JSON(w, http.StatusOK, v)
	}
}

// PUT /stock/{articles|products}/{id}/low-stock[?preview=1] — guarda (o dice qué pasaría).
func (h *Handlers) PutLowStock(itemType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idParam(r, "id")
		if err != nil {
			Error(w, err)
			return
		}
		preview, err := previewParam(r)
		if err != nil {
			Error(w, err)
			return
		}
		var body lowStockBody
		if err := DecodeStrict(r, &body); err != nil {
			Error(w, err)
			return
		}
		in, err := body.input()
		if err != nil {
			Error(w, err)
			return
		}
		u, _ := userFrom(r.Context())
		res, err := h.notifications.ConfigureLowStock(r.Context(), itemType, id, in, u.ID, preview)
		if err != nil {
			Error(w, err)
			return
		}
		JSON(w, http.StatusOK, res)
	}
}

// POST /stock/articles/{id}/low-stock/asked — «No, por ahora»: la pregunta no vuelve.
func (h *Handlers) LowStockAsked(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.notifications.LowStockAsked(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /stock/low-stock — la tabla de «Alertas por lote».
func (h *Handlers) ListLowStock(w http.ResponseWriter, r *http.Request) {
	rows, err := h.notifications.ListLowStock(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": rows})
}

// POST /stock/low-stock/batch[?preview=1] — `{items: [{itemType, id, enabled, minStock}]}`.
func (h *Handlers) LowStockBatch(w http.ResponseWriter, r *http.Request) {
	preview, err := previewParam(r)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Items []app.LowStockBatchItem `json:"items"`
	}
	if err := DecodeStrict(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	res, err := h.notifications.ConfigureBatch(r.Context(), body.Items, u.ID, preview)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": res})
}

// GET /stock/shopping-list — la lista de compra de la sucursal.
func (h *Handlers) ShoppingList(w http.ResponseWriter, r *http.Request) {
	lines, err := h.notifications.ShoppingList(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": lines})
}

// GET /settings/notification-recipients — quién recibe los avisos de inventario por correo.
func (h *Handlers) NotificationRecipients(w http.ResponseWriter, r *http.Request) {
	v, err := h.notifications.Recipients(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// PUT /settings/notification-recipients — la configuración completa.
func (h *Handlers) SetNotificationRecipients(w http.ResponseWriter, r *http.Request) {
	var body app.RecipientsView
	if err := DecodeStrict(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.notifications.SetRecipients(r.Context(), body, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /stock/shopping-list/email — «Enviarme por correo». Con tope por persona: cada envío es un
// correo real y el botón está a un toque.
func (h *Handlers) EmailShoppingList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	key := fmt.Sprintf("lista-correo:%d", u.ID)
	if h.listEmails.blocked(r.Context(), key) {
		tooManyRequests(w, h.listEmails.retryAfter(r.Context(), key))
		return
	}
	h.listEmails.record(r.Context(), key)
	to, err := h.notifications.EmailShoppingList(r.Context(), u.CompanyID, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]string{"sentTo": to})
}
