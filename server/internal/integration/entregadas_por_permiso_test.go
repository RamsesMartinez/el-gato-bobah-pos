//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// LA CAJERA VE LOS ENTREGADOS DEL DÍA, Y DEVOLVER SIGUE SIENDO DE QUIEN PUEDE SACAR DINERO (068).
//
// El 2026-11-06 una cajera no pudo reimprimir el ticket de un pedido de Uber que capturó y entregó
// otro usuario: la lista de entregados pedía rol admin o gerente, y tuvo que entrar una
// administradora con su usuario. Ver la lista es del mostrador; devolver dinero no.
//
// Se prueba en la frontera HTTP, con el router real: el defecto vivía en el gate de la ruta.
func TestLaCajeraVeLosEntregadosPeroNoDevuelve(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	// Con broker: devolver avisa a las tabletas, y sin él el handler truena.
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	r := httpapi.Router(config.Config{}, jm, httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Orders: app.NewOrdersService(st, clock), Broker: realtime.NewBroker()}), st)

	svc := app.NewOrdersService(st, clock)
	gerente := makeUser(t, st, "gerente_entrega_068", "gerente")
	cajera := makeUser(t, st, "cajera_entrega_068", "cajero")
	mesero := makeUser(t, st, "mesero_entrega_068", "mesero")
	admin := makeUser(t, st, "admin_entrega_068", "admin")
	cafe := makeProduct(t, st, "Café entregado por otro", decimal.RequireFromString("100"), false)
	abrirCajaPrincipal(t, st, gerente)

	// Lo captura y lo entrega OTRO usuario: es el caso del 6 de noviembre.
	nuevoEntregado := func() int64 {
		t.Helper()
		ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: gerente,
			Lines:    []domain.OrderLineInput{{ProductID: cafe, Qty: decimal.RequireFromString("1")}},
			Payments: []app.PaymentInput{{MethodID: paymentMethodID(t, st, "Efectivo"), Amount: decimal.RequireFromString("100")}},
		})
		if err != nil {
			t.Fatalf("crear el pedido: %v", err)
		}
		if err := svc.SetStatus(ctx, ord.ID, domain.StatusEntregada); err != nil {
			t.Fatalf("entregar: %v", err)
		}
		return ord.ID
	}
	id := nuevoEntregado()

	token := func(uid int64, rol domain.Role) string {
		t.Helper()
		tok, err := jm.Issue(domain.User{ID: uid, CompanyID: defaultCompanyID, Name: string(rol), Role: rol})
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		return tok
	}
	refund := []byte(`{"reason":"prueba de permiso"}`)

	t.Run("cajera", func(t *testing.T) {
		tok := token(cajera, domain.RoleCajero)
		w := do(t, r, http.MethodGet, "/api/v1/orders/delivered", tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("la cajera pide los entregados = %d, quiere 200: sin la lista no puede reimprimir el ticket: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), fmt.Sprintf(`"id":%d`, id)) {
			t.Errorf("la lista de la cajera no trae el pedido %d que entregó otro usuario: %s", id, w.Body.String())
		}
		if w := do(t, r, http.MethodGet, fmt.Sprintf("/api/v1/orders/%d/refund-info", id), tok, nil, ""); w.Code != http.StatusForbidden {
			t.Errorf("la cajera consulta cómo devolver = %d, quiere 403", w.Code)
		}
		if w := do(t, r, http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/refund", id), tok, refund, "application/json"); w.Code != http.StatusForbidden {
			t.Errorf("la cajera devuelve dinero = %d, quiere 403: ver la lista no es sacar dinero del corte: %s", w.Code, w.Body.String())
		}
	})

	t.Run("mesero", func(t *testing.T) {
		w := do(t, r, http.MethodGet, "/api/v1/orders/delivered", token(mesero, domain.RoleMesero), nil, "")
		if w.Code != http.StatusForbidden {
			t.Fatalf("el mesero pide los entregados = %d, quiere 403: no lo trae por omisión", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Tu usuario no puede ver los pedidos entregados") {
			t.Errorf("el rechazo no dice qué no puede hacer: %s", w.Body.String())
		}
	})

	// Separar ver de devolver no le quita nada a quien ya podía.
	for _, c := range []struct {
		uid int64
		rol domain.Role
	}{{gerente, domain.RoleGerente}, {admin, domain.RoleAdmin}} {
		t.Run(string(c.rol), func(t *testing.T) {
			tok := token(c.uid, c.rol)
			pid := nuevoEntregado()
			if w := do(t, r, http.MethodGet, "/api/v1/orders/delivered", tok, nil, ""); w.Code != http.StatusOK {
				t.Fatalf("%s pide los entregados = %d, quiere 200", c.rol, w.Code)
			}
			if w := do(t, r, http.MethodGet, fmt.Sprintf("/api/v1/orders/%d/refund-info", pid), tok, nil, ""); w.Code != http.StatusOK {
				t.Errorf("%s consulta cómo devolver = %d, quiere 200: %s", c.rol, w.Code, w.Body.String())
			}
			if w := do(t, r, http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/refund", pid), tok, refund, "application/json"); w.Code/100 != 2 {
				t.Errorf("%s devuelve dinero = %d, quiere 2xx: %s", c.rol, w.Code, w.Body.String())
			}
		})
	}
}
