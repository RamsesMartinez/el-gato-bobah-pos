package httpapi

import "net/http"

// GET /admin/web-sales/summary — cuántos productos se venden en la web y cuántos con precio propio.
func (h *Handlers) WebSalesSummary(w http.ResponseWriter, r *http.Request) {
	sum, err := h.webSales.Summary(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, sum)
}
