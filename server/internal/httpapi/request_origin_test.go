package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func originOf(t *testing.T, behindProxy bool, prep func(*http.Request)) domain.RequestOrigin {
	t.Helper()
	var got domain.RequestOrigin
	h := withRequestOrigin(behindProxy)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = app.OriginFrom(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.RemoteAddr = "10.0.0.2:51000"
	prep(req)
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

// La IP que se guarda como evidencia es la que el cliente no puede falsear: detrás del proxy, el
// ÚLTIMO X-Forwarded-For (el que agrega Caddy); sin proxy, el header no cuenta para nada.
func TestTheStoredIPIsTheOneTheClientCannotForge(t *testing.T) {
	forged := func(r *http.Request) { r.Header.Set("X-Forwarded-For", "6.6.6.6, 187.190.4.12") }
	if o := originOf(t, true, forged); o.IP == nil || o.IP.String() != "187.190.4.12" {
		t.Fatalf("detrás del proxy guardó %v; debía ser el último salto", o.IP)
	}
	if o := originOf(t, false, forged); o.IP == nil || o.IP.String() != "10.0.0.2" {
		t.Fatalf("sin proxy guardó %v; un X-Forwarded-For del cliente no debe contar", o.IP)
	}
}

func TestTheDeviceHeadersReachTheContext(t *testing.T) {
	o := originOf(t, false, func(r *http.Request) {
		r.Header.Set(DeviceKeyHeader, "0123456789abcdef0123456789abcdef")
		r.Header.Set(DeviceInfoHeader, `{"browser":"Edge 141"}`)
	})
	if o.DeviceKey == nil || *o.DeviceKey != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("clave = %v", o.DeviceKey)
	}
	if !strings.Contains(string(o.Device), "Edge 141") {
		t.Fatalf("descripción = %s", o.Device)
	}
}

// SIN ESTO, NINGUNA PETICIÓN DEL POS LLEGA. El front vive en otro dominio que la API: un header
// propio que CORS no anuncia hace que el navegador rechace el preflight, y el POS entero queda sin
// red con la API respondiendo bien.
func TestCORSAllowsTheDeviceHeaders(t *testing.T) {
	h := cors("https://pos.example.com", false)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/orders", nil)
	req.Header.Set("Origin", "https://pos.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	allowed := rec.Header().Get("Access-Control-Allow-Headers")
	for _, hd := range []string{DeviceKeyHeader, DeviceInfoHeader} {
		if !strings.Contains(allowed, hd) {
			t.Errorf("CORS no permite %s (Allow-Headers = %q)", hd, allowed)
		}
	}
}
