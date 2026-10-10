package uber

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// EL DETALLE SE PIDE SIGUIENDO LA LIGA DEL AVISO, no armando la ruta a mano.
//
// Armarla a mano es exactamente lo que nos dio 404 en siete formas distintas al buscar el detalle
// de un pedido: la plataforma entrega un `resource_href` y ése es el contrato. Si algún día cambia
// la forma de esa URL, seguir la liga sigue funcionando y adivinarla no.
func TestElDetalleSigueLaLigaDelAviso(t *testing.T) {
	var pedida string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/v2/token" {
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
			return
		}
		pedida = r.URL.Path
		_, _ = w.Write([]byte(`{"id":"ped-1","display_id":"ABC12","placed_at":"2026-09-17T18:04:00Z"}`))
	}))
	defer srv.Close()

	c := clienteContra(t, srv)
	liga := srv.URL + "/v1/eats/order/ped-1"
	crudo, err := c.TraerDetalleDePedido(context.Background(), liga)
	if err != nil {
		t.Fatalf("traer el detalle: %v", err)
	}
	if pedida != "/v1/eats/order/ped-1" {
		t.Fatalf("pidió %q en vez de seguir la liga del aviso", pedida)
	}
	if !strings.Contains(string(crudo), "ABC12") {
		t.Fatalf("el detalle no llegó entero: %s", crudo)
	}
}

// UNA LIGA QUE APUNTA A OTRO LADO NO SE SIGUE.
//
// El `resource_href` viene DENTRO del cuerpo del aviso, y el cuerpo lo manda quien llama. Seguirlo
// a ciegas convierte nuestro servidor en un cliente que pide lo que le digan: un atacante que
// consiga que se procese un aviso apunta la liga a una dirección interna y usa la API como sonda
// de la red. Se exige que sea del host de la plataforma.
func TestUnaLigaQueApuntaAOtroHostNoSeSigue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
	}))
	defer srv.Close()
	c := clienteContra(t, srv)

	for _, liga := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:5432/",
		"https://evil.example.com/v1/eats/order/x",
		"file:///etc/passwd",
		"",
	} {
		if _, err := c.TraerDetalleDePedido(context.Background(), liga); err == nil {
			t.Errorf("siguió una liga fuera de la plataforma: %q", liga)
		}
	}
}

// Un detalle enorme no se decodifica entero en memoria: el techo va antes de leer.
func TestElDetalleTieneTecho(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/v2/token" {
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(`{"basura":"` + strings.Repeat("x", maxBytesDeDetalle+1024) + `"}`))
	}))
	defer srv.Close()

	c := clienteContra(t, srv)
	crudo, err := c.TraerDetalleDePedido(context.Background(), srv.URL+"/v1/eats/order/ped-1")
	if err == nil && len(crudo) > maxBytesDeDetalle {
		t.Fatalf("se leyeron %d bytes, por encima del techo de %d", len(crudo), maxBytesDeDetalle)
	}
}
