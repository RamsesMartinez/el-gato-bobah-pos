package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// Editar un gasto ya guardado (spec 053). Las dos rutas son la misma operación: `?preview=1` devuelve
// los efectos sin escribir; sin él, confirma con la huella de la última vista previa.

// previewParam acepta solo `1` o ausente: un `preview=true` mal escrito que confirmara en silencio
// escribiría lo que la persona solo quería ver (constitución V).
func previewParam(r *http.Request) (bool, error) {
	v, ok := r.URL.Query()["preview"]
	if !ok {
		return false, nil
	}
	if len(v) == 1 && v[0] == "1" {
		return true, nil
	}
	return false, fmt.Errorf("%w: preview solo acepta 1", domain.ErrValidation)
}

// nullableID distingue «no viene» (no cambia) de «null» (se quita) en un campo del encabezado.
type nullableID struct {
	Set   bool
	Value *int64
}

func (n *nullableID) UnmarshalJSON(b []byte) error {
	n.Set = true
	if bytes.Equal(b, []byte("null")) {
		n.Value = nil
		return nil
	}
	var v int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

type editLineBody struct {
	ItemType       string          `json:"itemType"`
	IngredientID   *int64          `json:"ingredientId"`
	ProductID      *int64          `json:"productId"`
	Description    string          `json:"description"`
	Quantity       decimal.Decimal `json:"quantity"`
	PresentationID *int64          `json:"presentationId"`
	UnitID         *int16          `json:"unitId"`
	Amount         decimal.Decimal `json:"amount"`
	IsGift         bool            `json:"isGift"`
	ArrivedOn      string          `json:"arrivedOn"`
}

// POST /expenses/{id}/edit[?preview=1] — encabezado, renglones agregados y quitados.
func (h *Handlers) EditExpense(w http.ResponseWriter, r *http.Request) {
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
	var body struct {
		Header *struct {
			SupplierID  nullableID `json:"supplierId"`
			CategoryID  *int64     `json:"categoryId"`
			Description *string    `json:"description"`
			ExpenseDate *string    `json:"expenseDate"`
		} `json:"header"`
		Add         []editLineBody    `json:"add"`
		Remove      []int64           `json:"remove"`
		Decisions   map[string]string `json:"decisions"`
		Fingerprint string            `json:"fingerprint"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	in := app.EditInput{Remove: body.Remove, Decisions: body.Decisions, Fingerprint: body.Fingerprint}
	if body.Header != nil {
		in.Header = &app.EditHeaderInput{SupplierSet: body.Header.SupplierID.Set, SupplierID: body.Header.SupplierID.Value,
			CategoryID: body.Header.CategoryID, Description: body.Header.Description, ExpenseDate: body.Header.ExpenseDate}
	}
	for _, a := range body.Add {
		in.Add = append(in.Add, app.EditLineInput{ItemType: a.ItemType, IngredientID: a.IngredientID, ProductID: a.ProductID,
			Description: a.Description, Quantity: a.Quantity, PresentationID: a.PresentationID, UnitID: a.UnitID,
			Amount: a.Amount, IsGift: a.IsGift, ArrivedOn: a.ArrivedOn})
	}
	u, _ := userFrom(r.Context())
	// Agregar algo que «ya llegó» o quitar lo que ya sumó al almacén mueven existencias: además del
	// permiso de editar, el de recibir. Depende del estado del renglón, así que lo decide el servicio;
	// aquí se le dice si la persona lo tiene, con el mismo resolvedor que el router.
	in.CanReceive = slices.Contains(h.permissions(u.Role), domain.PermInventoryReceive)
	res, err := h.backoffice.EditExpense(r.Context(), id, in, u.ID, preview)
	if err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			logging.SecurityEvent(r.Context(), "forbidden", "user_id", u.ID, "role", string(u.Role),
				"permission", string(domain.PermInventoryReceive), "method", r.Method, "path", r.URL.Path, "ip", clientIP(r))
		}
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, res)
}

// POST /expenses/{id}/deliveries[?preview=1] — una entrega con su fecha.
func (h *Handlers) DeliverExpense(w http.ResponseWriter, r *http.Request) {
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
	var body struct {
		On    string `json:"on"`
		Lines []struct {
			ItemID      int64            `json:"itemId"`
			Qty         decimal.Decimal  `json:"qty"`
			On          string           `json:"on"`
			Shortfall   string           `json:"shortfall"`
			ExtraAmount *decimal.Decimal `json:"extraAmount"`
		} `json:"lines"`
		Decisions   map[string]string `json:"decisions"`
		Fingerprint string            `json:"fingerprint"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	in := app.DeliveryInput{On: body.On, Decisions: body.Decisions, Fingerprint: body.Fingerprint}
	for _, l := range body.Lines {
		in.Lines = append(in.Lines, app.DeliveryLineInput{ItemID: l.ItemID, Qty: l.Qty, On: l.On, Shortfall: l.Shortfall,
			ExtraAmount: l.ExtraAmount})
	}
	u, _ := userFrom(r.Context())
	res, err := h.backoffice.DeliverExpense(r.Context(), id, in, u.ID, preview)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, res)
}
