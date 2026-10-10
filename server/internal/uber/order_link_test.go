package uber

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// UNA LIGA DEL PEDIDO EN http:// NO SE SIGUE, AUNQUE SEA DEL HOST DE UBER. Para bajar el detalle se
// manda el token de acceso, y por http viajaría en claro: quien esté en medio lo lee, y con él se
// aceptan y rechazan pedidos a nombre del negocio. La regla es «el mismo esquema y host que la
// dirección configurada», y las configuradas son https: api.uber.com y test-api.uber.com.
func TestAnHTTPOrderLinkIsNotFollowed(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"access_token":"t","expires_in":2592000}`))
	}))
	defer srv.Close()
	c := clienteContra(t, srv)
	host := strings.TrimPrefix(srv.URL, "http://")
	c.apiBase = "https://" + host // la configurada es https, como en producción

	_, err := c.TraerDetalleDePedido(context.Background(), "http://"+host+"/v2/eats/order/ped-1")
	if !errors.Is(err, ErrRespuesta) {
		t.Fatalf("una liga http con la base en https se aceptó: %v", err)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("se hicieron %d peticiones: el token pudo haber salido antes de rechazar la liga", n)
	}
}
