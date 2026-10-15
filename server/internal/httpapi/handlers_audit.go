package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GET /orders/{id}/history
//
// Quién hizo qué con un pedido, en orden, con IP y aparato. Solo con `audit.view`: el router lo
// exige, y la pantalla ni siquiera ofrece la pestaña sin el permiso.
func (h *Handlers) OrderHistory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		Error(w, fmt.Errorf("%w: pedido inválido", domain.ErrValidation))
		return
	}
	evs, err := h.audit.OrderHistory(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, evs)
}

// GET /admin/audit?from&to&actorUserId&action&group&deviceKey&page&size
//
// La bitácora general. Un parámetro presente y mal formado se rechaza; el default (hoy, página 0,
// 25 por página) es solo para el ausente.
func (h *Handlers) ListAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	today := h.backoffice.Now()
	from, err := parseDate(q.Get("from"), today)
	if err != nil {
		Error(w, err)
		return
	}
	to, err := parseDate(q.Get("to"), from)
	if err != nil {
		Error(w, err)
		return
	}
	f := domain.AuditFilter{From: from, To: to, Action: q.Get("action"), Group: domain.AuditGroup(q.Get("group")),
		DeviceKey: q.Get("deviceKey"), Size: 25}
	for _, p := range []struct {
		name string
		dst  *int
	}{{"page", &f.Page}, {"size", &f.Size}} {
		if v := q.Get(p.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				Error(w, fmt.Errorf("%w: %s inválido", domain.ErrValidation, p.name))
				return
			}
			*p.dst = n
		}
	}
	if v := q.Get("actorUserId"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			Error(w, fmt.Errorf("%w: persona inválida", domain.ErrValidation))
			return
		}
		f.ActorUserID = &id
	}
	page, err := h.audit.List(r.Context(), f)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, page)
}

// GET /admin/audit/actors — las personas para el filtro de la bitácora.
func (h *Handlers) AuditActors(w http.ResponseWriter, r *http.Request) {
	out, err := h.audit.Actors(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}
