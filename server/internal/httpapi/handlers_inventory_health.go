package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// healthFilterFrom lee los parámetros de «Salud del inventario». Uno presente y mal formado se
// rechaza; el valor por omisión es solo para el ausente (constitución V).
func healthFilterFrom(q url.Values, today func() domain.Range) (domain.HealthFilter, error) {
	f := domain.HealthFilter{Section: domain.HealthSection(q.Get("section")), Limit: domain.HealthPageSize}
	def := today()
	from, err := parseDate(q.Get("from"), def.From)
	if err != nil {
		return f, err
	}
	to, err := parseDate(q.Get("to"), def.To)
	if err != nil {
		return f, err
	}
	f.Range = domain.Range{From: from, To: to}
	if v := q.Get("noPurchasesSince"); v != "" {
		d, err := parseDate(v, def.From)
		if err != nil {
			return f, err
		}
		f.NoPurchasesSince = &d
	}
	for name, dst := range map[string]*int32{"limit": &f.Limit, "offset": &f.Offset} {
		if v := q.Get(name); v != "" {
			n, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				return f, fmt.Errorf("%w: %s no es un número", domain.ErrValidation, name)
			}
			*dst = int32(n)
		}
	}
	return f, f.Validate()
}

// GetInventoryHealth es la pantalla de Almacén › Salud.
func (h *Handlers) GetInventoryHealth(w http.ResponseWriter, r *http.Request) {
	f, err := healthFilterFrom(r.URL.Query(), func() domain.Range { return domain.DefaultHealthRange(h.health.Today(r.Context())) })
	if err != nil {
		Error(w, err)
		return
	}
	v, err := h.health.Report(r.Context(), f)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}
