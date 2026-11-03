package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// La tableta revisa el menú cada minuto. Si «sin cambios» no fuera un 304 sin cuerpo, cada tableta
// bajaría el catálogo completo sesenta veces por hora; si un ETag distinto diera 304, se quedaría
// con el menú viejo, que es el incidente del 2026-10-10.
func TestWriteMenuAnswersNotModifiedOnlyForTheCurrentETag(t *testing.T) {
	const etag = `"v3-2-7-20261010"`
	cases := []struct {
		name       string
		ifNone     string
		wantStatus int
		wantBuilt  bool
	}{
		{"sin If-None-Match", "", http.StatusOK, true},
		{"mismo ETag", etag, http.StatusNotModified, false},
		{"ETag débil del mismo menú", "W/" + etag, http.StatusNotModified, false},
		{"en una lista", `"otro", ` + etag, http.StatusNotModified, false},
		{"versión anterior", `"v3-2-6-20261010"`, http.StatusOK, true},
		{"otra empresa", `"v3-1-7-20261010"`, http.StatusOK, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/pos/menu", nil)
			if c.ifNone != "" {
				r.Header.Set("If-None-Match", c.ifNone)
			}
			w := httptest.NewRecorder()
			built := false
			writeMenu(w, r, etag, func() ([]byte, error) { built = true; return []byte(`{"products":[]}`), nil })
			if w.Code != c.wantStatus {
				t.Fatalf("status %d, quería %d", w.Code, c.wantStatus)
			}
			if built != c.wantBuilt {
				t.Fatalf("armó el menú = %v, quería %v", built, c.wantBuilt)
			}
			if w.Header().Get("ETag") != etag {
				t.Fatalf("ETag %q, quería %q", w.Header().Get("ETag"), etag)
			}
			if c.wantStatus == http.StatusNotModified && w.Body.Len() != 0 {
				t.Fatalf("un 304 llevó cuerpo (%d bytes)", w.Body.Len())
			}
		})
	}
}

func TestWriteMenuReportsABuildFailure(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/pos/menu", nil)
	w := httptest.NewRecorder()
	writeMenu(w, r, `"x"`, func() ([]byte, error) { return nil, errors.New("base caída") })
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", w.Code)
	}
	if w.Header().Get("ETag") != "" {
		t.Fatal("un error salió con ETag: la tableta lo guardaría como si fuera el menú")
	}
}
