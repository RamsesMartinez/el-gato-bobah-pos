package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GET /business-settings — cualquier autenticado (el cobro necesita el costo de envío por defecto).
func (h *Handlers) BusinessSettings(w http.ResponseWriter, r *http.Request) {
	bs, err := h.settings.Get(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, bs)
}

// PUT /business-settings — solo admin/gerente (gateado en el router). Actualiza el costo de envío,
// la identidad que sale en el ticket y el interruptor de impresión automática.
func (h *Handlers) UpdateBusinessSettings(w http.ResponseWriter, r *http.Request) {
	// Punteros para distinguir "no vino" de "vino vacío": la pantalla de envío y la del ticket
	// guardan por separado, y una no debe borrar lo que capturó la otra.
	var body struct {
		DeliveryFee      *decimal.Decimal `json:"deliveryFee"`
		BusinessName     *string          `json:"businessName"`
		Address          *string          `json:"address"`
		Phone            *string          `json:"phone"`
		HeaderNote       *string          `json:"headerNote"`
		FooterNote       *string          `json:"footerNote"`
		AutoPrintOnClose *bool            `json:"autoPrintOnClose"`
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
	ctx := r.Context()

	if body.DeliveryFee != nil {
		if _, err := h.settings.SetDeliveryFee(ctx, *body.DeliveryFee, u.ID); err != nil {
			Error(w, err)
			return
		}
	}

	if body.BusinessName != nil || body.Address != nil || body.Phone != nil ||
		body.HeaderNote != nil || body.FooterNote != nil || body.AutoPrintOnClose != nil {
		cur, err := h.settings.Get(ctx)
		if err != nil {
			Error(w, err)
			return
		}
		info := domain.BusinessInfo{
			Name:       orCurrent(body.BusinessName, cur.BusinessName),
			Address:    orCurrent(body.Address, cur.Address),
			Phone:      orCurrent(body.Phone, cur.Phone),
			HeaderNote: orCurrent(body.HeaderNote, cur.HeaderNote),
			FooterNote: orCurrent(body.FooterNote, cur.FooterNote),
		}
		autoPrint := cur.AutoPrintOnClose
		if body.AutoPrintOnClose != nil {
			autoPrint = *body.AutoPrintOnClose
		}
		if _, err := h.settings.SetBusinessInfo(ctx, info, autoPrint, u.ID); err != nil {
			Error(w, err)
			return
		}
	}

	bs, err := h.settings.Get(ctx)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, bs)
}

func orCurrent(in *string, current string) string {
	if in == nil {
		return current
	}
	return *in
}

// maxLogoUpload acota el cuerpo ANTES de leerlo: 256 KB de imagen más holgura para el sobre del
// multipart. El tope real de la imagen lo vuelve a aplicar domain.ValidateLogo.
const maxLogoUpload = domain.MaxLogoBytes + (16 << 10)

// PUT /business-settings/ticket-logo — solo admin/gerente (gateado en el router).
func (h *Handlers) UploadTicketLogo(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	// MaxBytesReader es la cota real del cuerpo (corta la subida); el argumento de
	// ParseMultipartForm solo dice cuánto se guarda en RAM antes de ir a disco.
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoUpload)
	//nolint:gosec // G120: el cuerpo ya viene acotado por el MaxBytesReader de la línea anterior
	if err := r.ParseMultipartForm(maxLogoUpload); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			Error(w, domain.ErrLogoTooLarge)
			return
		}
		Error(w, fmt.Errorf("%w: no se pudo leer el archivo", domain.ErrValidation))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	file, _, err := r.FormFile("file")
	if err != nil {
		Error(w, fmt.Errorf("%w: falta el archivo en el campo \"file\"", domain.ErrValidation))
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		Error(w, fmt.Errorf("%w: no se pudo leer el archivo", domain.ErrValidation))
		return
	}
	bs, err := h.settings.SetLogo(r.Context(), data, u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, bs)
}

// DELETE /business-settings/ticket-logo — solo admin/gerente. Idempotente a propósito.
func (h *Handlers) DeleteTicketLogo(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	bs, err := h.settings.ClearLogo(r.Context(), u.ID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, bs)
}

// TicketLogo sirve el binario del logo del ticket. Autenticado pero sin rol: lo necesita cada caja
// al imprimir, no solo quien administra.
func (h *Handlers) TicketLogo(w http.ResponseWriter, r *http.Request) {
	logo, ok, err := h.settings.Logo(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	if !ok {
		// 404 y no un 200 vacío: "este negocio no subió logo" es ausencia de recurso, y el front
		// la traduce a su logo por default sin enseñar un error al operador.
		http.NotFound(w, r)
		return
	}
	// El ETag sale de la fecha de actualización porque el logo solo cambia cuando alguien lo sube;
	// sin él, cada ticket vuelve a bajar la imagen completa.
	etag := fmt.Sprintf(`"%d"`, logo.UpdatedAt.UnixNano())
	w.Header().Set("ETag", etag)
	// Es dato del negocio, no de un CDN: se cachea en el navegador pero se revalida siempre.
	w.Header().Set("Cache-Control", "private, must-revalidate")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	// El mime es el que se detectó por CONTENIDO al subir, no el que declaró el cliente. nosniff
	// impide que el navegador reinterprete el binario como HTML y lo ejecute en nuestro origen.
	w.Header().Set("Content-Type", logo.Mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(logo.Bytes)))
	_, _ = w.Write(logo.Bytes)
}
