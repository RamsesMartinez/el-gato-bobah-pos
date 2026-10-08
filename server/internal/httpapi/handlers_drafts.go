package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"uuid"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// La cuenta en captura (spec 030). Handlers finos: decodifican, arman el comando, llaman al servicio
// y avisan por SSE. Sin RequireRole: es el mismo gate que crear un pedido (contracts/api.md).

type draftLineBody struct {
	OpID       uuid.UUID              `json:"opId"`
	IntoLineID *uuid.UUID             `json:"intoLineId"`
	ProductID  int64                  `json:"productId"`
	Qty        decimal.Decimal        `json:"qty"`
	Modifiers  []domain.DraftModifier `json:"modifiers"`
	Notes      string                 `json:"notes"`
}

func (b draftLineBody) cmd() app.DraftLineCmd {
	return app.DraftLineCmd{OpID: b.OpID, IntoLineID: b.IntoLineID, ProductID: b.ProductID, Qty: b.Qty,
		Modifiers: b.Modifiers, Notes: b.Notes}
}

func lineCmds(bs []draftLineBody) []app.DraftLineCmd {
	out := make([]app.DraftLineCmd, len(bs))
	for i, b := range bs {
		out[i] = b.cmd()
	}
	return out
}

// draftHeaderBody es la cabecera tal como llega. Cada campo es crudo para distinguir AUSENTE (no
// cambia) de `null` (lo borra): con punteros los dos se verían iguales, y un descuento que se borra
// porque la pantalla no mandó el campo es dinero que cambia sin que nadie lo pidiera.
type draftHeaderBody map[string]json.RawMessage

func (b draftHeaderBody) patch() (app.DraftHeaderPatch, error) {
	var p app.DraftHeaderPatch
	for k, raw := range b {
		var err error
		switch k {
		case "expectedHeaderVersion":
			err = json.Unmarshal(raw, &p.ExpectedVersion)
		case "serviceType":
			p.ServiceType = new(string)
			err = json.Unmarshal(raw, p.ServiceType)
		case "customerName":
			p.CustomerName, err = fieldOf[string](raw)
		case "platformId":
			p.PlatformID, err = fieldOf[int16](raw)
		case "platformOrderRef":
			p.PlatformOrderRef, err = fieldOf[string](raw)
		case "deliveryFee":
			p.DeliveryFee = new(decimal.Decimal)
			err = json.Unmarshal(raw, p.DeliveryFee)
		case "discount":
			p.Discount, err = fieldOf[app.DraftDiscount](raw)
		default:
			// Un campo que no existe se rechaza: aceptarlo en silencio haría creer a quien lo manda que
			// cambió algo.
			err = fmt.Errorf("campo desconocido %q", k)
		}
		if err != nil {
			return p, domain.ErrValidation
		}
	}
	return p, nil
}

func fieldOf[T any](raw json.RawMessage) (app.Field[T], error) {
	f := app.Field[T]{Set: true}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return f, nil
	}
	f.Value = new(T)
	return f, json.Unmarshal(raw, f.Value)
}

func draftIDParam(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return id, domain.ErrValidation
	}
	return id, nil
}

// publishDraft avisa a las otras tabletas que una cuenta cambió. Solo el id y el estado, como los
// `order.updated`: cada tableta relee bajo su propia autorización.
func (h *Handlers) publishDraft(r *http.Request, v *app.DraftView) {
	u, _ := userFrom(r.Context())
	h.broker.Publish(u.CompanyID, realtime.Event{Type: "draft.updated", Data: map[string]any{
		"id": v.ID, "orderId": v.OrderID, "status": v.Status, "updatedAt": v.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}})
}

// POST /pos/drafts
func (h *Handlers) CreateDraft(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        uuid.UUID       `json:"id"`
		OrderID   *int64          `json:"orderId"`
		FolioName string          `json:"folioName"`
		Header    draftHeaderBody `json:"header"`
		Lines     []draftLineBody `json:"lines"`
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
	cmd := app.CreateDraftCmd{ID: body.ID, OrderID: body.OrderID, FolioName: body.FolioName,
		Lines: lineCmds(body.Lines), Actor: u.ID}
	if body.Header != nil {
		p, err := body.Header.patch()
		if err != nil {
			Error(w, err)
			return
		}
		cmd.Header = &p
	}
	v, created, err := h.drafts.Create(r.Context(), cmd)
	if err != nil {
		Error(w, err)
		return
	}
	h.publishDraft(r, v)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	JSON(w, status, v)
}

