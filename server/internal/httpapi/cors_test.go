package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// CORS_ORIGIN ACEPTA VARIOS ORÍGENES EXACTOS, y sigue siendo fail-closed.
//
// Nace de la consola de plataforma (spec 016): vive en `staff.elgatobobah.com`, un origen distinto
// del POS, y llama a la misma API. Con un solo origen permitido, el navegador bloquea la consola
// entera y el síntoma es un error de red que no menciona CORS por ningún lado.
//
// Lo que NO cambia: un origen que no está en la lista no recibe headers, y "*" sigue siendo cosa
// de desarrollo.
func TestCorsConVariosOrigenes(t *testing.T) {
	const pos = "https://app.elgatobobah.com"
	const consola = "https://staff.elgatobobah.com"

	permitido := func(config, origin string, dev bool) string {
		h := cors(config, dev)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Header().Get("Access-Control-Allow-Origin")
	}

	lista := pos + "," + consola
	t.Run("cada origen de la lista se refleja tal cual", func(t *testing.T) {
		if got := permitido(lista, pos, false); got != pos {
			t.Fatalf("origen del POS = %q, quiere %q", got, pos)
		}
		if got := permitido(lista, consola, false); got != consola {
			t.Fatalf("origen de la consola = %q, quiere %q", got, consola)
		}
	})

	// Lo que hace que la lista no sea un coladero: se compara COMPLETA y exacta, no por prefijo ni
	// por sufijo. "https://app.elgatobobah.com.atacante.com" contiene al origen bueno.
	t.Run("un origen ajeno no recibe nada", func(t *testing.T) {
		for _, malo := range []string{
			"https://atacante.com",
			"https://app.elgatobobah.com.atacante.com",
			"http://app.elgatobobah.com", // otro esquema es otro origen
		} {
			if got := permitido(lista, malo, false); got != "" {
				t.Errorf("origen %q recibió %q y no debe recibir nada", malo, got)
			}
		}
	})

	t.Run("un solo origen sigue funcionando igual", func(t *testing.T) {
		if got := permitido(pos, pos, false); got != pos {
			t.Fatalf("un origen solo = %q, quiere %q", got, pos)
		}
		if got := permitido(pos, consola, false); got != "" {
			t.Fatalf("con un solo origen configurado, la consola no debe pasar: %q", got)
		}
	})

	t.Run("vacío es solo mismo origen", func(t *testing.T) {
		if got := permitido("", pos, false); got != "" {
			t.Fatalf("sin configuración no se emiten headers, y llegó %q", got)
		}
	})

	t.Run("el asterisco sigue siendo de desarrollo", func(t *testing.T) {
		if got := permitido("*", consola, false); got != "" {
			t.Fatalf("\"*\" en producción no refleja nada, y llegó %q", got)
		}
		if got := permitido("*", consola, true); got != consola {
			t.Fatalf("\"*\" en desarrollo refleja el origen, y llegó %q", got)
		}
	})

	// Espacios alrededor de las comas: el .env se edita a mano y "a, b" es lo que sale de teclear.
	t.Run("tolera espacios en la lista", func(t *testing.T) {
		if got := permitido(pos+", "+consola, consola, false); got != consola {
			t.Fatalf("con espacios en la lista = %q, quiere %q", got, consola)
		}
	})
}
