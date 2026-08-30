package httpapi

import (
	"net/http"
	"strconv"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// Precios por plataforma digital: las excepciones que el negocio captura a mano.
//
// Accesibles a cualquier rol que pueda vender, incluido cajero. Es una decisión de agilidad: el
// pedido de la plataforma ya llegó y hay que imprimirlo, así que detenerlo para buscar un gerente
// cuesta más de lo que protege.
//
// La mitigación NO puede ser solo `updated_by`: el DELETE se lleva la fila entera —precio, quién y
// cuándo— y el upsert pisa al anterior, así que el mismo rol borra su propio rastro. El ataque
// concreto: poner un producto en $1 para Rappi, cobrar en efectivo lo que la plataforma facturó, y
// quitar la excepción; el corte cuadra contra lo que el sistema creyó cobrar y no queda un renglón
// que diga que el precio se movió.
//
// Por eso cada escritura deja un evento de seguridad con clave estable, que vive fuera de la tabla
// y el cajero no puede tocar. Es el mismo criterio con el que se registra el cambio de
// auto_declare, que mueve mucho menos dinero que esto.

type precioProductoBody struct {
	ProductID  int64           `json:"productId"`
	PlatformID int16           `json:"platformId"`
	Price      decimal.Decimal `json:"price"`
}

// PUT /platform-prices/product
func (h *Handlers) SetProductPlatformPrice(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var body precioProductoBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.ProductID == 0 || body.PlatformID == 0 {
		Error(w, domain.ErrValidation)
		return
	}
	if err := h.platformPrices.SetProductPrice(r.Context(), body.ProductID, body.PlatformID, body.Price, u.ID); err != nil {
		Error(w, err)
		return
	}
	logging.SecurityEvent(r.Context(), "platform_price_set",
		"user_id", u.ID, "product_id", body.ProductID, "platform_id", body.PlatformID,
		"price", domain.Round2(body.Price).String(), "ip", clientIP(r))
	h.invalidarMenu(r, u.CompanyID)
	JSON(w, http.StatusOK, map[string]any{"productId": body.ProductID, "platformId": body.PlatformID, "price": domain.Round2(body.Price)})
}

// DELETE /platform-prices/product?productId=&platformId=
func (h *Handlers) DeleteProductPlatformPrice(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	productID, platformID, err := idsDePrecio(r, "productId")
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.platformPrices.DeleteProductPrice(r.Context(), productID, int16(platformID)); err != nil {
		Error(w, err)
		return
	}
	logging.SecurityEvent(r.Context(), "platform_price_removed",
		"user_id", u.ID, "product_id", productID, "platform_id", platformID, "ip", clientIP(r))
	h.invalidarMenu(r, u.CompanyID)
	// 204 también cuando no había fila: borrar lo que no existe deja el mundo como se pidió.
	JSON(w, http.StatusNoContent, nil)
}

type deltaOpcionBody struct {
	OptionID   int64           `json:"optionId"`
	PlatformID int16           `json:"platformId"`
	PriceDelta decimal.Decimal `json:"priceDelta"`
}

// PUT /platform-prices/modifier-option
func (h *Handlers) SetOptionPlatformPrice(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	var body deltaOpcionBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.OptionID == 0 || body.PlatformID == 0 {
		Error(w, domain.ErrValidation)
		return
	}
	if err := h.platformPrices.SetOptionDelta(r.Context(), body.OptionID, body.PlatformID, body.PriceDelta, u.ID); err != nil {
		Error(w, err)
		return
	}
	logging.SecurityEvent(r.Context(), "platform_price_set",
		"user_id", u.ID, "option_id", body.OptionID, "platform_id", body.PlatformID,
		"price", domain.Round2(body.PriceDelta).String(), "ip", clientIP(r))
	h.invalidarMenu(r, u.CompanyID)
	JSON(w, http.StatusOK, map[string]any{"optionId": body.OptionID, "platformId": body.PlatformID, "priceDelta": domain.Round2(body.PriceDelta)})
}

// DELETE /platform-prices/modifier-option?optionId=&platformId=
func (h *Handlers) DeleteOptionPlatformPrice(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	optionID, platformID, err := idsDePrecio(r, "optionId")
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.platformPrices.DeleteOptionDelta(r.Context(), optionID, int16(platformID)); err != nil {
		Error(w, err)
		return
	}
	logging.SecurityEvent(r.Context(), "platform_price_removed",
		"user_id", u.ID, "option_id", optionID, "platform_id", platformID, "ip", clientIP(r))
	h.invalidarMenu(r, u.CompanyID)
	JSON(w, http.StatusNoContent, nil)
}

// invalidarMenu tira el menú cacheado y avisa a las demás pantallas. El TTL es de 24 horas: sin
// esto, otra tablet seguiría mostrando el precio viejo un día entero mientras el servidor cobra el
// nuevo — total impreso distinto del cobrado, justo lo que este diseño quiere evitar.
func (h *Handlers) invalidarMenu(r *http.Request, companyID int64) {
	ctx := r.Context()
	h.menuCache.Invalidate(ctx, companyID)
	h.broker.Publish(companyID, realtime.Event{Type: "menu.updated"})
}

// idsDePrecio lee el par de ids del query string. `nombre` es el parámetro propio de la ruta
// (productId u optionId): cada endpoint lee el SUYO, porque aceptar el del otro dejaba que
// `DELETE /platform-prices/modifier-option?productId=5` borrara el delta de la opción 5 y volvía
// ambiguo el registro de qué se borró.
func idsDePrecio(r *http.Request, nombre string) (int64, int64, error) {
	q := r.URL.Query()
	a, err := strconv.ParseInt(q.Get(nombre), 10, 64)
	if err != nil || a == 0 {
		return 0, 0, domain.ErrValidation
	}
	// 16 bits, no 64: el id de plataforma es smallint, y truncar int64→int16 hacía que
	// platformId=65537 borrara el precio de la plataforma 1 y respondiera 204. El PUT ya lo
	// rechazaba (el int16 del JSON no decodifica), así que las dos mitades no validaban igual.
	b, err := strconv.ParseInt(q.Get("platformId"), 10, 16)
	if err != nil || b == 0 {
		return 0, 0, domain.ErrValidation
	}
	return a, b, nil
}
