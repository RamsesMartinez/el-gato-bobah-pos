package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Details lleva el error como DATO además de como prosa, para que el front pinte sin escarbar
	// en el texto del mensaje. Se omite cuando no hay nada que detallar: la respuesta de siempre.
	Details *errorDetails `json:"details,omitempty"`
}

// errorDetails son los campos estructurados que un error puede aportar. Hoy solo el producto que
// tumbó el cobro; se agregan campos cuando otro error los necesite, no antes.
type errorDetails struct {
	ProductID   int64  `json:"productId,omitempty"`
	ProductName string `json:"productName,omitempty"`
}

// JSON writes v as JSON with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Error maps a domain error to an HTTP status + stable code and writes the envelope.
func Error(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "INTERNAL"
	msg := err.Error()

	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, domain.ErrInvalidCredentials):
		status, code = http.StatusUnauthorized, "INVALID_CREDENTIALS"
	case errors.Is(err, domain.ErrUnauthorized):
		status, code = http.StatusUnauthorized, "UNAUTHORIZED"
	case errors.Is(err, domain.ErrForbidden):
		status, code = http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, domain.ErrValidation):
		status, code = http.StatusBadRequest, "VALIDATION"
	case errors.Is(err, domain.ErrNoOpenRegister):
		// 409 y código propio: no es un error de lo que mandó el cliente sino del estado del
		// negocio. El front lo necesita distinguible para bloquear la pantalla de venta y mandar a
		// abrir turno, en vez de mostrar un mensaje que el operador no puede accionar desde ahí.
		status, code = http.StatusConflict, "NO_OPEN_REGISTER"
	case errors.Is(err, domain.ErrConflict):
		status, code = http.StatusConflict, "CONFLICT"
	case errors.Is(err, domain.ErrTooManyRequests):
		status, code = http.StatusTooManyRequests, "TOO_MANY_REQUESTS"
	case errors.Is(err, domain.ErrProductNotSell),
		errors.Is(err, domain.ErrOptionNotFound),
		errors.Is(err, domain.ErrEmptyOrder):
		// reglas de negocio del armado del pedido: 422 con el mensaje real (no 500 opaco)
		status, code = http.StatusUnprocessableEntity, "UNPROCESSABLE"
	case errors.Is(err, domain.ErrWeakPassword):
		// política de contraseña: 422 con el motivo concreto (envuelto en el error)
		status, code = http.StatusUnprocessableEntity, "WEAK_PASSWORD"
	case errors.Is(err, domain.ErrResetInvalid):
		// enlace de recuperación inválido/usado/vencido: 410 Gone con mensaje accionable
		status, code = http.StatusGone, "RESET_INVALID"
	case errors.Is(err, app.ErrDocExtractDisabled):
		// 501 y no 500: la extracción de documentos es opcional y no está configurada. Distinguirlo
		// permite al front ofrecer la captura manual en vez de mostrar "error del servidor".
		status, code = http.StatusNotImplemented, "NOT_CONFIGURED"
	}

	if status == http.StatusInternalServerError {
		slog.Error("request failed", "error", err)
		msg = "Error interno del servidor"
		// Sin details en un 500: el mensaje ya se ocultó por opaco, y adjuntar el interior del
		// error por otra puerta lo desharía.
		JSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: msg}})
		return
	}

	// El producto que tumbó el cobro viaja también como dato. Ojo: ProductName va VACÍO cuando el
	// catálogo de la empresa no conoce el id — ver domain.ProductUnavailable; rellenarlo ahí
	// filtraría el menú de otra empresa.
	var details *errorDetails
	var pu domain.ProductUnavailable
	if errors.As(err, &pu) {
		details = &errorDetails{ProductID: pu.ProductID, ProductName: pu.Name}
	}
	JSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: msg, Details: details}})
}

// Decode parses a JSON request body into v, returning a validation error on failure.
func Decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return domain.ErrValidation
	}
	return nil
}
