//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// EL AJUSTE «EL TABLERO PUEDE COBRAR» SE QUITÓ (decisión del dueño, 2026-10-09; migración 0084).
//
// La columna ya no existe, los ajustes se siguen leyendo y guardando como el rol de la app —una
// consulta que todavía la nombrara respondería 500 en el primer request— y el Down la devuelve con
// su default de siempre, apagado.
func TestKitchenCanChargeIsGone(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	columna := func() int {
		t.Helper()
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from information_schema.columns
			where table_name = 'business_settings' and column_name = 'kitchen_can_charge'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := columna(); n != 0 {
		t.Fatal("la columna kitchen_can_charge sigue en business_settings")
	}

	appSt := appRoleStore(t)
	settings := app.NewSettingsService(appSt, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_kcc", "admin")
	antes, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get como gatobobah_app: %v", err)
	}
	print := domain.PrintSettings{
		AutoPrintOnClose: antes.AutoPrintOnClose, PrintFreeModifiers: antes.PrintFreeModifiers,
		PrintKitchenTicket: true,
	}
	info := domain.BusinessInfo{
		Name: "Negocio de prueba", Address: antes.Address, Phone: antes.Phone,
		HeaderNote: antes.HeaderNote, FooterNote: antes.FooterNote,
	}
	ident := domain.IdentitySettings{
		PinOnlyUnlock: antes.PinOnlyUnlock, LockAfterSeconds: antes.LockAfterSeconds, SessionHours: antes.SessionHours,
	}
	if _, err := settings.SetBusinessInfo(ctx, info, print, ident, antes.Timezone, admin); err != nil {
		t.Fatalf("SetBusinessInfo como gatobobah_app: %v", err)
	}

	_, down, _ := strings.Cut(sqlCompletoDeLaMigracion(t, "0084_drop_kitchen_can_charge.sql"), "-- +goose Down")
	if _, err := st.Pool.Exec(ctx, down); err != nil {
		t.Fatalf("Down: %v", err)
	}
	var encendidos int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from business_settings where kitchen_can_charge`).Scan(&encendidos); err != nil {
		t.Fatalf("el Down no devolvió la columna: %v", err)
	}
	if encendidos != 0 {
		t.Fatalf("el Down dejó %d empresas con el tablero cobrando: el default es apagado", encendidos)
	}
}

func sqlCompletoDeLaMigracion(t *testing.T, nombre string) string {
	t.Helper()
	crudo, err := os.ReadFile(filepath.Join("..", "..", "migrations", nombre))
	if err != nil {
		t.Fatalf("leer la migración: %v", err)
	}
	return string(crudo)
}

// El aviso del POS lista lo que falta por cobrar, en cualquier estado que siga siendo cobrable.
// Existe aparte de la lista de entregadas porque esa es de admin/gerente, y el pendiente más caro
// —entregado y sin cobrar, el cliente ya se fue— tiene que poder saldarlo quien está en la caja.
func TestElAvisoDelPOSListaLoQueFaltaPorCobrar(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_unpaid", "cajero")
	prod := makeProduct(t, st, "Café unpaid", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	nuevo := func(pagos []app.PaymentInput) *app.OrderView {
		t.Helper()
		o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: pagos,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return o
	}

	pagado := nuevo([]app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}})
	sinCobrar := nuevo(nil)
	abonado := nuevo([]app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("20")}})
	cancelado := nuevo(nil)
	if err := svc.Cancel(ctx, cancelado.ID, cajero, "se arrepintió"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	fila, err := app.NewAccountsService(st, svc).Live(ctx, false)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	falta := map[int64]string{}
	for _, o := range fila.Items {
		if o.OrderID != nil {
			falta[*o.OrderID] = o.Outstanding.String()
		}
	}

	// El pedido ya cobrado SÍ sale, y debe cero. La lista dejó de ser solo de impagos cuando pasó a
	// alimentar la barra de pedidos en curso: el pedido cobrado que sigue en cocina es justo al que
	// el cliente le pide algo más, y esconderlo lo devolvería al agujero del que esta feature lo
	// sacó. Lo que no puede es aparecer debiendo dinero que ya se pagó.
	if falta[pagado.ID] != "0" {
		t.Errorf("el pedido cobrado y todavía en cocina dice deber %q, quiere 0", falta[pagado.ID])
	}
	// Su dinero ya se decidió: listarlo mandaría al operador a perseguir un cobro que nadie debe.
	if _, hay := falta[cancelado.ID]; hay {
		t.Error("un pedido cancelado salió en la lista de pendientes")
	}
	if falta[sinCobrar.ID] != "50" {
		t.Errorf("el pedido sin cobrar debe 50, dice %q", falta[sinCobrar.ID])
	}
	// Lo que FALTA, no el total: cobrarle 50 a quien ya dejó 20 es cobrarle dos veces esa parte.
	if falta[abonado.ID] != "30" {
		t.Errorf("el pedido abonado debe 30, dice %q", falta[abonado.ID])
	}
}
