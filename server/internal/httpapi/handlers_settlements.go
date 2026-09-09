package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// La liquidación de un pedido de plataforma: lo que dice su documento de pago.
//
// Todo lo de aquí exige rol de administración. Es dinero que NO pasó por la caja: no lo captura
// quien vende, lo captura quien concilia el estado de cuenta días después.

// Las tres cifras del documento que NO pueden faltar van como puntero, por el mismo motivo por el
// que `commissionPct` lo es: con un `decimal.Decimal` a secas, "no lo mandé" y "el documento dice
// cero" llegan iguales, y un cuerpo incompleto quedaría registrado como que la plataforma no cobró
// nada. Eso baja el "se quedó la plataforma" del periodo por un error de captura, no por un hecho.
//
// Los tres que SÍ pueden faltar —descuentos y retenciones— valen cero de verdad cuando no vienen:
// un pedido sin promoción no tiene descuento que declarar.
type settlementBody struct {
	ReportedGross    *decimal.Decimal `json:"reportedGross"`
	CommissionAmount *decimal.Decimal `json:"commissionAmount"`
	CommissionPct    *decimal.Decimal `json:"commissionPct"`
	DiscountTotal    decimal.Decimal  `json:"discountTotal"`
	DiscountPlatform decimal.Decimal  `json:"discountPlatform"`
	Withholdings     decimal.Decimal  `json:"withholdings"`
	NetAmount        *decimal.Decimal `json:"netAmount"`
	PayoutReference  string           `json:"payoutReference"`
	DocumentRef      string           `json:"documentRef"`
}

// exigidos rechaza el cuerpo incompleto ANTES de tocar la base.
func (b settlementBody) exigidos() error {
	for nombre, v := range map[string]*decimal.Decimal{
		"el importe que reporta la plataforma": b.ReportedGross,
		"la comisión":                          b.CommissionAmount,
		"el neto depositado":                   b.NetAmount,
	} {
		if v == nil {
			return fmt.Errorf("%w: falta %s. Si el documento dice cero, mándalo como 0 — no es lo "+
				"mismo que no decirlo", domain.ErrValidation, nombre)
		}
	}
	return nil
}

// PUT /orders/{id}/settlement — registra o REEMPLAZA la liquidación.
func (h *Handlers) UpsertSettlement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	var body settlementBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if err := body.exigidos(); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	view, err := h.settlements.Upsert(r.Context(), id, domain.Settlement{
		ReportedGross: *body.ReportedGross, CommissionAmount: *body.CommissionAmount,
		CommissionPct: body.CommissionPct, DiscountTotal: body.DiscountTotal,
		DiscountPlatform: body.DiscountPlatform, Withholdings: body.Withholdings,
		NetAmount: *body.NetAmount, PayoutReference: body.PayoutReference, DocumentRef: body.DocumentRef,
	}, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	// Sin importes en el log: son datos de negocio que ya viven en la fila, y el log no es donde se
	// consultan.
	logging.SecurityEvent(r.Context(), "platform_settlement_set", "user_id", u.ID, "order_id", id)
	JSON(w, http.StatusOK, view)
}

// GET /orders/{id}/settlement
//
// 404 cuando no hay liquidación, y es la respuesta CORRECTA: "todavía no llega el documento" y "el
// documento dice cero" no son lo mismo. Una liquidación de ceros es un 200 con ceros.
func (h *Handlers) GetSettlement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	view, err := h.settlements.Get(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, view)
}

// GET /platform-settlements/summary — las tres cifras del periodo.
//
// Reusa el filtro de la pantalla de Ventas para que el rango se resuelva con las MISMAS reglas: un
// preset desconocido o unas fechas que el preset no usa se rechazan igual aquí que allá.
//
// De ese filtro este endpoint usa SOLO EL RANGO. `status`, `serviceType`, `folioPlataforma`, `folio`,
// `sort` y `dir` se validan y se ignoran, y se dice aquí porque un consumidor de la API asumiría
// razonablemente que `serviceType` acota el corte de plataformas — y no lo hace. Las tres cifras son
// del periodo completo: acotarlas por tipo de venta no significa nada cuando todo pedido de
// plataforma es a domicilio por construcción.
func (h *Handlers) PlatformSettlementSummary(w http.ResponseWriter, r *http.Request) {
	f, err := h.filtroDeVentas(r)
	if err != nil {
		Error(w, err)
		return
	}
	view, err := h.settlements.Summary(r.Context(), f)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, view)
}
