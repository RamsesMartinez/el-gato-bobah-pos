package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// publicMenuCache: la Function del borde guarda su propia copia; esto es para quien llama directo.
// Corto a propósito: un agotado o un precio nuevo tiene que verse en minutos (SC-006).
const publicMenuCache = "public, max-age=30"

// publicMenuCacheControl decide qué se puede guardar: el menú y el «no existe» sí; un error del
// servidor (la base caída medio minuto) no, o una caché lo serviría a todos los visitantes.
func publicMenuCacheControl(err error) string {
	if err == nil || errors.Is(err, domain.ErrNotFound) {
		return publicMenuCache
	}
	return "no-store"
}

// GET /public/menus/{name} — el menú de un negocio, sin sesión (spec 097). El nombre resuelve la
// empresa; un nombre anterior responde a dónde llevar, y uno que no existe, el mismo 404 siempre.
func (h *Handlers) PublicMenu(w http.ResponseWriter, r *http.Request) {
	menu, moved, err := h.publicMenu.Menu(r.Context(), chi.URLParam(r, "name"))
	w.Header().Set("Cache-Control", publicMenuCacheControl(err))
	if err != nil {
		Error(w, err)
		return
	}
	if moved != nil {
		JSON(w, http.StatusOK, moved)
		return
	}
	JSON(w, http.StatusOK, menu)
}

// GET /public/menus/q/{token} — a qué nombre lleva el código de un QR impreso.
func (h *Handlers) PublicMenuByToken(w http.ResponseWriter, r *http.Request) {
	moved, err := h.publicMenu.ByToken(r.Context(), chi.URLParam(r, "token"))
	w.Header().Set("Cache-Control", publicMenuCacheControl(err))
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, moved)
}

// GET /admin/public-menu — nombre, enlace, QR y color del menú de la empresa.
func (h *Handlers) GetPublicMenuSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.publicMenu.Settings(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, s)
}

// PATCH /admin/public-menu — cambia el nombre, el color o los dos.
func (h *Handlers) UpdatePublicMenuSettings(w http.ResponseWriter, r *http.Request) {
	var body app.PublicMenuUpdate
	if err := DecodeStrict(r, &body); err != nil {
		Error(w, err)
		return
	}
	s, err := h.publicMenu.Update(r.Context(), body)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, s)
}
