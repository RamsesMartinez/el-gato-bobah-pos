package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// decodeExpectedWastePct lee `{"pct": número | null}`. La llave es obligatoria: un cuerpo sin ella no
// significa «sin captura», es un error (spec 096, contracts/api.md).
func decodeExpectedWastePct(r *http.Request) (*decimal.Decimal, error) {
	var body struct {
		Pct json.RawMessage `json:"pct"`
	}
	if err := DecodeStrict(r, &body); err != nil {
		return nil, err
	}
	if len(body.Pct) == 0 {
		return nil, fmt.Errorf("%w: falta el porcentaje", domain.ErrValidation)
	}
	if string(body.Pct) == "null" {
		return nil, nil
	}
	var v decimal.Decimal
	if err := json.Unmarshal(body.Pct, &v); err != nil || body.Pct[0] == '"' {
		return nil, fmt.Errorf("%w: el porcentaje es un número", domain.ErrValidation)
	}
	return &v, nil
}

// PUT /settings/expected-waste — menu.configure. La merma general de la empresa.
func (h *Handlers) PutGeneralExpectedWaste(w http.ResponseWriter, r *http.Request) {
	h.putExpectedWaste(w, r, func(pct *decimal.Decimal, user int64) error {
		return h.costing.SetGeneralExpectedWaste(r.Context(), pct, user)
	})
}

// PUT /admin/products/{id}/expected-waste — menu.configure. La merma propia de un producto.
func (h *Handlers) PutProductExpectedWaste(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	h.putExpectedWaste(w, r, func(pct *decimal.Decimal, user int64) error {
		return h.costing.SetProductExpectedWaste(r.Context(), id, pct, user)
	})
}

// PUT /stock/articles/{id}/expected-waste — menu.configure. La merma de un artículo.
func (h *Handlers) PutArticleExpectedWaste(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	h.putExpectedWaste(w, r, func(pct *decimal.Decimal, user int64) error {
		return h.costing.SetArticleExpectedWaste(r.Context(), id, pct, user)
	})
}

func (h *Handlers) putExpectedWaste(w http.ResponseWriter, r *http.Request, set func(*decimal.Decimal, int64) error) {
	pct, err := decodeExpectedWastePct(r)
	if err != nil {
		Error(w, err)
		return
	}
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if err := set(pct, u.ID); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"pct": pct})
}

// GET /stock/articles/{id}/real-waste — inventory.view. La merma real de 90 días por almacén.
func (h *Handlers) ArticleRealWaste(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	items, err := h.costing.ArticleRealWaste(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"days": 90, "items": items})
}
