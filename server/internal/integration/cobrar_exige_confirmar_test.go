//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// NO SE COBRA UN PEDIDO QUE COCINA NO VIO.
//
// Crear y cobrar de un golpe era el camino corto —y el que se usaba—, así que la comanda no salía
// nunca. La barrera vive en el SERVIDOR y no en la pantalla: esconder el botón no es una barrera, y
// el principio V es explícito en que el front es espejo.
func TestCrearUnPedidoYaCobradoSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_exige", "cajero")
	cafe := makeProduct(t, st, "Café exige", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	linea := []domain.OrderLineInput{{ProductID: cafe, Qty: decimal.RequireFromString("1")}}

	_, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    linea,
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
	})
	if !errors.Is(err, domain.ErrCobroFueraDeLugar) {
		t.Fatalf("crear con pagos = %v, quiere ErrCobroFueraDeLugar: el camino corto se salta la cocina", err)
	}

	// Y el camino bueno funciona: se confirma, y se cobra aparte.
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero, Lines: linea,
	})
	if err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("100"),
		Tip: decimal.Zero, ActorID: cajero,
	}); err != nil {
		t.Fatalf("cobrar el pedido confirmado: %v", err)
	}
}

// Un pedido de cero renglones ocuparía un folio, aparecería en la barra y sacaría una comanda en
// blanco. Ya se rechazaba; el test lo fija ahora que confirmar es obligatorio y el camino se usa en
// cada venta.
func TestConfirmarUnaCuentaVaciaSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_vacia", "cajero")
	abrirCajaPrincipal(t, st, cajero)

	_, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
	})
	if err == nil {
		t.Fatal("se creó un pedido sin renglones: ocupa folio y saca una comanda en blanco")
	}
}

// LOS PEDIDOS QUE YA EXISTÍAN SIGUEN FUNCIONANDO.
//
// Nacieron sin pasar por "confirmar" —el concepto no existía— y en producción hay varios. Mover la
// barrera no puede dejarlos sin poder cobrarse ni entregarse: el negocio está en operación.
func TestUnPedidoAnteriorSigueSiendoCobrableYEntregable(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_viejo", "cajero")
	cafe := makeProduct(t, st, "Café viejo", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	sesion := abrirCajaPrincipal(t, st, cajero)

	// Sembrado como owner, saltándose el servicio: es la forma de un pedido de antes de la feature.
	var orderID int64
	if err := st.Pool.QueryRow(ctx, `
		insert into orders (company_id, client_uuid, business_date, daily_number, service_type,
		                    subtotal, total, opened_by, register_session_id)
		values ($1, gen_random_uuid(), current_date, 9201, 'mostrador', 100, 100, $2, $3)
		returning id`, defaultCompanyID, cajero, sesion).Scan(&orderID); err != nil {
		t.Fatalf("sembrar el pedido viejo: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `
		insert into order_lines (company_id, order_id, product_id, product_name, quantity, unit_price, line_total)
		values ($1, $2, $3, 'Café viejo', 1, 100, 100)`, defaultCompanyID, orderID, cafe); err != nil {
		t.Fatalf("sembrar el renglón viejo: %v", err)
	}

	if err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: orderID, MethodID: efectivo, Amount: decimal.RequireFromString("100"),
		Tip: decimal.Zero, ActorID: cajero,
	}); err != nil {
		t.Errorf("un pedido anterior a la feature dejó de ser cobrable: %v", err)
	}
	if err := svc.DeliverAll(ctx, orderID); err != nil {
		t.Errorf("un pedido anterior a la feature dejó de ser entregable: %v", err)
	}
}
