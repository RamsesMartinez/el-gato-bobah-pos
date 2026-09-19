package uber

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// clienteContra arma un Client apuntado a un servidor de prueba, con los dos guardias puestos.
func clienteContra(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &Client{
		clientID: "id", clientSecret: "secreto",
		authBase: srv.URL, apiBase: srv.URL,
		http:  &http.Client{Transport: soloLectura(http.DefaultTransport)},
		auth:  &http.Client{Transport: soloElHostDeToken(u.Host, http.DefaultTransport)},
		ahora: time.Now,
	}
}

// EL TOKEN SE REUSA, Y NO ES UNA OPTIMIZACIÓN. Uber permite 100 tokens por hora y **el 101 invalida
// el más antiguo**: pedir uno por lectura invalida el que otro proceso está usando, y el síntoma es
// un 401 intermitente que nadie puede reproducir.
func TestElTokenSeReusaEntreLecturas(t *testing.T) {
	menu, err := os.ReadFile("testdata/menu.json")
	if err != nil {
		t.Fatal(err)
	}
	var tokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth/") {
			tokens++
			w.Write([]byte(`{"access_token":"t1","expires_in":2592000,"scope":"eats.store"}`))
			return
		}
		w.Write(menu)
	}))
	defer srv.Close()

	c := clienteContra(t, srv)
	for i := 0; i < 3; i++ {
		if _, err := c.LeerMenu(context.Background(), "tienda"); err != nil {
			t.Fatalf("lectura %d: %v", i, err)
		}
	}
	if tokens != 1 {
		t.Fatalf("pidió %d tokens para 3 lecturas: con el límite de 100/hora esto invalida el token de otro proceso", tokens)
	}
}

// EL MARGEN DE RENOVACIÓN SE ACOTA A LA MITAD DE LA VIDA DEL TOKEN.
//
// Con un margen fijo de 24 h, un token que dura menos que eso nunca cumple la condición de reuso y
// se pide uno POR LECTURA — que es exactamente lo que este caché existe para evitar, y el camino al
// «401 intermitente» cuando Uber invalida el más viejo pasado el 101 en una hora.
//
// La primera versión de este test afirmaba lo contrario («dos lecturas, dos tokens») y con eso
// fijaba el defecto en vez de vigilarlo. Hoy Uber devuelve 30 días y no muerde; el día que acorte,
// esto lo absorbe.
func TestUnTokenCortoSeReusaDentroDeSuVida(t *testing.T) {
	menu, _ := os.ReadFile("testdata/menu.json")
	var tokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth/") {
			tokens++
			w.Write([]byte(`{"access_token":"corto","expires_in":43200}`)) // 12 h
			return
		}
		w.Write(menu)
	}))
	defer srv.Close()

	c := clienteContra(t, srv)
	for i := 0; i < 3; i++ {
		if _, err := c.LeerMenu(context.Background(), "tienda"); err != nil {
			t.Fatal(err)
		}
	}
	if tokens != 1 {
		t.Fatalf("pidió %d tokens para 3 lecturas con uno de 12 h todavía vivo", tokens)
	}
}

// Y sí se renueva una vez pasado su margen: quedarse con uno vencido a media lectura es el otro
// extremo del mismo error.
func TestElTokenSeRenuevaPasadoSuMargen(t *testing.T) {
	menu, _ := os.ReadFile("testdata/menu.json")
	var tokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth/") {
			tokens++
			w.Write([]byte(`{"access_token":"corto","expires_in":43200}`)) // 12 h -> margen de 6 h
			return
		}
		w.Write(menu)
	}))
	defer srv.Close()

	reloj := time.Now()
	c := clienteContra(t, srv)
	c.ahora = func() time.Time { return reloj }

	if _, err := c.LeerMenu(context.Background(), "tienda"); err != nil {
		t.Fatal(err)
	}
	reloj = reloj.Add(7 * time.Hour) // dentro de los 6 h de margen
	if _, err := c.LeerMenu(context.Background(), "tienda"); err != nil {
		t.Fatal(err)
	}
	if tokens != 2 {
		t.Fatalf("pidió %d tokens; a 5 h de vencer y con margen de 6 h tiene que renovarse", tokens)
	}
}

// UN MENÚ VACÍO NO ES UN MENÚ: es una lectura que no sirve. Compararla reportaría que sobra todo el
// catálogo — el peor reporte posible y el que más invita a una acción destructiva (FR-005).
func TestUnMenuSinItemsEsUnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth/") {
			w.Write([]byte(`{"access_token":"t","expires_in":2592000}`))
			return
		}
		w.Write([]byte(`{"menus":[],"categories":[],"items":[],"modifier_groups":[]}`))
	}))
	defer srv.Close()

	if _, err := clienteContra(t, srv).LeerMenu(context.Background(), "tienda"); !errors.Is(err, ErrVacio) {
		t.Fatalf("un menú vacío debe ser ErrVacio, dio %v", err)
	}
}

// EL ERROR QUE VIAJA NO LLEVA LA DIRECCIÓN NI EL SECRETO (FR-022). DiDi transmite su `app_secret`
// en el query string, así que la costumbre es no filtrar la URL nunca, y no acordarse de en cuál sí.
func TestElErrorDeAutenticacionNoLlevaElSecreto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := clienteContra(t, srv).LeerMenu(context.Background(), "tienda")
	if err == nil {
		t.Fatal("un 401 al pedir token tiene que fallar")
	}
	if strings.Contains(err.Error(), "secreto") || strings.Contains(err.Error(), srv.URL) {
		t.Errorf("el error lleva el secreto o la dirección: %v", err)
	}
	if !errors.Is(err, ErrAuth) {
		t.Errorf("debe ser ErrAuth para que el servicio lo guarde como auth_rechazada: %v", err)
	}
}

func TestClaseDeFalloDe(t *testing.T) {
	casos := map[error]domain.ClaseDeFallo{
		ErrAuth:                      domain.FalloAuthRechazada,
		ErrVacio:                     domain.FalloMenuVacio,
		context.DeadlineExceeded:     domain.FalloTiempoAgotado,
		ErrRespuesta:                 domain.FalloRespuestaInvalida,
		errors.New("cualquier otro"): domain.FalloRespuestaInvalida,
	}
	for err, quiere := range casos {
		if got := ClaseDeFalloDe(err); got != quiere {
			t.Errorf("%v dio %q, quería %q", err, got, quiere)
		}
	}
}

// El ambiente no cae a un default: adivinarlo es elegir por el operador entre la tienda de pruebas
// y la que factura.
func TestNewRechazaUnAmbienteDesconocido(t *testing.T) {
	if _, err := New("id", "secreto", "staging"); err == nil {
		t.Fatal("un ambiente desconocido debe rechazarse, no caer a sandbox")
	}
	for _, a := range []string{"sandbox", "production"} {
		if _, err := New("id", "secreto", a); err != nil {
			t.Errorf("%q: %v", a, err)
		}
	}
}
