package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// La frontera de la subida de fotos: lo que el handler rechaza antes de llegar al servicio, y que un
// cuerpo gigante se corta sin leerlo entero. Con el almacenamiento apagado, un cuerpo bien formado
// llega al servicio y vuelve 503.
func TestPutProductImageBoundary(t *testing.T) {
	h := &Handlers{media: app.NewCatalogMediaService(nil, nil, "")}
	b64 := base64.StdEncoding.EncodeToString([]byte("algo"))
	cases := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"json roto", `{`, 400, "VALIDATION"},
		{"falta la de 400", `{"image1200":"` + b64 + `"}`, 400, "VALIDATION"},
		{"base64 roto", `{"image1200":"` + b64 + `","image400":"%%%"}`, 400, "VALIDATION"},
		{"bien formado sin almacenamiento", `{"image1200":"` + b64 + `","image400":"` + b64 + `"}`, 503, "MEDIA_DISABLED"},
		{"cuerpo de más de 1 MiB", `{"image1200":"` + strings.Repeat("A", 1<<20) + `","image400":""}`, 413, "IMAGE_TOO_LARGE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/admin/products/7/image", strings.NewReader(c.body))
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", "7")
			ctx := context.WithValue(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), userCtxKey, AuthUser{ID: 1, CompanyID: 1})
			w := httptest.NewRecorder()
			h.PutProductImage(w, req.WithContext(ctx))
			var got struct {
				Error struct{ Code string } `json:"error"`
			}
			_ = json.NewDecoder(w.Body).Decode(&got)
			if w.Code != c.status || got.Error.Code != c.code {
				t.Fatalf("%d %s, quería %d %s", w.Code, got.Error.Code, c.status, c.code)
			}
		})
	}
}
