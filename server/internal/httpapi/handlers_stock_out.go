package httpapi

import (
	"fmt"
	"net/http"
	"slices"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GET /stock/out-reports/search?q= — lo que se puede reportar como «se acabó».
func (h *Handlers) SearchOutArticles(w http.ResponseWriter, r *http.Request) {
	items, err := h.stockOut.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /stock/out-reports/suggestions — mosaicos y lo reportado hoy.
func (h *Handlers) OutSuggestions(w http.ResponseWriter, r *http.Request) {
	v, err := h.stockOut.Suggestions(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// clientKey lee la clave de idempotencia que manda la pantalla (un uuid por toque).
func clientKey(s string) (uuid.UUID, error) {
	k, err := uuid.Parse(s)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("%w: clientKey debe ser un uuid", domain.ErrValidation)
	}
	return k, nil
}

// POST /stock/out-reports — «se acabó». No acepta cantidad: deja la existencia en 0. DecodeStrict y no
// Decode: un `quantity` ignorado en silencio haría creer a quien lo mandó que restó esa cantidad.
func (h *Handlers) ReportOut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ItemType  string `json:"itemType"`
		ID        int64  `json:"id"`
		ClientKey string `json:"clientKey"`
	}
	if err := DecodeStrict(r, &body); err != nil {
		Error(w, err)
		return
	}
	key, err := clientKey(body.ClientKey)
	if err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	res, created, err := h.stockOut.ReportOut(r.Context(), app.ReportOutCmd{ItemType: body.ItemType, ID: body.ID, ClientKey: key, Actor: u.ID})
	if err != nil {
		Error(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	JSON(w, status, res)
}

// POST /stock/out-reports/{id}/undo — deshace el propio, o cualquiera con el permiso de configurar.
func (h *Handlers) UndoOut(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	canConfigure := slices.Contains(h.permissionsOf(r.Context(), u), domain.PermInventoryConfigure)
	if err := h.stockOut.UndoOut(r.Context(), id, u.ID, canConfigure); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /stock/not-found — «No lo encuentro»: guarda lo que se escribió, sin tocar existencias.
func (h *Handlers) ReportNotFound(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text      string `json:"text"`
		ClientKey string `json:"clientKey"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	key, err := clientKey(body.ClientKey)
	if err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	res, created, err := h.stockOut.ReportNotFound(r.Context(), body.Text, key, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	JSON(w, status, res)
}
