package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// maxImageBody acota el cuerpo de una subida antes de leerlo: las dos variantes en su tope (400 KB +
// 100 KB) en base64 son ~667 KB. Es el mismo MiB del router, repetido aquí para que el handler no
// dependa de que alguien recuerde montarlo detrás de ese middleware.
const maxImageBody = 1 << 20

type imageBody struct {
	Image1200 string `json:"image1200"`
	Image400  string `json:"image400"`
}

// decodeImage lee las dos variantes. El tipo, el peso de cada una y sus dimensiones los decide el
// servicio sobre los bytes decodificados; aquí solo la forma del cuerpo.
func decodeImage(w http.ResponseWriter, r *http.Request) (app.ImageUpload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBody)
	var b imageBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return app.ImageUpload{}, fmt.Errorf("%w: la foto pesa más de %d KB en total", domain.ErrImageTooLarge, maxImageBody>>10)
		}
		return app.ImageUpload{}, fmt.Errorf("%w: no se pudo leer la foto", domain.ErrValidation)
	}
	if b.Image1200 == "" || b.Image400 == "" {
		return app.ImageUpload{}, fmt.Errorf("%w: faltan las dos medidas de la foto (image1200 e image400)", domain.ErrValidation)
	}
	large, err1 := base64.StdEncoding.DecodeString(b.Image1200)
	small, err2 := base64.StdEncoding.DecodeString(b.Image400)
	if err1 != nil || err2 != nil {
		return app.ImageUpload{}, fmt.Errorf("%w: la foto no viene en base64", domain.ErrValidation)
	}
	return app.ImageUpload{Large: large, Small: small}, nil
}

func entityID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: id inválido", domain.ErrValidation)
	}
	return id, nil
}

func (h *Handlers) putImage(kind domain.MediaKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := userFrom(r.Context())
		if !ok {
			Error(w, domain.ErrUnauthorized)
			return
		}
		id, err := entityID(r)
		if err != nil {
			Error(w, err)
			return
		}
		up, err := decodeImage(w, r)
		if err != nil {
			Error(w, err)
			return
		}
		url, err := h.media.SetImage(r.Context(), kind, id, up, u.ID)
		if err != nil {
			Error(w, err)
			return
		}
		JSON(w, http.StatusOK, map[string]any{"imageUrl": url})
	}
}

func (h *Handlers) deleteImage(kind domain.MediaKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := userFrom(r.Context())
		if !ok {
			Error(w, domain.ErrUnauthorized)
			return
		}
		id, err := entityID(r)
		if err != nil {
			Error(w, err)
			return
		}
		if err := h.media.RemoveImage(r.Context(), kind, id, u.ID); err != nil {
			Error(w, err)
			return
		}
		JSON(w, http.StatusOK, map[string]any{"imageUrl": nil})
	}
}

func (h *Handlers) getImage(kind domain.MediaKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := entityID(r)
		if err != nil {
			Error(w, err)
			return
		}
		info, err := h.media.Info(r.Context(), kind, id)
		if err != nil {
			Error(w, err)
			return
		}
		JSON(w, http.StatusOK, info)
	}
}

// GET /admin/products/{id}/image — menu.configure. La foto actual (la vista previa del MCP la lee).
func (h *Handlers) GetProductImage(w http.ResponseWriter, r *http.Request) {
	h.getImage(domain.MediaProducts)(w, r)
}

// GET /admin/categories/{id}/image — menu.configure.
func (h *Handlers) GetCategoryImage(w http.ResponseWriter, r *http.Request) {
	h.getImage(domain.MediaCategories)(w, r)
}

// PUT /admin/products/{id}/image — menu.configure. Cuerpo: {"image1200":b64,"image400":b64}.
func (h *Handlers) PutProductImage(w http.ResponseWriter, r *http.Request) {
	h.putImage(domain.MediaProducts)(w, r)
}

// DELETE /admin/products/{id}/image — menu.configure. Idempotente.
func (h *Handlers) DeleteProductImage(w http.ResponseWriter, r *http.Request) {
	h.deleteImage(domain.MediaProducts)(w, r)
}

// PUT /admin/categories/{id}/image — menu.configure.
func (h *Handlers) PutCategoryImage(w http.ResponseWriter, r *http.Request) {
	h.putImage(domain.MediaCategories)(w, r)
}

// DELETE /admin/categories/{id}/image — menu.configure. Idempotente.
func (h *Handlers) DeleteCategoryImage(w http.ResponseWriter, r *http.Request) {
	h.deleteImage(domain.MediaCategories)(w, r)
}
