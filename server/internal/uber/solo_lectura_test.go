package uber

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// LA GARANTÍA DE QUE NADA ESCRIBE EN LA TIENDA no es una regla de estilo ni una revisión de código:
// es este transporte. El `PUT` de menú de Uber es reemplazo total —«overwrites any existing
// menus»— y el primer disparo contra la tienda viva sustituye el menú publicado por lo que vaya en
// el cuerpo. No hay deshacer, y el peor momento es un sábado.
//
// El test pide cada verbo de escritura contra un servidor que registraría el golpe, y exige que
// falle ANTES de abrir el socket: si el servidor recibe algo, el guardia no sirve.
func TestElTransporteRechazaTodoVerboQueNoSeaGET(t *testing.T) {
	var llegó bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llegó = true
	}))
	defer srv.Close()

	cli := &http.Client{Transport: soloLectura(http.DefaultTransport)}
	for _, verbo := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req, err := http.NewRequest(verbo, srv.URL+"/v2/eats/stores/x/menus", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := cli.Do(req)
		if err == nil {
			resp.Body.Close()
			t.Fatalf("%s pasó el transporte: el menú de la tienda viva se puede reemplazar", verbo)
		}
		// El mensaje tiene que nombrar plataforma y operación: un error genérico manda a depurar
		// la red cuando lo que pasó fue que alguien agregó una escritura.
		if !strings.Contains(err.Error(), "Uber") || !strings.Contains(err.Error(), verbo) {
			t.Errorf("%s: el error no nombra plataforma y operación: %v", verbo, err)
		}
	}
	if llegó {
		t.Fatal("una petición de escritura alcanzó el servidor: el guardia se aplica demasiado tarde")
	}
}

func TestElTransporteDejaPasarGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cli := &http.Client{Transport: soloLectura(http.DefaultTransport)}
	resp, err := cli.Get(srv.URL + "/v2/eats/stores/x/menus")
	if err != nil {
		t.Fatalf("un GET tiene que pasar: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET dio %d", resp.StatusCode)
	}
}

// El token SÍ necesita un POST, y por eso va por otro transporte. Ese otro tiene su propia
// restricción: un único destino permitido. Si el host de menú pudiera colarse por aquí, el guardia
// de arriba se evade con un cambio de cliente.
func TestElTransporteDeTokenSoloHablaConSuHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"x","expires_in":1}`))
	}))
	defer srv.Close()

	permitido := strings.TrimPrefix(srv.URL, "http://")
	cli := &http.Client{Transport: soloElHostDeToken(permitido, http.DefaultTransport)}

	resp, err := cli.Post(srv.URL+"/oauth/v2/token", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("el host de token tiene que pasar: %v", err)
	}
	_ = resp.Body.Close()
	otro := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("el transporte del token alcanzó un host que no es el suyo")
	}))
	defer otro.Close()
	fuera, err := cli.Post(otro.URL+"/v2/eats/stores/x/menus", "application/json", strings.NewReader("{}"))
	if err == nil {
		_ = fuera.Body.Close()
		t.Fatal("el transporte del token llegó a otro host: el guardia de escritura se evade cambiando de cliente")
	}
}
