package httpapi

import (
	"fmt"
	"net/http"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// POST /stock/transfers — mover mercancía entre almacenes de la sucursal (spec 094). 201 con el
// traspaso nuevo; 200 con el mismo si es un reintento con la misma clave y el mismo cuerpo.
func (h *Handlers) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientKey       string `json:"clientKey"`
		FromWarehouseID int64  `json:"fromWarehouseId"`
		ToWarehouseID   int64  `json:"toWarehouseId"`
		Note            string `json:"note"`
		ConfirmNegative bool   `json:"confirmNegative"`
		Lines           []struct {
			ItemType     string          `json:"itemType"`
			IngredientID *int64          `json:"ingredientId"`
			ProductID    *int64          `json:"productId"`
			Qty          decimal.Decimal `json:"qty"`
		} `json:"lines"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	key, err := uuid.Parse(body.ClientKey)
	if err != nil {
		Error(w, fmt.Errorf("%w: la clave del traspaso no es válida", domain.ErrValidation))
		return
	}
	in := app.TransferInput{ClientKey: key, FromWarehouseID: body.FromWarehouseID, ToWarehouseID: body.ToWarehouseID,
		Note: body.Note, ConfirmNegative: body.ConfirmNegative}
	for _, l := range body.Lines {
		var id int64
		switch {
		case l.ItemType == "ingrediente" && l.IngredientID != nil && l.ProductID == nil:
			id = *l.IngredientID
		case l.ItemType == "producto" && l.ProductID != nil && l.IngredientID == nil:
			id = *l.ProductID
		default:
			Error(w, fmt.Errorf("%w: cada renglón lleva un insumo o un producto, según su tipo", domain.ErrValidation))
			return
		}
		in.Lines = append(in.Lines, domain.TransferLine{ItemType: l.ItemType, ID: id, Qty: l.Qty})
	}
	u, _ := userFrom(r.Context())
	res, err := h.backoffice.CreateTransfer(r.Context(), in, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	status := http.StatusCreated
	if res.Replay {
		status = http.StatusOK
	}
	JSON(w, status, map[string]int64{"id": res.ID})
}

// GET /stock/transfers — los traspasos recientes de la sucursal (`?limit=`, 50 por omisión).
func (h *Handlers) ListTransfers(w http.ResponseWriter, r *http.Request) {
	limit, err := limiteDeQuery(r.URL.Query(), 50)
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.ListTransfers(r.Context(), limit)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"items": out})
}
