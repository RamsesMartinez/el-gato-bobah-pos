//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// Agregar renglones a un pedido que ya está en curso.
//
// Es el caso de todos los días en el local: la libreta vuelve de la mesa con "la 3 pidió dos más".
// Hasta ahora obligaba a abrir un segundo pedido —dos folios y dos tickets para el mismo cliente,
// y el corte contando dos ventas donde hubo una— o a cancelar y rehacer.
func TestAgregarRenglonesAUnPedidoEnCurso(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_agregar", "cajero")
	prod := makeProduct(t, st, "Café agregar", decimal.RequireFromString("50"), false)
	otro := makeProduct(t, st, "Pan agregar", decimal.RequireFromString("30"), false)
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	act, err := svc.AddLines(ctx, ord.ID, []domain.OrderLineInput{
		{ProductID: otro, Qty: decimal.RequireFromString("2")},
	}, cajero)
	if err != nil {
		t.Fatalf("AddLines: %v", err)
	}

	// 50 + 30×2 = 110. El total lo recalcula el SERVIDOR sobre lo que quedó en el pedido.
	if !act.Total.Equal(decimal.RequireFromString("110")) {
		t.Fatalf("total = %s, quiere 110", act.Total)
	}
	if len(act.Lines) != 2 {
		t.Fatalf("renglones = %d, quiere 2", len(act.Lines))
	}
	// Mismo folio: es el mismo cliente y la misma cuenta.
	if act.Number != ord.Number {
		t.Fatalf("el folio cambió de %d a %d", ord.Number, act.Number)
	}
}

// El stock se descuenta SOLO de lo nuevo. Si se recalculara sobre el pedido entero, cada agregado
// descontaría otra vez lo que ya se había descontado y el inventario se iría al piso sin que nadie
// entendiera por qué.
func TestAgregarDescuentaSoloElStockDeLoNuevo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_stock_agregar", "cajero")
	prod := makeProduct(t, st, "Refresco stock", decimal.RequireFromString("25"), true)
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.AddLines(ctx, ord.ID, []domain.OrderLineInput{
		{ProductID: prod, Qty: decimal.RequireFromString("3")},
	}, cajero); err != nil {
		t.Fatalf("AddLines: %v", err)
	}

	// Cuatro en total, no cinco: uno del original más tres del agregado.
	var salidas decimal.Decimal
	if err := st.Pool.QueryRow(ctx,
		`select coalesce(-sum(quantity), 0) from stock_movements where order_id = $1`, ord.ID).Scan(&salidas); err != nil {
		t.Fatalf("leer movimientos: %v", err)
	}
	if !salidas.Equal(decimal.RequireFromString("4")) {
		t.Fatalf("stock descontado = %s, quiere 4: se recalculó sobre el pedido entero", salidas)
	}
}

// A un pedido terminado no se le agrega nada: su venta ya entró al corte y su ticket ya está en
// manos del cliente. Cambiarle el total después es mover dinero que ya se contó.
func TestNoSeAgregaAUnPedidoTerminado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_terminado", "cajero")
	prod := makeProduct(t, st, "Café terminado", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero, Delivered: true,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.AddLines(ctx, ord.ID, []domain.OrderLineInput{
		{ProductID: prod, Qty: decimal.RequireFromString("1")},
	}, cajero)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("agregar a un pedido entregado debe rechazarse, fue: %v", err)
	}
}

// Sin renglones no hay nada que agregar: un cuerpo vacío que pasara dejaría el pedido intacto y la
// pantalla creyendo que agregó algo.
func TestAgregarSinRenglonesSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_vacio", "cajero")
	prod := makeProduct(t, st, "Café vacío", decimal.RequireFromString("50"), false)
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.AddLines(ctx, ord.ID, nil, cajero); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("agregar nada debe rechazarse, fue: %v", err)
	}
}

// La ruta, por el ROUTER real: que exista, que exija sesión y que el delta llegue completo.
//
// Es una ruta propia y no un PATCH del pedido porque lo que se manda es lo que el cliente pidió de
// MÁS. Un PATCH invitaría a mandar la lista entera, y el servidor tendría que adivinar qué renglón
// es nuevo para no volver a descontar su stock.
func TestLaRutaDeAgregarRenglones(t *testing.T) {
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Orders: app.NewOrdersService(st, clock), Broker: realtime.NewBroker(),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)

	cajeroID := makeUser(t, st, "cajero_ruta_agregar", "cajero")
	tok, err := jm.Issue(domain.User{ID: cajeroID, CompanyID: defaultCompanyID, Name: "cajero", Role: domain.RoleCajero})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	prod := makeProduct(t, st, "Café ruta", decimal.RequireFromString("50"), false)
	abrirCajaPrincipal(t, st, cajeroID)

	ord, err := app.NewOrdersService(st, clock).Create(context.Background(), app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajeroID,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ruta := fmt.Sprintf("/api/v1/orders/%d/lines", ord.ID)
	cuerpo := []byte(fmt.Sprintf(`{"lines":[{"productId":%d,"qty":2,"modifiers":[]}]}`, prod))

	if w := do(t, r, http.MethodPost, ruta, "", cuerpo, "application/json"); w.Code != http.StatusUnauthorized {
		t.Fatalf("sin token = %d, quiere 401", w.Code)
	}

	w := do(t, r, http.MethodPost, ruta, tok, cuerpo, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("agregar = %d: %s", w.Code, w.Body.String())
	}
	var view app.OrderView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("respuesta: %v (%s)", err, w.Body.String())
	}
	// 50 + 50×2 = 150, recalculado por el servidor sobre los renglones guardados.
	if !view.Total.Equal(decimal.RequireFromString("150")) {
		t.Fatalf("total = %s, quiere 150", view.Total)
	}
}
