package httpapi

import (
	"net/http"
)

// Nombres alternativos (spec 063). La lista va por su propia ruta y completa: el PATCH de la ficha
// no la lleva, así que una tableta con la pantalla anterior no puede borrarla por no mandarla.

type altNamesBody struct {
	AltNames []string `json:"altNames"`
}

// PUT /admin/products/{id}/alt-names
func (h *Handlers) PutProductAltNames(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var in altNamesBody
	if err := DecodeStrict(r, &in); err != nil {
		Error(w, err)
		return
	}
	out, err := h.admin.SetProductAltNames(r.Context(), id, in.AltNames)
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

// PUT /stock/articles/{id}/alt-names
func (h *Handlers) PutArticleAltNames(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		Error(w, err)
		return
	}
	var in altNamesBody
	if err := DecodeStrict(r, &in); err != nil {
		Error(w, err)
		return
	}
	out, err := h.backoffice.SetArticleAltNames(r.Context(), id, in.AltNames)
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
