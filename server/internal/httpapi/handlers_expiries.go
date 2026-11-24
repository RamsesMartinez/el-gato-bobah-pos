package httpapi

import "net/http"

// GET /stock/expiries?warehouseId= — Caducidades (spec 098): los lotes con su proyección. Sin
// warehouseId, el de venta; `all`, todos los de la sucursal.
func (h *Handlers) Expiries(w http.ResponseWriter, r *http.Request) {
	sel, err := warehouseFromQuery(r, true)
	if err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.Expiries(r.Context(), sel)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// GET /stock/expiries/summary?warehouseId= — lo caducado y lo que caduca en 7 días, para el aviso de
// Existencias.
func (h *Handlers) ExpirySummary(w http.ResponseWriter, r *http.Request) {
	sel, err := warehouseFromQuery(r, true)
	if err != nil {
		Error(w, err)
		return
	}
	v, err := h.backoffice.ExpirySummary(r.Context(), sel)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}