// GET /pos/drafts/{id}
func (h *Handlers) GetDraft(w http.ResponseWriter, r *http.Request) {
	id, err := draftIDParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	v, err := h.drafts.Get(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// POST /pos/drafts/{id}/lines
func (h *Handlers) AddDraftLine(w http.ResponseWriter, r *http.Request) {
	id, err := draftIDParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body draftLineBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	v, err := h.drafts.AddLine(r.Context(), id, body.cmd())
	if err != nil {
		Error(w, err)
		return
	}
	h.publishDraft(r, v)
	JSON(w, http.StatusOK, v)
}

// PATCH /pos/drafts/{id}/lines/{lineId}
func (h *Handlers) ChangeDraftLine(w http.ResponseWriter, r *http.Request) {
	id, err := draftIDParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	lineID, err := draftIDParam(r, "lineId")
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		ExpectedVersion *int32                  `json:"expectedVersion"`
		Qty             *decimal.Decimal        `json:"qty"`
		Modifiers       *[]domain.DraftModifier `json:"modifiers"`
		Notes           *string                 `json:"notes"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.ExpectedVersion == nil {
		Error(w, fmt.Errorf("%w: falta la versión del renglón", domain.ErrValidation))
		return
	}
	v, err := h.drafts.ChangeLine(r.Context(), app.ChangeDraftLineCmd{DraftID: id, LineID: lineID,
		ExpectedVersion: *body.ExpectedVersion, Qty: body.Qty, Modifiers: body.Modifiers, Notes: body.Notes})
	if err != nil {
		Error(w, err)
		return
	}
	h.publishDraft(r, v)
	JSON(w, http.StatusOK, v)
}

// DELETE /pos/drafts/{id}/lines/{lineId}?expectedVersion=N
func (h *Handlers) RemoveDraftLine(w http.ResponseWriter, r *http.Request) {
	id, err := draftIDParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	lineID, err := draftIDParam(r, "lineId")
	if err != nil {
		Error(w, err)
		return
	}
	// Obligatoria: un DELETE sin versión borraría lo que la otra tableta acaba de cambiar (D-5).
	version, err := strconv.ParseInt(r.URL.Query().Get("expectedVersion"), 10, 32)
	if err != nil {
		Error(w, fmt.Errorf("%w: falta la versión del renglón", domain.ErrValidation))
		return
	}
	v, err := h.drafts.RemoveLine(r.Context(), id, lineID, int32(version))
	if err != nil {
		Error(w, err)
		return
	}
	h.publishDraft(r, v)
	JSON(w, http.StatusOK, v)
}

// PATCH /pos/drafts/{id}
//
// Lleva el tope por usuario del descuento (router) y, cuando el descuento cambia, el MISMO evento de
// seguridad que `PUT /orders/{id}/discount`, con el anterior y el nuevo: el endpoint no pide rol, y
// el rastro es todo el control que queda (ver SetOrderDiscount).
func (h *Handlers) PatchDraft(w http.ResponseWriter, r *http.Request) {
	id, err := draftIDParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var body draftHeaderBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if _, ok := body["expectedHeaderVersion"]; !ok {
		Error(w, fmt.Errorf("%w: falta la versión de la cuenta", domain.ErrValidation))
		return
	}
	p, err := body.patch()
	if err != nil {
		Error(w, err)
		return
	}
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	res, err := h.drafts.PatchHeader(r.Context(), id, p, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	if res.DiscountChanged {
		logging.SecurityEvent(r.Context(), "draft_discount_set",
			"user_id", u.ID, "draft_id", id.String(),
			"descuento_anterior", res.DiscountBefore, "descuento_nuevo", res.DiscountAfter)
	}
	h.publishDraft(r, res.View)
	JSON(w, http.StatusOK, res.View)
}

// POST /pos/drafts/import
func (h *Handlers) ImportDrafts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Accounts []struct {
			ID        uuid.UUID       `json:"id"`
			FolioName string          `json:"folioName"`
			Header    draftHeaderBody `json:"header"`
			Lines     []draftLineBody `json:"lines"`
		} `json:"accounts"`
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
	accounts := make([]app.ImportAccount, 0, len(body.Accounts))
	for _, a := range body.Accounts {
		acc := app.ImportAccount{ID: a.ID, FolioName: a.FolioName, Lines: lineCmds(a.Lines)}
		if a.Header != nil {
			p, err := a.Header.patch()
			if err != nil {
				Error(w, err)
				return
			}
			acc.Header = &p
		}
		accounts = append(accounts, acc)
	}
	res, err := h.drafts.Import(r.Context(), accounts, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	for _, x := range res {
		if x.DraftID != nil {
			if v, err := h.drafts.Get(r.Context(), *x.DraftID); err == nil {
				h.publishDraft(r, v)
			}
		}
	}
	JSON(w, http.StatusOK, map[string]any{"results": res})
}

// GET /pos/accounts[?olderDebts=true]
//
// `olderDebts` acepta SOLO `true` o nada: un valor que no se entiende no cae a la ventana de 90 días
// en silencio — la hoja diría «no hay deudas de otros días» sobre un número que nadie pidió
// (constitución V).
func (h *Handlers) LiveAccounts(w http.ResponseWriter, r *http.Request) {
	olderDebts := false
	if v, ok := r.URL.Query()["olderDebts"]; ok {
		if len(v) != 1 || v[0] != "true" {
			Error(w, fmt.Errorf("%w: olderDebts solo admite true", domain.ErrValidation))
			return
		}
		olderDebts = true
	}
	res, err := h.accounts.Live(r.Context(), olderDebts)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, res)
}
