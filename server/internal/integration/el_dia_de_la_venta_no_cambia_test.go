//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// NINGUNA VENTA CAMBIA DE DÍA. ES LA INVARIANTE QUE HACE SEGURA TODA LA FEATURE.
//
// La feature 006 cambia QUÉ SE MUESTRA —la hora en pantalla, hasta cuándo se ven los entregados, qué
// pedidos siguen en la barra— y no en qué día cae una venta. Si algo de eso llegara a tocar
// `business_date`, movería dinero entre arqueos: el turno de ayer cuadraría distinto y nadie sabría
// por qué hasta el corte siguiente.
//
// Estaba solo como paso manual del recorrido de verificación. Un paso manual se salta, y este se
// salta justo cuando hay prisa por desplegar.
func TestNingunaVentaCambiaDeDia(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	settings := app.NewSettingsService(st, "pepper-de-prueba")

	cajero := makeUser(t, st, "cajero_invariante", "cajero")
	admin := makeUser(t, st, "admin_invariante", "admin")
	prod := makeProduct(t, st, "Café invariante", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	// Tres pedidos en estados distintos: abierto, entregado sin cobrar, y entregado y cobrado.
	var ids []int64
	for i, pagar := range []bool{false, false, true} {
		cmd := app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		}
		if pagar {
			cmd.Payments = []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}}
		}
		o, err := crearYCobrar(t, ctx, svc, cmd)
		if err != nil {
			t.Fatalf("sembrar pedido %d: %v", i, err)
		}
		if i > 0 {
			if err := svc.DeliverAll(ctx, o.ID); err != nil {
				t.Fatalf("entregar %d: %v", i, err)
			}
		}
		ids = append(ids, o.ID)
	}

	antes := fechasDeNegocio(t, st, ids)

	// Todo lo que la feature toca, corrido de punta a punta.
	if _, _, err := svc.Open(ctx, false); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := svc.DeliveredToday(ctx); err != nil {
		t.Fatalf("DeliveredToday: %v", err)
	}
	if _, err := back.CurrentByRegister(ctx, registerID(t, st, "Caja principal")); err != nil {
		t.Fatalf("arqueo: %v", err)
	}
	// Y el cambio de zona, que es lo que más se parece a mover el día de una venta.
	prev, _ := settings.Get(ctx)
	info := domain.BusinessInfo{Name: prev.BusinessName, Address: prev.Address, Phone: prev.Phone}
	if _, err := settings.SetBusinessInfo(ctx, info, domain.PrintSettings{}, domain.DefaultIdentity(),
		"America/Tijuana", admin); err != nil {
		t.Fatalf("cambiar la zona: %v", err)
	}

	despues := fechasDeNegocio(t, st, ids)
	for id, fecha := range antes {
		if !despues[id].Equal(fecha) {
			t.Errorf("el pedido %d pasó del día %s al %s: se movió dinero de un arqueo a otro",
				id, fecha.Format("2006-01-02"), despues[id].Format("2006-01-02"))
		}
	}
}

func fechasDeNegocio(t *testing.T, st *store.Store, ids []int64) map[int64]time.Time {
	t.Helper()
	out := map[int64]time.Time{}
	for _, id := range ids {
		var f time.Time
		if err := st.Pool.QueryRow(context.Background(),
			`select business_date from orders where id = $1`, id).Scan(&f); err != nil {
			t.Fatalf("leer la fecha del pedido %d: %v", id, err)
		}
		out[id] = f
	}
	return out
}
