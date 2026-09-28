package uber

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// AL GUARDAR, QUIEN OPERA TIENE QUE SABER QUÉ CORREGIR. Antes toda respuesta distinta de 200 era
// «rechazó las credenciales»: igual para un secret mal copiado, para una app a la que le faltan
// permisos y para Uber sin responder — y las tres piden hacer cosas distintas.
//
// Los cuerpos son los que documenta developer.uber.com/docs/eats/guides/authentication; el código
// HTTP por caso NO está verificado, por eso la clasificación sale del campo `error` y no del status.
func TestVerifyTellsWhatToFix(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"secret mal copiado", 401, `{"error":"invalid_client","error_description":"The client ID or secret provided is invalid"}`, ErrCredentialsRejected},
		// Medido en http/uber-eats.http: una app de pruebas contra el ambiente de producción.
		{"app del otro ambiente", 401, `{"error":"unauthorized_client"}`, ErrCredentialsRejected},
		{"app sin permisos", 400, `{"error":"invalid_scope","error_description":"The scope parameter is not valid"}`, ErrMissingScopes},
		{"401 sin body", 401, ``, ErrCredentialsRejected},
		{"uber caído", 503, `<html>down</html>`, ErrUnavailable},
		{"uber con error propio", 500, `{"error":"server_error"}`, ErrUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			err := clienteContra(t, srv).Verify(context.Background())
			if !errors.Is(err, c.want) {
				t.Fatalf("esperaba %v, fue %v", c.want, err)
			}
		})
	}
}

// Uber que no contesta NO es una credencial rechazada. Antes salía como ErrAuth y la lectura de
// menú lo registraba como `auth_rechazada`: mandaba a revisar las credenciales por una caída de red.
func TestNoNetworkIsNotARejectedCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	c := clienteContra(t, srv)
	srv.Close() // la dirección queda sin nadie escuchando
	err := c.Verify(context.Background())
	if !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrAuth) {
		t.Fatalf("esperaba ErrUnavailable y no ErrAuth, fue %v", err)
	}
	if got := ClaseDeFalloDe(err); got != domain.FalloTiempoAgotado {
		t.Fatalf("clase de fallo: %q", got)
	}
}

// Verify deja el token puesto: el cliente que se comprobó al guardar es el que se queda
// atendiendo, y pedir otro sería gastar uno de los 100 por hora.
func TestVerifyLeavesTheTokenForWhatFollows(t *testing.T) {
	var tokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens++
		_, _ = w.Write([]byte(`{"access_token":"t","expires_in":2592000}`))
	}))
	defer srv.Close()
	c := clienteContra(t, srv)
	if err := c.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.token(context.Background()); err != nil || tokens != 1 {
		t.Fatalf("tokens pedidos: %d (%v)", tokens, err)
	}
}
