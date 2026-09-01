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

// El método de pago de un pedido de plataforma tiene que ser el de ESA plataforma.
//
// No es una formalidad: el corte agrupa el dinero por método, y el método dice si el importe entra
// al cajón. Cobrar un pedido de Uber con el "Efectivo" de mostrador hace que el sistema espere
// billetes que nunca existieron —la plataforma pagó por transferencia— y el turno cierra con un
// faltante por el monto exacto, sin nada que lo explique. Ya pasó una vez en producción por otra
// causa parecida y costó una tarde reconstruirlo.
//
// El caso contrario importa igual: un pedido de mostrador cobrado con "Uber Eats en línea" saca del
// cajón dinero que sí estaba ahí, y el turno cierra con sobrante.
func TestUnPedidoDePlataformaExigeElMetodoDeSuPlataforma(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_metodo_plat", "cajero")
	prod := makeProduct(t, st, "Alitas plataforma", decimal.RequireFromString("100"), false)
	abrirCajaPrincipal(t, st, cajero)

	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	efectivo := paymentMethodID(t, st, "Efectivo")
	uberEnLinea := paymentMethodID(t, st, "Uber Eats en línea")
	uberEfectivo := paymentMethodID(t, st, "Uber Eats efectivo")
	didiEnLinea := paymentMethodID(t, st, "Didi en línea")

	// El precio de la lista de Uber: 100 con el 35% sembrado = 135.
	pedido := func(plataforma *int16, metodo int16, monto string) error {
		_, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID:         uuid.New(),
			ServiceType:        "domicilio",
			OpenedBy:           cajero,
			DeliveryPlatformID: plataforma,
			Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments:           []app.PaymentInput{{MethodID: metodo, Amount: decimal.RequireFromString(monto)}},
		})
		return err
	}

	t.Run("con el método de su plataforma pasa, en línea y en efectivo", func(t *testing.T) {
		if err := pedido(&uber, uberEnLinea, "135"); err != nil {
			t.Fatalf("Uber en línea debe poder cobrar un pedido de Uber: %v", err)
		}
		// El repartidor a veces paga en efectivo: es el motivo de que exista este método.
		if err := pedido(&uber, uberEfectivo, "135"); err != nil {
			t.Fatalf("Uber efectivo debe poder cobrar un pedido de Uber: %v", err)
		}
	})

	t.Run("con el método de OTRA plataforma se rechaza", func(t *testing.T) {
		if err := pedido(&uber, didiEnLinea, "135"); !errors.Is(err, domain.ErrPaymentMethodPlatform) {
			t.Fatalf("Didi no debe poder cobrar un pedido de Uber, fue: %v", err)
		}
	})

	t.Run("con el efectivo de mostrador se rechaza", func(t *testing.T) {
		if err := pedido(&uber, efectivo, "135"); !errors.Is(err, domain.ErrPaymentMethodPlatform) {
			t.Fatalf("el efectivo de mostrador no debe cobrar un pedido de plataforma, fue: %v", err)
		}
	})

	t.Run("un pedido de mostrador no se cobra con un método de plataforma", func(t *testing.T) {
		if err := pedido(nil, uberEnLinea, "100"); !errors.Is(err, domain.ErrPaymentMethodPlatform) {
			t.Fatalf("Uber en línea no debe cobrar un pedido sin plataforma, fue: %v", err)
		}
	})

	t.Run("un pedido de mostrador sí se cobra con efectivo", func(t *testing.T) {
		if err := pedido(nil, efectivo, "100"); err != nil {
			t.Fatalf("el caso de todos los días no puede romperse: %v", err)
		}
	})

	// Un pago dividido con un método bueno y uno malo se rechaza entero. Aceptar la mitad dejaría
	// el pedido pagado a medias con dinero en el método equivocado, que es peor que no cobrarlo.
	t.Run("en un pago dividido basta un método ajeno para rechazar todo", func(t *testing.T) {
		_, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID:         uuid.New(),
			ServiceType:        "domicilio",
			OpenedBy:           cajero,
			DeliveryPlatformID: &uber,
			Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: []app.PaymentInput{
				{MethodID: uberEnLinea, Amount: decimal.RequireFromString("35")},
				{MethodID: didiEnLinea, Amount: decimal.RequireFromString("100")},
			},
		})
		if !errors.Is(err, domain.ErrPaymentMethodPlatform) {
			t.Fatalf("un método ajeno en el pago dividido debe rechazar todo, fue: %v", err)
		}
	})
}
