package app

import (
	"context"
	"fmt"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// DraftQuote es lo que costaría una cuenta que todavía no existe (spec 090): la vista previa de
// order_draft_create. Sale de priceDraft, la misma cuenta que hace la cuenta guardada.
type DraftQuote struct {
	ServiceType string `json:"serviceType"`
	PlatformID  *int16 `json:"platformId"`
	// MarkupPct es el margen de la plataforma (0 sin plataforma).
	MarkupPct     decimal.Decimal `json:"markupPct"`
	Lines         []QuoteLineView `json:"lines"`
	Subtotal      decimal.Decimal `json:"subtotal"`
	DiscountTotal decimal.Decimal `json:"discountTotal"`
	DeliveryFee   decimal.Decimal `json:"deliveryFee"`
	Total         decimal.Decimal `json:"total"`
	Unavailable   []string        `json:"unavailable"`
}

// QuoteLineView es un renglón cotizado con el origen de su precio.
type QuoteLineView struct {
	DraftLineView
	// PriceSource: domain.PriceFrom*; BasePrice es el precio del menú, antes de la plataforma.
	PriceSource string          `json:"priceSource"`
	BasePrice   decimal.Decimal `json:"basePrice"`
	Modifiers   []QuoteModView  `json:"modifiers"`
}

// QuoteModView es un extra cotizado con el origen de su precio.
type QuoteModView struct {
	DraftModView
	PriceSource string          `json:"priceSource"`
	BasePrice   decimal.Decimal `json:"basePrice"`
}

// Quote cotiza una cuenta sin crearla: valida cabecera y renglones con las mismas reglas que Create
// (lo que crear rechaza, cotizar también) y no escribe nada.
func (s *DraftsService) Quote(ctx context.Context, header *DraftHeaderPatch, lines []DraftLineCmd, actor int64) (*DraftQuote, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: la cuenta nace con su primer producto", domain.ErrValidation)
	}
	if len(lines) > domain.MaxDraftLines {
		return nil, tooManyLines()
	}
	q := s.store.QC(ctx)
	h, err := newHeader(ctx, q, header, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]db.OrderDraftLine, 0, len(lines))
	for i := range lines {
		c := &lines[i]
		if c.IntoLineID != nil {
			return nil, fmt.Errorf("%w: una cuenta nueva no tiene renglones a los cuales sumar", domain.ErrValidation)
		}
		if c.OpID == uuid.Nil() {
			c.OpID = uuid.New()
		}
		if err := validLineCmd(c); err != nil {
			return nil, err
		}
		if err := checkLineSells(ctx, q, domain.DraftLineInput{ProductID: c.ProductID, Qty: c.Qty, Modifiers: c.Modifiers, Notes: c.Notes}); err != nil {
			return nil, err
		}
		rows = append(rows, db.OrderDraftLine{ID: c.OpID, ProductID: c.ProductID, Qty: c.Qty,
			Modifiers: modsJSON(c.Modifiers), Notes: notesPtr(c.Notes)})
	}
	lista, err := listaDePreciosQ(ctx, q, h.platformID)
	if err != nil {
		return nil, err
	}
	t, err := priceDraft(ctx, q, lista, rows, h)
	if err != nil {
		return nil, err
	}
	if t.discountOver {
		// Create lo rechaza al escribir (checkDraftDiscount): la cotización dice lo mismo.
		if _, err := domain.ResolverDescuento(t.subtotal, h.discountAmount, h.discountPercent); err != nil {
			return nil, err
		}
	}
	return quoteOf(ctx, q, lista, h, rows, t)
}

// quoteOf agrega a los renglones valuados el precio del menú y de dónde salió el que se cobra.
func quoteOf(ctx context.Context, q *db.Queries, lista listaDePrecios, h draftHeader, rows []db.OrderDraftLine,
	t draftTotals) (*DraftQuote, error) {
	var prodIDs, optIDs []int64
	for _, r := range rows {
		prodIDs = append(prodIDs, r.ProductID)
		for _, m := range modsOf(r.Modifiers) {
			optIDs = append(optIDs, m.OptionID)
		}
	}
	baseProduct := map[int64]decimal.Decimal{}
	baseOption := map[int64]decimal.Decimal{}
	prods, err := q.GetPricedProducts(ctx, prodIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range prods {
		baseProduct[p.ID] = p.Price
	}
	if len(optIDs) > 0 {
		opts, err := q.GetPricedOptions(ctx, optIDs)
		if err != nil {
			return nil, err
		}
		for _, o := range opts {
			baseOption[o.ID] = o.PriceDelta
		}
	}
	platform := h.platformID != nil
	out := &DraftQuote{ServiceType: h.serviceType, PlatformID: h.platformID, MarkupPct: lista.margen,
		Lines: make([]QuoteLineView, 0, len(t.lines)), Subtotal: t.subtotal, DiscountTotal: t.discount,
		DeliveryFee: h.deliveryFee, Total: t.total, Unavailable: t.unavailable}
	for _, l := range t.lines {
		ql := QuoteLineView{DraftLineView: l, BasePrice: baseProduct[l.ProductID],
			PriceSource: domain.PlatformPriceSource(platform, lista.margen, lista.producto[l.ProductID]),
			Modifiers:   make([]QuoteModView, 0, len(l.Modifiers))}
		for _, m := range l.Modifiers {
			ql.Modifiers = append(ql.Modifiers, QuoteModView{DraftModView: m, BasePrice: baseOption[m.OptionID],
				PriceSource: domain.PlatformPriceSource(platform, lista.margen, lista.opcion[m.OptionID])})
		}
		out.Lines = append(out.Lines, ql)
	}
	return out, nil
}
