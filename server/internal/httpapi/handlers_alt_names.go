package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Nombres alternativos (spec 063). La lista va por su propia ruta y completa: el PATCH de la ficha
// no la lleva, así que una tableta con la pantalla anterior no puede borrarla por no mandarla.

// altNamesIn es la lista completa. Puntero: un cuerpo sin la lista es una entrada mal formada, no
// «vaciarla» (vaciar exige `[]` explícito); si no, un `{}` borraba todos (revisión de seguridad).
type altNamesIn struct {
	AltNames *[]string `json:"altNames"`
}

type altNamesBody struct {
	AltNames []string `json:"altNames"`
}

func decodeAltNames(r *http.Request) ([]string, error) {
	var in altNamesIn
	if err := DecodeStrict(r, &in); err != nil {
		return nil, err
	}
	if in.AltNames == nil {
		return nil, fmt.Errorf("%w: falta la lista de nombres", domain.ErrValidation)
	}
	return *in.AltNames, nil
}

// PUT /admin/products/{id}/alt-names
func (h *Handlers) PutProductAltNames(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	names, err := decodeAltNames(r)
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.admin.SetProductAltNames(r.Context(), id, names)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, altNamesBody{AltNames: out})
}

// GET /admin/products/{id}/alt-names/similar?name= — el aviso de parecidos; no bloquea nada.
func (h *Handlers) SimilarProductAltNames(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.admin.SimilarProductAltNames(r.Context(), id, r.URL.Query().Get("name"))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// articleAltNamesIn lleva además la versión de la ficha que vio la pantalla (STALE si cambió).
type articleAltNamesIn struct {
	AltNames          *[]string  `json:"altNames"`
	ExpectedUpdatedAt *time.Time `json:"expectedUpdatedAt"`
}

// PUT /stock/articles/{id}/alt-names
func (h *Handlers) PutArticleAltNames(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var in articleAltNamesIn
	if err := DecodeStrict(r, &in); err != nil {
		Error(w, err)
		return
	}
	if in.AltNames == nil || in.ExpectedUpdatedAt == nil {
		Error(w, fmt.Errorf("%w: faltan la lista de nombres o la versión de la ficha", domain.ErrValidation))
		return
	}
	out, err := h.backoffice.SetArticleAltNames(r.Context(), id, *in.AltNames, *in.ExpectedUpdatedAt)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// GET /stock/articles/{id}/alt-names/similar?name=
func (h *Handlers) SimilarArticleAltNames(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.SimilarArticleAltNames(r.Context(), id, r.URL.Query().Get("name"))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}
