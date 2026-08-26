package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// maxUploadBytes acota el multipart ANTES de leerlo a memoria. El servicio vuelve a validar el
// tamaño del archivo; esto corta la subida en la frontera para que un archivo de 500 MB no se
// lea completo solo para rechazarlo.
const maxUploadBytes = 13 << 20 // 13 MiB (el servicio acepta hasta 12 del documento)

// ExtractPurchaseDoc recibe un ticket/factura/pedido (PDF o foto) y devuelve el borrador
// estructurado. NO escribe nada: el operador revisa y confirma, y el gasto se crea después con
// POST /expenses. Una extracción equivocada cuesta una corrección, no un inventario corrupto.
func (h *Handlers) ExtractPurchaseDoc(w http.ResponseWriter, r *http.Request) {
	if !h.purchaseDoc.Enabled() {
		// 501: no es un error del cliente ni una falla, es una feature no configurada.
		Error(w, fmt.Errorf("%w", app.ErrDocExtractDisabled))
		return
	}
	u, _ := userFrom(r.Context())
	// Por usuario y no por IP: todo el local sale por la misma IP, y el tope es de presupuesto.
	key := fmt.Sprintf("extract:%d", u.ID)
	if h.docExtract.blocked(r.Context(), key) {
		logging.SecurityEvent(r.Context(), "doc_extract_rate_limited", "user_id", u.ID)
		tooManyRequests(w, h.docExtract.retryAfter(r.Context(), key))
		return
	}
	// Se cuenta la llamada, no el fallo: lo que cuesta dinero es el intento.
	h.docExtract.record(r.Context(), key)

	// MaxBytesReader es la cota real del cuerpo (corta la subida, no solo la memoria); el
	// argumento de ParseMultipartForm es solo cuánto se guarda en RAM antes de ir a disco.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	//nolint:gosec // G120: el cuerpo ya viene acotado por el MaxBytesReader de la línea anterior
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		Error(w, fmt.Errorf("%w: no se pudo leer el archivo (máximo %d MB)", domain.ErrValidation, maxUploadBytes>>20))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	file, header, err := r.FormFile("file")
	if err != nil {
		Error(w, fmt.Errorf("%w: falta el archivo (campo 'file')", domain.ErrValidation))
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		Error(w, err)
		return
	}

	doc, raw, err := h.purchaseDoc.Extract(r.Context(), header.Filename, data)
	if err != nil {
		// Un timeout o un 5xx del proveedor del modelo no es culpa del cliente: se distingue del
		// documento inválido para que el front pueda ofrecer "reintentar" en vez de "corrige el
		// archivo".
		if !errors.Is(err, domain.ErrValidation) {
			logging.SecurityEvent(r.Context(), "doc_extract_failed", "user_id", u.ID)
		}
		Error(w, err)
		return
	}
	rec := doc.Reconcile()
	JSON(w, http.StatusOK, map[string]any{
		"doc": doc,
		// El crudo va de vuelta para que el front lo mande en docRaw al crear el gasto: así queda
		// guardado tal como lo devolvió el modelo, sin que el servidor tenga que retenerlo entre
		// dos requests.
		"raw": raw,
		// reconciliation es el semáforo de confianza: dice si el documento se explica a sí mismo.
		// Es informativo — un pedido con descuento a nivel documento no cuadra por línea y sigue
		// siendo válido.
		"reconciliation": map[string]any{
			"linesSum":           rec.LinesSum,
			"chargesSum":         rec.ChargesSum,
			"breakdownSum":       rec.BreakdownSum,
			"total":              rec.Total,
			"diff":               rec.Diff,
			"balanced":           rec.Balanced(),
			"paymentsSum":        rec.PaymentsSum,
			"paymentsMatchTotal": rec.PaymentsMatchTotal(),
			"subtotal":           rec.Subtotal,
			"hasSubtotal":        rec.HasSubtotal,
			"linesMatchSubtotal": rec.LinesMatchSubtotal(),
			"unreadable":         rec.Unreadable,
		},
	})
}
