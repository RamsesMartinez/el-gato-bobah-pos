package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
)

// GET /pos/menu — documento denormalizado del catálogo, con ETag. La versión se lee ANTES de
// armar: un cambio que llegue en medio deja el menú nuevo bajo la llave vieja (se rearma de más),
// nunca un menú viejo bajo la llave nueva.
func (h *Handlers) PosMenu(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := userFrom(ctx)
	ver, err := h.menu.Version(ctx)
	if err != nil {
		Error(w, err)
		return
	}
	etag := cache.MenuETag(u.CompanyID, ver.Catalog, ver.Day)
	writeMenu(w, r, etag, func() ([]byte, error) {
		if b, ok := h.menuCache.Get(ctx, u.CompanyID, ver.Catalog, ver.Day); ok {
			return b, nil
		}
		doc, err := h.menu.Build(ctx)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(doc)
		if err != nil {
			return nil, err
		}
		h.menuCache.Set(ctx, u.CompanyID, ver.Catalog, ver.Day, b)
		return b, nil
	})
}

// writeMenu responde 304 sin cuerpo cuando la tableta ya tiene ese menú, y si no lo arma. `private`:
// el menú es de una empresa y ningún caché intermedio debe compartirlo; `no-cache`: se revalida
// siempre en vez de usar una copia vencida.
func writeMenu(w http.ResponseWriter, r *http.Request, etag string, body func() ([]byte, error)) {
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "private, no-cache")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	b, err := body()
	if err != nil {
		Error(w, err)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

// etagMatches compara como pide RFC 9110 para If-None-Match: comparación débil, y el encabezado
// puede traer una lista.
func etagMatches(header, etag string) bool {
	for _, t := range strings.Split(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(t), "W/") == etag {
			return true
		}
	}
	return false
}

// GET /pos/popular — IDs de producto más vendidos (read model aparte, TTL corto).
// Redis primero; si falla, lo calcula desde Postgres y lo cachea 5 min.
func (h *Handlers) PosPopular(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := userFrom(ctx)
	if b, ok := h.menuCache.GetPopular(ctx, u.CompanyID); ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
		return
	}
	ids, err := h.menu.Popular(ctx)
	if err != nil {
		Error(w, err)
		return
	}
	b, err := json.Marshal(map[string]any{"items": ids})
	if err != nil {
		Error(w, err)
		return
	}
	h.menuCache.SetPopular(ctx, u.CompanyID, b)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// GET /pos/modifier-defaults — opciones de modificador más probables por contexto
// (producto→grupo→[optionId rankeadas]). El POS las usa para preseleccionar.
func (h *Handlers) ModifierDefaults(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	defaults, err := h.suggest.Defaults(r.Context(), u.CompanyID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, defaults)
}

// POST /admin/reload — recarga el estado en caché sin reiniciar el proceso: popularidad y el memo
// del recomendador. El menú ya no hace falta: su llave cambia sola con la versión de catálogo
// (0093), también tras un script.
// OJO: recarga ESTADO, no CÓDIGO — cambios de lógica siguen requiriendo recompilar.
func (h *Handlers) AdminReload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := userFrom(ctx)
	h.menuCache.InvalidatePopular(ctx, u.CompanyID)
	h.suggest.Invalidate(u.CompanyID)
	JSON(w, http.StatusOK, map[string]any{"reloaded": []string{"popular", "recommendations"}})
}

// GET /products/{id}/costing — costo calculado del producto (desglose simple).
func (h *Handlers) ProductCosting(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		Error(w, err)
		return
	}
	// De dónde sale el costo (spec 096): renglones con su %, de dónde vino y la merma real.
	v, err := h.costing.ProductCosting(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// GET /pos/menu/sold-out — los productos que salen «Agotado» ahora (spec 092). Sin caché: la
// existencia cambia con cada venta.
func (h *Handlers) PosSoldOut(w http.ResponseWriter, r *http.Request) {
	ids, err := h.menu.SoldOut(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	writeSoldOut(w, ids)
}

// writeSoldOut escribe la lista siempre como arreglo: un `null` tumba la pantalla de venta sin un
// solo error en el servidor (AGENTS.md, slices nil).
func writeSoldOut(w http.ResponseWriter, ids []int64) {
	if ids == nil {
		ids = []int64{}
	}
	w.Header().Set("Cache-Control", "no-store")
	JSON(w, http.StatusOK, app.SoldOutView{ProductIDs: ids})
}
