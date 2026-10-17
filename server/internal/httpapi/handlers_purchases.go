package httpapi

import (
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// stockLineBody es un renglón de almacén de «Corregir lo que llegó» (spec 038). Las cantidades van
// en la unidad del renglón: el empaque si trae `presentationId`, si no la unidad.
type stockLineBody struct {
	ID             *int64           `json:"id"`
	SeenApplied    *decimal.Decimal `json:"seenApplied"`
	ItemType       string           `json:"itemType"`
	IngredientID   *int64           `json:"ingredientId"`
	ProductID      *int64           `json:"productId"`
	Description    string           `json:"description"`
	Quantity       decimal.Decimal  `json:"quantity"`
	Received       decimal.Decimal  `json:"received"`
	NotComing      decimal.Decimal  `json:"notComing"`
	PresentationID *int64           `json:"presentationId"`
	UnitID         *int16           `json:"unitId"`
	Amount         decimal.Decimal  `json:"amount"`
	IsGift         bool             `json:"isGift"`
}

// PUT /expenses/{id}/stock-lines — permiso inventory.receive. El conjunto deseado de renglones de
// almacén: el servidor mueve el almacén solo por la diferencia.
func (h *Handlers) SaveStockLines(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Lines []stockLineBody `json:"lines"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	lines := make([]app.StockLineInput, len(body.Lines))
	for i, l := range body.Lines {
		lines[i] = app.StockLineInput{
			ID: l.ID, SeenApplied: l.SeenApplied, ItemType: l.ItemType, IngredientID: l.IngredientID,
			ProductID: l.ProductID, Description: l.Description, Quantity: l.Quantity, Received: l.Received,
			NotComing: l.NotComing, PresentationID: l.PresentationID, UnitID: l.UnitID, Amount: l.Amount,
			IsGift: l.IsGift,
		}
	}
	v, err := h.backoffice.SaveStockLines(r.Context(), id, lines, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// PUT /settings/inventory — permiso inventory.configure. Método de costeo y palabra del almacén.
func (h *Handlers) UpdateInventorySettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CostingMethod  string `json:"costingMethod"`
		StockItemLabel string `json:"stockItemLabel"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	bs, err := h.settings.SetInventory(r.Context(), body.CostingMethod, body.StockItemLabel, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, bs)
}

// GET /stock/pending-receipts — permiso inventory.receive. Siempre arreglo, aunque vacío.
func (h *Handlers) PendingReceipts(w http.ResponseWriter, r *http.Request) {
	list, err := h.backoffice.PendingReceipts(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, list)
}

// POST /expenses/{id}/items/{itemId}/receive — lo que llegó de un renglón, en su unidad.
func (h *Handlers) ReceiveItem(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	itemID, err := idParam(r, "itemId")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Qty         decimal.Decimal  `json:"qty"`
		SeenApplied decimal.Decimal  `json:"seenApplied"`
		ExtraAmount *decimal.Decimal `json:"extraAmount"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.backoffice.ReceiveItem(r.Context(), id, itemID, app.ReceiveItemInput{
		Qty: body.Qty, SeenApplied: body.SeenApplied, ExtraAmount: body.ExtraAmount,
	}, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /expenses/{id}/items/{itemId}/not-coming — «Ya no va a llegar», hasta lo que falta.
func (h *Handlers) MarkNotComing(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	itemID, err := idParam(r, "itemId")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Qty decimal.Decimal `json:"qty"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.backoffice.MarkNotComing(r.Context(), id, itemID, body.Qty, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /expenses/{id}/items — «Llegó algo más». Llega ya recibido.
func (h *Handlers) AddArrivedItem(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		ItemType       string          `json:"itemType"`
		IngredientID   *int64          `json:"ingredientId"`
		ProductID      *int64          `json:"productId"`
		Description    string          `json:"description"`
		Quantity       decimal.Decimal `json:"quantity"`
		PresentationID *int64          `json:"presentationId"`
		UnitID         *int16          `json:"unitId"`
		Amount         decimal.Decimal `json:"amount"`
		IsGift         bool            `json:"isGift"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.backoffice.AddArrivedItem(r.Context(), id, app.ArrivedItemInput{
		ItemType: body.ItemType, IngredientID: body.IngredientID, ProductID: body.ProductID,
		Description: body.Description, Quantity: body.Quantity, PresentationID: body.PresentationID,
		UnitID: body.UnitID, Amount: body.Amount, IsGift: body.IsGift,
	}, u.ID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
