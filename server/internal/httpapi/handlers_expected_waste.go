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
	return parseExpectedWastePct(body.Pct)
}

// decodeArticleExpectedWaste es decodeExpectedWastePct con el motivo opcional que manda el MCP
// (spec 099). Solo la merma del artículo lo acepta: es la única que el MCP cambia.
func decodeArticleExpectedWaste(r *http.Request) (*decimal.Decimal, string, error) {
	var body struct {
		Pct    json.RawMessage `json:"pct"`
		Reason string          `json:"reason"`
	}
	if err := DecodeStrict(r, &body); err != nil {
		return nil, "", err
	}
	pct, err := parseExpectedWastePct(body.Pct)
	return pct, body.Reason, err
}

func parseExpectedWastePct(raw json.RawMessage) (*decimal.Decimal, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: falta el porcentaje", domain.ErrValidation)
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var v decimal.Decimal
	if err := json.Unmarshal(raw, &v); err != nil || raw[0] == '"' {
		return nil, fmt.Errorf("%w: el porcentaje es un número", domain.ErrValidation)
	}
	return &v, nil
}

// parsePreviewPct lee el % de la vista previa de la URL: un número, o `none` para «sin captura».
func parsePreviewPct(r *http.Request) (*decimal.Decimal, error) {
	raw := r.URL.Query().Get("pct")
	if raw == "none" {
		return nil, nil
	}
	v, err := decimal.NewFromString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: pct es un número de 0 a menos de 100, o none (sin captura)", domain.ErrValidation)
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

// PUT /stock/articles/{id}/expected-waste — menu.configure. La merma de un artículo, con el motivo
// opcional que manda el MCP (spec 099).
func (h *Handlers) PutArticleExpectedWaste(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	pct, reason, err := decodeArticleExpectedWaste(r)
	if err != nil {
		Error(w, err)
		return
	}
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if err := h.costing.SetArticleExpectedWaste(r.Context(), id, pct, u.ID, reason); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"pct": pct})
}

// GET /stock/articles/{id}/expected-waste/preview?pct= — menu.configure. Qué le pasaría al costo de
// lo que usa el artículo con otra merma, sin guardar nada (la vista previa del MCP, spec 099).
func (h *Handlers) ArticleExpectedWastePreview(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	pct, err := parsePreviewPct(r)
	if err != nil {
		Error(w, err)
		return
	}
	p, err := h.costing.ArticleExpectedWastePreview(r.Context(), id, pct)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, p)
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

// PutExpectedWasteDeducts es PUT /settings/expected-waste-deducts (inventory.configure): enciende o
// apaga que la merma esperada descuente del almacén. La llave es obligatoria.
func (h *Handlers) PutExpectedWasteDeducts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Deducts *bool `json:"deducts"`
	}
	if err := DecodeStrict(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.Deducts == nil {
		Error(w, fmt.Errorf("%w: falta si descuenta o no", domain.ErrValidation))
		return
	}
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	if err := h.costing.SetExpectedWasteDeducts(r.Context(), *body.Deducts, u.ID); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"deducts": *body.Deducts})
}
