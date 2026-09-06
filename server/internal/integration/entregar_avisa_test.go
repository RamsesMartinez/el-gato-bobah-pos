//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// ENTREGAR TIENE QUE AVISARLE A LA OTRA TABLETA.
//
// Era la mutación de pedido que no publicaba evento. La segunda tableta seguía ofreciendo
// "Entregar" sobre comida que ya salió hasta su refresco de 30 segundos, y quien la tocaba recibía
// un error sobre una entrega que sí ocurrió — el operador concluye que el sistema se equivocó y la
// vuelve a sacar de cocina.
//
// Se prueba por el ROUTER y no por el servicio: el aviso vive en el handler, así que un test de
// servicio pasaría verde con el evento quitado.
func TestEntregarPublicaEventoParaLaOtraTableta(t *testing.T) {
	r, st, broker, token := newEntregasAPI(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_sse_entrega", "cajero")
	prod := makeProduct(t, st, "Café que sale", decimal.RequireFromString("40"), false)
	abrirCajaPrincipal(t, st, cajero)
	svc := app.NewOrdersService(st, clock)
	pedido, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("sembrar el pedido: %v", err)
	}

	// La OTRA tableta, escuchando.
	eventos, cerrar := broker.Subscribe(defaultCompanyID)
	defer cerrar()

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/orders/"+itoa(int(pedido.ID))+"/deliver", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token("gerente_sse", "gerente"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("entregar respondió %d: %s", w.Code, w.Body.String())
	}

	select {
	case ev := <-eventos:
		if ev.Type != "order.updated" {
			t.Errorf("la otra tableta recibió %q y esperaba order.updated", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Error("entregar no avisó: la otra tableta seguirá ofreciendo comida que ya salió hasta " +
			"su refresco, y quien la toque recibirá un error sobre una entrega que sí ocurrió")
	}
}

func newEntregasAPI(t *testing.T) (http.Handler, *store.Store, *realtime.Broker, func(string, string) string) {
	t.Helper()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	broker := realtime.NewBroker()
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT:       jm,
		Orders:    app.NewOrdersService(st, clock),
		MenuCache: cache.NewMenuCache(""),
		Broker:    broker,
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatalf("Issue(%s): %v", username, err)
		}
		return tok
	}
	return r, st, broker, token
}
