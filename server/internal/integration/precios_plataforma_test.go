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
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Una venta por plataforma se valúa con la lista de ESA plataforma, y el servidor la recalcula: el
// precio que mande el cliente se ignora, igual que en mostrador.
func TestVentaPorPlataformaUsaSuLista(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_plat", "cajero")
	prod := makeProduct(t, st, "Boneless", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, cajero)

	// Uber trae 35% sembrado por la migración: 100 → 135.
	ord, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:         uuid.New(),
		ServiceType:        "domicilio",
		DeliveryPlatformID: &uber,
		OpenedBy:           cajero,
		Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:           []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("135")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ord.Total.Equal(decimal.RequireFromString("135")) {
		t.Fatalf("la venta por Uber debe cobrar 135 (100 + 35%%), cobró %s", ord.Total)
	}
	if !ord.Lines[0].UnitPrice.Equal(decimal.RequireFromString("135")) {
		t.Fatalf("el unitario guardado debe ser el de la lista: %s", ord.Lines[0].UnitPrice)
	}
}

// El precio capturado a mano gana sobre el calculado, y PERSISTE: la siguiente venta en esa
// plataforma ya entra con él. Es lo que convierte corregir un precio en trabajo de una sola vez.
func TestPrecioManualGanaYPersiste(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_manual", "cajero")
	prod := makeProduct(t, st, "Alitas", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	rappi := platformID(t, st, defaultCompanyID, "Rappi")
	abrirCajaPrincipal(t, st, cajero)

	if err := st.Q.UpsertProductPlatformPrice(ctx, db.UpsertProductPlatformPriceParams{
		ProductID: prod, PlatformID: rappi, Price: decimal.RequireFromString("149"), UpdatedBy: cajero,
	}); err != nil {
		t.Fatalf("capturar el precio: %v", err)
	}

	ord, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:         uuid.New(),
		ServiceType:        "domicilio",
		DeliveryPlatformID: &rappi,
		OpenedBy:           cajero,
		Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:           []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("149")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ord.Total.Equal(decimal.RequireFromString("149")) {
		t.Fatalf("el precio capturado debe ganar sobre el calculado (135): cobró %s", ord.Total)
	}

	// Y no contamina: en mostrador el mismo producto sigue en su base.
	enMostrador, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cajero,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
	})
	if err != nil {
		t.Fatalf("Create mostrador: %v", err)
	}
	if !enMostrador.Total.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("mostrador debe cobrar el base: cobró %s", enMostrador.Total)
	}
}

// Una plataforma que no es de esta empresa se rechaza con 422. NUNCA se cae a margen 0: eso
// cobraría precio de mostrador en Uber, con el ticket bien impreso, y el descuadre aparecería
// semanas después al conciliar el depósito.
func TestPlataformaAjenaSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	appSt := appRoleStore(t)
	ordersSvc := app.NewOrdersService(appSt, clock)

	otra := makeCompany(t, st, "otra-plataforma")
	cajero := makeUser(t, st, "cajero_ajena", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	ajena := platformID(t, st, otra, "Uber Eats")

	tenantCtx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()

	_, err = ordersSvc.Create(tenantCtx, app.CreateOrderCmd{
		ClientUUID:         uuid.New(),
		ServiceType:        "domicilio",
		DeliveryPlatformID: &ajena,
		OpenedBy:           cajero,
		Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:           []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
	})
	if !errors.Is(err, domain.ErrPlatformNotFound) {
		t.Fatalf("una plataforma ajena debe rechazarse, fue %v", err)
	}
}

// El reparto lo cobra la plataforma: el costo de envío del negocio se fuerza a 0 aunque el cliente
// mande otra cosa. Sin esto, cada pedido de Uber saldría con $20 de más.
func TestPedidoDePlataformaNoCobraEnvio(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_envio", "cajero")
	prod := makeProduct(t, st, "Pizza", decimal.RequireFromString("200"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	didi := platformID(t, st, defaultCompanyID, "Didi")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:         uuid.New(),
		ServiceType:        "domicilio",
		DeliveryPlatformID: &didi,
		DeliveryFee:        decimal.RequireFromString("20"), // el cliente lo manda; se ignora
		OpenedBy:           cajero,
		Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:           []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("270")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ord.DeliveryFee.IsZero() {
		t.Fatalf("un pedido de plataforma no cobra envío del negocio, cobró %s", ord.DeliveryFee)
	}
	if !ord.Total.Equal(decimal.RequireFromString("270")) {
		t.Fatalf("total = %s, quería 270 (200 + 35%%)", ord.Total)
	}
}
